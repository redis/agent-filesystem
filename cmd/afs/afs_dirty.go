package main

import (
	"context"
	"time"
)

func setAFSWorkspaceDirtyState(ctx context.Context, cfg config, store *afsStore, workspaceInfo workspaceMeta, localState afsLocalState, dirty bool) error {
	now := time.Now().UTC()
	localState.Dirty = dirty
	localState.LastScanAt = now
	if err := saveAFSLocalState(cfg, localState); err != nil {
		return err
	}

	workspaceInfo.DirtyHint = dirty
	return store.putWorkspaceMeta(ctx, workspaceInfo)
}

func refreshAFSWorkspaceDirtyState(ctx context.Context, cfg config, store *afsStore, workspace string) (bool, error) {
	workspaceInfo, localState, err := requireMaterializedWorkspace(ctx, store, cfg, workspace)
	if err != nil {
		return false, err
	}

	headManifest, err := store.getManifest(ctx, workspace, workspaceInfo.HeadSavepoint)
	if err != nil {
		return false, err
	}

	treePath := afsWorkspaceTreePath(cfg, workspace)
	localManifest, _, _, err := buildManifestFromDirectory(treePath, workspace, workspaceInfo.HeadSavepoint)
	if err != nil {
		return false, err
	}

	dirty := !manifestEquivalent(headManifest, localManifest)
	if err := setAFSWorkspaceDirtyState(ctx, cfg, store, workspaceInfo, localState, dirty); err != nil {
		return false, err
	}
	return dirty, nil
}
