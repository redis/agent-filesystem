package main

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/controlplane"
)

func TestEffectiveProductModeDefaults(t *testing.T) {
	t.Helper()

	cases := []struct {
		name string
		cfg  config
		want string
		err  bool
	}{
		{name: "empty defaults to local", cfg: config{}, want: productModeLocal},
		{name: "explicit local", cfg: config{ProductMode: productModeLocal}, want: productModeLocal},
		{name: "legacy direct normalizes to local", cfg: config{ProductMode: legacyProductModeDirect}, want: productModeLocal},
		{name: "explicit self hosted", cfg: config{ProductMode: productModeSelfHosted}, want: productModeSelfHosted},
		{name: "explicit cloud", cfg: config{ProductMode: productModeCloud}, want: productModeCloud},
		{name: "garbage errors", cfg: config{ProductMode: "garbage"}, err: true},
	}

	for _, tc := range cases {
		got, err := effectiveProductMode(tc.cfg)
		if tc.err {
			if err == nil {
				t.Errorf("%s: effectiveProductMode(%+v): expected error", tc.name, tc.cfg)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: effectiveProductMode(%+v): %v", tc.name, tc.cfg, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: effectiveProductMode(%+v) = %q, want %q", tc.name, tc.cfg, got, tc.want)
		}
	}
}

func TestOpenAFSStoreRejectsUnsupportedProductMode(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.ProductMode = productModeCloud
	cfg.URL = "https://afs.example.com"
	saveTempConfig(t, cfg)

	_, _, _, err := openAFSStore(context.Background())
	if err == nil {
		t.Fatal("openAFSStore() returned nil error, want unsupported cloud mode error")
	}
	if !strings.Contains(err.Error(), "cloud mode is not implemented yet") {
		t.Fatalf("openAFSStore() error = %q, want unsupported product mode guidance", err)
	}
}

func TestOpenAFSControlPlaneSelfHostedSingleDatabaseStillWorksWithoutConfiguredDatabase(t *testing.T) {
	t.Helper()

	server := newSelfHostedControlPlaneServer(t)

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = server.URL
	cfg.DatabaseID = ""
	saveTempConfig(t, cfg)

	loadedCfg, service, closeFn, err := openAFSControlPlane(context.Background())
	if err != nil {
		t.Fatalf("openAFSControlPlane() returned error: %v", err)
	}
	defer closeFn()
	if strings.TrimSpace(loadedCfg.DatabaseID) != "" {
		t.Fatalf("loadedCfg.DatabaseID = %q, want empty workspace-first config", loadedCfg.DatabaseID)
	}

	workspaces, err := service.ListWorkspaceSummaries(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaceSummaries() returned error: %v", err)
	}
	if len(workspaces.Items) != 1 || workspaces.Items[0].Name != "repo" {
		t.Fatalf("workspaces = %#v, want one repo workspace", workspaces.Items)
	}

	session, err := service.CreateWorkspaceSession(context.Background(), "repo", controlplane.CreateWorkspaceSessionRequest{
		ClientKind: "sync",
		Hostname:   "test-host",
		LocalPath:  "/tmp/repo",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() returned error: %v", err)
	}
	if session.Workspace != "repo" {
		t.Fatalf("session workspace = %q, want %q", session.Workspace, "repo")
	}
	if strings.TrimSpace(session.RedisKey) == "" {
		t.Fatal("expected workspace session to include redis key")
	}
	if strings.TrimSpace(session.SessionID) == "" {
		t.Fatal("expected workspace session to include a session id")
	}
}

func TestPrepareSyncBootstrapSelfHostedResolvesWorkspaceAcrossDatabases(t *testing.T) {
	t.Helper()

	server, secondaryWorkspace, secondaryRedisAddr, secondaryDatabaseID := newMultiDatabaseSelfHostedControlPlaneServer(t)

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = server.URL
	cfg.DatabaseID = ""
	cfg.CurrentWorkspace = secondaryWorkspace
	cfg.LocalPath = filepath.Join(t.TempDir(), secondaryWorkspace)

	bootstrap, closeFn, err := prepareSyncBootstrap(context.Background(), cfg)
	if err != nil {
		t.Fatalf("prepareSyncBootstrap() returned error: %v", err)
	}
	defer closeFn()

	if bootstrap.workspace != secondaryWorkspace {
		t.Fatalf("bootstrap workspace = %q, want %q", bootstrap.workspace, secondaryWorkspace)
	}
	if bootstrap.redisKey != workspaceRedisKey(secondaryWorkspace) {
		t.Fatalf("bootstrap redisKey = %q, want %q", bootstrap.redisKey, workspaceRedisKey(secondaryWorkspace))
	}
	if bootstrap.cfg.RedisAddr != secondaryRedisAddr {
		t.Fatalf("bootstrap RedisAddr = %q, want %q", bootstrap.cfg.RedisAddr, secondaryRedisAddr)
	}
	if bootstrap.cfg.DatabaseID != secondaryDatabaseID {
		t.Fatalf("bootstrap DatabaseID = %q, want %q", bootstrap.cfg.DatabaseID, secondaryDatabaseID)
	}
	if strings.TrimSpace(bootstrap.sessionID) == "" {
		t.Fatal("expected bootstrap session to include a session id")
	}
}

func TestOpenAFSStoreRejectsSelfHostedModeForDirectStoreCommands(t *testing.T) {
	t.Helper()

	server := newSelfHostedControlPlaneServer(t)

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = server.URL
	saveTempConfig(t, cfg)

	_, _, _, err := openAFSStore(context.Background())
	if err == nil {
		t.Fatal("openAFSStore() returned nil error, want direct-store guidance")
	}
	if !strings.Contains(err.Error(), "does not expose a local Redis store yet") {
		t.Fatalf("openAFSStore() error = %q, want self-hosted local-store guidance", err)
	}
}

func newSelfHostedControlPlaneServer(t *testing.T) *httptest.Server {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	saveTempConfig(t, cfg)

	manager, err := controlplane.OpenDatabaseManager(cfgPathOverride)
	if err != nil {
		t.Fatalf("OpenDatabaseManager() returned error: %v", err)
	}
	t.Cleanup(manager.Close)

	databases, err := manager.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("ListDatabases() returned error: %v", err)
	}
	if len(databases.Items) != 1 {
		t.Fatalf("len(databases.items) = %d, want 1", len(databases.Items))
	}

	if _, err := manager.CreateWorkspace(context.Background(), databases.Items[0].ID, controlplane.CreateWorkspaceRequest{
		Name: "repo",
		Source: controlplane.SourceRef{
			Kind: controlplane.SourceBlank,
		},
	}); err != nil {
		t.Fatalf("CreateWorkspace() returned error: %v", err)
	}

	server := httptest.NewServer(controlplane.NewHandler(manager, "*"))
	t.Cleanup(server.Close)
	return server
}

