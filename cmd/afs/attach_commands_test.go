package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseAttachOptionsAllowsOptionalDirectory(t *testing.T) {
	t.Helper()

	opts, err := parseAttachOptions([]string{"--dry-run", "--yes", "--verbose", "notes", "~/notes"})
	if err != nil {
		t.Fatalf("parseAttachOptions() returned error: %v", err)
	}
	if opts.workspace != "notes" || opts.directory != "~/notes" {
		t.Fatalf("parseAttachOptions() = %#v, want notes + ~/notes", opts)
	}
	if !opts.dryRun || !opts.yes || !opts.verbose {
		t.Fatalf("parseAttachOptions() flags = dryRun:%v yes:%v verbose:%v, want true/true/true", opts.dryRun, opts.yes, opts.verbose)
	}

	opts, err = parseAttachOptions([]string{"notes"})
	if err != nil {
		t.Fatalf("parseAttachOptions(workspace only) returned error: %v", err)
	}
	if opts.workspace != "notes" || opts.directory != "" {
		t.Fatalf("parseAttachOptions(workspace only) = %#v, want notes + empty directory", opts)
	}
}

func TestPromptAttachPathForWorkspaceDefaultsToHomeWorkspace(t *testing.T) {
	t.Helper()

	input, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("CreateTemp() returned error: %v", err)
	}
	if _, err := input.WriteString("\n"); err != nil {
		t.Fatalf("WriteString() returned error: %v", err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatalf("Seek() returned error: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = input.Close()
	})

	var gotPath string
	out, err := captureStdout(t, func() error {
		path, ok, err := promptAttachPathForWorkspace("notes")
		if err != nil {
			return err
		}
		if !ok {
			t.Fatal("promptAttachPathForWorkspace() cancelled, want default path")
		}
		gotPath = path
		return nil
	})
	if err != nil {
		t.Fatalf("promptAttachPathForWorkspace() returned error: %v", err)
	}
	if gotPath != "~/notes" {
		t.Fatalf("path = %q, want ~/notes", gotPath)
	}
	if !strings.Contains(out, "Local folder [~/notes]:") {
		t.Fatalf("output missing default prompt:\n%s", out)
	}
}

func TestRecordAttachShellDirectoryWritesEnvFile(t *testing.T) {
	t.Helper()

	target := filepath.Join(t.TempDir(), "cd-path")
	t.Setenv(attachShellCDFileEnv, target)

	recordAttachShellDirectory("/tmp/demo")

	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(%q) returned error: %v", target, err)
	}
	if got := string(raw); got != "/tmp/demo\n" {
		t.Fatalf("recorded path = %q, want %q", got, "/tmp/demo\n")
	}
}

func TestParseDetachOptionsDefaultsToPreserveLocal(t *testing.T) {
	t.Helper()

	opts, err := parseDetachOptions([]string{"notes"}, false)
	if err != nil {
		t.Fatalf("parseDetachOptions() returned error: %v", err)
	}
	if opts.deleteLocal {
		t.Fatal("deleteLocal = true, want false by default")
	}
	if opts.target != "notes" {
		t.Fatalf("target = %q, want notes", opts.target)
	}
}

func TestParseDetachOptionsAllowsNoTarget(t *testing.T) {
	t.Helper()

	opts, err := parseDetachOptions(nil, false)
	if err != nil {
		t.Fatalf("parseDetachOptions() returned error: %v", err)
	}
	if opts.target != "" {
		t.Fatalf("target = %q, want empty", opts.target)
	}
}

