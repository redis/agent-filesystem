package controlplane

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestHostedMCPFileCreateExclusiveCreatesFile(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	provider := &hostedMCPProvider{
		manager:    manager,
		databaseID: databaseID,
		workspace:  "repo",
	}

	result := provider.CallTool(context.Background(), "file_create_exclusive", map[string]any{
		"path":    "/locks/deploy.lock",
		"content": "agent-1\n",
	})
	if result.IsError {
		t.Fatalf("file_create_exclusive returned error result: %+v", result)
	}

	var payload map[string]any
	if err := decodeHostedStructuredContent(result.StructuredContent, &payload); err != nil {
		t.Fatalf("decodeHostedStructuredContent(create) returned error: %v", err)
	}
	if op, _ := payload["operation"].(string); op != "create_exclusive" {
		t.Fatalf("operation = %#v, want %q", payload["operation"], "create_exclusive")
	}
	if created, _ := payload["created"].(bool); !created {
		t.Fatalf("created = %#v, want true", payload["created"])
	}

	readResult := provider.CallTool(context.Background(), "file_read", map[string]any{
		"path": "/locks/deploy.lock",
	})
	if readResult.IsError {
		t.Fatalf("file_read returned error result: %+v", readResult)
	}

	var readPayload map[string]any
	if err := decodeHostedStructuredContent(readResult.StructuredContent, &readPayload); err != nil {
		t.Fatalf("decodeHostedStructuredContent(read) returned error: %v", err)
	}
	if got := readPayload["content"]; got != "agent-1\n" {
		t.Fatalf("file_read content = %#v, want written content", got)
	}
}

func TestHostedMCPFileCreateExclusiveFailsWhenFileExists(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	provider := &hostedMCPProvider{
		manager:    manager,
		databaseID: databaseID,
		workspace:  "repo",
	}

	first := provider.CallTool(context.Background(), "file_create_exclusive", map[string]any{
		"path":    "/locks/deploy.lock",
		"content": "agent-1\n",
	})
	if first.IsError {
		t.Fatalf("first file_create_exclusive returned error result: %+v", first)
	}

	second := provider.CallTool(context.Background(), "file_create_exclusive", map[string]any{
		"path":    "/locks/deploy.lock",
		"content": "agent-2\n",
	})
	if !second.IsError {
		t.Fatal("second file_create_exclusive should fail, got success")
	}
}

func TestHostedMCPCheckpointCreateAllowsUnchangedWorkspace(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	provider := &hostedMCPProvider{
		manager:    manager,
		databaseID: databaseID,
		workspace:  "repo",
		profile:    MCPProfileWorkspaceRWCheckpoint,
	}

	checkpointResult := provider.CallTool(context.Background(), "checkpoint_create", map[string]any{
		"checkpoint": "unchanged-head",
	})
	if checkpointResult.IsError {
		t.Fatalf("checkpoint_create on unchanged workspace returned error result: %+v", checkpointResult)
	}

	var checkpointPayload map[string]any
	if err := decodeHostedStructuredContent(checkpointResult.StructuredContent, &checkpointPayload); err != nil {
		t.Fatalf("decodeHostedStructuredContent(checkpoint unchanged) returned error: %v", err)
	}
	if created, _ := checkpointPayload["created"].(bool); !created {
		t.Fatalf("checkpoint_create created = %#v, want true", checkpointPayload["created"])
	}
	if checkpoint, _ := checkpointPayload["checkpoint"].(string); checkpoint != "unchanged-head" {
		t.Fatalf("checkpoint_create checkpoint = %#v, want %q", checkpointPayload["checkpoint"], "unchanged-head")
	}

	restoreResult := provider.CallTool(context.Background(), "checkpoint_restore", map[string]any{
		"checkpoint": "unchanged-head",
	})
	if restoreResult.IsError {
		t.Fatalf("checkpoint_restore after unchanged create returned error result: %+v", restoreResult)
	}
}

