package controlplane

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	afsclient "github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

type FileVersionSelector struct {
	VersionID string `json:"version_id,omitempty"`
	FileID    string `json:"file_id,omitempty"`
	Ordinal   int64  `json:"ordinal,omitempty"`
}

type FileVersionRestoreResponse struct {
	WorkspaceID           string `json:"workspace_id"`
	Path                  string `json:"path"`
	Dirty                 bool   `json:"dirty"`
	FileID                string `json:"file_id,omitempty"`
	VersionID             string `json:"version_id,omitempty"`
	RestoredFromVersionID string `json:"restored_from_version_id,omitempty"`
	RestoredFromFileID    string `json:"restored_from_file_id,omitempty"`
	RestoredFromOrdinal   int64  `json:"restored_from_ordinal,omitempty"`
}

type FileVersionUndeleteResponse struct {
	WorkspaceID            string `json:"workspace_id"`
	Path                   string `json:"path"`
	Dirty                  bool   `json:"dirty"`
	FileID                 string `json:"file_id,omitempty"`
	VersionID              string `json:"version_id,omitempty"`
	UndeletedFromVersionID string `json:"undeleted_from_version_id,omitempty"`
	UndeletedFromFileID    string `json:"undeleted_from_file_id,omitempty"`
	UndeletedFromOrdinal   int64  `json:"undeleted_from_ordinal,omitempty"`
}

func (s *Service) RestoreFileVersion(ctx context.Context, workspace, rawPath string, selector FileVersionSelector) (FileVersionRestoreResponse, error) {
	normalizedPath, err := normalizeVersionedPath(rawPath)
	if err != nil {
		return FileVersionRestoreResponse{}, err
	}
	selected, err := s.resolveSelectedFileVersion(ctx, workspace, selector)
	if err != nil {
		return FileVersionRestoreResponse{}, err
	}
	if selected.Kind == FileVersionKindTombstone {
		return FileVersionRestoreResponse{}, fmt.Errorf("version %q is a tombstone; use undelete instead", selected.VersionID)
	}
	if normalizedPath != selected.Path {
		return FileVersionRestoreResponse{}, fmt.Errorf("version %q belongs to %q, not %q", selected.VersionID, selected.Path, normalizedPath)
	}

	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return FileVersionRestoreResponse{}, err
	}
	storageID := workspaceStorageID(meta)
	fsKey, _, _, err := EnsureWorkspaceRoot(ctx, s.store, workspace)
	if err != nil {
		return FileVersionRestoreResponse{}, err
	}
	fsClient := afsclient.New(s.store.rdb, fsKey)
	stat, err := fsClient.Stat(ctx, normalizedPath)
	if err != nil && !errors.Is(err, redis.Nil) {
		return FileVersionRestoreResponse{}, err
	}
	if errors.Is(err, redis.Nil) {
		stat = nil
	}
	if stat != nil && stat.Type == "dir" {
		return FileVersionRestoreResponse{}, fmt.Errorf("path %q is a directory", normalizedPath)
	}

	beforeSnapshot, err := fileVersionedSnapshotFromFS(ctx, fsClient, normalizedPath, stat)
	if err != nil {
		return FileVersionRestoreResponse{}, err
	}
	if err := applySelectedVersionToWorkspacePath(ctx, s.store, workspace, fsClient, normalizedPath, stat, selected); err != nil {
		return FileVersionRestoreResponse{}, err
	}
	if err := MarkWorkspaceRootDirty(ctx, s.store, storageID); err != nil {
		return FileVersionRestoreResponse{}, err
	}
	meta.DirtyHint = true
	if err := s.store.PutWorkspaceMeta(ctx, meta); err != nil {
		return FileVersionRestoreResponse{}, err
	}

	updatedStat, err := fsClient.Stat(ctx, normalizedPath)
	if err != nil && !errors.Is(err, redis.Nil) {
		return FileVersionRestoreResponse{}, err
	}
	if errors.Is(err, redis.Nil) {
		updatedStat = nil
	}
	afterSnapshot, err := fileVersionedSnapshotFromFS(ctx, fsClient, normalizedPath, updatedStat)
	if err != nil {
		return FileVersionRestoreResponse{}, err
	}
	template := s.buildChangelogTemplate(ctx, storageID, strings.TrimSpace(meta.HeadSavepoint), ChangeSourceVersionRestore)
	restored, err := s.store.RecordFileVersionMutation(ctx, storageID, beforeSnapshot, afterSnapshot, FileVersionMutationMetadata{
		Source:       ChangeSourceVersionRestore,
		SessionID:    template.SessionID,
		AgentID:      template.AgentID,
		User:         template.User,
		CheckpointID: strings.TrimSpace(meta.HeadSavepoint),
	})
	if err != nil {
		return FileVersionRestoreResponse{}, err
	}
	entry := template
	entry.Path = normalizedPath
	entry.Op = ChangeOpPut
	if afterSnapshot.Kind == "symlink" {
		entry.Op = ChangeOpSymlink
	}
	entry.PrevHash = beforeSnapshot.ContentHash
	entry.DeltaBytes = -beforeSnapshot.SizeBytes
	if afterSnapshot.Exists {
		entry.ContentHash = afterSnapshot.ContentHash
		entry.SizeBytes = afterSnapshot.SizeBytes
		entry.DeltaBytes = afterSnapshot.SizeBytes - beforeSnapshot.SizeBytes
		entry.Mode = afterSnapshot.Mode
	}
	if restored != nil {
		entry.FileID = restored.FileID
		entry.VersionID = restored.VersionID
	}
	WriteChangeEntries(ctx, s.store.rdb, storageID, []ChangeEntry{entry})

	response := FileVersionRestoreResponse{
		WorkspaceID:           workspace,
		Path:                  normalizedPath,
		Dirty:                 true,
		RestoredFromVersionID: selected.VersionID,
		RestoredFromFileID:    selected.FileID,
		RestoredFromOrdinal:   selected.Ordinal,
	}
	if restored != nil {
		response.FileID = restored.FileID
		response.VersionID = restored.VersionID
	}
	return response, nil
}

