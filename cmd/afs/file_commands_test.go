package main

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/controlplane"
	mountclient "github.com/redis/agent-filesystem/mount/client"
)

func extractBoxValue(output, label string) string {
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, label) {
			continue
		}
		index := strings.Index(line, label)
		if index < 0 {
			continue
		}
		return strings.TrimSpace(line[index+len(label):])
	}
	return ""
}

func extractHistoryCursor(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "next cursor:") {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(line, "next cursor:"))
	}
	return ""
}

func TestParseFileHistoryArgs(t *testing.T) {
	parsed, err := parseFileHistoryArgs([]string{"repo", "/notes/app.txt", "--order=asc", "--limit=5", "--cursor", "cursor_123"})
	if err != nil {
		t.Fatalf("parseFileHistoryArgs() returned error: %v", err)
	}
	if parsed.workspace != "repo" {
		t.Fatalf("workspace = %q, want repo", parsed.workspace)
	}
	if parsed.path != "/notes/app.txt" {
		t.Fatalf("path = %q, want /notes/app.txt", parsed.path)
	}
	if parsed.newestFirst {
		t.Fatal("newestFirst = true, want false for asc")
	}
	if parsed.limit != 5 {
		t.Fatalf("limit = %d, want 5", parsed.limit)
	}
	if parsed.cursor != "cursor_123" {
		t.Fatalf("cursor = %q, want cursor_123", parsed.cursor)
	}
}

func TestParseFileShowArgs(t *testing.T) {
	parsed, err := parseFileShowArgs([]string{"repo", "/notes/app.txt", "--version", "fv_123"})
	if err != nil {
		t.Fatalf("parseFileShowArgs() returned error: %v", err)
	}
	if parsed.workspace != "repo" || parsed.path != "/notes/app.txt" || parsed.versionID != "fv_123" {
		t.Fatalf("parsed = %#v, want workspace/path/version populated", parsed)
	}
}

func TestParseFileShowArgsByOrdinal(t *testing.T) {
	parsed, err := parseFileShowArgs([]string{"repo", "/notes/app.txt", "--file-id", "file_123", "--ordinal", "7"})
	if err != nil {
		t.Fatalf("parseFileShowArgs() returned error: %v", err)
	}
	if parsed.fileID != "file_123" || parsed.ordinal == nil || *parsed.ordinal != 7 {
		t.Fatalf("parsed = %#v, want file_id/ordinal populated", parsed)
	}
}

func TestParseFileRestoreArgs(t *testing.T) {
	parsed, err := parseFileRestoreArgs([]string{"repo", "/notes/app.txt", "--version", "fv_123"})
	if err != nil {
		t.Fatalf("parseFileRestoreArgs() returned error: %v", err)
	}
	if parsed.workspace != "repo" || parsed.path != "/notes/app.txt" || parsed.versionID != "fv_123" {
		t.Fatalf("parsed = %#v, want workspace/path/version populated", parsed)
	}
}

func TestParseFileDiffArgs(t *testing.T) {
	parsed, err := parseFileDiffArgs([]string{"repo", "/notes/app.txt", "--from-version", "fv_123"})
	if err != nil {
		t.Fatalf("parseFileDiffArgs() returned error: %v", err)
	}
	if parsed.workspace != "repo" || parsed.path != "/notes/app.txt" {
		t.Fatalf("parsed = %#v, want workspace/path populated", parsed)
	}
	if parsed.from.versionID != "fv_123" {
		t.Fatalf("parsed.from.versionID = %q, want fv_123", parsed.from.versionID)
	}
	if parsed.to.ref != "head" {
		t.Fatalf("parsed.to.ref = %q, want head default", parsed.to.ref)
	}
}

