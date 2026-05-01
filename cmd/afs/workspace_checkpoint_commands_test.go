package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

func TestWorkspaceCommandsImportCloneForkListAndDelete(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
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
	if !strings.Contains(listOutput, "workspaces on redis://") {
		t.Fatalf("cmdWorkspace(list) output = %q, want database-scoped title", listOutput)
	}
	if !strings.Contains(listOutput, "repo") || !strings.Contains(listOutput, "repo-copy") {
		t.Fatalf("cmdWorkspace(list) output = %q, want both workspace names", listOutput)
	}
	if !strings.Contains(listOutput, "✓") {
		t.Fatalf("cmdWorkspace(list) output = %q, want selected workspace checkmark", listOutput)
	}
	if strings.Contains(listOutput, "<active>") {
		t.Fatalf("cmdWorkspace(list) output = %q, did not expect trailing active marker", listOutput)
	}
	if strings.Contains(listOutput, "checkpoint") {
		t.Fatalf("cmdWorkspace(list) output = %q, did not expect checkpoint-count column", listOutput)
	}

	stripped := stripAnsi(listOutput)
	if !strings.Contains(stripped, "Workspace") || !strings.Contains(stripped, "Database") || !strings.Contains(stripped, "Updated") {
		t.Fatalf("cmdWorkspace(list) output = %q, want table headers", listOutput)
	}
	var repoLine, copyLine string
	var headerLine string
	for _, line := range strings.Split(listOutput, "\n") {
		strippedLine := stripAnsi(line)
		switch {
		case strings.Contains(strippedLine, "Workspace") && strings.Contains(strippedLine, "Database"):
			headerLine = strippedLine
		case strings.Contains(strippedLine, "repo-copy"):
			copyLine = strippedLine
		case strings.Contains(strippedLine, "repo"):
			repoLine = strippedLine
		}
	}
	if headerLine == "" || repoLine == "" || copyLine == "" {
		t.Fatalf("cmdWorkspace(list) output = %q, want header and both workspace rows", listOutput)
	}
	databaseIdx := strings.Index(headerLine, "Database")
	idIdx := strings.Index(headerLine, "ID")
	updatedIdx := strings.Index(headerLine, "Updated")
	if databaseIdx == -1 || idIdx == -1 || updatedIdx == -1 {
		t.Fatalf("workspace list output = %q, want fixed header columns", listOutput)
	}
	if len(repoLine) < updatedIdx || len(copyLine) < updatedIdx {
		t.Fatalf("workspace list output = %q, want rows wide enough for all columns", listOutput)
	}
	if got := strings.TrimSpace(repoLine[databaseIdx:idIdx]); got == "" {
		t.Fatalf("repo database column empty\nheader: %q\nrow: %q", headerLine, repoLine)
	}
	if got := strings.TrimSpace(copyLine[databaseIdx:idIdx]); got == "" {
		t.Fatalf("copy database column empty\nheader: %q\nrow: %q", headerLine, copyLine)
	}
	if got := strings.TrimSpace(repoLine[idIdx:updatedIdx]); !strings.HasPrefix(got, "ws_") {
		t.Fatalf("repo id column = %q, want opaque workspace id\nheader: %q\nrow: %q", got, headerLine, repoLine)
	}
	if got := strings.TrimSpace(copyLine[idIdx:updatedIdx]); !strings.HasPrefix(got, "ws_") {
		t.Fatalf("copy id column = %q, want opaque workspace id\nheader: %q\nrow: %q", got, headerLine, copyLine)
	}

	if err := cmdWorkspace([]string{"workspace", "delete", "--no-confirmation", "repo-copy"}); err != nil {
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

func TestParseWorkspaceDeleteArgsSupportsConfirmationBypass(t *testing.T) {
	t.Helper()

	opts, err := parseWorkspaceDeleteArgs([]string{"--no-confirmation", "repo-copy"})
	if err != nil {
		t.Fatalf("parseWorkspaceDeleteArgs() returned error: %v", err)
	}
	if !opts.noConfirmation {
		t.Fatal("noConfirmation = false, want true")
	}
	if len(opts.names) != 1 || opts.names[0] != "repo-copy" {
		t.Fatalf("names = %#v, want repo-copy", opts.names)
	}
}

func TestConfirmWorkspaceDeleteDefaultsNo(t *testing.T) {
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

	out, err := captureStdout(t, func() error {
		ok, err := confirmWorkspaceDelete([]string{"repo-copy"})
		if err != nil {
			return err
		}
		if ok {
			t.Fatal("confirmWorkspaceDelete() = true, want false for default answer")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("confirmWorkspaceDelete() returned error: %v", err)
	}
	if !strings.Contains(out, "Are you sure you want to delete repo-copy? [y/N]") {
		t.Fatalf("output missing confirmation prompt:\n%s", out)
	}
}

func TestConfirmWorkspaceDeleteAcceptsYes(t *testing.T) {
	t.Helper()

	input, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("CreateTemp() returned error: %v", err)
	}
	if _, err := input.WriteString("y\n"); err != nil {
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

	ok, err := confirmWorkspaceDelete([]string{"repo-copy"})
	if err != nil {
		t.Fatalf("confirmWorkspaceDelete() returned error: %v", err)
	}
	if !ok {
		t.Fatal("confirmWorkspaceDelete() = false, want true for y")
	}
}

func TestWorkspaceListSelfHostedAggregatesAcrossDatabasesWithoutConfiguredDatabase(t *testing.T) {
	t.Helper()

	server, secondaryWorkspace, _, _ := newMultiDatabaseSelfHostedControlPlaneServer(t)

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = server.URL
	cfg.DatabaseID = ""
	cfg.CurrentWorkspace = secondaryWorkspace
	saveTempConfig(t, cfg)

	listOutput, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"workspace", "list"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(list) returned error: %v", err)
	}
	if !strings.Contains(listOutput, "workspaces on "+server.URL+" (auto database)") {
		t.Fatalf("cmdWorkspace(list) output = %q, want workspace-first managed title", listOutput)
	}
	if !strings.Contains(listOutput, "repo") || !strings.Contains(listOutput, secondaryWorkspace) {
		t.Fatalf("cmdWorkspace(list) output = %q, want workspaces from both databases", listOutput)
	}
	if !strings.Contains(listOutput, "✓") {
		t.Fatalf("cmdWorkspace(list) output = %q, want selected workspace marker for %q", listOutput, secondaryWorkspace)
	}
	if !strings.Contains(stripAnsi(listOutput), "Workspace") || !strings.Contains(stripAnsi(listOutput), "ID") {
		t.Fatalf("cmdWorkspace(list) output = %q, want workspace list headers", listOutput)
	}
}

func TestWorkspaceListShowsMountedFolder(t *testing.T) {
	t.Helper()

	homeDir := withTempHome(t)
	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.WorkRoot = filepath.Join(homeDir, ".afs", "workspaces")
	saveTempConfig(t, cfg)

	if err := cmdWorkspace([]string{"ws", "create", "repo"}); err != nil {
		t.Fatalf("cmdWorkspace(create) returned error: %v", err)
	}
	localPath := filepath.Join(homeDir, "repo")
	if err := saveMountRegistry(mountRegistry{Mounts: []mountRecord{
		{
			ID:        "att_repo",
			Workspace: "repo",
			LocalPath: localPath,
			Mode:      modeSync,
			StartedAt: time.Now().UTC(),
		},
	}}); err != nil {
		t.Fatalf("saveMountRegistry() returned error: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "list"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(list) returned error: %v", err)
	}
	stripped := stripAnsi(out)
	for _, want := range []string{"Mounted", "repo", "~/repo"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("cmdWorkspace(list) output = %q, want %q", out, want)
		}
	}
}

func TestWorkspaceListSelfHostedIgnoresStaleConfiguredDatabaseForWorkspaceFirstRoutes(t *testing.T) {
	t.Helper()

	server, secondaryWorkspace, _, secondaryDatabaseID := newMultiDatabaseSelfHostedControlPlaneServer(t)

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = server.URL
	cfg.DatabaseID = secondaryDatabaseID
	cfg.CurrentWorkspace = secondaryWorkspace
	saveTempConfig(t, cfg)

	listOutput, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"workspace", "list"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(list) returned error: %v", err)
	}
	if !strings.Contains(listOutput, "repo") || !strings.Contains(listOutput, secondaryWorkspace) {
		t.Fatalf("cmdWorkspace(list) output = %q, want workspaces from both databases despite stale config database", listOutput)
	}
}

func TestWorkspaceListSelfHostedShowsDatabaseAndIDForDuplicateNames(t *testing.T) {
	t.Helper()

	server, _, _, _ := newDuplicateNameSelfHostedControlPlaneServer(t)

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = server.URL
	saveTempConfig(t, cfg)

	listOutput, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"workspace", "list"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(list) returned error: %v", err)
	}
	if !strings.Contains(listOutput, "primary") || !strings.Contains(listOutput, "secondary") {
		t.Fatalf("cmdWorkspace(list) output = %q, want database names for duplicate workspaces", listOutput)
	}
	if !strings.Contains(listOutput, "ws_") {
		t.Fatalf("cmdWorkspace(list) output = %q, want workspace ids for duplicate workspaces", listOutput)
	}
}

