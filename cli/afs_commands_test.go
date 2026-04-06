package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestMaterializeWorkspaceWritesTreeAndState(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, "README.md"), "hello afs\n")
	writeTestFile(t, filepath.Join(sourceDir, "nested", "app.txt"), "data\n")
	if err := os.Symlink("README.md", filepath.Join(sourceDir, "link.txt")); err != nil {
		t.Fatalf("Symlink() returned error: %v", err)
	}

	store := newAFSStore(rdb)
	cfg := defaultConfig()
	cfg.WorkRoot = t.TempDir()

	manifest, blobs, stats, err := buildManifestFromDirectory(sourceDir, "repo", "initial")
	if err != nil {
		t.Fatalf("buildManifestFromDirectory() returned error: %v", err)
	}
	hash, err := hashManifest(manifest)
	if err != nil {
		t.Fatalf("hashManifest() returned error: %v", err)
	}
	now := time.Now().UTC()
	if err := store.saveBlobs(context.Background(), "repo", blobs); err != nil {
		t.Fatalf("saveBlobs() returned error: %v", err)
	}
	if err := store.addBlobRefs(context.Background(), "repo", manifest, now); err != nil {
		t.Fatalf("addBlobRefs() returned error: %v", err)
	}
	if err := store.putWorkspaceMeta(context.Background(), workspaceMeta{
		Version:          afsFormatVersion,
		Name:             "repo",
		CreatedAt:        now,
		UpdatedAt:        now,
		HeadSavepoint:    "initial",
		DefaultSavepoint: "initial",
	}); err != nil {
		t.Fatalf("putWorkspaceMeta() returned error: %v", err)
	}
	if err := store.putSavepoint(context.Background(), savepointMeta{
		Version:      afsFormatVersion,
		ID:           "initial",
		Name:         "initial",
		Workspace:    "repo",
		ManifestHash: hash,
		CreatedAt:    now,
		FileCount:    stats.FileCount,
		DirCount:     stats.DirCount,
		TotalBytes:   stats.TotalBytes,
	}, manifest); err != nil {
		t.Fatalf("putSavepoint() returned error: %v", err)
	}
	if err := materializeWorkspace(context.Background(), store, cfg, "repo"); err != nil {
		t.Fatalf("materializeWorkspace() returned error: %v", err)
	}

	treePath := afsWorkspaceTreePath(cfg, "repo")
	readme, err := os.ReadFile(filepath.Join(treePath, "README.md"))
	if err != nil {
		t.Fatalf("ReadFile(README.md) returned error: %v", err)
	}
	if string(readme) != "hello afs\n" {
		t.Fatalf("README.md = %q, want %q", string(readme), "hello afs\n")
	}

	nested, err := os.ReadFile(filepath.Join(treePath, "nested", "app.txt"))
	if err != nil {
		t.Fatalf("ReadFile(nested/app.txt) returned error: %v", err)
	}
	if string(nested) != "data\n" {
		t.Fatalf("nested/app.txt = %q, want %q", string(nested), "data\n")
	}

	linkTarget, err := os.Readlink(filepath.Join(treePath, "link.txt"))
	if err != nil {
		t.Fatalf("Readlink(link.txt) returned error: %v", err)
	}
	if linkTarget != "README.md" {
		t.Fatalf("Readlink(link.txt) = %q, want %q", linkTarget, "README.md")
	}

	localState, err := loadAFSLocalState(cfg, "repo")
	if err != nil {
		t.Fatalf("loadAFSLocalState() returned error: %v", err)
	}
	if localState.HeadSavepoint != "initial" {
		t.Fatalf("HeadSavepoint = %q, want %q", localState.HeadSavepoint, "initial")
	}
	if localState.Dirty {
		t.Fatal("expected materialized workspace to be clean")
	}
}

func TestCurrentWorkspaceNameUsesConfiguredCurrentWorkspace(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	cfg := defaultConfig()
	cfg.WorkRoot = t.TempDir()
	cfg.CurrentWorkspace = "beta"
	store := newAFSStore(rdb)

	ctx := context.Background()
	if err := createEmptyWorkspace(ctx, cfg, store, "alpha"); err != nil {
		t.Fatalf("createEmptyWorkspace(alpha) returned error: %v", err)
	}
	if err := createEmptyWorkspace(ctx, cfg, store, "beta"); err != nil {
		t.Fatalf("createEmptyWorkspace(beta) returned error: %v", err)
	}

	got, err := currentWorkspaceName(ctx, cfg, store)
	if err != nil {
		t.Fatalf("currentWorkspaceName() returned error: %v", err)
	}
	if got != "beta" {
		t.Fatalf("currentWorkspaceName() = %q, want %q", got, "beta")
	}
}

func TestCurrentWorkspaceNameErrorsWhenConfiguredCurrentWorkspaceMissing(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	cfg := defaultConfig()
	cfg.WorkRoot = t.TempDir()
	cfg.CurrentWorkspace = "missing"
	store := newAFSStore(rdb)

	ctx := context.Background()
	if err := createEmptyWorkspace(ctx, cfg, store, "alpha"); err != nil {
		t.Fatalf("createEmptyWorkspace(alpha) returned error: %v", err)
	}

	_, err := currentWorkspaceName(ctx, cfg, store)
	if err == nil {
		t.Fatal("currentWorkspaceName() returned nil error, want missing current workspace error")
	}
	if !strings.Contains(err.Error(), `current workspace "missing" does not exist`) {
		t.Fatalf("currentWorkspaceName() error = %q, want missing configured current workspace", err)
	}
}

