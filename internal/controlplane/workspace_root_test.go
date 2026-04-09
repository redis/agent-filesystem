package controlplane

import (
	"bytes"
	"context"
	"encoding/base64"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

func TestSyncWorkspaceRootMaterializesLiveWorkspaceFS(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	store := NewStore(rdb)
	ctx := context.Background()

	readme := []byte("# demo\n")
	mainGo := []byte("package main\n")
	largeBlob := bytes.Repeat([]byte("x"), inlineThreshold+128)
	if err := store.SaveBlobs(ctx, "repo", map[string][]byte{
		"blob-large": largeBlob,
	}); err != nil {
		t.Fatalf("SaveBlobs() returned error: %v", err)
	}

	manifestValue := Manifest{
		Version:   formatVersion,
		Workspace: "repo",
		Savepoint: "snapshot",
		Entries: map[string]ManifestEntry{
			"/":                 {Type: "dir", Mode: 0o755, MtimeMs: 100},
			"/README.md":        {Type: "file", Mode: 0o644, MtimeMs: 110, Size: int64(len(readme)), Inline: base64.StdEncoding.EncodeToString(readme)},
			"/latest":           {Type: "symlink", Mode: 0o777, MtimeMs: 120, Target: "src/deep/main.go", Size: int64(len("src/deep/main.go"))},
			"/src/deep/main.go": {Type: "file", Mode: 0o644, MtimeMs: 130, Size: int64(len(mainGo)), Inline: base64.StdEncoding.EncodeToString(mainGo)},
			"/assets/large.bin": {Type: "file", Mode: 0o600, MtimeMs: 140, Size: int64(len(largeBlob)), BlobID: "blob-large"},
		},
	}

	if err := SyncWorkspaceRoot(ctx, store, "repo", manifestValue); err != nil {
		t.Fatalf("SyncWorkspaceRoot() returned error: %v", err)
	}

	fsClient := client.New(rdb, WorkspaceFSKey("repo"))
	info, err := fsClient.Info(ctx)
	if err != nil {
		t.Fatalf("Info() returned error: %v", err)
	}
	if info.Files != 3 {
		t.Fatalf("info.Files = %d, want 3", info.Files)
	}
	if info.Directories != 4 {
		t.Fatalf("info.Directories = %d, want 4", info.Directories)
	}
	if info.Symlinks != 1 {
		t.Fatalf("info.Symlinks = %d, want 1", info.Symlinks)
	}
	if info.TotalDataBytes != int64(len(readme)+len(mainGo)+len(largeBlob)) {
		t.Fatalf("info.TotalDataBytes = %d, want %d", info.TotalDataBytes, int64(len(readme)+len(mainGo)+len(largeBlob)))
	}

	readmeData, err := fsClient.Cat(ctx, "/README.md")
	if err != nil {
		t.Fatalf("Cat(/README.md) returned error: %v", err)
	}
	if string(readmeData) != string(readme) {
		t.Fatalf("README.md = %q, want %q", string(readmeData), string(readme))
	}

	mainData, err := fsClient.Cat(ctx, "/src/deep/main.go")
	if err != nil {
		t.Fatalf("Cat(/src/deep/main.go) returned error: %v", err)
	}
	if string(mainData) != string(mainGo) {
		t.Fatalf("main.go = %q, want %q", string(mainData), string(mainGo))
	}

	blobData, err := fsClient.Cat(ctx, "/assets/large.bin")
	if err != nil {
		t.Fatalf("Cat(/assets/large.bin) returned error: %v", err)
	}
	if !bytes.Equal(blobData, largeBlob) {
		t.Fatal("large.bin content mismatch")
	}

	target, err := fsClient.Readlink(ctx, "/latest")
	if err != nil {
		t.Fatalf("Readlink(/latest) returned error: %v", err)
	}
	if target != "src/deep/main.go" {
		t.Fatalf("latest target = %q, want %q", target, "src/deep/main.go")
	}

	mainStat, err := fsClient.Stat(ctx, "/src/deep/main.go")
	if err != nil {
		t.Fatalf("Stat(/src/deep/main.go) returned error: %v", err)
	}
	if mainStat == nil {
		t.Fatal("expected main.go stat to exist")
	}

	ancestors, err := rdb.HGet(ctx, workspaceFSInodeKey("repo", strconv.FormatUint(mainStat.Inode, 10)), "path_ancestors").Result()
	if err != nil {
		t.Fatalf("HGet(path_ancestors) returned error: %v", err)
	}
	if ancestors != "/src,/src/deep,/src/deep/main.go" {
		t.Fatalf("path_ancestors = %q, want %q", ancestors, "/src,/src/deep,/src/deep/main.go")
	}

	rootHead, err := rdb.Get(ctx, workspaceRootHeadKey("repo")).Result()
	if err != nil {
		t.Fatalf("Get(root_head_savepoint) returned error: %v", err)
	}
	if rootHead != "snapshot" {
		t.Fatalf("root_head_savepoint = %q, want %q", rootHead, "snapshot")
	}
	rootDirty, err := rdb.Get(ctx, workspaceRootDirtyKey("repo")).Result()
	if err != nil {
		t.Fatalf("Get(root_dirty) returned error: %v", err)
	}
	if rootDirty != "0" {
		t.Fatalf("root_dirty = %q, want %q", rootDirty, "0")
	}

	nextInode, err := rdb.Get(ctx, workspaceFSNextInodeKey("repo")).Result()
	if err != nil {
		t.Fatalf("Get(next_inode) returned error: %v", err)
	}
	if nextInode != "8" {
		t.Fatalf("next_inode = %q, want %q", nextInode, "8")
	}
}