func TestWorkspaceCreateSuggestsMountFirst(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = "nfs"
	cfg.NFSBin = "/usr/bin/true"
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	output, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"workspace", "create", "demo"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(create) returned error: %v", err)
	}
	if !strings.Contains(output, "afs.test up demo <folder>") {
		t.Fatalf("cmdWorkspace(create) output = %q, want mount-first next hint", output)
	}
	if strings.Contains(output, "workspace run demo -- /bin/sh") {
		t.Fatalf("cmdWorkspace(create) output = %q, did not expect workspace run hint", output)
	}
}

func TestWorkspaceCloneRejectsNonEmptyDestination(t *testing.T) {
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
	if err := client.New(store.rdb, rootKey).Echo(context.Background(), "/main.go", []byte("package updated\n")); err != nil {
		t.Fatalf("Echo(/main.go) returned error: %v", err)
	}
	if err := cmdCheckpoint([]string{"checkpoint", "create", "repo", "after-edit", "--description", "Before restoring a known-good file."}); err != nil {
		t.Fatalf("cmdCheckpoint(create) returned error: %v", err)
	}
	checkpointMeta, err := store.getSavepointMeta(context.Background(), "repo", "after-edit")
	if err != nil {
		t.Fatalf("getSavepointMeta(after-edit) returned error: %v", err)
	}
	if checkpointMeta.Description != "Before restoring a known-good file." {
		t.Fatalf("checkpoint description = %q, want %q", checkpointMeta.Description, "Before restoring a known-good file.")
	}
	diffOutput, err := captureStdout(t, func() error {
		return cmdCheckpoint([]string{"checkpoint", "diff", "repo", "initial", "after-edit"})
	})
	if err != nil {
		t.Fatalf("cmdCheckpoint(diff) returned error: %v", err)
	}
	if !strings.Contains(diffOutput, "checkpoint diff") || !strings.Contains(diffOutput, "Update") || !strings.Contains(diffOutput, "/main.go") {
		t.Fatalf("cmdCheckpoint(diff) output = %q, want update for /main.go", diffOutput)
	}
	if !strings.Contains(diffOutput, "-package main") || !strings.Contains(diffOutput, "+package updated") {
		t.Fatalf("cmdCheckpoint(diff) output = %q, want text diff for /main.go", diffOutput)
	}
	diffJSONOutput, err := captureStdout(t, func() error {
		return cmdCheckpoint([]string{"checkpoint", "diff", "repo", "initial", "after-edit", "--json"})
	})
	if err != nil {
		t.Fatalf("cmdCheckpoint(diff --json) returned error: %v", err)
	}
	var diffJSON controlplane.WorkspaceDiffResponse
	if err := json.Unmarshal([]byte(diffJSONOutput), &diffJSON); err != nil {
		t.Fatalf("Unmarshal(diff json) returned error: %v\n%s", err, diffJSONOutput)
	}
	if diffJSON.Summary.Updated != 1 || len(diffJSON.Entries) != 1 || diffJSON.Entries[0].TextDiff == nil {
		t.Fatalf("diff json = %+v, want one updated entry with text diff", diffJSON)
	}
	showOutput, err := captureStdout(t, func() error {
		return cmdCheckpoint([]string{"checkpoint", "show", "repo", "after-edit"})
	})
	if err != nil {
		t.Fatalf("cmdCheckpoint(show) returned error: %v", err)
	}
	if !strings.Contains(showOutput, "checkpoint") || !strings.Contains(showOutput, "after-edit") || !strings.Contains(showOutput, "Before restoring a known-good file.") {
		t.Fatalf("cmdCheckpoint(show) output = %q, want checkpoint detail", showOutput)
	}
	showJSONOutput, err := captureStdout(t, func() error {
		return cmdCheckpoint([]string{"checkpoint", "show", "repo", "after-edit", "--json"})
	})
	if err != nil {
		t.Fatalf("cmdCheckpoint(show --json) returned error: %v", err)
	}
	var showJSON controlplane.CheckpointDetail
	if err := json.Unmarshal([]byte(showJSONOutput), &showJSON); err != nil {
		t.Fatalf("Unmarshal(show json) returned error: %v\n%s", err, showJSONOutput)
	}
	if showJSON.ID != "after-edit" || showJSON.Description != "Before restoring a known-good file." || showJSON.ChangeSummary.Updated != 1 {
		t.Fatalf("checkpoint show json = %+v, want after-edit detail with update summary", showJSON)
	}

	if err := client.New(store.rdb, rootKey).Echo(context.Background(), "/main.go", []byte("package broken\n")); err != nil {
		t.Fatalf("Echo(/main.go broken) returned error: %v", err)
	}
	if err := store.markWorkspaceRootDirty(context.Background(), "repo"); err != nil {
		t.Fatalf("MarkWorkspaceRootDirty() returned error: %v", err)
	}
	restoreOutput, err := captureStdout(t, func() error {
		return cmdCheckpoint([]string{"checkpoint", "restore", "repo", "after-edit"})
	})
	if err != nil {
		t.Fatalf("cmdCheckpoint(restore) returned error: %v", err)
	}
	if !strings.Contains(restoreOutput, "safety checkpoint") || !strings.Contains(restoreOutput, "restore-safety-") {
		t.Fatalf("cmdCheckpoint(restore) output = %q, want safety checkpoint row", restoreOutput)
	}

	liveMain, err := client.New(store.rdb, rootKey).Cat(context.Background(), "/main.go")
	if err != nil {
		t.Fatalf("Cat(/main.go) returned error: %v", err)
	}
	if string(liveMain) != "package updated\n" {
		t.Fatalf("live main.go after restore = %q, want %q", string(liveMain), "package updated\n")
	}

	listOutput, err := captureStdout(t, func() error {
		return cmdCheckpoint([]string{"checkpoint", "list", "repo"})
	})
	if err != nil {
		t.Fatalf("cmdCheckpoint(list) returned error: %v", err)
	}
	if !strings.Contains(listOutput, "checkpoints in workspace: repo") {
		t.Fatalf("cmdCheckpoint(list) output = %q, want workspace title", listOutput)
	}
	if strings.Contains(listOutput, "redis://") {
		t.Fatalf("cmdCheckpoint(list) output = %q, did not expect database in title", listOutput)
	}
	if !strings.Contains(listOutput, "initial") || !strings.Contains(listOutput, "after-edit") {
		t.Fatalf("cmdCheckpoint(list) output = %q, want both checkpoint names", listOutput)
	}
	if !strings.Contains(listOutput, "active") {
		t.Fatalf("cmdCheckpoint(list) output = %q, want active marker", listOutput)
	}
	strippedListOutput := stripAnsi(listOutput)
	if !strings.Contains(strippedListOutput, "Checkpoint") || !strings.Contains(strippedListOutput, "Active") || !strings.Contains(strippedListOutput, "Created") || !strings.Contains(strippedListOutput, "Size") {
		t.Fatalf("cmdCheckpoint(list) output = %q, want checkpoint table headers", listOutput)
	}
	var checkpointHeaderLine, initialLine, activeLine string
	for _, line := range strings.Split(strippedListOutput, "\n") {
		switch {
		case strings.Contains(line, "Checkpoint") && strings.Contains(line, "Active") && strings.Contains(line, "Created"):
			checkpointHeaderLine = line
		case strings.Contains(line, "initial"):
			initialLine = line
		case strings.Contains(line, "after-edit"):
			activeLine = line
		}
	}
	if checkpointHeaderLine == "" || initialLine == "" || activeLine == "" {
		t.Fatalf("cmdCheckpoint(list) output = %q, want header and checkpoint rows", listOutput)
	}
	activeIdx := strings.Index(checkpointHeaderLine, "Active")
	createdIdx := strings.Index(checkpointHeaderLine, "Created")
	sizeIdx := strings.Index(checkpointHeaderLine, "Size")
	if activeIdx == -1 || createdIdx == -1 || sizeIdx == -1 || activeIdx >= createdIdx || createdIdx >= sizeIdx {
		t.Fatalf("checkpoint list header = %q, want Active, Created, Size columns", checkpointHeaderLine)
	}
	if len(initialLine) < createdIdx || len(activeLine) < sizeIdx {
		t.Fatalf("cmdCheckpoint(list) output = %q, want rows wide enough for active and date columns", listOutput)
	}
	if got := strings.TrimSpace(activeLine[activeIdx:createdIdx]); got != "active" {
		t.Fatalf("active checkpoint column = %q, want active\nheader: %q\nrow: %q", got, checkpointHeaderLine, activeLine)
	}
	if got := strings.TrimSpace(initialLine[activeIdx:createdIdx]); got != "" {
		t.Fatalf("inactive checkpoint column = %q, want empty\nheader: %q\nrow: %q", got, checkpointHeaderLine, initialLine)
	}
	if got := activeLine[createdIdx:sizeIdx]; strings.Contains(got, "active") {
		t.Fatalf("created column = %q, did not expect active marker\nheader: %q\nrow: %q", got, checkpointHeaderLine, activeLine)
	}
	if strings.Contains(listOutput, "T") || strings.Contains(listOutput, "Z") {
		t.Fatalf("cmdCheckpoint(list) output = %q, want human-readable timestamps instead of raw RFC3339", listOutput)
	}
}

