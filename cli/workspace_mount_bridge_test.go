package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis-fs/mount/client"
	"github.com/redis/go-redis/v9"
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

	sessionMeta, err := store.getSessionMeta(ctx, "repo", "main")
	if err != nil {
		t.Fatalf("getSessionMeta(repo, main) returned error: %v", err)
	}
	if sessionMeta.HeadSavepoint == head {
		t.Fatalf("HeadSavepoint = %q, want a new savepoint after mounted edits", sessionMeta.HeadSavepoint)
	}

	data, err := os.ReadFile(filepath.Join(rafSessionTreePath(cfg, "repo", "main"), "mounted.txt"))
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

	sessionMeta, err := store.getSessionMeta(ctx, "repo", "main")
	if err != nil {
		t.Fatalf("getSessionMeta(repo, main) returned error: %v", err)
	}
	if sessionMeta.HeadSavepoint != head {
		t.Fatalf("HeadSavepoint = %q, want %q when only ignored mount artifacts changed", sessionMeta.HeadSavepoint, head)
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
