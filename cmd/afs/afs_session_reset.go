package main

import (
	"context"
)

type afsWorkspaceResetResult struct{}

type afsCheckpointRestorer interface {
	RestoreCheckpoint(ctx context.Context, workspace, checkpointID string) error
}

func resetAFSWorkspaceHead(ctx context.Context, service afsCheckpointRestorer, workspace, savepoint string) (afsWorkspaceResetResult, error) {
	if err := service.RestoreCheckpoint(ctx, workspace, savepoint); err != nil {
		return afsWorkspaceResetResult{}, err
	}

	return afsWorkspaceResetResult{}, nil
}
