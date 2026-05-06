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
	mountclient "github.com/redis/agent-filesystem/mount/client"
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

func (s stubAFSControlPlane) GetWorkspaceVersioningPolicy(context.Context, string) (controlplane.WorkspaceVersioningPolicy, error) {
	return controlplane.WorkspaceVersioningPolicy{}, fmt.Errorf("unexpected GetWorkspaceVersioningPolicy call")
}

func (s stubAFSControlPlane) GetFileHistory(context.Context, string, string, bool) (controlplane.FileHistoryResponse, error) {
	return controlplane.FileHistoryResponse{}, fmt.Errorf("unexpected GetFileHistory call")
}

func (s stubAFSControlPlane) GetFileHistoryPage(context.Context, string, controlplane.FileHistoryRequest) (controlplane.FileHistoryResponse, error) {
	return controlplane.FileHistoryResponse{}, fmt.Errorf("unexpected GetFileHistoryPage call")
}

func (s stubAFSControlPlane) GetFileVersionContent(context.Context, string, string) (controlplane.FileVersionContentResponse, error) {
	return controlplane.FileVersionContentResponse{}, fmt.Errorf("unexpected GetFileVersionContent call")
}

func (s stubAFSControlPlane) GetFileVersionContentAtOrdinal(context.Context, string, string, int64) (controlplane.FileVersionContentResponse, error) {
	return controlplane.FileVersionContentResponse{}, fmt.Errorf("unexpected GetFileVersionContentAtOrdinal call")
}

func (s stubAFSControlPlane) DiffFileVersions(context.Context, string, string, controlplane.FileVersionDiffOperand, controlplane.FileVersionDiffOperand) (controlplane.FileVersionDiffResponse, error) {
	return controlplane.FileVersionDiffResponse{}, fmt.Errorf("unexpected DiffFileVersions call")
}

func (s stubAFSControlPlane) RestoreFileVersion(context.Context, string, string, controlplane.FileVersionSelector) (controlplane.FileVersionRestoreResponse, error) {
	return controlplane.FileVersionRestoreResponse{}, fmt.Errorf("unexpected RestoreFileVersion call")
}

func (s stubAFSControlPlane) UndeleteFileVersion(context.Context, string, string, controlplane.FileVersionSelector) (controlplane.FileVersionUndeleteResponse, error) {
	return controlplane.FileVersionUndeleteResponse{}, fmt.Errorf("unexpected UndeleteFileVersion call")
}

func (s stubAFSControlPlane) CreateWorkspace(context.Context, controlplane.CreateWorkspaceRequest) (controlplane.WorkspaceDetail, error) {
	return controlplane.WorkspaceDetail{}, fmt.Errorf("unexpected CreateWorkspace call")
}

func (s stubAFSControlPlane) ImportWorkspace(context.Context, controlplane.ImportWorkspaceRequest) (controlplane.ImportWorkspaceResponse, error) {
	return controlplane.ImportWorkspaceResponse{}, fmt.Errorf("unexpected ImportWorkspace call")
}

func (s stubAFSControlPlane) UpdateWorkspaceVersioningPolicy(context.Context, string, controlplane.WorkspaceVersioningPolicy) (controlplane.WorkspaceVersioningPolicy, error) {
	return controlplane.WorkspaceVersioningPolicy{}, fmt.Errorf("unexpected UpdateWorkspaceVersioningPolicy call")
}

func (s stubAFSControlPlane) DeleteWorkspace(context.Context, string) error {
	return fmt.Errorf("unexpected DeleteWorkspace call")
}

func (s stubAFSControlPlane) CreateWorkspaceSession(context.Context, string, controlplane.CreateWorkspaceSessionRequest) (controlplane.WorkspaceSession, error) {
	return controlplane.WorkspaceSession{}, fmt.Errorf("unexpected CreateWorkspaceSession call")
}

