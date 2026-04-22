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
	"strings"
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
	ID                      string    `json:"id,omitempty"`
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
	Label           string    `json:"label,omitempty"`
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

func workspaceStorageID(meta WorkspaceMeta) string {
	return WorkspaceStorageID(meta)
}

// WorkspaceStorageID returns the identifier used as the `{hashtag}` portion
// of every Redis key for this workspace. Prefers the opaque ID (set by the
// server for multi-tenant workspaces) and falls back to the name (legacy
// local-mode workspaces that never had an opaque ID assigned).
func WorkspaceStorageID(meta WorkspaceMeta) string {
	if id := strings.TrimSpace(meta.ID); id != "" {
		return id
	}
	return strings.TrimSpace(meta.Name)
}

func workspaceNameIndexKey() string {
	return "afs:workspace:index:names"
}

func (s *Store) resolveWorkspaceMeta(ctx context.Context, workspace string) (WorkspaceMeta, string, error) {
	ref := strings.TrimSpace(workspace)
	if ref == "" {
		return WorkspaceMeta{}, "", os.ErrNotExist
	}

	meta, err := getJSON[WorkspaceMeta](ctx, s.rdb, workspaceMetaKey(ref))
	if err == nil {
		return meta, workspaceStorageID(meta), nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return WorkspaceMeta{}, "", err
	}

	storageID, err := s.rdb.HGet(ctx, workspaceNameIndexKey(), ref).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return WorkspaceMeta{}, "", os.ErrNotExist
		}
		return WorkspaceMeta{}, "", err
	}
	meta, err = getJSON[WorkspaceMeta](ctx, s.rdb, workspaceMetaKey(storageID))
	if err != nil {
		return WorkspaceMeta{}, "", err
	}
	return meta, workspaceStorageID(meta), nil
}

func (s *Store) resolveWorkspaceStorageOrRaw(ctx context.Context, workspace string) (string, error) {
	ref := strings.TrimSpace(workspace)
	if ref == "" {
		return "", os.ErrNotExist
	}
	_, storageID, err := s.resolveWorkspaceMeta(ctx, ref)
	if err == nil {
		return storageID, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return ref, nil
	}
	return "", err
}

func (s *Store) Now(ctx context.Context) (time.Time, error) {
	now, err := s.rdb.Time(ctx).Result()
	if err != nil {
		return time.Time{}, err
	}
	return now.UTC(), nil
}

func (s *Store) WorkspaceExists(ctx context.Context, workspace string) (bool, error) {
	_, _, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (s *Store) DeleteWorkspace(ctx context.Context, workspace string) error {
	meta, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return err
	}
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, workspacePattern(storageID), 128).Result()
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
			if strings.TrimSpace(meta.ID) != "" {
				if err := s.rdb.HDel(ctx, workspaceNameIndexKey(), meta.Name).Err(); err != nil {
					return err
				}
			}
			return nil
		}
	}
}

