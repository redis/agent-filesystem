package main

import (
	"context"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/rowantrollope/agent-filesystem/cli/qmd"
)

func ensureWorkspaceSearchIndex(ctx context.Context, rdb *redis.Client, fsKey string) {
	if rdb == nil || strings.TrimSpace(fsKey) == "" {
		return
	}

	client := qmd.NewClient(rdb, fsKey, "")
	if _, err := rdb.Do(ctx, "FT._LIST").Result(); err != nil {
		return
	}
	_ = client.CreateIndex(ctx)
}
