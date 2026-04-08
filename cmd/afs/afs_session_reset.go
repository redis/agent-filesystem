package main

import (
	"context"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

type afsWorkspaceResetResult struct{}

func resetAFSWorkspaceHead(ctx context.Context, service *controlplane.Service, workspace, savepoint string) (afsWorkspaceResetResult, error) {
	if err := service.RestoreCheckpoint(ctx, workspace, savepoint); err != nil {
		return afsWorkspaceResetResult{}, err
	}

	return afsWorkspaceResetResult{}, nil
}