func (s *Store) PutWorkspaceMeta(ctx context.Context, meta WorkspaceMeta) error {
	storageID := workspaceStorageID(meta)
	if storageID == "" {
		return fmt.Errorf("workspace id is required")
	}
	previous, err := getJSON[WorkspaceMeta](ctx, s.rdb, workspaceMetaKey(storageID))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := setJSON(ctx, s.rdb, workspaceMetaKey(storageID), meta); err != nil {
		return err
	}
	if strings.TrimSpace(meta.ID) != "" {
		if err := s.rdb.HSet(ctx, workspaceNameIndexKey(), meta.Name, storageID).Err(); err != nil {
			return err
		}
		if strings.TrimSpace(previous.Name) != "" && previous.Name != meta.Name {
			currentID, err := s.rdb.HGet(ctx, workspaceNameIndexKey(), previous.Name).Result()
			if err == nil && strings.TrimSpace(currentID) == storageID {
				if err := s.rdb.HDel(ctx, workspaceNameIndexKey(), previous.Name).Err(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *Store) GetWorkspaceMeta(ctx context.Context, workspace string) (WorkspaceMeta, error) {
	meta, _, err := s.resolveWorkspaceMeta(ctx, workspace)
	return meta, err
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
	storageID, err := s.resolveWorkspaceStorageOrRaw(ctx, meta.Workspace)
	if err != nil {
		return err
	}
	if storageID == "" {
		return fmt.Errorf("savepoint workspace id is required")
	}
	meta.Workspace = storageID
	if err := setJSON(ctx, s.rdb, savepointMetaKey(storageID, meta.ID), meta); err != nil {
		return err
	}
	if err := setJSON(ctx, s.rdb, savepointManifestKey(storageID, meta.ID), m); err != nil {
		return err
	}
	return s.rdb.ZAdd(ctx, workspaceSavepointsKey(storageID), redis.Z{
		Score:  float64(meta.CreatedAt.UTC().UnixMilli()),
		Member: meta.ID,
	}).Err()
}

func (s *Store) SavepointExists(ctx context.Context, workspace, savepoint string) (bool, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	count, err := s.rdb.Exists(ctx, savepointMetaKey(storageID, savepoint)).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) GetSavepointMeta(ctx context.Context, workspace, savepoint string) (SavepointMeta, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return SavepointMeta{}, err
	}
	return getJSON[SavepointMeta](ctx, s.rdb, savepointMetaKey(storageID, savepoint))
}

func (s *Store) GetManifest(ctx context.Context, workspace, savepoint string) (Manifest, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return Manifest{}, err
	}
	return getJSON[Manifest](ctx, s.rdb, savepointManifestKey(storageID, savepoint))
}

func (s *Store) ListSavepoints(ctx context.Context, workspace string, limit int64) ([]SavepointMeta, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return nil, err
	}
	stop := int64(-1)
	if limit > 0 {
		stop = limit - 1
	}
	ids, err := s.rdb.ZRevRange(ctx, workspaceSavepointsKey(storageID), 0, stop).Result()
	if err != nil {
		return nil, err
	}
	savepoints := make([]SavepointMeta, 0, len(ids))
	for _, id := range ids {
		meta, err := getJSON[SavepointMeta](ctx, s.rdb, savepointMetaKey(storageID, id))
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
	storageID, err := s.resolveWorkspaceStorageOrRaw(ctx, workspace)
	if err != nil {
		return err
	}
	for blobID, data := range blobs {
		if err := s.rdb.SetNX(ctx, blobKey(storageID, blobID), data, 0).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) AddBlobRefs(ctx context.Context, workspace string, m Manifest, createdAt time.Time) error {
	storageID, err := s.resolveWorkspaceStorageOrRaw(ctx, workspace)
	if err != nil {
		return err
	}
	refs := map[string]int64{}
	for _, entry := range m.Entries {
		if entry.BlobID == "" {
			continue
		}
		refs[entry.BlobID] = entry.Size
	}
	for blobID, size := range refs {
		ref, err := s.getBlobRef(ctx, storageID, blobID)
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
		if err := setJSON(ctx, s.rdb, blobRefKey(storageID, blobID), ref); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) getBlobRef(ctx context.Context, workspace, blobID string) (blobRef, error) {
	return getJSON[blobRef](ctx, s.rdb, blobRefKey(workspace, blobID))
}

func (s *Store) GetBlob(ctx context.Context, workspace, blobID string) ([]byte, error) {
	storageID, err := s.resolveWorkspaceStorageOrRaw(ctx, workspace)
	if err != nil {
		return nil, err
	}
	data, err := s.rdb.Get(ctx, blobKey(storageID, blobID)).Bytes()
	if err == redis.Nil {
		return nil, os.ErrNotExist
	}
	return data, err
}

func (s *Store) BlobStats(ctx context.Context, workspace string) (BlobStats, error) {
	stats := BlobStats{}
	storageID, err := s.resolveWorkspaceStorageOrRaw(ctx, workspace)
	if err != nil {
		return stats, err
	}
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, blobRefKey(storageID, "*"), 128).Result()
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
	meta, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return err
	}
	return s.rdb.Watch(ctx, func(tx *redis.Tx) error {
		current, err := getJSON[WorkspaceMeta](ctx, tx, workspaceMetaKey(storageID))
		if err != nil {
			return err
		}
		current.HeadSavepoint = savepoint
		current.UpdatedAt = updatedAt.UTC()
		current.DirtyHint = false

		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			return setJSON(ctx, pipe, workspaceMetaKey(storageID), current)
		})
		return err
	}, workspaceMetaKey(storageID), workspaceNameIndexKey(), workspaceMetaKey(workspaceStorageID(meta)))
}

