package controlplane

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestServiceWorkspaceSessionLifecycle(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cfg := Config{
		RedisConfig: RedisConfig{
			RedisAddr: mr.Addr(),
			RedisDB:   0,
		},
	}
	store := NewStore(rdb)
	ctx := context.Background()

	if err := createWorkspaceWithMetadata(ctx, cfg, store, "repo", workspaceCreateSpec{
		DatabaseID:   "db-demo",
		DatabaseName: "demo",
		CloudAccount: "Redis Cloud / Test",
		Region:       "us-test-1",
		Source:       sourceBlank,
	}); err != nil {
		t.Fatalf("createWorkspaceWithMetadata() returned error: %v", err)
	}

	service := NewService(cfg, store)

	session, err := service.CreateWorkspaceSession(ctx, "repo", CreateWorkspaceSessionRequest{
		AgentID:         "agt_devbox",
		ClientKind:      "sync",
		Hostname:        "devbox",
		OperatingSystem: "darwin",
		LocalPath:       "/tmp/repo",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() returned error: %v", err)
	}
	if session.SessionID == "" {
		t.Fatal("expected managed session to include a session id")
	}

	sessions, err := service.ListWorkspaceSessions(ctx, "repo")
	if err != nil {
		t.Fatalf("ListWorkspaceSessions() returned error: %v", err)
	}
	if len(sessions.Items) != 1 {
		t.Fatalf("len(ListWorkspaceSessions().Items) = %d, want 1", len(sessions.Items))
	}
	if sessions.Items[0].AgentID != "agt_devbox" {
		t.Fatalf("listed agent_id = %q, want %q", sessions.Items[0].AgentID, "agt_devbox")
	}
	if sessions.Items[0].State != workspaceSessionStateStarting {
		t.Fatalf("session state = %q, want %q", sessions.Items[0].State, workspaceSessionStateStarting)
	}

	heartbeat, err := service.HeartbeatWorkspaceSession(ctx, "repo", session.SessionID)
	if err != nil {
		t.Fatalf("HeartbeatWorkspaceSession() returned error: %v", err)
	}
	if heartbeat.State != workspaceSessionStateActive {
		t.Fatalf("heartbeat state = %q, want %q", heartbeat.State, workspaceSessionStateActive)
	}

	mr.FastForward(workspaceSessionLeaseTTL + time.Second)

	sessions, err = service.ListWorkspaceSessions(ctx, "repo")
	if err != nil {
		t.Fatalf("ListWorkspaceSessions() after expiry returned error: %v", err)
	}
	if len(sessions.Items) != 0 {
		t.Fatalf("len(ListWorkspaceSessions().Items) after expiry = %d, want 0", len(sessions.Items))
	}

	record, err := store.GetWorkspaceSession(ctx, "repo", session.SessionID)
	if err != nil {
		t.Fatalf("GetWorkspaceSession() returned error: %v", err)
	}
	if record.State != workspaceSessionStateStale {
		t.Fatalf("stored session state = %q, want %q", record.State, workspaceSessionStateStale)
	}
}

func TestServiceCloseWorkspaceSessionRemovesPresence(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cfg := Config{
		RedisConfig: RedisConfig{
			RedisAddr: mr.Addr(),
			RedisDB:   0,
		},
	}
	store := NewStore(rdb)
	ctx := context.Background()

	if err := createWorkspaceWithMetadata(ctx, cfg, store, "repo", workspaceCreateSpec{
		DatabaseID:   "db-demo",
		DatabaseName: "demo",
		CloudAccount: "Redis Cloud / Test",
		Region:       "us-test-1",
		Source:       sourceBlank,
	}); err != nil {
		t.Fatalf("createWorkspaceWithMetadata() returned error: %v", err)
	}

	service := NewService(cfg, store)
	session, err := service.CreateWorkspaceSession(ctx, "repo", CreateWorkspaceSessionRequest{
		ClientKind: "sync",
		Hostname:   "devbox",
		LocalPath:  "/tmp/repo",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() returned error: %v", err)
	}

	if err := service.CloseWorkspaceSession(ctx, "repo", session.SessionID); err != nil {
		t.Fatalf("CloseWorkspaceSession() returned error: %v", err)
	}

	sessions, err := service.ListWorkspaceSessions(ctx, "repo")
	if err != nil {
		t.Fatalf("ListWorkspaceSessions() returned error: %v", err)
	}
	if len(sessions.Items) != 0 {
		t.Fatalf("len(ListWorkspaceSessions().Items) = %d, want 0", len(sessions.Items))
	}

	record, err := store.GetWorkspaceSession(ctx, "repo", session.SessionID)
	if err != nil {
		t.Fatalf("GetWorkspaceSession() returned error: %v", err)
	}
	if record.State != workspaceSessionStateClosed {
		t.Fatalf("stored session state = %q, want %q", record.State, workspaceSessionStateClosed)
	}
}

func TestServiceWorkspaceSessionCatalogLifecycle(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cfg := Config{
		RedisConfig: RedisConfig{
			RedisAddr: mr.Addr(),
			RedisDB:   0,
		},
	}
	store := NewStore(rdb)
	ctx := context.Background()

	if err := createWorkspaceWithMetadata(ctx, cfg, store, "repo", workspaceCreateSpec{
		DatabaseID:   "db-demo",
		DatabaseName: "demo",
		CloudAccount: "Redis Cloud / Test",
		Region:       "us-test-1",
		Source:       sourceBlank,
	}); err != nil {
		t.Fatalf("createWorkspaceWithMetadata() returned error: %v", err)
	}

	configPath := filepath.Join(t.TempDir(), "afs.config.json")
	catalog, err := openWorkspaceCatalog(configPath)
	if err != nil {
		t.Fatalf("openWorkspaceCatalog() returned error: %v", err)
	}
	t.Cleanup(func() { _ = catalog.Close() })

	detailService := NewService(cfg, store)
	detail, err := detailService.GetWorkspace(ctx, "repo")
	if err != nil {
		t.Fatalf("GetWorkspace() returned error: %v", err)
	}
	summaryInput := workspaceSummaryFromDetail(detail)
	summaryInput.DatabaseID = "redis-test"
	summaryInput.DatabaseName = "test"
	summary, err := catalog.UpsertWorkspace(ctx, summaryInput)
	if err != nil {
		t.Fatalf("UpsertWorkspace() returned error: %v", err)
	}
	if summary.ID == "" {
		t.Fatal("expected workspace catalog to assign an opaque workspace id")
	}

	service := NewServiceWithCatalog(cfg, store, catalog, "redis-test", "test")
	session, err := service.CreateWorkspaceSession(ctx, "repo", CreateWorkspaceSessionRequest{
		AgentID:         "agt_catalog",
		ClientKind:      "sync",
		Hostname:        "devbox",
		OperatingSystem: "darwin",
		LocalPath:       "/tmp/repo",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() returned error: %v", err)
	}

	catalogRecord, err := catalog.GetSession(ctx, session.SessionID)
	if err != nil {
		t.Fatalf("catalog.GetSession() returned error: %v", err)
	}
	if catalogRecord.WorkspaceID != summary.ID {
		t.Fatalf("catalog workspace_id = %q, want %q", catalogRecord.WorkspaceID, summary.ID)
	}
	if catalogRecord.AgentID != "agt_catalog" {
		t.Fatalf("catalog agent_id = %q, want %q", catalogRecord.AgentID, "agt_catalog")
	}
	if catalogRecord.State != workspaceSessionStateStarting {
		t.Fatalf("catalog state = %q, want %q", catalogRecord.State, workspaceSessionStateStarting)
	}

	if _, err := service.HeartbeatWorkspaceSession(ctx, "repo", session.SessionID); err != nil {
		t.Fatalf("HeartbeatWorkspaceSession() returned error: %v", err)
	}
	catalogRecord, err = catalog.GetSession(ctx, session.SessionID)
	if err != nil {
		t.Fatalf("catalog.GetSession() after heartbeat returned error: %v", err)
	}
	if catalogRecord.State != workspaceSessionStateActive {
		t.Fatalf("catalog state after heartbeat = %q, want %q", catalogRecord.State, workspaceSessionStateActive)
	}

	mr.FastForward(workspaceSessionLeaseTTL + time.Second)
	sessions, err := service.ListWorkspaceSessions(ctx, "repo")
	if err != nil {
		t.Fatalf("ListWorkspaceSessions() returned error: %v", err)
	}
	if len(sessions.Items) != 0 {
		t.Fatalf("len(ListWorkspaceSessions().Items) = %d, want 0", len(sessions.Items))
	}
	catalogRecord, err = catalog.GetSession(ctx, session.SessionID)
	if err != nil {
		t.Fatalf("catalog.GetSession() after expiry returned error: %v", err)
	}
	if catalogRecord.State != workspaceSessionStateStale {
		t.Fatalf("catalog state after expiry = %q, want %q", catalogRecord.State, workspaceSessionStateStale)
	}
	if catalogRecord.CloseReason != "expired" {
		t.Fatalf("catalog close_reason after expiry = %q, want %q", catalogRecord.CloseReason, "expired")
	}
}