func (s stubAFSControlPlane) HeartbeatWorkspaceSession(context.Context, string, string, ...controlplane.CreateWorkspaceSessionRequest) (controlplane.WorkspaceSessionInfo, error) {
	return controlplane.WorkspaceSessionInfo{}, fmt.Errorf("unexpected HeartbeatWorkspaceSession call")
}

func (s stubAFSControlPlane) CloseWorkspaceSession(context.Context, string, string) error {
	return fmt.Errorf("unexpected CloseWorkspaceSession call")
}

func (s stubAFSControlPlane) ListCheckpoints(context.Context, string, int) ([]controlplane.CheckpointSummary, error) {
	return nil, fmt.Errorf("unexpected ListCheckpoints call")
}

func (s stubAFSControlPlane) GetCheckpoint(context.Context, string, string) (controlplane.CheckpointDetail, error) {
	return controlplane.CheckpointDetail{}, fmt.Errorf("unexpected GetCheckpoint call")
}

func (s stubAFSControlPlane) GetTree(context.Context, string, string, string, int) (controlplane.TreeResponse, error) {
	return controlplane.TreeResponse{}, fmt.Errorf("unexpected GetTree call")
}

func (s stubAFSControlPlane) GetFileContent(context.Context, string, string, string) (controlplane.FileContentResponse, error) {
	return controlplane.FileContentResponse{}, fmt.Errorf("unexpected GetFileContent call")
}

func (s stubAFSControlPlane) DiffWorkspace(context.Context, string, string, string) (controlplane.WorkspaceDiffResponse, error) {
	return controlplane.WorkspaceDiffResponse{}, fmt.Errorf("unexpected DiffWorkspace call")
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

func (s stubAFSControlPlane) ListChangelog(context.Context, string, controlplane.ChangelogListRequest) (controlplane.ChangelogListResponse, error) {
	return controlplane.ChangelogListResponse{}, fmt.Errorf("unexpected ListChangelog call")
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

func TestResolveWorkspaceSelectionPrefersCWDMountedWorkspace(t *testing.T) {
	t.Helper()

	homeDir := withTempHome(t)
	mountDir := filepath.Join(homeDir, "beta")
	subDir := filepath.Join(mountDir, "docs")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(mount) returned error: %v", err)
	}
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldCWD)
	})
	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Chdir(subdir) returned error: %v", err)
	}

	cfg := defaultConfig()
	cfg.RedisAddr = "127.0.0.1:6379"
	cfg.CurrentWorkspace = "alpha"
	if err := saveMountRegistry(mountRegistry{Mounts: []mountRecord{{
		ID:          "att_beta",
		Workspace:   "beta",
		WorkspaceID: "ws_beta",
		LocalPath:   mountDir,
		ProductMode: productModeLocal,
		RedisAddr:   cfg.RedisAddr,
		RedisDB:     cfg.RedisDB,
		PID:         os.Getpid(),
		StartedAt:   time.Now().UTC(),
	}}}); err != nil {
		t.Fatalf("saveMountRegistry() returned error: %v", err)
	}

	selection, err := resolveWorkspaceSelectionFromSummaries(cfg, "", []workspaceSummary{
		{ID: "ws_alpha", Name: "alpha"},
		{ID: "ws_beta", Name: "beta"},
	}, false)
	if err != nil {
		t.Fatalf("resolveWorkspaceSelectionFromSummaries() returned error: %v", err)
	}
	if selection.ID != "ws_beta" || selection.Name != "beta" || selection.Source != workspaceSelectionCWD {
		t.Fatalf("selection = %+v, want CWD-mounted beta", selection)
	}
}

