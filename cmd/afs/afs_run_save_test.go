package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/mount/client"
)

func TestWorkspaceRunSyncsChangesBackToLiveWorkspaceWithoutCheckpoint(t *testing.T) {
	t.Helper()

	cfg, treePath, store, closeStore := importAFSWorkspaceForTest(t)
	defer closeStore()

	err := cmdWorkspace([]string{"workspace", "run", "repo", "--", "/bin/sh", "-c", "pwd > cwd.txt && printf 'hello\n' > generated.txt"})
	if err != nil {
		t.Fatalf("cmdWorkspace(run) returned error: %v", err)
	}

	cwdBytes, err := os.ReadFile(filepath.Join(treePath, "cwd.txt"))
	if err != nil {
		t.Fatalf("ReadFile(cwd.txt) returned error: %v", err)
	}
	gotPath := strings.TrimSpace(string(cwdBytes))
	resolvedTreePath, err := filepath.EvalSymlinks(treePath)
	if err != nil {
		t.Fatalf("EvalSymlinks(treePath) returned error: %v", err)
	}
	if gotPath != treePath && gotPath != resolvedTreePath {
		t.Fatalf("pwd output = %q, want %q or %q", gotPath, treePath, resolvedTreePath)
	}

	generated, err := os.ReadFile(filepath.Join(treePath, "generated.txt"))
	if err != nil {
		t.Fatalf("ReadFile(generated.txt) returned error: %v", err)
	}
	if string(generated) != "hello\n" {
		t.Fatalf("generated.txt = %q, want %q", string(generated), "hello\n")
	}

	localState, err := loadAFSLocalState(cfg, "repo")
	if err != nil {
		t.Fatalf("loadAFSLocalState() returned error: %v", err)
	}
	if !localState.Dirty {
		t.Fatal("expected workspace run to leave local state dirty")
	}
	if localState.HeadSavepoint != "initial" {
		t.Fatalf("local HeadSavepoint = %q, want %q", localState.HeadSavepoint, "initial")
	}

	workspaceMeta, err := store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta() returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != "initial" {
		t.Fatalf("workspace head = %q, want %q", workspaceMeta.HeadSavepoint, "initial")
	}
	if !workspaceMeta.DirtyHint {
		t.Fatal("expected workspace metadata to report a dirty live workspace after run")
	}
	rootDirty, err := store.rdb.Get(context.Background(), controlplane.WorkspaceRootDirtyKey("repo")).Result()
	if err != nil {
		t.Fatalf("Get(root_dirty) returned error: %v", err)
	}
	if rootDirty != "1" {
		t.Fatalf("root_dirty = %q, want %q", rootDirty, "1")
	}

	liveGenerated, err := client.New(store.rdb, workspaceRedisKey("repo")).Cat(context.Background(), "/generated.txt")
	if err != nil {
		t.Fatalf("Cat(/generated.txt) returned error: %v", err)
	}
	if string(liveGenerated) != "hello\n" {
		t.Fatalf("live generated.txt = %q, want %q", string(liveGenerated), "hello\n")
	}
}

func TestWorkspaceRunReadonlyDiscardsLocalChanges(t *testing.T) {
	t.Helper()

	cfg, treePath, store, closeStore := importAFSWorkspaceForTest(t)
	defer closeStore()

	err := cmdWorkspace([]string{"workspace", "run", "repo", "--readonly", "--", "/bin/sh", "-c", "printf 'hello\n' > generated.txt"})
	if err != nil {
		t.Fatalf("cmdWorkspace(run) returned error: %v", err)
	}

	localState, err := loadAFSLocalState(cfg, "repo")
	if err != nil {
		t.Fatalf("loadAFSLocalState() returned error: %v", err)
	}
	if localState.Dirty {
		t.Fatal("expected readonly run to discard local changes")
	}
	if localState.HeadSavepoint != "initial" {
		t.Fatalf("local HeadSavepoint = %q, want %q", localState.HeadSavepoint, "initial")
	}

	workspaceMeta, err := store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta() returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != "initial" {
		t.Fatalf("workspace HeadSavepoint = %q, want %q", workspaceMeta.HeadSavepoint, "initial")
	}
	if workspaceMeta.DirtyHint {
		t.Fatal("expected readonly run to leave the live workspace clean")
	}
	if _, err := os.Stat(filepath.Join(treePath, "generated.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected generated.txt to be discarded from the local tree, got err=%v", err)
	}
	if st, err := client.New(store.rdb, workspaceRedisKey("repo")).Stat(context.Background(), "/generated.txt"); err == nil && st != nil {
		t.Fatal("expected readonly run to leave the live workspace unchanged")
	}
}

