package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/controlplane"
)

func TestAFSMCPServerInitializeAndToolsList(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	var input bytes.Buffer
	input.WriteString(frameForTest(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	input.WriteString(frameForTest(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))

	var output bytes.Buffer
	if err := server.serve(context.Background(), &input, &output); err != nil {
		t.Fatalf("serve() returned error: %v", err)
	}

	reader := bufio.NewReader(&output)
	firstPayload, err := readMCPFrame(reader)
	if err != nil {
		t.Fatalf("readMCPFrame(first) returned error: %v", err)
	}
	secondPayload, err := readMCPFrame(reader)
	if err != nil {
		t.Fatalf("readMCPFrame(second) returned error: %v", err)
	}

	var first map[string]any
	if err := json.Unmarshal(firstPayload, &first); err != nil {
		t.Fatalf("Unmarshal(first) returned error: %v", err)
	}
	result, ok := first["result"].(map[string]any)
	if !ok {
		t.Fatalf("initialize result missing: %#v", first)
	}
	if got := result["protocolVersion"]; got != afsMCPProtocolVersion {
		t.Fatalf("protocolVersion = %#v, want %q", got, afsMCPProtocolVersion)
	}

	var second map[string]any
	if err := json.Unmarshal(secondPayload, &second); err != nil {
		t.Fatalf("Unmarshal(second) returned error: %v", err)
	}
	secondResult, ok := second["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list result missing: %#v", second)
	}
	tools, ok := secondResult["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list tools missing: %#v", secondResult)
	}
	if len(tools) == 0 {
		t.Fatal("tools/list returned no tools")
	}
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		if name, _ := tool["name"].(string); name == "file_delete_version" {
			t.Fatalf("tools/list unexpectedly exposes %q", name)
		}
	}
}

func TestAFSMCPFileWriteLeavesWorkspaceDirtyAndReadReturnsContent(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	writeResult := server.callTool(context.Background(), "file_write", map[string]any{
		"path":    "/notes/todo.md",
		"content": "# TODO\n- item 1\n",
	})
	if writeResult.IsError {
		t.Fatalf("file_write returned error result: %+v", writeResult)
	}

	var writePayload map[string]any
	if err := decodeStructuredContent(writeResult.StructuredContent, &writePayload); err != nil {
		t.Fatalf("decodeStructuredContent(write) returned error: %v", err)
	}
	if dirty, _ := writePayload["dirty"].(bool); !dirty {
		t.Fatalf("file_write dirty = %#v, want true", writePayload["dirty"])
	}
	if _, ok := writePayload["checkpoint"]; ok {
		t.Fatalf("file_write checkpoint = %#v, want no implicit checkpoint", writePayload["checkpoint"])
	}
	if got := writePayload["operation"]; got != "write" {
		t.Fatalf("file_write operation = %#v, want %q", got, "write")
	}
	if got := writePayload["kind"]; got != "file" {
		t.Fatalf("file_write kind = %#v, want %q", got, "file")
	}
	if got := writePayload["size"]; got != float64(len("# TODO\n- item 1\n")) {
		t.Fatalf("file_write size = %#v, want %d", got, len("# TODO\n- item 1\n"))
	}

	readResult := server.callTool(context.Background(), "file_read", map[string]any{
		"path": "/notes/todo.md",
	})
	if readResult.IsError {
		t.Fatalf("file_read returned error result: %+v", readResult)
	}

	var readPayload map[string]any
	if err := decodeStructuredContent(readResult.StructuredContent, &readPayload); err != nil {
		t.Fatalf("decodeStructuredContent(read) returned error: %v", err)
	}
	if got := readPayload["content"]; got != "# TODO\n- item 1\n" {
		t.Fatalf("file_read content = %#v, want written content", got)
	}

	workspaceMeta, err := server.store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta() returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != "initial" {
		t.Fatalf("workspace HeadSavepoint = %q, want %q", workspaceMeta.HeadSavepoint, "initial")
	}
	if !workspaceMeta.DirtyHint {
		t.Fatal("expected MCP edit to leave the live workspace dirty")
	}
	rootDirty, err := server.store.rdb.Get(context.Background(), controlplane.WorkspaceRootDirtyKey(workspaceMeta.ID)).Result()
	if err != nil {
		t.Fatalf("Get(root_dirty) returned error: %v", err)
	}
	if rootDirty != "1" {
		t.Fatalf("root_dirty = %q, want %q", rootDirty, "1")
	}
}

