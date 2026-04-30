package controlplane

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestManifestDiffAddsCreatedPaths(t *testing.T) {
	parent := Manifest{Entries: map[string]ManifestEntry{}}
	child := Manifest{Entries: map[string]ManifestEntry{
		"/a.txt": {Type: "file", Size: 10, BlobID: "blob-a"},
		"/dir":   {Type: "dir"},
		"/link":  {Type: "symlink", Target: "a.txt"},
	}}

	entries := manifestDiff(parent, child, ChangeEntry{Source: ChangeSourceCheckpoint})
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}

	byPath := map[string]ChangeEntry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}
	if byPath["/a.txt"].Op != ChangeOpPut {
		t.Errorf("a.txt op = %q, want %q", byPath["/a.txt"].Op, ChangeOpPut)
	}
	if byPath["/a.txt"].DeltaBytes != 10 {
		t.Errorf("a.txt delta = %d, want 10", byPath["/a.txt"].DeltaBytes)
	}
	if byPath["/a.txt"].ContentHash != "blob-a" {
		t.Errorf("a.txt hash = %q, want blob-a", byPath["/a.txt"].ContentHash)
	}
	if byPath["/dir"].Op != ChangeOpMkdir {
		t.Errorf("dir op = %q, want %q", byPath["/dir"].Op, ChangeOpMkdir)
	}
	if byPath["/link"].Op != ChangeOpSymlink {
		t.Errorf("link op = %q, want %q", byPath["/link"].Op, ChangeOpSymlink)
	}
	for _, e := range entries {
		if e.Source != ChangeSourceCheckpoint {
			t.Errorf("template source leaked: %q", e.Source)
		}
	}
}

func TestManifestDiffDetectsDeletes(t *testing.T) {
	parent := Manifest{Entries: map[string]ManifestEntry{
		"/a.txt": {Type: "file", Size: 42, BlobID: "blob-a"},
		"/b.txt": {Type: "file", Size: 7, BlobID: "blob-b"},
	}}
	child := Manifest{Entries: map[string]ManifestEntry{
		"/a.txt": {Type: "file", Size: 42, BlobID: "blob-a"},
	}}

	entries := manifestDiff(parent, child, ChangeEntry{})
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Path != "/b.txt" {
		t.Errorf("path = %q, want /b.txt", e.Path)
	}
	if e.Op != ChangeOpDelete {
		t.Errorf("op = %q, want %q", e.Op, ChangeOpDelete)
	}
	if e.DeltaBytes != -7 {
		t.Errorf("delta = %d, want -7", e.DeltaBytes)
	}
	if e.PrevHash != "blob-b" {
		t.Errorf("prev hash = %q, want blob-b", e.PrevHash)
	}
}

func TestManifestDiffDetectsModify(t *testing.T) {
	parent := Manifest{Entries: map[string]ManifestEntry{
		"/a.txt": {Type: "file", Size: 10, BlobID: "blob-v1", MtimeMs: 100},
	}}
	child := Manifest{Entries: map[string]ManifestEntry{
		"/a.txt": {Type: "file", Size: 25, BlobID: "blob-v2", MtimeMs: 200},
	}}

	entries := manifestDiff(parent, child, ChangeEntry{})
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Op != ChangeOpPut {
		t.Errorf("op = %q, want %q", e.Op, ChangeOpPut)
	}
	if e.DeltaBytes != 15 {
		t.Errorf("delta = %d, want 15", e.DeltaBytes)
	}
	if e.PrevHash != "blob-v1" {
		t.Errorf("prev hash = %q, want blob-v1", e.PrevHash)
	}
	if e.ContentHash != "blob-v2" {
		t.Errorf("content hash = %q, want blob-v2", e.ContentHash)
	}
}

func TestManifestDiffDetectsChmodOnly(t *testing.T) {
	parent := Manifest{Entries: map[string]ManifestEntry{
		"/a.txt": {Type: "file", Size: 10, BlobID: "blob-a", Mode: 0644},
	}}
	child := Manifest{Entries: map[string]ManifestEntry{
		"/a.txt": {Type: "file", Size: 10, BlobID: "blob-a", Mode: 0755, MtimeMs: 200},
	}}

	entries := manifestDiff(parent, child, ChangeEntry{})
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Op != ChangeOpChmod {
		t.Errorf("op = %q, want %q", entries[0].Op, ChangeOpChmod)
	}
	if entries[0].DeltaBytes != 0 {
		t.Errorf("delta = %d, want 0", entries[0].DeltaBytes)
	}
}

