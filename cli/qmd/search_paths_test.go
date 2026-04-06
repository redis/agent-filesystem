package qmd

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestEnsurePathFieldsReconstructsCanonicalInodePaths(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ctx := context.Background()
	if err := rdb.HSet(ctx, "afs:{demo}:inode:1", map[string]interface{}{
		"type": "dir",
	}).Err(); err != nil {
		t.Fatalf("HSet(root) returned error: %v", err)
	}
	if err := rdb.HSet(ctx, "afs:{demo}:inode:2", map[string]interface{}{
		"type":   "dir",
		"parent": "1",
		"name":   "src",
	}).Err(); err != nil {
		t.Fatalf("HSet(src) returned error: %v", err)
	}
	if err := rdb.HSet(ctx, "afs:{demo}:inode:3", map[string]interface{}{
		"type":    "file",
		"parent":  "2",
		"name":    "main.go",
		"content": "package main\n",
	}).Err(); err != nil {
		t.Fatalf("HSet(main.go) returned error: %v", err)
	}

	client := NewClient(rdb, "demo", "")
	if client.IndexName() != "afs_idx:{demo}" {
		t.Fatalf("IndexName() = %q, want %q", client.IndexName(), "afs_idx:{demo}")
	}

	updated, err := client.EnsurePathFields(ctx)
	if err != nil {
		t.Fatalf("EnsurePathFields() returned error: %v", err)
	}
	if updated != 3 {
		t.Fatalf("EnsurePathFields() updated %d docs, want 3", updated)
	}

	root, err := rdb.HGetAll(ctx, "afs:{demo}:inode:1").Result()
	if err != nil {
		t.Fatalf("HGetAll(root) returned error: %v", err)
	}
	if root["path"] != "/" {
		t.Fatalf("root path = %q, want %q", root["path"], "/")
	}

	file, err := rdb.HGetAll(ctx, "afs:{demo}:inode:3").Result()
	if err != nil {
		t.Fatalf("HGetAll(main.go) returned error: %v", err)
	}
	if file["path"] != "/src/main.go" {
		t.Fatalf("file path = %q, want %q", file["path"], "/src/main.go")
	}
	if file["path_ancestors"] != "/src,/src/main.go" {
		t.Fatalf("file path_ancestors = %q, want %q", file["path_ancestors"], "/src,/src/main.go")
	}
}
