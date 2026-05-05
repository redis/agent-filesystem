package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseMountOptionsAllowsOptionalDirectory(t *testing.T) {
	t.Helper()

	opts, err := parseMountOptions([]string{"--dry-run", "--yes", "--verbose", "--session", "auth refactor", "notes", "~/notes"})
	if err != nil {
		t.Fatalf("parseMountOptions() returned error: %v", err)
	}
	if opts.workspace != "notes" || opts.directory != "~/notes" {
		t.Fatalf("parseMountOptions() = %#v, want notes + ~/notes", opts)
	}
	if opts.sessionName != "auth refactor" {
		t.Fatalf("sessionName = %q, want auth refactor", opts.sessionName)
	}
	if !opts.dryRun || !opts.yes || !opts.verbose {
		t.Fatalf("parseMountOptions() flags = dryRun:%v yes:%v verbose:%v, want true/true/true", opts.dryRun, opts.yes, opts.verbose)
	}

	opts, err = parseMountOptions([]string{"notes"})
	if err != nil {
		t.Fatalf("parseMountOptions(workspace only) returned error: %v", err)
	}
	if opts.workspace != "notes" || opts.directory != "" {
		t.Fatalf("parseMountOptions(workspace only) = %#v, want notes + empty directory", opts)
	}

	opts, err = parseMountOptions([]string{"--session=bench run", "notes"})
	if err != nil {
		t.Fatalf("parseMountOptions(--session=) returned error: %v", err)
	}
	if opts.sessionName != "bench run" || opts.workspace != "notes" {
		t.Fatalf("parseMountOptions(--session=) = %#v, want session and notes workspace", opts)
	}
}

func TestParseMountOptionsRejectsMissingSessionName(t *testing.T) {
	t.Helper()

	if _, err := parseMountOptions([]string{"--session"}); err == nil {
		t.Fatal("parseMountOptions(--session) returned nil error, want missing session name")
	}
}

func TestPromptMountPathForWorkspaceDefaultsToHomeWorkspace(t *testing.T) {
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
		path, ok, err := promptMountPathForWorkspace("notes")
		if err != nil {
			return err
		}
		if !ok {
			t.Fatal("promptMountPathForWorkspace() cancelled, want default path")
		}
		gotPath = path
		return nil
	})
	if err != nil {
		t.Fatalf("promptMountPathForWorkspace() returned error: %v", err)
	}
	if gotPath != "~/notes" {
		t.Fatalf("path = %q, want ~/notes", gotPath)
	}
	if !strings.Contains(out, "Local folder [~/notes]:") {
		t.Fatalf("output missing default prompt:\n%s", out)
	}
}

func TestMountPromptRowsShowWorkspaceIDAndDatabase(t *testing.T) {
	t.Helper()

	out, err := captureStdout(t, func() error {
		printPlainTable(
			[]string{"#", "Workspace", "Workspace ID", "Database", "Status", "Path"},
			mountPromptRows([]mountPromptChoice{
				{
					Workspace:   "getting-started",
					WorkspaceID: "ws_primary",
					DatabaseID:  "db_primary",
					Database:    "Primary Redis",
					Mounted:     false,
				},
			}),
		)
		return nil
	})
	if err != nil {
		t.Fatalf("printPlainTable() returned error: %v", err)
	}
	stripped := stripAnsi(out)
	for _, want := range []string{"Workspace ID", "Database", "ws_primary", "Primary Redis"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("mount prompt output = %q, want %q", out, want)
		}
	}
}

func TestMountPromptChoicesPreserveDuplicateNamesAcrossDatabases(t *testing.T) {
	t.Helper()

	choices := mountPromptChoices(mountRegistry{Mounts: []mountRecord{
		{
			ID:                   "mnt_primary",
			Workspace:            "getting-started",
			WorkspaceID:          "ws_primary",
			ControlPlaneDatabase: "db_primary",
			LocalPath:            "/tmp/getting-started",
		},
	}}, []workspaceSummary{
		{
			ID:           "ws_primary",
			Name:         "getting-started",
			DatabaseID:   "db_primary",
			DatabaseName: "Primary Redis",
		},
		{
			ID:           "ws_secondary",
			Name:         "getting-started",
			DatabaseID:   "db_secondary",
			DatabaseName: "Secondary Redis",
		},
	})
	if len(choices) != 2 {
		t.Fatalf("len(choices) = %d, want mounted and available duplicate-name workspaces: %#v", len(choices), choices)
	}
	if !choices[0].Mounted || choices[0].WorkspaceID != "ws_primary" || choices[0].Database != "Primary Redis" {
		t.Fatalf("choices[0] = %#v, want mounted primary workspace with database name", choices[0])
	}
	if choices[1].Mounted || choices[1].WorkspaceID != "ws_secondary" || choices[1].Database != "Secondary Redis" {
		t.Fatalf("choices[1] = %#v, want available secondary workspace with database name", choices[1])
	}
}

func TestRecordMountShellDirectoryWritesEnvFile(t *testing.T) {
	t.Helper()

	target := filepath.Join(t.TempDir(), "cd-path")
	t.Setenv(mountShellCDFileEnv, target)

	recordMountShellDirectory("/tmp/demo")

	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(%q) returned error: %v", target, err)
	}
	if got := string(raw); got != "/tmp/demo\n" {
		t.Fatalf("recorded path = %q, want %q", got, "/tmp/demo\n")
	}
}

