package controlplane

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	formatVersion         = 1
	inlineThreshold       = 4 * 1024
	initialCheckpointName = "initial"
)

const (
	FormatVersion         = formatVersion
	InlineThreshold       = inlineThreshold
	InitialCheckpointName = initialCheckpointName
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type WorkspaceMeta struct {
	Version                 int       `json:"version"`
	Name                    string    `json:"name"`
	Description             string    `json:"description,omitempty"`
	DatabaseID              string    `json:"database_id,omitempty"`
	DatabaseName            string    `json:"database_name,omitempty"`
	CloudAccount            string    `json:"cloud_account,omitempty"`
	Region                  string    `json:"region,omitempty"`
	Source                  string    `json:"source,omitempty"`
	Tags                    []string  `json:"tags,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
	HeadSavepoint           string    `json:"head_savepoint"`
	DefaultSavepoint        string    `json:"default_savepoint"`
	DirtyHint               bool      `json:"dirty_hint"`
	LastMaterializedAt      time.Time `json:"last_materialized_at"`
	LastKnownMaterializedAt string    `json:"last_materialized_host,omitempty"`
}

type SavepointMeta struct {
	Version         int       `json:"version"`
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	Author          string    `json:"author,omitempty"`
	Workspace       string    `json:"workspace"`
	ParentSavepoint string    `json:"parent_savepoint,omitempty"`
	ManifestHash    string    `json:"manifest_hash"`
	CreatedAt       time.Time `json:"created_at"`
	FileCount       int       `json:"file_count"`
	DirCount        int       `json:"dir_count"`
	TotalBytes      int64     `json:"total_bytes"`
}

type WorkspaceSessionRecord struct {
	SessionID       string    `json:"session_id"`
	Workspace       string    `json:"workspace"`
	ClientKind      string    `json:"client_kind,omitempty"`
	AFSVersion      string    `json:"afs_version,omitempty"`
	Hostname        string    `json:"hostname,omitempty"`
	OperatingSystem string    `json:"os,omitempty"`
	LocalPath       string    `json:"local_path,omitempty"`
	Readonly        bool      `json:"readonly,omitempty"`
	State           string    `json:"state"`
	StartedAt       time.Time `json:"started_at"`
	LastSeenAt      time.Time `json:"last_seen_at"`
	LeaseExpiresAt  time.Time `json:"lease_expires_at"`
}

type Manifest struct {
	Version   int                      `json:"version"`
	Workspace string                   `json:"workspace"`
	Savepoint string                   `json:"savepoint"`
	Entries   map[string]ManifestEntry `json:"entries"`
}

type ManifestEntry struct {
	Type    string `json:"type"`
	Mode    uint32 `json:"mode"`
	MtimeMs int64  `json:"mtime_ms"`
	Size    int64  `json:"size"`
	BlobID  string `json:"blob_id,omitempty"`
	Inline  string `json:"inline,omitempty"`
	Target  string `json:"target,omitempty"`
}

type blobRef struct {
	BlobID    string    `json:"blob_id"`
	Size      int64     `json:"size"`
	RefCount  int64     `json:"ref_count"`
	CreatedAt time.Time `json:"created_at"`
}

type auditRecord struct {
	ID        string
	Workspace string
	Op        string
	CreatedAt time.Time
	Fields    map[string]string
}

type BlobRef = blobRef
type AuditRecord = auditRecord

type BlobStats struct {
	Count int
	Bytes int64
}

type Store struct {
	rdb *redis.Client
}

func NewStore(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

func (s *Store) WorkspaceExists(ctx context.Context, workspace string) (bool, error) {
	count, err := s.rdb.Exists(ctx, workspaceMetaKey(workspace)).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) DeleteWorkspace(ctx context.Context, workspace string) error {
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, workspacePattern(workspace), 128).Result()
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

func (s *Store) PutWorkspaceMeta(ctx context.Context, meta WorkspaceMeta) error {
	return setJSON(ctx, s.rdb, workspaceMetaKey(meta.Name), meta)
}

func (s *Store) GetWorkspaceMeta(ctx context.Context, workspace string) (WorkspaceMeta, error) {
	return getJSON[WorkspaceMeta](ctx, s.rdb, workspaceMetaKey(workspace))
}

func (s *Store) ListWorkspaces(ctx context.Context) ([]WorkspaceMeta, error) {
	metas := make([]WorkspaceMeta, 0)
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, "afs:{*}:workspace:meta", 128).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			meta, err := getJSON[WorkspaceMeta](ctx, s.rdb, key)
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
	sort.Slice(metas, func(i, j int) bool { return metas[i].Name < metas[j].Name })
	return metas, nil
}

func (s *Store) PutSavepoint(ctx context.Context, meta SavepointMeta, m Manifest) error {
	if err := setJSON(ctx, s.rdb, savepointMetaKey(meta.Workspace, meta.ID), meta); err != nil {
		return err
	}
	if err := setJSON(ctx, s.rdb, savepointManifestKey(meta.Workspace, meta.ID), m); err != nil {
		return err
	}
	return s.rdb.ZAdd(ctx, workspaceSavepointsKey(meta.Workspace), redis.Z{
		Score:  float64(meta.CreatedAt.UTC().UnixMilli()),
		Member: meta.ID,
	}).Err()
}

func (s *Store) SavepointExists(ctx context.Context, workspace, savepoint string) (bool, error) {
	count, err := s.rdb.Exists(ctx, savepointMetaKey(workspace, savepoint)).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) GetSavepointMeta(ctx context.Context, workspace, savepoint string) (SavepointMeta, error) {
	return getJSON[SavepointMeta](ctx, s.rdb, savepointMetaKey(workspace, savepoint))
}

func (s *Store) GetManifest(ctx context.Context, workspace, savepoint string) (Manifest, error) {
	return getJSON[Manifest](ctx, s.rdb, savepointManifestKey(workspace, savepoint))
}

func (s *Store) ListSavepoints(ctx context.Context, workspace string, limit int64) ([]SavepointMeta, error) {
	stop := int64(-1)
	if limit > 0 {
		stop = limit - 1
	}
	ids, err := s.rdb.ZRevRange(ctx, workspaceSavepointsKey(workspace), 0, stop).Result()
	if err != nil {
		return nil, err
	}
	savepoints := make([]SavepointMeta, 0, len(ids))
	for _, id := range ids {
		meta, err := s.GetSavepointMeta(ctx, workspace, id)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		savepoints = append(savepoints, meta)
	}
	return savepoints, nil
}

func (s *Store) SaveBlobs(ctx context.Context, workspace string, blobs map[string][]byte) error {
	for blobID, data := range blobs {
		if err := s.rdb.SetNX(ctx, blobKey(workspace, blobID), data, 0).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) AddBlobRefs(ctx context.Context, workspace string, m Manifest, createdAt time.Time) error {
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
		if ref.Size == 0 {
			ref.Size = size
		}
		if err := setJSON(ctx, s.rdb, blobRefKey(workspace, blobID), ref); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) getBlobRef(ctx context.Context, workspace, blobID string) (blobRef, error) {
	return getJSON[blobRef](ctx, s.rdb, blobRefKey(workspace, blobID))
}

func (s *Store) GetBlob(ctx context.Context, workspace, blobID string) ([]byte, error) {
	data, err := s.rdb.Get(ctx, blobKey(workspace, blobID)).Bytes()
	if err == redis.Nil {
		return nil, os.ErrNotExist
	}
	return data, err
}

func (s *Store) BlobStats(ctx context.Context, workspace string) (BlobStats, error) {
	stats := BlobStats{}
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, blobRefKey(workspace, "*"), 128).Result()
		if err != nil {
			return stats, err
		}
		for _, key := range keys {
			ref, err := getJSON[blobRef](ctx, s.rdb, key)
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

func (s *Store) MoveWorkspaceHead(ctx context.Context, workspace, savepoint string, updatedAt time.Time) error {
	return s.rdb.Watch(ctx, func(tx *redis.Tx) error {
		current, err := getJSON[WorkspaceMeta](ctx, tx, workspaceMetaKey(workspace))
		if err != nil {
			return err
		}
		current.HeadSavepoint = savepoint
		current.UpdatedAt = updatedAt.UTC()
		current.DirtyHint = false

		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			return setJSON(ctx, pipe, workspaceMetaKey(workspace), current)
		})
		return err
	}, workspaceMetaKey(workspace))
}

func (s *Store) PutWorkspaceSession(ctx context.Context, record WorkspaceSessionRecord) error {
	if record.SessionID == "" {
		return fmt.Errorf("workspace session id is required")
	}
	if record.Workspace == "" {
		return fmt.Errorf("workspace session workspace is required")
	}
	if record.LeaseExpiresAt.IsZero() {
		return fmt.Errorf("workspace session lease expiry is required")
	}
	if err := setJSON(ctx, s.rdb, workspaceSessionKey(record.Workspace, record.SessionID), record); err != nil {
		return err
	}
	return s.rdb.ZAdd(ctx, workspaceSessionsKey(record.Workspace), redis.Z{
		Score:  float64(record.LeaseExpiresAt.UTC().UnixMilli()),
		Member: record.SessionID,
	}).Err()
}

func (s *Store) PutWorkspaceSessionWithTTL(ctx context.Context, record WorkspaceSessionRecord, ttl time.Duration) error {
	if record.SessionID == "" {
		return fmt.Errorf("workspace session id is required")
	}
	if record.Workspace == "" {
		return fmt.Errorf("workspace session workspace is required")
	}
	if err := setJSONWithTTL(ctx, s.rdb, workspaceSessionKey(record.Workspace, record.SessionID), record, ttl); err != nil {
		return err
	}
	return nil
}

func (s *Store) GetWorkspaceSession(ctx context.Context, workspace, sessionID string) (WorkspaceSessionRecord, error) {
	return getJSON[WorkspaceSessionRecord](ctx, s.rdb, workspaceSessionKey(workspace, sessionID))
}

func (s *Store) RemoveWorkspaceSessionPresence(ctx context.Context, workspace, sessionID string) error {
	return s.rdb.ZRem(ctx, workspaceSessionsKey(workspace), sessionID).Err()
}

func (s *Store) ListWorkspaceSessions(ctx context.Context, workspace string) ([]WorkspaceSessionRecord, error) {
	sessionIDs, err := s.rdb.ZRevRange(ctx, workspaceSessionsKey(workspace), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	records := make([]WorkspaceSessionRecord, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		record, err := s.GetWorkspaceSession(ctx, workspace, sessionID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				_ = s.RemoveWorkspaceSessionPresence(ctx, workspace, sessionID)
				continue
			}
			return nil, err
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].LastSeenAt.After(records[j].LastSeenAt)
	})
	return records, nil
}

func (s *Store) ListExpiredWorkspaceSessionIDs(ctx context.Context, workspace string, now time.Time) ([]string, error) {
	return s.rdb.ZRangeByScore(ctx, workspaceSessionsKey(workspace), &redis.ZRangeBy{
		Min: "-inf",
		Max: strconv.FormatInt(now.UTC().UnixMilli(), 10),
	}).Result()
}

func (s *Store) Audit(ctx context.Context, workspace, op string, extra map[string]any) error {
	fields := map[string]any{
		"ts_ms":     strconv.FormatInt(time.Now().UTC().UnixMilli(), 10),
		"workspace": workspace,
		"op":        op,
	}
	for key, value := range extra {
		fields[key] = fmt.Sprint(value)
	}
	return s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: workspaceAuditKey(workspace),
		Values: fields,
	}).Err()
}

func (s *Store) ListAudit(ctx context.Context, workspace string, limit int64) ([]auditRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	streams, err := s.rdb.XRevRangeN(ctx, workspaceAuditKey(workspace), "+", "-", limit).Result()
	if err != nil {
		return nil, err
	}
	records := make([]auditRecord, 0, len(streams))
	for _, stream := range streams {
		record := auditRecord{
			ID:        stream.ID,
			Workspace: workspace,
			CreatedAt: time.Now().UTC(),
			Fields:    map[string]string{},
		}
		for key, value := range stream.Values {
			record.Fields[key] = fmt.Sprint(value)
		}
		if tsText := record.Fields["ts_ms"]; tsText != "" {
			if ts, err := strconv.ParseInt(tsText, 10, 64); err == nil {
				record.CreatedAt = time.UnixMilli(ts).UTC()
			}
		}
		record.Workspace = defaultString(record.Fields["workspace"], workspace)
		record.Op = record.Fields["op"]
		records = append(records, record)
	}
	return records, nil
}

func ValidateName(kind, value string) error {
	if value == "" {
		return fmt.Errorf("%s name is required", kind)
	}
	if !namePattern.MatchString(value) {
		return fmt.Errorf("%s name %q is invalid; use letters, numbers, dot, dash, and underscore", kind, value)
	}
	return nil
}

func HashManifest(m Manifest) (string, error) {
	data, err := canonicalManifestBytes(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func ManifestEntryData(entry ManifestEntry, fetchBlob func(blobID string) ([]byte, error)) ([]byte, error) {
	switch {
	case entry.Inline != "":
		return base64.StdEncoding.DecodeString(entry.Inline)
	case entry.BlobID != "":
		return fetchBlob(entry.BlobID)
	case entry.Type == "file" && entry.Size == 0:
		return []byte{}, nil
	default:
		return nil, fmt.Errorf("manifest entry does not contain file data")
	}
}

func canonicalManifestBytes(m Manifest) ([]byte, error) {
	paths := manifestPaths(m)
	type canonicalManifestEntry struct {
		Path string `json:"path"`
		ManifestEntry
	}
	type canonicalManifest struct {
		Version   int                      `json:"version"`
		Workspace string                   `json:"workspace"`
		Savepoint string                   `json:"savepoint"`
		Entries   []canonicalManifestEntry `json:"entries"`
	}
	entries := make([]canonicalManifestEntry, 0, len(paths))
	for _, p := range paths {
		entries = append(entries, canonicalManifestEntry{
			Path:          p,
			ManifestEntry: m.Entries[p],
		})
	}
	return json.Marshal(canonicalManifest{
		Version:   m.Version,
		Workspace: m.Workspace,
		Savepoint: m.Savepoint,
		Entries:   entries,
	})
}

func manifestPaths(m Manifest) []string {
	paths := make([]string, 0, len(m.Entries))
	for p := range m.Entries {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

func WorkspaceMetaKey(workspace string) string {
	return workspaceMetaKey(workspace)
}

func WorkspaceSavepointsKey(workspace string) string {
	return workspaceSavepointsKey(workspace)
}

func WorkspaceAuditKey(workspace string) string {
	return workspaceAuditKey(workspace)
}

func SavepointMetaKey(workspace, savepoint string) string {
	return savepointMetaKey(workspace, savepoint)
}

func SavepointManifestKey(workspace, savepoint string) string {
	return savepointManifestKey(workspace, savepoint)
}

func BlobKey(workspace, blobID string) string {
	return blobKey(workspace, blobID)
}

func BlobRefKey(workspace, blobID string) string {
	return blobRefKey(workspace, blobID)
}

func WorkspacePattern(workspace string) string {
	return workspacePattern(workspace)
}

func workspaceMetaKey(workspace string) string {
	return fmt.Sprintf("afs:{%s}:workspace:meta", workspace)
}

func workspaceSavepointsKey(workspace string) string {
	return fmt.Sprintf("afs:{%s}:workspace:savepoints", workspace)
}

func workspaceSessionsKey(workspace string) string {
	return fmt.Sprintf("afs:{%s}:workspace:sessions", workspace)
}

func workspaceAuditKey(workspace string) string {
	return fmt.Sprintf("afs:{%s}:workspace:audit", workspace)
}

func workspaceSessionKey(workspace, sessionID string) string {
	return fmt.Sprintf("afs:{%s}:workspace:session:%s", workspace, sessionID)
}

func savepointMetaKey(workspace, savepoint string) string {
	return fmt.Sprintf("afs:{%s}:savepoint:%s:meta", workspace, savepoint)
}

func savepointManifestKey(workspace, savepoint string) string {
	return fmt.Sprintf("afs:{%s}:savepoint:%s:manifest", workspace, savepoint)
}

func blobKey(workspace, blobID string) string {
	return fmt.Sprintf("afs:{%s}:blob:%s", workspace, blobID)
}

func blobRefKey(workspace, blobID string) string {
	return fmt.Sprintf("afs:{%s}:blobref:%s", workspace, blobID)
}

func workspacePattern(workspace string) string {
	return fmt.Sprintf("afs:{%s}:*", workspace)
}

func setJSON(ctx context.Context, cmd redis.Cmdable, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return cmd.Set(ctx, key, data, 0).Err()
}

func setJSONWithTTL(ctx context.Context, cmd redis.Cmdable, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return cmd.Set(ctx, key, data, ttl).Err()
}

func getJSON[T any](ctx context.Context, cmd redis.Cmdable, key string) (T, error) {
	var value T
	data, err := cmd.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return value, os.ErrNotExist
	}
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, err
	}
	return value, nil
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
