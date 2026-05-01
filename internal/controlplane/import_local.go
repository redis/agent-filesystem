package controlplane

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ImportLocalRequest describes a local directory to import as a workspace.
type ImportLocalRequest struct {
	DatabaseID  string `json:"database_id,omitempty"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

// ImportLocalResponse is returned on successful local import.
type ImportLocalResponse struct {
	WorkspaceID string          `json:"workspace_id"`
	Workspace   workspaceDetail `json:"workspace"`
	FileCount   int             `json:"file_count"`
	DirCount    int             `json:"dir_count"`
	TotalBytes  int64           `json:"total_bytes"`
}

// ImportLocal creates a workspace from a directory on the local filesystem.
// It mirrors the behaviour of `afs ws import` but runs via the HTTP API.
func (m *DatabaseManager) ImportLocal(ctx context.Context, databaseID string, input ImportLocalRequest) (ImportLocalResponse, error) {
	workspace := strings.TrimSpace(input.Name)
	if err := ValidateName("workspace", workspace); err != nil {
		return ImportLocalResponse{}, err
	}

	dirPath := strings.TrimSpace(input.Path)
	if dirPath == "" {
		return ImportLocalResponse{}, fmt.Errorf("path is required")
	}

	// Expand ~ to home directory.
	if strings.HasPrefix(dirPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ImportLocalResponse{}, fmt.Errorf("cannot expand ~: %w", err)
		}
		dirPath = filepath.Join(home, dirPath[2:])
	}

	info, err := os.Stat(dirPath)
	if err != nil {
		return ImportLocalResponse{}, fmt.Errorf("cannot access %s: %w", dirPath, err)
	}
	if !info.IsDir() {
		return ImportLocalResponse{}, fmt.Errorf("%s is not a directory", dirPath)
	}

	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return ImportLocalResponse{}, err
	}

	// Check if workspace already exists.
	exists, err := service.store.WorkspaceExists(ctx, workspace)
	if err != nil {
		return ImportLocalResponse{}, err
	}
	if exists {
		return ImportLocalResponse{}, fmt.Errorf("workspace %q already exists", workspace)
	}

	// Build manifest by walking the directory.
	now := time.Now().UTC()
	manifest, fileCount, dirCount, totalBytes, err := buildManifestFromDirectory(dirPath, workspace, now)
	if err != nil {
		return ImportLocalResponse{}, fmt.Errorf("scan directory: %w", err)
	}

	manifestHash, err := HashManifest(manifest)
	if err != nil {
		return ImportLocalResponse{}, err
	}
	workspaceID, err := newOpaqueWorkspaceID()
	if err != nil {
		return ImportLocalResponse{}, err
	}
	manifest.Workspace = workspaceID

	description := strings.TrimSpace(input.Description)
	if description == "" {
		description = fmt.Sprintf("Imported from %s.", dirPath)
	}

	meta := WorkspaceMeta{
		Version:          formatVersion,
		ID:               workspaceID,
		Name:             workspace,
		Description:      description,
		DatabaseID:       profile.ID,
		DatabaseName:     profile.Name,
		CloudAccount:     "Direct Redis",
		Source:           SourceGitImport,
		Tags:             WorkspaceTags("", SourceGitImport),
		CreatedAt:        now,
		UpdatedAt:        now,
		HeadSavepoint:    initialCheckpointName,
		DefaultSavepoint: initialCheckpointName,
	}

	checkpoint := SavepointMeta{
		Version:      formatVersion,
		ID:           initialCheckpointName,
		Name:         initialCheckpointName,
		Description:  "Initial import snapshot.",
		Kind:         CheckpointKindImport,
		Source:       CheckpointSourceImport,
		Author:       "afs",
		Workspace:    workspaceID,
		ManifestHash: manifestHash,
		CreatedAt:    now,
		FileCount:    fileCount,
		DirCount:     dirCount,
		TotalBytes:   totalBytes,
	}

	store := service.store
	if err := store.PutWorkspaceMeta(ctx, meta); err != nil {
		return ImportLocalResponse{}, err
	}
	if err := store.PutSavepoint(ctx, checkpoint, manifest); err != nil {
		return ImportLocalResponse{}, err
	}
	if err := SyncWorkspaceRoot(ctx, store, workspaceID, manifest); err != nil {
		return ImportLocalResponse{}, err
	}
	template := service.buildChangelogTemplate(ctx, workspaceID, initialCheckpointName, ChangeSourceImport)
	writeChangeEntries(ctx, store.rdb, workspaceID, manifestSeedEntries(manifest, template))
	if err := store.Audit(ctx, workspaceID, "import", map[string]any{
		"checkpoint":  initialCheckpointName,
		"source":      dirPath,
		"files":       fileCount,
		"dirs":        dirCount,
		"total_bytes": totalBytes,
	}); err != nil {
		return ImportLocalResponse{}, err
	}

	detail, err := service.getWorkspace(ctx, workspaceID)
	if err != nil {
		return ImportLocalResponse{}, err
	}
	if err := m.attachWorkspaceDetailIdentity(ctx, &detail, profile); err != nil {
		return ImportLocalResponse{}, err
	}

	return ImportLocalResponse{
		WorkspaceID: detail.ID,
		Workspace:   detail,
		FileCount:   fileCount,
		DirCount:    dirCount,
		TotalBytes:  totalBytes,
	}, nil
}

func (m *DatabaseManager) ImportResolvedLocal(ctx context.Context, input ImportLocalRequest) (ImportLocalResponse, error) {
	profile, err := m.resolveTargetDatabase(ctx, input.DatabaseID)
	if err != nil {
		return ImportLocalResponse{}, err
	}
	return m.ImportLocal(ctx, profile.ID, input)
}

// buildManifestFromDirectory walks a local directory and builds a Manifest.
// For simplicity and safety via the web UI, it inlines all file content and
// skips files larger than 10MB and common non-essential directories.
func buildManifestFromDirectory(root, workspace string, now time.Time) (Manifest, int, int, int64, error) {
	const maxFileSize = 10 * 1024 * 1024 // 10 MB

	entries := make(map[string]ManifestEntry)
	ms := now.UnixMilli()
	var fileCount, dirCount int
	var totalBytes int64

	// Always include root dir.
	entries["/"] = ManifestEntry{
		Type:    "dir",
		Mode:    0o755,
		MtimeMs: ms,
		Size:    0,
	}
	dirCount++

	skipDirs := map[string]bool{
		".git":          true,
		"node_modules":  true,
		".afs":          true,
		"__pycache__":   true,
		".tox":          true,
		".pytest_cache": true,
		"vendor":        true,
		".venv":         true,
		"venv":          true,
	}

	err := filepath.WalkDir(root, func(absPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries
		}

		relPath, err := filepath.Rel(root, absPath)
		if err != nil {
			return nil
		}
		if relPath == "." {
			return nil
		}

		// Skip common ignored directories.
		if d.IsDir() && skipDirs[d.Name()] {
			return filepath.SkipDir
		}

		manifestPath := "/" + filepath.ToSlash(relPath)

		if d.IsDir() {
			dirCount++
			entries[manifestPath] = ManifestEntry{
				Type:    "dir",
				Mode:    0o755,
				MtimeMs: ms,
				Size:    0,
			}
			return nil
		}

		// Handle symlinks.
		if d.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(absPath)
			if err != nil {
				return nil
			}
			fileCount++
			entries[manifestPath] = ManifestEntry{
				Type:    "symlink",
				Mode:    0o777,
				MtimeMs: ms,
				Target:  target,
			}
			return nil
		}

		// Regular file.
		info, err := d.Info()
		if err != nil {
			return nil
		}

		size := info.Size()
		if size > maxFileSize {
			return nil // skip very large files
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			return nil
		}

		fileCount++
		totalBytes += size
		entries[manifestPath] = ManifestEntry{
			Type:    "file",
			Mode:    0o644,
			MtimeMs: info.ModTime().UnixMilli(),
			Size:    size,
			Inline:  base64.StdEncoding.EncodeToString(data),
		}
		return nil
	})

	if err != nil {
		return Manifest{}, 0, 0, 0, err
	}

	manifest := Manifest{
		Version:   formatVersion,
		Workspace: workspace,
		Savepoint: initialCheckpointName,
		Entries:   entries,
	}

	return manifest, fileCount, dirCount, totalBytes, nil
}