func TestAFSMCPCheckpointCreatePersistsPendingWrite(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()
	server.profile = controlplane.MCPProfileWorkspaceRWCheckpoint

	writeResult := server.callTool(context.Background(), "file_write", map[string]any{
		"path":    "/notes/todo.md",
		"content": "# TODO\n- item 1\n",
	})
	if writeResult.IsError {
		t.Fatalf("file_write returned error result: %+v", writeResult)
	}

	checkpointResult := server.callTool(context.Background(), "checkpoint_create", map[string]any{
		"checkpoint": "after-edit",
	})
	if checkpointResult.IsError {
		t.Fatalf("checkpoint_create returned error result: %+v", checkpointResult)
	}

	var checkpointPayload map[string]any
	if err := decodeStructuredContent(checkpointResult.StructuredContent, &checkpointPayload); err != nil {
		t.Fatalf("decodeStructuredContent(checkpoint) returned error: %v", err)
	}
	if created, _ := checkpointPayload["created"].(bool); !created {
		t.Fatalf("checkpoint_create created = %#v, want true", checkpointPayload["created"])
	}
	if checkpoint, _ := checkpointPayload["checkpoint"].(string); checkpoint != "after-edit" {
		t.Fatalf("checkpoint_create checkpoint = %#v, want %q", checkpointPayload["checkpoint"], "after-edit")
	}

	workspaceMeta, err := server.store.getWorkspaceMeta(context.Background(), "repo")
	if err != nil {
		t.Fatalf("getWorkspaceMeta() returned error: %v", err)
	}
	if workspaceMeta.HeadSavepoint != "after-edit" {
		t.Fatalf("workspace HeadSavepoint = %q, want %q", workspaceMeta.HeadSavepoint, "after-edit")
	}
	if workspaceMeta.DirtyHint {
		t.Fatal("expected explicit checkpoint to leave the live workspace clean")
	}
	checkpointMeta, err := server.store.getSavepointMeta(context.Background(), "repo", "after-edit")
	if err != nil {
		t.Fatalf("getSavepointMeta(after-edit) returned error: %v", err)
	}
	if checkpointMeta.Kind != controlplane.CheckpointKindManual {
		t.Fatalf("checkpoint kind = %q, want %q", checkpointMeta.Kind, controlplane.CheckpointKindManual)
	}
	if checkpointMeta.Source != controlplane.CheckpointSourceMCP {
		t.Fatalf("checkpoint source = %q, want %q", checkpointMeta.Source, controlplane.CheckpointSourceMCP)
	}
	rootDirty, err := server.store.rdb.Get(context.Background(), controlplane.WorkspaceRootDirtyKey(workspaceMeta.ID)).Result()
	if err != nil {
		t.Fatalf("Get(root_dirty) returned error: %v", err)
	}
	if rootDirty != "0" {
		t.Fatalf("root_dirty = %q, want %q", rootDirty, "0")
	}

	manifest, err := server.store.getManifest(context.Background(), "repo", "after-edit")
	if err != nil {
		t.Fatalf("getManifest(after-edit) returned error: %v", err)
	}
	if _, ok := manifest.Entries["/notes/todo.md"]; !ok {
		t.Fatal("expected checkpoint manifest to include /notes/todo.md")
	}
}

func TestAFSMCPFileWriteDoesNotRematerializeLocalWorkspaceCache(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	treePath := afsWorkspaceTreePath(server.cfg, "repo")
	if err := os.MkdirAll(treePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(treePath) returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(treePath, "local-only.txt"), []byte("keep me\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(local-only.txt) returned error: %v", err)
	}
	now := time.Now().UTC()
	if err := saveAFSLocalState(server.cfg, afsLocalState{
		Version:        afsFormatVersion,
		Workspace:      "repo",
		HeadSavepoint:  "initial",
		Dirty:          false,
		MaterializedAt: now,
		LastScanAt:     now,
	}); err != nil {
		t.Fatalf("saveAFSLocalState() returned error: %v", err)
	}

	writeResult := server.callTool(context.Background(), "file_write", map[string]any{
		"path":    "/notes/todo.md",
		"content": "# TODO\n- item 1\n",
	})
	if writeResult.IsError {
		t.Fatalf("file_write returned error result: %+v", writeResult)
	}

	localOnly, err := os.ReadFile(filepath.Join(treePath, "local-only.txt"))
	if err != nil {
		t.Fatalf("ReadFile(local-only.txt) returned error: %v", err)
	}
	if string(localOnly) != "keep me\n" {
		t.Fatalf("local-only.txt = %q, want %q", string(localOnly), "keep me\n")
	}
	if _, err := os.Stat(filepath.Join(treePath, "notes", "todo.md")); !os.IsNotExist(err) {
		t.Fatalf("expected mounted MCP edit to leave the local cache untouched, got err=%v", err)
	}
}

