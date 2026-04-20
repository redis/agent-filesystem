package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

func TestCmdAuthLoginPersistsCloudConfig(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/exchange" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var input struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("Decode(exchange request) returned error: %v", err)
		}
		if input.Token != "afs_otk_test" {
			t.Fatalf("token = %q, want %q", input.Token, "afs_otk_test")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(authExchangeResponse{
			DatabaseID:    "afs-cloud",
			WorkspaceID:   "ws_demo",
			WorkspaceName: "getting-started",
			AccessToken:   "afs_cli_demo",
		})
	}))
	defer server.Close()

	saveTempConfig(t, defaultConfig())

	if err := cmdAuth([]string{"auth", "login", "--control-plane-url", server.URL, "--token", "afs_otk_test"}); err != nil {
		t.Fatalf("cmdAuth(login) returned error: %v", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() returned error: %v", err)
	}
	if cfg.ProductMode != productModeCloud {
		t.Fatalf("ProductMode = %q, want %q", cfg.ProductMode, productModeCloud)
	}
	if cfg.URL != server.URL {
		t.Fatalf("URL = %q, want %q", cfg.URL, server.URL)
	}
	if cfg.DatabaseID != "afs-cloud" {
		t.Fatalf("DatabaseID = %q, want %q", cfg.DatabaseID, "afs-cloud")
	}
	if cfg.CurrentWorkspaceID != "ws_demo" {
		t.Fatalf("CurrentWorkspaceID = %q, want %q", cfg.CurrentWorkspaceID, "ws_demo")
	}
	if cfg.CurrentWorkspace != "getting-started" {
		t.Fatalf("CurrentWorkspace = %q, want %q", cfg.CurrentWorkspace, "getting-started")
	}
	if cfg.AuthToken != "afs_cli_demo" {
		t.Fatalf("AuthToken = %q, want %q", cfg.AuthToken, "afs_cli_demo")
	}
}

func TestCmdAuthLogoutClearsCloudConfig(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.ProductMode = productModeCloud
	cfg.URL = "https://afs.example.com"
	cfg.DatabaseID = "afs-cloud"
	cfg.AuthToken = "afs_cli_demo"
	cfg.CurrentWorkspace = "getting-started"
	cfg.CurrentWorkspaceID = "ws_demo"
	saveTempConfig(t, cfg)

	if err := cmdAuth([]string{"auth", "logout"}); err != nil {
		t.Fatalf("cmdAuth(logout) returned error: %v", err)
	}

	saved, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() returned error: %v", err)
	}
	if saved.ProductMode != productModeLocal {
		t.Fatalf("ProductMode = %q, want %q", saved.ProductMode, productModeLocal)
	}
	if saved.URL != "" || saved.DatabaseID != "" || saved.AuthToken != "" || saved.CurrentWorkspace != "" || saved.CurrentWorkspaceID != "" {
		t.Fatalf("logout should clear cloud config, got %#v", saved)
	}
}

func TestCmdAuthStatusShowsSignedInCloudState(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.ProductMode = productModeCloud
	cfg.URL = "https://afs.example.com"
	cfg.DatabaseID = "afs-cloud"
	cfg.AuthToken = "afs_cli_demo"
	cfg.CurrentWorkspace = "getting-started"
	cfg.CurrentWorkspaceID = "ws_demo"
	saveTempConfig(t, cfg)

	out, err := captureStdout(t, func() error {
		return cmdAuth([]string{"auth", "status"})
	})
	if err != nil {
		t.Fatalf("cmdAuth(status) returned error: %v", err)
	}
	for _, want := range []string{"signed in", "https://afs.example.com", "getting-started", "afs-cloud"} {
		if !strings.Contains(out, want) {
			t.Fatalf("auth status output = %q, want substring %q", out, want)
		}
	}
}

