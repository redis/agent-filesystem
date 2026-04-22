package main

import "testing"

func TestManagedWorkspaceSessionRequestSkipsLocalMode(t *testing.T) {
	t.Helper()

	req := managedWorkspaceSessionRequest(config{
		ProductMode: productModeLocal,
		LocalPath:   "/tmp/repo",
	})
	if req.ClientKind != "" || req.LocalPath != "" || req.Hostname != "" {
		t.Fatalf("managedWorkspaceSessionRequest(local) = %#v, want empty request", req)
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