func TestCurrentWorkspaceNameErrorsWhenNoWorkspaceSelected(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	cfg := defaultConfig()
	cfg.WorkRoot = t.TempDir()
	store := newAFSStore(rdb)

	ctx := context.Background()
	if err := createEmptyWorkspace(ctx, cfg, store, "alpha"); err != nil {
		t.Fatalf("createEmptyWorkspace(alpha) returned error: %v", err)
	}

	_, err := currentWorkspaceName(ctx, cfg, store)
	if err == nil {
		t.Fatal("currentWorkspaceName() returned nil error, want no current workspace error")
	}
	if !strings.Contains(err.Error(), "no current workspace is selected") {
		t.Fatalf("currentWorkspaceName() error = %q, want no current workspace selected", err)
	}
}

func TestCmdImportCreatesWorkspaceAndCommandsSucceed(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.UseExistingRedis = true
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = "nfs"
	cfg.NFSBin = "/usr/bin/true"
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, "main.go"), "package main\n")
	writeTestFile(t, filepath.Join(sourceDir, "docs", "notes.md"), "hello\n")

	if err := cmdImport([]string{"import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdImport() returned error: %v", err)
	}

	loadedCfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()

	workspaceMeta, err := store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta() returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != "initial" {
		t.Fatalf("HeadSavepoint = %q, want %q", workspaceMeta.HeadSavepoint, "initial")
	}

	if _, err := loadAFSLocalState(loadedCfg, "repo"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected imported workspace to remain unmaterialized, got err=%v", err)
	}
	if _, err := os.Stat(afsWorkspaceTreePath(loadedCfg, "repo")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no working copy after import, got err=%v", err)
	}

	if err := cmdWorkspace([]string{"workspace", "list"}); err != nil {
		t.Fatalf("cmdWorkspace(list) returned error: %v", err)
	}
	if err := cmdWorkspace([]string{"workspace", "run", "repo", "--", "/bin/sh", "-c", "true"}); err != nil {
		t.Fatalf("cmdWorkspace(run) returned error: %v", err)
	}
	if err := cmdCheckpoint([]string{"checkpoint", "list", "repo"}); err != nil {
		t.Fatalf("cmdCheckpoint(list) returned error: %v", err)
	}
}

func TestCmdImportRespectsAFSIgnore(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.UseExistingRedis = true
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = "nfs"
	cfg.NFSBin = "/usr/bin/true"
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, ".afsignore"), "node_modules/\n*.log\n")
	writeTestFile(t, filepath.Join(sourceDir, "main.go"), "package main\n")
	writeTestFile(t, filepath.Join(sourceDir, "node_modules", "left-pad", "index.js"), "module.exports = 1\n")
	writeTestFile(t, filepath.Join(sourceDir, "debug.log"), "skip me\n")

	if err := cmdImport([]string{"import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdImport() returned error: %v", err)
	}

	loadedCfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()

	manifest, err := store.getManifest(context.Background(), "repo", "initial")
	if err != nil {
		t.Fatalf("getManifest() returned error: %v", err)
	}
	if _, ok := manifest.Entries["/node_modules"]; ok {
		t.Fatal("expected ignored directory to be excluded from manifest")
	}
	if _, ok := manifest.Entries["/debug.log"]; ok {
		t.Fatal("expected ignored file to be excluded from manifest")
	}
	if _, ok := manifest.Entries["/.afsignore"]; !ok {
		t.Fatal("expected .afsignore to be imported")
	}

	if _, err := os.Stat(afsWorkspaceTreePath(loadedCfg, "repo")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no working copy after import, got err=%v", err)
	}
}

func TestCmdImportHandlesEmptyFiles(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.UseExistingRedis = true
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = "nfs"
	cfg.NFSBin = "/usr/bin/true"
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, "main.go"), "package main\n")
	writeTestFile(t, filepath.Join(sourceDir, "empty.txt"), "")

	if err := cmdImport([]string{"import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdImport() returned error: %v", err)
	}

	if err := cmdWorkspace([]string{"workspace", "run", "repo", "--", "/bin/sh", "-c", "true"}); err != nil {
		t.Fatalf("cmdWorkspace(run) returned error: %v", err)
	}

	loadedCfg, _, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()

	data, err := os.ReadFile(filepath.Join(afsWorkspaceTreePath(loadedCfg, "repo"), "empty.txt"))
	if err != nil {
		t.Fatalf("ReadFile(empty.txt) returned error: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("empty.txt length = %d, want 0", len(data))
	}
}

func TestCmdWorkspaceRunCreatesWorkingCopy(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.UseExistingRedis = true
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = "nfs"
	cfg.NFSBin = "/usr/bin/true"
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, "docs", "notes.md"), "hello\n")

	if err := cmdImport([]string{"import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdImport() returned error: %v", err)
	}
	if err := cmdWorkspace([]string{"workspace", "run", "repo", "--", "/bin/sh", "-c", "true"}); err != nil {
		t.Fatalf("cmdWorkspace(run) returned error: %v", err)
	}

	loadedCfg, _, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()

	if _, err := loadAFSLocalState(loadedCfg, "repo"); err != nil {
		t.Fatalf("expected working copy state after clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(afsWorkspaceTreePath(loadedCfg, "repo"), "docs", "notes.md")); err != nil {
		t.Fatalf("expected working copy contents after clone: %v", err)
	}
}

func saveTempConfig(t *testing.T, cfg config) {
	t.Helper()

	configFile := filepath.Join(t.TempDir(), "afs.config.json")
	orig := cfgPathOverride
	cfgPathOverride = configFile
	t.Cleanup(func() {
		cfgPathOverride = orig
	})

	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig() returned error: %v", err)
	}
}
