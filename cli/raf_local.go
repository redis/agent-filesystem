package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var errRAFWorkspaceConflict = errors.New("afs workspace head conflict")
var errRAFWorkspaceNotMaterialized = errors.New("afs workspace is not materialized")

func rafWorkspaceDir(cfg config, workspace string) string {
	return filepath.Join(cfg.WorkRoot, workspace)
}

func rafWorkspaceArchiveDir(cfg config, workspace string) string {
	return filepath.Join(rafWorkspaceDir(cfg, workspace), "archive")
}

func rafWorkspaceStatePath(cfg config, workspace string) string {
	return filepath.Join(rafWorkspaceDir(cfg, workspace), "state.json")
}

func rafWorkspaceTreePath(cfg config, workspace string) string {
	return filepath.Join(rafWorkspaceDir(cfg, workspace), "tree")
}

func loadRAFLocalState(cfg config, workspace string) (rafLocalState, error) {
	var st rafLocalState
	raw, err := os.ReadFile(rafWorkspaceStatePath(cfg, workspace))
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, err
	}
	return st, nil
}

func saveRAFLocalState(cfg config, st rafLocalState) error {
	if err := os.MkdirAll(rafWorkspaceDir(cfg, st.Workspace), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(rafWorkspaceStatePath(cfg, st.Workspace), raw, 0o600)
}

func removeLocalWorkspace(cfg config, workspace string) error {
	return os.RemoveAll(rafWorkspaceDir(cfg, workspace))
}

func materializeWorkspace(ctx context.Context, store *rafStore, cfg config, workspace string) error {
	return materializeWorkspaceWithProgress(ctx, store, cfg, workspace, nil)
}

func materializeWorkspaceWithProgress(ctx context.Context, store *rafStore, cfg config, workspace string, onProgress func(importStats)) error {
	meta, err := store.getWorkspaceMeta(ctx, workspace)
	if err != nil {
		return err
	}
	m, err := store.getManifest(ctx, workspace, meta.HeadSavepoint)
	if err != nil {
		return err
	}

	treePath := rafWorkspaceTreePath(cfg, workspace)
	if err := os.MkdirAll(rafWorkspaceDir(cfg, workspace), 0o755); err != nil {
		return err
	}
	if _, err := materializeManifestToDirectory(treePath, m, func(blobID string) ([]byte, error) {
		return store.getBlob(ctx, workspace, blobID)
	}, manifestMaterializeOptions{
		onProgress:       onProgress,
		preserveMetadata: true,
	}); err != nil {
		return err
	}

	now := time.Now().UTC()
	localState := rafLocalState{
		Version:        rafFormatVersion,
		Workspace:      workspace,
		HeadSavepoint:  meta.HeadSavepoint,
		Dirty:          false,
		MaterializedAt: now,
		LastScanAt:     now,
	}
	if err := saveRAFLocalState(cfg, localState); err != nil {
		return err
	}

	host, _ := os.Hostname()
	meta.LastMaterializedAt = now
	meta.LastKnownMaterializedAt = host
	meta.DirtyHint = false
	return store.putWorkspaceMeta(ctx, meta)
}

func ensureMaterializedWorkspace(ctx context.Context, store *rafStore, cfg config, workspace string) (workspaceMeta, rafLocalState, error) {
	meta, err := store.getWorkspaceMeta(ctx, workspace)
	if err != nil {
		return workspaceMeta{}, rafLocalState{}, err
	}

	treePath := rafWorkspaceTreePath(cfg, workspace)
	localState, err := loadRAFLocalState(cfg, workspace)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := materializeWorkspace(ctx, store, cfg, workspace); err != nil {
				return workspaceMeta{}, rafLocalState{}, err
			}
			localState, err = loadRAFLocalState(cfg, workspace)
			return meta, localState, err
		}
		return workspaceMeta{}, rafLocalState{}, err
	}

	if _, err := os.Stat(treePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := materializeWorkspace(ctx, store, cfg, workspace); err != nil {
				return workspaceMeta{}, rafLocalState{}, err
			}
			localState, err = loadRAFLocalState(cfg, workspace)
			return meta, localState, err
		}
		return workspaceMeta{}, rafLocalState{}, err
	}

	if localState.HeadSavepoint != meta.HeadSavepoint {
		if localState.Dirty {
			return workspaceMeta{}, rafLocalState{}, errRAFWorkspaceConflict
		}
		if err := materializeWorkspace(ctx, store, cfg, workspace); err != nil {
			return workspaceMeta{}, rafLocalState{}, err
		}
		localState, err = loadRAFLocalState(cfg, workspace)
		return meta, localState, err
	}

	return meta, localState, nil
}

func requireMaterializedWorkspace(ctx context.Context, store *rafStore, cfg config, workspace string) (workspaceMeta, rafLocalState, error) {
	meta, err := store.getWorkspaceMeta(ctx, workspace)
	if err != nil {
		return workspaceMeta{}, rafLocalState{}, err
	}

	treePath := rafWorkspaceTreePath(cfg, workspace)
	localState, err := loadRAFLocalState(cfg, workspace)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return workspaceMeta{}, rafLocalState{}, errRAFWorkspaceNotMaterialized
		}
		return workspaceMeta{}, rafLocalState{}, err
	}

	if _, err := os.Stat(treePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return workspaceMeta{}, rafLocalState{}, errRAFWorkspaceNotMaterialized
		}
		return workspaceMeta{}, rafLocalState{}, err
	}

	if localState.HeadSavepoint != meta.HeadSavepoint {
		if localState.Dirty {
			return workspaceMeta{}, rafLocalState{}, errRAFWorkspaceConflict
		}
		return workspaceMeta{}, rafLocalState{}, errRAFWorkspaceNotMaterialized
	}

	return meta, localState, nil
}

func archiveLocalTree(cfg config, workspace string) (string, error) {
	treePath := rafWorkspaceTreePath(cfg, workspace)
	if _, err := os.Stat(treePath); err != nil {
		return "", err
	}

	archiveDir := rafWorkspaceArchiveDir(cfg, workspace)
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return "", err
	}

	target := filepath.Join(archiveDir, fmt.Sprintf("%s-%d", workspace, time.Now().UTC().UnixNano()))
	if err := os.Rename(treePath, target); err != nil {
		return "", err
	}
	return target, nil
}

func rafMaterializedPath(treePath, manifestPath string) string {
	if manifestPath == "/" {
		return treePath
	}
	trimmed := manifestPath
	for len(trimmed) > 0 && trimmed[0] == '/' {
		trimmed = trimmed[1:]
	}
	return filepath.Join(treePath, filepath.FromSlash(trimmed))
}

func fileModeOrDefault(mode uint32, fallback os.FileMode) os.FileMode {
	if mode == 0 {
		return fallback
	}
	return os.FileMode(mode)
}