func TestPrintMountReconcilePlanUsesCompactOperationTable(t *testing.T) {
	t.Helper()

	plan := mountReconcilePlan{
		LocalCount:        1,
		RemoteCount:       2,
		StateLiveCount:    2,
		DownloadCount:     1,
		DeleteRemoteCount: 1,
		ConflictCount:     1,
		Operations: []mountReconcileOperation{
			{Code: "D", Path: "docs", Kind: "dir", Details: "remote changed while unmounted; download to local folder"},
			{Code: "DR", Path: "README.md", Kind: "file", Details: "local deleted while unmounted; remove from workspace"},
			{Code: "C", Path: "notes.txt", Kind: "file", Details: "local and remote both changed while unmounted"},
		},
	}

	out, err := captureStdout(t, func() error {
		printMountReconcilePlan("Mount changes", "repo", "/tmp/repo", plan, true)
		return nil
	})
	if err != nil {
		t.Fatalf("printMountReconcilePlan() returned error: %v", err)
	}
	for _, want := range []string{
		"# OPs: I=Import, D=Download, U=Upload, DL=Delete local, DR=Delete remote,",
		"OP  PATH",
		"D   docs/",
		"DR  README.md",
		"C   notes.txt (local and remote both changed while unmounted)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "DETAILS") {
		t.Fatalf("output should not include DETAILS column:\n%s", out)
	}
}

func TestParseUnmountOptionsDefaultsToPreserveLocal(t *testing.T) {
	t.Helper()

	opts, err := parseUnmountOptions([]string{"notes"}, false)
	if err != nil {
		t.Fatalf("parseUnmountOptions() returned error: %v", err)
	}
	if opts.deleteLocal {
		t.Fatal("deleteLocal = true, want false by default")
	}
	if opts.target != "notes" {
		t.Fatalf("target = %q, want notes", opts.target)
	}
}

func TestParseUnmountOptionsAllowsNoTarget(t *testing.T) {
	t.Helper()

	opts, err := parseUnmountOptions(nil, false)
	if err != nil {
		t.Fatalf("parseUnmountOptions() returned error: %v", err)
	}
	if opts.target != "" {
		t.Fatalf("target = %q, want empty", opts.target)
	}
}

func TestUnmountWorkspaceTargetByWorkspaceName(t *testing.T) {
	t.Helper()

	withTempHome(t)
	root := t.TempDir()
	keepPath := filepath.Join(root, "keep")
	unmountPath := filepath.Join(root, "notes")
	if err := os.MkdirAll(keepPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(keepPath) returned error: %v", err)
	}
	if err := os.MkdirAll(unmountPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(unmountPath) returned error: %v", err)
	}
	reg := mountRegistry{Mounts: []mountRecord{
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
			LocalPath:   unmountPath,
			Mode:        modeSync,
			StartedAt:   time.Now().UTC(),
		},
	}}
	if err := saveMountRegistry(reg); err != nil {
		t.Fatalf("saveMountRegistry() returned error: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return unmountWorkspaceTarget("notes", false)
	})
	if err != nil {
		t.Fatalf("unmountWorkspaceTarget() returned error: %v", err)
	}
	if !strings.Contains(out, "Workspace unmounted") || !strings.Contains(out, "workspace  notes") {
		t.Fatalf("output missing unmount result:\n%s", out)
	}
	loaded, err := loadMountRegistry()
	if err != nil {
		t.Fatalf("loadMountRegistry() returned error: %v", err)
	}
	if _, ok := mountByPath(loaded, unmountPath); ok {
		t.Fatalf("unmounted workspace still registered at %s", unmountPath)
	}
	if _, ok := mountByPath(loaded, keepPath); !ok {
		t.Fatalf("unrelated mount missing at %s", keepPath)
	}
}

func TestUnmountWorkspaceTargetByDirectory(t *testing.T) {
	t.Helper()

	withTempHome(t)
	localPath := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(localPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() returned error: %v", err)
	}
	rec := mountRecord{
		ID:        "att_notes",
		Workspace: "notes",
		LocalPath: localPath,
		Mode:      modeSync,
		StartedAt: time.Now().UTC(),
	}
	if err := saveMountRegistry(mountRegistry{Mounts: []mountRecord{rec}}); err != nil {
		t.Fatalf("saveMountRegistry() returned error: %v", err)
	}

	if err := unmountWorkspaceTarget(localPath, false); err != nil {
		t.Fatalf("unmountWorkspaceTarget(path) returned error: %v", err)
	}
	loaded, err := loadMountRegistry()
	if err != nil {
		t.Fatalf("loadMountRegistry() returned error: %v", err)
	}
	if len(loaded.Mounts) != 0 {
		t.Fatalf("mounts = %#v, want empty", loaded.Mounts)
	}
}

func TestCmdUnmountNoArgsPromptsForSelection(t *testing.T) {
	t.Helper()

	withTempHome(t)
	root := t.TempDir()
	alphaPath := filepath.Join(root, "alpha")
	betaPath := filepath.Join(root, "beta")
	if err := saveMountRegistry(mountRegistry{Mounts: []mountRecord{
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
		t.Fatalf("saveMountRegistry() returned error: %v", err)
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
		return cmdUnmountArgs(nil)
	})
	if err != nil {
		t.Fatalf("cmdUnmountArgs(nil) returned error: %v", err)
	}
	for _, want := range []string{
		"Unmount workspace",
		"#  Workspace  Path",
		"1  alpha",
		"2  beta",
		"Workspace to unmount:",
		"Workspace unmounted",
		"workspace  beta",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	loaded, err := loadMountRegistry()
	if err != nil {
		t.Fatalf("loadMountRegistry() returned error: %v", err)
	}
	if _, ok := mountByPath(loaded, betaPath); ok {
		t.Fatalf("selected mount still registered at %s", betaPath)
	}
	if _, ok := mountByPath(loaded, alphaPath); !ok {
		t.Fatalf("unselected mount missing at %s", alphaPath)
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
