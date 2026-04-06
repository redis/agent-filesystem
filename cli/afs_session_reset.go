package main

import (
	"context"
	"errors"
	"os"

	"github.com/rowantrollope/agent-filesystem/cli/internal/controlplane"
)

type afsWorkspaceResetResult struct {
	treePath    string
	archivePath string
}

func resetAFSWorkspaceHead(ctx context.Context, cfg config, store *afsStore, service *controlplane.Service, workspace, savepoint string) (afsWorkspaceResetResult, error) {
	result := afsWorkspaceResetResult{
		treePath: afsWorkspaceTreePath(cfg, workspace),
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

	if err := service.RestoreCheckpoint(ctx, workspace, savepoint); err != nil {
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
