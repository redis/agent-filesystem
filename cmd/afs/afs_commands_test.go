package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/go-redis/v9"
)

type stubAFSControlPlane struct {
	workspaces controlplane.WorkspaceListResponse
}

func (s stubAFSControlPlane) ListWorkspaceSummaries(context.Context) (controlplane.WorkspaceListResponse, error) {
	return s.workspaces, nil
}

func (s stubAFSControlPlane) GetWorkspace(context.Context, string) (controlplane.WorkspaceDetail, error) {
	return controlplane.WorkspaceDetail{}, fmt.Errorf("unexpected GetWorkspace call")
}

func (s stubAFSControlPlane) CreateWorkspace(context.Context, controlplane.CreateWorkspaceRequest) (controlplane.WorkspaceDetail, error) {
	return controlplane.WorkspaceDetail{}, fmt.Errorf("unexpected CreateWorkspace call")
}

func (s stubAFSControlPlane) ImportWorkspace(context.Context, controlplane.ImportWorkspaceRequest) (controlplane.ImportWorkspaceResponse, error) {
	return controlplane.ImportWorkspaceResponse{}, fmt.Errorf("unexpected ImportWorkspace call")
}

func (s stubAFSControlPlane) DeleteWorkspace(context.Context, string) error {
	return fmt.Errorf("unexpected DeleteWorkspace call")
}

func (s stubAFSControlPlane) CreateWorkspaceSession(context.Context, string, controlplane.CreateWorkspaceSessionRequest) (controlplane.WorkspaceSession, error) {
	return controlplane.WorkspaceSession{}, fmt.Errorf("unexpected CreateWorkspaceSession call")
}

func (s stubAFSControlPlane) HeartbeatWorkspaceSession(context.Context, string, string) (controlplane.WorkspaceSessionInfo, error) {
	return controlplane.WorkspaceSessionInfo{}, fmt.Errorf("unexpected HeartbeatWorkspaceSession call")
}

func (s stubAFSControlPlane) CloseWorkspaceSession(context.Context, string, string) error {
	return fmt.Errorf("unexpected CloseWorkspaceSession call")
}

func (s stubAFSControlPlane) ListCheckpoints(context.Context, string, int) ([]controlplane.CheckpointSummary, error) {
	return nil, fmt.Errorf("unexpected ListCheckpoints call")
}

func (s stubAFSControlPlane) RestoreCheckpoint(context.Context, string, string) error {
	return fmt.Errorf("unexpected RestoreCheckpoint call")
}

func (s stubAFSControlPlane) SaveCheckpoint(context.Context, controlplane.SaveCheckpointRequest) (bool, error) {
	return false, fmt.Errorf("unexpected SaveCheckpoint call")
}

func (s stubAFSControlPlane) SaveCheckpointFromLive(context.Context, string, string) (bool, error) {
	return false, fmt.Errorf("unexpected SaveCheckpointFromLive call")
}

func (s stubAFSControlPlane) ForkWorkspace(context.Context, string, string) error {
	return fmt.Errorf("unexpected ForkWorkspace call")
}

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

func TestCurrentWorkspaceNamePrefersActiveMountedWorkspace(t *testing.T) {
	t.Helper()

	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv(HOME) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.WorkRoot = t.TempDir()
	cfg.CurrentWorkspace = "alpha"
	store := newAFSStore(rdb)

	ctx := context.Background()
	if err := createEmptyWorkspace(ctx, cfg, store, "alpha"); err != nil {
		t.Fatalf("createEmptyWorkspace(alpha) returned error: %v", err)
	}
	if err := createEmptyWorkspace(ctx, cfg, store, "beta"); err != nil {
		t.Fatalf("createEmptyWorkspace(beta) returned error: %v", err)
	}

	if err := saveState(state{
		StartedAt:        time.Now().UTC(),
		CurrentWorkspace: "beta",
		MountBackend:     mountBackendNFS,
		RedisKey:         workspaceRedisKey("beta"),
	}); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	got, err := currentWorkspaceName(ctx, cfg, store)
	if err != nil {
		t.Fatalf("currentWorkspaceName() returned error: %v", err)
	}
	if got != "beta" {
		t.Fatalf("currentWorkspaceName() = %q, want %q", got, "beta")
	}
}