func TestManifestDiffSkipsEquivalentEntries(t *testing.T) {
	m := Manifest{Entries: map[string]ManifestEntry{
		"/a.txt": {Type: "file", Size: 10, BlobID: "blob-a", MtimeMs: 100},
	}}
	if entries := manifestDiff(m, m, ChangeEntry{}); len(entries) != 0 {
		t.Fatalf("identical manifests must emit no entries, got %d", len(entries))
	}
}

func TestManifestDiffSkipsRootEntry(t *testing.T) {
	child := Manifest{Entries: map[string]ManifestEntry{
		"/":      {Type: "dir"},
		"/a.txt": {Type: "file", Size: 1, BlobID: "b"},
	}}
	entries := manifestDiff(Manifest{}, child, ChangeEntry{})
	if len(entries) != 1 || entries[0].Path != "/a.txt" {
		t.Fatalf("want only /a.txt, got %+v", entries)
	}
}

func TestManifestSeedEntriesEmitsOnePerPath(t *testing.T) {
	m := Manifest{Entries: map[string]ManifestEntry{
		"/a.txt":     {Type: "file", Size: 1, BlobID: "b1"},
		"/sub":       {Type: "dir"},
		"/sub/b.txt": {Type: "file", Size: 2, BlobID: "b2"},
	}}
	entries := manifestSeedEntries(m, ChangeEntry{Source: ChangeSourceImport})
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Source != ChangeSourceImport {
			t.Errorf("entry %q source = %q, want %q", e.Path, e.Source, ChangeSourceImport)
		}
	}
}

func TestEnqueueChangeEntriesWritesStreamAndHashes(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	entries := []ChangeEntry{
		{
			SessionID:    "sess-1",
			Op:           ChangeOpPut,
			Path:         "/a.txt",
			SizeBytes:    10,
			DeltaBytes:   10,
			ContentHash:  "blob-a",
			Source:       ChangeSourceCheckpoint,
			CheckpointID: "cp-1",
		},
		{
			SessionID:  "sess-1",
			Op:         ChangeOpDelete,
			Path:       "/b.txt",
			DeltaBytes: -5,
			PrevHash:   "blob-b",
			Source:     ChangeSourceCheckpoint,
		},
	}

	pipe := store.rdb.Pipeline()
	enqueueChangeEntries(ctx, pipe, "ws1", entries)
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatalf("exec: %v", err)
	}

	resp, err := store.ListChangelog(ctx, "ws1", ChangelogListRequest{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("stream entries = %d, want 2", len(resp.Entries))
	}
	if resp.Entries[0].Path != "/a.txt" || resp.Entries[1].Path != "/b.txt" {
		t.Fatalf("unexpected order: %v / %v", resp.Entries[0].Path, resp.Entries[1].Path)
	}

	summary, err := store.GetSessionChangelogSummary(ctx, "ws1", "sess-1")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.OpCounts[ChangeOpPut] != 1 {
		t.Errorf("put count = %d, want 1", summary.OpCounts[ChangeOpPut])
	}
	if summary.OpCounts[ChangeOpDelete] != 1 {
		t.Errorf("delete count = %d, want 1", summary.OpCounts[ChangeOpDelete])
	}
	if summary.DeltaBytes != 5 {
		t.Errorf("delta bytes = %d, want 5", summary.DeltaBytes)
	}

	last, err := store.GetPathLastWriter(ctx, "ws1", "/a.txt")
	if err != nil {
		t.Fatalf("path last: %v", err)
	}
	if last.SessionID != "sess-1" || last.Op != ChangeOpPut || last.ContentHash != "blob-a" {
		t.Errorf("path-last = %+v, want sess-1/put/blob-a", last)
	}
}

func TestListChangelogFiltersBySession(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	entries := []ChangeEntry{
		{SessionID: "sess-a", Op: ChangeOpPut, Path: "/one", Source: ChangeSourceCheckpoint},
		{SessionID: "sess-b", Op: ChangeOpPut, Path: "/two", Source: ChangeSourceCheckpoint},
		{SessionID: "sess-a", Op: ChangeOpPut, Path: "/three", Source: ChangeSourceCheckpoint},
	}
	pipe := store.rdb.Pipeline()
	enqueueChangeEntries(ctx, pipe, "ws1", entries)
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatalf("exec: %v", err)
	}

	resp, err := store.ListChangelog(ctx, "ws1", ChangelogListRequest{SessionID: "sess-a", Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("filtered count = %d, want 2", len(resp.Entries))
	}
	for _, e := range resp.Entries {
		if e.SessionID != "sess-a" {
			t.Errorf("leaked session %q", e.SessionID)
		}
	}
}