func TestAFSMCPFileGrepUsesCurrentWorkspace(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	if _, err := server.toolFileWrite(context.Background(), map[string]any{
		"path":    "/logs/app.log",
		"content": "Error: boom\nok\n",
	}); err != nil {
		t.Fatalf("toolFileWrite(app.log) returned error: %v", err)
	}
	if _, err := server.toolFileWrite(context.Background(), map[string]any{
		"path":    "/logs/worker.log",
		"content": "warning: queued\n",
	}); err != nil {
		t.Fatalf("toolFileWrite(worker.log) returned error: %v", err)
	}

	result := server.callTool(context.Background(), "file_grep", map[string]any{
		"path":        "/logs",
		"pattern":     "error|warning",
		"regexp":      true,
		"ignore_case": true,
	})
	if result.IsError {
		t.Fatalf("file_grep returned error result: %+v", result)
	}

	var payload map[string]any
	if err := decodeStructuredContent(result.StructuredContent, &payload); err != nil {
		t.Fatalf("decodeStructuredContent(grep) returned error: %v", err)
	}
	matches, ok := payload["matches"].([]any)
	if !ok {
		t.Fatalf("grep matches missing: %#v", payload)
	}
	if len(matches) != 2 {
		t.Fatalf("grep matches len = %d, want 2", len(matches))
	}
}

func TestAFSMCPFileGlobFindsFiles(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	if _, err := server.toolFileWrite(context.Background(), map[string]any{
		"path":    "/docs/readme.md",
		"content": "# readme\n",
	}); err != nil {
		t.Fatalf("toolFileWrite(readme) returned error: %v", err)
	}
	if _, err := server.toolFileWrite(context.Background(), map[string]any{
		"path":    "/docs/notes.txt",
		"content": "notes\n",
	}); err != nil {
		t.Fatalf("toolFileWrite(notes) returned error: %v", err)
	}

	result := server.callTool(context.Background(), "file_glob", map[string]any{
		"path":    "/docs",
		"pattern": "*.md",
		"kind":    "file",
	})
	if result.IsError {
		t.Fatalf("file_glob returned error result: %+v", result)
	}

	var payload map[string]any
	if err := decodeStructuredContent(result.StructuredContent, &payload); err != nil {
		t.Fatalf("decodeStructuredContent(glob) returned error: %v", err)
	}
	items, ok := payload["items"].([]any)
	if !ok {
		t.Fatalf("glob items missing: %#v", payload)
	}
	if len(items) != 1 {
		t.Fatalf("glob items len = %d, want 1", len(items))
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("glob first item = %#v, want map", items[0])
	}
	if got := first["path"]; got != "/docs/readme.md" {
		t.Fatalf("glob path = %#v, want %q", got, "/docs/readme.md")
	}
}

func TestAFSMCPFileReplaceRequiresDisambiguationForMultipleMatches(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	if _, err := server.toolFileWrite(context.Background(), map[string]any{
		"path":    "/app.txt",
		"content": "hello\nhello\n",
	}); err != nil {
		t.Fatalf("toolFileWrite(app.txt) returned error: %v", err)
	}

	result := server.callTool(context.Background(), "file_replace", map[string]any{
		"path": "/app.txt",
		"old":  "hello",
		"new":  "hi",
	})
	if !result.IsError {
		t.Fatalf("file_replace expected ambiguity error, got %+v", result)
	}
}

func TestAFSMCPFileReplaceSupportsStartLineGuards(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	if _, err := server.toolFileWrite(context.Background(), map[string]any{
		"path":    "/app.txt",
		"content": "hello\nhello\n",
	}); err != nil {
		t.Fatalf("toolFileWrite(app.txt) returned error: %v", err)
	}

	result := server.callTool(context.Background(), "file_replace", map[string]any{
		"path":                 "/app.txt",
		"old":                  "hello",
		"new":                  "hi",
		"start_line":           2,
		"expected_occurrences": 1,
	})
	if result.IsError {
		t.Fatalf("file_replace returned error result: %+v", result)
	}

	readResult := server.callTool(context.Background(), "file_read", map[string]any{
		"path": "/app.txt",
	})
	if readResult.IsError {
		t.Fatalf("file_read returned error result: %+v", readResult)
	}
	var payload map[string]any
	if err := decodeStructuredContent(readResult.StructuredContent, &payload); err != nil {
		t.Fatalf("decodeStructuredContent(read) returned error: %v", err)
	}
	if got := payload["content"]; got != "hello\nhi\n" {
		t.Fatalf("file_read content = %#v, want %q", got, "hello\nhi\n")
	}
}