func TestCheckpointCreateNoChangesIsANoop(t *testing.T) {
	t.Helper()

	cfg, _, store, closeStore := importAFSWorkspaceForTest(t)
	defer closeStore()

	output, err := captureStdout(t, func() error {
		return cmdCheckpoint([]string{"checkpoint", "create", "repo"})
	})
	if err != nil {
		t.Fatalf("cmdCheckpoint(create) returned error: %v", err)
	}
	if !strings.Contains(output, "checkpoint unchanged") || !strings.Contains(output, "result") || !strings.Contains(output, "no changes") {
		t.Fatalf("cmdCheckpoint(create) output = %q, want checkpoint unchanged box", output)
	}

	workspaceMeta, err := store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta() returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != "initial" {
		t.Fatalf("HeadSavepoint = %q, want %q", workspaceMeta.HeadSavepoint, "initial")
	}

	localState, err := loadAFSLocalState(cfg, "repo")
	if err != nil {
		t.Fatalf("loadAFSLocalState() returned error: %v", err)
	}
	if localState.Dirty {
		t.Fatal("expected local state to remain clean after no-op checkpoint")
	}
	rootDirty, err := store.rdb.Get(context.Background(), controlplane.WorkspaceRootDirtyKey("repo")).Result()
	if err != nil {
		t.Fatalf("Get(root_dirty) returned error: %v", err)
	}
	if rootDirty != "0" {
		t.Fatalf("root_dirty = %q, want %q", rootDirty, "0")
	}
}

func TestCheckpointCreateUsesLiveWorkspaceRootWithoutLocalTree(t *testing.T) {
	t.Helper()

	cfg, _, store, closeStore := importAFSWorkspaceForTest(t)
	defer closeStore()

	if err := client.New(store.rdb, workspaceRedisKey("repo")).Echo(context.Background(), "/live-only.txt", []byte("from live root\n")); err != nil {
		t.Fatalf("Echo(/live-only.txt) returned error: %v", err)
	}

	if err := removeLocalWorkspace(cfg, "repo"); err != nil {
		t.Fatalf("removeLocalWorkspace() returned error: %v", err)
	}

	if err := cmdCheckpoint([]string{"checkpoint", "create", "repo", "live-root-save"}); err != nil {
		t.Fatalf("cmdCheckpoint(create) returned error: %v", err)
	}

	workspaceMeta, err := store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta() returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != "live-root-save" {
		t.Fatalf("workspace head = %q, want %q", workspaceMeta.HeadSavepoint, "live-root-save")
	}
	rootDirty, err := store.rdb.Get(context.Background(), controlplane.WorkspaceRootDirtyKey("repo")).Result()
	if err != nil {
		t.Fatalf("Get(root_dirty) returned error: %v", err)
	}
	if rootDirty != "0" {
		t.Fatalf("root_dirty = %q, want %q", rootDirty, "0")
	}

	savedManifest, err := store.getManifest(context.Background(), "repo", "live-root-save")
	if err != nil {
		t.Fatalf("getManifest(live-root-save) returned error: %v", err)
	}
	if _, ok := savedManifest.Entries["/live-only.txt"]; !ok {
		t.Fatal("expected checkpoint to capture the live workspace root without a local tree")
	}
}

