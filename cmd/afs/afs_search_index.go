package main

import (
	"context"

	"github.com/redis/go-redis/v9"
)

func ensureWorkspaceSearchIndex(_ context.Context, _ *redis.Client, _ string) {}
