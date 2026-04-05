package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type rafStore struct {
	rdb *redis.Client
}

func newRAFStore(rdb *redis.Client) *rafStore {
	return &rafStore{rdb: rdb}
}

func (s *rafStore) workspaceExists(ctx context.Context, workspace string) (bool, error) {
	count, err := s.rdb.Exists(ctx, rafWorkspaceMetaKey(workspace)).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *rafStore) deleteWorkspace(ctx context.Context, workspace string) error {
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, rafWorkspacePattern(workspace), 128).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := s.rdb.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func (s *rafStore) putWorkspaceMeta(ctx context.Context, meta workspaceMeta) error {
	return rafSetJSON(ctx, s.rdb, rafWorkspaceMetaKey(meta.Name), meta)
}

func (s *rafStore) getWorkspaceMeta(ctx context.Context, workspace string) (workspaceMeta, error) {
	return rafGetJSON[workspaceMeta](ctx, s.rdb, rafWorkspaceMetaKey(workspace))
}

func (s *rafStore) listWorkspaces(ctx context.Context) ([]workspaceMeta, error) {
	metas := make([]workspaceMeta, 0)
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, "raf:{*}:workspace:meta", 128).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			meta, err := rafGetJSON[workspaceMeta](ctx, s.rdb, key)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return nil, err
			}
			metas = append(metas, meta)
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].Name < metas[j].Name
	})
	return metas, nil
}

func (s *rafStore) putSessionMeta(ctx context.Context, meta sessionMeta) error {
	if err := rafSetJSON(ctx, s.rdb, rafSessionMetaKey(meta.Workspace, meta.Name), meta); err != nil {
		return err
	}
	return s.rdb.ZAdd(ctx, rafWorkspaceSessionsKey(meta.Workspace), redis.Z{
		Score:  float64(meta.UpdatedAt.UTC().UnixMilli()),
		Member: meta.Name,
	}).Err()
}

func (s *rafStore) getSessionMeta(ctx context.Context, workspace, session string) (sessionMeta, error) {
	return rafGetJSON[sessionMeta](ctx, s.rdb, rafSessionMetaKey(workspace, session))
}

func (s *rafStore) moveSessionHead(ctx context.Context, workspace, session, savepoint string, updatedAt time.Time) error {
	return s.rdb.Watch(ctx, func(tx *redis.Tx) error {
		current, err := rafGetJSON[sessionMeta](ctx, tx, rafSessionMetaKey(workspace, session))
		if err != nil {
			return err
		}
		current.HeadSavepoint = savepoint
		current.UpdatedAt = updatedAt.UTC()
		current.DirtyHint = false

		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			if err := rafSetJSON(ctx, pipe, rafSessionMetaKey(workspace, session), current); err != nil {
				return err
			}
			pipe.ZAdd(ctx, rafWorkspaceSessionsKey(workspace), redis.Z{
				Score:  float64(updatedAt.UTC().UnixMilli()),
				Member: session,
			})
			return nil
		})
		return err
	}, rafSessionMetaKey(workspace, session))
}

