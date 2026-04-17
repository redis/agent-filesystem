package controlplane

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	mountclient "github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

func TestNormalizeCLITargetAliases(t *testing.T) {
	t.Helper()

	target, err := normalizeCLITarget("macos", "x86_64")
	if err != nil {
		t.Fatalf("normalizeCLITarget() returned error: %v", err)
	}
	if target.GOOS != "darwin" || target.GOARCH != "amd64" || target.Filename != "afs" {
		t.Fatalf("normalizeCLITarget() = %+v, want darwin/amd64/afs", target)
	}
}

func TestHandleCLIDownloadServesRequestedPrebuiltArtifact(t *testing.T) {
	t.Helper()

	artifactDir := t.TempDir()
	targetDir := filepath.Join(artifactDir, "darwin-arm64")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() returned error: %v", err)
	}
	want := []byte("fake-cli")
	if err := os.WriteFile(filepath.Join(targetDir, "afs"), want, 0o755); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}
	t.Setenv("AFS_CLI_ARTIFACT_DIR", artifactDir)

	req := httptest.NewRequest(http.MethodGet, "/v1/cli?os=darwin&arch=arm64", nil)
	rec := httptest.NewRecorder()
	handleCLIDownload(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /v1/cli status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() returned error: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("GET /v1/cli body = %q, want %q", got, want)
	}
	if disp := resp.Header.Get("Content-Disposition"); !strings.Contains(disp, `filename="afs"`) {
		t.Fatalf("Content-Disposition = %q, want afs filename", disp)
	}
}