func TestResolveWorkspaceSelectionUsesOnlyMountedWorkspace(t *testing.T) {
	t.Helper()

	homeDir := withTempHome(t)
	otherDir := filepath.Join(homeDir, "outside")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(outside) returned error: %v", err)
	}
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldCWD)
	})
	if err := os.Chdir(otherDir); err != nil {
		t.Fatalf("Chdir(outside) returned error: %v", err)
	}

	cfg := defaultConfig()
	cfg.RedisAddr = "127.0.0.1:6379"
	if err := saveMountRegistry(mountRegistry{Mounts: []mountRecord{{
		ID:          "att_beta",
		Workspace:   "beta",
		WorkspaceID: "ws_beta",
		LocalPath:   filepath.Join(homeDir, "beta"),
		ProductMode: productModeLocal,
		RedisAddr:   cfg.RedisAddr,
		RedisDB:     cfg.RedisDB,
		PID:         os.Getpid(),
		StartedAt:   time.Now().UTC(),
	}}}); err != nil {
		t.Fatalf("saveMountRegistry() returned error: %v", err)
	}

	selection, err := resolveWorkspaceSelectionFromSummaries(cfg, "", []workspaceSummary{
		{ID: "ws_beta", Name: "beta"},
	}, false)
	if err != nil {
		t.Fatalf("resolveWorkspaceSelectionFromSummaries() returned error: %v", err)
	}
	if selection.ID != "ws_beta" || selection.Name != "beta" || selection.Source != workspaceSelectionSingleMount {
		t.Fatalf("selection = %+v, want only mounted beta", selection)
	}
}

func TestResolveWorkspaceSelectionMultipleMountsRequireWorkspace(t *testing.T) {
	t.Helper()

	homeDir := withTempHome(t)
	outsideDir := filepath.Join(homeDir, "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(outside) returned error: %v", err)
	}
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldCWD)
	})
	if err := os.Chdir(outsideDir); err != nil {
		t.Fatalf("Chdir(outside) returned error: %v", err)
	}

	cfg := defaultConfig()
	cfg.RedisAddr = "127.0.0.1:6379"
	if err := saveMountRegistry(mountRegistry{Mounts: []mountRecord{
		{
			ID:        "att_alpha",
			Workspace: "alpha",
			LocalPath: filepath.Join(homeDir, "alpha"),
			RedisAddr: cfg.RedisAddr,
			RedisDB:   cfg.RedisDB,
			PID:       os.Getpid(),
		},
		{
			ID:        "att_beta",
			Workspace: "beta",
			LocalPath: filepath.Join(homeDir, "beta"),
			RedisAddr: cfg.RedisAddr,
			RedisDB:   cfg.RedisDB,
			PID:       os.Getpid(),
		},
	}}); err != nil {
		t.Fatalf("saveMountRegistry() returned error: %v", err)
	}

	_, err = resolveWorkspaceSelectionFromSummaries(cfg, "", []workspaceSummary{
		{Name: "alpha"},
		{Name: "beta"},
	}, false)
	if err == nil {
		t.Fatal("resolveWorkspaceSelectionFromSummaries() returned nil error, want multiple-mount guidance")
	}
	if !strings.Contains(err.Error(), "multiple workspaces are mounted") {
		t.Fatalf("error = %q, want multiple-mount guidance", err)
	}
}

func TestResolveWorkspaceSelectionPromptsWhenSavedDefaultIsMissing(t *testing.T) {
	t.Helper()

	withTempHome(t)
	input, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("CreateTemp(stdin) returned error: %v", err)
	}
	if _, err := input.WriteString("2\n"); err != nil {
		t.Fatalf("WriteString(stdin) returned error: %v", err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatalf("Seek(stdin) returned error: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = input.Close()
	})

	cfg := defaultConfig()
	cfg.CurrentWorkspace = "missing"
	selection, err := resolveWorkspaceSelectionFromSummaries(cfg, "", []workspaceSummary{
		{ID: "ws_alpha", Name: "alpha"},
		{ID: "ws_beta", Name: "beta"},
	}, true)
	if err != nil {
		t.Fatalf("resolveWorkspaceSelectionFromSummaries() returned error: %v", err)
	}
	if selection.ID != "ws_beta" || selection.Name != "beta" || selection.Source != workspaceSelectionPrompt {
		t.Fatalf("selection = %+v, want prompted beta", selection)
	}
}