func TestCheckpointRestoreLeavesLocalWorkspaceUntouched(t *testing.T) {
	t.Helper()

	cfg, treePath, store, closeStore := importAFSWorkspaceForTest(t)
	defer closeStore()

	if err := cmdWorkspace([]string{"workspace", "run", "repo", "--", "/bin/sh", "-c", "printf 'package saved\\n' > main.go"}); err != nil {
		t.Fatalf("cmdWorkspace(run) returned error: %v", err)
	}
	if err := cmdCheckpoint([]string{"checkpoint", "create", "repo", "after-edit"}); err != nil {
		t.Fatalf("cmdCheckpoint(create) returned error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(treePath, "main.go"), []byte("package dirty\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go dirty) returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(treePath, "scratch.txt"), []byte("temp\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(scratch.txt) returned error: %v", err)
	}
	localStateBefore, err := loadAFSLocalState(cfg, "repo")
	if err != nil {
		t.Fatalf("loadAFSLocalState(before restore) returned error: %v", err)
	}

	if err := cmdCheckpoint([]string{"checkpoint", "restore", "repo", "initial"}); err != nil {
		t.Fatalf("cmdCheckpoint(restore) returned error: %v", err)
	}

	workspaceMeta, err := store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta() returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != "initial" {
		t.Fatalf("HeadSavepoint = %q, want %q", workspaceMeta.HeadSavepoint, "initial")
	}

	liveMain, err := client.New(store.rdb, workspaceRedisKey("repo")).Cat(context.Background(), "/main.go")
	if err != nil {
		t.Fatalf("Cat(/main.go) returned error: %v", err)
	}
	if string(liveMain) != "package main\n" {
		t.Fatalf("live main.go after restore = %q, want %q", string(liveMain), "package main\n")
	}

	localState, err := loadAFSLocalState(cfg, "repo")
	if err != nil {
		t.Fatalf("loadAFSLocalState() returned error: %v", err)
	}
	if localState.Dirty != localStateBefore.Dirty {
		t.Fatalf("local Dirty = %v, want %v", localState.Dirty, localStateBefore.Dirty)
	}
	if localState.HeadSavepoint != localStateBefore.HeadSavepoint {
		t.Fatalf("local HeadSavepoint = %q, want %q", localState.HeadSavepoint, localStateBefore.HeadSavepoint)
	}

	mainBytes, err := os.ReadFile(filepath.Join(treePath, "main.go"))
	if err != nil {
		t.Fatalf("ReadFile(main.go) returned error: %v", err)
	}
	if string(mainBytes) != "package dirty\n" {
		t.Fatalf("local main.go after restore = %q, want %q", string(mainBytes), "package dirty\n")
	}
	if _, err := os.Stat(filepath.Join(treePath, "scratch.txt")); err != nil {
		t.Fatalf("expected scratch.txt to remain in the untouched local tree, got err=%v", err)
	}

	archiveDir := afsWorkspaceArchiveDir(cfg, "repo")
	if _, err := os.Stat(archiveDir); !os.IsNotExist(err) {
		t.Fatalf("expected restore to skip local archiving in mounted mode, got err=%v", err)
	}
}

func importAFSWorkspaceForTest(t *testing.T) (config, string, *afsStore, func()) {
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

	if err := cmdImport([]string{"import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdImport() returned error: %v", err)
	}
	if err := cmdWorkspace([]string{"workspace", "run", "repo", "--", "/bin/sh", "-c", "true"}); err != nil {
		t.Fatalf("cmdWorkspace(run) returned error: %v", err)
	}

	loadedCfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	return loadedCfg, afsWorkspaceTreePath(loadedCfg, "repo"), store, closeStore
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	origStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() returned error: %v", err)
	}
	os.Stdout = writePipe
	runErr := fn()
	_ = writePipe.Close()
	os.Stdout = origStdout

	out, readErr := io.ReadAll(readPipe)
	_ = readPipe.Close()
	if readErr != nil {
		t.Fatalf("io.ReadAll() returned error: %v", readErr)
	}
	return string(out), runErr
}
