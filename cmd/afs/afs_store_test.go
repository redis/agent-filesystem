package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/rowantrollope/agent-filesystem/internal/controlplane"
)

func TestAFSKeyHelpers(t *testing.T) {
	t.Helper()

	if got := controlplane.WorkspaceMetaKey("repo"); got != "afs:{repo}:workspace:meta" {
		t.Fatalf("WorkspaceMetaKey() = %q", got)
	}
	if got := controlplane.SavepointManifestKey("repo", "initial"); got != "afs:{repo}:savepoint:initial:manifest" {
		t.Fatalf("SavepointManifestKey() = %q", got)
	}
	if got := controlplane.BlobRefKey("repo", "abc123"); got != "afs:{repo}:blobref:abc123" {
		t.Fatalf("BlobRefKey() = %q", got)
	}
}

func TestHashManifestStableAcrossMapOrder(t *testing.T) {
	t.Helper()

	left := manifest{
		Version:   afsFormatVersion,
		Workspace: "repo",
		Savepoint: "initial",
		Entries: map[string]manifestEntry{
			"/b.txt": {Type: "file", Mode: 0o644, MtimeMs: 20, Size: 1, Inline: "Yg=="},
			"/":      {Type: "dir", Mode: 0o755, MtimeMs: 10},
			"/a.txt": {Type: "file", Mode: 0o644, MtimeMs: 15, Size: 1, Inline: "YQ=="},
		},
	}
	right := manifest{
		Version:   afsFormatVersion,
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

func TestAFSStoreRoundTripAndAudit(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	store := newAFSStore(rdb)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	largeData := []byte(strings.Repeat("x", afsInlineThreshold+32))
	largeBlobID := sha256Hex(largeData)

	wsMeta := workspaceMeta{
		Version:          afsFormatVersion,
		Name:             "repo",
		CreatedAt:        now,
		UpdatedAt:        now,
		HeadSavepoint:    "initial",
		DefaultSavepoint: "initial",
	}
	if err := store.putWorkspaceMeta(ctx, wsMeta); err != nil {
		t.Fatalf("putWorkspaceMeta() returned error: %v", err)
	}

	manifest := manifest{
		Version:   afsFormatVersion,
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
		Version:      afsFormatVersion,
		ID:           "initial",
		Name:         "initial",
		Workspace:    "repo",
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

	if err := store.audit(ctx, "repo", "import", map[string]any{"savepoint": "initial"}); err != nil {
		t.Fatalf("audit() returned error: %v", err)
	}

	gotWorkspace, err := store.getWorkspaceMeta(ctx, "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta() returned error: %v", err)
	}
	if gotWorkspace.DefaultSavepoint != "initial" {
		t.Fatalf("DefaultSavepoint = %q, want %q", gotWorkspace.DefaultSavepoint, "initial")
	}
	if gotWorkspace.HeadSavepoint != "initial" {
		t.Fatalf("HeadSavepoint = %q, want %q", gotWorkspace.HeadSavepoint, "initial")
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

	savepoints, err := store.listSavepoints(ctx, "repo", 10)
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

	workspaceAuditLen, err := rdb.XLen(ctx, controlplane.WorkspaceAuditKey("repo")).Result()
	if err != nil {
		t.Fatalf("XLen(workspace audit) returned error: %v", err)
	}
	if workspaceAuditLen != 1 {
		t.Fatalf("workspace audit length = %d, want 1", workspaceAuditLen)
	}
}

func TestAFSStoreListWorkspacesSorted(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	store := newAFSStore(rdb)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	for _, name := range []string{"zeta", "alpha"} {
		if err := store.putWorkspaceMeta(ctx, workspaceMeta{
			Version:          afsFormatVersion,
			Name:             name,
			CreatedAt:        now,
			UpdatedAt:        now,
			HeadSavepoint:    "initial",
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