func newMultiDatabaseSelfHostedControlPlaneServer(t *testing.T) (*httptest.Server, string, string, string) {
	t.Helper()

	primary := miniredis.RunT(t)
	secondary := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = primary.Addr()
	saveTempConfig(t, cfg)

	manager, err := controlplane.OpenDatabaseManager(cfgPathOverride)
	if err != nil {
		t.Fatalf("OpenDatabaseManager() returned error: %v", err)
	}
	t.Cleanup(manager.Close)

	databases, err := manager.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("ListDatabases() returned error: %v", err)
	}
	if len(databases.Items) != 1 {
		t.Fatalf("len(databases.items) = %d, want 1", len(databases.Items))
	}
	primaryID := databases.Items[0].ID

	secondaryRecord, err := manager.UpsertDatabase(context.Background(), "", controlplane.UpsertDatabaseRequest{
		Name:      "secondary",
		RedisAddr: secondary.Addr(),
		RedisDB:   0,
	})
	if err != nil {
		t.Fatalf("UpsertDatabase(secondary) returned error: %v", err)
	}

	if _, err := manager.CreateWorkspace(context.Background(), primaryID, controlplane.CreateWorkspaceRequest{
		Name: "repo",
		Source: controlplane.SourceRef{
			Kind: controlplane.SourceBlank,
		},
	}); err != nil {
		t.Fatalf("CreateWorkspace(primary) returned error: %v", err)
	}

	secondaryWorkspace := "repo-secondary"
	if _, err := manager.CreateWorkspace(context.Background(), secondaryRecord.ID, controlplane.CreateWorkspaceRequest{
		Name: secondaryWorkspace,
		Source: controlplane.SourceRef{
			Kind: controlplane.SourceBlank,
		},
	}); err != nil {
		t.Fatalf("CreateWorkspace(secondary) returned error: %v", err)
	}

	server := httptest.NewServer(controlplane.NewHandler(manager, "*"))
	t.Cleanup(server.Close)
	return server, secondaryWorkspace, secondary.Addr(), secondaryRecord.ID
}