func TestDetachWorkspaceTargetByWorkspaceName(t *testing.T) {
	t.Helper()

	withTempHome(t)
	root := t.TempDir()
	keepPath := filepath.Join(root, "keep")
	detachPath := filepath.Join(root, "notes")
	if err := os.MkdirAll(keepPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(keepPath) returned error: %v", err)
	}
	if err := os.MkdirAll(detachPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(detachPath) returned error: %v", err)
	}
	reg := attachmentRegistry{Attachments: []attachmentRecord{
		{
			ID:        "att_keep",
			Workspace: "keep",
			LocalPath: keepPath,
			Mode:      modeSync,
			StartedAt: time.Now().UTC(),
		},
		{
			ID:          "att_notes",
			Workspace:   "notes",
			WorkspaceID: "w_notes",
			LocalPath:   detachPath,
			Mode:        modeSync,
			StartedAt:   time.Now().UTC(),
		},
	}}
	if err := saveAttachmentRegistry(reg); err != nil {
		t.Fatalf("saveAttachmentRegistry() returned error: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return detachWorkspaceTarget("notes", false)
	})
	if err != nil {
		t.Fatalf("detachWorkspaceTarget() returned error: %v", err)
	}
	if !strings.Contains(out, "Workspace detached") || !strings.Contains(out, "workspace  notes") {
		t.Fatalf("output missing detach result:\n%s", out)
	}
	loaded, err := loadAttachmentRegistry()
	if err != nil {
		t.Fatalf("loadAttachmentRegistry() returned error: %v", err)
	}
	if _, ok := attachmentByPath(loaded, detachPath); ok {
		t.Fatalf("detached workspace still registered at %s", detachPath)
	}
	if _, ok := attachmentByPath(loaded, keepPath); !ok {
		t.Fatalf("unrelated attachment missing at %s", keepPath)
	}
}

func TestDetachWorkspaceTargetByDirectory(t *testing.T) {
	t.Helper()

	withTempHome(t)
	localPath := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(localPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() returned error: %v", err)
	}
	rec := attachmentRecord{
		ID:        "att_notes",
		Workspace: "notes",
		LocalPath: localPath,
		Mode:      modeSync,
		StartedAt: time.Now().UTC(),
	}
	if err := saveAttachmentRegistry(attachmentRegistry{Attachments: []attachmentRecord{rec}}); err != nil {
		t.Fatalf("saveAttachmentRegistry() returned error: %v", err)
	}

	if err := detachWorkspaceTarget(localPath, false); err != nil {
		t.Fatalf("detachWorkspaceTarget(path) returned error: %v", err)
	}
	loaded, err := loadAttachmentRegistry()
	if err != nil {
		t.Fatalf("loadAttachmentRegistry() returned error: %v", err)
	}
	if len(loaded.Attachments) != 0 {
		t.Fatalf("attachments = %#v, want empty", loaded.Attachments)
	}
}

func TestCmdDetachNoArgsPromptsForSelection(t *testing.T) {
	t.Helper()

	withTempHome(t)
	root := t.TempDir()
	alphaPath := filepath.Join(root, "alpha")
	betaPath := filepath.Join(root, "beta")
	if err := saveAttachmentRegistry(attachmentRegistry{Attachments: []attachmentRecord{
		{
			ID:        "att_beta",
			Workspace: "beta",
			LocalPath: betaPath,
			Mode:      modeSync,
			StartedAt: time.Now().UTC(),
		},
		{
			ID:        "att_alpha",
			Workspace: "alpha",
			LocalPath: alphaPath,
			Mode:      modeSync,
			StartedAt: time.Now().UTC(),
		},
	}}); err != nil {
		t.Fatalf("saveAttachmentRegistry() returned error: %v", err)
	}

	input, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("CreateTemp() returned error: %v", err)
	}
	if _, err := input.WriteString("2\n"); err != nil {
		t.Fatalf("WriteString() returned error: %v", err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatalf("Seek() returned error: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = input.Close()
	})

	out, err := captureStdout(t, func() error {
		return cmdDetachArgs(nil)
	})
	if err != nil {
		t.Fatalf("cmdDetachArgs(nil) returned error: %v", err)
	}
	for _, want := range []string{
		"Detach workspace",
		"#  Workspace  Path",
		"1  alpha",
		"2  beta",
		"Workspace to detach:",
		"Workspace detached",
		"workspace  beta",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	loaded, err := loadAttachmentRegistry()
	if err != nil {
		t.Fatalf("loadAttachmentRegistry() returned error: %v", err)
	}
	if _, ok := attachmentByPath(loaded, betaPath); ok {
		t.Fatalf("selected attachment still registered at %s", betaPath)
	}
	if _, ok := attachmentByPath(loaded, alphaPath); !ok {
		t.Fatalf("unselected attachment missing at %s", alphaPath)
	}
}

func TestParseStatusOptionsVerbose(t *testing.T) {
	t.Helper()

	opts, err := parseStatusOptions([]string{"--verbose"})
	if err != nil {
		t.Fatalf("parseStatusOptions() returned error: %v", err)
	}
	if !opts.verbose {
		t.Fatal("verbose = false, want true")
	}
}
