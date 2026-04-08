package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/rowantrollope/agent-filesystem/internal/controlplane"
	"github.com/rowantrollope/agent-filesystem/mount/client"
)

func TestWorkspaceCommandsImportRunCloneForkListAndDelete(t *testing.T) {
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

	if err := cmdWorkspace([]string{"workspace", "import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdWorkspace(import) returned error: %v", err)
	}
	if err := cmdWorkspace([]string{"workspace", "run", "repo", "--", "/bin/sh", "-c", "true"}); err != nil {
		t.Fatalf("cmdWorkspace(run) returned error: %v", err)
	}

	loadedCfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()

	treePath := afsWorkspaceTreePath(loadedCfg, "repo")
	if _, err := os.Stat(filepath.Join(treePath, "main.go")); err != nil {
		t.Fatalf("expected opened workspace tree to exist: %v", err)
	}

	clonedDir := filepath.Join(t.TempDir(), "repo-clone")
	if err := cmdWorkspace([]string{"workspace", "clone", "repo", clonedDir}); err != nil {
		t.Fatalf("cmdWorkspace(clone) returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(clonedDir, "main.go")); err != nil {
		t.Fatalf("expected cloned directory to contain main.go: %v", err)
	}

	if err := cmdWorkspace([]string{"workspace", "fork", "repo", "repo-copy"}); err != nil {
		t.Fatalf("cmdWorkspace(fork) returned error: %v", err)
	}

	listOutput, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"workspace", "list"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(list) returned error: %v", err)
	}
	if !strings.Contains(listOutput, "repo") || !strings.Contains(listOutput, "repo-copy") {
		t.Fatalf("cmdWorkspace(list) output = %q, want both workspace names", listOutput)
	}

	if err := cmdWorkspace([]string{"workspace", "delete", "repo-copy"}); err != nil {
		t.Fatalf("cmdWorkspace(delete) returned error: %v", err)
	}

	exists, err := store.workspaceExists(context.Background(), "repo-copy")
	if err != nil {
		t.Fatalf("workspaceExists(repo-copy) returned error: %v", err)
	}
	if exists {
		t.Fatal("expected forked workspace to be deleted")
	}
}

func TestWorkspaceCloneRejectsNonEmptyDestination(t *testing.T) {
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
	if err := cmdWorkspace([]string{"workspace", "import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdWorkspace(import) returned error: %v", err)
	}

	targetDir := t.TempDir()
	writeTestFile(t, filepath.Join(targetDir, "existing.txt"), "keep me\n")

	err := cmdWorkspace([]string{"workspace", "clone", "repo", targetDir})
	if err == nil {
		t.Fatal("cmdWorkspace(clone) returned nil error, want destination rejection")
	}
	if !strings.Contains(err.Error(), "not an empty directory") {
		t.Fatalf("cmdWorkspace(clone) error = %q, want non-empty directory rejection", err)
	}
}

func TestCheckpointCommandsCreateAndRestore(t *testing.T) {
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

	if err := cmdWorkspace([]string{"workspace", "import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdWorkspace(import) returned error: %v", err)
	}
	if err := cmdWorkspace([]string{"workspace", "run", "repo", "--", "/bin/sh", "-c", "true"}); err != nil {
		t.Fatalf("cmdWorkspace(run) returned error: %v", err)
	}

	loadedCfg, _, _, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	treePath := afsWorkspaceTreePath(loadedCfg, "repo")
	targetFile := filepath.Join(treePath, "main.go")

	if err := cmdWorkspace([]string{"workspace", "run", "repo", "--", "/bin/sh", "-c", "printf 'package updated\\n' > main.go"}); err != nil {
		t.Fatalf("cmdWorkspace(run updated) returned error: %v", err)
	}
	if err := cmdCheckpoint([]string{"checkpoint", "create", "repo", "after-edit"}); err != nil {
		t.Fatalf("cmdCheckpoint(create) returned error: %v", err)
	}

	if err := os.WriteFile(targetFile, []byte("package broken\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go broken) returned error: %v", err)
	}
	if err := cmdCheckpoint([]string{"checkpoint", "restore", "repo", "after-edit"}); err != nil {
		t.Fatalf("cmdCheckpoint(restore) returned error: %v", err)
	}

	restored, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("ReadFile(main.go) returned error: %v", err)
	}
	if string(restored) != "package updated\n" {
		t.Fatalf("main.go after restore = %q, want %q", string(restored), "package updated\n")
	}

	listOutput, err := captureStdout(t, func() error {
		return cmdCheckpoint([]string{"checkpoint", "list", "repo"})
	})
	if err != nil {
		t.Fatalf("cmdCheckpoint(list) returned error: %v", err)
	}
	if !strings.Contains(listOutput, "initial") || !strings.Contains(listOutput, "after-edit") {
		t.Fatalf("cmdCheckpoint(list) output = %q, want both checkpoint names", listOutput)
	}
	if strings.Contains(listOutput, "T") || strings.Contains(listOutput, "Z") {
		t.Fatalf("cmdCheckpoint(list) output = %q, want human-readable timestamps instead of raw RFC3339", listOutput)
	}
}

