package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

func liveWorkspaceManifest(ctx context.Context, store *afsStore, workspace, savepointID string) (manifest, map[string][]byte, error) {
	if _, _, _, err := store.ensureWorkspaceRoot(ctx, workspace); err != nil {
		return manifest{}, nil, err
	}
	m, blobs, _, err := buildManifestFromWorkspaceRoot(ctx, store.rdb, workspaceRedisKey(workspace), workspace, savepointID)
	if err != nil {
		return manifest{}, nil, err
	}
	return m, blobs, nil
}

func materializeAFSWorkspaceFromLiveRoot(ctx context.Context, cfg config, store *afsStore, workspace string, onProgress func(importStats)) (workspaceMeta, afsLocalState, manifest, error) {
	meta, err := store.getWorkspaceMeta(ctx, workspace)
	if err != nil {
		return workspaceMeta{}, afsLocalState{}, manifest{}, err
	}

	liveManifest, blobs, err := liveWorkspaceManifest(ctx, store, workspace, meta.HeadSavepoint)
	if err != nil {
		return workspaceMeta{}, afsLocalState{}, manifest{}, err
	}

	if err := os.MkdirAll(afsWorkspaceDir(cfg, workspace), 0o755); err != nil {
		return workspaceMeta{}, afsLocalState{}, manifest{}, err
	}
	if _, err := materializeManifestToDirectory(afsWorkspaceTreePath(cfg, workspace), liveManifest, func(blobID string) ([]byte, error) {
		data, ok := blobs[blobID]
		if !ok {
			return nil, fmt.Errorf("live workspace blob %q is missing during materialize", blobID)
		}
		return data, nil
	}, manifestMaterializeOptions{onProgress: onProgress, preserveMetadata: true}); err != nil {
		return workspaceMeta{}, afsLocalState{}, manifest{}, err
	}

	dirty, err := workspaceManifestIsDirty(ctx, store, workspace, meta.HeadSavepoint, liveManifest)
	if err != nil {
		return workspaceMeta{}, afsLocalState{}, manifest{}, err
	}

	localState, err := persistAFSMaterializedState(ctx, cfg, store, meta, dirty)
	if err != nil {
		return workspaceMeta{}, afsLocalState{}, manifest{}, err
	}
	return meta, localState, liveManifest, nil
}

func persistAFSMaterializedState(ctx context.Context, cfg config, store *afsStore, meta workspaceMeta, dirty bool) (afsLocalState, error) {
	now := time.Now().UTC()
	localState := afsLocalState{
		Version:        afsFormatVersion,
		Workspace:      meta.Name,
		HeadSavepoint:  meta.HeadSavepoint,
		Dirty:          dirty,
		MaterializedAt: now,
		LastScanAt:     now,
	}
	if existing, err := loadAFSLocalState(cfg, meta.Name); err == nil {
		localState.MaterializedAt = existing.MaterializedAt
		if localState.MaterializedAt.IsZero() {
			localState.MaterializedAt = now
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return afsLocalState{}, err
	}
	if err := saveAFSLocalState(cfg, localState); err != nil {
		return afsLocalState{}, err
	}

	host, _ := os.Hostname()
	meta.LastMaterializedAt = now
	meta.LastKnownMaterializedAt = host
	meta.DirtyHint = dirty
	if dirty {
		if err := store.markWorkspaceRootDirty(ctx, meta.Name); err != nil {
			return afsLocalState{}, err
		}
	} else {
		if err := store.markWorkspaceRootClean(ctx, meta.Name, meta.HeadSavepoint); err != nil {
			return afsLocalState{}, err
		}
	}
	if err := store.putWorkspaceMeta(ctx, meta); err != nil {
		return afsLocalState{}, err
	}
	return localState, nil
}

func workspaceManifestIsDirty(ctx context.Context, store *afsStore, workspace, headSavepoint string, current manifest) (bool, error) {
	headManifest, err := store.getManifest(ctx, workspace, headSavepoint)
	if err != nil {
		return false, err
	}
	return !manifestEquivalent(headManifest, current), nil
}

func saveAFSWorkspaceOrLiveRoot(ctx context.Context, cfg config, store *afsStore, workspace, savepointID string, printResult bool) (bool, error) {
	return saveLiveWorkspaceCheckpoint(ctx, store, workspace, savepointID, printResult)
}

func saveLiveWorkspaceCheckpoint(ctx context.Context, store *afsStore, workspace, savepointID string, printResult bool) (bool, error) {
	workspaceInfo, err := store.getWorkspaceMeta(ctx, workspace)
	if err != nil {
		return false, err
	}
	if dirty, known, err := store.workspaceRootDirtyState(ctx, workspace); err != nil {
		return false, err
	} else if known && !dirty {
		if printResult {
			fmt.Println("No changes to save")
		}
		return false, nil
	}
	if _, _, _, err := store.ensureWorkspaceRoot(ctx, workspace); err != nil {
		return false, err
	}
	saved, err := saveWorkspaceRootCheckpoint(ctx, store, workspace, workspaceInfo.HeadSavepoint, savepointID)
	if err != nil {
		if errors.Is(err, errAFSWorkspaceConflict) {
			return false, fmt.Errorf("checkpoint conflict: workspace %q moved while saving; reopen it before retrying", workspace)
		}
		return false, err
	}
	if !saved {
		if printResult {
			fmt.Println("No changes to save")
		}
		return false, nil
	}

	if printResult {
		printBox(markerSuccess+" "+clr(ansiBold, "save complete"), []boxRow{
			{Label: "workspace", Value: workspace},
			{Label: "savepoint", Value: savepointID},
			{Label: "source", Value: "live workspace"},
		})
	}
	return true, nil
}

func activeMountedLiveWorkspace(workspace string) bool {
	st, err := loadState()
	if err != nil {
		return false
	}
	if strings.TrimSpace(st.CurrentWorkspace) != workspace {
		return false
	}

	backendName := strings.TrimSpace(st.MountBackend)
	if backendName == "" {
		backendName = mountBackendNone
	}
	if backendName == mountBackendNone {
		return false
	}

	return strings.TrimSpace(st.RedisKey) == workspaceRedisKey(workspace)
}
