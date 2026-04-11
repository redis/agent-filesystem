package controlplane

import (
	"bytes"
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestSyncWorkspaceRootWithOptionsUsesBlobProvider(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewStore(rdb)
	ctx := context.Background()

	large := bytes.Repeat([]byte("x"), inlineThreshold+128)
	m := Manifest{
		Version:   formatVersion,
		Workspace: "demo",
		Savepoint: "snap",
		Entries: map[string]ManifestEntry{
			"/":         {Type: "dir", Mode: 0o755, MtimeMs: 1},
			"/blob.bin": {Type: "file", Mode: 0o644, MtimeMs: 1, Size: int64(len(large)), BlobID: "blob-x"},
		},
	}

	// Blob is NOT stored in Redis. Provider supplies it.
	providerCalls := 0
	opts := SyncOptions{
		BlobProvider: func(id string) ([]byte, bool) {
			providerCalls++
			if id == "blob-x" {
				return large, true
			}
			return nil, false
		},
		SkipNamespaceReset: true,
	}
	if err := SyncWorkspaceRootWithOptions(ctx, store, "demo", m, opts); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if providerCalls == 0 {
		t.Fatalf("expected provider to be called at least once")
	}

	// Confirm materialized content reads from the hash, not Redis blob key.
	content, err := rdb.HGet(ctx, workspaceFSInodeKey("demo", "2"), "content").Bytes()
	if err != nil {
		t.Fatalf("hget content: %v", err)
	}
	if !bytes.Equal(content, large) {
		t.Fatalf("materialized content mismatch")
	}

	// Blob key should not exist because we never wrote it.
	if exists, _ := rdb.Exists(ctx, blobKey("demo", "blob-x")).Result(); exists != 0 {
		t.Fatalf("blob should not have been written to redis")
	}
}

func TestSyncWorkspaceRootWithOptionsSkipsNamespaceReset(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewStore(rdb)
	ctx := context.Background()

	// Pre-seed a fake leftover key in the dirents pattern to prove the
	// reset-skip path doesn't touch it.
	if err := rdb.HSet(ctx, "afs:{demo}:dirents:99", "stale", "yes").Err(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	m := Manifest{
		Version:   formatVersion,
		Workspace: "demo",
		Savepoint: "snap",
		Entries: map[string]ManifestEntry{
			"/":     {Type: "dir", Mode: 0o755, MtimeMs: 1},
			"/a.md": {Type: "file", Mode: 0o644, MtimeMs: 1, Size: 2, Inline: "YWE="},
		},
	}
	if err := SyncWorkspaceRootWithOptions(ctx, store, "demo", m, SyncOptions{SkipNamespaceReset: true}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if val, _ := rdb.HGet(ctx, "afs:{demo}:dirents:99", "stale").Result(); val != "yes" {
		t.Fatalf("namespace reset should have been skipped, but stale key was removed")
	}

	// With reset enabled, the stale key goes away.
	if err := SyncWorkspaceRootWithOptions(ctx, store, "demo", m, SyncOptions{}); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if exists, _ := rdb.Exists(ctx, "afs:{demo}:dirents:99").Result(); exists != 0 {
		t.Fatalf("namespace reset should have cleared stale key")
	}
}