func TestCheckpointCreateUsesLiveWorkspaceWhenNoLocalTreeExists(t *testing.T) {
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

	if err := cmdWorkspace([]string{"workspace", "import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdWorkspace(import) returned error: %v", err)
	}

	loadedCfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()

	rootKey, _, _, err := seedWorkspaceMountKey(context.Background(), store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}
	if err := client.New(store.rdb, rootKey).Echo(context.Background(), "/mounted.txt", []byte("hello from live root\n")); err != nil {
		t.Fatalf("Echo(/mounted.txt) returned error: %v", err)
	}

	if err := cmdCheckpoint([]string{"checkpoint", "create", "repo", "after-mounted"}); err != nil {
		t.Fatalf("cmdCheckpoint(create) returned error: %v", err)
	}

	workspaceMeta, err := store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta(repo) returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != "after-mounted" {
		t.Fatalf("HeadSavepoint = %q, want %q", workspaceMeta.HeadSavepoint, "after-mounted")
	}

	manifest, err := store.getManifest(context.Background(), "repo", "after-mounted")
	if err != nil {
		t.Fatalf("getManifest(after-mounted) returned error: %v", err)
	}
	if _, ok := manifest.Entries["/mounted.txt"]; !ok {
		t.Fatal("expected /mounted.txt in after-mounted checkpoint")
	}

	if _, err := os.Stat(afsWorkspaceTreePath(loadedCfg, "repo")); !os.IsNotExist(err) {
		t.Fatalf("expected no local workspace tree, stat err = %v", err)
	}
}

func TestCheckpointCreatePrefersMountedLiveWorkspaceOverLocalTree(t *testing.T) {
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

	cfg := defaultConfig()
	cfg.UseExistingRedis = true
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = "nfs"
	cfg.NFSBin = "/usr/bin/true"
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, "main.go"), "package main\n")

	if err := cmdWorkspace([]string{"workspace", "import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdWorkspace(import) returned error: %v", err)
	}
	if err := cmdWorkspace([]string{"workspace", "run", "repo", "--", "/bin/sh", "-c", "true"}); err != nil {
		t.Fatalf("cmdWorkspace(run) returned error: %v", err)
	}

	_, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()

	rootKey, _, _, err := seedWorkspaceMountKey(context.Background(), store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}
	if err := client.New(store.rdb, rootKey).Echo(context.Background(), "/newfile.txt", []byte("from mounted workspace\n")); err != nil {
		t.Fatalf("Echo(/newfile.txt) returned error: %v", err)
	}

	st := state{
		StartedAt:            time.Now().UTC(),
		ManageRedis:          false,
		RedisAddr:            cfg.RedisAddr,
		RedisDB:              cfg.RedisDB,
		CurrentWorkspace:     "repo",
		MountedHeadSavepoint: "initial",
		MountBackend:         mountBackendNFS,
		Mountpoint:           filepath.Join(t.TempDir(), "mount"),
		RedisKey:             workspaceRedisKey("repo"),
	}
	if err := saveState(st); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	if err := cmdCheckpoint([]string{"checkpoint", "create", "repo", "after-mounted-live"}); err != nil {
		t.Fatalf("cmdCheckpoint(create) returned error: %v", err)
	}

	workspaceMeta, err := store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta(repo) returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != "after-mounted-live" {
		t.Fatalf("HeadSavepoint = %q, want %q", workspaceMeta.HeadSavepoint, "after-mounted-live")
	}

	manifest, err := store.getManifest(context.Background(), "repo", "after-mounted-live")
	if err != nil {
		t.Fatalf("getManifest(after-mounted-live) returned error: %v", err)
	}
	entry, ok := manifest.Entries["/newfile.txt"]
	if !ok {
		t.Fatal("expected /newfile.txt in after-mounted-live checkpoint")
	}
	data, err := controlplane.ManifestEntryData(entry, func(blobID string) ([]byte, error) {
		return store.getBlob(context.Background(), "repo", blobID)
	})
	if err != nil {
		t.Fatalf("ManifestEntryData(/newfile.txt) returned error: %v", err)
	}
	if string(data) != "from mounted workspace\n" {
		t.Fatalf("newfile.txt = %q, want %q", string(data), "from mounted workspace\n")
	}
}