func TestAFSMCPFilePatchAppliesStructuredEdits(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	initial := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	if _, err := server.toolFileWrite(context.Background(), map[string]any{
		"path":    "/main.go",
		"content": initial,
	}); err != nil {
		t.Fatalf("toolFileWrite(main.go) returned error: %v", err)
	}

	sum := sha256.Sum256([]byte(initial))
	result := server.callTool(context.Background(), "file_patch", map[string]any{
		"path":            "/main.go",
		"expected_sha256": hex.EncodeToString(sum[:]),
		"patches": []map[string]any{
			{
				"op":         "replace",
				"old":        "println(\"hello\")",
				"new":        "println(\"hello, world\")",
				"start_line": 4,
			},
			{
				"op":         "insert",
				"start_line": 2,
				"new":        "import \"fmt\"\n",
			},
		},
	})
	if result.IsError {
		t.Fatalf("file_patch returned error result: %+v", result)
	}

	readResult := server.callTool(context.Background(), "file_read", map[string]any{
		"path": "/main.go",
	})
	if readResult.IsError {
		t.Fatalf("file_read returned error result: %+v", readResult)
	}
	var payload map[string]any
	if err := decodeStructuredContent(readResult.StructuredContent, &payload); err != nil {
		t.Fatalf("decodeStructuredContent(read) returned error: %v", err)
	}
	want := "package main\n\nimport \"fmt\"\nfunc main() {\n\tprintln(\"hello, world\")\n}\n"
	if got := payload["content"]; got != want {
		t.Fatalf("file_read content = %#v, want %q", got, want)
	}
}

func TestAFSMCPStatusAndWorkspaceCurrentPreferActiveSyncWorkspace(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	if err := createEmptyWorkspace(context.Background(), server.cfg, server.store, "beta"); err != nil {
		t.Fatalf("createEmptyWorkspace(beta) returned error: %v", err)
	}

	if err := saveState(state{
		StartedAt:        time.Now().UTC(),
		ProductMode:      productModeLocal,
		RedisAddr:        server.cfg.RedisAddr,
		RedisDB:          server.cfg.RedisDB,
		CurrentWorkspace: "beta",
		Mode:             modeSync,
		SyncPID:          os.Getpid(),
	}); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	status, err := server.toolAFSStatus()
	if err != nil {
		t.Fatalf("toolAFSStatus() returned error: %v", err)
	}
	statusMap, ok := status.(map[string]any)
	if !ok {
		t.Fatalf("toolAFSStatus() = %#v, want map", status)
	}
	if got := statusMap["current_workspace"]; got != "beta" {
		t.Fatalf("afs_status current_workspace = %#v, want %q", got, "beta")
	}

	current, err := server.toolWorkspaceCurrent(context.Background())
	if err != nil {
		t.Fatalf("toolWorkspaceCurrent() returned error: %v", err)
	}
	currentMap, ok := current.(map[string]any)
	if !ok {
		t.Fatalf("toolWorkspaceCurrent() = %#v, want map", current)
	}
	if got := currentMap["workspace"]; got != "beta" {
		t.Fatalf("workspace_current workspace = %#v, want %q", got, "beta")
	}
	if got := currentMap["exists"]; got != true {
		t.Fatalf("workspace_current exists = %#v, want true", got)
	}
}

func TestAFSMCPWorkspaceVersioningPolicyToolsRoundTrip(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	getResult := server.callTool(context.Background(), "workspace_get_versioning_policy", map[string]any{})
	if getResult.IsError {
		t.Fatalf("workspace_get_versioning_policy returned error result: %+v", getResult)
	}
	var getPayload map[string]any
	if err := decodeStructuredContent(getResult.StructuredContent, &getPayload); err != nil {
		t.Fatalf("decodeStructuredContent(get) returned error: %v", err)
	}
	policyMap, ok := getPayload["policy"].(map[string]any)
	if !ok {
		t.Fatalf("get policy = %#v, want map", getPayload["policy"])
	}
	if got := policyMap["mode"]; got != controlplane.WorkspaceVersioningModeOff {
		t.Fatalf("initial policy.mode = %#v, want %q", got, controlplane.WorkspaceVersioningModeOff)
	}

	setResult := server.callTool(context.Background(), "workspace_set_versioning_policy", map[string]any{
		"mode":                    controlplane.WorkspaceVersioningModePaths,
		"include_globs":           []any{"src/**", "docs/**"},
		"exclude_globs":           []any{"**/*.tmp"},
		"max_versions_per_file":   7,
		"max_age_days":            30,
		"max_total_bytes":         int64(2048),
		"large_file_cutoff_bytes": int64(512),
	})
	if setResult.IsError {
		t.Fatalf("workspace_set_versioning_policy returned error result: %+v", setResult)
	}

	var setPayload map[string]any
	if err := decodeStructuredContent(setResult.StructuredContent, &setPayload); err != nil {
		t.Fatalf("decodeStructuredContent(set) returned error: %v", err)
	}
	if got := setPayload["workspace"]; got != "repo" {
		t.Fatalf("set workspace = %#v, want %q", got, "repo")
	}

	getUpdated := server.callTool(context.Background(), "workspace_get_versioning_policy", map[string]any{})
	if getUpdated.IsError {
		t.Fatalf("workspace_get_versioning_policy(updated) returned error result: %+v", getUpdated)
	}
	var updatedPayload map[string]any
	if err := decodeStructuredContent(getUpdated.StructuredContent, &updatedPayload); err != nil {
		t.Fatalf("decodeStructuredContent(updated) returned error: %v", err)
	}
	updatedPolicy, ok := updatedPayload["policy"].(map[string]any)
	if !ok {
		t.Fatalf("updated policy = %#v, want map", updatedPayload["policy"])
	}
	if got := updatedPolicy["mode"]; got != controlplane.WorkspaceVersioningModePaths {
		t.Fatalf("updated policy.mode = %#v, want %q", got, controlplane.WorkspaceVersioningModePaths)
	}
	if got := updatedPolicy["max_versions_per_file"]; got != float64(7) {
		t.Fatalf("updated policy.max_versions_per_file = %#v, want 7", got)
	}
	if got := updatedPolicy["max_age_days"]; got != float64(30) {
		t.Fatalf("updated policy.max_age_days = %#v, want 30", got)
	}
	if got := updatedPolicy["max_total_bytes"]; got != float64(2048) {
		t.Fatalf("updated policy.max_total_bytes = %#v, want 2048", got)
	}
	if got := updatedPolicy["large_file_cutoff_bytes"]; got != float64(512) {
		t.Fatalf("updated policy.large_file_cutoff_bytes = %#v, want 512", got)
	}
}

