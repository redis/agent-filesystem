package controlplane

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ImportWorkspaceRequest uploads a client-built manifest and blob set to create
// a workspace with an initial checkpoint.
type ImportWorkspaceRequest struct {
	DatabaseID  string            `json:"database_id,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Manifest    Manifest          `json:"manifest"`
	Blobs       map[string][]byte `json:"blobs,omitempty"`
	FileCount   int               `json:"file_count"`
	DirCount    int               `json:"dir_count"`
	TotalBytes  int64             `json:"total_bytes"`
}

// ImportWorkspaceResponse is returned after a successful uploaded import.
type ImportWorkspaceResponse struct {
	WorkspaceID string          `json:"workspace_id"`
	Workspace   workspaceDetail `json:"workspace"`
	FileCount   int             `json:"file_count"`
	DirCount    int             `json:"dir_count"`
	TotalBytes  int64           `json:"total_bytes"`
}

func (s *Service) importWorkspace(ctx context.Context, input ImportWorkspaceRequest) (ImportWorkspaceResponse, error) {
	workspace := strings.TrimSpace(input.Name)
	if err := ValidateName("workspace", workspace); err != nil {
		return ImportWorkspaceResponse{}, err
	}
	if len(input.Manifest.Entries) == 0 {
		return ImportWorkspaceResponse{}, fmt.Errorf("manifest is required")
	}

	exists, err := s.store.WorkspaceExists(ctx, workspace)
	if err != nil {
		return ImportWorkspaceResponse{}, err
	}
	if exists {
		return ImportWorkspaceResponse{}, fmt.Errorf("workspace %q already exists", workspace)
	}

	manifest := input.Manifest
	workspaceID, err := newOpaqueWorkspaceID()
	if err != nil {
		return ImportWorkspaceResponse{}, err
	}
	manifest.Workspace = workspaceID
	manifest.Savepoint = initialCheckpointName

	now := time.Now().UTC()
	manifestHash, err := HashManifest(manifest)
	if err != nil {
		return ImportWorkspaceResponse{}, err
	}
	if err := s.store.SaveBlobs(ctx, workspaceID, input.Blobs); err != nil {
		return ImportWorkspaceResponse{}, err
	}

	description := strings.TrimSpace(input.Description)
	if description == "" {
		description = "Imported from a local client."
	}

	meta := applyWorkspaceMetaDefaults(s.cfg, WorkspaceMeta{
		Version:          formatVersion,
		ID:               workspaceID,
		Name:             workspace,
		Description:      description,
		Source:           SourceGitImport,
		Tags:             WorkspaceTags("", SourceGitImport),
		CreatedAt:        now,
		UpdatedAt:        now,
		HeadSavepoint:    initialCheckpointName,
		DefaultSavepoint: initialCheckpointName,
	})
	checkpoint := SavepointMeta{
		Version:      formatVersion,
		ID:           initialCheckpointName,
		Name:         initialCheckpointName,
		Description:  "Initial import snapshot.",
		Author:       "afs",
		Workspace:    workspaceID,
		ManifestHash: manifestHash,
		CreatedAt:    now,
		FileCount:    input.FileCount,
		DirCount:     input.DirCount,
		TotalBytes:   input.TotalBytes,
	}

	if err := s.store.PutWorkspaceMeta(ctx, meta); err != nil {
		return ImportWorkspaceResponse{}, err
	}
	if err := s.store.PutSavepoint(ctx, checkpoint, manifest); err != nil {
		return ImportWorkspaceResponse{}, err
	}
	if err := SyncWorkspaceRoot(ctx, s.store, workspaceID, manifest); err != nil {
		return ImportWorkspaceResponse{}, err
	}
	if err := s.store.Audit(ctx, workspaceID, "import", map[string]any{
		"checkpoint":  initialCheckpointName,
		"source":      "client-upload",
		"files":       input.FileCount,
		"dirs":        input.DirCount,
		"total_bytes": input.TotalBytes,
	}); err != nil {
		return ImportWorkspaceResponse{}, err
	}

	detail, err := s.getWorkspace(ctx, workspaceID)
	if err != nil {
		return ImportWorkspaceResponse{}, err
	}
	return ImportWorkspaceResponse{
		WorkspaceID: detail.ID,
		Workspace:   detail,
		FileCount:   input.FileCount,
		DirCount:    input.DirCount,
		TotalBytes:  input.TotalBytes,
	}, nil
}

// ImportWorkspace creates a workspace from a client-uploaded manifest and blob
// payload, preserving the initial checkpoint semantics used by local imports.
func (s *Service) ImportWorkspace(ctx context.Context, input ImportWorkspaceRequest) (ImportWorkspaceResponse, error) {
	return s.importWorkspace(ctx, input)
}

// ImportWorkspace creates a workspace from a client-uploaded manifest and blob
// payload through the scoped database manager API.
func (m *DatabaseManager) ImportWorkspace(ctx context.Context, databaseID string, input ImportWorkspaceRequest) (ImportWorkspaceResponse, error) {
	service, profile, err := m.serviceFor(ctx, databaseID)
	if err != nil {
		return ImportWorkspaceResponse{}, err
	}

	response, err := service.importWorkspace(ctx, input)
	if err != nil {
		return ImportWorkspaceResponse{}, err
	}
	if err := m.attachWorkspaceDetailIdentity(ctx, &response.Workspace, profile); err != nil {
		return ImportWorkspaceResponse{}, err
	}
	response.WorkspaceID = response.Workspace.ID
	return response, nil
}

func (m *DatabaseManager) ImportResolvedWorkspace(ctx context.Context, input ImportWorkspaceRequest) (ImportWorkspaceResponse, error) {
	profile, err := m.resolveTargetDatabase(ctx, input.DatabaseID)
	if err != nil {
		return ImportWorkspaceResponse{}, err
	}
	return m.ImportWorkspace(ctx, profile.ID, input)
}