func TestHostedMCPWorkspaceVersioningPolicyToolsRoundTrip(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	provider := &hostedMCPProvider{
		manager:    manager,
		databaseID: databaseID,
		workspace:  "repo",
		profile:    MCPProfileWorkspaceRW,
	}

	getResult := provider.CallTool(context.Background(), "workspace_get_versioning_policy", map[string]any{})
	if getResult.IsError {
		t.Fatalf("workspace_get_versioning_policy returned error result: %+v", getResult)
	}
	var initial map[string]any
	if err := decodeHostedStructuredContent(getResult.StructuredContent, &initial); err != nil {
		t.Fatalf("decodeHostedStructuredContent(get) returned error: %v", err)
	}
	initialPolicy, ok := initial["policy"].(map[string]any)
	if !ok {
		t.Fatalf("initial policy = %#v, want map", initial["policy"])
	}
	if got := initialPolicy["mode"]; got != WorkspaceVersioningModeOff {
		t.Fatalf("initial policy.mode = %#v, want %q", got, WorkspaceVersioningModeOff)
	}

	setResult := provider.CallTool(context.Background(), "workspace_set_versioning_policy", map[string]any{
		"mode":                  WorkspaceVersioningModeAll,
		"exclude_globs":         []any{"**/*.tmp"},
		"max_versions_per_file": 5,
	})
	if setResult.IsError {
		t.Fatalf("workspace_set_versioning_policy returned error result: %+v", setResult)
	}

	var updated map[string]any
	if err := decodeHostedStructuredContent(setResult.StructuredContent, &updated); err != nil {
		t.Fatalf("decodeHostedStructuredContent(set) returned error: %v", err)
	}
	updatedPolicy, ok := updated["policy"].(map[string]any)
	if !ok {
		t.Fatalf("updated policy = %#v, want map", updated["policy"])
	}
	if got := updatedPolicy["mode"]; got != WorkspaceVersioningModeAll {
		t.Fatalf("updated policy.mode = %#v, want %q", got, WorkspaceVersioningModeAll)
	}
	if got := updatedPolicy["max_versions_per_file"]; got != float64(5) {
		t.Fatalf("updated policy.max_versions_per_file = %#v, want 5", got)
	}
}

func TestHostedControlPlaneWorkspaceVersioningPolicyToolsRoundTrip(t *testing.T) {
	t.Helper()

	manager, _ := newTestManager(t)
	provider := &hostedMCPProvider{
		manager: manager,
		scope:   mcpScopeControlPlane,
	}

	setResult := provider.CallTool(context.Background(), "workspace_set_versioning_policy", map[string]any{
		"workspace":             "repo",
		"mode":                  WorkspaceVersioningModePaths,
		"include_globs":         []any{"src/**"},
		"max_versions_per_file": 9,
	})
	if setResult.IsError {
		t.Fatalf("workspace_set_versioning_policy(control-plane) returned error result: %+v", setResult)
	}

	getResult := provider.CallTool(context.Background(), "workspace_get_versioning_policy", map[string]any{
		"workspace": "repo",
	})
	if getResult.IsError {
		t.Fatalf("workspace_get_versioning_policy(control-plane) returned error result: %+v", getResult)
	}
	var payload map[string]any
	if err := decodeHostedStructuredContent(getResult.StructuredContent, &payload); err != nil {
		t.Fatalf("decodeHostedStructuredContent(get) returned error: %v", err)
	}
	policy, ok := payload["policy"].(map[string]any)
	if !ok {
		t.Fatalf("policy = %#v, want map", payload["policy"])
	}
	if got := policy["mode"]; got != WorkspaceVersioningModePaths {
		t.Fatalf("policy.mode = %#v, want %q", got, WorkspaceVersioningModePaths)
	}
	if got := policy["max_versions_per_file"]; got != float64(9) {
		t.Fatalf("policy.max_versions_per_file = %#v, want 9", got)
	}
}