func (s *Service) resolveSelectedFileVersion(ctx context.Context, workspace string, selector FileVersionSelector) (FileVersion, error) {
	switch {
	case strings.TrimSpace(selector.VersionID) != "":
		return s.store.GetFileVersion(ctx, workspace, selector.VersionID)
	case strings.TrimSpace(selector.FileID) != "" && selector.Ordinal > 0:
		return s.store.GetFileVersionAtOrdinal(ctx, workspace, selector.FileID, selector.Ordinal)
	default:
		return FileVersion{}, fmt.Errorf("version_id or file_id+ordinal is required")
	}
}

func (s *Service) UndeleteFileVersion(ctx context.Context, workspace, rawPath string, selector FileVersionSelector) (FileVersionUndeleteResponse, error) {
	normalizedPath, err := normalizeVersionedPath(rawPath)
	if err != nil {
		return FileVersionUndeleteResponse{}, err
	}
	selected, lineage, err := s.resolveUndeleteTargetVersion(ctx, workspace, normalizedPath, selector)
	if err != nil {
		return FileVersionUndeleteResponse{}, err
	}
	if lineage.State != FileLineageStateDeleted {
		return FileVersionUndeleteResponse{}, fmt.Errorf("file lineage %q is not deleted", lineage.FileID)
	}
	if _, err := s.store.ResolveLiveFileLineageByPath(ctx, workspace, normalizedPath); err == nil {
		return FileVersionUndeleteResponse{}, ErrWorkspaceConflict
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return FileVersionUndeleteResponse{}, err
	}

	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return FileVersionUndeleteResponse{}, err
	}
	storageID := workspaceStorageID(meta)
	fsKey, _, _, err := EnsureWorkspaceRoot(ctx, s.store, workspace)
	if err != nil {
		return FileVersionUndeleteResponse{}, err
	}
	fsClient := afsclient.New(s.store.rdb, fsKey)
	stat, err := fsClient.Stat(ctx, normalizedPath)
	if err != nil && !errors.Is(err, redis.Nil) {
		return FileVersionUndeleteResponse{}, err
	}
	if err == nil && stat != nil {
		return FileVersionUndeleteResponse{}, ErrWorkspaceConflict
	}

	if err := applySelectedVersionToWorkspacePath(ctx, s.store, workspace, fsClient, normalizedPath, nil, selected); err != nil {
		return FileVersionUndeleteResponse{}, err
	}
	if err := MarkWorkspaceRootDirty(ctx, s.store, storageID); err != nil {
		return FileVersionUndeleteResponse{}, err
	}
	meta.DirtyHint = true
	if err := s.store.PutWorkspaceMeta(ctx, meta); err != nil {
		return FileVersionUndeleteResponse{}, err
	}

	updatedStat, err := fsClient.Stat(ctx, normalizedPath)
	if err != nil && !errors.Is(err, redis.Nil) {
		return FileVersionUndeleteResponse{}, err
	}
	if errors.Is(err, redis.Nil) {
		updatedStat = nil
	}
	afterSnapshot, err := fileVersionedSnapshotFromFS(ctx, fsClient, normalizedPath, updatedStat)
	if err != nil {
		return FileVersionUndeleteResponse{}, err
	}
	if _, err := s.store.ReviveFileLineage(ctx, workspace, lineage.FileID, normalizedPath, time.Now().UTC()); err != nil {
		return FileVersionUndeleteResponse{}, err
	}

	version := FileVersion{
		Path:          normalizedPath,
		Kind:          selected.Kind,
		Source:        ChangeSourceVersionUndelete,
		CheckpointIDs: nil,
		CreatedAt:     time.Now().UTC(),
		Mode:          afterSnapshot.Mode,
		SizeBytes:     afterSnapshot.SizeBytes,
		DeltaBytes:    afterSnapshot.SizeBytes,
	}
	if selected.Kind == FileVersionKindSymlink {
		version.Op = ChangeOpSymlink
		version.Target = selected.Target
	} else {
		version.Op = ChangeOpPut
		version.BlobID = afterSnapshot.BlobID
		version.ContentHash = afterSnapshot.ContentHash
	}
	template := s.buildChangelogTemplate(ctx, storageID, strings.TrimSpace(meta.HeadSavepoint), ChangeSourceVersionUndelete)
	version.SessionID = template.SessionID
	version.AgentID = template.AgentID
	version.User = template.User
	if checkpointID := strings.TrimSpace(meta.HeadSavepoint); checkpointID != "" {
		version.CheckpointIDs = []string{checkpointID}
	}
	appended, err := s.store.AppendFileVersion(ctx, storageID, lineage.FileID, version)
	if err != nil {
		return FileVersionUndeleteResponse{}, err
	}

	entry := template
	entry.Path = normalizedPath
	entry.Op = version.Op
	entry.SizeBytes = afterSnapshot.SizeBytes
	entry.DeltaBytes = afterSnapshot.SizeBytes
	entry.ContentHash = afterSnapshot.ContentHash
	entry.Mode = afterSnapshot.Mode
	entry.FileID = appended.FileID
	entry.VersionID = appended.VersionID
	WriteChangeEntries(ctx, s.store.rdb, storageID, []ChangeEntry{entry})

	return FileVersionUndeleteResponse{
		WorkspaceID:            workspace,
		Path:                   normalizedPath,
		Dirty:                  true,
		FileID:                 appended.FileID,
		VersionID:              appended.VersionID,
		UndeletedFromVersionID: selected.VersionID,
		UndeletedFromFileID:    selected.FileID,
		UndeletedFromOrdinal:   selected.Ordinal,
	}, nil
}

