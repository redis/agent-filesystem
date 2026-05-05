package controlplane

import (
	"context"
	"testing"
	"time"
)

func TestMonitorSubscriptionReceivesActivityEvents(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, closeFn := manager.subscribeMonitorEvents(ctx)
	defer closeFn()

	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	if err := service.store.Audit(context.Background(), "repo", "test_activity", map[string]any{"path": "/README.md"}); err != nil {
		t.Fatalf("Audit() returned error: %v", err)
	}

	select {
	case event := <-events:
		if event.Type != "activity" {
			t.Fatalf("monitor event type = %q, want activity", event.Type)
		}
		if event.DatabaseID != databaseID {
			t.Fatalf("monitor event database_id = %q, want %q", event.DatabaseID, databaseID)
		}
		if event.WorkspaceName != "repo" {
			t.Fatalf("monitor event workspace_name = %q, want repo", event.WorkspaceName)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for monitor activity event")
	}
}

func TestMonitorSubscriptionReceivesHeartbeatEvents(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, closeFn := manager.subscribeMonitorEvents(ctx)
	defer closeFn()

	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	session, err := service.CreateWorkspaceSession(context.Background(), "repo", CreateWorkspaceSessionRequest{
		AgentID:    "agt_monitor",
		ClientKind: "sync",
		Hostname:   "devbox",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() returned error: %v", err)
	}
	if _, err := service.HeartbeatWorkspaceSession(context.Background(), "repo", session.SessionID); err != nil {
		t.Fatalf("HeartbeatWorkspaceSession() returned error: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Type == "agents" && event.Reason == "heartbeat" {
				if event.SessionID != session.SessionID {
					t.Fatalf("monitor event session_id = %q, want %q", event.SessionID, session.SessionID)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for monitor heartbeat event")
		}
	}
}

func TestMonitorSubscriptionReceivesWorkspaceCreateEvents(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, closeFn := manager.subscribeMonitorEvents(ctx)
	defer closeFn()

	detail, err := manager.CreateWorkspace(context.Background(), databaseID, createWorkspaceRequest{
		Name: "new-workspace",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace() returned error: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Type == "workspaces" && event.Reason == "created" {
				if event.WorkspaceID != detail.ID {
					t.Fatalf("monitor event workspace_id = %q, want %q", event.WorkspaceID, detail.ID)
				}
				if event.WorkspaceName != "new-workspace" {
					t.Fatalf("monitor event workspace_name = %q, want new-workspace", event.WorkspaceName)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for monitor workspace create event")
		}
	}
}

func TestMonitorSubscriptionReceivesWorkspaceDeleteEvents(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, closeFn := manager.subscribeMonitorEvents(ctx)
	defer closeFn()

	if err := manager.DeleteWorkspace(context.Background(), databaseID, "repo"); err != nil {
		t.Fatalf("DeleteWorkspace() returned error: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Type == "workspaces" && event.Reason == "deleted" {
				if event.WorkspaceName != "repo" {
					t.Fatalf("monitor event workspace_name = %q, want repo", event.WorkspaceName)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for monitor workspace delete event")
		}
	}
}

func TestMonitorSubscriptionReceivesChangeEvents(t *testing.T) {
	t.Helper()

	manager, databaseID := newTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, closeFn := manager.subscribeMonitorEvents(ctx)
	defer closeFn()

	service, _, err := manager.serviceFor(context.Background(), databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}
	WriteChangeEntries(context.Background(), service.store.rdb, "repo", []ChangeEntry{{
		Path:        "/notes.md",
		Op:          ChangeOpPut,
		Source:      ChangeSourceMCP,
		SizeBytes:   12,
		DeltaBytes:  12,
		ContentHash: "sha256:test",
	}})

	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Type == "changes" && event.Reason == "changed" {
				if event.WorkspaceID != "repo" {
					t.Fatalf("monitor event workspace_id = %q, want repo", event.WorkspaceID)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for monitor change event")
		}
	}
}
