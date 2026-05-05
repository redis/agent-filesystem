package controlplane

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
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
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST restore status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}
	var restoreResult RestoreCheckpointResult
	if err := json.NewDecoder(resp.Body).Decode(&restoreResult); err != nil {
		t.Fatalf("Decode(restore result) returned error: %v", err)
	}
	if !restoreResult.Restored || restoreResult.CheckpointID != "initial" {
		t.Fatalf("restore result = %#v, want restored initial", restoreResult)
	}
	if restoreResult.SafetyCheckpointCreated || restoreResult.SafetyCheckpointID != "" {
		t.Fatalf("restore result safety checkpoint = %#v, want none for clean head restore", restoreResult)
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

func TestHTTPRestoreCreatesSafetyCheckpointForDirtyWorkingCopy(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	ctx := context.Background()
	service, _, err := manager.serviceFor(ctx, databaseID)
	if err != nil {
		t.Fatalf("manager.serviceFor() returned error: %v", err)
	}
	redisKey, _, _, err := EnsureWorkspaceRoot(ctx, service.store, "repo")
	if err != nil {
		t.Fatalf("EnsureWorkspaceRoot() returned error: %v", err)
	}
	if err := mountclient.New(service.store.rdb, redisKey).Echo(ctx, "/drafts/notes.txt", []byte("working copy\n")); err != nil {
		t.Fatalf("Echo() returned error: %v", err)
	}
	if err := MarkWorkspaceRootDirty(ctx, service.store, "repo"); err != nil {
		t.Fatalf("MarkWorkspaceRootDirty() returned error: %v", err)
	}

	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Post(
		server.URL+"/v1/databases/"+databaseID+"/workspaces/repo:restore",
		"application/json",
		strings.NewReader(`{"checkpoint_id":"initial"}`),
	)
	if err != nil {
		t.Fatalf("POST scoped restore returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST restore status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var result RestoreCheckpointResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Decode(restore result) returned error: %v", err)
	}
	if !result.SafetyCheckpointCreated || !strings.HasPrefix(result.SafetyCheckpointID, "restore-safety-") {
		t.Fatalf("restore result safety checkpoint = %#v, want restore-safety checkpoint", result)
	}

	meta, err := service.store.GetSavepointMeta(ctx, "repo", result.SafetyCheckpointID)
	if err != nil {
		t.Fatalf("GetSavepointMeta(%s) returned error: %v", result.SafetyCheckpointID, err)
	}
	if meta.Kind != CheckpointKindSafety {
		t.Fatalf("safety checkpoint kind = %q, want %q", meta.Kind, CheckpointKindSafety)
	}
	if meta.Source != CheckpointSourceServer {
		t.Fatalf("safety checkpoint source = %q, want %q", meta.Source, CheckpointSourceServer)
	}
	if !strings.Contains(meta.Description, "before restoring initial") {
		t.Fatalf("safety checkpoint description = %q, want restore target", meta.Description)
	}

	manifestValue, err := service.store.GetManifest(ctx, "repo", result.SafetyCheckpointID)
	if err != nil {
		t.Fatalf("GetManifest(%s) returned error: %v", result.SafetyCheckpointID, err)
	}
	if _, ok := manifestValue.Entries["/drafts/notes.txt"]; !ok {
		t.Fatalf("safety checkpoint manifest missing draft file: %#v", manifestValue.Entries)
	}
}

func TestHTTPCheckpointDiff(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/diff?base=checkpoint:initial&head=checkpoint:snapshot")
	if err != nil {
		t.Fatalf("GET checkpoint diff returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET checkpoint diff status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var diff WorkspaceDiffResponse
	if err := json.NewDecoder(resp.Body).Decode(&diff); err != nil {
		t.Fatalf("Decode(diff) returned error: %v", err)
	}
	if diff.Base.CheckpointID != "initial" || diff.Head.CheckpointID != "snapshot" {
		t.Fatalf("diff refs = base %q head %q, want initial -> snapshot", diff.Base.CheckpointID, diff.Head.CheckpointID)
	}
	if diff.Summary.Total != 3 || diff.Summary.Created != 3 {
		t.Fatalf("diff summary = %+v, want 3 created", diff.Summary)
	}
	paths := make(map[string]string, len(diff.Entries))
	entriesByPath := make(map[string]DiffEntry, len(diff.Entries))
	for _, entry := range diff.Entries {
		paths[entry.Path] = entry.Op
		entriesByPath[entry.Path] = entry
	}
	for _, path := range []string{"/README.md", "/src", "/src/main.go"} {
		if paths[path] != DiffOpCreate {
			t.Fatalf("diff entry %s op = %q, want create; entries=%#v", path, paths[path], diff.Entries)
		}
	}
	readmeDiff := entriesByPath["/README.md"].TextDiff
	if readmeDiff == nil || !readmeDiff.Available || len(readmeDiff.Hunks) == 0 {
		t.Fatalf("README text diff = %#v, want available hunks", readmeDiff)
	}
	foundInsert := false
	for _, line := range readmeDiff.Hunks[0].Lines {
		if line.Kind == "insert" && line.Text == "# demo" {
			foundInsert = true
			break
		}
	}
	if !foundInsert {
		t.Fatalf("README text diff lines = %#v, want inserted # demo line", readmeDiff.Hunks[0].Lines)
	}
}

func TestHTTPCheckpointDetailAndEvents(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/checkpoints/snapshot")
	if err != nil {
		t.Fatalf("GET checkpoint detail returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET checkpoint detail status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}
	var detail checkpointDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("Decode(checkpoint detail) returned error: %v", err)
	}
	if detail.ID != "snapshot" || detail.Description != "Snapshot workspace state." {
		t.Fatalf("checkpoint detail = %+v, want snapshot with description", detail)
	}
	if detail.ParentCheckpointID != initialCheckpointName || detail.Parent == nil || detail.Parent.ID != initialCheckpointName {
		t.Fatalf("checkpoint parent = id %q parent %#v, want initial", detail.ParentCheckpointID, detail.Parent)
	}
	if detail.ChangeSummary.Total != 3 || detail.ChangeSummary.Created != 3 {
		t.Fatalf("checkpoint change summary = %+v, want 3 created", detail.ChangeSummary)
	}

	ctx := context.Background()
	service, _, err := manager.serviceFor(ctx, databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	manifestValue, err := service.store.GetManifest(ctx, "repo", "snapshot")
	if err != nil {
		t.Fatalf("GetManifest(snapshot) returned error: %v", err)
	}
	updatedReadme := []byte("# demo\nupdated\n")
	manifestValue.Savepoint = "event-snapshot"
	manifestValue.Entries["/README.md"] = ManifestEntry{
		Type:    "file",
		Mode:    0o644,
		Size:    int64(len(updatedReadme)),
		Inline:  base64.StdEncoding.EncodeToString(updatedReadme),
		MtimeMs: time.Now().UTC().UnixMilli(),
	}
	saved, err := manager.SaveCheckpoint(ctx, databaseID, "repo", SaveCheckpointRequest{
		Workspace:    "repo",
		ExpectedHead: "snapshot",
		CheckpointID: "event-snapshot",
		Description:  "Event test checkpoint.",
		Manifest:     manifestValue,
		FileCount:    2,
		DirCount:     1,
		TotalBytes:   int64(len(updatedReadme) + len("package main\n")),
	})
	if err != nil {
		t.Fatalf("SaveCheckpoint(event-snapshot) returned error: %v", err)
	}
	if !saved {
		t.Fatalf("SaveCheckpoint(event-snapshot) saved = false, want true")
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/events?kind=checkpoint&direction=desc")
	if err != nil {
		t.Fatalf("GET checkpoint events returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET checkpoint events status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}
	var checkpointEvents EventListResponse
	if err := json.NewDecoder(resp.Body).Decode(&checkpointEvents); err != nil {
		t.Fatalf("Decode(checkpoint events) returned error: %v", err)
	}
	if len(checkpointEvents.Items) == 0 || checkpointEvents.Items[0].CheckpointID != "event-snapshot" || checkpointEvents.Items[0].Op != "save" {
		t.Fatalf("checkpoint events = %#v, want save event for event-snapshot", checkpointEvents.Items)
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/events?kind=file&path=/README.md")
	if err != nil {
		t.Fatalf("GET file events returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET file events status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}
	var fileEvents EventListResponse
	if err := json.NewDecoder(resp.Body).Decode(&fileEvents); err != nil {
		t.Fatalf("Decode(file events) returned error: %v", err)
	}
	if len(fileEvents.Items) == 0 || fileEvents.Items[0].CheckpointID != "event-snapshot" || fileEvents.Items[0].Path != "/README.md" {
		t.Fatalf("file events = %#v, want README event for event-snapshot", fileEvents.Items)
	}
}

func TestHTTPWorkspaceVersioningPolicyRoutes(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/versioning")
	if err != nil {
		t.Fatalf("GET scoped workspace versioning returned error: %v", err)
	}
	defer resp.Body.Close()

	var scoped WorkspaceVersioningPolicy
	if err := json.NewDecoder(resp.Body).Decode(&scoped); err != nil {
		t.Fatalf("Decode(scoped versioning) returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET scoped workspace versioning status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if scoped.Mode != WorkspaceVersioningModeOff {
		t.Fatalf("scoped mode = %q, want %q", scoped.Mode, WorkspaceVersioningModeOff)
	}

	payload := `{"mode":"paths","include_globs":["src/**","docs/**"],"exclude_globs":["**/*.log"],"max_versions_per_file":7,"max_age_days":30,"max_total_bytes":2048,"large_file_cutoff_bytes":4096}`
	req, err := http.NewRequest(http.MethodPut, server.URL+"/v1/databases/"+databaseID+"/workspaces/repo/versioning", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("NewRequest(PUT scoped versioning) returned error: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT scoped workspace versioning returned error: %v", err)
	}
	defer resp.Body.Close()

	var updated WorkspaceVersioningPolicy
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("Decode(updated scoped versioning) returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT scoped workspace versioning status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if updated.Mode != WorkspaceVersioningModePaths {
		t.Fatalf("updated mode = %q, want %q", updated.Mode, WorkspaceVersioningModePaths)
	}
	if len(updated.IncludeGlobs) != 2 || updated.IncludeGlobs[0] != "src/**" || updated.IncludeGlobs[1] != "docs/**" {
		t.Fatalf("updated include_globs = %#v, want [src/** docs/**]", updated.IncludeGlobs)
	}
	if len(updated.ExcludeGlobs) != 1 || updated.ExcludeGlobs[0] != "**/*.log" {
		t.Fatalf("updated exclude_globs = %#v, want [**/*.log]", updated.ExcludeGlobs)
	}

	resp, err = http.Get(server.URL + "/v1/workspaces/repo/versioning")
	if err != nil {
		t.Fatalf("GET resolved workspace versioning returned error: %v", err)
	}
	defer resp.Body.Close()

	var resolved WorkspaceVersioningPolicy
	if err := json.NewDecoder(resp.Body).Decode(&resolved); err != nil {
		t.Fatalf("Decode(resolved versioning) returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET resolved workspace versioning status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if !reflect.DeepEqual(resolved, updated) {
		t.Fatalf("resolved policy = %+v, want %+v", resolved, updated)
	}
}

func TestHTTPFileHistoryAndVersionContentRoutes(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.PutWorkspaceVersioningPolicy(context.Background(), "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy() returned error: %v", err)
	}
	version, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/notes/http.txt"}, VersionedFileSnapshot{
		Path:    "/notes/http.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("http history\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation() returned error: %v", err)
	}
	secondVersion, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{
		Path:    "/notes/http.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("http history\n"),
	}, VersionedFileSnapshot{
		Path:    "/notes/http.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("http history v2\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(second version) returned error: %v", err)
	}
	fsKey, _, _, err := EnsureWorkspaceRoot(context.Background(), service.store, "repo")
	if err != nil {
		t.Fatalf("EnsureWorkspaceRoot() returned error: %v", err)
	}
	if err := mountclient.New(service.store.rdb, fsKey).EchoCreate(context.Background(), "/notes/http.txt", []byte("http working copy\n"), 0o644); err != nil {
		t.Fatalf("EchoCreate() returned error: %v", err)
	}

	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/files/history?path=/notes/http.txt&direction=asc&limit=1")
	if err != nil {
		t.Fatalf("GET scoped file history returned error: %v", err)
	}
	defer resp.Body.Close()

	var history FileHistoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
		t.Fatalf("Decode(file history) returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET scoped file history status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if history.Order != "asc" {
		t.Fatalf("history.Order = %q, want asc", history.Order)
	}
	if len(history.Lineages) != 1 || len(history.Lineages[0].Versions) != 1 {
		t.Fatalf("history lineages = %#v, want one lineage with one version", history.Lineages)
	}
	if history.Lineages[0].Versions[0].VersionID != version.VersionID {
		t.Fatalf("first page version_id = %q, want %q", history.Lineages[0].Versions[0].VersionID, version.VersionID)
	}
	if history.NextCursor == "" {
		t.Fatal("history.NextCursor = empty, want cursor for second page")
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/files/history?path=/notes/http.txt&direction=asc&limit=1&cursor=" + url.QueryEscape(history.NextCursor))
	if err != nil {
		t.Fatalf("GET scoped file history second page returned error: %v", err)
	}
	defer resp.Body.Close()

	var historyPage2 FileHistoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&historyPage2); err != nil {
		t.Fatalf("Decode(file history page 2) returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET scoped file history second page status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if len(historyPage2.Lineages) != 1 || len(historyPage2.Lineages[0].Versions) != 1 {
		t.Fatalf("historyPage2 lineages = %#v, want one lineage with one version", historyPage2.Lineages)
	}
	if historyPage2.Lineages[0].Versions[0].VersionID != secondVersion.VersionID {
		t.Fatalf("second page version_id = %q, want %q", historyPage2.Lineages[0].Versions[0].VersionID, secondVersion.VersionID)
	}
	if historyPage2.NextCursor != "" {
		t.Fatalf("historyPage2.NextCursor = %q, want empty", historyPage2.NextCursor)
	}

	resp, err = http.Get(server.URL + "/v1/workspaces/repo/files/version-content?version_id=" + version.VersionID)
	if err != nil {
		t.Fatalf("GET resolved file version content returned error: %v", err)
	}
	defer resp.Body.Close()

	var content FileVersionContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&content); err != nil {
		t.Fatalf("Decode(file version content) returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET resolved file version content status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if content.Content != "http history\n" {
		t.Fatalf("content.Content = %q, want %q", content.Content, "http history\n")
	}

	resp, err = http.Get(server.URL + "/v1/workspaces/repo/files/version-content?file_id=" + version.FileID + "&ordinal=1")
	if err != nil {
		t.Fatalf("GET resolved file version content by ordinal returned error: %v", err)
	}
	defer resp.Body.Close()

	var ordinalContent FileVersionContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&ordinalContent); err != nil {
		t.Fatalf("Decode(file version content by ordinal) returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET resolved file version content by ordinal status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ordinalContent.VersionID != version.VersionID {
		t.Fatalf("ordinal content version_id = %q, want %q", ordinalContent.VersionID, version.VersionID)
	}

	diffReq := diffFileVersionsRequest{
		Path: "/notes/http.txt",
		From: FileVersionDiffOperand{VersionID: version.VersionID},
		To:   FileVersionDiffOperand{Ref: "working-copy"},
	}
	body, err := json.Marshal(diffReq)
	if err != nil {
		t.Fatalf("Marshal(diffReq) returned error: %v", err)
	}
	resp, err = http.Post(server.URL+"/v1/workspaces/repo/files/diff", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST file diff returned error: %v", err)
	}
	defer resp.Body.Close()

	var diff FileVersionDiffResponse
	if err := json.NewDecoder(resp.Body).Decode(&diff); err != nil {
		t.Fatalf("Decode(file diff) returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST file diff status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if diff.Binary {
		t.Fatal("diff.Binary = true, want false")
	}
	if !strings.Contains(diff.Diff, "http history\n") || !strings.Contains(diff.Diff, "http working copy\n") {
		t.Fatalf("diff.Diff = %q, want both historical and working-copy content", diff.Diff)
	}

	restoreReq := restoreFileVersionRequest{
		Path:      "/notes/http.txt",
		VersionID: version.VersionID,
	}
	body, err = json.Marshal(restoreReq)
	if err != nil {
		t.Fatalf("Marshal(restoreReq) returned error: %v", err)
	}
	resp, err = http.Post(server.URL+"/v1/workspaces/repo:restore-version", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST restore version returned error: %v", err)
	}
	defer resp.Body.Close()
	var restored FileVersionRestoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&restored); err != nil {
		t.Fatalf("Decode(restore response) returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST restore version status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if restored.RestoredFromVersionID != version.VersionID {
		t.Fatalf("restored.RestoredFromVersionID = %q, want %q", restored.RestoredFromVersionID, version.VersionID)
	}

	deletedVersion, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/notes/deleted-http.txt"}, VersionedFileSnapshot{
		Path:    "/notes/deleted-http.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("deleted http\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(undelete create) returned error: %v", err)
	}
	if _, err := service.store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{
		Path:    "/notes/deleted-http.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("deleted http\n"),
	}, VersionedFileSnapshot{
		Path: "/notes/deleted-http.txt",
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP}); err != nil {
		t.Fatalf("RecordFileVersionMutation(undelete delete) returned error: %v", err)
	}

	undeleteReq := undeleteFileVersionRequest{Path: "/notes/deleted-http.txt"}
	body, err = json.Marshal(undeleteReq)
	if err != nil {
		t.Fatalf("Marshal(undeleteReq) returned error: %v", err)
	}
	resp, err = http.Post(server.URL+"/v1/workspaces/repo:undelete", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST undelete returned error: %v", err)
	}
	defer resp.Body.Close()
	var undeleted FileVersionUndeleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&undeleted); err != nil {
		t.Fatalf("Decode(undelete response) returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST undelete status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if undeleted.UndeletedFromVersionID != deletedVersion.VersionID {
		t.Fatalf("undeleted.UndeletedFromVersionID = %q, want %q", undeleted.UndeletedFromVersionID, deletedVersion.VersionID)
	}
}

func TestHTTPWorkspaceFirstChangeRoutesResolveOpaqueWorkspaceID(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	manager, databaseID := newTestManager(t)
	detail, err := manager.GetWorkspace(ctx, databaseID, "repo")
	if err != nil {
		t.Fatalf("GetWorkspace() returned error: %v", err)
	}
	if !strings.HasPrefix(detail.ID, "ws_") {
		t.Fatalf("detail.ID = %q, want opaque workspace id", detail.ID)
	}

	service, _, err := manager.serviceFor(ctx, databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}

	const (
		sessionID = "sess-workspace-first"
		path      = "/notes/opaque-route.txt"
	)
	WriteChangeEntries(ctx, service.store.rdb, detail.ID, []ChangeEntry{{
		SessionID:   sessionID,
		AgentID:     "agt-http",
		Op:          ChangeOpPut,
		Path:        path,
		DeltaBytes:  17,
		ContentHash: "blob-opaque",
		Source:      ChangeSourceMCP,
	}})

	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/workspaces/" + detail.ID + "/changes?limit=10")
	if err != nil {
		t.Fatalf("GET workspace-first changes returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET workspace-first changes status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var changes ChangelogListResponse
	if err := json.NewDecoder(resp.Body).Decode(&changes); err != nil {
		t.Fatalf("Decode(workspace-first changes) returned error: %v", err)
	}
	if len(changes.Entries) != 1 {
		t.Fatalf("len(workspace-first changes.entries) = %d, want 1", len(changes.Entries))
	}
	if changes.Entries[0].Path != path || changes.Entries[0].SessionID != sessionID {
		t.Fatalf("workspace-first change entry = %#v, want path/session %q/%q", changes.Entries[0], path, sessionID)
	}
	if changes.Entries[0].DatabaseID != databaseID {
		t.Fatalf("workspace-first change database_id = %q, want %q", changes.Entries[0].DatabaseID, databaseID)
	}

	resp, err = http.Get(server.URL + "/v1/workspaces/" + detail.ID + "/path-last?path=" + url.QueryEscape(path))
	if err != nil {
		t.Fatalf("GET workspace-first path-last returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET workspace-first path-last status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var last PathLastWriter
	if err := json.NewDecoder(resp.Body).Decode(&last); err != nil {
		t.Fatalf("Decode(workspace-first path-last) returned error: %v", err)
	}
	if last.Path != path || last.SessionID != sessionID || last.Op != ChangeOpPut || last.ContentHash != "blob-opaque" {
		t.Fatalf("workspace-first path-last = %#v, want path/session/op/hash %q/%q/%q/%q", last, path, sessionID, ChangeOpPut, "blob-opaque")
	}

	resp, err = http.Get(server.URL + "/v1/workspaces/" + detail.ID + "/sessions/" + sessionID + "/summary")
	if err != nil {
		t.Fatalf("GET workspace-first session summary returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET workspace-first session summary status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var summary SessionChangelogSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		t.Fatalf("Decode(workspace-first session summary) returned error: %v", err)
	}
	if summary.SessionID != sessionID {
		t.Fatalf("workspace-first session summary session_id = %q, want %q", summary.SessionID, sessionID)
	}
	if summary.OpCounts[ChangeOpPut] != 1 {
		t.Fatalf("workspace-first session summary put count = %d, want 1", summary.OpCounts[ChangeOpPut])
	}
	if summary.DeltaBytes != 17 {
		t.Fatalf("workspace-first session summary delta_bytes = %d, want 17", summary.DeltaBytes)
	}
}

func TestHTTPUpdateWorkspaceRenamesOpaqueWorkspace(t *testing.T) {
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
	if len(summaries.Items) != 1 {
		t.Fatalf("len(workspaces.items) = %d, want 1", len(summaries.Items))
	}
	workspaceID := summaries.Items[0].ID
	if !strings.HasPrefix(workspaceID, "ws_") {
		t.Fatalf("workspace id = %q, want opaque ws_* id", workspaceID)
	}

	req, err := http.NewRequest(
		http.MethodPut,
		server.URL+"/v1/databases/"+databaseID+"/workspaces/"+workspaceID,
		strings.NewReader(`{"name":"renamed-repo","description":"Renamed from settings","database_name":"demo-db-us-test-1","cloud_account":"Redis Cloud / Test","region":"us-test-1"}`),
	)
	if err != nil {
		t.Fatalf("NewRequest(PUT update workspace) returned error: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT scoped workspace returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT workspace status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, body)
	}

	var detail workspaceDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("Decode(updated workspace detail) returned error: %v", err)
	}
	if detail.ID != workspaceID {
		t.Fatalf("updated workspace id = %q, want %q", detail.ID, workspaceID)
	}
	if detail.Name != "renamed-repo" {
		t.Fatalf("updated workspace name = %q, want %q", detail.Name, "renamed-repo")
	}
	if detail.Description != "Renamed from settings" {
		t.Fatalf("updated workspace description = %q, want %q", detail.Description, "Renamed from settings")
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/" + workspaceID)
	if err != nil {
		t.Fatalf("GET renamed workspace by id returned error: %v", err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("Decode(renamed workspace by id) returned error: %v", err)
	}
	if detail.Name != "renamed-repo" {
		t.Fatalf("renamed workspace name by id = %q, want %q", detail.Name, "renamed-repo")
	}

	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/renamed-repo")
	if err != nil {
		t.Fatalf("GET renamed workspace by name returned error: %v", err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("Decode(renamed workspace by name) returned error: %v", err)
	}
	if detail.ID != workspaceID {
		t.Fatalf("renamed workspace id by name = %q, want %q", detail.ID, workspaceID)
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

func TestHostedMCPTokenFlowCreatesVisibleAgentSession(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	auth, err := NewAuthHandler(AuthConfig{
		Mode:              AuthModeTrustedHeader,
		TrustedUserHeader: "X-Forwarded-User",
		TrustedNameHeader: "X-Forwarded-Name",
	})
	if err != nil {
		t.Fatalf("NewAuthHandler() returned error: %v", err)
	}

	server := httptest.NewServer(NewHandlerWithOptions(manager, HandlerOptions{
		AllowOrigin: "*",
		Auth:        auth,
	}))
	defer server.Close()

	createReq, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/v1/workspaces/repo/mcp-tokens",
		strings.NewReader(`{"name":"Claude Desktop","readonly":false}`),
	)
	if err != nil {
		t.Fatalf("NewRequest(create token) returned error: %v", err)
	}
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-Forwarded-User", "alice@example.com")
	createReq.Header.Set("X-Forwarded-Name", "Alice")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("POST mcp token returned error: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("POST mcp token status = %d, want %d, body=%s", createResp.StatusCode, http.StatusCreated, body)
	}

	var token mcpAccessTokenResponse
	if err := json.NewDecoder(createResp.Body).Decode(&token); err != nil {
		t.Fatalf("Decode(mcp token) returned error: %v", err)
	}
	if token.Token == "" {
		t.Fatal("expected created mcp token secret")
	}

	toolsReq, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`),
	)
	if err != nil {
		t.Fatalf("NewRequest(mcp tools/list) returned error: %v", err)
	}
	toolsReq.Header.Set("Content-Type", "application/json")
	toolsReq.Header.Set("Authorization", "Bearer "+token.Token)
	toolsReq.Header.Set(AgentIDHeader, "agt_hosted")
	toolsReq.Header.Set("X-AFS-Hostname", "devbox")
	toolsReq.Header.Set("X-AFS-Client-Kind", "mcp")
	toolsResp, err := http.DefaultClient.Do(toolsReq)
	if err != nil {
		t.Fatalf("POST /mcp tools/list returned error: %v", err)
	}
	defer toolsResp.Body.Close()
	if toolsResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(toolsResp.Body)
		t.Fatalf("POST /mcp tools/list status = %d, want %d, body=%s", toolsResp.StatusCode, http.StatusOK, body)
	}

	var toolsPayload struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.NewDecoder(toolsResp.Body).Decode(&toolsPayload); err != nil {
		t.Fatalf("Decode(mcp tools/list) returned error: %v", err)
	}
	toolNames := make(map[string]struct{}, len(toolsPayload.Result.Tools))
	for _, tool := range toolsPayload.Result.Tools {
		toolNames[tool.Name] = struct{}{}
	}
	if _, ok := toolNames["file_patch"]; !ok {
		t.Fatalf("tools/list missing file_patch: %#v", toolsPayload.Result.Tools)
	}
	if _, ok := toolNames["file_grep"]; !ok {
		t.Fatalf("tools/list missing file_grep: %#v", toolsPayload.Result.Tools)
	}
	if _, ok := toolNames["workspace_current"]; ok {
		t.Fatalf("workspace_current should not be exposed to workspace-rw tokens: %#v", toolsPayload.Result.Tools)
	}

	callBody := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"file_write","arguments":{"path":"/notes/hosted.txt","content":"hello from hosted mcp\n"}}}`
	callReq, err := http.NewRequest(http.MethodPost, server.URL+"/mcp", strings.NewReader(callBody))
	if err != nil {
		t.Fatalf("NewRequest(mcp call) returned error: %v", err)
	}
	callReq.Header.Set("Content-Type", "application/json")
	callReq.Header.Set("Authorization", "Bearer "+token.Token)
	callReq.Header.Set(AgentIDHeader, "agt_hosted")
	callReq.Header.Set("X-AFS-Hostname", "devbox")
	callReq.Header.Set("X-AFS-Client-Kind", "mcp")
	callResp, err := http.DefaultClient.Do(callReq)
	if err != nil {
		t.Fatalf("POST /mcp returned error: %v", err)
	}
	defer callResp.Body.Close()
	if callResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(callResp.Body)
		t.Fatalf("POST /mcp status = %d, want %d, body=%s", callResp.StatusCode, http.StatusOK, body)
	}

	var mcpPayload struct {
		Result struct {
			StructuredContent map[string]any `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.NewDecoder(callResp.Body).Decode(&mcpPayload); err != nil {
		t.Fatalf("Decode(mcp response) returned error: %v", err)
	}
	if got := mcpPayload.Result.StructuredContent["operation"]; got != "write" {
		t.Fatalf("mcp file_write operation = %#v, want %#v", got, "write")
	}
	if got := mcpPayload.Result.StructuredContent["dirty"]; got != true {
		t.Fatalf("mcp file_write dirty = %#v, want true", got)
	}

	changesReq, err := http.NewRequest(http.MethodGet, server.URL+"/v1/databases/"+databaseID+"/workspaces/repo/changes?limit=10", nil)
	if err != nil {
		t.Fatalf("NewRequest(changes) returned error: %v", err)
	}
	changesReq.Header.Set("X-Forwarded-User", "alice@example.com")
	changesResp, err := http.DefaultClient.Do(changesReq)
	if err != nil {
		t.Fatalf("GET /v1/databases/%s/workspaces/repo/changes returned error: %v", databaseID, err)
	}
	defer changesResp.Body.Close()
	if changesResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(changesResp.Body)
		t.Fatalf("GET /v1/databases/%s/workspaces/repo/changes status = %d, want %d, body=%s", databaseID, changesResp.StatusCode, http.StatusOK, body)
	}

	var changes ChangelogListResponse
	if err := json.NewDecoder(changesResp.Body).Decode(&changes); err != nil {
		t.Fatalf("Decode(changes) returned error: %v", err)
	}
	foundChange := false
	for _, entry := range changes.Entries {
		if entry.Path == "/notes/hosted.txt" && entry.Source == ChangeSourceMCP {
			foundChange = true
			break
		}
	}
	if !foundChange {
		t.Fatalf("expected mcp changelog entry for /notes/hosted.txt, got %#v", changes.Entries)
	}

	agentsReq, err := http.NewRequest(http.MethodGet, server.URL+"/v1/agents", nil)
	if err != nil {
		t.Fatalf("NewRequest(agents) returned error: %v", err)
	}
	agentsReq.Header.Set("X-Forwarded-User", "alice@example.com")
	agentsResp, err := http.DefaultClient.Do(agentsReq)
	if err != nil {
		t.Fatalf("GET /v1/agents returned error: %v", err)
	}
	defer agentsResp.Body.Close()
	if agentsResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(agentsResp.Body)
		t.Fatalf("GET /v1/agents status = %d, want %d, body=%s", agentsResp.StatusCode, http.StatusOK, body)
	}

	var sessions workspaceSessionListResponse
	if err := json.NewDecoder(agentsResp.Body).Decode(&sessions); err != nil {
		t.Fatalf("Decode(agents) returned error: %v", err)
	}
	if len(sessions.Items) != 1 {
		t.Fatalf("len(agents.items) = %d, want 1", len(sessions.Items))
	}
	if sessions.Items[0].ClientKind != "mcp" {
		t.Fatalf("agent client_kind = %q, want %q", sessions.Items[0].ClientKind, "mcp")
	}
	if sessions.Items[0].AgentID != "agt_hosted" {
		t.Fatalf("agent agent_id = %q, want %q", sessions.Items[0].AgentID, "agt_hosted")
	}
	if sessions.Items[0].WorkspaceName != "repo" {
		t.Fatalf("agent workspace_name = %q, want %q", sessions.Items[0].WorkspaceName, "repo")
	}
	if sessions.Items[0].Hostname != "devbox" {
		t.Fatalf("agent hostname = %q, want %q", sessions.Items[0].Hostname, "devbox")
	}

	tokensReq, err := http.NewRequest(http.MethodGet, server.URL+"/v1/mcp-tokens", nil)
	if err != nil {
		t.Fatalf("NewRequest(mcp tokens) returned error: %v", err)
	}
	tokensReq.Header.Set("X-Forwarded-User", "alice@example.com")
	tokensResp, err := http.DefaultClient.Do(tokensReq)
	if err != nil {
		t.Fatalf("GET /v1/mcp-tokens returned error: %v", err)
	}
	defer tokensResp.Body.Close()
	if tokensResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(tokensResp.Body)
		t.Fatalf("GET /v1/mcp-tokens status = %d, want %d, body=%s", tokensResp.StatusCode, http.StatusOK, body)
	}

	var tokenList struct {
		Items []mcpAccessTokenResponse `json:"items"`
	}
	if err := json.NewDecoder(tokensResp.Body).Decode(&tokenList); err != nil {
		t.Fatalf("Decode(mcp tokens) returned error: %v", err)
	}
	if len(tokenList.Items) != 1 {
		t.Fatalf("len(mcp_tokens.items) = %d, want 1", len(tokenList.Items))
	}
	if tokenList.Items[0].ID != token.ID {
		t.Fatalf("mcp token id = %q, want %q", tokenList.Items[0].ID, token.ID)
	}
}

func TestMCPTokenFlowWorksWithoutAuth(t *testing.T) {
	t.Helper()

	manager, _ := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	createReq, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/v1/workspaces/repo/mcp-tokens",
		strings.NewReader(`{"name":"Local agent","profile":"workspace-rw"}`),
	)
	if err != nil {
		t.Fatalf("NewRequest(create token) returned error: %v", err)
	}
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("POST mcp token returned error: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("POST mcp token status = %d, want %d, body=%s", createResp.StatusCode, http.StatusCreated, body)
	}

	var token mcpAccessTokenResponse
	if err := json.NewDecoder(createResp.Body).Decode(&token); err != nil {
		t.Fatalf("Decode(mcp token) returned error: %v", err)
	}
	if token.Token == "" {
		t.Fatal("expected created mcp token secret")
	}
	if token.Profile != MCPProfileWorkspaceRW {
		t.Fatalf("token profile = %q, want %q", token.Profile, MCPProfileWorkspaceRW)
	}

	callBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`
	callReq, err := http.NewRequest(http.MethodPost, server.URL+"/mcp", strings.NewReader(callBody))
	if err != nil {
		t.Fatalf("NewRequest(mcp initialize) returned error: %v", err)
	}
	callReq.Header.Set("Content-Type", "application/json")
	callReq.Header.Set("Accept", "application/json, text/event-stream")
	callReq.Header.Set("Authorization", "Bearer "+token.Token)
	callResp, err := http.DefaultClient.Do(callReq)
	if err != nil {
		t.Fatalf("POST /mcp returned error: %v", err)
	}
	defer callResp.Body.Close()
	if callResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(callResp.Body)
		t.Fatalf("POST /mcp status = %d, want %d, body=%s", callResp.StatusCode, http.StatusOK, body)
	}
}

func TestHostedControlPlaneTokenCanIssueWorkspaceTokenWithoutAuth(t *testing.T) {
	t.Helper()

	manager, _ := newTestManager(t)
	server := httptest.NewServer(NewHandler(manager, "*"))
	defer server.Close()

	createReq, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/v1/mcp-tokens",
		strings.NewReader(`{"name":"Control plane token"}`),
	)
	if err != nil {
		t.Fatalf("NewRequest(create control-plane token) returned error: %v", err)
	}
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("POST control-plane mcp token returned error: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("POST control-plane mcp token status = %d, want %d, body=%s", createResp.StatusCode, http.StatusCreated, body)
	}

	var controlPlaneToken mcpAccessTokenResponse
	if err := json.NewDecoder(createResp.Body).Decode(&controlPlaneToken); err != nil {
		t.Fatalf("Decode(control-plane mcp token) returned error: %v", err)
	}
	if controlPlaneToken.Token == "" {
		t.Fatal("expected created control-plane mcp token secret")
	}
	if controlPlaneToken.Scope != mcpScopeControlPlane {
		t.Fatalf("control-plane token scope = %q, want %q", controlPlaneToken.Scope, mcpScopeControlPlane)
	}

	callBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"mcp_token_issue","arguments":{"workspace":"repo","name":"Mounted FS","profile":"workspace-rw"}}}`
	callReq, err := http.NewRequest(http.MethodPost, server.URL+"/mcp", strings.NewReader(callBody))
	if err != nil {
		t.Fatalf("NewRequest(mcp_token_issue) returned error: %v", err)
	}
	callReq.Header.Set("Content-Type", "application/json")
	callReq.Header.Set("Authorization", "Bearer "+controlPlaneToken.Token)
	callReq.Header.Set("Accept", "application/json, text/event-stream")
	callResp, err := http.DefaultClient.Do(callReq)
	if err != nil {
		t.Fatalf("POST /mcp mcp_token_issue returned error: %v", err)
	}
	defer callResp.Body.Close()
	if callResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(callResp.Body)
		t.Fatalf("POST /mcp mcp_token_issue status = %d, want %d, body=%s", callResp.StatusCode, http.StatusOK, body)
	}

	var issuePayload struct {
		Result struct {
			StructuredContent map[string]any `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.NewDecoder(callResp.Body).Decode(&issuePayload); err != nil {
		t.Fatalf("Decode(mcp_token_issue response) returned error: %v", err)
	}
	issuedToken, _ := issuePayload.Result.StructuredContent["token"].(string)
	if issuedToken == "" {
		t.Fatalf("mcp_token_issue token = %#v, want non-empty", issuePayload.Result.StructuredContent["token"])
	}
	if got := issuePayload.Result.StructuredContent["workspace"]; got != "repo" {
		t.Fatalf("mcp_token_issue workspace = %#v, want %q", got, "repo")
	}
	if got := issuePayload.Result.StructuredContent["profile"]; got != MCPProfileWorkspaceRW {
		t.Fatalf("mcp_token_issue profile = %#v, want %q", got, MCPProfileWorkspaceRW)
	}
	if got, _ := issuePayload.Result.StructuredContent["scope"].(string); !strings.HasPrefix(got, mcpScopeWorkspacePrefix) {
		t.Fatalf("mcp_token_issue scope = %#v, want workspace scope", issuePayload.Result.StructuredContent["scope"])
	}

	toolsReq, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`),
	)
	if err != nil {
		t.Fatalf("NewRequest(workspace-token tools/list) returned error: %v", err)
	}
	toolsReq.Header.Set("Content-Type", "application/json")
	toolsReq.Header.Set("Authorization", "Bearer "+issuedToken)
	toolsReq.Header.Set("Accept", "application/json, text/event-stream")
	toolsResp, err := http.DefaultClient.Do(toolsReq)
	if err != nil {
		t.Fatalf("POST /mcp workspace-token tools/list returned error: %v", err)
	}
	defer toolsResp.Body.Close()
	if toolsResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(toolsResp.Body)
		t.Fatalf("POST /mcp workspace-token tools/list status = %d, want %d, body=%s", toolsResp.StatusCode, http.StatusOK, body)
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
	redisKey, _, _, err := EnsureWorkspaceRoot(ctx, service.store, "repo")
	if err != nil {
		t.Fatalf("EnsureWorkspaceRoot() returned error: %v", err)
	}

	fsClient := mountclient.New(service.store.rdb, redisKey)
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
	if summaries.Items[0].DraftState != "dirty" {
		t.Fatalf("summary draft_state = %q, want dirty", summaries.Items[0].DraftState)
	}
	if summaries.Items[0].Status != "attention" {
		t.Fatalf("summary status = %q, want attention", summaries.Items[0].Status)
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
	if detail.DraftState != "dirty" {
		t.Fatalf("detail draft_state = %q, want dirty", detail.DraftState)
	}
	if detail.Status != "attention" {
		t.Fatalf("detail status = %q, want attention", detail.Status)
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
		Description:  "Second snapshot.",
		Kind:         CheckpointKindManual,
		Source:       CheckpointSourceMCP,
		Author:       "codex",
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
	resp, err = http.Get(server.URL + "/v1/databases/" + databaseID + "/workspaces/repo/checkpoints?limit=10")
	if err != nil {
		t.Fatalf("GET checkpoints after save returned error: %v", err)
	}
	defer resp.Body.Close()
	var checkpointsAfterSave []checkpointSummary
	if err := json.NewDecoder(resp.Body).Decode(&checkpointsAfterSave); err != nil {
		t.Fatalf("Decode(checkpoints after save) returned error: %v", err)
	}
	var latestCheckpoint checkpointSummary
	for _, checkpoint := range checkpointsAfterSave {
		if checkpoint.ID == "snapshot-2" {
			latestCheckpoint = checkpoint
			break
		}
	}
	if latestCheckpoint.ID == "" {
		t.Fatalf("checkpoint snapshot-2 not found: %#v", checkpointsAfterSave)
	}
	if latestCheckpoint.Note != "Second snapshot." {
		t.Errorf("checkpoint note = %q, want Second snapshot.", latestCheckpoint.Note)
	}
	if latestCheckpoint.Kind != CheckpointKindManual {
		t.Errorf("checkpoint kind = %q, want %q", latestCheckpoint.Kind, CheckpointKindManual)
	}
	if latestCheckpoint.Source != CheckpointSourceMCP {
		t.Errorf("checkpoint source = %q, want %q", latestCheckpoint.Source, CheckpointSourceMCP)
	}
	if latestCheckpoint.Author != "codex" {
		t.Errorf("checkpoint author = %q, want codex", latestCheckpoint.Author)
	}
	if latestCheckpoint.ParentCheckpointID != "snapshot" {
		t.Errorf("checkpoint parent = %q, want snapshot", latestCheckpoint.ParentCheckpointID)
	}
	if latestCheckpoint.ManifestHash == "" {
		t.Error("checkpoint manifest hash is empty")
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
		strings.NewReader(`{"agent_id":"agt_http","client_kind":"sync","hostname":"devbox","os":"darwin","local_path":"/tmp/repo"}`),
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
	if !strings.HasPrefix(session.Workspace, "ws_") {
		t.Fatalf("session workspace = %q, want opaque workspace id", session.Workspace)
	}
	if session.SessionID == "" {
		t.Fatal("expected workspace session to include a session id")
	}
	if session.RedisKey != WorkspaceFSKey(session.Workspace) {
		t.Fatalf("session redis_key = %q, want %q", session.RedisKey, WorkspaceFSKey(session.Workspace))
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
		strings.NewReader(`{"agent_id":"agt_http","client_kind":"sync","hostname":"devbox","os":"darwin","local_path":"/tmp/repo"}`),
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
	if sessions.Items[0].AgentID != "agt_http" {
		t.Fatalf("listed agent_id = %q, want %q", sessions.Items[0].AgentID, "agt_http")
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

func TestHTTPWorkspaceFirstListFallsBackToCatalogWhenDatabaseBecomesUnreachable(t *testing.T) {
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
	if len(workspaces.Items) != 2 {
		t.Fatalf("len(workspaces.items) = %d, want 2", len(workspaces.Items))
	}
	foundSecondary := false
	for _, item := range workspaces.Items {
		if item.DatabaseID == secondary.ID {
			foundSecondary = true
			break
		}
	}
	if !foundSecondary {
		t.Fatalf("expected cached workspace from unreachable database %q to remain listed", secondary.ID)
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
	bodyText := string(body)
	if !strings.Contains(bodyText, "control plane workspace is ambiguous") {
		t.Fatalf("GET /v1/workspaces/repo body = %q, want ambiguity guidance", bodyText)
	}
	if strings.Contains(bodyText, created.ID) || strings.Contains(bodyText, secondary.ID) || strings.Contains(bodyText, secondary.Name) {
		t.Fatalf("GET /v1/workspaces/repo body = %q, leaked workspace or database identifiers", bodyText)
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
