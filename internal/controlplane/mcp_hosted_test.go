package controlplane

import (
	"context"
	"encoding/json"
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

func decodeHostedStructuredContent(value any, target any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}