func TestCurrentWorkspaceNamePrefersActiveSyncWorkspace(t *testing.T) {
	t.Helper()

	withTempHome(t)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.WorkRoot = t.TempDir()
	cfg.CurrentWorkspace = "alpha"
	store := newAFSStore(rdb)

	ctx := context.Background()
	if err := createEmptyWorkspace(ctx, cfg, store, "alpha"); err != nil {
		t.Fatalf("createEmptyWorkspace(alpha) returned error: %v", err)
	}
	if err := createEmptyWorkspace(ctx, cfg, store, "beta"); err != nil {
		t.Fatalf("createEmptyWorkspace(beta) returned error: %v", err)
	}

	if err := saveState(state{
		StartedAt:          time.Now().UTC(),
		ProductMode:        productModeLocal,
		RedisAddr:          mr.Addr(),
		RedisDB:            0,
		CurrentWorkspace:   "beta",
		CurrentWorkspaceID: "ws_beta",
		Mode:               modeSync,
		SyncPID:            os.Getpid(),
	}); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	got, err := currentWorkspaceName(ctx, cfg, store)
	if err != nil {
		t.Fatalf("currentWorkspaceName() returned error: %v", err)
	}
	if got != "beta" {
		t.Fatalf("currentWorkspaceName() = %q, want %q", got, "beta")
	}
	if gotRef := selectedWorkspaceReference(cfg); gotRef != "ws_beta" {
		t.Fatalf("selectedWorkspaceReference() = %q, want %q", gotRef, "ws_beta")
	}
}

func TestResolveWorkspaceSelectionFromControlPlanePrefersConfiguredWorkspaceID(t *testing.T) {
	t.Helper()

	withTempHome(t)

	if err := saveState(state{
		StartedAt:          time.Now().UTC(),
		ProductMode:        productModeSelfHosted,
		ControlPlaneURL:    "http://afs.test",
		CurrentWorkspace:   "codex",
		CurrentWorkspaceID: "ws_codex",
		Mode:               modeSync,
		SyncPID:            os.Getpid(),
	}); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = "http://afs.test"
	cfg.CurrentWorkspace = "repo"
	cfg.CurrentWorkspaceID = "ws_repo"

	selection, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, stubAFSControlPlane{
		workspaces: controlplane.WorkspaceListResponse{
			Items: []controlplane.WorkspaceSummary{
				{ID: "ws_repo", Name: "repo"},
				{ID: "ws_codex", Name: "codex"},
			},
		},
	}, "")
	if err != nil {
		t.Fatalf("resolveWorkspaceSelectionFromControlPlane() returned error: %v", err)
	}
	if selection.ID != "ws_repo" || selection.Name != "repo" {
		t.Fatalf("selection = %+v, want ws_repo/repo", selection)
	}
}

