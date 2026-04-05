package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
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

	loadedCfg, store, closeStore, err := openRAFStore(context.Background())
	if err != nil {
		t.Fatalf("openRAFStore() returned error: %v", err)
	}
	defer closeStore()

	treePath := rafSessionTreePath(loadedCfg, "repo", "main")
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

	loadedCfg, _, _, err := openRAFStore(context.Background())
	if err != nil {
		t.Fatalf("openRAFStore() returned error: %v", err)
	}
	treePath := rafSessionTreePath(loadedCfg, "repo", "main")
	targetFile := filepath.Join(treePath, "main.go")

	if err := os.WriteFile(targetFile, []byte("package updated\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go) returned error: %v", err)
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
}

func TestWorkspaceRunRejectsSessionFlag(t *testing.T) {
	t.Helper()

	err := cmdWorkspace([]string{"workspace", "run", "repo", "--session", "main", "--", "/bin/sh", "-c", "true"})
	if err == nil {
		t.Fatal("cmdWorkspace(run) returned nil error, want session flag rejection")
	}
	if !strings.Contains(err.Error(), "does not accept --session") {
		t.Fatalf("cmdWorkspace(run) error = %q, want session rejection", err)
	}
}
