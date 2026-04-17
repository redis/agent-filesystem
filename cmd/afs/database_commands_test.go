package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestDatabaseListAndUseSelfHosted(t *testing.T) {
	t.Helper()

	server, _, _, secondaryDatabaseID := newMultiDatabaseSelfHostedControlPlaneServer(t)

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = server.URL
	cfg.DatabaseID = ""
	saveTempConfig(t, cfg)

	listOutput, err := captureStdout(t, func() error {
		return cmdDatabase([]string{"database", "list"})
	})
	if err != nil {
		t.Fatalf("cmdDatabase(list) returned error: %v", err)
	}
	stripped := stripAnsi(listOutput)
	if !strings.Contains(stripped, "databases on "+server.URL+" (auto database)") {
		t.Fatalf("cmdDatabase(list) output = %q, want managed title", listOutput)
	}
	if !strings.Contains(stripped, "Name") || !strings.Contains(stripped, "Role") {
		t.Fatalf("cmdDatabase(list) output = %q, want table headers", listOutput)
	}
	if !strings.Contains(stripped, "default") {
		t.Fatalf("cmdDatabase(list) output = %q, want default marker", listOutput)
	}

	if err := cmdDatabase([]string{"database", "use", "secondary"}); err != nil {
		t.Fatalf("cmdDatabase(use) returned error: %v", err)
	}
	updated, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() returned error: %v", err)
	}
	if updated.DatabaseID != secondaryDatabaseID {
		t.Fatalf("cfg.DatabaseID = %q, want %q", updated.DatabaseID, secondaryDatabaseID)
	}
}

func TestWorkspaceCreateSelfHostedUsesDefaultOrExplicitDatabase(t *testing.T) {
	t.Helper()

	server, _, _, secondaryDatabaseID := newMultiDatabaseSelfHostedControlPlaneServer(t)

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = server.URL
	cfg.DatabaseID = ""
	saveTempConfig(t, cfg)

	if err := cmdWorkspace([]string{"workspace", "create", "default-created"}); err != nil {
		t.Fatalf("cmdWorkspace(create default) returned error: %v", err)
	}

	resp, err := http.Get(server.URL + "/v1/workspaces/default-created")
	if err != nil {
		t.Fatalf("GET /v1/workspaces/default-created returned error: %v", err)
	}
	defer resp.Body.Close()

	var created struct {
		DatabaseID string `json:"database_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("Decode(default-created) returned error: %v", err)
	}
	if created.DatabaseID == secondaryDatabaseID {
		t.Fatalf("default-created database_id = %q, did not expect explicit secondary database", created.DatabaseID)
	}

	if err := cmdWorkspace([]string{"workspace", "create", "--database", "secondary", "explicit-created"}); err != nil {
		t.Fatalf("cmdWorkspace(create explicit) returned error: %v", err)
	}

	resp, err = http.Get(server.URL + "/v1/workspaces/explicit-created")
	if err != nil {
		t.Fatalf("GET /v1/workspaces/explicit-created returned error: %v", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("Decode(explicit-created) returned error: %v", err)
	}
	if created.DatabaseID != secondaryDatabaseID {
		t.Fatalf("explicit-created database_id = %q, want %q", created.DatabaseID, secondaryDatabaseID)
	}
}