func TestListChangelogPaginationCursorsAreExclusive(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	entries := make([]ChangeEntry, 0, 5)
	for i := 0; i < 5; i++ {
		entries = append(entries, ChangeEntry{
			Op:     ChangeOpPut,
			Path:   "/f" + strconv.Itoa(i),
			Source: ChangeSourceCheckpoint,
		})
	}
	pipe := store.rdb.Pipeline()
	enqueueChangeEntries(ctx, pipe, "ws1", entries)
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatalf("exec: %v", err)
	}

	// Forward pagination: Since should exclude the cursor row.
	page1, err := store.ListChangelog(ctx, "ws1", ChangelogListRequest{Limit: 2})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1.Entries) != 2 {
		t.Fatalf("page1 entries = %d, want 2", len(page1.Entries))
	}
	page2, err := store.ListChangelog(ctx, "ws1", ChangelogListRequest{Since: page1.NextCursor, Limit: 2})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2.Entries) != 2 {
		t.Fatalf("page2 entries = %d, want 2", len(page2.Entries))
	}
	if page2.Entries[0].ID == page1.Entries[len(page1.Entries)-1].ID {
		t.Fatalf("forward pagination duplicated cursor row %q", page2.Entries[0].ID)
	}

	// Reverse pagination: Until should exclude the cursor row.
	rev1, err := store.ListChangelog(ctx, "ws1", ChangelogListRequest{Limit: 2, Reverse: true})
	if err != nil {
		t.Fatalf("rev1: %v", err)
	}
	if len(rev1.Entries) != 2 {
		t.Fatalf("rev1 entries = %d, want 2", len(rev1.Entries))
	}
	rev2, err := store.ListChangelog(ctx, "ws1", ChangelogListRequest{Until: rev1.NextCursor, Limit: 2, Reverse: true})
	if err != nil {
		t.Fatalf("rev2: %v", err)
	}
	if len(rev2.Entries) != 2 {
		t.Fatalf("rev2 entries = %d, want 2", len(rev2.Entries))
	}
	if rev2.Entries[0].ID == rev1.Entries[len(rev1.Entries)-1].ID {
		t.Fatalf("reverse pagination duplicated cursor row %q", rev2.Entries[0].ID)
	}
}

func TestWorkspaceActivityPaginationCursorsAreExclusive(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cfg := Config{RedisConfig: RedisConfig{RedisAddr: mr.Addr()}}
	store := NewStore(rdb)
	ctx := context.Background()

	if err := createWorkspaceWithMetadata(ctx, cfg, store, "repo", workspaceCreateSpec{
		DatabaseID: "db", DatabaseName: "demo", Source: sourceBlank,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := store.Audit(ctx, "repo", "run_start", map[string]any{
			"argv": "cmd-" + strconv.Itoa(i),
		}); err != nil {
			t.Fatalf("audit %d: %v", i, err)
		}
	}

	service := NewService(cfg, store)
	page1, err := service.ListWorkspaceActivityPage(ctx, "repo", ActivityListRequest{Limit: 2})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("page1 items = %d, want 2", len(page1.Items))
	}
	if page1.NextCursor == "" {
		t.Fatal("page1 next cursor is empty")
	}

	page2, err := service.ListWorkspaceActivityPage(ctx, "repo", ActivityListRequest{
		Limit: 2,
		Until: page1.NextCursor,
	})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2.Items) != 2 {
		t.Fatalf("page2 items = %d, want 2", len(page2.Items))
	}
	if page2.Items[0].ID == page1.Items[len(page1.Items)-1].ID {
		t.Fatalf("activity pagination duplicated cursor row %q", page2.Items[0].ID)
	}
}

