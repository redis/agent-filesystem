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

var errRAFSessionConflict = errors.New("raf session head conflict")
var errRAFSessionNotMaterialized = errors.New("raf session is not materialized")

func rafWorkspaceDir(cfg config, workspace string) string {
	return filepath.Join(cfg.WorkRoot, workspace)
}

func rafWorkspaceArchiveDir(cfg config, workspace string) string {
	return filepath.Join(rafWorkspaceDir(cfg, workspace), "archive")
}

func rafSessionDir(cfg config, workspace, session string) string {
	return filepath.Join(rafWorkspaceDir(cfg, workspace), "sessions", session)
}

func rafSessionStatePath(cfg config, workspace, session string) string {
	return filepath.Join(rafSessionDir(cfg, workspace, session), "state.json")
}

func rafSessionTreePath(cfg config, workspace, session string) string {
	return filepath.Join(rafSessionDir(cfg, workspace, session), "tree")
}

func loadRAFLocalState(cfg config, workspace, session string) (rafLocalState, error) {
	var st rafLocalState
	raw, err := os.ReadFile(rafSessionStatePath(cfg, workspace, session))
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, err
	}
	return st, nil
}

func saveRAFLocalState(cfg config, st rafLocalState) error {
	if err := os.MkdirAll(rafSessionDir(cfg, st.Workspace, st.Session), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(rafSessionStatePath(cfg, st.Workspace, st.Session), raw, 0o600)
}

func removeLocalWorkspace(cfg config, workspace string) error {
	return os.RemoveAll(rafWorkspaceDir(cfg, workspace))
}

func materializeSession(ctx context.Context, store *rafStore, cfg config, workspace, session string) error {
	return materializeSessionWithProgress(ctx, store, cfg, workspace, session, nil)
}

func materializeSessionWithProgress(ctx context.Context, store *rafStore, cfg config, workspace, session string, onProgress func(importStats)) error {
	meta, err := store.getSessionMeta(ctx, workspace, session)
	if err != nil {
		return err
	}
	m, err := store.getManifest(ctx, workspace, meta.HeadSavepoint)
	if err != nil {
		return err
	}

	sessionDir := rafSessionDir(cfg, workspace, session)
	treePath := rafSessionTreePath(cfg, workspace, session)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
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
		Session:        session,
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
	return store.putSessionMeta(ctx, meta)
}

func ensureMaterializedSession(ctx context.Context, store *rafStore, cfg config, workspace, session string) (sessionMeta, rafLocalState, error) {
	meta, err := store.getSessionMeta(ctx, workspace, session)
	if err != nil {
		return sessionMeta{}, rafLocalState{}, err
	}

	treePath := rafSessionTreePath(cfg, workspace, session)
	localState, err := loadRAFLocalState(cfg, workspace, session)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := materializeSession(ctx, store, cfg, workspace, session); err != nil {
				return sessionMeta{}, rafLocalState{}, err
			}
			localState, err = loadRAFLocalState(cfg, workspace, session)
			return meta, localState, err
		}
		return sessionMeta{}, rafLocalState{}, err
	}

	if _, err := os.Stat(treePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := materializeSession(ctx, store, cfg, workspace, session); err != nil {
				return sessionMeta{}, rafLocalState{}, err
			}
			localState, err = loadRAFLocalState(cfg, workspace, session)
			return meta, localState, err
		}
		return sessionMeta{}, rafLocalState{}, err
	}

	if localState.HeadSavepoint != meta.HeadSavepoint {
		if localState.Dirty {
			return sessionMeta{}, rafLocalState{}, errRAFSessionConflict
		}
		if err := materializeSession(ctx, store, cfg, workspace, session); err != nil {
			return sessionMeta{}, rafLocalState{}, err
		}
		localState, err = loadRAFLocalState(cfg, workspace, session)
		return meta, localState, err
	}

	return meta, localState, nil
}

func requireMaterializedSession(ctx context.Context, store *rafStore, cfg config, workspace, session string) (sessionMeta, rafLocalState, error) {
	meta, err := store.getSessionMeta(ctx, workspace, session)
	if err != nil {
		return sessionMeta{}, rafLocalState{}, err
	}

	treePath := rafSessionTreePath(cfg, workspace, session)
	localState, err := loadRAFLocalState(cfg, workspace, session)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sessionMeta{}, rafLocalState{}, errRAFSessionNotMaterialized
		}
		return sessionMeta{}, rafLocalState{}, err
	}

	if _, err := os.Stat(treePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sessionMeta{}, rafLocalState{}, errRAFSessionNotMaterialized
		}
		return sessionMeta{}, rafLocalState{}, err
	}

	if localState.HeadSavepoint != meta.HeadSavepoint {
		if localState.Dirty {
			return sessionMeta{}, rafLocalState{}, errRAFSessionConflict
		}
		return sessionMeta{}, rafLocalState{}, errRAFSessionNotMaterialized
	}

	return meta, localState, nil
}

func archiveLocalTree(cfg config, workspace, session string) (string, error) {
	treePath := rafSessionTreePath(cfg, workspace, session)
	if _, err := os.Stat(treePath); err != nil {
		return "", err
	}

	archiveDir := rafWorkspaceArchiveDir(cfg, workspace)
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return "", err
	}

	target := filepath.Join(archiveDir, fmt.Sprintf("%s-%d", session, time.Now().UTC().UnixNano()))
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
