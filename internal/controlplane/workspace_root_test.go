package controlplane

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/rediscontent"
	"github.com/redis/agent-filesystem/internal/searchindex"
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
	searchState, err := rdb.HGet(ctx, workspaceFSInodeKey("repo", strconv.FormatUint(mainStat.Inode, 10)), "search_state").Result()
	if err != nil {
		t.Fatalf("HGet(search_state) returned error: %v", err)
	}
	if searchState != searchindex.StateReady {
		t.Fatalf("search_state = %q, want %q", searchState, searchindex.StateReady)
	}
	grepGrams, err := rdb.HGet(ctx, workspaceFSInodeKey("repo", strconv.FormatUint(mainStat.Inode, 10)), "grep_grams_ci").Result()
	if err != nil {
		t.Fatalf("HGet(grep_grams_ci) returned error: %v", err)
	}
	if grepGrams != searchindex.BuildFileFields(mainGo).GrepGramsCI {
		t.Fatalf("grep_grams_ci = %q, want %q", grepGrams, searchindex.BuildFileFields(mainGo).GrepGramsCI)
	}
	contentRef, err := rdb.HGet(ctx, workspaceFSInodeKey("repo", strconv.FormatUint(mainStat.Inode, 10)), "content_ref").Result()
	if err != nil {
		t.Fatalf("HGet(content_ref) returned error: %v", err)
	}
	if contentRef != "ext" {
		t.Fatalf("content_ref = %q, want %q", contentRef, "ext")
	}
	contentKey := workspaceFSContentKey("repo", strconv.FormatUint(mainStat.Inode, 10))
	content, err := rdb.Get(ctx, contentKey).Bytes()
	if err != nil {
		t.Fatalf("Get(content key) returned error: %v", err)
	}
	if !bytes.Equal(content, mainGo) {
		t.Fatalf("content key = %q, want %q", string(content), string(mainGo))
	}
	if _, err := rdb.HGet(ctx, workspaceFSInodeKey("repo", strconv.FormatUint(mainStat.Inode, 10)), "content").Result(); !errors.Is(err, redis.Nil) {
		t.Fatalf("legacy inline content field should be absent, got error %v", err)
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

	if _, err := rdb.Get(ctx, searchindex.ReadyKey("repo")).Result(); !errors.Is(err, redis.Nil) {
		t.Fatalf("Get(search ready key) error = %v, want redis.Nil", err)
	}
}

