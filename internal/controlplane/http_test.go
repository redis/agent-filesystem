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
	"github.com/redis/go-redis/v9"
)

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
	if detail.Capabilities.BrowseWorkingCopy {
		t.Fatal("detail capabilities unexpectedly expose working-copy browsing")
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

func TestHTTPRejectsUnsupportedWorkingCopyView(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/tree?view=working-copy&path=/&depth=1")
	if err != nil {
		t.Fatalf("GET working-copy tree returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotImplemented {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET working-copy tree status = %d, want %d, body=%s", resp.StatusCode, http.StatusNotImplemented, body)
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

	databaseID, _ := activeDatabaseIdentity(cfg)
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