func (s *Service) resolveUndeleteTargetVersion(ctx context.Context, workspace, normalizedPath string, selector FileVersionSelector) (FileVersion, FileLineage, error) {
	if strings.TrimSpace(selector.VersionID) == "" && strings.TrimSpace(selector.FileID) == "" && selector.Ordinal == 0 {
		history, err := s.getFileHistory(ctx, workspace, normalizedPath, true)
		if err != nil {
			return FileVersion{}, FileLineage{}, err
		}
		for _, candidate := range history.Lineages {
			if candidate.State != FileLineageStateDeleted {
				continue
			}
			lineage, err := s.store.GetFileLineage(ctx, workspace, candidate.FileID)
			if err != nil {
				return FileVersion{}, FileLineage{}, err
			}
			version, err := s.latestRestorableVersionForLineage(ctx, workspace, candidate.FileID)
			if err != nil {
				return FileVersion{}, FileLineage{}, err
			}
			return version, lineage, nil
		}
		return FileVersion{}, FileLineage{}, os.ErrNotExist
	}

	selected, err := s.resolveSelectedFileVersion(ctx, workspace, selector)
	if err != nil {
		return FileVersion{}, FileLineage{}, err
	}
	if selected.Path != normalizedPath {
		return FileVersion{}, FileLineage{}, fmt.Errorf("version %q belongs to %q, not %q", selected.VersionID, selected.Path, normalizedPath)
	}
	lineage, err := s.store.GetFileLineage(ctx, workspace, selected.FileID)
	if err != nil {
		return FileVersion{}, FileLineage{}, err
	}
	if selected.Kind == FileVersionKindTombstone {
		selected, err = s.latestRestorableVersionForLineage(ctx, workspace, selected.FileID)
		if err != nil {
			return FileVersion{}, FileLineage{}, err
		}
	}
	return selected, lineage, nil
}