func TestHTTPBrowseAndRestore(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces")
	if err != nil {
		t.Fatalf("GET scoped workspaces returned error: %v", err)
	}
	defer resp.Body.Close()

	var summaries workspaceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&summaries); err != nil {
		t.Fatalf("Decode(workspaces) returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET scoped workspaces status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if len(summaries.Items) != 1 {
		t.Fatalf("len(workspaces.items) = %d, want 1", len(summaries.Items))
	}
	if summaries.Items[0].FileCount != 2 {
		t.Fatalf("summary file_count = %d, want 2", summaries.Items[0].FileCount)
	}
	if summaries.Items[0].DatabaseID != databaseID {
		t.Fatalf("summary database_id = %q, want %q", summaries.Items[0].DatabaseID, databaseID)
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo")
	if err != nil {
		t.Fatalf("GET scoped workspace detail returned error: %v", err)
	}
	defer resp.Body.Close()

	var detail workspaceDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("Decode(workspace detail) returned error: %v", err)
	}
	if detail.HeadCheckpointID != "snapshot" {
		t.Fatalf("detail head_checkpoint_id = %q, want %q", detail.HeadCheckpointID, "snapshot")
	}
	if detail.CheckpointCount != 2 {
		t.Fatalf("detail checkpoint_count = %d, want 2", detail.CheckpointCount)
	}
	if !detail.Capabilities.BrowseWorkingCopy {
		t.Fatal("detail capabilities should expose working-copy browsing")
	}
	if detail.DatabaseID != databaseID {
		t.Fatalf("detail database_id = %q, want %q", detail.DatabaseID, databaseID)
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/tree?view=head&path=/&depth=1")
	if err != nil {
		t.Fatalf("GET scoped tree returned error: %v", err)
	}
	defer resp.Body.Close()

	var tree treeResponse
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		t.Fatalf("Decode(tree) returned error: %v", err)
	}
	if len(tree.Items) != 2 {
		t.Fatalf("len(tree.items) = %d, want 2", len(tree.Items))
	}
	if tree.Items[0].Path != "/src" || tree.Items[1].Path != "/README.md" {
		t.Fatalf("tree root items = %#v, want /src and /README.md", tree.Items)
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/files/content?view=head&path=/README.md")
	if err != nil {
		t.Fatalf("GET scoped file content returned error: %v", err)
	}
	defer resp.Body.Close()

	var file fileContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		t.Fatalf("Decode(file content) returned error: %v", err)
	}
	if file.Content != "# demo\n" {
		t.Fatalf("file content = %q, want %q", file.Content, "# demo\n")
	}

	resp, err = http.Post(
		server.URL+"/v1/databases/"+databaseID+"/workspaces/repo:restore",
		"application/json",
		strings.NewReader(`{"checkpoint_id":"initial"}`),
	)
	if err != nil {
		t.Fatalf("POST scoped restore returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST restore status = %d, want %d, body=%s", resp.StatusCode, http.StatusNoContent, body)
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo")
	if err != nil {
		t.Fatalf("GET scoped workspace detail after restore returned error: %v", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("Decode(workspace detail after restore) returned error: %v", err)
	}
	if detail.HeadCheckpointID != "initial" {
		t.Fatalf("restored head_checkpoint_id = %q, want %q", detail.HeadCheckpointID, "initial")
	}
}

func TestHTTPOnboardingTokenExchange(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Post(server.URL+"/v1/databases/"+databaseID+"/workspaces/repo/onboarding-token", "application/json", nil)
	if err != nil {
		t.Fatalf("POST onboarding token returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST onboarding token status = %d, want %d, body=%s", resp.StatusCode, http.StatusCreated, body)
	}

	var token onboardingTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		t.Fatalf("Decode(onboarding token) returned error: %v", err)
	}
	if token.Token == "" {
		t.Fatal("expected onboarding token to be populated")
	}
	if token.DatabaseID != databaseID {
		t.Fatalf("token database_id = %q, want %q", token.DatabaseID, databaseID)
	}
	if token.WorkspaceName != "repo" {
		t.Fatalf("token workspace_name = %q, want %q", token.WorkspaceName, "repo")
	}

	body := strings.NewReader(fmtJSON(t, onboardingExchangeRequest{Token: token.Token}))
	resp, err = http.Post(server.URL+"/v1/auth/exchange", "application/json", body)
	if err != nil {
		t.Fatalf("POST auth exchange returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST auth exchange status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, payload)
	}

	var exchange onboardingExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&exchange); err != nil {
		t.Fatalf("Decode(onboarding exchange) returned error: %v", err)
	}
	if exchange.DatabaseID != databaseID {
		t.Fatalf("exchange database_id = %q, want %q", exchange.DatabaseID, databaseID)
	}
	if exchange.WorkspaceName != "repo" {
		t.Fatalf("exchange workspace_name = %q, want %q", exchange.WorkspaceName, "repo")
	}

	resp, err = http.Post(server.URL+"/v1/auth/exchange", "application/json", strings.NewReader(fmtJSON(t, onboardingExchangeRequest{Token: token.Token})))
	if err != nil {
		t.Fatalf("POST auth exchange second call returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST auth exchange second status = %d, want %d, body=%s", resp.StatusCode, http.StatusUnauthorized, payload)
	}
}

func TestHTTPResolvedOnboardingTokenExchange(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Post(server.URL+"/v1/workspaces/repo/onboarding-token", "application/json", nil)
	if err != nil {
		t.Fatalf("POST resolved onboarding token returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST resolved onboarding token status = %d, want %d, body=%s", resp.StatusCode, http.StatusCreated, body)
	}

	var token onboardingTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		t.Fatalf("Decode(resolved onboarding token) returned error: %v", err)
	}
	if token.Token == "" {
		t.Fatal("expected resolved onboarding token to be populated")
	}
	if token.DatabaseID != databaseID {
		t.Fatalf("resolved token database_id = %q, want %q", token.DatabaseID, databaseID)
	}
	if token.WorkspaceName != "repo" {
		t.Fatalf("resolved token workspace_name = %q, want %q", token.WorkspaceName, "repo")
	}

	resp, err = http.Post(server.URL+"/v1/auth/exchange", "application/json", strings.NewReader(fmtJSON(t, onboardingExchangeRequest{Token: token.Token})))
	if err != nil {
		t.Fatalf("POST resolved auth exchange returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST resolved auth exchange status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, payload)
	}
}

func TestHTTPOnboardingTokenExchangeRejectsUnknownToken(t *testing.T) {
	t.Helper()

	manager, _ := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Post(server.URL+"/v1/auth/exchange", "application/json", strings.NewReader(fmtJSON(t, onboardingExchangeRequest{Token: "afs_otk_missing"})))
	if err != nil {
		t.Fatalf("POST auth exchange for unknown token returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST auth exchange unknown token status = %d, want %d, body=%s", resp.StatusCode, http.StatusUnauthorized, payload)
	}

	var payload map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode(unknown token response) returned error: %v", err)
	}
	if payload["error"] != ErrOnboardingTokenInvalid.Error() {
		t.Fatalf("unknown token error = %q, want %q", payload["error"], ErrOnboardingTokenInvalid.Error())
	}
}