func TestFileDiffWorkspaceRefUsesNameForLiveRefsInLocalMode(t *testing.T) {
	parsed := fileDiffArgs{
		from: fileDiffOperandArgs{versionID: "ver_123"},
		to:   fileDiffOperandArgs{ref: "head"},
	}
	selection := workspaceSelection{ID: "ws_repo", Name: "repo"}
	cfg := defaultConfig()
	cfg.ProductMode = productModeLocal
	if got := fileDiffWorkspaceRef(cfg, selection, parsed); got != "repo" {
		t.Fatalf("fileDiffWorkspaceRef(local live ref) = %q, want %q", got, "repo")
	}

	cfg.ProductMode = productModeSelfHosted
	if got := fileDiffWorkspaceRef(cfg, selection, parsed); got != "ws_repo" {
		t.Fatalf("fileDiffWorkspaceRef(managed live ref) = %q, want %q", got, "ws_repo")
	}

	parsed.to = fileDiffOperandArgs{versionID: "ver_456"}
	cfg.ProductMode = productModeLocal
	if got := fileDiffWorkspaceRef(cfg, selection, parsed); got != "ws_repo" {
		t.Fatalf("fileDiffWorkspaceRef(version diff) = %q, want %q", got, "ws_repo")
	}
}

func TestParseFileUndeleteArgs(t *testing.T) {
	parsed, err := parseFileUndeleteArgs([]string{"repo", "/notes/app.txt", "--version", "fv_123"})
	if err != nil {
		t.Fatalf("parseFileUndeleteArgs() returned error: %v", err)
	}
	if parsed.workspace != "repo" || parsed.path != "/notes/app.txt" || parsed.versionID != "fv_123" {
		t.Fatalf("parsed = %#v, want workspace/path/version populated", parsed)
	}
}

func TestFileUsageDoesNotExposeVersionDeletion(t *testing.T) {
	usage := fileUsageText("afs")
	if strings.Contains(usage, "delete-version") || strings.Contains(usage, "delete version") {
		t.Fatalf("fileUsageText() unexpectedly exposes historical version deletion: %q", usage)
	}
}

