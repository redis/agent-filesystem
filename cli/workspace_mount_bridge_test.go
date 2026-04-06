package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/rowantrollope/agent-filesystem/mount/client"
)

func TestEnsureMountWorkspaceCreatesMissingCurrentWorkspace(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	cfg := defaultConfig()
	cfg.UseExistingRedis = true
	cfg.RedisAddr = mr.Addr()
	cfg.WorkRoot = t.TempDir()
	cfg.CurrentWorkspace = "newfiles"

	store := newRAFStore(mustRedisClient(t, cfg))
	defer func() { _ = store.rdb.Close() }()

	workspace, created, err := ensureMountWorkspace(context.Background(), cfg, store)
	if err != nil {
		t.Fatalf("ensureMountWorkspace() returned error: %v", err)
	}
	if workspace != "newfiles" {
		t.Fatalf("workspace = %q, want %q", workspace, "newfiles")
	}
	if !created {
		t.Fatal("expected ensureMountWorkspace() to create the missing workspace")
	}

	exists, err := store.workspaceExists(context.Background(), "newfiles")
	if err != nil {
		t.Fatalf("workspaceExists(newfiles) returned error: %v", err)
	}
	if !exists {
		t.Fatal("expected newfiles workspace to exist after ensureMountWorkspace()")
	}
}

func TestSeedWorkspaceMountKeyUsesCurrentWorkspaceTree(t *testing.T) {
	t.Helper()

	cfg, treePath, store, closeStore := importRAFWorkspaceForTest(t)
	defer closeStore()

	writeTestFile(t, filepath.Join(treePath, "main.go"), "package dirty\n")

	ctx := context.Background()
	mountKey, head, err := seedWorkspaceMountKey(ctx, cfg, store, store.rdb, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}
	if head != "initial" {
		t.Fatalf("head = %q, want %q", head, "initial")
	}

	data, err := client.New(store.rdb, mountKey).Cat(ctx, "/main.go")
	if err != nil {
		t.Fatalf("Cat(/main.go) returned error: %v", err)
	}
	if string(data) != "package dirty\n" {
		t.Fatalf("mounted main.go = %q, want %q", string(data), "package dirty\n")
	}
}

func TestSeedWorkspaceMountKeyUsesCanonicalAFSKeysOnly(t *testing.T) {
	t.Helper()

	cfg, _, store, closeStore := importRAFWorkspaceForTest(t)
	defer closeStore()

	ctx := context.Background()
	mountKey, _, err := seedWorkspaceMountKey(ctx, cfg, store, store.rdb, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}

	legacyKeys, err := store.rdb.Keys(ctx, "rfs:{"+mountKey+"}:*").Result()
	if err != nil {
		t.Fatalf("Keys(legacy mount prefix) returned error: %v", err)
	}
	if len(legacyKeys) != 0 {
		t.Fatalf("expected no legacy rfs mount keys, got %v", legacyKeys)
	}

	keys, err := store.rdb.Keys(ctx, "afs:{"+mountKey+"}:*").Result()
	if err != nil {
		t.Fatalf("Keys(afs mount prefix) returned error: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("expected canonical afs mount keys to exist")
	}

	for _, key := range keys {
		if strings.HasPrefix(key, "afs:{"+mountKey+"}:children:") {
			t.Fatalf("unexpected legacy children key: %s", key)
		}
		if key == "afs:{"+mountKey+"}:inode:/" {
			t.Fatalf("unexpected legacy path-keyed root inode key: %s", key)
		}
	}

	rootExists, err := store.rdb.Exists(ctx, "afs:{"+mountKey+"}:inode:1").Result()
	if err != nil {
		t.Fatalf("Exists(canonical root inode) returned error: %v", err)
	}
	if rootExists != 1 {
		t.Fatal("expected canonical root inode key afs:{<mount>}:inode:1")
	}
}

func TestSyncMountedWorkspaceBackSavesMountChangesIntoWorkspace(t *testing.T) {
	t.Helper()

	cfg, _, store, closeStore := importRAFWorkspaceForTest(t)
	defer closeStore()

	ctx := context.Background()
	mountKey, head, err := seedWorkspaceMountKey(ctx, cfg, store, store.rdb, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}

	fsClient := client.New(store.rdb, mountKey)
	if err := fsClient.Echo(ctx, "/mounted.txt", []byte("hello from mount\n")); err != nil {
		t.Fatalf("Echo(/mounted.txt) returned error: %v", err)
	}

	saved, err := syncMountedWorkspaceBack(ctx, cfg, store, store.rdb, "repo", head)
	if err != nil {
		t.Fatalf("syncMountedWorkspaceBack() returned error: %v", err)
	}
	if !saved {
		t.Fatal("expected mounted workspace sync to create a new savepoint")
	}

	workspaceMeta, err := store.getWorkspaceMeta(ctx, "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta(repo) returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint == head {
		t.Fatalf("HeadSavepoint = %q, want a new savepoint after mounted edits", workspaceMeta.HeadSavepoint)
	}

	data, err := os.ReadFile(filepath.Join(rafWorkspaceTreePath(cfg, "repo"), "mounted.txt"))
	if err != nil {
		t.Fatalf("ReadFile(mounted.txt) returned error: %v", err)
	}
	if string(data) != "hello from mount\n" {
		t.Fatalf("mounted.txt = %q, want %q", string(data), "hello from mount\n")
	}
}

func TestSyncMountedWorkspaceBackIgnoresMountedSystemArtifacts(t *testing.T) {
	t.Helper()

	cfg, _, store, closeStore := importRAFWorkspaceForTest(t)
	defer closeStore()

	ctx := context.Background()
	mountKey, head, err := seedWorkspaceMountKey(ctx, cfg, store, store.rdb, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}

	fsClient := client.New(store.rdb, mountKey)
	if err := fsClient.Touch(ctx, "/.nfs-check"); err != nil {
		t.Fatalf("Touch(/.nfs-check) returned error: %v", err)
	}
	if err := fsClient.Echo(ctx, "/._root.txt", []byte("artifact")); err != nil {
		t.Fatalf("Echo(/._root.txt) returned error: %v", err)
	}

	saved, err := syncMountedWorkspaceBack(ctx, cfg, store, store.rdb, "repo", head)
	if err != nil {
		t.Fatalf("syncMountedWorkspaceBack() returned error: %v", err)
	}
	if saved {
		t.Fatal("expected mounted system artifacts to be ignored during sync")
	}

	workspaceMeta, err := store.getWorkspaceMeta(ctx, "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta(repo) returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != head {
		t.Fatalf("HeadSavepoint = %q, want %q when only ignored mount artifacts changed", workspaceMeta.HeadSavepoint, head)
	}
}

func mustRedisClient(t *testing.T, cfg config) *redis.Client {
	t.Helper()

	rdb := redis.NewClient(buildRedisOptions(cfg, 4))
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		_ = rdb.Close()
		t.Fatalf("Ping() returned error: %v", err)
	}
	return rdb
}