func (s *rafStore) listSessions(ctx context.Context, workspace string) ([]sessionMeta, error) {
	names, err := s.rdb.ZRange(ctx, rafWorkspaceSessionsKey(workspace), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	metas := make([]sessionMeta, 0, len(names))
	for _, name := range names {
		meta, err := s.getSessionMeta(ctx, workspace, name)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		metas = append(metas, meta)
	}
	return metas, nil
}

func (s *rafStore) putSavepoint(ctx context.Context, meta savepointMeta, m manifest) error {
	if err := rafSetJSON(ctx, s.rdb, rafSavepointMetaKey(meta.Workspace, meta.ID), meta); err != nil {
		return err
	}
	if err := rafSetJSON(ctx, s.rdb, rafSavepointManifestKey(meta.Workspace, meta.ID), m); err != nil {
		return err
	}
	return s.rdb.ZAdd(ctx, rafWorkspaceSavepointsKey(meta.Workspace), redis.Z{
		Score:  float64(meta.CreatedAt.UTC().UnixMilli()),
		Member: meta.ID,
	}).Err()
}

func (s *rafStore) savepointExists(ctx context.Context, workspace, savepoint string) (bool, error) {
	count, err := s.rdb.Exists(ctx, rafSavepointMetaKey(workspace, savepoint)).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *rafStore) getSavepointMeta(ctx context.Context, workspace, savepoint string) (savepointMeta, error) {
	return rafGetJSON[savepointMeta](ctx, s.rdb, rafSavepointMetaKey(workspace, savepoint))
}

func (s *rafStore) getManifest(ctx context.Context, workspace, savepoint string) (manifest, error) {
	return rafGetJSON[manifest](ctx, s.rdb, rafSavepointManifestKey(workspace, savepoint))
}

func (s *rafStore) listSavepoints(ctx context.Context, workspace, session string, limit int64) ([]savepointMeta, error) {
	stop := int64(-1)
	if limit > 0 {
		stop = limit - 1
	}
	ids, err := s.rdb.ZRevRange(ctx, rafWorkspaceSavepointsKey(workspace), 0, stop).Result()
	if err != nil {
		return nil, err
	}
	savepoints := make([]savepointMeta, 0, len(ids))
	for _, id := range ids {
		meta, err := s.getSavepointMeta(ctx, workspace, id)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if session != "" && meta.Session != session {
			continue
		}
		savepoints = append(savepoints, meta)
	}
	return savepoints, nil
}

func (s *rafStore) saveBlobs(ctx context.Context, workspace string, blobs map[string][]byte) error {
	for blobID, data := range blobs {
		if err := s.rdb.SetNX(ctx, rafBlobKey(workspace, blobID), data, 0).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (s *rafStore) addBlobRefs(ctx context.Context, workspace string, m manifest, createdAt time.Time) error {
	refs := map[string]int64{}
	for _, entry := range m.Entries {
		if entry.BlobID == "" {
			continue
		}
		refs[entry.BlobID] = entry.Size
	}
	for blobID, size := range refs {
		ref, err := s.getBlobRef(ctx, workspace, blobID)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			ref = blobRef{
				BlobID:    blobID,
				Size:      size,
				RefCount:  0,
				CreatedAt: createdAt.UTC(),
			}
		}
		ref.RefCount++
		if err := rafSetJSON(ctx, s.rdb, rafBlobRefKey(workspace, blobID), ref); err != nil {
			return err
		}
	}
	return nil
}

func (s *rafStore) getBlobRef(ctx context.Context, workspace, blobID string) (blobRef, error) {
	return rafGetJSON[blobRef](ctx, s.rdb, rafBlobRefKey(workspace, blobID))
}

func (s *rafStore) getBlob(ctx context.Context, workspace, blobID string) ([]byte, error) {
	data, err := s.rdb.Get(ctx, rafBlobKey(workspace, blobID)).Bytes()
	if err == redis.Nil {
		return nil, os.ErrNotExist
	}
	return data, err
}

func (s *rafStore) blobStats(ctx context.Context, workspace string) (workspaceBlobStats, error) {
	stats := workspaceBlobStats{}
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, rafBlobRefKey(workspace, "*"), 128).Result()
		if err != nil {
			return stats, err
		}
		for _, key := range keys {
			ref, err := rafGetJSON[blobRef](ctx, s.rdb, key)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return stats, err
			}
			stats.Count++
			stats.Bytes += ref.Size
		}
		cursor = next
		if cursor == 0 {
			return stats, nil
		}
	}
}

func (s *rafStore) audit(ctx context.Context, workspace, session, op string, extra map[string]any) error {
	fields := map[string]any{
		"ts_ms":     strconv.FormatInt(time.Now().UTC().UnixMilli(), 10),
		"workspace": workspace,
		"op":        op,
	}
	if session != "" {
		fields["session"] = session
	}
	for key, value := range extra {
		fields[key] = fmt.Sprint(value)
	}
	if err := s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: rafWorkspaceAuditKey(workspace),
		Values: fields,
	}).Err(); err != nil {
		return err
	}
	if session == "" {
		return nil
	}
	return s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: rafSessionAuditKey(workspace, session),
		Values: fields,
	}).Err()
}

func rafSetJSON(ctx context.Context, cmd redis.Cmdable, key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return cmd.Set(ctx, key, b, 0).Err()
}

func rafGetJSON[T any](ctx context.Context, cmd redis.Cmdable, key string) (T, error) {
	var value T
	raw, err := cmd.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return value, os.ErrNotExist
		}
		return value, err
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, err
	}
	return value, nil
}
