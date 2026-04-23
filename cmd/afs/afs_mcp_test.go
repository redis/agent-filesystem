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

func decodeStructuredContent(value any, target any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}