func (s *Store) PutWorkspaceSession(ctx context.Context, record WorkspaceSessionRecord) error {
	if record.SessionID == "" {
		return fmt.Errorf("workspace session id is required")
	}
	if record.Workspace == "" {
		return fmt.Errorf("workspace session workspace is required")
	}
	_, storageID, err := s.resolveWorkspaceMeta(ctx, record.Workspace)
	if err != nil {
		return err
	}
	if record.LeaseExpiresAt.IsZero() {
		return fmt.Errorf("workspace session lease expiry is required")
	}
	record.Workspace = storageID
	if err := setJSON(ctx, s.rdb, workspaceSessionKey(storageID, record.SessionID), record); err != nil {
		return err
	}
	ttl := time.Until(record.LeaseExpiresAt)
	if ttl <= 0 {
		ttl = time.Second
	}
	if err := s.rdb.Set(ctx, workspaceSessionLeaseKey(storageID, record.SessionID), "1", ttl).Err(); err != nil {
		return err
	}
	return s.rdb.ZAdd(ctx, workspaceSessionsKey(storageID), redis.Z{
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
	_, storageID, err := s.resolveWorkspaceMeta(ctx, record.Workspace)
	if err != nil {
		return err
	}
	record.Workspace = storageID
	if err := setJSONWithTTL(ctx, s.rdb, workspaceSessionKey(storageID, record.SessionID), record, ttl); err != nil {
		return err
	}
	return nil
}

func (s *Store) GetWorkspaceSession(ctx context.Context, workspace, sessionID string) (WorkspaceSessionRecord, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return WorkspaceSessionRecord{}, err
	}
	return getJSON[WorkspaceSessionRecord](ctx, s.rdb, workspaceSessionKey(storageID, sessionID))
}

func (s *Store) RemoveWorkspaceSessionPresence(ctx context.Context, workspace, sessionID string) error {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return err
	}
	_, err = s.rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.ZRem(ctx, workspaceSessionsKey(storageID), sessionID)
		pipe.Del(ctx, workspaceSessionLeaseKey(storageID, sessionID))
		return nil
	})
	return err
}

func (s *Store) ListWorkspaceSessions(ctx context.Context, workspace string) ([]WorkspaceSessionRecord, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return nil, err
	}
	sessionIDs, err := s.rdb.ZRevRange(ctx, workspaceSessionsKey(storageID), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	records := make([]WorkspaceSessionRecord, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		record, err := getJSON[WorkspaceSessionRecord](ctx, s.rdb, workspaceSessionKey(storageID, sessionID))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				_ = s.RemoveWorkspaceSessionPresence(ctx, storageID, sessionID)
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
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return nil, err
	}
	return s.rdb.ZRangeByScore(ctx, workspaceSessionsKey(storageID), &redis.ZRangeBy{
		Min: "-inf",
		Max: strconv.FormatInt(now.UTC().UnixMilli(), 10),
	}).Result()
}

func (s *Store) WorkspaceSessionLeaseAlive(ctx context.Context, workspace, sessionID string) (bool, error) {
	_, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return false, err
	}
	count, err := s.rdb.Exists(ctx, workspaceSessionLeaseKey(storageID, sessionID)).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) Audit(ctx context.Context, workspace, op string, extra map[string]any) error {
	meta, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return err
	}
	fields := map[string]any{
		"ts_ms":     strconv.FormatInt(time.Now().UTC().UnixMilli(), 10),
		"workspace": meta.Name,
		"op":        op,
	}
	for key, value := range extra {
		fields[key] = fmt.Sprint(value)
	}
	return s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: workspaceAuditKey(storageID),
		Values: fields,
	}).Err()
}

func (s *Store) ListAudit(ctx context.Context, workspace string, limit int64) ([]auditRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	meta, storageID, err := s.resolveWorkspaceMeta(ctx, workspace)
	if err != nil {
		return nil, err
	}
	streams, err := s.rdb.XRevRangeN(ctx, workspaceAuditKey(storageID), "+", "-", limit).Result()
	if err != nil {
		return nil, err
	}
	records := make([]auditRecord, 0, len(streams))
	for _, stream := range streams {
		record := auditRecord{
			ID:        stream.ID,
			Workspace: meta.Name,
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
		record.Workspace = defaultString(record.Fields["workspace"], meta.Name)
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

func workspaceSessionLeaseKey(workspace, sessionID string) string {
	return fmt.Sprintf("afs:{%s}:workspace:session:%s:lease", workspace, sessionID)
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