func TestAFSMCPFileWriteCreatesVersionWhenPolicyEnabled(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	if err := server.store.cp.PutWorkspaceVersioningPolicy(context.Background(), "repo", controlplane.WorkspaceVersioningPolicy{
		Mode: controlplane.WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	result := server.callTool(context.Background(), "file_write", map[string]any{
		"path":    "/notes/history.txt",
		"content": "hello history\n",
	})
	if result.IsError {
		t.Fatalf("file_write returned error result: %+v", result)
	}

	var payload map[string]any
	if err := decodeStructuredContent(result.StructuredContent, &payload); err != nil {
		t.Fatalf("decodeStructuredContent() returned error: %v", err)
	}
	if payload["file_id"] == nil || payload["version_id"] == nil {
		t.Fatalf("payload missing version identifiers: %#v", payload)
	}

	lineage, err := server.store.cp.ResolveLiveFileLineageByPath(context.Background(), "repo", "/notes/history.txt")
	if err != nil {
		t.Fatalf("ResolveLiveFileLineageByPath() returned error: %v", err)
	}
	versions, err := server.store.cp.ListFileVersions(context.Background(), "repo", lineage.FileID, true)
	if err != nil {
		t.Fatalf("ListFileVersions() returned error: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("len(versions) = %d, want 1", len(versions))
	}
	if versions[0].ContentHash == "" {
		t.Fatalf("versions[0].ContentHash = %q, want non-empty", versions[0].ContentHash)
	}
	if versions[0].BlobID == "" {
		t.Fatalf("versions[0].BlobID = %q, want non-empty", versions[0].BlobID)
	}
	blob, err := server.store.cp.GetBlob(context.Background(), "repo", versions[0].BlobID)
	if err != nil {
		t.Fatalf("GetBlob() returned error: %v", err)
	}
	if string(blob) != "hello history\n" {
		t.Fatalf("blob = %q, want %q", string(blob), "hello history\n")
	}
}

func TestAFSMCPFileHistoryAndReadVersionTools(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	if err := server.store.cp.PutWorkspaceVersioningPolicy(context.Background(), "repo", controlplane.WorkspaceVersioningPolicy{
		Mode: controlplane.WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}

	writeResult := server.callTool(context.Background(), "file_write", map[string]any{
		"path":    "/notes/history-tools.txt",
		"content": "tool history\n",
	})
	if writeResult.IsError {
		t.Fatalf("file_write returned error result: %+v", writeResult)
	}

	var writePayload map[string]any
	if err := decodeStructuredContent(writeResult.StructuredContent, &writePayload); err != nil {
		t.Fatalf("decodeStructuredContent(write) returned error: %v", err)
	}
	versionID, _ := writePayload["version_id"].(string)
	if versionID == "" {
		t.Fatalf("version_id = %#v, want non-empty", writePayload["version_id"])
	}

	updatedWrite := server.callTool(context.Background(), "file_write", map[string]any{
		"path":    "/notes/history-tools.txt",
		"content": "tool history updated\n",
	})
	if updatedWrite.IsError {
		t.Fatalf("second file_write returned error result: %+v", updatedWrite)
	}
	var updatedWritePayload map[string]any
	if err := decodeStructuredContent(updatedWrite.StructuredContent, &updatedWritePayload); err != nil {
		t.Fatalf("decodeStructuredContent(updated write) returned error: %v", err)
	}
	secondVersionID, _ := updatedWritePayload["version_id"].(string)

	historyResult := server.callTool(context.Background(), "file_history", map[string]any{
		"path":      "/notes/history-tools.txt",
		"direction": "asc",
		"limit":     1,
	})
	if historyResult.IsError {
		t.Fatalf("file_history returned error result: %+v", historyResult)
	}
	var historyPayload map[string]any
	if err := decodeStructuredContent(historyResult.StructuredContent, &historyPayload); err != nil {
		t.Fatalf("decodeStructuredContent(history) returned error: %v", err)
	}
	history, ok := historyPayload["history"].(map[string]any)
	if !ok {
		t.Fatalf("history = %#v, want map", historyPayload["history"])
	}
	if got := history["order"]; got != "asc" {
		t.Fatalf("history.order = %#v, want asc", got)
	}
	lineages, ok := history["lineages"].([]any)
	if !ok || len(lineages) != 1 {
		t.Fatalf("history.lineages = %#v, want one lineage", history["lineages"])
	}
	if history["next_cursor"] == nil || history["next_cursor"] == "" {
		t.Fatalf("history.next_cursor = %#v, want non-empty", history["next_cursor"])
	}
	nextCursor, _ := history["next_cursor"].(string)

	historyPage2Result := server.callTool(context.Background(), "file_history", map[string]any{
		"path":      "/notes/history-tools.txt",
		"direction": "asc",
		"limit":     1,
		"cursor":    nextCursor,
	})
	if historyPage2Result.IsError {
		t.Fatalf("file_history page 2 returned error result: %+v", historyPage2Result)
	}
	var historyPage2Payload map[string]any
	if err := decodeStructuredContent(historyPage2Result.StructuredContent, &historyPage2Payload); err != nil {
		t.Fatalf("decodeStructuredContent(history page 2) returned error: %v", err)
	}
	historyPage2, ok := historyPage2Payload["history"].(map[string]any)
	if !ok {
		t.Fatalf("history page 2 = %#v, want map", historyPage2Payload["history"])
	}
	lineagesPage2, ok := historyPage2["lineages"].([]any)
	if !ok || len(lineagesPage2) != 1 {
		t.Fatalf("history page 2 lineages = %#v, want one lineage", historyPage2["lineages"])
	}
	lineagePage2, ok := lineagesPage2[0].(map[string]any)
	if !ok {
		t.Fatalf("history page 2 lineage = %#v, want map", lineagesPage2[0])
	}
	versionsPage2, ok := lineagePage2["versions"].([]any)
	if !ok || len(versionsPage2) != 1 {
		t.Fatalf("history page 2 versions = %#v, want one version", lineagePage2["versions"])
	}
	versionPage2, ok := versionsPage2[0].(map[string]any)
	if !ok {
		t.Fatalf("history page 2 version = %#v, want map", versionsPage2[0])
	}
	if got := versionPage2["version_id"]; got != secondVersionID {
		t.Fatalf("history page 2 version_id = %#v, want %q", got, secondVersionID)
	}

	versionResult := server.callTool(context.Background(), "file_read_version", map[string]any{
		"version_id": versionID,
	})
	if versionResult.IsError {
		t.Fatalf("file_read_version returned error result: %+v", versionResult)
	}
	var versionPayload map[string]any
	if err := decodeStructuredContent(versionResult.StructuredContent, &versionPayload); err != nil {
		t.Fatalf("decodeStructuredContent(version) returned error: %v", err)
	}
	version, ok := versionPayload["version"].(map[string]any)
	if !ok {
		t.Fatalf("version = %#v, want map", versionPayload["version"])
	}
	if got := version["content"]; got != "tool history\n" {
		t.Fatalf("version.content = %#v, want written content", got)
	}
	fileID, _ := version["file_id"].(string)
	if fileID == "" {
		t.Fatalf("version.file_id = %#v, want non-empty", version["file_id"])
	}

	ordinalResult := server.callTool(context.Background(), "file_read_version", map[string]any{
		"file_id": fileID,
		"ordinal": 1,
	})
	if ordinalResult.IsError {
		t.Fatalf("file_read_version by ordinal returned error result: %+v", ordinalResult)
	}

	diffResult := server.callTool(context.Background(), "file_diff_versions", map[string]any{
		"path":            "/notes/history-tools.txt",
		"from_version_id": versionID,
		"to_ref":          "working-copy",
	})
	if diffResult.IsError {
		t.Fatalf("file_diff_versions returned error result: %+v", diffResult)
	}
	var diffPayload map[string]any
	if err := decodeStructuredContent(diffResult.StructuredContent, &diffPayload); err != nil {
		t.Fatalf("decodeStructuredContent(diff) returned error: %v", err)
	}
	diff, ok := diffPayload["diff"].(map[string]any)
	if !ok {
		t.Fatalf("diff = %#v, want map", diffPayload["diff"])
	}
	if got, _ := diff["binary"].(bool); got {
		t.Fatalf("diff.binary = true, want false")
	}
	if got, _ := diff["diff"].(string); !strings.Contains(got, "tool history\n") || !strings.Contains(got, "tool history updated\n") {
		t.Fatalf("diff.diff = %q, want historical and working-copy content", got)
	}

	restoreResult := server.callTool(context.Background(), "file_restore_version", map[string]any{
		"path":       "/notes/history-tools.txt",
		"version_id": versionID,
	})
	if restoreResult.IsError {
		t.Fatalf("file_restore_version returned error result: %+v", restoreResult)
	}

	deletedVersion, err := server.store.cp.RecordFileVersionMutation(context.Background(), "repo", controlplane.VersionedFileSnapshot{Path: "/notes/deleted-tools.txt"}, controlplane.VersionedFileSnapshot{
		Path:    "/notes/deleted-tools.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("deleted tool\n"),
	}, controlplane.FileVersionMutationMetadata{Source: controlplane.ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(undelete create) returned error: %v", err)
	}
	if _, err := server.store.cp.RecordFileVersionMutation(context.Background(), "repo", controlplane.VersionedFileSnapshot{
		Path:    "/notes/deleted-tools.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("deleted tool\n"),
	}, controlplane.VersionedFileSnapshot{
		Path: "/notes/deleted-tools.txt",
	}, controlplane.FileVersionMutationMetadata{Source: controlplane.ChangeSourceMCP}); err != nil {
		t.Fatalf("RecordFileVersionMutation(undelete delete) returned error: %v", err)
	}

	undeleteResult := server.callTool(context.Background(), "file_undelete", map[string]any{
		"path": "/notes/deleted-tools.txt",
	})
	if undeleteResult.IsError {
		t.Fatalf("file_undelete returned error result: %+v", undeleteResult)
	}
	var undeletePayload map[string]any
	if err := decodeStructuredContent(undeleteResult.StructuredContent, &undeletePayload); err != nil {
		t.Fatalf("decodeStructuredContent(undelete) returned error: %v", err)
	}
	undelete, ok := undeletePayload["undelete"].(map[string]any)
	if !ok {
		t.Fatalf("undelete = %#v, want map", undeletePayload["undelete"])
	}
	if got, _ := undelete["undeleted_from_version_id"].(string); got != deletedVersion.VersionID {
		t.Fatalf("undelete.undeleted_from_version_id = %q, want %q", got, deletedVersion.VersionID)
	}
}

func setupAFSMCPTestServer(t *testing.T) (*afsMCPServer, func()) {
	t.Helper()

	mr := miniredis.RunT(t)
	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNone
	cfg.WorkRoot = t.TempDir()
	cfg.CurrentWorkspace = "repo"
	saveTempConfig(t, cfg)

	loadedCfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	if err := createEmptyWorkspace(context.Background(), loadedCfg, store, "repo"); err != nil {
		closeStore()
		t.Fatalf("createEmptyWorkspace() returned error: %v", err)
	}

	server := &afsMCPServer{
		cfg:     loadedCfg,
		store:   store,
		service: controlPlaneServiceFromStore(loadedCfg, store),
	}
	return server, closeStore
}

func frameForTest(body string) string {
	return "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
}

func TestAFSMCPFileCreateExclusiveCreatesNewFile(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	result := server.callTool(context.Background(), "file_create_exclusive", map[string]any{
		"path":    "/tasks/001.claim",
		"content": "agent-sre\n",
	})
	if result.IsError {
		t.Fatalf("file_create_exclusive returned error result: %+v", result)
	}

	var payload map[string]any
	if err := decodeStructuredContent(result.StructuredContent, &payload); err != nil {
		t.Fatalf("decodeStructuredContent(create) returned error: %v", err)
	}
	if op, _ := payload["operation"].(string); op != "create_exclusive" {
		t.Fatalf("operation = %#v, want %q", payload["operation"], "create_exclusive")
	}
	if created, _ := payload["created"].(bool); !created {
		t.Fatalf("created = %#v, want true", payload["created"])
	}
	if dirty, _ := payload["dirty"].(bool); !dirty {
		t.Fatalf("dirty = %#v, want true", payload["dirty"])
	}

	// Verify the file is readable with expected content.
	readResult := server.callTool(context.Background(), "file_read", map[string]any{
		"path": "/tasks/001.claim",
	})
	if readResult.IsError {
		t.Fatalf("file_read returned error result: %+v", readResult)
	}
	var readPayload map[string]any
	if err := decodeStructuredContent(readResult.StructuredContent, &readPayload); err != nil {
		t.Fatalf("decodeStructuredContent(read) returned error: %v", err)
	}
	if got := readPayload["content"]; got != "agent-sre\n" {
		t.Fatalf("file_read content = %#v, want written content", got)
	}
}

func TestAFSMCPFileCreateExclusiveFailsWhenFileExists(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	// Create the file first via normal write.
	writeResult := server.callTool(context.Background(), "file_write", map[string]any{
		"path":    "/tasks/001.claim",
		"content": "agent-sre\n",
	})
	if writeResult.IsError {
		t.Fatalf("file_write returned error result: %+v", writeResult)
	}

	// Attempt exclusive create — should fail.
	result := server.callTool(context.Background(), "file_create_exclusive", map[string]any{
		"path":    "/tasks/001.claim",
		"content": "agent-deploy\n",
	})
	if !result.IsError {
		t.Fatal("file_create_exclusive on existing file should return error, got success")
	}
}

func TestAFSMCPFileCreateExclusiveDoubleCallSecondFails(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	// First exclusive create succeeds.
	first := server.callTool(context.Background(), "file_create_exclusive", map[string]any{
		"path":    "/locks/deploy.lock",
		"content": "agent-1\n",
	})
	if first.IsError {
		t.Fatalf("first file_create_exclusive returned error: %+v", first)
	}

	// Second exclusive create to same path fails.
	second := server.callTool(context.Background(), "file_create_exclusive", map[string]any{
		"path":    "/locks/deploy.lock",
		"content": "agent-2\n",
	})
	if !second.IsError {
		t.Fatal("second file_create_exclusive should fail, got success")
	}

	// Original content should be preserved.
	readResult := server.callTool(context.Background(), "file_read", map[string]any{
		"path": "/locks/deploy.lock",
	})
	if readResult.IsError {
		t.Fatalf("file_read returned error: %+v", readResult)
	}
	var readPayload map[string]any
	if err := decodeStructuredContent(readResult.StructuredContent, &readPayload); err != nil {
		t.Fatalf("decodeStructuredContent(read) returned error: %v", err)
	}
	if got := readPayload["content"]; got != "agent-1\n" {
		t.Fatalf("content = %#v, want original agent-1 content", got)
	}
}

func TestAFSMCPFileCreateExclusiveRequiresPath(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	result := server.callTool(context.Background(), "file_create_exclusive", map[string]any{
		"content": "data\n",
	})
	if !result.IsError {
		t.Fatal("file_create_exclusive without path should return error, got success")
	}
}

func TestAFSMCPFileCreateExclusiveRequiresContent(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	result := server.callTool(context.Background(), "file_create_exclusive", map[string]any{
		"path": "/tasks/empty.claim",
	})
	if !result.IsError {
		t.Fatal("file_create_exclusive without content should return error, got success")
	}
}

func TestAFSMCPFileCreateExclusiveAllowsEmptyContent(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	result := server.callTool(context.Background(), "file_create_exclusive", map[string]any{
		"path":    "/locks/empty.lock",
		"content": "",
	})
	if result.IsError {
		t.Fatalf("file_create_exclusive with empty content returned error: %+v", result)
	}

	readResult := server.callTool(context.Background(), "file_read", map[string]any{
		"path": "/locks/empty.lock",
	})
	if readResult.IsError {
		t.Fatalf("file_read returned error: %+v", readResult)
	}

	var readPayload map[string]any
	if err := decodeStructuredContent(readResult.StructuredContent, &readPayload); err != nil {
		t.Fatalf("decodeStructuredContent(read) returned error: %v", err)
	}
	if got := readPayload["content"]; got != "" {
		t.Fatalf("content = %#v, want empty string", got)
	}
}

func TestAFSMCPFileCreateExclusiveCreatesParentDirs(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	result := server.callTool(context.Background(), "file_create_exclusive", map[string]any{
		"path":    "/deep/nested/dir/file.lock",
		"content": "locked\n",
	})
	if result.IsError {
		t.Fatalf("file_create_exclusive with nested path returned error: %+v", result)
	}

	// Verify parent directory exists by listing it.
	listResult := server.callTool(context.Background(), "file_list", map[string]any{
		"path": "/deep/nested/dir",
	})
	if listResult.IsError {
		t.Fatalf("file_list on parent dir returned error: %+v", listResult)
	}
}

func TestAFSMCPFileCreateExclusiveOnDirectoryFails(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	// Create a file to ensure parent dir exists.
	writeResult := server.callTool(context.Background(), "file_write", map[string]any{
		"path":    "/mydir/placeholder.txt",
		"content": "x",
	})
	if writeResult.IsError {
		t.Fatalf("file_write returned error: %+v", writeResult)
	}

	// Attempt exclusive create on the directory path.
	result := server.callTool(context.Background(), "file_create_exclusive", map[string]any{
		"path":    "/mydir",
		"content": "should fail",
	})
	if !result.IsError {
		t.Fatal("file_create_exclusive on a directory should return error, got success")
	}
}

func TestAFSMCPFileCreateExclusiveNonexistentWorkspaceFails(t *testing.T) {
	t.Helper()

	server, closeFn := setupAFSMCPTestServer(t)
	defer closeFn()

	result := server.callTool(context.Background(), "file_create_exclusive", map[string]any{
		"workspace": "no-such-workspace",
		"path":      "/test.lock",
		"content":   "data\n",
	})
	if !result.IsError {
		t.Fatal("file_create_exclusive with nonexistent workspace should return error, got success")
	}
}

func decodeStructuredContent(value any, target any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}
