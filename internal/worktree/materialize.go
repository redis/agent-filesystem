package worktree

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/rowantrollope/agent-filesystem/internal/controlplane"
)

func MaterializeWorkspace(ctx context.Context, store *controlplane.Store, cfg Config, workspace string, onProgress func(ImportStats)) error {
	meta, err := store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return err
	}
	m, err := store.GetManifest(ctx, workspace, meta.HeadSavepoint)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(WorkspaceDir(cfg, workspace), 0o755); err != nil {
		return err
	}
	if _, err := MaterializeManifestToDirectory(WorkspaceTreePath(cfg, workspace), m, func(blobID string) ([]byte, error) {
		return store.GetBlob(ctx, workspace, blobID)
	}, MaterializeOptions{
		OnProgress:       onProgress,
		PreserveMetadata: true,
	}); err != nil {
		return err
	}

	now := time.Now().UTC()
	localState := LocalState{
		Version:        controlplane.FormatVersion,
		Workspace:      workspace,
		HeadSavepoint:  meta.HeadSavepoint,
		Dirty:          false,
		MaterializedAt: now,
		LastScanAt:     now,
	}
	if err := SaveLocalState(cfg, localState); err != nil {
		return err
	}

	host, _ := os.Hostname()
	meta.LastMaterializedAt = now
	meta.LastKnownMaterializedAt = host
	meta.DirtyHint = false
	return store.PutWorkspaceMeta(ctx, meta)
}

func EnsureMaterializedWorkspace(ctx context.Context, store *controlplane.Store, cfg Config, workspace string) (controlplane.WorkspaceMeta, LocalState, error) {
	meta, err := store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return controlplane.WorkspaceMeta{}, LocalState{}, err
	}

	treePath := WorkspaceTreePath(cfg, workspace)
	localState, err := LoadLocalState(cfg, workspace)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := MaterializeWorkspace(ctx, store, cfg, workspace, nil); err != nil {
				return controlplane.WorkspaceMeta{}, LocalState{}, err
			}
			localState, err = LoadLocalState(cfg, workspace)
			return meta, localState, err
		}
		return controlplane.WorkspaceMeta{}, LocalState{}, err
	}

	if _, err := os.Stat(treePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := MaterializeWorkspace(ctx, store, cfg, workspace, nil); err != nil {
				return controlplane.WorkspaceMeta{}, LocalState{}, err
			}
			localState, err = LoadLocalState(cfg, workspace)
			return meta, localState, err
		}
		return controlplane.WorkspaceMeta{}, LocalState{}, err
	}

	if localState.HeadSavepoint != meta.HeadSavepoint {
		if localState.Dirty {
			return controlplane.WorkspaceMeta{}, LocalState{}, ErrWorkspaceConflict
		}
		if err := MaterializeWorkspace(ctx, store, cfg, workspace, nil); err != nil {
			return controlplane.WorkspaceMeta{}, LocalState{}, err
		}
		localState, err = LoadLocalState(cfg, workspace)
		return meta, localState, err
	}

	return meta, localState, nil
}

func RequireMaterializedWorkspace(ctx context.Context, store *controlplane.Store, cfg Config, workspace string) (controlplane.WorkspaceMeta, LocalState, error) {
	meta, err := store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return controlplane.WorkspaceMeta{}, LocalState{}, err
	}

	treePath := WorkspaceTreePath(cfg, workspace)
	localState, err := LoadLocalState(cfg, workspace)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return controlplane.WorkspaceMeta{}, LocalState{}, ErrWorkspaceNotMaterialized
		}
		return controlplane.WorkspaceMeta{}, LocalState{}, err
	}

	if _, err := os.Stat(treePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return controlplane.WorkspaceMeta{}, LocalState{}, ErrWorkspaceNotMaterialized
		}
		return controlplane.WorkspaceMeta{}, LocalState{}, err
	}

	if localState.HeadSavepoint != meta.HeadSavepoint {
		if localState.Dirty {
			return controlplane.WorkspaceMeta{}, LocalState{}, ErrWorkspaceConflict
		}
		return controlplane.WorkspaceMeta{}, LocalState{}, ErrWorkspaceNotMaterialized
	}

	return meta, localState, nil
}
