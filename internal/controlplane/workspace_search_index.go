package controlplane

import (
	"context"
	"errors"
	"strings"

	"github.com/redis/agent-filesystem/internal/searchindex"
	"github.com/redis/go-redis/v9"
)

const (
	workspaceSearchIndexReady       = "ready"
	workspaceSearchIndexBuilding    = "building"
	workspaceSearchIndexMissing     = "missing"
	workspaceSearchIndexUnavailable = "unavailable"
	workspaceSearchIndexError       = "error"
)

type workspaceSearchIndex struct {
	Name           string  `json:"name"`
	Present        bool    `json:"present"`
	Ready          bool    `json:"ready"`
	Status         string  `json:"status"`
	DocumentCount  int64   `json:"document_count,omitempty"`
	PercentIndexed float64 `json:"percent_indexed,omitempty"`
	Error          string  `json:"error,omitempty"`
}

func inspectWorkspaceSearchIndex(ctx context.Context, rdb *redis.Client, workspace string) (workspaceSearchIndex, error) {
	fsKey := WorkspaceFSKey(workspace)
	indexName := searchindex.IndexName(fsKey)
	result := workspaceSearchIndex{Name: indexName, Status: workspaceSearchIndexMissing}
	if rdb == nil {
		return result, nil
	}

	searchRDB := newSearchInfoRedisClient(rdb)
	defer searchRDB.Close()

	info, err := searchRDB.FTInfo(ctx, indexName).Result()
	if err != nil {
		switch {
		case ctx.Err() != nil:
			return result, ctx.Err()
		case isWorkspaceSearchUnavailable(err):
			result.Status = workspaceSearchIndexUnavailable
			return result, nil
		case isWorkspaceSearchUnknownIndex(err):
			result.Status = workspaceSearchIndexMissing
			return result, nil
		default:
			result.Status = workspaceSearchIndexError
			result.Error = err.Error()
			return result, nil
		}
	}

	result.Present = true
	result.DocumentCount = int64(info.NumDocs)
	result.PercentIndexed = info.PercentIndexed

	ready, err := rdb.Get(ctx, searchindex.ReadyKey(fsKey)).Result()
	switch {
	case err == nil && strings.TrimSpace(ready) == "1":
		result.Ready = true
	case err == nil:
	case errors.Is(err, redis.Nil):
	case ctx.Err() != nil:
		return result, ctx.Err()
	default:
		result.Status = workspaceSearchIndexError
		result.Error = err.Error()
		return result, nil
	}

	if info.HashIndexingFailures > 0 {
		result.Status = workspaceSearchIndexError
		result.Error = strings.TrimSpace(info.IndexErrors.LastIndexingError)
		if result.Error == "" {
			result.Error = "RediSearch reported indexing failures"
		}
		return result, nil
	}
	if result.Ready || info.PercentIndexed >= 1 {
		result.Status = workspaceSearchIndexReady
		result.Ready = true
		return result, nil
	}
	result.Status = workspaceSearchIndexBuilding
	return result, nil
}

func newSearchInfoRedisClient(base *redis.Client) *redis.Client {
	opts := *base.Options()
	opts.Protocol = 2
	opts.UnstableResp3 = false
	return redis.NewClient(&opts)
}

func isWorkspaceSearchUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown command") ||
		strings.Contains(msg, "module not loaded") ||
		strings.Contains(msg, "unknown module") ||
		strings.Contains(msg, "no such module") ||
		strings.Contains(msg, "search is not available")
}

func isWorkspaceSearchUnknownIndex(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown index name") ||
		strings.Contains(msg, "no such index") ||
		strings.Contains(msg, "index not found")
}
