package controlplane

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/queryindex"
	"github.com/redis/go-redis/v9"
)

func TestQueryIndexStatusDrainsPendingWork(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	store := NewStore(rdb)
	service := NewService(Config{}, store)
	now := time.Now().UTC()
	meta := WorkspaceMeta{
		Version:          formatVersion,
		Name:             "repo",
		CreatedAt:        now,
		UpdatedAt:        now,
		HeadSavepoint:    initialCheckpointName,
		DefaultSavepoint: initialCheckpointName,
	}
	if err := store.PutWorkspaceMeta(ctx, meta); err != nil {
		t.Fatalf("PutWorkspaceMeta() returned error: %v", err)
	}

	manifestValue := Manifest{
		Version:   formatVersion,
		Workspace: "repo",
		Savepoint: initialCheckpointName,
		Entries: map[string]ManifestEntry{
			"/": {Type: "dir", Mode: 0o755, MtimeMs: now.UnixMilli()},
		},
	}
	for i := 0; i < 13; i++ {
		filePath := fmt.Sprintf("/docs/file-%02d.md", i+1)
		content := []byte(fmt.Sprintf("file %02d query index content\n", i+1))
		manifestValue.Entries[filePath] = ManifestEntry{
			Type:    "file",
			Mode:    0o644,
			MtimeMs: now.UnixMilli(),
			Size:    int64(len(content)),
			Inline:  base64.StdEncoding.EncodeToString(content),
		}
	}
	savepoint := SavepointMeta{
		Version:    formatVersion,
		ID:         initialCheckpointName,
		Name:       initialCheckpointName,
		Workspace:  "repo",
		CreatedAt:  now,
		FileCount:  13,
		DirCount:   1,
		TotalBytes: 13,
	}
	if err := store.PutSavepoint(ctx, savepoint, manifestValue); err != nil {
		t.Fatalf("PutSavepoint() returned error: %v", err)
	}

	if _, _, _, err := EnsureWorkspaceRoot(ctx, store, "repo"); err != nil {
		t.Fatalf("EnsureWorkspaceRoot() returned error: %v", err)
	}
	before, err := queryindex.Inspect(ctx, rdb, WorkspaceFSKey("repo"), "/")
	if err != nil {
		t.Fatalf("Inspect(before) returned error: %v", err)
	}
	if before.Pending != 13 {
		t.Fatalf("Inspect(before).Pending = %d, want 13", before.Pending)
	}

	status, err := service.QueryIndexStatus(ctx, "repo", WorkspaceQueryIndexStatusRequest{Path: "/"})
	if err != nil {
		t.Fatalf("QueryIndexStatus() returned error: %v", err)
	}
	if status.Keyword.Pending != 0 || status.Keyword.Stale != 0 {
		t.Fatalf("QueryIndexStatus() keyword = %+v, want drained pending/stale work", status.Keyword)
	}
	if status.Keyword.Ready != 13 {
		t.Fatalf("QueryIndexStatus().Keyword.Ready = %d, want 13", status.Keyword.Ready)
	}
}