func TestPromptWorkspaceSelectionShowsWorkspaceIDAndDatabase(t *testing.T) {
	t.Helper()

	withTempHome(t)
	input, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("CreateTemp(stdin) returned error: %v", err)
	}
	if _, err := input.WriteString("1\n"); err != nil {
		t.Fatalf("WriteString(stdin) returned error: %v", err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatalf("Seek(stdin) returned error: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = input.Close()
	})

	var selection workspaceSelection
	out, err := captureStdout(t, func() error {
		var runErr error
		selection, runErr = promptWorkspaceSelectionFromSummaries([]workspaceSummary{
			{
				ID:           "ws_alpha",
				Name:         "alpha",
				DatabaseID:   "db_primary",
				DatabaseName: "Primary Redis",
			},
			{
				ID:           "ws_beta",
				Name:         "alpha",
				DatabaseID:   "db_secondary",
				DatabaseName: "Secondary Redis",
			},
		})
		return runErr
	})
	if err != nil {
		t.Fatalf("promptWorkspaceSelectionFromSummaries() returned error: %v", err)
	}
	if selection.ID != "ws_alpha" || selection.Name != "alpha" || selection.Source != workspaceSelectionPrompt {
		t.Fatalf("selection = %+v, want prompted alpha", selection)
	}
	stripped := stripAnsi(out)
	for _, want := range []string{"Workspace ID", "Database", "ws_alpha", "Primary Redis", "ws_beta", "Secondary Redis"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("prompt output = %q, want %q", out, want)
		}
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

func TestResolveWorkspaceSelectionFromControlPlaneDefaultWorkspaceAmbiguityIncludesRecovery(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = "http://afs.test"
	cfg.CurrentWorkspace = "getting-started"

	_, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, stubAFSControlPlane{
		workspaces: controlplane.WorkspaceListResponse{
			Items: []controlplane.WorkspaceSummary{
				{ID: "ws_local", Name: "getting-started", DatabaseName: "Local Development"},
				{ID: "ws_cloud", Name: "getting-started", DatabaseName: "Cloud Redis"},
			},
		},
	}, "")
	if err == nil {
		t.Fatal("resolveWorkspaceSelectionFromControlPlane() returned nil error, want default-workspace ambiguity guidance")
	}
	if !strings.Contains(err.Error(), `default workspace "getting-started" is ambiguous`) {
		t.Fatalf("error = %q, want default workspace ambiguity preamble", err)
	}
	if !strings.Contains(err.Error(), "ws set-default <workspace-id>") {
		t.Fatalf("error = %q, want recovery command", err)
	}
}

func TestResolveWorkspaceSelectionFromControlPlaneDuplicateNameDoesNotSilentlyPreferLegacyID(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = "http://afs.test"

	_, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, stubAFSControlPlane{
		workspaces: controlplane.WorkspaceListResponse{
			Items: []controlplane.WorkspaceSummary{
				{ID: "getting-started", Name: "getting-started", DatabaseName: "Local Development"},
				{ID: "ws_cloud", Name: "getting-started", DatabaseName: "Cloud Redis"},
			},
		},
	}, "getting-started")
	if err == nil {
		t.Fatal("resolveWorkspaceSelectionFromControlPlane() returned nil error, want duplicate-name guidance")
	}
	if !strings.Contains(err.Error(), "workspace id instead") {
		t.Fatalf("error = %q, want duplicate-name guidance instead of implicit legacy-id selection", err)
	}
}

