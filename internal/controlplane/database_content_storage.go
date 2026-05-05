package controlplane

import (
	"context"
	"sort"
	"strings"

	"github.com/redis/agent-filesystem/internal/rediscontent"
)

func inspectDatabaseArraySupport(ctx context.Context, store *Store) (*bool, error) {
	if store == nil || store.rdb == nil {
		return nil, nil
	}
	supported, err := rediscontent.SupportsArrays(ctx, store.rdb)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, nil
	}
	return &supported, nil
}

func inspectDatabaseSearchSupport(ctx context.Context, store *Store) (*bool, error) {
	if store == nil || store.rdb == nil {
		return nil, nil
	}
	_, err := store.rdb.Do(ctx, "FT._LIST").Result()
	if err == nil {
		supported := true
		return &supported, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if isWorkspaceSearchUnavailable(err) {
		supported := false
		return &supported, nil
	}
	return nil, nil
}

type databaseWorkspaceStorage struct {
	WorkspaceID    string                  `json:"workspace_id"`
	WorkspaceName  string                  `json:"workspace_name"`
	RedisKey       string                  `json:"redis_key"`
	ContentStorage workspaceContentStorage `json:"content_storage"`
}

func inspectDatabaseWorkspaceStorage(ctx context.Context, service *Service, workspaces []workspaceSummary) (*bool, []databaseWorkspaceStorage, error) {
	if service == nil || service.store == nil || service.store.rdb == nil {
		return nil, nil, nil
	}

	supportsArrays, err := inspectDatabaseArraySupport(ctx, service.store)
	if err != nil {
		return nil, nil, err
	}

	items := make([]databaseWorkspaceStorage, 0, len(workspaces))
	for _, workspace := range workspaces {
		if ctx.Err() != nil {
			return supportsArrays, nil, ctx.Err()
		}
		storage, err := inspectWorkspaceContentStorage(ctx, service.store, workspace.ID)
		if err != nil {
			return supportsArrays, nil, err
		}
		items = append(items, databaseWorkspaceStorage{
			WorkspaceID:    workspace.ID,
			WorkspaceName:  workspace.Name,
			RedisKey:       workspace.RedisKey,
			ContentStorage: storage,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if strings.EqualFold(items[i].WorkspaceName, items[j].WorkspaceName) {
			return items[i].WorkspaceID < items[j].WorkspaceID
		}
		return strings.ToLower(items[i].WorkspaceName) < strings.ToLower(items[j].WorkspaceName)
	})

	return supportsArrays, items, nil
}