func TestHostedMCPFileWriteCreatesVersionWhenPolicyEnabled(t *testing.T) {
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

	provider := &hostedMCPProvider{
		manager:    manager,
		databaseID: databaseID,
		workspace:  "repo",
		profile:    MCPProfileWorkspaceRW,
	}

	result := provider.CallTool(context.Background(), "file_write", map[string]any{
		"path":    "/notes/hosted-history.txt",
		"content": "hello hosted history\n",
	})
	if result.IsError {
		t.Fatalf("file_write returned error result: %+v", result)
	}

	var payload map[string]any
	if err := decodeHostedStructuredContent(result.StructuredContent, &payload); err != nil {
		t.Fatalf("decodeHostedStructuredContent() returned error: %v", err)
	}
	if payload["file_id"] == nil || payload["version_id"] == nil {
		t.Fatalf("payload missing version identifiers: %#v", payload)
	}

	lineage, err := service.store.ResolveLiveFileLineageByPath(context.Background(), "repo", "/notes/hosted-history.txt")
	if err != nil {
		t.Fatalf("ResolveLiveFileLineageByPath() returned error: %v", err)
	}
	versions, err := service.store.ListFileVersions(context.Background(), "repo", lineage.FileID, true)
	if err != nil {
		t.Fatalf("ListFileVersions() returned error: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("len(versions) = %d, want 1", len(versions))
	}
	if versions[0].Source != ChangeSourceMCP {
		t.Fatalf("versions[0].Source = %q, want %q", versions[0].Source, ChangeSourceMCP)
	}
	changelog, err := manager.ListChangelog(context.Background(), databaseID, "repo", ChangelogListRequest{Limit: 5})
	if err != nil {
		t.Fatalf("ListChangelog() returned error: %v", err)
	}
	if len(changelog.Entries) == 0 {
		t.Fatal("len(changelog.Entries) = 0, want at least one row")
	}
	if changelog.Entries[0].FileID == "" || changelog.Entries[0].VersionID == "" {
		t.Fatalf("changelog entry missing version linkage: %+v", changelog.Entries[0])
	}
}

func TestHostedMCPFileHistoryAndReadVersionTools(t *testing.T) {
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

	provider := &hostedMCPProvider{
		manager:    manager,
		databaseID: databaseID,
		workspace:  "repo",
		profile:    MCPProfileWorkspaceRW,
	}

	writeResult := provider.CallTool(context.Background(), "file_write", map[string]any{
		"path":    "/notes/hosted-tools.txt",
		"content": "hosted tool history\n",
	})
	if writeResult.IsError {
		t.Fatalf("file_write returned error result: %+v", writeResult)
	}
	var writePayload map[string]any
	if err := decodeHostedStructuredContent(writeResult.StructuredContent, &writePayload); err != nil {
		t.Fatalf("decodeHostedStructuredContent(write) returned error: %v", err)
	}
	versionID, _ := writePayload["version_id"].(string)
	if versionID == "" {
		t.Fatalf("version_id = %#v, want non-empty", writePayload["version_id"])
	}

	updatedWrite := provider.CallTool(context.Background(), "file_write", map[string]any{
		"path":    "/notes/hosted-tools.txt",
		"content": "hosted tool history updated\n",
	})
	if updatedWrite.IsError {
		t.Fatalf("second file_write returned error result: %+v", updatedWrite)
	}
	var updatedWritePayload map[string]any
	if err := decodeHostedStructuredContent(updatedWrite.StructuredContent, &updatedWritePayload); err != nil {
		t.Fatalf("decodeHostedStructuredContent(updated write) returned error: %v", err)
	}
	secondVersionID, _ := updatedWritePayload["version_id"].(string)

	historyResult := provider.CallTool(context.Background(), "file_history", map[string]any{
		"path":      "/notes/hosted-tools.txt",
		"direction": "asc",
		"limit":     1,
	})
	if historyResult.IsError {
		t.Fatalf("file_history returned error result: %+v", historyResult)
	}
	var historyPayload map[string]any
	if err := decodeHostedStructuredContent(historyResult.StructuredContent, &historyPayload); err != nil {
		t.Fatalf("decodeHostedStructuredContent(history) returned error: %v", err)
	}
	history, ok := historyPayload["history"].(map[string]any)
	if !ok {
		t.Fatalf("history = %#v, want map", historyPayload["history"])
	}
	if got := history["order"]; got != "asc" {
		t.Fatalf("history.order = %#v, want asc", got)
	}
	nextCursor, _ := history["next_cursor"].(string)
	if nextCursor == "" {
		t.Fatalf("history.next_cursor = %#v, want non-empty", history["next_cursor"])
	}

	historyPage2Result := provider.CallTool(context.Background(), "file_history", map[string]any{
		"path":      "/notes/hosted-tools.txt",
		"direction": "asc",
		"limit":     1,
		"cursor":    nextCursor,
	})
	if historyPage2Result.IsError {
		t.Fatalf("file_history page 2 returned error result: %+v", historyPage2Result)
	}
	var historyPage2Payload map[string]any
	if err := decodeHostedStructuredContent(historyPage2Result.StructuredContent, &historyPage2Payload); err != nil {
		t.Fatalf("decodeHostedStructuredContent(history page 2) returned error: %v", err)
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

	versionResult := provider.CallTool(context.Background(), "file_read_version", map[string]any{
		"version_id": versionID,
	})
	if versionResult.IsError {
		t.Fatalf("file_read_version returned error result: %+v", versionResult)
	}
	var versionPayload map[string]any
	if err := decodeHostedStructuredContent(versionResult.StructuredContent, &versionPayload); err != nil {
		t.Fatalf("decodeHostedStructuredContent(version) returned error: %v", err)
	}
	version, ok := versionPayload["version"].(map[string]any)
	if !ok {
		t.Fatalf("version = %#v, want map", versionPayload["version"])
	}
	if got := version["content"]; got != "hosted tool history\n" {
		t.Fatalf("version.content = %#v, want written content", got)
	}
	fileID, _ := version["file_id"].(string)
	if fileID == "" {
		t.Fatalf("version.file_id = %#v, want non-empty", version["file_id"])
	}

	ordinalResult := provider.CallTool(context.Background(), "file_read_version", map[string]any{
		"file_id": fileID,
		"ordinal": 1,
	})
	if ordinalResult.IsError {
		t.Fatalf("file_read_version by ordinal returned error result: %+v", ordinalResult)
	}

	diffResult := provider.CallTool(context.Background(), "file_diff_versions", map[string]any{
		"path":            "/notes/hosted-tools.txt",
		"from_version_id": versionID,
		"to_ref":          "working-copy",
	})
	if diffResult.IsError {
		t.Fatalf("file_diff_versions returned error result: %+v", diffResult)
	}
	var diffPayload map[string]any
	if err := decodeHostedStructuredContent(diffResult.StructuredContent, &diffPayload); err != nil {
		t.Fatalf("decodeHostedStructuredContent(diff) returned error: %v", err)
	}
	diff, ok := diffPayload["diff"].(map[string]any)
	if !ok {
		t.Fatalf("diff = %#v, want map", diffPayload["diff"])
	}
	if got, _ := diff["binary"].(bool); got {
		t.Fatalf("diff.binary = true, want false")
	}
	if got, _ := diff["diff"].(string); !strings.Contains(got, "hosted tool history\n") || !strings.Contains(got, "hosted tool history updated\n") {
		t.Fatalf("diff.diff = %q, want historical and working-copy content", got)
	}

	restoreResult := provider.CallTool(context.Background(), "file_restore_version", map[string]any{
		"path":       "/notes/hosted-tools.txt",
		"version_id": versionID,
	})
	if restoreResult.IsError {
		t.Fatalf("file_restore_version returned error result: %+v", restoreResult)
	}

	deletedVersion, err := manager.runtimes[databaseID].store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{Path: "/notes/deleted-hosted.txt"}, VersionedFileSnapshot{
		Path:    "/notes/deleted-hosted.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("deleted hosted\n"),
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP})
	if err != nil {
		t.Fatalf("RecordFileVersionMutation(undelete create) returned error: %v", err)
	}
	if _, err := manager.runtimes[databaseID].store.RecordFileVersionMutation(context.Background(), "repo", VersionedFileSnapshot{
		Path:    "/notes/deleted-hosted.txt",
		Exists:  true,
		Kind:    "file",
		Mode:    0o644,
		Content: []byte("deleted hosted\n"),
	}, VersionedFileSnapshot{
		Path: "/notes/deleted-hosted.txt",
	}, FileVersionMutationMetadata{Source: ChangeSourceMCP}); err != nil {
		t.Fatalf("RecordFileVersionMutation(undelete delete) returned error: %v", err)
	}

	undeleteResult := provider.CallTool(context.Background(), "file_undelete", map[string]any{
		"path": "/notes/deleted-hosted.txt",
	})
	if undeleteResult.IsError {
		t.Fatalf("file_undelete returned error result: %+v", undeleteResult)
	}
	var undeletePayload map[string]any
	if err := decodeHostedStructuredContent(undeleteResult.StructuredContent, &undeletePayload); err != nil {
		t.Fatalf("decodeHostedStructuredContent(undelete) returned error: %v", err)
	}
	undelete, ok := undeletePayload["undelete"].(map[string]any)
	if !ok {
		t.Fatalf("undelete = %#v, want map", undeletePayload["undelete"])
	}
	if got, _ := undelete["undeleted_from_version_id"].(string); got != deletedVersion.VersionID {
		t.Fatalf("undelete.undeleted_from_version_id = %q, want %q", got, deletedVersion.VersionID)
	}
}

func decodeHostedStructuredContent(value any, target any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}
