package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/agent-filesystem/internal/rediscontent"
	"github.com/redis/agent-filesystem/internal/searchindex"
	"github.com/redis/go-redis/v9"
)

const searchIndexReadyTimeout = 5 * time.Second
const searchIndexReadyPollInterval = 25 * time.Millisecond

func ensureWorkspaceSearchIndex(ctx context.Context, rdb *redis.Client, fsKey string) (bool, error) {
	indexName := searchindex.IndexName(fsKey)
	if _, err := rdb.FTInfo(ctx, indexName).Result(); err != nil {
		switch {
		case isSearchUnavailable(err):
			return false, nil
		case !isUnknownSearchIndex(err):
			return false, err
		}

		_, err = rdb.FTCreate(ctx, indexName, &redis.FTCreateOptions{
			OnHash:    true,
			Prefix:    []interface{}{fmt.Sprintf("afs:{%s}:inode:", fsKey)},
			NoOffsets: true,
			NoHL:      true,
			NoFreqs:   true,
		},
			&redis.FieldSchema{FieldName: "type", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "path", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "path_ancestors", FieldType: redis.SearchFieldTypeTag, Separator: ","},
			&redis.FieldSchema{FieldName: "search_state", FieldType: redis.SearchFieldTypeTag},
			&redis.FieldSchema{FieldName: "grep_grams_ci", FieldType: redis.SearchFieldTypeText, NoStem: true},
			&redis.FieldSchema{FieldName: "size", FieldType: redis.SearchFieldTypeNumeric},
			&redis.FieldSchema{FieldName: "mtime_ms", FieldType: redis.SearchFieldTypeNumeric},
		).Result()
		if err != nil {
			switch {
			case isSearchUnavailable(err):
				return false, nil
			case isIndexAlreadyExists(err):
			default:
				return false, err
			}
		}
	}

	ready, err := rdb.Get(ctx, searchindex.ReadyKey(fsKey)).Result()
	switch {
	case err == nil && strings.TrimSpace(ready) == "1":
		return true, nil
	case err != nil && !errors.Is(err, redis.Nil):
		return true, err
	}

	if err := backfillWorkspaceSearchFields(ctx, rdb, fsKey); err != nil {
		return true, err
	}
	expectedDocs, err := expectedSearchDocCount(ctx, rdb, fsKey)
	if err != nil {
		return true, err
	}
	if err := waitForSearchIndexReady(ctx, rdb, indexName, expectedDocs, searchIndexReadyTimeout); err != nil {
		return true, err
	}
	if err := rdb.Set(ctx, searchindex.ReadyKey(fsKey), "1", 0).Err(); err != nil {
		return true, err
	}
	return true, nil
}

func expectedSearchDocCount(ctx context.Context, rdb *redis.Client, fsKey string) (int, error) {
	values, err := rdb.HMGet(ctx, fmt.Sprintf("afs:{%s}:info", fsKey), "files", "directories", "symlinks").Result()
	if err != nil {
		return 0, err
	}
	total := 0
	for _, value := range values {
		total += searchIndexInt(value)
	}
	return total, nil
}

func waitForSearchIndexReady(ctx context.Context, rdb *redis.Client, indexName string, expectedDocs int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		info, err := rdb.FTInfo(ctx, indexName).Result()
		if err != nil {
			return err
		}
		if info.HashIndexingFailures > 0 {
			if info.IndexErrors.LastIndexingError != "" {
				return fmt.Errorf("search index %s indexing failed: %s", indexName, info.IndexErrors.LastIndexingError)
			}
			return fmt.Errorf("search index %s indexing failed", indexName)
		}
		if info.PercentIndexed >= 1 && (expectedDocs == 0 || info.NumDocs >= expectedDocs) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("search index %s not ready after %s (percent_indexed=%.3f num_docs=%d expected_docs=%d)", indexName, timeout, info.PercentIndexed, info.NumDocs, expectedDocs)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(searchIndexReadyPollInterval):
		}
	}
}

func searchIndexInt(value interface{}) int {
	switch v := value.(type) {
	case nil:
		return 0
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	case []byte:
		n, _ := strconv.Atoi(strings.TrimSpace(string(v)))
		return n
	case int:
		return v
	case int64:
		return int(v)
	default:
		n, _ := strconv.Atoi(fmt.Sprint(v))
		return n
	}
}

func searchIndexInt64(value interface{}) int64 {
	switch v := value.(type) {
	case nil:
		return 0
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	case []byte:
		n, _ := strconv.ParseInt(strings.TrimSpace(string(v)), 10, 64)
		return n
	case int:
		return int64(v)
	case int64:
		return v
	default:
		n, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(v)), 10, 64)
		return n
	}
}

func backfillWorkspaceSearchFields(ctx context.Context, rdb *redis.Client, fsKey string) error {
	pattern := fmt.Sprintf("afs:{%s}:inode:*", fsKey)
	prefix := fmt.Sprintf("afs:{%s}:inode:", fsKey)

	type inodeSummary struct {
		key        string
		inodeID    string
		contentRef string
		size       int64
	}

	var cursor uint64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, pattern, 256).Result()
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			metaPipe := rdb.Pipeline()
			metaCmds := make([]*redis.SliceCmd, len(keys))
			for i, key := range keys {
				metaCmds[i] = metaPipe.HMGet(ctx, key, "type", "search_state", "content_ref", "size")
			}
			if _, err := metaPipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
				return err
			}

			missing := make([]inodeSummary, 0)
			for i, cmd := range metaCmds {
				values, err := cmd.Result()
				if err != nil && !errors.Is(err, redis.Nil) {
					return err
				}
				if searchIndexString(values, 0) != "file" || strings.TrimSpace(searchIndexString(values, 1)) != "" {
					continue
				}
				missing = append(missing, inodeSummary{
					key:        keys[i],
					inodeID:    strings.TrimPrefix(keys[i], prefix),
					contentRef: searchIndexString(values, 2),
					size:       searchIndexInt64(values[3]),
				})
			}

			if len(missing) > 0 {
				writePipe := rdb.Pipeline()
				for _, item := range missing {
					content := ""
					if item.contentRef == "" {
						inline, err := rdb.HGet(ctx, item.key, "content").Result()
						if err != nil && !errors.Is(err, redis.Nil) {
							return err
						}
						content = inline
					} else {
						data, err := rediscontent.Load(ctx, rdb, fmt.Sprintf("afs:{%s}:content:%s", fsKey, item.inodeID), item.contentRef, item.size)
						if err != nil {
							return err
						}
						content = string(data)
					}
					fields := searchindex.BuildFileFields([]byte(content))
					writePipe.HSet(ctx, item.key, map[string]interface{}{
						"search_state":  fields.SearchState,
						"grep_grams_ci": fields.GrepGramsCI,
					})
				}
				if _, err := writePipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
					return err
				}
			}
		}

		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func isSearchUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown command") || strings.Contains(msg, "module disabled")
}

func isUnknownSearchIndex(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown index name") || strings.Contains(msg, "no such index")
}

func isIndexAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "index already exists")
}

func searchIndexString(values []interface{}, idx int) string {
	if idx < 0 || idx >= len(values) || values[idx] == nil {
		return ""
	}
	switch value := values[idx].(type) {
	case string:
		return value
	case []byte:
		return string(value)
	default:
		return fmt.Sprint(value)
	}
}
