package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

func TestResolveLoginModePromptsForFreshLogin(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		cfg   config
		want  string
	}{
		{name: "default cloud", input: "\n", cfg: defaultConfig(), want: productModeCloud},
		{name: "self managed choice", input: "2\n", cfg: defaultConfig(), want: productModeSelfHosted},
		{name: "saved self managed defaults to self managed", input: "\n", cfg: config{ProductMode: productModeSelfHosted}, want: productModeSelfHosted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withStdin(t, tc.input)
			out, err := captureStdout(t, func() error {
				got, err := resolveLoginMode(tc.cfg, false, false, "", "")
				if err != nil {
					return err
				}
				if got != tc.want {
					t.Fatalf("resolveLoginMode() = %q, want %q", got, tc.want)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("resolveLoginMode() returned error: %v", err)
			}
			if !strings.Contains(out, "Connect to a control plane") || !strings.Contains(out, "Self-managed") {
				t.Fatalf("prompt output = %q, want login mode choices", out)
			}
		})
	}
}

func withStdin(t *testing.T, input string) {
	t.Helper()

	stdinPath := t.TempDir() + "/stdin.txt"
	if err := os.WriteFile(stdinPath, []byte(input), 0o644); err != nil {
		t.Fatalf("WriteFile(stdin) returned error: %v", err)
	}
	stdinFile, err := os.Open(stdinPath)
	if err != nil {
		t.Fatalf("Open(stdin) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = stdinFile.Close()
	})

	origStdin := os.Stdin
	os.Stdin = stdinFile
	t.Cleanup(func() {
		os.Stdin = origStdin
	})
}

func TestResolveLoginModeInfersCloudFromKnownURL(t *testing.T) {
	for _, rawURL := range []string{
		"https://afs.cloud",
		"https://www.afs.cloud",
		"https://agentfilesystem.ai",
		"https://www.agentfilesystem.ai",
		"https://redis-afs.com",
		"https://agentfilesystem.vercel.app",
	} {
		t.Run(rawURL, func(t *testing.T) {
			got, err := resolveLoginMode(defaultConfig(), false, false, rawURL, "")
			if err != nil {
				t.Fatalf("resolveLoginMode() returned error: %v", err)
			}
			if got != productModeCloud {
				t.Fatalf("resolveLoginMode() = %q, want %q", got, productModeCloud)
			}
		})
	}
}

func TestResolveLoginModeInfersSelfHostedFromUnknownURL(t *testing.T) {
	for _, rawURL := range []string{
		"http://127.0.0.1:8091",
		"https://afs.cloud.example.com",
		"https://control-plane.internal.example",
	} {
		t.Run(rawURL, func(t *testing.T) {
			got, err := resolveLoginMode(defaultConfig(), false, false, rawURL, "")
			if err != nil {
				t.Fatalf("resolveLoginMode() returned error: %v", err)
			}
			if got != productModeSelfHosted {
				t.Fatalf("resolveLoginMode() = %q, want %q", got, productModeSelfHosted)
			}
		})
	}
}

func TestAuthUsageShowsNestedCommandFamily(t *testing.T) {
	t.Helper()

	out := stripAnsi(authUsageText("afs"))
	for _, want := range []string{
		"Usage: afs auth [options] [command]",
		"Manage authentication",
		"login [options]   Connect to afs control plane",
		"logout            Log out from afs control plane",
		"status [options]  Show authentication status",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("authUsageText() = %q, want substring %q", out, want)
		}
	}
}

func TestTopLevelUsageSurfacesAuthInsteadOfLoginLogout(t *testing.T) {
	t.Helper()

	out, err := captureStderr(t, func() error {
		printUsage()
		return nil
	})
	if err != nil {
		t.Fatalf("printUsage() returned error: %v", err)
	}
	out = stripAnsi(out)
	if !strings.Contains(out, "auth") || !strings.Contains(out, "Login, logout, and inspect authentication") {
		t.Fatalf("printUsage() = %q, want auth command", out)
	}
	if strings.Contains(out, "\n  login") || strings.Contains(out, "\n  logout") {
		t.Fatalf("printUsage() = %q, should not surface top-level login/logout commands", out)
	}
}

func TestCmdLoginPersistsCloudConfig(t *testing.T) {
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

	if err := cmdLogin([]string{"--control-plane-url", server.URL, "--token", "afs_otk_test"}); err != nil {
		t.Fatalf("cmdLogin() returned error: %v", err)
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
	cfg.Account = "alice@example.com"
	saveTempConfig(t, cfg)

	out, err := captureStdout(t, func() error {
		return cmdAuth([]string{"auth", "status"})
	})
	if err != nil {
		t.Fatalf("cmdAuth(status) returned error: %v", err)
	}
	for _, want := range []string{"Authentication", "Cloud", "signed in", "yes", "account", "alice@example.com", "database", "afs-cloud"} {
		if !strings.Contains(out, want) {
			t.Fatalf("auth status output = %q, want substring %q", out, want)
		}
	}
}

func TestCmdStatusShowsSignedInCloudState(t *testing.T) {
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
		return cmdStatus()
	})
	if err != nil {
		t.Fatalf("cmdStatus() returned error: %v", err)
	}
	for _, want := range []string{"getting-started", "afs-cloud"} {
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

	if err := cmdLogin([]string{"--control-plane-url", server.URL, "--token", "afs_otk_test"}); err != nil {
		t.Fatalf("cmdLogin() returned error: %v", err)
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

func TestCmdLoginUsesBrowserFlowWhenTokenMissing(t *testing.T) {
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

	if err := cmdLogin([]string{"--cloud", "--control-plane-url", server.URL, "--workspace", "ws_demo"}); err != nil {
		t.Fatalf("cmdLogin() returned error: %v", err)
	}

	if seenURL != server.URL {
		t.Fatalf("browser flow controlPlaneURL = %q, want %q", seenURL, server.URL)
	}
	if seenWorkspace != "ws_demo" {
		t.Fatalf("browser flow workspace = %q, want %q", seenWorkspace, "ws_demo")
	}
}
