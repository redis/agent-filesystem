package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/mount/client"
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

	store := newAFSStore(mustRedisClient(t, cfg))
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

func TestSeedWorkspaceMountKeyUsesWorkspaceHeadInsteadOfLocalTree(t *testing.T) {
	t.Helper()

	cfg, store, closeStore := seedWorkspaceMountBridgeFixture(t)
	defer closeStore()

	writeTestFile(t, filepath.Join(afsWorkspaceTreePath(cfg, "repo"), "main.go"), "package local\n")

	ctx := context.Background()
	mountKey, head, initialized, err := seedWorkspaceMountKey(ctx, store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}
	if !initialized {
		t.Fatal("expected first workspace mount open to initialize the live workspace root")
	}
	if head != "initial" {
		t.Fatalf("head = %q, want %q", head, "initial")
	}
	if mountKey != workspaceRedisKey("repo") {
		t.Fatalf("mountKey = %q, want %q", mountKey, workspaceRedisKey("repo"))
	}

	data, err := client.New(store.rdb, mountKey).Cat(ctx, "/main.go")
	if err != nil {
		t.Fatalf("Cat(/main.go) returned error: %v", err)
	}
	if string(data) != "package main\n" {
		t.Fatalf("mounted main.go = %q, want %q", string(data), "package main\n")
	}

	st, err := client.New(store.rdb, mountKey).Stat(ctx, "/main.go")
	if err != nil {
		t.Fatalf("Stat(/main.go) returned error: %v", err)
	}
	if st == nil || st.Inode == 0 {
		t.Fatalf("expected inode for /main.go, got %+v", st)
	}
}

func TestSeedWorkspaceMountKeyUsesCanonicalAFSKeysOnly(t *testing.T) {
	t.Helper()

	_, store, closeStore := seedWorkspaceMountBridgeFixture(t)
	defer closeStore()

	ctx := context.Background()
	mountKey, _, _, err := seedWorkspaceMountKey(ctx, store, "repo")
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

func TestSeedWorkspaceMountKeyKeepsExistingLiveWorkspaceRoot(t *testing.T) {
	t.Helper()

	_, store, closeStore := seedWorkspaceMountBridgeFixture(t)
	defer closeStore()

	ctx := context.Background()
	mountKey, _, initialized, err := seedWorkspaceMountKey(ctx, store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}
	if !initialized {
		t.Fatal("expected first workspace mount open to initialize the live workspace root")
	}

	fsClient := client.New(store.rdb, mountKey)
	if err := fsClient.Echo(ctx, "/live.txt", []byte("live change\n")); err != nil {
		t.Fatalf("Echo(/live.txt) returned error: %v", err)
	}

	secondKey, secondHead, initializedAgain, err := seedWorkspaceMountKey(ctx, store, "repo")
	if err != nil {
		t.Fatalf("second seedWorkspaceMountKey() returned error: %v", err)
	}
	if initializedAgain {
		t.Fatal("expected repeated workspace mount open to reuse the live workspace root")
	}
	if secondKey != mountKey {
		t.Fatalf("second mountKey = %q, want %q", secondKey, mountKey)
	}
	if secondHead != "initial" {
		t.Fatalf("second head = %q, want %q", secondHead, "initial")
	}

	data, err := fsClient.Cat(ctx, "/live.txt")
	if err != nil {
		t.Fatalf("Cat(/live.txt) returned error: %v", err)
	}
	if string(data) != "live change\n" {
		t.Fatalf("live.txt = %q, want %q", string(data), "live change\n")
	}
}

func TestSaveWorkspaceRootCheckpointSavesLiveWorkspaceChangesIntoWorkspace(t *testing.T) {
	t.Helper()

	cfg, store, closeStore := seedWorkspaceMountBridgeFixture(t)
	defer closeStore()

	ctx := context.Background()
	mountKey, head, _, err := seedWorkspaceMountKey(ctx, store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}

	fsClient := client.New(store.rdb, mountKey)
	if err := fsClient.Echo(ctx, "/mounted.txt", []byte("hello from mount\n")); err != nil {
		t.Fatalf("Echo(/mounted.txt) returned error: %v", err)
	}

	saved, err := saveWorkspaceRootCheckpoint(ctx, store, "repo", head, "mounted-save")
	if err != nil {
		t.Fatalf("saveWorkspaceRootCheckpoint() returned error: %v", err)
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

	manifest, err := store.getManifest(ctx, "repo", workspaceMeta.HeadSavepoint)
	if err != nil {
		t.Fatalf("getManifest(new head) returned error: %v", err)
	}
	entry, ok := manifest.Entries["/mounted.txt"]
	if !ok {
		t.Fatal("expected /mounted.txt in saved workspace manifest")
	}
	data, err := controlplane.ManifestEntryData(entry, func(blobID string) ([]byte, error) {
		return store.getBlob(ctx, "repo", blobID)
	})
	if err != nil {
		t.Fatalf("ManifestEntryData(/mounted.txt) returned error: %v", err)
	}
	if string(data) != "hello from mount\n" {
		t.Fatalf("saved /mounted.txt = %q, want %q", string(data), "hello from mount\n")
	}
	if _, err := os.Stat(afsWorkspaceTreePath(cfg, "repo")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no local workspace tree after mounted sync, stat err = %v", err)
	}
}

func TestSaveWorkspaceRootCheckpointIgnoresMountedSystemArtifacts(t *testing.T) {
	t.Helper()

	cfg, store, closeStore := seedWorkspaceMountBridgeFixture(t)
	defer closeStore()

	ctx := context.Background()
	mountKey, head, _, err := seedWorkspaceMountKey(ctx, store, "repo")
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

	saved, err := saveWorkspaceRootCheckpoint(ctx, store, "repo", head, "artifact-only")
	if err != nil {
		t.Fatalf("saveWorkspaceRootCheckpoint() returned error: %v", err)
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
	if _, err := os.Stat(afsWorkspaceTreePath(cfg, "repo")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no local workspace tree after artifact-only sync, stat err = %v", err)
	}
}

func seedWorkspaceMountBridgeFixture(t *testing.T) (config, *afsStore, func()) {
	t.Helper()

	mr := miniredis.RunT(t)
	cfg := defaultConfig()
	cfg.UseExistingRedis = true
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNone
	cfg.WorkRoot = t.TempDir()
	cfg.CurrentWorkspace = "repo"
	saveTempConfig(t, cfg)

	loadedCfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}

	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, "main.go"), "package main\n")
	seedWorkspaceFromDirectory(t, store, "repo", "initial", sourceDir)
	return loadedCfg, store, closeStore
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