func TestCheckpointCreateUsesLiveWorkspaceWhenNoLocalTreeExists(t *testing.T) {
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
	if err := client.New(store.rdb, rootKey).Echo(context.Background(), "/newfile.txt", []byte("from mounted workspace\n")); err != nil {
		t.Fatalf("Echo(/newfile.txt) returned error: %v", err)
	}

	st := state{
		StartedAt:            time.Now().UTC(),
		RedisAddr:            cfg.RedisAddr,
		RedisDB:              cfg.RedisDB,
		CurrentWorkspace:     "repo",
		CurrentWorkspaceID:   rootKey,
		MountedHeadSavepoint: "initial",
		MountBackend:         mountBackendNFS,
		LocalPath:            filepath.Join(t.TempDir(), "mount"),
		RedisKey:             rootKey,
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

func TestCheckpointCommandsPromptForWorkspaceWhenOmitted(t *testing.T) {
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
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = "nfs"
	cfg.NFSBin = "/usr/bin/true"
	cfg.WorkRoot = t.TempDir()
	cfg.CurrentWorkspace = "ignored-default"
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

	withStdin := func(lines string) func() {
		input, err := os.CreateTemp(t.TempDir(), "stdin")
		if err != nil {
			t.Fatalf("CreateTemp() returned error: %v", err)
		}
		if _, err := input.WriteString(lines); err != nil {
			t.Fatalf("WriteString() returned error: %v", err)
		}
		if _, err := input.Seek(0, 0); err != nil {
			t.Fatalf("Seek() returned error: %v", err)
		}
		origStdin := os.Stdin
		os.Stdin = input
		return func() {
			os.Stdin = origStdin
			_ = input.Close()
		}
	}

	restoreStdin := withStdin("1\n")
	if err := cmdCheckpoint([]string{"checkpoint", "create", "current-save"}); err != nil {
		restoreStdin()
		t.Fatalf("cmdCheckpoint(create current-save) returned error: %v", err)
	}
	restoreStdin()

	listOutput, err := captureStdout(t, func() error {
		restoreStdin := withStdin("1\n")
		defer restoreStdin()
		return cmdCheckpoint([]string{"checkpoint", "list"})
	})
	if err != nil {
		t.Fatalf("cmdCheckpoint(list) returned error: %v", err)
	}
	if !strings.Contains(listOutput, "current-save") {
		t.Fatalf("cmdCheckpoint(list) output = %q, want current-save", listOutput)
	}

	restoreStdin = withStdin("1\n")
	if err := cmdCheckpoint([]string{"checkpoint", "restore", "initial"}); err != nil {
		restoreStdin()
		t.Fatalf("cmdCheckpoint(restore initial) returned error: %v", err)
	}
	restoreStdin()

	workspaceMeta, err := store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta(repo) returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != "initial" {
		t.Fatalf("HeadSavepoint = %q, want %q", workspaceMeta.HeadSavepoint, "initial")
	}
}

func TestCheckpointCreateSavesEvenWhenUnchanged(t *testing.T) {
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

	if err := cmdWorkspace([]string{"workspace", "import", "repo", sourceDir}); err != nil {
		t.Fatalf("cmdWorkspace(import) returned error: %v", err)
	}

	_, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()

	out, err := captureStdout(t, func() error {
		return cmdCheckpoint([]string{"checkpoint", "create", "repo", "unchanged-snapshot"})
	})
	if err != nil {
		t.Fatalf("cmdCheckpoint(create unchanged) returned error: %v", err)
	}
	if strings.Contains(out, "checkpoint unchanged") || strings.Contains(out, "no changes") {
		t.Fatalf("cmdCheckpoint(create unchanged) output = %q, want checkpoint created", out)
	}
	if !strings.Contains(out, "checkpoint created") {
		t.Fatalf("cmdCheckpoint(create unchanged) output = %q, want created message", out)
	}

	meta, err := store.getSavepointMeta(context.Background(), "repo", "unchanged-snapshot")
	if err != nil {
		t.Fatalf("getSavepointMeta(unchanged-snapshot) returned error: %v", err)
	}
	if meta.ParentSavepoint != "initial" {
		t.Fatalf("ParentSavepoint = %q, want initial", meta.ParentSavepoint)
	}
}

func TestCheckpointRestoreIgnoresManagedSyncDaemonHandles(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
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

	localRoot := filepath.Join(t.TempDir(), "repo")
	if err := saveState(state{
		StartedAt:        time.Now().UTC(),
		RedisAddr:        cfg.RedisAddr,
		RedisDB:          cfg.RedisDB,
		CurrentWorkspace: "repo",
		Mode:             modeSync,
		SyncPID:          os.Getpid(),
		LocalPath:        localRoot,
	}); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	orig := checkOpenHandlesUnderPath
	checkOpenHandlesUnderPath = func(root string) ([]openFileHandle, error) {
		if root != localRoot {
			t.Fatalf("checkOpenHandlesUnderPath(%q), want %q", root, localRoot)
		}
		return []openFileHandle{
			{PID: os.Getpid(), Command: "afs", Path: localRoot},
			{PID: os.Getpid(), Command: "afs", Path: filepath.Join(localRoot, "README.md")},
		}, nil
	}
	t.Cleanup(func() { checkOpenHandlesUnderPath = orig })

	origTerminate := checkpointRestoreTerminatePID
	origStart := checkpointRestoreStartSyncServices
	stopped := false
	restarted := false
	checkpointRestoreTerminatePID = func(pid int, timeout time.Duration) error {
		if pid != os.Getpid() {
			t.Fatalf("checkpointRestoreTerminatePID(%d), want %d", pid, os.Getpid())
		}
		stopped = true
		return nil
	}
	checkpointRestoreStartSyncServices = func(cfg config, foreground bool) error {
		if cfg.LocalPath != localRoot {
			t.Fatalf("restart LocalPath = %q, want %q", cfg.LocalPath, localRoot)
		}
		if foreground {
			t.Fatal("restore restart should run sync in background")
		}
		restarted = true
		return nil
	}
	t.Cleanup(func() {
		checkpointRestoreTerminatePID = origTerminate
		checkpointRestoreStartSyncServices = origStart
	})

	err := cmdCheckpoint([]string{"checkpoint", "restore", "repo", "initial"})
	if err != nil {
		t.Fatalf("cmdCheckpoint(restore) returned error: %v", err)
	}
	if !stopped || !restarted {
		t.Fatalf("restore sync lifecycle stopped=%v restarted=%v, want both true", stopped, restarted)
	}
}

func TestCheckpointRestoreRejectsActiveSyncClientWithOpenHandles(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
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

	localRoot := t.TempDir()
	if err := saveState(state{
		StartedAt:        time.Now().UTC(),
		RedisAddr:        cfg.RedisAddr,
		RedisDB:          cfg.RedisDB,
		CurrentWorkspace: "repo",
		Mode:             modeSync,
		SyncPID:          os.Getpid(),
		LocalPath:        localRoot,
	}); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	orig := checkOpenHandlesUnderPath
	checkOpenHandlesUnderPath = func(root string) ([]openFileHandle, error) {
		if root != localRoot {
			t.Fatalf("checkOpenHandlesUnderPath(%q), want %q", root, localRoot)
		}
		return []openFileHandle{
			{PID: os.Getpid(), Command: "afs", Path: localRoot},
			{PID: 123, Command: "editor", Path: filepath.Join(localRoot, "main.go")},
		}, nil
	}
	t.Cleanup(func() { checkOpenHandlesUnderPath = orig })

	err := cmdCheckpoint([]string{"checkpoint", "restore", "repo", "initial"})
	if err == nil {
		t.Fatal("cmdCheckpoint(restore) returned nil error, want open handle rejection")
	}
	if !strings.Contains(err.Error(), "files are open") || !strings.Contains(err.Error(), "editor pid 123") {
		t.Fatalf("cmdCheckpoint(restore) error = %q, want open handle guidance", err)
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
		RedisAddr:            cfg.RedisAddr,
		RedisDB:              cfg.RedisDB,
		CurrentWorkspace:     "repo",
		CurrentWorkspaceID:   rootKey,
		MountedHeadSavepoint: "initial",
		MountBackend:         mountBackendNFS,
		LocalPath:            filepath.Join(t.TempDir(), "mount"),
		RedisKey:             rootKey,
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

func TestCheckpointCreatePrefersActiveSyncWorkspaceOverStaleConfigInSelfHostedMode(t *testing.T) {
	t.Helper()

	withTempHome(t)

	server, secondaryWorkspace, secondaryRedisAddr, secondaryDatabaseID := newMultiDatabaseSelfHostedControlPlaneServer(t)

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = server.URL
	cfg.DatabaseID = ""
	cfg.CurrentWorkspace = "repo"
	saveTempConfig(t, cfg)

	rdb := redis.NewClient(&redis.Options{Addr: secondaryRedisAddr})
	t.Cleanup(func() {
		_ = rdb.Close()
	})
	store := newAFSStore(rdb)

	rootKey, _, _, err := seedWorkspaceMountKey(context.Background(), store, secondaryWorkspace)
	if err != nil {
		t.Fatalf("seedWorkspaceMountKey() returned error: %v", err)
	}
	if err := client.New(store.rdb, rootKey).Echo(context.Background(), "/sync-only.txt", []byte("from active sync state\n")); err != nil {
		t.Fatalf("Echo(/sync-only.txt) returned error: %v", err)
	}

	if err := saveState(state{
		StartedAt:            time.Now().UTC(),
		ProductMode:          productModeSelfHosted,
		ControlPlaneURL:      server.URL,
		ControlPlaneDatabase: secondaryDatabaseID,
		CurrentWorkspace:     secondaryWorkspace,
		Mode:                 modeSync,
		SyncPID:              os.Getpid(),
		RedisAddr:            secondaryRedisAddr,
		RedisDB:              0,
		RedisKey:             workspaceRedisKey(secondaryWorkspace),
		LocalPath:            filepath.Join(t.TempDir(), secondaryWorkspace),
	}); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	if err := cmdCheckpoint([]string{"checkpoint", "create", "sync-state-save"}); err != nil {
		t.Fatalf("cmdCheckpoint(create sync-state-save) returned error: %v", err)
	}

	secondaryMeta, err := store.getWorkspaceMeta(context.Background(), secondaryWorkspace)
	if err != nil {
		t.Fatalf("getWorkspaceMeta(%s) returned error: %v", secondaryWorkspace, err)
	}
	if secondaryMeta.HeadSavepoint != "sync-state-save" {
		t.Fatalf("secondary HeadSavepoint = %q, want %q", secondaryMeta.HeadSavepoint, "sync-state-save")
	}
}

func TestWorkspaceCloneAndForkUseCurrentWorkspaceWhenOmitted(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
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

func TestWorkspaceRunCommandIsRemoved(t *testing.T) {
	t.Helper()

	err := cmdWorkspace([]string{"workspace", "run", "repo", "--session", "main", "--", "/bin/sh", "-c", "true"})
	if err == nil {
		t.Fatal("cmdWorkspace(run) returned nil error, want removed-command error")
	}
	if !strings.Contains(err.Error(), `unknown workspace subcommand "run"`) {
		t.Fatalf("cmdWorkspace(run) error = %q, want removed run subcommand error", err)
	}
}

func TestWorkspaceImportRejectsRemovedCloneAtSourceFlag(t *testing.T) {
	t.Helper()

	err := cmdWorkspace([]string{"workspace", "import", "--clone-at-source", "repo", "/tmp/repo"})
	if err == nil {
		t.Fatal("cmdWorkspace(import) returned nil error, want removed flag rejection")
	}
	if !strings.Contains(err.Error(), `unknown flag "--clone-at-source"`) {
		t.Fatalf("cmdWorkspace(import) error = %q, want unknown flag", err)
	}
}

func TestParseAFSArgsSupportsMountAtSource(t *testing.T) {
	t.Helper()

	parsed, err := parseAFSArgs([]string{"--mount-at-source", "--force", "repo", "/tmp/repo"}, true, false)
	if err != nil {
		t.Fatalf("parseAFSArgs() returned error: %v", err)
	}
	if !parsed.mountAtSource {
		t.Fatal("mountAtSource = false, want true")
	}
	if !parsed.force {
		t.Fatal("force = false, want true")
	}
	if len(parsed.positionals) != 2 || parsed.positionals[0] != "repo" || parsed.positionals[1] != "/tmp/repo" {
		t.Fatalf("positionals = %#v, want repo and /tmp/repo", parsed.positionals)
	}
}