func TestCmdFileHistoryAndShow(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNone
	cfg.CurrentWorkspace = "repo"
	saveTempConfig(t, cfg)

	loadedCfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()

	if err := createEmptyWorkspace(context.Background(), loadedCfg, store, "repo"); err != nil {
		t.Fatalf("createEmptyWorkspace() returned error: %v", err)
	}
	service := controlPlaneServiceFromStore(loadedCfg, store)
	cpStore := controlPlaneStoreFromAFS(store)
	if _, err := service.UpdateWorkspaceVersioningPolicy(context.Background(), "repo", controlplane.WorkspaceVersioningPolicy{
		Mode: controlplane.WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("UpdateWorkspaceVersioningPolicy() returned error: %v", err)
	}

	content := []byte("cli file history\n")
	version, err := cpStore.RecordFileVersionMutation(context.Background(), "repo", controlplane.VersionedFileSnapshot{}, controlplane.VersionedFileSnapshot{
		Path:        "/notes/cli-history.txt",
		Exists:      true,
		Kind:        "file",
		Mode:        0o644,
		Content:     content,
		BlobID:      sha256Hex(content),
		ContentHash: sha256Hex(content),
		SizeBytes:   int64(len(content)),
	}, controlplane.FileVersionMutationMetadata{Source: controlplane.ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation() returned error: %v", err)
	}
	version2, err := cpStore.RecordFileVersionMutation(context.Background(), "repo", controlplane.VersionedFileSnapshot{
		Path:        "/notes/cli-history.txt",
		Exists:      true,
		Kind:        "file",
		Mode:        0o644,
		Content:     content,
		BlobID:      sha256Hex(content),
		ContentHash: sha256Hex(content),
		SizeBytes:   int64(len(content)),
	}, controlplane.VersionedFileSnapshot{
		Path:        "/notes/cli-history.txt",
		Exists:      true,
		Kind:        "file",
		Mode:        0o644,
		Content:     []byte("cli file history v2\n"),
		BlobID:      sha256Hex([]byte("cli file history v2\n")),
		ContentHash: sha256Hex([]byte("cli file history v2\n")),
		SizeBytes:   int64(len("cli file history v2\n")),
	}, controlplane.FileVersionMutationMetadata{Source: controlplane.ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(second version) returned error: %v", err)
	}

	historyOutput, err := captureStdout(t, func() error {
		return cmdFile([]string{"file", "history", "repo", "/notes/cli-history.txt", "--order", "asc", "--limit", "1"})
	})
	if err != nil {
		t.Fatalf("cmdFile(history) returned error: %v", err)
	}
	if !strings.Contains(historyOutput, version.VersionID) {
		t.Fatalf("cmdFile(history) output = %q, want version id %q", historyOutput, version.VersionID)
	}
	if !strings.Contains(historyOutput, "ordinal 1") {
		t.Fatalf("cmdFile(history) output = %q, want ordinal details", historyOutput)
	}
	if !strings.Contains(historyOutput, "next cursor") {
		t.Fatalf("cmdFile(history) output = %q, want pagination cursor", historyOutput)
	}

	cursorLine := extractHistoryCursor(historyOutput)
	if cursorLine == "" {
		t.Fatalf("extractHistoryCursor() = empty from output %q", historyOutput)
	}

	historyPage2Output, err := captureStdout(t, func() error {
		return cmdFile([]string{"file", "history", "repo", "/notes/cli-history.txt", "--order", "asc", "--limit", "1", "--cursor", cursorLine})
	})
	if err != nil {
		t.Fatalf("cmdFile(history page 2) returned error: %v", err)
	}
	if !strings.Contains(historyPage2Output, version2.VersionID) {
		t.Fatalf("cmdFile(history page 2) output = %q, want version id %q", historyPage2Output, version2.VersionID)
	}

	showOutput, err := captureStdout(t, func() error {
		return cmdFile([]string{"file", "show", "repo", "/notes/cli-history.txt", "--version", version.VersionID})
	})
	if err != nil {
		t.Fatalf("cmdFile(show) returned error: %v", err)
	}
	if showOutput != string(content) {
		t.Fatalf("cmdFile(show) output = %q, want %q", showOutput, string(content))
	}

	showByOrdinalOutput, err := captureStdout(t, func() error {
		return cmdFile([]string{"file", "show", "repo", "/notes/cli-history.txt", "--file-id", version.FileID, "--ordinal", "1"})
	})
	if err != nil {
		t.Fatalf("cmdFile(show by ordinal) returned error: %v", err)
	}
	if showByOrdinalOutput != string(content) {
		t.Fatalf("cmdFile(show by ordinal) output = %q, want %q", showByOrdinalOutput, string(content))
	}

	fsKey, _, _, err := controlplane.EnsureWorkspaceRoot(context.Background(), cpStore, "repo")
	if err != nil {
		t.Fatalf("EnsureWorkspaceRoot() returned error: %v", err)
	}
	fsClient := mountclient.New(store.rdb, fsKey)
	if err := fsClient.Mkdir(context.Background(), "/notes"); err != nil {
		t.Fatalf("Mkdir() returned error: %v", err)
	}
	if err := fsClient.EchoCreate(context.Background(), "/notes/cli-history.txt", []byte("cli file history updated\n"), 0o644); err != nil {
		t.Fatalf("EchoCreate() returned error: %v", err)
	}

	diffOutput, err := captureStdout(t, func() error {
		return cmdFile([]string{"file", "diff", "repo", "/notes/cli-history.txt", "--from-version", version.VersionID, "--to-ref", "working-copy"})
	})
	if err != nil {
		t.Fatalf("cmdFile(diff) returned error: %v", err)
	}
	if !strings.Contains(diffOutput, "cli file history\n") || !strings.Contains(diffOutput, "cli file history updated\n") {
		t.Fatalf("cmdFile(diff) output = %q, want historical and working-copy content", diffOutput)
	}

	liveOnlyVersion, err := cpStore.RecordFileVersionMutation(context.Background(), "repo", controlplane.VersionedFileSnapshot{Path: "/notes/live-only.txt"}, controlplane.VersionedFileSnapshot{
		Path:    "/notes/live-only.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("historical live only\n"),
	}, controlplane.FileVersionMutationMetadata{Source: controlplane.ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(live-only) returned error: %v", err)
	}
	if err := fsClient.EchoCreate(context.Background(), "/notes/live-only.txt", []byte("current live only\n"), 0o644); err != nil {
		t.Fatalf("EchoCreate(live-only) returned error: %v", err)
	}

	headFallbackDiff, err := captureStdout(t, func() error {
		return cmdFile([]string{"file", "diff", "repo", "/notes/live-only.txt", "--from-version", liveOnlyVersion.VersionID, "--to-ref", "head"})
	})
	if err != nil {
		t.Fatalf("cmdFile(diff head fallback) returned error: %v", err)
	}
	if !strings.Contains(headFallbackDiff, "historical live only\n") || !strings.Contains(headFallbackDiff, "current live only\n") {
		t.Fatalf("cmdFile(diff head fallback) output = %q, want historical and live content", headFallbackDiff)
	}

	restoreOutput, err := captureStdout(t, func() error {
		return cmdFile([]string{"file", "restore", "repo", "/notes/cli-history.txt", "--version", version.VersionID})
	})
	if err != nil {
		t.Fatalf("cmdFile(restore) returned error: %v", err)
	}
	if !strings.Contains(restoreOutput, "file restored from history") {
		t.Fatalf("cmdFile(restore) output = %q, want restore confirmation", restoreOutput)
	}

	deletedVersion, err := cpStore.RecordFileVersionMutation(context.Background(), "repo", controlplane.VersionedFileSnapshot{Path: "/notes/deleted-cli.txt"}, controlplane.VersionedFileSnapshot{
		Path:        "/notes/deleted-cli.txt",
		Exists:      true,
		Kind:        "file",
		Mode:        0o644,
		Content:     []byte("deleted cli\n"),
		BlobID:      sha256Hex([]byte("deleted cli\n")),
		ContentHash: sha256Hex([]byte("deleted cli\n")),
		SizeBytes:   int64(len("deleted cli\n")),
	}, controlplane.FileVersionMutationMetadata{Source: controlplane.ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(undelete create) returned error: %v", err)
	}
	if _, err := cpStore.RecordFileVersionMutation(context.Background(), "repo", controlplane.VersionedFileSnapshot{
		Path:        "/notes/deleted-cli.txt",
		Exists:      true,
		Kind:        "file",
		Mode:        0o644,
		Content:     []byte("deleted cli\n"),
		BlobID:      sha256Hex([]byte("deleted cli\n")),
		ContentHash: sha256Hex([]byte("deleted cli\n")),
		SizeBytes:   int64(len("deleted cli\n")),
	}, controlplane.VersionedFileSnapshot{
		Path: "/notes/deleted-cli.txt",
	}, controlplane.FileVersionMutationMetadata{Source: controlplane.ChangeSourceMCP}); err != nil {
		t.Fatalf("RecordFileVersionMutation(undelete delete) returned error: %v", err)
	}

	undeleteOutput, err := captureStdout(t, func() error {
		return cmdFile([]string{"file", "undelete", "repo", "/notes/deleted-cli.txt"})
	})
	if err != nil {
		t.Fatalf("cmdFile(undelete) returned error: %v", err)
	}
	if !strings.Contains(undeleteOutput, "file undeleted from history") || !strings.Contains(undeleteOutput, deletedVersion.VersionID) {
		t.Fatalf("cmdFile(undelete) output = %q, want undelete confirmation with source version", undeleteOutput)
	}
}