func TestResolveWorkspaceSelectionFromControlPlaneDuplicateNameErrorIncludesDatabaseNames(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = "http://afs.test"

	_, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, stubAFSControlPlane{
		workspaces: controlplane.WorkspaceListResponse{
			Items: []controlplane.WorkspaceSummary{
				{ID: "ws_local", Name: "getting-started", DatabaseName: "Local Development"},
				{ID: "ws_cloud", Name: "getting-started", DatabaseName: "Cloud Redis"},
			},
		},
	}, "getting-started")
	if err == nil {
		t.Fatal("resolveWorkspaceSelectionFromControlPlane() returned nil error, want duplicate-name guidance")
	}
	if !strings.Contains(err.Error(), "ws_local (Local Development)") || !strings.Contains(err.Error(), "ws_cloud (Cloud Redis)") {
		t.Fatalf("resolveWorkspaceSelectionFromControlPlane() error = %q, want ids with database names", err)
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
	storageID := workspaceMeta.Name
	if strings.TrimSpace(workspaceMeta.ID) != "" {
		storageID = workspaceMeta.ID
	}
	redisKey := storageID
	liveRootKeys, err := store.rdb.Exists(
		context.Background(),
		"afs:{"+redisKey+"}:info",
		"afs:{"+redisKey+"}:inode:1",
		"afs:{"+redisKey+"}:root_head_savepoint",
	).Result()
	if err != nil {
		t.Fatalf("Exists(live root keys) returned error: %v", err)
	}
	if liveRootKeys != 3 {
		t.Fatalf("expected import to initialize live root, got %d live root keys", liveRootKeys)
	}

	rootHead, err := store.rdb.Get(context.Background(), "afs:{"+redisKey+"}:root_head_savepoint").Result()
	if err != nil {
		t.Fatalf("Get(root_head_savepoint) returned error: %v", err)
	}
	if rootHead != "initial" {
		t.Fatalf("root_head_savepoint = %q, want %q", rootHead, "initial")
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
	clonedDir := filepath.Join(t.TempDir(), "repo-clone")
	if err := cmdWorkspace([]string{"workspace", "clone", "repo", clonedDir}); err != nil {
		t.Fatalf("cmdWorkspace(clone) returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(clonedDir, "main.go")); err != nil {
		t.Fatalf("expected cloned directory to contain main.go: %v", err)
	}
	if err := cmdCheckpoint([]string{"checkpoint", "list", "repo"}); err != nil {
		t.Fatalf("cmdCheckpoint(list) returned error: %v", err)
	}
}

func TestCmdImportSelfHostedCreatesWorkspace(t *testing.T) {
	t.Helper()

	server := newSelfHostedControlPlaneServer(t)

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = server.URL
	cfg.DatabaseID = ""
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, "main.go"), "package main\n")
	writeTestFile(t, filepath.Join(sourceDir, "docs", "notes.md"), "hello\n")

	if err := cmdImport([]string{"import", "codex", sourceDir}); err != nil {
		t.Fatalf("cmdImport() returned error: %v", err)
	}

	_, service, closeFn, err := openAFSControlPlane(context.Background())
	if err != nil {
		t.Fatalf("openAFSControlPlane() returned error: %v", err)
	}
	defer closeFn()

	detail, err := service.GetWorkspace(context.Background(), "codex")
	if err != nil {
		t.Fatalf("GetWorkspace(codex) returned error: %v", err)
	}
	if detail.HeadCheckpointID != "initial" {
		t.Fatalf("HeadCheckpointID = %q, want %q", detail.HeadCheckpointID, "initial")
	}
	if detail.FileCount != 2 {
		t.Fatalf("FileCount = %d, want %d", detail.FileCount, 2)
	}
	if detail.FolderCount != 1 {
		t.Fatalf("FolderCount = %d, want %d", detail.FolderCount, 1)
	}
}

func TestCmdImportRespectsAFSIgnore(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
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

	clonedDir := filepath.Join(t.TempDir(), "repo-clone")
	if err := cmdWorkspace([]string{"workspace", "clone", "repo", clonedDir}); err != nil {
		t.Fatalf("cmdWorkspace(clone) returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(clonedDir, "empty.txt"))
	if err != nil {
		t.Fatalf("ReadFile(empty.txt) returned error: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("empty.txt length = %d, want 0", len(data))
	}
}

func TestCmdWorkspaceCloneCreatesLocalCopy(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
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
	clonedDir := filepath.Join(t.TempDir(), "repo-clone")
	if err := cmdWorkspace([]string{"workspace", "clone", "repo", clonedDir}); err != nil {
		t.Fatalf("cmdWorkspace(clone) returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(clonedDir, "docs", "notes.md")); err != nil {
		t.Fatalf("expected cloned workspace contents: %v", err)
	}
}

func saveTempConfig(t *testing.T, cfg config) {
	t.Helper()

	if cfg.WorkRoot != "" && cfg.WorkRoot != defaultWorkRoot() {
		homeDir := withTempHome(t)
		cfg.WorkRoot = filepath.Join(homeDir, ".afs", "workspaces")
	}

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
