package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestFSRemoteCommandsListCatAndFind(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = "nfs"
	cfg.NFSBin = "/usr/bin/true"
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, "README.md"), "# Demo\n")
	writeTestFile(t, filepath.Join(sourceDir, "notes", "todo.md"), "- item\n")
	writeTestFile(t, filepath.Join(sourceDir, "notes", "data.txt"), "data\n")

	if err := cmdWorkspace([]string{"workspace", "import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdWorkspace(import) returned error: %v", err)
	}
	_, _, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()

	listOutput, err := captureStdout(t, func() error {
		return cmdFS([]string{"fs", "-w", "repo", "ls", "/"})
	})
	if err != nil {
		t.Fatalf("cmdFS(ls) returned error: %v", err)
	}
	for _, want := range []string{"workspace: repo", "README.md", "notes", "Name", "Type", "Size"} {
		if !strings.Contains(listOutput, want) {
			t.Fatalf("cmdFS(ls) output missing %q:\n%s", want, listOutput)
		}
	}

	catOutput, err := captureStdout(t, func() error {
		return cmdFS([]string{"fs", "-w", "repo", "cat", "notes/todo.md"})
	})
	if err != nil {
		t.Fatalf("cmdFS(cat) returned error: %v", err)
	}
	if catOutput != "- item\n" {
		t.Fatalf("cmdFS(cat) output = %q, want todo content", catOutput)
	}

	findOutput, err := captureStdout(t, func() error {
		return cmdFS([]string{"fs", "-w", "repo", "find", ".", "-name", "*.md", "-print"})
	})
	if err != nil {
		t.Fatalf("cmdFS(find) returned error: %v", err)
	}
	if !strings.Contains(findOutput, "/README.md") || !strings.Contains(findOutput, "/notes/todo.md") {
		t.Fatalf("cmdFS(find) output = %q, want markdown files", findOutput)
	}
	if strings.Contains(findOutput, "data.txt") {
		t.Fatalf("cmdFS(find) output = %q, did not expect data.txt", findOutput)
	}

	grepOutput, err := captureStdout(t, func() error {
		return cmdFS([]string{"fs", "-w", "repo", "grep", "item"})
	})
	if err != nil {
		t.Fatalf("cmdFS(grep) returned error: %v", err)
	}
	if !strings.Contains(grepOutput, "/notes/todo.md:1:- item") {
		t.Fatalf("cmdFS(grep) output = %q, want todo match", grepOutput)
	}
}
