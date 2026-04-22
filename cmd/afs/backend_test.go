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

func TestOpenAFSStoreRejectsCloudModeForLocalStoreCommands(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.ProductMode = productModeCloud
	cfg.URL = "https://afs.example.com"
	saveTempConfig(t, cfg)

	_, _, _, err := openAFSStore(context.Background())
	if err == nil {
		t.Fatal("openAFSStore() returned nil error, want cloud local-store guidance")
	}
	if !strings.Contains(err.Error(), "does not expose a local Redis store yet") {
		t.Fatalf("openAFSStore() error = %q, want local-store guidance", err)
	}
}

func TestRewriteManagedRedisAddrForLocalhostDockerService(t *testing.T) {
	t.Helper()

	got := rewriteManagedRedisAddrForLocalhost("http://127.0.0.1:8091", "redis:6379")
	if got != "localhost:6379" {
		t.Fatalf("rewriteManagedRedisAddrForLocalhost() = %q, want %q", got, "localhost:6379")
	}
}

func TestRewriteManagedRedisAddrForLocalhostLeavesReachableHostsAlone(t *testing.T) {
	t.Helper()

	cases := []string{
		"localhost:6379",
		"127.0.0.1:6379",
		"cache.example.com:6379",
		"10.0.0.8:6379",
	}

	for _, addr := range cases {
		if got := rewriteManagedRedisAddrForLocalhost("http://127.0.0.1:8091", addr); got != addr {
			t.Fatalf("rewriteManagedRedisAddrForLocalhost(%q) = %q, want unchanged", addr, got)
		}
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
	if !strings.HasPrefix(session.Workspace, "ws_") {
		t.Fatalf("session workspace = %q, want opaque workspace id", session.Workspace)
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
	if bootstrap.redisKey != workspaceRedisKey(bootstrap.cfg.CurrentWorkspaceID) {
		t.Fatalf("bootstrap redisKey = %q, want %q", bootstrap.redisKey, workspaceRedisKey(bootstrap.cfg.CurrentWorkspaceID))
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

func TestPrepareSyncBootstrapCloudResolvesWorkspaceAcrossDatabases(t *testing.T) {
	t.Helper()

	server, secondaryWorkspace, secondaryRedisAddr, secondaryDatabaseID := newMultiDatabaseSelfHostedControlPlaneServer(t)

	cfg := defaultConfig()
	cfg.ProductMode = productModeCloud
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
	if bootstrap.redisKey != workspaceRedisKey(bootstrap.cfg.CurrentWorkspaceID) {
		t.Fatalf("bootstrap redisKey = %q, want %q", bootstrap.redisKey, workspaceRedisKey(bootstrap.cfg.CurrentWorkspaceID))
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

func TestPrepareSyncBootstrapSelfHostedIgnoresStaleConfiguredDatabaseForWorkspaceFirstRoutes(t *testing.T) {
	t.Helper()

	server, primaryWorkspace, primaryRedisAddr, primaryDatabaseID, _, _, secondaryDatabaseID := newDetailedMultiDatabaseSelfHostedControlPlaneServer(t)

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = server.URL
	cfg.DatabaseID = secondaryDatabaseID
	cfg.CurrentWorkspace = primaryWorkspace
	cfg.LocalPath = filepath.Join(t.TempDir(), primaryWorkspace)

	bootstrap, closeFn, err := prepareSyncBootstrap(context.Background(), cfg)
	if err != nil {
		t.Fatalf("prepareSyncBootstrap() returned error: %v", err)
	}
	defer closeFn()

	if bootstrap.workspace != primaryWorkspace {
		t.Fatalf("bootstrap workspace = %q, want %q", bootstrap.workspace, primaryWorkspace)
	}
	if bootstrap.cfg.RedisAddr != primaryRedisAddr {
		t.Fatalf("bootstrap RedisAddr = %q, want %q", bootstrap.cfg.RedisAddr, primaryRedisAddr)
	}
	if bootstrap.cfg.DatabaseID != primaryDatabaseID {
		t.Fatalf("bootstrap DatabaseID = %q, want %q", bootstrap.cfg.DatabaseID, primaryDatabaseID)
	}
}

func TestPrepareSyncBootstrapSelfHostedUsesWorkspaceIDForDuplicateNames(t *testing.T) {
	t.Helper()

	server, secondaryWorkspaceID, secondaryRedisAddr, secondaryDatabaseID := newDuplicateNameSelfHostedControlPlaneServer(t)

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = server.URL
	cfg.DatabaseID = ""
	cfg.CurrentWorkspace = "repo"
	cfg.CurrentWorkspaceID = secondaryWorkspaceID
	cfg.LocalPath = filepath.Join(t.TempDir(), "repo")

	bootstrap, closeFn, err := prepareSyncBootstrap(context.Background(), cfg)
	if err != nil {
		t.Fatalf("prepareSyncBootstrap() returned error: %v", err)
	}
	defer closeFn()

	if bootstrap.workspace != "repo" {
		t.Fatalf("bootstrap workspace = %q, want %q", bootstrap.workspace, "repo")
	}
	if bootstrap.cfg.RedisAddr != secondaryRedisAddr {
		t.Fatalf("bootstrap RedisAddr = %q, want %q", bootstrap.cfg.RedisAddr, secondaryRedisAddr)
	}
	if bootstrap.cfg.DatabaseID != secondaryDatabaseID {
		t.Fatalf("bootstrap DatabaseID = %q, want %q", bootstrap.cfg.DatabaseID, secondaryDatabaseID)
	}
	if bootstrap.cfg.CurrentWorkspaceID != secondaryWorkspaceID {
		t.Fatalf("bootstrap CurrentWorkspaceID = %q, want %q", bootstrap.cfg.CurrentWorkspaceID, secondaryWorkspaceID)
	}
}

func TestPrepareMountBootstrapSelfHostedResolvesWorkspaceAcrossDatabases(t *testing.T) {
	t.Helper()

	server, secondaryWorkspace, secondaryRedisAddr, secondaryDatabaseID := newMultiDatabaseSelfHostedControlPlaneServer(t)

	cfg := defaultConfig()
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = server.URL
	cfg.DatabaseID = ""
	cfg.CurrentWorkspace = secondaryWorkspace
	cfg.LocalPath = filepath.Join(t.TempDir(), secondaryWorkspace)

	bootstrap, closeFn, err := prepareMountBootstrap(context.Background(), cfg)
	if err != nil {
		t.Fatalf("prepareMountBootstrap() returned error: %v", err)
	}
	defer closeFn()

	if bootstrap.workspace != secondaryWorkspace {
		t.Fatalf("bootstrap workspace = %q, want %q", bootstrap.workspace, secondaryWorkspace)
	}
	if bootstrap.redisKey != workspaceRedisKey(bootstrap.cfg.CurrentWorkspaceID) {
		t.Fatalf("bootstrap redisKey = %q, want %q", bootstrap.redisKey, workspaceRedisKey(bootstrap.cfg.CurrentWorkspaceID))
	}
	if bootstrap.cfg.RedisAddr != secondaryRedisAddr {
		t.Fatalf("bootstrap RedisAddr = %q, want %q", bootstrap.cfg.RedisAddr, secondaryRedisAddr)
	}
	if bootstrap.cfg.DatabaseID != secondaryDatabaseID {
		t.Fatalf("bootstrap DatabaseID = %q, want %q", bootstrap.cfg.DatabaseID, secondaryDatabaseID)
	}
	if bootstrap.cfg.CurrentWorkspaceID == "" {
		t.Fatal("expected bootstrap CurrentWorkspaceID to be populated")
	}
	if strings.TrimSpace(bootstrap.sessionID) == "" {
		t.Fatal("expected mount bootstrap session to include a session id")
	}
	if bootstrap.heartbeatEvery <= 0 {
		t.Fatalf("bootstrap heartbeatEvery = %v, want > 0", bootstrap.heartbeatEvery)
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

	primaryRecord, err := manager.UpsertDatabase(context.Background(), "", controlplane.UpsertDatabaseRequest{
		Name:      "primary",
		RedisAddr: mr.Addr(),
		RedisDB:   0,
	})
	if err != nil {
		t.Fatalf("UpsertDatabase(primary) returned error: %v", err)
	}

	if _, err := manager.CreateWorkspace(context.Background(), primaryRecord.ID, controlplane.CreateWorkspaceRequest{
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
	server, _, _, _, secondaryWorkspace, secondaryRedisAddr, secondaryDatabaseID := newDetailedMultiDatabaseSelfHostedControlPlaneServer(t)
	return server, secondaryWorkspace, secondaryRedisAddr, secondaryDatabaseID
}

func newDetailedMultiDatabaseSelfHostedControlPlaneServer(t *testing.T) (*httptest.Server, string, string, string, string, string, string) {
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

	primaryRecord, err := manager.UpsertDatabase(context.Background(), "", controlplane.UpsertDatabaseRequest{
		Name:      "primary",
		RedisAddr: primary.Addr(),
		RedisDB:   0,
	})
	if err != nil {
		t.Fatalf("UpsertDatabase(primary) returned error: %v", err)
	}
	primaryID := primaryRecord.ID

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
	return server, "repo", primary.Addr(), primaryID, secondaryWorkspace, secondary.Addr(), secondaryRecord.ID
}

func newDuplicateNameSelfHostedControlPlaneServer(t *testing.T) (*httptest.Server, string, string, string) {
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

	primaryRecord, err := manager.UpsertDatabase(context.Background(), "", controlplane.UpsertDatabaseRequest{
		Name:      "primary",
		RedisAddr: primary.Addr(),
		RedisDB:   0,
	})
	if err != nil {
		t.Fatalf("UpsertDatabase(primary) returned error: %v", err)
	}
	primaryID := primaryRecord.ID

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

	secondaryWorkspace, err := manager.CreateWorkspace(context.Background(), secondaryRecord.ID, controlplane.CreateWorkspaceRequest{
		Name: "repo",
		Source: controlplane.SourceRef{
			Kind: controlplane.SourceBlank,
		},
	})
	if err != nil {
		t.Fatalf("CreateWorkspace(secondary) returned error: %v", err)
	}

	server := httptest.NewServer(controlplane.NewHandler(manager, "*"))
	t.Cleanup(server.Close)
	return server, secondaryWorkspace.ID, secondary.Addr(), secondaryRecord.ID
}
