package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/controlplane"
)

func TestWorkspaceConfigVersioningRoundTrip(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNone
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	loadedCfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	if err := createEmptyWorkspace(context.Background(), loadedCfg, store, "repo"); err != nil {
		closeStore()
		t.Fatalf("createEmptyWorkspace() returned error: %v", err)
	}
	closeStore()

	setIncludeOutput, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "config", "repo", "set", "versioning.includeGlobs", "src/**,docs/**", "--json"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(config set includeGlobs) returned error: %v", err)
	}
	var includePayload workspaceConfigJSON
	if err := json.Unmarshal([]byte(setIncludeOutput), &includePayload); err != nil {
		t.Fatalf("Unmarshal(include payload) returned error: %v\n%s", err, setIncludeOutput)
	}
	if includePayload.Workspace != "repo" || includePayload.Key != "versioning.includeGlobs" {
		t.Fatalf("include payload = %#v, want repo/versioning.includeGlobs", includePayload)
	}

	setModeOutput, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "config", "repo", "set", "versioning.mode", "paths", "--json"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(config set mode) returned error: %v", err)
	}
	var modePayload workspaceConfigJSON
	if err := json.Unmarshal([]byte(setModeOutput), &modePayload); err != nil {
		t.Fatalf("Unmarshal(mode payload) returned error: %v\n%s", err, setModeOutput)
	}
	if modePayload.Value != controlplane.WorkspaceVersioningModePaths {
		t.Fatalf("mode payload value = %#v, want %q", modePayload.Value, controlplane.WorkspaceVersioningModePaths)
	}

	getOutput, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "config", "repo", "get", "versioning.mode", "--json"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(config get mode) returned error: %v", err)
	}
	var getPayload workspaceConfigJSON
	if err := json.Unmarshal([]byte(getOutput), &getPayload); err != nil {
		t.Fatalf("Unmarshal(get payload) returned error: %v\n%s", err, getOutput)
	}
	if getPayload.Value != controlplane.WorkspaceVersioningModePaths {
		t.Fatalf("get payload value = %#v, want %q", getPayload.Value, controlplane.WorkspaceVersioningModePaths)
	}

	setEmbeddingsOutput, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "config", "repo", "set", "query.embeddings.enabled", "true", "--json"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(config set query.embeddings.enabled) returned error: %v", err)
	}
	var embeddingsPayload workspaceConfigJSON
	if err := json.Unmarshal([]byte(setEmbeddingsOutput), &embeddingsPayload); err != nil {
		t.Fatalf("Unmarshal(embeddings payload) returned error: %v\n%s", err, setEmbeddingsOutput)
	}
	if embeddingsPayload.Value != true {
		t.Fatalf("embeddings payload value = %#v, want true", embeddingsPayload.Value)
	}

	setModelOutput, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "config", "repo", "set", "query.embeddings.model", "embeddinggemma", "--json"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(config set query.embeddings.model) returned error: %v", err)
	}
	var modelPayload workspaceConfigJSON
	if err := json.Unmarshal([]byte(setModelOutput), &modelPayload); err != nil {
		t.Fatalf("Unmarshal(model payload) returned error: %v\n%s", err, setModelOutput)
	}
	if modelPayload.Value != "embeddinggemma" {
		t.Fatalf("model payload value = %#v, want embeddinggemma", modelPayload.Value)
	}

	listOutput, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "config", "repo", "list", "--json"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(config list) returned error: %v", err)
	}
	var listPayload workspaceConfigListJSON
	if err := json.Unmarshal([]byte(listOutput), &listPayload); err != nil {
		t.Fatalf("Unmarshal(list payload) returned error: %v\n%s", err, listOutput)
	}
	if listPayload.Values["versioning.mode"] != controlplane.WorkspaceVersioningModePaths {
		t.Fatalf("list versioning.mode = %#v, want %q", listPayload.Values["versioning.mode"], controlplane.WorkspaceVersioningModePaths)
	}
	if listPayload.Values["query.embeddings.enabled"] != true {
		t.Fatalf("list query.embeddings.enabled = %#v, want true", listPayload.Values["query.embeddings.enabled"])
	}
	if listPayload.Values["query.embeddings.model"] != "embeddinggemma" {
		t.Fatalf("list query.embeddings.model = %#v, want embeddinggemma", listPayload.Values["query.embeddings.model"])
	}

	unsetOutput, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "config", "repo", "unset", "versioning.mode", "--json"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(config unset mode) returned error: %v", err)
	}
	var unsetPayload workspaceConfigJSON
	if err := json.Unmarshal([]byte(unsetOutput), &unsetPayload); err != nil {
		t.Fatalf("Unmarshal(unset payload) returned error: %v\n%s", err, unsetOutput)
	}
	if unsetPayload.Value != controlplane.WorkspaceVersioningModeOff {
		t.Fatalf("unset payload value = %#v, want %q", unsetPayload.Value, controlplane.WorkspaceVersioningModeOff)
	}

	unsetEmbeddingsOutput, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "config", "repo", "unset", "query.embeddings.enabled", "--json"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(config unset query.embeddings.enabled) returned error: %v", err)
	}
	var unsetEmbeddingsPayload workspaceConfigJSON
	if err := json.Unmarshal([]byte(unsetEmbeddingsOutput), &unsetEmbeddingsPayload); err != nil {
		t.Fatalf("Unmarshal(unset embeddings payload) returned error: %v\n%s", err, unsetEmbeddingsOutput)
	}
	if unsetEmbeddingsPayload.Value != false {
		t.Fatalf("unset embeddings payload value = %#v, want false", unsetEmbeddingsPayload.Value)
	}
}

func TestParseWorkspaceConfigArgs(t *testing.T) {
	parsed, err := parseWorkspaceConfigArgs([]string{"repo", "set", "versioning.mode", "all", "--json"})
	if err != nil {
		t.Fatalf("parseWorkspaceConfigArgs() returned error: %v", err)
	}
	if parsed.workspace != "repo" || parsed.command != "set" || !parsed.jsonOut {
		t.Fatalf("parsed = %#v, want repo set with json", parsed)
	}
	if len(parsed.rest) != 2 || parsed.rest[0] != "versioning.mode" || parsed.rest[1] != "all" {
		t.Fatalf("parsed.rest = %#v, want key/value", parsed.rest)
	}
}
