package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestWorkspaceRunUsesWorkspaceTreeAndAutoSaves(t *testing.T) {
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
	if localState.Dirty {
		t.Fatal("expected local state to be clean after auto-save")
	}
	if localState.HeadSavepoint == "initial" {
		t.Fatal("expected workspace run to advance the head savepoint")
	}

	workspaceMeta, err := store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta() returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != localState.HeadSavepoint {
		t.Fatalf("workspace head = %q, want %q", workspaceMeta.HeadSavepoint, localState.HeadSavepoint)
	}
}

func TestWorkspaceRunReadonlyLeavesChangesUnsaved(t *testing.T) {
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
	if !localState.Dirty {
		t.Fatal("expected readonly run to leave local state dirty")
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
	if _, err := os.Stat(filepath.Join(treePath, "generated.txt")); err != nil {
		t.Fatalf("expected generated.txt to remain in working copy: %v", err)
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
	if !strings.Contains(output, "No changes to checkpoint") {
		t.Fatalf("cmdCheckpoint(create) output = %q, want no changes message", output)
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
}

func TestCheckpointCreateDetectsWorkspaceHeadConflict(t *testing.T) {
	t.Helper()

	cfg, treePath, store, closeStore := importAFSWorkspaceForTest(t)
	defer closeStore()

	if err := os.WriteFile(filepath.Join(treePath, "main.go"), []byte("package dirty\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go) returned error: %v", err)
	}

	localState, err := loadAFSLocalState(cfg, "repo")
	if err != nil {
		t.Fatalf("loadAFSLocalState() returned error: %v", err)
	}
	localState.Dirty = true
	if err := saveAFSLocalState(cfg, localState); err != nil {
		t.Fatalf("saveAFSLocalState() returned error: %v", err)
	}

	currentWorkspace, err := store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta() returned error: %v", err)
	}
	currentManifest, err := store.getManifest(context.Background(), "repo", currentWorkspace.HeadSavepoint)
	if err != nil {
		t.Fatalf("getManifest() returned error: %v", err)
	}

	remoteMeta := savepointMeta{
		Version:         afsFormatVersion,
		ID:              "remote-head",
		Name:            "remote-head",
		Workspace:       "repo",
		ParentSavepoint: currentWorkspace.HeadSavepoint,
		CreatedAt:       time.Now().UTC(),
	}
	remoteHash, err := hashManifest(currentManifest)
	if err != nil {
		t.Fatalf("hashManifest() returned error: %v", err)
	}
	remoteMeta.ManifestHash = remoteHash
	remoteMeta.FileCount = 1
	remoteMeta.DirCount = 1
	remoteMeta.TotalBytes = int64(len("package main\n"))
	if err := store.putSavepoint(context.Background(), remoteMeta, currentManifest); err != nil {
		t.Fatalf("putSavepoint(remote) returned error: %v", err)
	}

	currentWorkspace.HeadSavepoint = "remote-head"
	currentWorkspace.UpdatedAt = time.Now().UTC()
	if err := store.putWorkspaceMeta(context.Background(), currentWorkspace); err != nil {
		t.Fatalf("putWorkspaceMeta(remote head) returned error: %v", err)
	}

	err = cmdCheckpoint([]string{"checkpoint", "create", "repo", "should-conflict"})
	if err == nil {
		t.Fatal("expected cmdCheckpoint(create) to fail with a conflict")
	}
	if !strings.Contains(err.Error(), "moved") && !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("cmdCheckpoint(create) error = %q, want conflict message", err.Error())
	}
}

func TestCheckpointRestoreArchivesAndRematerializesWorkspace(t *testing.T) {
	t.Helper()

	cfg, treePath, store, closeStore := importAFSWorkspaceForTest(t)
	defer closeStore()

	if err := os.WriteFile(filepath.Join(treePath, "main.go"), []byte("package saved\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go) returned error: %v", err)
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

	localState, err := loadAFSLocalState(cfg, "repo")
	if err != nil {
		t.Fatalf("loadAFSLocalState() returned error: %v", err)
	}
	if localState.Dirty {
		t.Fatal("expected local state to be clean after restore")
	}
	if localState.HeadSavepoint != "initial" {
		t.Fatalf("local HeadSavepoint = %q, want %q", localState.HeadSavepoint, "initial")
	}

	mainBytes, err := os.ReadFile(filepath.Join(treePath, "main.go"))
	if err != nil {
		t.Fatalf("ReadFile(main.go) returned error: %v", err)
	}
	if string(mainBytes) != "package main\n" {
		t.Fatalf("main.go after restore = %q, want %q", string(mainBytes), "package main\n")
	}
	if _, err := os.Stat(filepath.Join(treePath, "scratch.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected scratch.txt to be removed after restore, got err=%v", err)
	}

	archiveDir := afsWorkspaceArchiveDir(cfg, "repo")
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		t.Fatalf("ReadDir(archive) returned error: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected restore to create an archive entry")
	}

	foundScratch := false
	for _, entry := range entries {
		candidate := filepath.Join(archiveDir, entry.Name(), "scratch.txt")
		if _, err := os.Stat(candidate); err == nil {
			foundScratch = true
			break
		}
	}
	if !foundScratch {
		t.Fatal("expected archived tree to contain scratch.txt")
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
