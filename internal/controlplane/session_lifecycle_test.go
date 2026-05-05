package controlplane

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	afsclient "github.com/redis/agent-filesystem/mount/client"
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
		AgentName:       "Rowan's Agent",
		SessionName:     "auth refactor",
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
	if sessions.Items[0].AgentName != "Rowan's Agent" {
		t.Fatalf("listed agent_name = %q, want %q", sessions.Items[0].AgentName, "Rowan's Agent")
	}
	if sessions.Items[0].SessionName != "auth refactor" {
		t.Fatalf("listed session_name = %q, want %q", sessions.Items[0].SessionName, "auth refactor")
	}
	if sessions.Items[0].Label != "auth refactor" {
		t.Fatalf("listed label = %q, want compatibility label auth refactor", sessions.Items[0].Label)
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

func TestWorkspaceSessionNamesKeepAgentAndSessionSeparate(t *testing.T) {
	t.Helper()

	agentName, sessionName, label := workspaceSessionNames(createWorkspaceSessionRequest{
		AgentName: "Rowan's Agent",
		Label:     "Rowan's Agent",
	})
	if agentName != "Rowan's Agent" {
		t.Fatalf("agentName = %q, want Rowan's Agent", agentName)
	}
	if sessionName != "" {
		t.Fatalf("sessionName = %q, want empty", sessionName)
	}
	if label != "Rowan's Agent" {
		t.Fatalf("label = %q, want Rowan's Agent", label)
	}

	agentOnlyRecord := WorkspaceSessionRecord{
		AgentName: "Rowan's Agent",
		Label:     "Rowan's Agent",
	}
	if got := workspaceSessionRecordAgentName(agentOnlyRecord); got != "Rowan's Agent" {
		t.Fatalf("workspaceSessionRecordAgentName(agentOnly) = %q, want Rowan's Agent", got)
	}
	if got := workspaceSessionRecordSessionName(agentOnlyRecord); got != "" {
		t.Fatalf("workspaceSessionRecordSessionName(agentOnly) = %q, want empty", got)
	}

	namedRecord := WorkspaceSessionRecord{
		AgentName:   "Rowan's Agent",
		SessionName: "auth refactor",
		Label:       "auth refactor",
	}
	if got := workspaceSessionRecordAgentName(namedRecord); got != "Rowan's Agent" {
		t.Fatalf("workspaceSessionRecordAgentName(named) = %q, want Rowan's Agent", got)
	}
	if got := workspaceSessionRecordSessionName(namedRecord); got != "auth refactor" {
		t.Fatalf("workspaceSessionRecordSessionName(named) = %q, want auth refactor", got)
	}

	legacyRecord := WorkspaceSessionRecord{Label: "Codex"}
	if got := workspaceSessionRecordAgentName(legacyRecord); got != "Codex" {
		t.Fatalf("workspaceSessionRecordAgentName(legacy) = %q, want Codex", got)
	}
	if got := workspaceSessionRecordSessionName(legacyRecord); got != "" {
		t.Fatalf("workspaceSessionRecordSessionName(legacy) = %q, want empty", got)
	}
}

func TestWorkspaceSessionHeartbeatRefreshesAgentMetadata(t *testing.T) {
	t.Helper()

	service, ctx := newWorkspaceSessionTestService(t)
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

	heartbeat, err := service.HeartbeatWorkspaceSession(ctx, "repo", session.SessionID, CreateWorkspaceSessionRequest{
		AgentID:         "agt_devbox",
		AgentName:       "Office Desktop",
		ClientKind:      "sync",
		Hostname:        "devbox",
		OperatingSystem: "darwin",
		LocalPath:       "/tmp/repo",
	})
	if err != nil {
		t.Fatalf("HeartbeatWorkspaceSession() returned error: %v", err)
	}
	if heartbeat.AgentName != "Office Desktop" {
		t.Fatalf("heartbeat agent_name = %q, want %q", heartbeat.AgentName, "Office Desktop")
	}
	if heartbeat.SessionName != "" {
		t.Fatalf("heartbeat session_name = %q, want empty", heartbeat.SessionName)
	}
	if heartbeat.Label != "Office Desktop" {
		t.Fatalf("heartbeat label = %q, want %q", heartbeat.Label, "Office Desktop")
	}

	heartbeat, err = service.HeartbeatWorkspaceSession(ctx, "repo", session.SessionID, CreateWorkspaceSessionRequest{
		AgentID:         "agt_devbox",
		AgentName:       "Office Desktop",
		SessionName:     "auth refactor",
		ClientKind:      "sync",
		Hostname:        "devbox",
		OperatingSystem: "darwin",
		LocalPath:       "/tmp/repo",
	})
	if err != nil {
		t.Fatalf("HeartbeatWorkspaceSession(named) returned error: %v", err)
	}
	if heartbeat.AgentName != "Office Desktop" {
		t.Fatalf("named heartbeat agent_name = %q, want %q", heartbeat.AgentName, "Office Desktop")
	}
	if heartbeat.SessionName != "auth refactor" {
		t.Fatalf("named heartbeat session_name = %q, want %q", heartbeat.SessionName, "auth refactor")
	}
	if heartbeat.Label != "auth refactor" {
		t.Fatalf("named heartbeat label = %q, want %q", heartbeat.Label, "auth refactor")
	}
}

func TestServiceRestoreCheckpointAllowsActiveWritableWorkspaceSession(t *testing.T) {
	t.Helper()

	service, ctx := newWorkspaceSessionTestService(t)
	if _, err := service.CreateWorkspaceSession(ctx, "repo", CreateWorkspaceSessionRequest{
		ClientKind: "sync",
		Hostname:   "devbox",
		LocalPath:  "/tmp/repo",
		Label:      "Codex",
		Readonly:   false,
	}); err != nil {
		t.Fatalf("CreateWorkspaceSession() returned error: %v", err)
	}

	if err := service.RestoreCheckpoint(ctx, "repo", "initial"); err != nil {
		t.Fatalf("RestoreCheckpoint() returned error: %v", err)
	}
}

func TestServiceRestoreCheckpointAllowsReadonlyWorkspaceSession(t *testing.T) {
	t.Helper()

	service, ctx := newWorkspaceSessionTestService(t)
	if _, err := service.CreateWorkspaceSession(ctx, "repo", CreateWorkspaceSessionRequest{
		ClientKind: "mount",
		Hostname:   "devbox",
		LocalPath:  "/tmp/repo",
		Readonly:   true,
	}); err != nil {
		t.Fatalf("CreateWorkspaceSession() returned error: %v", err)
	}

	if err := service.RestoreCheckpoint(ctx, "repo", "initial"); err != nil {
		t.Fatalf("RestoreCheckpoint() returned error: %v", err)
	}
}

func TestServiceRestoreCheckpointPublishesRootReplaceInvalidation(t *testing.T) {
	t.Helper()

	service, ctx := newWorkspaceSessionTestService(t)
	if err := service.RestoreCheckpoint(ctx, "repo", "initial"); err != nil {
		t.Fatalf("RestoreCheckpoint() returned error: %v", err)
	}

	meta, err := service.store.GetWorkspaceMeta(ctx, "repo")
	if err != nil {
		t.Fatalf("GetWorkspaceMeta() returned error: %v", err)
	}
	c := afsclient.New(service.store.rdb, WorkspaceFSKey(WorkspaceStorageID(meta)))
	readCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	entries, err := c.ReadChangeStream(readCtx, "0-0", 10)
	if err != nil {
		t.Fatalf("ReadChangeStream() returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(ReadChangeStream()) = %d, want 1", len(entries))
	}
	event := entries[0].Event
	if event.Op != afsclient.InvalidateOpRootReplace || len(event.Paths) != 1 || event.Paths[0] != "/" {
		t.Fatalf("restore invalidation = %#v, want root replace for /", event)
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
		AgentName:       "Catalog Agent",
		SessionName:     "catalog sync",
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
	if catalogRecord.AgentName != "Catalog Agent" {
		t.Fatalf("catalog agent_name = %q, want %q", catalogRecord.AgentName, "Catalog Agent")
	}
	if catalogRecord.SessionName != "catalog sync" {
		t.Fatalf("catalog session_name = %q, want %q", catalogRecord.SessionName, "catalog sync")
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

func newWorkspaceSessionTestService(t *testing.T) (*Service, context.Context) {
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
	return NewService(cfg, store), ctx
}
