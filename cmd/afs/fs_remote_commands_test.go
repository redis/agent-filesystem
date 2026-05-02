package main

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestParseFSDispatchArgsDisambiguatesWorkspaceAndSubcommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want fsDispatchArgs
	}{
		{
			name: "subcommand without workspace",
			args: []string{"ls"},
			want: fsDispatchArgs{subcommand: "ls", args: []string{}},
		},
		{
			name: "workspace before subcommand",
			args: []string{"repo", "ls", "/docs"},
			want: fsDispatchArgs{workspace: "repo", subcommand: "ls", args: []string{"/docs"}},
		},
		{
			name: "workspace named like subcommand",
			args: []string{"ls", "cat", "README.md"},
			want: fsDispatchArgs{workspace: "ls", subcommand: "cat", args: []string{"README.md"}},
		},
		{
			name: "path named like subcommand remains command arg with inferred workspace",
			args: []string{"ls", "./cat"},
			want: fsDispatchArgs{subcommand: "ls", args: []string{"./cat"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFSDispatchArgs(tt.args)
			if err != nil {
				t.Fatalf("parseFSDispatchArgs() returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseFSDispatchArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

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
		return cmdFS([]string{"fs", "repo", "ls", "/"})
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
		return cmdFS([]string{"fs", "repo", "cat", "notes/todo.md"})
	})
	if err != nil {
		t.Fatalf("cmdFS(cat) returned error: %v", err)
	}
	if catOutput != "- item\n" {
		t.Fatalf("cmdFS(cat) output = %q, want todo content", catOutput)
	}

	findOutput, err := captureStdout(t, func() error {
		return cmdFS([]string{"fs", "repo", "find", ".", "-name", "*.md", "-print"})
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
		return cmdFS([]string{"fs", "repo", "grep", "item"})
	})
	if err != nil {
		t.Fatalf("cmdFS(grep) returned error: %v", err)
	}
	if !strings.Contains(grepOutput, "/notes/todo.md:1:- item") {
		t.Fatalf("cmdFS(grep) output = %q, want todo match", grepOutput)
	}
}
