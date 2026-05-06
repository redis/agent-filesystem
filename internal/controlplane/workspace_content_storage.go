package controlplane

import (
	"context"
	"strings"

	"github.com/redis/agent-filesystem/internal/rediscontent"
	"github.com/redis/go-redis/v9"
)

const (
	workspaceContentStorageNone   = "none"
	workspaceContentStorageLegacy = "legacy"
	workspaceContentStorageArray  = "array"
	workspaceContentStorageMixed  = "mixed"
)

type workspaceContentStorage struct {
	Profile         string `json:"profile"`
	FileCount       int    `json:"file_count"`
	ArrayFileCount  int    `json:"array_file_count"`
	LegacyFileCount int    `json:"legacy_file_count"`
}

func inspectWorkspaceContentStorage(ctx context.Context, store *Store, workspace string) (workspaceContentStorage, error) {
	meta, storageID, err := store.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return workspaceContentStorage{}, err
	}
	exists, err := workspaceRootExists(ctx, store.rdb, storageID, meta.HeadSavepoint)
	if err != nil {
		return workspaceContentStorage{}, err
	}
	if !exists {
		if _, _, _, err := EnsureWorkspaceRoot(ctx, store, workspace); err != nil {
			return workspaceContentStorage{}, err
		}
	}
	return scanWorkspaceContentStorage(ctx, store.rdb, storageID)
}

func scanWorkspaceContentStorage(ctx context.Context, rdb *redis.Client, workspace string) (workspaceContentStorage, error) {
	fsKey := WorkspaceFSKey(workspace)
	type bfsItem struct {
		inodeID string
		path    string
	}
	queue := []bfsItem{{inodeID: workspaceFSRootInodeID, path: "/"}}
	stats := workspaceContentStorage{}

	for len(queue) > 0 {
		if ctx.Err() != nil {
			return workspaceContentStorage{}, ctx.Err()
		}

		pipe := rdb.Pipeline()
		inodeCmds := make([]*redis.SliceCmd, len(queue))
		for i, item := range queue {
			inodeCmds[i] = pipe.HMGet(ctx, workspaceFSInodeKey(fsKey, item.inodeID), "type", "content_ref")
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return workspaceContentStorage{}, err
		}

		dirItems := make([]bfsItem, 0, len(queue))
		for i, item := range queue {
			if shouldIgnoreMountPath(item.path) {
				continue
			}
			values, err := inodeCmds[i].Result()
			if err != nil || len(values) < 1 || values[0] == nil {
				continue
			}
			kind := redisString(values[0])
			contentRef := ""
			if len(values) > 1 {
				contentRef = strings.TrimSpace(redisString(values[1]))
			}
			switch kind {
			case "dir":
				dirItems = append(dirItems, item)
			case "file":
				stats.FileCount++
				if contentRef == rediscontent.RefArray {
					stats.ArrayFileCount++
				} else {
					stats.LegacyFileCount++
				}
			}
		}

		if len(dirItems) == 0 {
			break
		}

		pipe = rdb.Pipeline()
		direntCmds := make([]*redis.MapStringStringCmd, len(dirItems))
		for i, item := range dirItems {
			direntCmds[i] = pipe.HGetAll(ctx, workspaceFSDirentsKey(fsKey, item.inodeID))
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return workspaceContentStorage{}, err
		}

		nextQueue := make([]bfsItem, 0)
		for i, item := range dirItems {
			children, err := direntCmds[i].Result()
			if err != nil {
				continue
			}
			for name, inodeID := range children {
				nextQueue = append(nextQueue, bfsItem{
					inodeID: inodeID,
					path:    joinWorkspaceRootPath(item.path, name),
				})
			}
		}
		queue = nextQueue
	}

	switch {
	case stats.FileCount == 0:
		stats.Profile = workspaceContentStorageNone
	case stats.ArrayFileCount == stats.FileCount:
		stats.Profile = workspaceContentStorageArray
	case stats.LegacyFileCount == stats.FileCount:
		stats.Profile = workspaceContentStorageLegacy
	default:
		stats.Profile = workspaceContentStorageMixed
	}

	return stats, nil
}