func TestHTTPBrowseWorkingCopyView(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	ctx := context.Background()
	service, _, err := manager.serviceFor(ctx, databaseID)
	if err != nil {
		t.Fatalf("manager.serviceFor() returned error: %v", err)
	}
	if _, _, _, err := EnsureWorkspaceRoot(ctx, service.store, "repo"); err != nil {
		t.Fatalf("EnsureWorkspaceRoot() returned error: %v", err)
	}

	fsClient := mountclient.New(service.store.rdb, WorkspaceFSKey("repo"))
	if err := fsClient.Echo(ctx, "/drafts/notes.txt", []byte("working copy\n")); err != nil {
		t.Fatalf("Echo() returned error: %v", err)
	}
	if err := MarkWorkspaceRootDirty(ctx, service.store, "repo"); err != nil {
		t.Fatalf("MarkWorkspaceRootDirty() returned error: %v", err)
	}
	expectedBytes := int64(len("# demo\n") + len("package main\n") + len("working copy\n"))

	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces")
	if err != nil {
		t.Fatalf("GET scoped workspaces returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET scoped workspaces status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var summaries workspaceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&summaries); err != nil {
		t.Fatalf("Decode(workspaces) returned error: %v", err)
	}
	if len(summaries.Items) != 1 {
		t.Fatalf("len(workspaces.items) = %d, want 1", len(summaries.Items))
	}
	if summaries.Items[0].FileCount != 3 {
		t.Fatalf("summary file_count = %d, want 3", summaries.Items[0].FileCount)
	}
	if summaries.Items[0].FolderCount != 2 {
		t.Fatalf("summary folder_count = %d, want 2", summaries.Items[0].FolderCount)
	}
	if summaries.Items[0].TotalBytes != expectedBytes {
		t.Fatalf("summary total_bytes = %d, want %d", summaries.Items[0].TotalBytes, expectedBytes)
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo")
	if err != nil {
		t.Fatalf("GET scoped workspace detail returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET scoped workspace detail status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var detail workspaceDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("Decode(workspace detail) returned error: %v", err)
	}
	if detail.FileCount != 3 {
		t.Fatalf("detail file_count = %d, want 3", detail.FileCount)
	}
	if detail.FolderCount != 2 {
		t.Fatalf("detail folder_count = %d, want 2", detail.FolderCount)
	}
	if detail.TotalBytes != expectedBytes {
		t.Fatalf("detail total_bytes = %d, want %d", detail.TotalBytes, expectedBytes)
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/tree?view=working-copy&path=/&depth=2")
	if err != nil {
		t.Fatalf("GET working-copy tree returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET working-copy tree status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var tree treeResponse
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		t.Fatalf("Decode(working-copy tree) returned error: %v", err)
	}
	if tree.View != "working-copy" {
		t.Fatalf("working-copy tree view = %q, want %q", tree.View, "working-copy")
	}

	paths := make(map[string]treeItem, len(tree.Items))
	for _, item := range tree.Items {
		paths[item.Path] = item
	}
	for _, want := range []string{"/README.md", "/src", "/src/main.go", "/drafts", "/drafts/notes.txt"} {
		if _, ok := paths[want]; !ok {
			t.Fatalf("working-copy tree missing %q: %#v", want, tree.Items)
		}
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/files/content?view=working-copy&path=/drafts/notes.txt")
	if err != nil {
		t.Fatalf("GET working-copy file content returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET working-copy file content status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var file fileContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		t.Fatalf("Decode(working-copy file content) returned error: %v", err)
	}
	if file.View != "working-copy" {
		t.Fatalf("working-copy file view = %q, want %q", file.View, "working-copy")
	}
	if file.Content != "working copy\n" {
		t.Fatalf("working-copy file content = %q, want %q", file.Content, "working copy\n")
	}
}

func TestHTTPCheckpointListSaveAndFork(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/checkpoints?limit=10")
	if err != nil {
		t.Fatalf("GET checkpoints returned error: %v", err)
	}
	defer resp.Body.Close()

	var checkpoints []checkpointSummary
	if err := json.NewDecoder(resp.Body).Decode(&checkpoints); err != nil {
		t.Fatalf("Decode(checkpoints) returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET checkpoints status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}
	if len(checkpoints) != 2 {
		t.Fatalf("len(checkpoints) = %d, want 2", len(checkpoints))
	}

	saveRequest := fmtJSON(t, saveCheckpointRequest{
		ExpectedHead: "snapshot",
		CheckpointID: "snapshot-2",
		Manifest: Manifest{
			Version:   formatVersion,
			Workspace: "repo",
			Savepoint: "snapshot-2",
			Entries: map[string]ManifestEntry{
				"/":            {Type: "dir", Mode: 0o755},
				"/README.md":   {Type: "file", Mode: 0o644, Size: int64(len("# demo\n")), Inline: base64.StdEncoding.EncodeToString([]byte("# demo\n"))},
				"/notes.txt":   {Type: "file", Mode: 0o644, Size: int64(len("phase-2\n")), Inline: base64.StdEncoding.EncodeToString([]byte("phase-2\n"))},
				"/src":         {Type: "dir", Mode: 0o755},
				"/src/main.go": {Type: "file", Mode: 0o644, Size: int64(len("package main\n")), Inline: base64.StdEncoding.EncodeToString([]byte("package main\n"))},
			},
		},
		FileCount:  3,
		DirCount:   1,
		TotalBytes: int64(len("# demo\n") + len("phase-2\n") + len("package main\n")),
	})

	resp, err = http.Post(
		server.URL+"/v1/databases/"+databaseID+"/workspaces/repo/checkpoints",
		"application/json",
		strings.NewReader(saveRequest),
	)
	if err != nil {
		t.Fatalf("POST checkpoint save returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST checkpoint save status = %d, want %d, body=%s", resp.StatusCode, http.StatusCreated, body)
	}

	var saveResponse saveCheckpointHTTPResponse
	if err := json.NewDecoder(resp.Body).Decode(&saveResponse); err != nil {
		t.Fatalf("Decode(save response) returned error: %v", err)
	}
	if !saveResponse.Saved {
		t.Fatal("expected checkpoint save response to report saved=true")
	}

	resp, err = http.Post(
		server.URL+"/v1/databases/"+databaseID+"/workspaces/repo:fork",
		"application/json",
		strings.NewReader(`{"new_workspace":"repo-copy"}`),
	)
	if err != nil {
		t.Fatalf("POST workspace fork returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST workspace fork status = %d, want %d, body=%s", resp.StatusCode, http.StatusNoContent, body)
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo-copy")
	if err != nil {
		t.Fatalf("GET forked workspace returned error: %v", err)
	}
	defer resp.Body.Close()

	var detail workspaceDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("Decode(forked workspace detail) returned error: %v", err)
	}
	if detail.HeadCheckpointID != "initial" {
		t.Fatalf("forked workspace head_checkpoint_id = %q, want %q", detail.HeadCheckpointID, "initial")
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo-copy/files/content?view=head&path=/notes.txt")
	if err != nil {
		t.Fatalf("GET forked workspace file content returned error: %v", err)
	}
	defer resp.Body.Close()

	var forkedFile fileContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&forkedFile); err != nil {
		t.Fatalf("Decode(forked file content) returned error: %v", err)
	}
	if forkedFile.Content != "phase-2\n" {
		t.Fatalf("forked file content = %q, want %q", forkedFile.Content, "phase-2\n")
	}
}

func TestHTTPClientWorkspaceSession(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Post(
		server.URL+"/v1/client/databases/"+databaseID+"/workspaces/repo/sessions",
		"application/json",
		strings.NewReader(`{"client_kind":"sync","hostname":"devbox","os":"darwin","local_path":"/tmp/repo"}`),
	)
	if err != nil {
		t.Fatalf("POST client workspace session returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST client workspace session status = %d, want %d, body=%s", resp.StatusCode, http.StatusCreated, body)
	}

	var session workspaceSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("Decode(session) returned error: %v", err)
	}
	if session.Workspace != "repo" {
		t.Fatalf("session workspace = %q, want %q", session.Workspace, "repo")
	}
	if session.SessionID == "" {
		t.Fatal("expected workspace session to include a session id")
	}
	if session.RedisKey != WorkspaceFSKey("repo") {
		t.Fatalf("session redis_key = %q, want %q", session.RedisKey, WorkspaceFSKey("repo"))
	}
	if session.Redis.RedisAddr == "" {
		t.Fatal("expected workspace session to include redis bootstrap info")
	}
	if session.HeartbeatIntervalSeconds == 0 {
		t.Fatal("expected workspace session to include heartbeat interval")
	}
}

func TestHTTPClientWorkspaceSessionHeartbeatAndClose(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Post(
		server.URL+"/v1/client/databases/"+databaseID+"/workspaces/repo/sessions",
		"application/json",
		strings.NewReader(`{"client_kind":"sync","hostname":"devbox","os":"darwin","local_path":"/tmp/repo"}`),
	)
	if err != nil {
		t.Fatalf("POST client workspace session returned error: %v", err)
	}
	defer resp.Body.Close()

	var session workspaceSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("Decode(session) returned error: %v", err)
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/sessions")
	if err != nil {
		t.Fatalf("GET workspace sessions returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET workspace sessions status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var sessions workspaceSessionListResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		t.Fatalf("Decode(session list) returned error: %v", err)
	}
	if len(sessions.Items) != 1 {
		t.Fatalf("len(session list) = %d, want 1", len(sessions.Items))
	}

	resp, err = http.Post(server.URL+"/v1/client/databases/"+databaseID+"/workspaces/repo/sessions/"+session.SessionID+"/heartbeat", "application/json", nil)
	if err != nil {
		t.Fatalf("POST heartbeat returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST heartbeat status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var heartbeat workspaceSessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&heartbeat); err != nil {
		t.Fatalf("Decode(heartbeat) returned error: %v", err)
	}
	if heartbeat.State != workspaceSessionStateActive {
		t.Fatalf("heartbeat state = %q, want %q", heartbeat.State, workspaceSessionStateActive)
	}

	req, err := http.NewRequest(http.MethodDelete, server.URL+"/v1/client/databases/"+databaseID+"/workspaces/repo/sessions/"+session.SessionID, nil)
	if err != nil {
		t.Fatalf("NewRequest(DELETE session) returned error: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE session returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("DELETE session status = %d, want %d, body=%s", resp.StatusCode, http.StatusNoContent, body)
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/sessions")
	if err != nil {
		t.Fatalf("GET workspace sessions after delete returned error: %v", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		t.Fatalf("Decode(session list after delete) returned error: %v", err)
	}
	if len(sessions.Items) != 0 {
		t.Fatalf("len(session list after delete) = %d, want 0", len(sessions.Items))
	}
}

func TestHTTPRouteSurfacesStaySeparated(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	admin := httptest.NewServer(NewAdminHandler(manager, "*"))
	defer admin.Close()
	client := httptest.NewServer(NewClientHandler(manager, "*"))
	defer client.Close()

	resp, err := http.Get(admin.URL + "/v1/databases")
	if err != nil {
		t.Fatalf("GET admin databases returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin /v1/databases status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	resp, err = http.Get(client.URL + "/v1/databases")
	if err != nil {
		t.Fatalf("GET client databases returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("client /v1/databases status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	resp, err = http.Get(client.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET client healthz returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("client /healthz status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	resp, err = http.Post(client.URL+"/databases/"+databaseID+"/workspaces/repo/sessions", "application/json", nil)
	if err != nil {
		t.Fatalf("POST client session returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("client session status = %d, want %d, body=%s", resp.StatusCode, http.StatusCreated, body)
	}
}

func TestHTTPDatabaseCRUDAndScopedWorkspaces(t *testing.T) {
	t.Helper()

	manager, primaryDatabaseID := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	secondaryRedis := miniredis.RunT(t)
	requestBody := fmtJSON(t, upsertDatabaseRequest{
		Name:        "secondary",
		Description: "Second test database",
		RedisAddr:   secondaryRedis.Addr(),
		RedisDB:     0,
	})

	resp, err := http.Post(server.URL+"/v1/databases", "application/json", strings.NewReader(requestBody))
	if err != nil {
		t.Fatalf("POST /v1/databases returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /v1/databases status = %d, want %d, body=%s", resp.StatusCode, http.StatusCreated, body)
	}

	var secondary databaseRecord
	if err := json.NewDecoder(resp.Body).Decode(&secondary); err != nil {
		t.Fatalf("Decode(database) returned error: %v", err)
	}
	if secondary.ID == "" {
		t.Fatal("expected database id to be assigned")
	}

	createWorkspaceBody := `{"name":"other-db-workspace","description":"debug","source":{"kind":"blank"}}`
	resp, err = http.Post(
		server.URL+"/v1/databases/"+secondary.ID+"/workspaces",
		"application/json",
		strings.NewReader(createWorkspaceBody),
	)
	if err != nil {
		t.Fatalf("POST scoped workspace create returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST scoped workspace create status = %d, want %d, body=%s", resp.StatusCode, http.StatusCreated, body)
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + primaryDatabaseID + "/workspaces")
	if err != nil {
		t.Fatalf("GET primary scoped workspaces returned error: %v", err)
	}
	defer resp.Body.Close()

	var primaryWorkspaces workspaceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&primaryWorkspaces); err != nil {
		t.Fatalf("Decode(primary workspaces) returned error: %v", err)
	}
	if len(primaryWorkspaces.Items) != 1 || primaryWorkspaces.Items[0].Name != "repo" {
		t.Fatalf("primary workspaces = %#v, want only repo", primaryWorkspaces.Items)
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + secondary.ID + "/workspaces")
	if err != nil {
		t.Fatalf("GET secondary scoped workspaces returned error: %v", err)
	}
	defer resp.Body.Close()

	var secondaryWorkspaces workspaceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&secondaryWorkspaces); err != nil {
		t.Fatalf("Decode(secondary workspaces) returned error: %v", err)
	}
	if len(secondaryWorkspaces.Items) != 1 || secondaryWorkspaces.Items[0].Name != "other-db-workspace" {
		t.Fatalf("secondary workspaces = %#v, want only other-db-workspace", secondaryWorkspaces.Items)
	}

	resp, err = http.Get(server.URL + "/v1/databases")
	if err != nil {
		t.Fatalf("GET /v1/databases returned error: %v", err)
	}
	defer resp.Body.Close()

	var databases databaseListResponse
	if err := json.NewDecoder(resp.Body).Decode(&databases); err != nil {
		t.Fatalf("Decode(databases) returned error: %v", err)
	}
	if len(databases.Items) != 2 {
		t.Fatalf("len(databases.items) = %d, want 2", len(databases.Items))
	}

	req, err := http.NewRequest(http.MethodDelete, server.URL+"/v1/databases/"+secondary.ID, nil)
	if err != nil {
		t.Fatalf("NewRequest(DELETE database) returned error: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /v1/databases/:id returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("DELETE /v1/databases/:id status = %d, want %d, body=%s", resp.StatusCode, http.StatusNoContent, body)
	}
}

func TestHTTPWorkspaceFirstRoutesResolveAcrossDatabases(t *testing.T) {
	t.Helper()

	manager, _ := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	secondaryRedis := miniredis.RunT(t)
	createDatabaseBody := fmtJSON(t, upsertDatabaseRequest{
		Name:      "secondary",
		RedisAddr: secondaryRedis.Addr(),
		RedisDB:   0,
	})

	resp, err := http.Post(server.URL+"/v1/databases", "application/json", strings.NewReader(createDatabaseBody))
	if err != nil {
		t.Fatalf("POST /v1/databases returned error: %v", err)
	}
	defer resp.Body.Close()

	var secondary databaseRecord
	if err := json.NewDecoder(resp.Body).Decode(&secondary); err != nil {
		t.Fatalf("Decode(database) returned error: %v", err)
	}

	resp, err = http.Post(
		server.URL+"/v1/databases/"+secondary.ID+"/workspaces",
		"application/json",
		strings.NewReader(`{"name":"repo-secondary","source":{"kind":"blank"}}`),
	)
	if err != nil {
		t.Fatalf("POST scoped workspace create returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST scoped workspace create status = %d, want %d, body=%s", resp.StatusCode, http.StatusCreated, body)
	}
	var created workspaceDetail
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("Decode(created workspace) returned error: %v", err)
	}
	if created.ID == "" || created.ID == created.Name {
		t.Fatalf("created workspace id = %q, want opaque id distinct from name %q", created.ID, created.Name)
	}

	resp, err = http.Get(server.URL + "/v1/workspaces")
	if err != nil {
		t.Fatalf("GET /v1/workspaces returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /v1/workspaces status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var workspaces workspaceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&workspaces); err != nil {
		t.Fatalf("Decode(workspaces) returned error: %v", err)
	}
	if len(workspaces.Items) != 2 {
		t.Fatalf("len(workspaces.items) = %d, want 2", len(workspaces.Items))
	}

	resp, err = http.Get(server.URL + "/v1/workspaces/" + created.ID)
	if err != nil {
		t.Fatalf("GET /v1/workspaces/:id returned error: %v", err)
	}
	defer resp.Body.Close()

	var detail workspaceDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("Decode(workspace detail) returned error: %v", err)
	}
	if detail.DatabaseID != secondary.ID {
		t.Fatalf("detail database_id = %q, want %q", detail.DatabaseID, secondary.ID)
	}
	if detail.Name != "repo-secondary" {
		t.Fatalf("detail name = %q, want %q", detail.Name, "repo-secondary")
	}

	resp, err = http.Post(
		server.URL+"/v1/client/workspaces/"+created.ID+"/sessions",
		"application/json",
		strings.NewReader(`{"client_kind":"sync","hostname":"devbox","os":"darwin","local_path":"/tmp/repo-secondary"}`),
	)
	if err != nil {
		t.Fatalf("POST workspace-first client session returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST workspace-first client session status = %d, want %d, body=%s", resp.StatusCode, http.StatusCreated, body)
	}

	var session workspaceSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("Decode(session) returned error: %v", err)
	}
	if session.DatabaseID != secondary.ID {
		t.Fatalf("session database_id = %q, want %q", session.DatabaseID, secondary.ID)
	}
	if session.Redis.RedisAddr != secondaryRedis.Addr() {
		t.Fatalf("session redis addr = %q, want %q", session.Redis.RedisAddr, secondaryRedis.Addr())
	}
}

func TestHTTPUnscopedWorkspaceCreateUsesDefaultDatabase(t *testing.T) {
	t.Helper()

	manager, primaryDatabaseID := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	secondaryRedis := miniredis.RunT(t)
	createDatabaseBody := fmtJSON(t, upsertDatabaseRequest{
		Name:      "secondary",
		RedisAddr: secondaryRedis.Addr(),
		RedisDB:   0,
	})

	resp, err := http.Post(server.URL+"/v1/databases", "application/json", strings.NewReader(createDatabaseBody))
	if err != nil {
		t.Fatalf("POST /v1/databases returned error: %v", err)
	}
	defer resp.Body.Close()

	var secondary databaseRecord
	if err := json.NewDecoder(resp.Body).Decode(&secondary); err != nil {
		t.Fatalf("Decode(database) returned error: %v", err)
	}
	if secondary.IsDefault {
		t.Fatal("new secondary database should not become the default automatically")
	}

	resp, err = http.Post(
		server.URL+"/v1/workspaces",
		"application/json",
		strings.NewReader(`{"name":"repo-default","source":{"kind":"blank"}}`),
	)
	if err != nil {
		t.Fatalf("POST /v1/workspaces returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /v1/workspaces status = %d, want %d, body=%s", resp.StatusCode, http.StatusCreated, body)
	}

	var created workspaceDetail
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("Decode(created workspace) returned error: %v", err)
	}
	if created.DatabaseID != primaryDatabaseID {
		t.Fatalf("created database_id = %q, want default %q", created.DatabaseID, primaryDatabaseID)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/databases/"+secondary.ID+"/default", nil)
	if err != nil {
		t.Fatalf("NewRequest(POST default) returned error: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/databases/:id/default returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /v1/databases/:id/default status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var updated databaseRecord
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("Decode(updated database) returned error: %v", err)
	}
	if !updated.IsDefault {
		t.Fatal("updated database should be marked default")
	}

	resp, err = http.Post(
		server.URL+"/v1/workspaces",
		"application/json",
		strings.NewReader(`{"name":"repo-secondary-default","source":{"kind":"blank"}}`),
	)
	if err != nil {
		t.Fatalf("POST /v1/workspaces after default switch returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /v1/workspaces after default switch status = %d, want %d, body=%s", resp.StatusCode, http.StatusCreated, body)
	}

	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("Decode(created workspace after default switch) returned error: %v", err)
	}
	if created.DatabaseID != secondary.ID {
		t.Fatalf("created database_id after default switch = %q, want %q", created.DatabaseID, secondary.ID)
	}
}

func TestHTTPWorkspaceFirstListSkipsDatabasesThatAreNoLongerReachable(t *testing.T) {
	t.Helper()

	manager, _ := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	secondaryRedis := miniredis.RunT(t)
	createDatabaseBody := fmtJSON(t, upsertDatabaseRequest{
		Name:      "secondary",
		RedisAddr: secondaryRedis.Addr(),
		RedisDB:   0,
	})

	resp, err := http.Post(server.URL+"/v1/databases", "application/json", strings.NewReader(createDatabaseBody))
	if err != nil {
		t.Fatalf("POST /v1/databases returned error: %v", err)
	}
	defer resp.Body.Close()

	var secondary databaseRecord
	if err := json.NewDecoder(resp.Body).Decode(&secondary); err != nil {
		t.Fatalf("Decode(database) returned error: %v", err)
	}

	resp, err = http.Post(
		server.URL+"/v1/databases/"+secondary.ID+"/workspaces",
		"application/json",
		strings.NewReader(`{"name":"repo-secondary","source":{"kind":"blank"}}`),
	)
	if err != nil {
		t.Fatalf("POST scoped workspace create returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST scoped workspace create status = %d, want %d, body=%s", resp.StatusCode, http.StatusCreated, body)
	}

	secondaryRedis.Close()

	resp, err = http.Get(server.URL + "/v1/workspaces")
	if err != nil {
		t.Fatalf("GET /v1/workspaces returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /v1/workspaces status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var workspaces workspaceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&workspaces); err != nil {
		t.Fatalf("Decode(workspaces) returned error: %v", err)
	}
	if len(workspaces.Items) != 1 {
		t.Fatalf("len(workspaces.items) = %d, want 1", len(workspaces.Items))
	}
	if workspaces.Items[0].DatabaseID == secondary.ID {
		t.Fatalf("stale workspace from unreachable database %q was still listed", secondary.ID)
	}
}

func TestHTTPWorkspaceFirstListRefreshesStaleCatalogEntriesAgainstLiveRedis(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/workspaces")
	if err != nil {
		t.Fatalf("GET /v1/workspaces returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("initial GET /v1/workspaces status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.DeleteWorkspace(context.Background(), "repo"); err != nil {
		t.Fatalf("DeleteWorkspace() returned error: %v", err)
	}

	resp, err = http.Get(server.URL + "/v1/workspaces")
	if err != nil {
		t.Fatalf("GET /v1/workspaces after live delete returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /v1/workspaces after live delete status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var workspaces workspaceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&workspaces); err != nil {
		t.Fatalf("Decode(workspaces after live delete) returned error: %v", err)
	}
	if len(workspaces.Items) != 0 {
		t.Fatalf("len(workspaces.items) after live delete = %d, want 0", len(workspaces.Items))
	}
}

func TestHTTPWorkspaceFirstRouteRejectsAmbiguousWorkspaceNames(t *testing.T) {
	t.Helper()

	manager, _ := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	secondaryRedis := miniredis.RunT(t)
	createDatabaseBody := fmtJSON(t, upsertDatabaseRequest{
		Name:      "secondary",
		RedisAddr: secondaryRedis.Addr(),
		RedisDB:   0,
	})

	resp, err := http.Post(server.URL+"/v1/databases", "application/json", strings.NewReader(createDatabaseBody))
	if err != nil {
		t.Fatalf("POST /v1/databases returned error: %v", err)
	}
	defer resp.Body.Close()

	var secondary databaseRecord
	if err := json.NewDecoder(resp.Body).Decode(&secondary); err != nil {
		t.Fatalf("Decode(database) returned error: %v", err)
	}

	resp, err = http.Post(
		server.URL+"/v1/databases/"+secondary.ID+"/workspaces",
		"application/json",
		strings.NewReader(`{"name":"repo","source":{"kind":"blank"}}`),
	)
	if err != nil {
		t.Fatalf("POST scoped workspace create returned error: %v", err)
	}
	defer resp.Body.Close()

	var created workspaceDetail
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("Decode(created workspace) returned error: %v", err)
	}

	resp, err = http.Get(server.URL + "/v1/workspaces/" + created.ID)
	if err != nil {
		t.Fatalf("GET /v1/workspaces/:id returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /v1/workspaces/:id status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	resp, err = http.Get(server.URL + "/v1/workspaces/repo")
	if err != nil {
		t.Fatalf("GET /v1/workspaces/repo returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /v1/workspaces/repo status = %d, want %d, body=%s", resp.StatusCode, http.StatusBadRequest, body)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "multiple databases") {
		t.Fatalf("GET /v1/workspaces/repo body = %q, want ambiguity guidance", string(body))
	}
}

func TestHTTPCatalogHealthAndReconcile(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/catalog/health")
	if err != nil {
		t.Fatalf("GET /v1/catalog/health returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /v1/catalog/health status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var health catalogHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("Decode(catalog health) returned error: %v", err)
	}
	if len(health.Items) != 1 {
		t.Fatalf("len(catalog health items) = %d, want 1", len(health.Items))
	}
	if health.Items[0].ID != databaseID {
		t.Fatalf("catalog health database id = %q, want %q", health.Items[0].ID, databaseID)
	}
	if health.Items[0].LastWorkspaceRefreshAt == "" {
		t.Fatal("expected last_workspace_refresh_at to be populated")
	}

	resp, err = http.Post(server.URL+"/v1/client/workspaces/"+health.Items[0].ID+"/sessions", "application/json", strings.NewReader(`{"client_kind":"sync","hostname":"devbox","local_path":"/tmp/repo"}`))
	if err == nil {
		resp.Body.Close()
	}

	resp, err = http.Post(server.URL+"/v1/client/databases/"+databaseID+"/workspaces/repo/sessions", "application/json", strings.NewReader(`{"client_kind":"sync","hostname":"devbox","local_path":"/tmp/repo"}`))
	if err != nil {
		t.Fatalf("POST client session returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST client session status = %d, want %d, body=%s", resp.StatusCode, http.StatusCreated, body)
	}

	resp, err = http.Post(server.URL+"/v1/catalog/reconcile", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /v1/catalog/reconcile returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /v1/catalog/reconcile status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("Decode(catalog reconcile response) returned error: %v", err)
	}
	if health.Items[0].LastSessionReconcileAt == "" {
		t.Fatal("expected last_session_reconcile_at to be populated")
	}
	if health.Items[0].ActiveSessionCount != 1 {
		t.Fatalf("active_session_count = %d, want 1", health.Items[0].ActiveSessionCount)
	}
}

func newTestManager(t *testing.T) (*DatabaseManager, string) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	cfg := Config{
		RedisConfig: RedisConfig{
			RedisAddr: mr.Addr(),
			RedisDB:   0,
		},
	}
	store := NewStore(rdb)
	ctx := context.Background()

	if err := createWorkspaceWithMetadata(ctx, cfg, store, "repo", workspaceCreateSpec{
		Description:  "Control plane demo workspace.",
		DatabaseID:   "db-demo",
		DatabaseName: "demo-db-us-test-1",
		CloudAccount: "Redis Cloud / Test",
		Region:       "us-test-1",
		Source:       sourceGitImport,
	}); err != nil {
		t.Fatalf("createWorkspaceWithMetadata() returned error: %v", err)
	}

	now := time.Now().UTC().Add(time.Second)
	readme := []byte("# demo\n")
	mainGo := []byte("package main\n")
	manifestValue := Manifest{
		Version:   formatVersion,
		Workspace: "repo",
		Savepoint: "snapshot",
		Entries: map[string]ManifestEntry{
			"/":            {Type: "dir", Mode: 0o755, MtimeMs: now.UnixMilli()},
			"/README.md":   {Type: "file", Mode: 0o644, MtimeMs: now.UnixMilli(), Size: int64(len(readme)), Inline: base64.StdEncoding.EncodeToString(readme)},
			"/src":         {Type: "dir", Mode: 0o755, MtimeMs: now.UnixMilli()},
			"/src/main.go": {Type: "file", Mode: 0o644, MtimeMs: now.UnixMilli(), Size: int64(len(mainGo)), Inline: base64.StdEncoding.EncodeToString(mainGo)},
		},
	}
	manifestHash, err := HashManifest(manifestValue)
	if err != nil {
		t.Fatalf("HashManifest() returned error: %v", err)
	}
	if err := store.PutSavepoint(ctx, SavepointMeta{
		Version:         formatVersion,
		ID:              "snapshot",
		Name:            "snapshot",
		Author:          "afs",
		Description:     "Snapshot workspace state.",
		Workspace:       "repo",
		ParentSavepoint: initialCheckpointName,
		ManifestHash:    manifestHash,
		CreatedAt:       now,
		FileCount:       2,
		DirCount:        1,
		TotalBytes:      int64(len(readme) + len(mainGo)),
	}, manifestValue); err != nil {
		t.Fatalf("PutSavepoint() returned error: %v", err)
	}
	if err := store.MoveWorkspaceHead(ctx, "repo", "snapshot", now); err != nil {
		t.Fatalf("MoveWorkspaceHead() returned error: %v", err)
	}

	dir := t.TempDir()
	configPath := filepath.Join(dir, "afs.config.json")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal(cfg) returned error: %v", err)
	}
	if err := os.WriteFile(configPath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile(config) returned error: %v", err)
	}

	manager, err := OpenDatabaseManager(configPath)
	if err != nil {
		t.Fatalf("OpenDatabaseManager() returned error: %v", err)
	}
	t.Cleanup(manager.Close)

	databaseID, databaseName := activeDatabaseIdentity(cfg)
	if _, err := manager.UpsertDatabase(ctx, databaseID, upsertDatabaseRequest{
		Name:      databaseName,
		RedisAddr: cfg.RedisAddr,
		RedisDB:   cfg.RedisDB,
	}); err != nil {
		t.Fatalf("UpsertDatabase() returned error: %v", err)
	}
	return manager, databaseID
}

func fmtJSON(t *testing.T, value any) string {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() returned error: %v", err)
	}
	return string(data)
}
