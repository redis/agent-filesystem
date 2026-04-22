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
		LocalPath:   "/tmp/repo",
	})
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
