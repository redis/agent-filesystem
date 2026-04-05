package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRAFKeyHelpers(t *testing.T) {
	t.Helper()

	if got := rafWorkspaceMetaKey("repo"); got != "raf:{repo}:workspace:meta" {
		t.Fatalf("rafWorkspaceMetaKey() = %q", got)
	}
	if got := rafSessionMetaKey("repo", "main"); got != "raf:{repo}:session:main:meta" {
		t.Fatalf("rafSessionMetaKey() = %q", got)
	}
	if got := rafSavepointManifestKey("repo", "initial"); got != "raf:{repo}:savepoint:initial:manifest" {
		t.Fatalf("rafSavepointManifestKey() = %q", got)
	}
	if got := rafBlobRefKey("repo", "abc123"); got != "raf:{repo}:blobref:abc123" {
		t.Fatalf("rafBlobRefKey() = %q", got)
	}
}

func TestHashManifestStableAcrossMapOrder(t *testing.T) {
	t.Helper()

	left := manifest{
		Version:   rafFormatVersion,
		Workspace: "repo",
		Savepoint: "initial",
		Entries: map[string]manifestEntry{
			"/b.txt": {Type: "file", Mode: 0o644, MtimeMs: 20, Size: 1, Inline: "Yg=="},
			"/":      {Type: "dir", Mode: 0o755, MtimeMs: 10},
			"/a.txt": {Type: "file", Mode: 0o644, MtimeMs: 15, Size: 1, Inline: "YQ=="},
		},
	}
	right := manifest{
		Version:   rafFormatVersion,
		Workspace: "repo",
		Savepoint: "initial",
		Entries: map[string]manifestEntry{
			"/a.txt": {Type: "file", Mode: 0o644, MtimeMs: 15, Size: 1, Inline: "YQ=="},
			"/b.txt": {Type: "file", Mode: 0o644, MtimeMs: 20, Size: 1, Inline: "Yg=="},
			"/":      {Type: "dir", Mode: 0o755, MtimeMs: 10},
		},
	}

	leftHash, err := hashManifest(left)
	if err != nil {
		t.Fatalf("hashManifest(left) returned error: %v", err)
	}
	rightHash, err := hashManifest(right)
	if err != nil {
		t.Fatalf("hashManifest(right) returned error: %v", err)
	}
	if leftHash != rightHash {
		t.Fatalf("hashManifest() mismatch: %q != %q", leftHash, rightHash)
	}
}