func TestCloudModeUsesHTTPControlPlaneBackend(t *testing.T) {
	t.Helper()

	server := newSelfHostedControlPlaneServer(t)
	cfg := defaultConfig()
	cfg.ProductMode = productModeCloud
	cfg.URL = server.URL
	cfg.DatabaseID = ""
	saveTempConfig(t, cfg)

	loadedCfg, service, closeFn, err := openAFSControlPlane(context.Background())
	if err != nil {
		t.Fatalf("openAFSControlPlane() returned error: %v", err)
	}
	defer closeFn()
	if loadedCfg.ProductMode != productModeCloud {
		t.Fatalf("loadedCfg.ProductMode = %q, want %q", loadedCfg.ProductMode, productModeCloud)
	}
	workspaces, err := service.ListWorkspaceSummaries(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaceSummaries() returned error: %v", err)
	}
	if len(workspaces.Items) != 1 || workspaces.Items[0].Name != "repo" {
		t.Fatalf("workspaces = %#v, want one repo workspace", workspaces.Items)
	}
}

func TestCloudModeUsesPersistedAuthTokenForWorkspaceList(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/auth/exchange" && r.Method == http.MethodPost:
			var input struct {
				Token string `json:"token"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatalf("Decode(exchange request) returned error: %v", err)
			}
			if input.Token != "afs_otk_test" {
				t.Fatalf("token = %q, want %q", input.Token, "afs_otk_test")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(authExchangeResponse{
				DatabaseID:    "afs-cloud",
				WorkspaceID:   "ws_demo",
				WorkspaceName: "getting-started",
				AccessToken:   "afs_cli_demo",
			})
		case r.URL.Path == "/v1/workspaces" && r.Method == http.MethodGet:
			if got := strings.TrimSpace(r.Header.Get("Authorization")); got != "Bearer afs_cli_demo" {
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(controlplane.WorkspaceListResponse{
				Items: []controlplane.WorkspaceSummary{
					{
						ID:           "ws_demo",
						Name:         "getting-started",
						DatabaseID:   "afs-cloud",
						DatabaseName: "AFS Cloud",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	saveTempConfig(t, defaultConfig())

	if err := cmdAuth([]string{"auth", "login", "--control-plane-url", server.URL, "--token", "afs_otk_test"}); err != nil {
		t.Fatalf("cmdAuth(login) returned error: %v", err)
	}

	cfg, service, closeFn, err := openAFSControlPlane(context.Background())
	if err != nil {
		t.Fatalf("openAFSControlPlane() returned error: %v", err)
	}
	defer closeFn()
	if cfg.AuthToken != "afs_cli_demo" {
		t.Fatalf("cfg.AuthToken = %q, want %q", cfg.AuthToken, "afs_cli_demo")
	}

	workspaces, err := service.ListWorkspaceSummaries(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaceSummaries() returned error: %v", err)
	}
	if len(workspaces.Items) != 1 || workspaces.Items[0].Name != "getting-started" {
		t.Fatalf("workspaces = %#v, want one getting-started workspace", workspaces.Items)
	}
}

func TestCmdAuthLoginUsesBrowserFlowWhenTokenMissing(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/exchange" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var input struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("Decode(exchange request) returned error: %v", err)
		}
		if input.Token != "afs_otk_browser" {
			t.Fatalf("token = %q, want %q", input.Token, "afs_otk_browser")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(authExchangeResponse{
			DatabaseID:    "afs-cloud",
			WorkspaceID:   "ws_demo",
			WorkspaceName: "getting-started",
			AccessToken:   "afs_cli_browser",
		})
	}))
	defer server.Close()

	saveTempConfig(t, defaultConfig())

	original := runBrowserLoginFlow
	t.Cleanup(func() {
		runBrowserLoginFlow = original
	})

	var seenURL string
	var seenWorkspace string
	runBrowserLoginFlow = func(controlPlaneURL, workspace string) (string, error) {
		seenURL = controlPlaneURL
		seenWorkspace = workspace
		return "afs_otk_browser", nil
	}

	if err := cmdAuth([]string{"auth", "login", "--control-plane-url", server.URL, "--workspace", "ws_demo"}); err != nil {
		t.Fatalf("cmdAuth(login browser flow) returned error: %v", err)
	}

	if seenURL != server.URL {
		t.Fatalf("browser flow controlPlaneURL = %q, want %q", seenURL, server.URL)
	}
	if seenWorkspace != "ws_demo" {
		t.Fatalf("browser flow workspace = %q, want %q", seenWorkspace, "ws_demo")
	}
}
