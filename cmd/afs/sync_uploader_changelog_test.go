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
	u.mountChangelog(rdb, "ws-123", "sess-abc", "user-1", "agt-sync", "feature/auth", "dev")

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

func TestUploaderChangelogDisabledWhenUnmounted(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	u := newUploader(nil, nil, 0, false, nil)
	// No mountChangelog call.

	u.emitChange(context.Background(), uploadResult{
		Op: uploadOp{Kind: opUploadFile, Path: "x", Content: []byte("a"), LocalHash: "h"},
	})

	stream := controlplane.ChangelogStreamKey("ws-123")
	entries, err := rdb.XRange(context.Background(), stream, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries when unmounted, got %d", len(entries))
	}
}

func TestUploaderEmitChangeRecordsVersionHistoryForTrackedMutations(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	store := newAFSStore(rdb)
	cfg := defaultConfig()
	if err := createEmptyWorkspace(context.Background(), cfg, store, "repo"); err != nil {
		t.Fatalf("createEmptyWorkspace() returned error: %v", err)
	}
	service := controlPlaneServiceFromStore(cfg, store)
	if err := store.cp.PutWorkspaceVersioningPolicy(context.Background(), "repo", controlplane.WorkspaceVersioningPolicy{
		Mode: controlplane.WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}
	meta, err := store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta() returned error: %v", err)
	}

	u := newUploader(nil, nil, 0, false, nil)
	u.attachChangelog(rdb, meta.ID, "sess-sync", "user-1", "agt-sync", "sync/versioning", "dev")

	ctx := context.Background()
	u.emitChange(ctx, uploadResult{
		Op: uploadOp{
			Kind:      opUploadFile,
			Path:      "src/main.go",
			Content:   []byte("package main\n"),
			Mode:      0o644,
			LocalHash: sha256Hex([]byte("package main\n")),
		},
	})
	u.emitChange(ctx, uploadResult{
		Op: uploadOp{
			Kind:      opUploadChmod,
			Path:      "src/main.go",
			Mode:      0o600,
			LocalHash: sha256Hex([]byte("package main\n")),
			HasStored: true,
			StoredEntry: SyncEntry{
				Type:       "file",
				Mode:       0o644,
				Size:       int64(len("package main\n")),
				RemoteHash: sha256Hex([]byte("package main\n")),
			},
		},
	})
	u.emitChange(ctx, uploadResult{
		Op: uploadOp{
			Kind:    opUploadSymlink,
			Path:    "src/link.go",
			Mode:    0o777,
			Symlink: "../main.go",
		},
	})
	u.emitChange(ctx, uploadResult{
		Op: uploadOp{
			Kind:      opUploadDelete,
			Path:      "src/main.go",
			HasStored: true,
			StoredEntry: SyncEntry{
				Type:       "file",
				Mode:       0o600,
				Size:       int64(len("package main\n")),
				RemoteHash: sha256Hex([]byte("package main\n")),
			},
		},
	})
	u.emitChange(ctx, uploadResult{
		Op: uploadOp{
			Kind:      opUploadFile,
			Path:      "src/main.go",
			Content:   []byte("package recreated\n"),
			Mode:      0o644,
			LocalHash: sha256Hex([]byte("package recreated\n")),
		},
	})

	changelog, err := store.cp.ListChangelog(ctx, meta.ID, controlplane.ChangelogListRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListChangelog() returned error: %v", err)
	}
	if len(changelog.Entries) != 5 {
		t.Fatalf("len(changelog.Entries) = %d, want 5", len(changelog.Entries))
	}
	for _, entry := range changelog.Entries {
		if entry.Op == controlplane.ChangeOpMkdir || entry.Op == controlplane.ChangeOpRmdir {
			continue
		}
		if entry.FileID == "" || entry.VersionID == "" {
			t.Fatalf("changelog entry missing version linkage: %+v", entry)
		}
	}

	mainHistory, err := service.GetFileHistory(ctx, "repo", "/src/main.go", false)
	if err != nil {
		t.Fatalf("GetFileHistory(main.go) returned error: %v", err)
	}
	if len(mainHistory.Lineages) != 2 {
		t.Fatalf("len(mainHistory.Lineages) = %d, want 2 after delete and recreate", len(mainHistory.Lineages))
	}
	if len(mainHistory.Lineages[0].Versions) != 3 {
		t.Fatalf("len(mainHistory.Lineages[0].Versions) = %d, want 3 for original lineage", len(mainHistory.Lineages[0].Versions))
	}
	if mainHistory.Lineages[0].Versions[1].Op != controlplane.ChangeOpChmod {
		t.Fatalf("main history chmod version = %#v, want chmod in original lineage", mainHistory.Lineages[0].Versions[1])
	}
	if mainHistory.Lineages[0].Versions[2].Op != controlplane.ChangeOpDelete {
		t.Fatalf("main history delete version = %#v, want delete tombstone in original lineage", mainHistory.Lineages[0].Versions[2])
	}
	if len(mainHistory.Lineages[1].Versions) != 1 || mainHistory.Lineages[1].Versions[0].Op != controlplane.ChangeOpPut {
		t.Fatalf("recreated lineage = %#v, want one recreated put version", mainHistory.Lineages[1].Versions)
	}
	if mainHistory.Lineages[0].FileID == mainHistory.Lineages[1].FileID {
		t.Fatalf("recreated path reused file_id %q, want new lineage", mainHistory.Lineages[0].FileID)
	}

	linkHistory, err := service.GetFileHistory(ctx, "repo", "/src/link.go", false)
	if err != nil {
		t.Fatalf("GetFileHistory(link.go) returned error: %v", err)
	}
	if len(linkHistory.Lineages) != 1 || len(linkHistory.Lineages[0].Versions) != 1 {
		t.Fatalf("linkHistory.Lineages = %#v, want one symlink version", linkHistory.Lineages)
	}
	if linkHistory.Lineages[0].Versions[0].Kind != controlplane.FileVersionKindSymlink {
		t.Fatalf("link version kind = %q, want %q", linkHistory.Lineages[0].Versions[0].Kind, controlplane.FileVersionKindSymlink)
	}
	if linkHistory.Lineages[0].Versions[0].Target != "../main.go" {
		t.Fatalf("link version target = %q, want %q", linkHistory.Lineages[0].Versions[0].Target, "../main.go")
	}
}