func TestCheckpointCommandsUseCurrentWorkspaceWhenOmitted(t *testing.T) {
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

	cfg := defaultConfig()
	cfg.UseExistingRedis = true
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = "nfs"
	cfg.NFSBin = "/usr/bin/true"
	cfg.WorkRoot = t.TempDir()
	cfg.CurrentWorkspace = "repo"
	saveTempConfig(t, cfg)

	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, "main.go"), "package main\n")

	if err := cmdWorkspace([]string{"workspace", "import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdWorkspace(import) returned error: %v", err)
	}

	_, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()

	rootKey, _, _, err := seedWorkspaceMountKey(context.Background(), store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}
	if err := client.New(store.rdb, rootKey).Echo(context.Background(), "/current-only.txt", []byte("via current workspace\n")); err != nil {
		t.Fatalf("Echo(/current-only.txt) returned error: %v", err)
	}

	st := state{
		StartedAt:            time.Now().UTC(),
		ManageRedis:          false,
		RedisAddr:            cfg.RedisAddr,
		RedisDB:              cfg.RedisDB,
		CurrentWorkspace:     "repo",
		MountedHeadSavepoint: "initial",
		MountBackend:         mountBackendNFS,
		Mountpoint:           filepath.Join(t.TempDir(), "mount"),
		RedisKey:             workspaceRedisKey("repo"),
	}
	if err := saveState(st); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	if err := cmdCheckpoint([]string{"checkpoint", "create", "current-save"}); err != nil {
		t.Fatalf("cmdCheckpoint(create current-save) returned error: %v", err)
	}

	listOutput, err := captureStdout(t, func() error {
		return cmdCheckpoint([]string{"checkpoint", "list"})
	})
	if err != nil {
		t.Fatalf("cmdCheckpoint(list) returned error: %v", err)
	}
	if !strings.Contains(listOutput, "current-save") {
		t.Fatalf("cmdCheckpoint(list) output = %q, want current-save", listOutput)
	}

	if err := cmdCheckpoint([]string{"checkpoint", "restore", "initial"}); err != nil {
		t.Fatalf("cmdCheckpoint(restore initial) returned error: %v", err)
	}

	workspaceMeta, err := store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta(repo) returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != "initial" {
		t.Fatalf("HeadSavepoint = %q, want %q", workspaceMeta.HeadSavepoint, "initial")
	}
}

func TestCheckpointCreateUsesActiveMountedWorkspaceWhenConfigUnset(t *testing.T) {
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

	cfg := defaultConfig()
	cfg.UseExistingRedis = true
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = "nfs"
	cfg.NFSBin = "/usr/bin/true"
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, "main.go"), "package main\n")

	if err := cmdWorkspace([]string{"workspace", "import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdWorkspace(import) returned error: %v", err)
	}

	_, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()

	rootKey, _, _, err := seedWorkspaceMountKey(context.Background(), store, "repo")
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}
	if err := client.New(store.rdb, rootKey).Echo(context.Background(), "/active-state.txt", []byte("from active state\n")); err != nil {
		t.Fatalf("Echo(/active-state.txt) returned error: %v", err)
	}

	if err := saveState(state{
		StartedAt:            time.Now().UTC(),
		ManageRedis:          false,
		RedisAddr:            cfg.RedisAddr,
		RedisDB:              cfg.RedisDB,
		CurrentWorkspace:     "repo",
		MountedHeadSavepoint: "initial",
		MountBackend:         mountBackendNFS,
		Mountpoint:           filepath.Join(t.TempDir(), "mount"),
		RedisKey:             workspaceRedisKey("repo"),
	}); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	if err := cmdCheckpoint([]string{"checkpoint", "create", "active-state-save"}); err != nil {
		t.Fatalf("cmdCheckpoint(create active-state-save) returned error: %v", err)
	}

	workspaceMeta, err := store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta(repo) returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != "active-state-save" {
		t.Fatalf("HeadSavepoint = %q, want %q", workspaceMeta.HeadSavepoint, "active-state-save")
	}
}

