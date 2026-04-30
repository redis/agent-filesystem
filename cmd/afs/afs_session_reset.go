package main

import (
	"context"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

type afsWorkspaceResetResult struct {
	SafetyCheckpointID      string
	SafetyCheckpointCreated bool
}

type afsCheckpointRestorer interface {
	RestoreCheckpoint(ctx context.Context, workspace, checkpointID string) error
}

type afsCheckpointRestorerWithResult interface {
	RestoreCheckpointWithResult(ctx context.Context, workspace, checkpointID string) (controlplane.RestoreCheckpointResult, error)
}

func resetAFSWorkspaceHead(ctx context.Context, service afsCheckpointRestorer, workspace, savepoint string) (afsWorkspaceResetResult, error) {
	if restorer, ok := service.(afsCheckpointRestorerWithResult); ok {
		result, err := restorer.RestoreCheckpointWithResult(ctx, workspace, savepoint)
		if err != nil {
			return afsWorkspaceResetResult{}, err
		}
		return afsWorkspaceResetResult{
			SafetyCheckpointID:      result.SafetyCheckpointID,
			SafetyCheckpointCreated: result.SafetyCheckpointCreated,
		}, nil
	}

	if err := service.RestoreCheckpoint(ctx, workspace, savepoint); err != nil {
		return afsWorkspaceResetResult{}, err
	}

	return afsWorkspaceResetResult{}, nil
}
