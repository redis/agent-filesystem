package main

import (
	"context"
	"errors"
	"os"
	"time"
)

type rafWorkspaceResetResult struct {
	treePath    string
	archivePath string
}

func resetRAFWorkspaceHead(ctx context.Context, cfg config, store *rafStore, workspace, savepoint string) (rafWorkspaceResetResult, error) {
	result := rafWorkspaceResetResult{
		treePath: rafWorkspaceTreePath(cfg, workspace),
	}

	if _, err := os.Stat(result.treePath); err == nil {
		archivePath, err := archiveLocalTree(cfg, workspace)
		if err != nil {
			return result, err
		}
		result.archivePath = archivePath
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, err
	}

	if err := store.moveWorkspaceHead(ctx, workspace, savepoint, time.Now().UTC()); err != nil {
		if result.archivePath != "" {
			_ = os.Rename(result.archivePath, result.treePath)
		}
		return result, err
	}

	if err := materializeWorkspace(ctx, store, cfg, workspace); err != nil {
		return result, err
	}

	return result, nil
}