func TestSaveCheckpointEmitsChangelog(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cfg := Config{RedisConfig: RedisConfig{RedisAddr: mr.Addr()}}
	store := NewStore(rdb)
	ctx := context.Background()

	if err := createWorkspaceWithMetadata(ctx, cfg, store, "repo", workspaceCreateSpec{
		DatabaseID: "db", DatabaseName: "demo", Source: sourceBlank,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	service := NewService(cfg, store)
	meta, err := store.GetWorkspaceMeta(ctx, "repo")
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	storageID := workspaceStorageID(meta)

	// Agent sends a checkpoint with two new files and one new dir.
	now := time.Now().UTC().UnixMilli()
	childManifest := Manifest{
		Version:   formatVersion,
		Workspace: storageID,
		Savepoint: "cp-1",
		Entries: map[string]ManifestEntry{
			"/":          {Type: "dir", Mode: 0o755, MtimeMs: now},
			"/a.txt":     {Type: "file", Mode: 0o644, MtimeMs: now, Size: 3, BlobID: "blob-a"},
			"/sub":       {Type: "dir", Mode: 0o755, MtimeMs: now},
			"/sub/b.txt": {Type: "file", Mode: 0o644, MtimeMs: now, Size: 5, BlobID: "blob-b"},
		},
	}
	ctx = WithChangeSessionContext(ctx, ChangeSessionContext{SessionID: "sess-xyz"})

	saved, err := service.SaveCheckpoint(ctx, SaveCheckpointRequest{
		Workspace:             storageID,
		ExpectedHead:          initialCheckpointName,
		CheckpointID:          "cp-1",
		Description:           "Agent checkpoint.",
		Kind:                  CheckpointKindManual,
		Source:                CheckpointSourceMCP,
		Author:                "codex",
		Manifest:              childManifest,
		Blobs:                 map[string][]byte{"blob-a": []byte("aaa"), "blob-b": []byte("bbbbb")},
		FileCount:             2,
		DirCount:              1,
		TotalBytes:            8,
		SkipWorkspaceRootSync: true,
	})
	if err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	if !saved {
		t.Fatal("expected checkpoint to be saved")
	}
	checkpoint, err := store.GetSavepointMeta(ctx, storageID, "cp-1")
	if err != nil {
		t.Fatalf("GetSavepointMeta: %v", err)
	}
	if checkpoint.Description != "Agent checkpoint." {
		t.Errorf("checkpoint description = %q, want Agent checkpoint.", checkpoint.Description)
	}
	if checkpoint.Kind != CheckpointKindManual {
		t.Errorf("checkpoint kind = %q, want %q", checkpoint.Kind, CheckpointKindManual)
	}
	if checkpoint.Source != CheckpointSourceMCP {
		t.Errorf("checkpoint source = %q, want %q", checkpoint.Source, CheckpointSourceMCP)
	}
	if checkpoint.Author != "codex" {
		t.Errorf("checkpoint author = %q, want codex", checkpoint.Author)
	}
	if checkpoint.SessionID != "sess-xyz" {
		t.Errorf("checkpoint session = %q, want sess-xyz", checkpoint.SessionID)
	}
	if checkpoint.ParentSavepoint != initialCheckpointName {
		t.Errorf("checkpoint parent = %q, want %q", checkpoint.ParentSavepoint, initialCheckpointName)
	}

	resp, err := store.ListChangelog(ctx, storageID, ChangelogListRequest{Limit: 100})
	if err != nil {
		t.Fatalf("ListChangelog: %v", err)
	}
	// Three new paths: /a.txt, /sub, /sub/b.txt (root "/" is skipped by diff).
	if len(resp.Entries) != 3 {
		t.Fatalf("changelog entries = %d, want 3: %+v", len(resp.Entries), resp.Entries)
	}
	for _, e := range resp.Entries {
		if e.SessionID != "sess-xyz" {
			t.Errorf("entry %q session = %q, want sess-xyz", e.Path, e.SessionID)
		}
		if e.CheckpointID != "cp-1" {
			t.Errorf("entry %q checkpoint = %q, want cp-1", e.Path, e.CheckpointID)
		}
		if e.Source != ChangeSourceCheckpoint {
			t.Errorf("entry %q source = %q, want %q", e.Path, e.Source, ChangeSourceCheckpoint)
		}
	}

	summary, err := store.GetSessionChangelogSummary(ctx, storageID, "sess-xyz")
	if err != nil {
		t.Fatalf("GetSessionChangelogSummary: %v", err)
	}
	if summary.OpCounts[ChangeOpPut] != 2 {
		t.Errorf("put count = %d, want 2", summary.OpCounts[ChangeOpPut])
	}
	if summary.OpCounts[ChangeOpMkdir] != 1 {
		t.Errorf("mkdir count = %d, want 1", summary.OpCounts[ChangeOpMkdir])
	}
	if summary.DeltaBytes != 8 {
		t.Errorf("delta bytes = %d, want 8", summary.DeltaBytes)
	}

	last, err := store.GetPathLastWriter(ctx, storageID, "/a.txt")
	if err != nil {
		t.Fatalf("GetPathLastWriter: %v", err)
	}
	if last.SessionID != "sess-xyz" || last.ContentHash != "blob-a" {
		t.Errorf("path-last /a.txt = %+v", last)
	}
}

func TestChangeSessionContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	if _, ok := ChangeSessionContextFromContext(ctx); ok {
		t.Fatal("empty context should not yield session context")
	}
	ctx = WithChangeSessionContext(ctx, ChangeSessionContext{SessionID: "sess-x"})
	sc, ok := ChangeSessionContextFromContext(ctx)
	if !ok || sc.SessionID != "sess-x" {
		t.Fatalf("round trip failed: %+v ok=%v", sc, ok)
	}
	// Empty session ID should leave ctx untouched.
	ctx2 := WithChangeSessionContext(context.Background(), ChangeSessionContext{})
	if _, ok := ChangeSessionContextFromContext(ctx2); ok {
		t.Error("empty session id must not attach context")
	}
}