func TestWorkspaceCloneAndForkUseCurrentWorkspaceWhenOmitted(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.UseExistingRedis = true
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = "nfs"
	cfg.NFSBin = "/usr/bin/true"
	cfg.WorkRoot = t.TempDir()
	cfg.CurrentWorkspace = "repo"
	saveTempConfig(t, cfg)

	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, "main.go"), "package main\n")

	if err := cmdWorkspace([]string{"workspace", "import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdWorkspace(import) returned error: %v", err)
	}

	clonedDir := filepath.Join(t.TempDir(), "repo-clone")
	if err := cmdWorkspace([]string{"workspace", "clone", clonedDir}); err != nil {
		t.Fatalf("cmdWorkspace(clone omitted workspace) returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(clonedDir, "main.go")); err != nil {
		t.Fatalf("expected cloned directory to contain main.go: %v", err)
	}

	if err := cmdWorkspace([]string{"workspace", "fork", "repo-copy"}); err != nil {
		t.Fatalf("cmdWorkspace(fork omitted source) returned error: %v", err)
	}

	loadedCfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()

	exists, err := store.workspaceExists(context.Background(), "repo-copy")
	if err != nil {
		t.Fatalf("workspaceExists(repo-copy) returned error: %v", err)
	}
	if !exists {
		t.Fatal("expected repo-copy workspace to exist after fork")
	}

	if loadedCfg.CurrentWorkspace != "repo" {
		t.Fatalf("CurrentWorkspace = %q, want %q", loadedCfg.CurrentWorkspace, "repo")
	}
}

func TestWorkspaceRunRejectsLegacyFlag(t *testing.T) {
	t.Helper()

	err := cmdWorkspace([]string{"workspace", "run", "repo", "--session", "main", "--", "/bin/sh", "-c", "true"})
	if err == nil {
		t.Fatal("cmdWorkspace(run) returned nil error, want legacy flag rejection")
	}
	if !strings.Contains(err.Error(), `unknown flag "--session"`) {
		t.Fatalf("cmdWorkspace(run) error = %q, want legacy --session rejection", err)
	}
}

func TestWorkspaceImportRejectsRemovedCloneAtSourceFlag(t *testing.T) {
	t.Helper()

	err := cmdWorkspace([]string{"workspace", "import", "--clone-at-source", "repo", "/tmp/repo"})
	if err == nil {
		t.Fatal("cmdWorkspace(import) returned nil error, want removed flag rejection")
	}
	if !strings.Contains(err.Error(), "--mount-at-source") {
		t.Fatalf("cmdWorkspace(import) error = %q, want replacement flag guidance", err)
	}
}

func TestLoadStateForMountAtSourceRejectsExistingMountedState(t *testing.T) {
	t.Helper()

	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv(HOME) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})

	st := state{
		StartedAt:        time.Now().UTC(),
		CurrentWorkspace: "repo",
		MountBackend:     mountBackendNFS,
		Mountpoint:       filepath.Join(t.TempDir(), "mount"),
		ArchivePath:      filepath.Join(t.TempDir(), "mount.pre-afs"),
	}
	if err := saveState(st); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	_, err := loadStateForMountAtSource()
	if err == nil {
		t.Fatal("loadStateForMountAtSource() returned nil error, want running mount rejection")
	}
	if !strings.Contains(err.Error(), "run '") || !strings.Contains(err.Error(), "down' first") {
		t.Fatalf("loadStateForMountAtSource() error = %q, want afs down guidance", err)
	}
}
