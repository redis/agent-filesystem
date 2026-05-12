package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCmdTokenCreatePostsWorkspaceScopedMountToken(t *testing.T) {
	t.Helper()

	origArgs := os.Args
	os.Args = []string{"afs"}
	t.Cleanup(func() {
		os.Args = origArgs
	})

	var sawRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/workspaces/repo/cli-tokens" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		sawRequest = true
		if got := strings.TrimSpace(r.Header.Get("Authorization")); got != "Bearer afs_cli_account" {
			t.Fatalf("Authorization = %q, want bearer account token", got)
		}
		var input httpCreateCLIAccessTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("Decode(create token request) returned error: %v", err)
		}
		if input.Name != "ci mount" {
			t.Fatalf("Name = %q, want %q", input.Name, "ci mount")
		}
		if input.Capability != "mount-ro" || !input.Readonly {
			t.Fatalf("request capability=%q readonly=%v, want mount-ro readonly", input.Capability, input.Readonly)
		}
		if input.ExpiresAt != "" {
			t.Fatalf("ExpiresAt = %q, want empty for never", input.ExpiresAt)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(httpCLIAccessTokenResponse{
			ID:            "tok_123",
			DatabaseID:    "db_123",
			WorkspaceID:   "ws_repo",
			WorkspaceName: "repo",
			Scope:         "workspace:ws_repo",
			Capability:    "mount-ro",
			Token:         "afs_cli_workspace_secret",
			CreatedAt:     "2026-05-10T12:00:00Z",
		})
	}))
	defer server.Close()

	cfg := defaultConfig()
	cfg.ProductMode = productModeCloud
	cfg.URL = server.URL
	cfg.AuthToken = "afs_cli_account"
	saveTempConfig(t, cfg)

	out, err := captureStdout(t, func() error {
		return cmdTokenCreate([]string{"--workspace", "repo", "--permission", "ro", "--expires", "never", "--name", "ci mount"})
	})
	if err != nil {
		t.Fatalf("cmdTokenCreate() returned error: %v", err)
	}
	if !sawRequest {
		t.Fatalf("server did not receive create token request")
	}
	for _, want := range []string{
		"Access token created",
		"afs_cli_workspace_secret",
		"workspace:ws_repo",
		"mount-ro",
		"afs auth login --url '" + server.URL + "' --access-token 'afs_cli_workspace_secret'",
		"afs ws mount 'repo' <directory>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("cmdTokenCreate() output = %q, want substring %q", out, want)
		}
	}
}