func (s *Service) latestRestorableVersionForLineage(ctx context.Context, workspace, fileID string) (FileVersion, error) {
	versions, err := s.store.ListFileVersions(ctx, workspace, fileID, false)
	if err != nil {
		return FileVersion{}, err
	}
	for _, version := range versions {
		if version.Kind != FileVersionKindTombstone {
			return version, nil
		}
	}
	return FileVersion{}, os.ErrNotExist
}

func applySelectedVersionToWorkspacePath(ctx context.Context, store *Store, workspace string, fsClient afsclient.Client, normalizedPath string, stat *afsclient.StatResult, version FileVersion) error {
	switch version.Kind {
	case FileVersionKindFile:
		if stat != nil && stat.Type != "file" {
			if err := fsClient.Rm(ctx, normalizedPath); err != nil {
				return err
			}
			stat = nil
		}
		if strings.TrimSpace(version.BlobID) == "" {
			return fmt.Errorf("version %q content is unavailable", version.VersionID)
		}
		data, err := store.GetBlob(ctx, workspace, version.BlobID)
		if err != nil {
			return err
		}
		if stat == nil {
			if err := fsClient.EchoCreate(ctx, normalizedPath, data, max(version.Mode, 0o644)); err != nil {
				return err
			}
		} else if err := fsClient.Echo(ctx, normalizedPath, data); err != nil {
			return err
		}
		if version.Mode != 0 {
			if err := fsClient.Chmod(ctx, normalizedPath, version.Mode); err != nil {
				return err
			}
		}
		return nil
	case FileVersionKindSymlink:
		if stat != nil {
			if err := fsClient.Rm(ctx, normalizedPath); err != nil {
				return err
			}
		}
		if err := ensureVersionedParentDirs(ctx, fsClient, normalizedPath); err != nil {
			return err
		}
		return fsClient.Ln(ctx, version.Target, normalizedPath)
	default:
		return fmt.Errorf("unsupported restore version kind %q", version.Kind)
	}
}

func ensureVersionedParentDirs(ctx context.Context, fsClient afsclient.Client, normalizedPath string) error {
	trimmed := strings.Trim(normalizedPath, "/")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) <= 1 {
		return nil
	}
	current := ""
	for _, part := range parts[:len(parts)-1] {
		current += "/" + part
		if stat, err := fsClient.Stat(ctx, current); err == nil && stat != nil {
			continue
		} else if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		if err := fsClient.Mkdir(ctx, current); err != nil {
			return err
		}
	}
	return nil
}

func fileVersionedSnapshotFromFS(ctx context.Context, fsClient afsclient.Client, normalizedPath string, stat *afsclient.StatResult) (VersionedFileSnapshot, error) {
	snapshot := VersionedFileSnapshot{Path: normalizedPath}
	if stat == nil {
		return snapshot, nil
	}
	snapshot.Exists = true
	snapshot.Kind = stat.Type
	snapshot.Mode = stat.Mode
	switch stat.Type {
	case "file":
		content, err := fsClient.Cat(ctx, normalizedPath)
		if err != nil {
			return VersionedFileSnapshot{}, err
		}
		snapshot.Content = content
		snapshot.SizeBytes = int64(len(content))
	case "symlink":
		target, err := fsClient.Readlink(ctx, normalizedPath)
		if err != nil {
			return VersionedFileSnapshot{}, err
		}
		snapshot.Target = target
	default:
		snapshot.Exists = false
	}
	return completeVersionedSnapshot(snapshot), nil
}

func max(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}
