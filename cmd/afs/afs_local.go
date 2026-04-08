package main

import (
	"context"

	"github.com/redis/agent-filesystem/internal/worktree"
)

var errAFSWorkspaceConflict = worktree.ErrWorkspaceConflict
var errAFSWorkspaceNotMaterialized = worktree.ErrWorkspaceNotMaterialized

func afsWorkspaceDir(cfg config, workspace string) string {
	return worktree.WorkspaceDir(worktreeConfigFromCLI(cfg), workspace)
}

func afsWorkspaceArchiveDir(cfg config, workspace string) string {
	return worktree.WorkspaceArchiveDir(worktreeConfigFromCLI(cfg), workspace)
}

func afsWorkspaceStatePath(cfg config, workspace string) string {
	return worktree.WorkspaceStatePath(worktreeConfigFromCLI(cfg), workspace)
}

func afsWorkspaceTreePath(cfg config, workspace string) string {
	return worktree.WorkspaceTreePath(worktreeConfigFromCLI(cfg), workspace)
}

func loadAFSLocalState(cfg config, workspace string) (afsLocalState, error) {
	return worktree.LoadLocalState(worktreeConfigFromCLI(cfg), workspace)
}

func saveAFSLocalState(cfg config, st afsLocalState) error {
	return worktree.SaveLocalState(worktreeConfigFromCLI(cfg), st)
}

func removeLocalWorkspace(cfg config, workspace string) error {
	return worktree.RemoveLocalWorkspace(worktreeConfigFromCLI(cfg), workspace)
}

func materializeWorkspace(ctx context.Context, store *afsStore, cfg config, workspace string) error {
	return materializeWorkspaceWithProgress(ctx, store, cfg, workspace, nil)
}

func materializeWorkspaceWithProgress(ctx context.Context, store *afsStore, cfg config, workspace string, onProgress func(importStats)) error {
	var progressFn func(worktree.ImportStats)
	if onProgress != nil {
		progressFn = func(progress worktree.ImportStats) {
			onProgress(importStats(progress))
		}
	}
	return worktree.MaterializeWorkspace(ctx, controlPlaneStoreFromAFS(store), worktreeConfigFromCLI(cfg), workspace, progressFn)
}

func ensureMaterializedWorkspace(ctx context.Context, store *afsStore, cfg config, workspace string) (workspaceMeta, afsLocalState, error) {
	return worktree.EnsureMaterializedWorkspace(ctx, controlPlaneStoreFromAFS(store), worktreeConfigFromCLI(cfg), workspace)
}

func requireMaterializedWorkspace(ctx context.Context, store *afsStore, cfg config, workspace string) (workspaceMeta, afsLocalState, error) {
	return worktree.RequireMaterializedWorkspace(ctx, controlPlaneStoreFromAFS(store), worktreeConfigFromCLI(cfg), workspace)
}

func archiveLocalTree(cfg config, workspace string) (string, error) {
	return worktree.ArchiveLocalTree(worktreeConfigFromCLI(cfg), workspace)
}

func afsMaterializedPath(treePath, manifestPath string) string {
	return worktree.MaterializedPath(treePath, manifestPath)
}
