package controlplane

import (
	"context"
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