func TestResolveWorkspaceSelectionFromControlPlaneRejectsTypo(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = "http://afs.test"
	// A valid current workspace that exists — the regression was that a
	// typo'd request silently resolved to this instead of erroring.
	cfg.CurrentWorkspace = "repo"
	cfg.CurrentWorkspaceID = "ws_repo"

	_, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, stubAFSControlPlane{
		workspaces: controlplane.WorkspaceListResponse{
			Items: []controlplane.WorkspaceSummary{
				{ID: "ws_repo", Name: "repo"},
			},
		},
	}, "rpo")
	if err == nil {
		t.Fatal("resolveWorkspaceSelectionFromControlPlane(typo) returned nil, want error")
	}
	if !strings.Contains(err.Error(), `"rpo"`) || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %q, want message about the typo'd name", err)
	}
}

func TestCurrentWorkspaceNameErrorsWhenConfiguredDefaultWorkspaceMissing(t *testing.T) {
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
		t.Fatal("currentWorkspaceName() returned nil error, want missing default workspace error")
	}
	if !strings.Contains(err.Error(), `default workspace "missing" does not exist`) {
		t.Fatalf("currentWorkspaceName() error = %q, want missing configured default workspace", err)
	}
}

func TestCurrentWorkspaceNameErrorsWhenNoWorkspaceCanBeInferred(t *testing.T) {
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
	if err := createEmptyWorkspace(ctx, cfg, store, "beta"); err != nil {
		t.Fatalf("createEmptyWorkspace(beta) returned error: %v", err)
	}

	_, err := currentWorkspaceName(ctx, cfg, store)
	if err == nil {
		t.Fatal("currentWorkspaceName() returned nil error, want explicit workspace error")
	}
	if !strings.Contains(err.Error(), "workspace is required") {
		t.Fatalf("currentWorkspaceName() error = %q, want missing workspace guidance", err)
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
	cfg.DatabaseID = "primary"
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

func TestCmdImportDirectForceReplacePreservesVersioningHistory(t *testing.T) {
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

	if err := cmdImport([]string{"import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdImport(initial) returned error: %v", err)
	}

	loadedCfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()

	service := controlPlaneServiceFromStore(loadedCfg, store)
	if _, err := service.UpdateWorkspaceVersioningPolicy(context.Background(), "repo", controlplane.WorkspaceVersioningPolicy{
		Mode: controlplane.WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("UpdateWorkspaceVersioningPolicy() returned error: %v", err)
	}

	writeTestFile(t, filepath.Join(sourceDir, "main.go"), "package main // v2\n")
	if err := cmdImport([]string{"import", "--force", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdImport(force) returned error: %v", err)
	}

	policy, err := service.GetWorkspaceVersioningPolicy(context.Background(), "repo")
	if err != nil {
		t.Fatalf("GetWorkspaceVersioningPolicy() returned error: %v", err)
	}
	if policy.Mode != controlplane.WorkspaceVersioningModeAll {
		t.Fatalf("policy.Mode = %q, want %q", policy.Mode, controlplane.WorkspaceVersioningModeAll)
	}

	history, err := service.GetFileHistoryPage(context.Background(), "repo", controlplane.FileHistoryRequest{
		Path:        "/main.go",
		NewestFirst: false,
	})
	if err != nil {
		t.Fatalf("GetFileHistoryPage() returned error: %v", err)
	}
	if len(history.Lineages) != 1 || len(history.Lineages[0].Versions) != 1 {
		t.Fatalf("history lineages = %#v, want one lineage with one imported version", history.Lineages)
	}
	content, err := service.GetFileVersionContent(context.Background(), "repo", history.Lineages[0].Versions[0].VersionID)
	if err != nil {
		t.Fatalf("GetFileVersionContent() returned error: %v", err)
	}
	if content.Content != "package main // v2\n" {
		t.Fatalf("version content = %q, want %q", content.Content, "package main // v2\n")
	}
}

func TestCmdImportSelfHostedForceReplacePreservesVersioningHistory(t *testing.T) {
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

	if err := cmdImport([]string{"import", "codex", sourceDir}); err != nil {
		t.Fatalf("cmdImport(initial) returned error: %v", err)
	}

	_, service, closeFn, err := openAFSControlPlane(context.Background())
	if err != nil {
		t.Fatalf("openAFSControlPlane() returned error: %v", err)
	}
	defer closeFn()

	if _, err := service.UpdateWorkspaceVersioningPolicy(context.Background(), "codex", controlplane.WorkspaceVersioningPolicy{
		Mode: controlplane.WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("UpdateWorkspaceVersioningPolicy() returned error: %v", err)
	}

	writeTestFile(t, filepath.Join(sourceDir, "main.go"), "package main // hosted v2\n")
	if err := cmdImport([]string{"import", "--force", "codex", sourceDir}); err != nil {
		t.Fatalf("cmdImport(force) returned error: %v", err)
	}

	policy, err := service.GetWorkspaceVersioningPolicy(context.Background(), "codex")
	if err != nil {
		t.Fatalf("GetWorkspaceVersioningPolicy() returned error: %v", err)
	}
	if policy.Mode != controlplane.WorkspaceVersioningModeAll {
		t.Fatalf("policy.Mode = %q, want %q", policy.Mode, controlplane.WorkspaceVersioningModeAll)
	}

	history, err := service.GetFileHistoryPage(context.Background(), "codex", controlplane.FileHistoryRequest{
		Path:        "/main.go",
		NewestFirst: false,
	})
	if err != nil {
		t.Fatalf("GetFileHistoryPage() returned error: %v", err)
	}
	if len(history.Lineages) != 1 || len(history.Lineages[0].Versions) != 1 {
		t.Fatalf("history lineages = %#v, want one lineage with one imported version", history.Lineages)
	}
	content, err := service.GetFileVersionContent(context.Background(), "codex", history.Lineages[0].Versions[0].VersionID)
	if err != nil {
		t.Fatalf("GetFileVersionContent() returned error: %v", err)
	}
	if content.Content != "package main // hosted v2\n" {
		t.Fatalf("version content = %q, want %q", content.Content, "package main // hosted v2\n")
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

func TestCmdWorkspaceCloneIncludesLiveWorkspaceChanges(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = "nfs"
	cfg.NFSBin = "/usr/bin/true"
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, "docs", "notes.md"), "checkpoint\n")

	if err := cmdImport([]string{"import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdImport() returned error: %v", err)
	}

	_, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()

	fsKey, _, _, err := controlplane.EnsureWorkspaceRoot(context.Background(), store.cp, "repo")
	if err != nil {
		t.Fatalf("EnsureWorkspaceRoot() returned error: %v", err)
	}
	fsClient := mountclient.New(store.rdb, fsKey)
	if err := fsClient.Echo(context.Background(), "/docs/notes.md", []byte("live workspace change\n")); err != nil {
		t.Fatalf("Echo(notes.md) returned error: %v", err)
	}
	if err := fsClient.EchoCreate(context.Background(), "/docs/live-only.md", []byte("live only\n"), 0o644); err != nil {
		t.Fatalf("EchoCreate(live-only.md) returned error: %v", err)
	}

	clonedDir := filepath.Join(t.TempDir(), "repo-live-clone")
	if err := cmdWorkspace([]string{"workspace", "clone", "repo", clonedDir}); err != nil {
		t.Fatalf("cmdWorkspace(clone live) returned error: %v", err)
	}

	if got, err := os.ReadFile(filepath.Join(clonedDir, "docs", "notes.md")); err != nil {
		t.Fatalf("ReadFile(notes.md) returned error: %v", err)
	} else if string(got) != "live workspace change\n" {
		t.Fatalf("notes.md content = %q, want %q", string(got), "live workspace change\n")
	}
	if got, err := os.ReadFile(filepath.Join(clonedDir, "docs", "live-only.md")); err != nil {
		t.Fatalf("ReadFile(live-only.md) returned error: %v", err)
	} else if string(got) != "live only\n" {
		t.Fatalf("live-only.md content = %q, want %q", string(got), "live only\n")
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