func TestRAFStoreRoundTripAndAudit(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	store := newRAFStore(rdb)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	largeData := []byte(strings.Repeat("x", rafInlineThreshold+32))
	largeBlobID := sha256Hex(largeData)

	wsMeta := workspaceMeta{
		Version:          rafFormatVersion,
		Name:             "repo",
		CreatedAt:        now,
		DefaultSession:   "main",
		DefaultSavepoint: "initial",
	}
	if err := store.putWorkspaceMeta(ctx, wsMeta); err != nil {
		t.Fatalf("putWorkspaceMeta() returned error: %v", err)
	}

	manifest := manifest{
		Version:   rafFormatVersion,
		Workspace: "repo",
		Savepoint: "initial",
		Entries: map[string]manifestEntry{
			"/":          {Type: "dir", Mode: 0o755, MtimeMs: now.UnixMilli()},
			"/small.txt": {Type: "file", Mode: 0o644, MtimeMs: now.UnixMilli(), Size: 5, Inline: "aGVsbG8="},
			"/large.bin": {Type: "file", Mode: 0o644, MtimeMs: now.UnixMilli(), Size: int64(len(largeData)), BlobID: largeBlobID},
		},
	}
	hash, err := hashManifest(manifest)
	if err != nil {
		t.Fatalf("hashManifest() returned error: %v", err)
	}

	saveMeta := savepointMeta{
		Version:      rafFormatVersion,
		ID:           "initial",
		Name:         "initial",
		Workspace:    "repo",
		Session:      "main",
		ManifestHash: hash,
		CreatedAt:    now,
		FileCount:    2,
		DirCount:     0,
		TotalBytes:   int64(len("hello")) + int64(len(largeData)),
	}
	if err := store.saveBlobs(ctx, "repo", map[string][]byte{largeBlobID: largeData}); err != nil {
		t.Fatalf("saveBlobs() returned error: %v", err)
	}
	if err := store.addBlobRefs(ctx, "repo", manifest, now); err != nil {
		t.Fatalf("addBlobRefs() returned error: %v", err)
	}
	if err := store.putSavepoint(ctx, saveMeta, manifest); err != nil {
		t.Fatalf("putSavepoint() returned error: %v", err)
	}

	sessionMeta := sessionMeta{
		Version:       rafFormatVersion,
		Workspace:     "repo",
		Name:          "main",
		HeadSavepoint: "initial",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.putSessionMeta(ctx, sessionMeta); err != nil {
		t.Fatalf("putSessionMeta() returned error: %v", err)
	}
	if err := store.audit(ctx, "repo", "main", "import", map[string]any{"savepoint": "initial"}); err != nil {
		t.Fatalf("audit() returned error: %v", err)
	}

	gotWorkspace, err := store.getWorkspaceMeta(ctx, "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta() returned error: %v", err)
	}
	if gotWorkspace.DefaultSavepoint != "initial" {
		t.Fatalf("DefaultSavepoint = %q, want %q", gotWorkspace.DefaultSavepoint, "initial")
	}

	gotSession, err := store.getSessionMeta(ctx, "repo", "main")
	if err != nil {
		t.Fatalf("getSessionMeta() returned error: %v", err)
	}
	if gotSession.HeadSavepoint != "initial" {
		t.Fatalf("HeadSavepoint = %q, want %q", gotSession.HeadSavepoint, "initial")
	}

	gotSavepoint, err := store.getSavepointMeta(ctx, "repo", "initial")
	if err != nil {
		t.Fatalf("getSavepointMeta() returned error: %v", err)
	}
	if gotSavepoint.ManifestHash != hash {
		t.Fatalf("ManifestHash = %q, want %q", gotSavepoint.ManifestHash, hash)
	}

	gotManifest, err := store.getManifest(ctx, "repo", "initial")
	if err != nil {
		t.Fatalf("getManifest() returned error: %v", err)
	}
	if !manifestEquivalent(gotManifest, manifest) {
		t.Fatal("stored manifest does not round-trip")
	}

	sessions, err := store.listSessions(ctx, "repo")
	if err != nil {
		t.Fatalf("listSessions() returned error: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Name != "main" {
		t.Fatalf("listSessions() = %#v, want one main session", sessions)
	}

	savepoints, err := store.listSavepoints(ctx, "repo", "main", 10)
	if err != nil {
		t.Fatalf("listSavepoints() returned error: %v", err)
	}
	if len(savepoints) != 1 || savepoints[0].ID != "initial" {
		t.Fatalf("listSavepoints() = %#v, want one initial savepoint", savepoints)
	}

	stats, err := store.blobStats(ctx, "repo")
	if err != nil {
		t.Fatalf("blobStats() returned error: %v", err)
	}
	if stats.Count != 1 || stats.Bytes != int64(len(largeData)) {
		t.Fatalf("blobStats() = %#v, want count=1 bytes=%d", stats, len(largeData))
	}

	blobData, err := store.getBlob(ctx, "repo", largeBlobID)
	if err != nil {
		t.Fatalf("getBlob() returned error: %v", err)
	}
	if string(blobData) != string(largeData) {
		t.Fatal("getBlob() returned unexpected data")
	}

	workspaceAuditLen, err := rdb.XLen(ctx, rafWorkspaceAuditKey("repo")).Result()
	if err != nil {
		t.Fatalf("XLen(workspace audit) returned error: %v", err)
	}
	sessionAuditLen, err := rdb.XLen(ctx, rafSessionAuditKey("repo", "main")).Result()
	if err != nil {
		t.Fatalf("XLen(session audit) returned error: %v", err)
	}
	if workspaceAuditLen != 1 || sessionAuditLen != 1 {
		t.Fatalf("audit lengths = (%d, %d), want (1, 1)", workspaceAuditLen, sessionAuditLen)
	}
}

func TestRAFStoreListWorkspacesSorted(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	store := newRAFStore(rdb)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	for _, name := range []string{"zeta", "alpha"} {
		if err := store.putWorkspaceMeta(ctx, workspaceMeta{
			Version:          rafFormatVersion,
			Name:             name,
			CreatedAt:        now,
			DefaultSession:   "main",
			DefaultSavepoint: "initial",
		}); err != nil {
			t.Fatalf("putWorkspaceMeta(%q) returned error: %v", name, err)
		}
	}

	workspaces, err := store.listWorkspaces(ctx)
	if err != nil {
		t.Fatalf("listWorkspaces() returned error: %v", err)
	}
	if len(workspaces) != 2 {
		t.Fatalf("len(listWorkspaces()) = %d, want 2", len(workspaces))
	}
	if workspaces[0].Name != "alpha" || workspaces[1].Name != "zeta" {
		t.Fatalf("listWorkspaces() names = %q, %q; want alpha, zeta", workspaces[0].Name, workspaces[1].Name)
	}
}