func TestSyncWorkspaceRootUsesArrayBackendWhenAvailable(t *testing.T) {
	addr := strings.TrimSpace(os.Getenv("AFS_TEST_ARRAY_REDIS_ADDR"))
	if addr == "" {
		t.Skip("set AFS_TEST_ARRAY_REDIS_ADDR to run Redis Array integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 14})
	t.Cleanup(func() {
		_ = rdb.Close()
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("Ping() returned error: %v", err)
	}
	supported, err := rediscontent.SupportsArrays(ctx, rdb)
	if err != nil {
		t.Fatalf("SupportsArrays() returned error: %v", err)
	}
	if !supported {
		t.Skipf("redis at %s does not support arrays", addr)
	}
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("FlushDB() returned error: %v", err)
	}

	store := NewStore(rdb)
	manifestValue := Manifest{
		Version:   formatVersion,
		Workspace: "repo-array",
		Savepoint: "snapshot",
		Entries: map[string]ManifestEntry{
			"/":          {Type: "dir", Mode: 0o755, MtimeMs: 100},
			"/README.md": {Type: "file", Mode: 0o644, MtimeMs: 110, Size: 6, Inline: base64.StdEncoding.EncodeToString([]byte("hello\n"))},
		},
	}

	if err := SyncWorkspaceRoot(ctx, store, "repo-array", manifestValue); err != nil {
		t.Fatalf("SyncWorkspaceRoot() returned error: %v", err)
	}

	fsClient := client.New(rdb, WorkspaceFSKey("repo-array"))
	readmeStat, err := fsClient.Stat(ctx, "/README.md")
	if err != nil {
		t.Fatalf("Stat(/README.md) returned error: %v", err)
	}
	if readmeStat == nil {
		t.Fatal("expected README.md stat to exist")
	}

	inodeID := strconv.FormatUint(readmeStat.Inode, 10)
	contentRef, err := rdb.HGet(ctx, workspaceFSInodeKey("repo-array", inodeID), "content_ref").Result()
	if err != nil {
		t.Fatalf("HGet(content_ref) returned error: %v", err)
	}
	if contentRef != rediscontent.RefArray {
		t.Fatalf("content_ref = %q, want %q", contentRef, rediscontent.RefArray)
	}

	contentKey := workspaceFSContentKey("repo-array", inodeID)
	length, err := rdb.Do(ctx, "ARLEN", contentKey).Int64()
	if err != nil {
		t.Fatalf("ARLEN(%s) returned error: %v", contentKey, err)
	}
	if length != 1 {
		t.Fatalf("ARLEN(%s) = %d, want 1", contentKey, length)
	}

	data, err := fsClient.Cat(ctx, "/README.md")
	if err != nil {
		t.Fatalf("Cat(/README.md) returned error: %v", err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("Cat(/README.md) = %q, want %q", string(data), "hello\n")
	}
}

func TestScanWorkspaceContentStorageReportsLegacyMixedAndArrayProfiles(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	store := NewStore(rdb)
	ctx := context.Background()
	manifestValue := Manifest{
		Version:   formatVersion,
		Workspace: "repo-storage",
		Savepoint: "snapshot",
		Entries: map[string]ManifestEntry{
			"/":          {Type: "dir", Mode: 0o755, MtimeMs: 100},
			"/README.md": {Type: "file", Mode: 0o644, MtimeMs: 110, Size: 6, Inline: base64.StdEncoding.EncodeToString([]byte("hello\n"))},
			"/notes.txt": {Type: "file", Mode: 0o644, MtimeMs: 120, Size: 5, Inline: base64.StdEncoding.EncodeToString([]byte("todo\n"))},
		},
	}
	if err := SyncWorkspaceRoot(ctx, store, "repo-storage", manifestValue); err != nil {
		t.Fatalf("SyncWorkspaceRoot() returned error: %v", err)
	}

	stats, err := scanWorkspaceContentStorage(ctx, rdb, "repo-storage")
	if err != nil {
		t.Fatalf("scanWorkspaceContentStorage() returned error: %v", err)
	}
	if stats.Profile != workspaceContentStorageLegacy || stats.FileCount != 2 || stats.LegacyFileCount != 2 || stats.ArrayFileCount != 0 {
		t.Fatalf("legacy stats = %+v, want legacy with 2 legacy files", stats)
	}

	fsClient := client.New(rdb, WorkspaceFSKey("repo-storage"))
	readmeStat, err := fsClient.Stat(ctx, "/README.md")
	if err != nil {
		t.Fatalf("Stat(/README.md) returned error: %v", err)
	}
	notesStat, err := fsClient.Stat(ctx, "/notes.txt")
	if err != nil {
		t.Fatalf("Stat(/notes.txt) returned error: %v", err)
	}

	if err := rdb.HSet(ctx, workspaceFSInodeKey("repo-storage", strconv.FormatUint(readmeStat.Inode, 10)), "content_ref", rediscontent.RefArray).Err(); err != nil {
		t.Fatalf("HSet(readme content_ref=array) returned error: %v", err)
	}
	stats, err = scanWorkspaceContentStorage(ctx, rdb, "repo-storage")
	if err != nil {
		t.Fatalf("scanWorkspaceContentStorage(mixed) returned error: %v", err)
	}
	if stats.Profile != workspaceContentStorageMixed || stats.FileCount != 2 || stats.LegacyFileCount != 1 || stats.ArrayFileCount != 1 {
		t.Fatalf("mixed stats = %+v, want mixed with 1 array and 1 legacy file", stats)
	}

	if err := rdb.HSet(ctx, workspaceFSInodeKey("repo-storage", strconv.FormatUint(notesStat.Inode, 10)), "content_ref", rediscontent.RefArray).Err(); err != nil {
		t.Fatalf("HSet(notes content_ref=array) returned error: %v", err)
	}
	stats, err = scanWorkspaceContentStorage(ctx, rdb, "repo-storage")
	if err != nil {
		t.Fatalf("scanWorkspaceContentStorage(array) returned error: %v", err)
	}
	if stats.Profile != workspaceContentStorageArray || stats.FileCount != 2 || stats.LegacyFileCount != 0 || stats.ArrayFileCount != 2 {
		t.Fatalf("array stats = %+v, want array with 2 array files", stats)
	}
}
