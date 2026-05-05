package main

import "testing"

func TestManagedWorkspaceSessionRequestIncludesLocalMetadata(t *testing.T) {
	t.Helper()

	req := managedWorkspaceSessionRequest(config{
		ProductMode: productModeLocal,
		LocalPath:   "/tmp/repo",
	})
	if req.ClientKind != "sync" {
		t.Fatalf("managedWorkspaceSessionRequest(local).ClientKind = %q, want %q", req.ClientKind, "sync")
	}
	if req.LocalPath != "/tmp/repo" {
		t.Fatalf("managedWorkspaceSessionRequest(local).LocalPath = %q, want %q", req.LocalPath, "/tmp/repo")
	}
	if req.Hostname == "" {
		t.Fatal("expected Hostname to be populated for local mode")
	}
	if req.OperatingSystem == "" {
		t.Fatal("expected OperatingSystem to be populated for local mode")
	}
}

func TestManagedWorkspaceSessionRequestUsesManagedMetadata(t *testing.T) {
	t.Helper()

	req := managedWorkspaceSessionRequest(config{
		ProductMode: productModeSelfHosted,
		agentSettings: agentSettings{
			ID:   "agt_test123",
			Name: "Rowan's Agent",
		},
		LocalPath: "/tmp/repo",
	})
	if req.AgentID != "agt_test123" {
		t.Fatalf("AgentID = %q, want %q", req.AgentID, "agt_test123")
	}
	if req.AgentName != "Rowan's Agent" {
		t.Fatalf("AgentName = %q, want %q", req.AgentName, "Rowan's Agent")
	}
	if req.Label != "Rowan's Agent" {
		t.Fatalf("Label = %q, want %q", req.Label, "Rowan's Agent")
	}
	if req.ClientKind != "sync" {
		t.Fatalf("ClientKind = %q, want %q", req.ClientKind, "sync")
	}
	if req.LocalPath != "/tmp/repo" {
		t.Fatalf("LocalPath = %q, want %q", req.LocalPath, "/tmp/repo")
	}
	if req.OperatingSystem == "" {
		t.Fatal("expected OperatingSystem to be populated")
	}
}

func TestManagedWorkspaceSessionRequestUsesSessionNameSeparately(t *testing.T) {
	t.Helper()

	req := managedWorkspaceSessionRequest(config{
		ProductMode: productModeSelfHosted,
		agentSettings: agentSettings{
			ID:   "agt_test123",
			Name: "Rowan's Agent",
		},
		LocalPath: "/tmp/repo",
	}, "auth refactor")
	if req.AgentName != "Rowan's Agent" {
		t.Fatalf("AgentName = %q, want Rowan's Agent", req.AgentName)
	}
	if req.SessionName != "auth refactor" {
		t.Fatalf("SessionName = %q, want auth refactor", req.SessionName)
	}
	if req.Label != "auth refactor" {
		t.Fatalf("Label = %q, want compatibility label auth refactor", req.Label)
	}
}

func TestManagedWorkspaceSessionRefPrefersOpaqueWorkspaceID(t *testing.T) {
	t.Helper()

	got := managedWorkspaceSessionRef(config{CurrentWorkspaceID: " ws_123 "}, " getting-started ")
	if got != "ws_123" {
		t.Fatalf("managedWorkspaceSessionRef = %q, want %q", got, "ws_123")
	}
}

func TestManagedWorkspaceSessionRefFallsBackToWorkspaceName(t *testing.T) {
	t.Helper()

	got := managedWorkspaceSessionRef(config{}, " getting-started ")
	if got != "getting-started" {
		t.Fatalf("managedWorkspaceSessionRef = %q, want %q", got, "getting-started")
	}
}
