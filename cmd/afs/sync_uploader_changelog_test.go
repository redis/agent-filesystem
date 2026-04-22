package main

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/go-redis/v9"
)

// TestUploaderEmitsChangelogEntryPerOp exercises the sync-mode changelog
// emission path: for every successful uploadResult, the uploader should
// XADD one row to the workspace changelog stream tagged agent_sync.
func TestUploaderEmitsChangelogEntryPerOp(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	u := newUploader(nil, nil, 0, false, nil)
	u.attachChangelog(rdb, "ws-123", "sess-abc", "user-1", "agt-sync", "feature/auth", "dev")

	ctx := context.Background()

	// File put (no prior entry): +N bytes, op=put.
	u.emitChange(ctx, uploadResult{
		Op: uploadOp{
			Kind:      opUploadFile,
			Path:      "src/main.go",
			Content:   []byte("package main\n"),
			Mode:      0o644,
			LocalHash: "hash-new",
		},
	})

	// File modify (had prior entry): delta reflects size change.
	u.emitChange(ctx, uploadResult{
		Op: uploadOp{
			Kind:      opUploadFile,
			Path:      "src/main.go",
			Content:   []byte("package main\n// comment\n"),
			Mode:      0o644,
			LocalHash: "hash-mod",
			HasStored: true,
			StoredEntry: SyncEntry{
				Type:       "file",
				Size:       13,
				RemoteHash: "hash-new",
			},
		},
	})

	// Delete of the same path (had prior entry of size 24).
	u.emitChange(ctx, uploadResult{
		Op: uploadOp{
			Kind:      opUploadDelete,
			Path:      "src/main.go",
			HasStored: true,
			StoredEntry: SyncEntry{
				Type:       "file",
				Size:       24,
				RemoteHash: "hash-mod",
			},
		},
	})

	// Errors and conflicts must NOT emit.
	u.emitChange(ctx, uploadResult{
		Op:  uploadOp{Kind: opUploadFile, Path: "err"},
		Err: context.Canceled,
	})
	u.emitChange(ctx, uploadResult{
		Op:       uploadOp{Kind: opUploadFile, Path: "conflict"},
		Conflict: true,
	})

	stream := controlplane.ChangelogStreamKey("ws-123")
	entries, err := rdb.XRange(ctx, stream, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 changelog rows, got %d", len(entries))
	}

	wantOps := []string{"put", "put", "delete"}
	for i, entry := range entries {
		if entry.Values["op"] != wantOps[i] {
			t.Errorf("entry %d op = %v, want %s", i, entry.Values["op"], wantOps[i])
		}
		if entry.Values["session_id"] != "sess-abc" {
			t.Errorf("entry %d session_id = %v, want sess-abc", i, entry.Values["session_id"])
		}
		if entry.Values["agent_id"] != "agt-sync" {
			t.Errorf("entry %d agent_id = %v, want agt-sync", i, entry.Values["agent_id"])
		}
		if entry.Values["source"] != controlplane.ChangeSourceAgentSync {
			t.Errorf("entry %d source = %v, want %s", i, entry.Values["source"], controlplane.ChangeSourceAgentSync)
		}
		if entry.Values["label"] != "feature/auth" {
			t.Errorf("entry %d label = %v, want feature/auth", i, entry.Values["label"])
		}
	}

	// Summary hash should tally the ops.
	sumKey := controlplane.SessionSummaryKey("ws-123", "sess-abc")
	summary, err := rdb.HGetAll(ctx, sumKey).Result()
	if err != nil {
		t.Fatalf("HGetAll: %v", err)
	}
	if summary["op_put"] != "2" {
		t.Errorf("op_put = %v, want 2", summary["op_put"])
	}
	if summary["op_delete"] != "1" {
		t.Errorf("op_delete = %v, want 1", summary["op_delete"])
	}
}

func TestUploaderChangelogDisabledWhenUnattached(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	u := newUploader(nil, nil, 0, false, nil)
	// No attachChangelog call.

	u.emitChange(context.Background(), uploadResult{
		Op: uploadOp{Kind: opUploadFile, Path: "x", Content: []byte("a"), LocalHash: "h"},
	})

	stream := controlplane.ChangelogStreamKey("ws-123")
	entries, err := rdb.XRange(context.Background(), stream, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries when unattached, got %d", len(entries))
	}
}
