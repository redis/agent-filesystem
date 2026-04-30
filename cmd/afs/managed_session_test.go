package main

import "testing"

func TestManagedWorkspaceSessionRequestTracksLocalMode(t *testing.T) {
	t.Helper()

	req := managedWorkspaceSessionRequest(config{
		ProductMode: productModeLocal,
		LocalPath:   "/tmp/repo",
	})
	if req.ClientKind != "sync" {
		t.Fatalf("ClientKind = %q, want sync", req.ClientKind)
	}
	if req.LocalPath != "/tmp/repo" {
		t.Fatalf("LocalPath = %q, want /tmp/repo", req.LocalPath)
	}
	if req.Hostname == "" {
		t.Fatal("expected Hostname to be populated")
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
