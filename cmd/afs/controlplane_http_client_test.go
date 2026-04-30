package main

import "testing"

func TestHTTPControlPlaneClientSessionPathUsesScopedDatabase(t *testing.T) {
	t.Helper()

	client := &httpControlPlaneClient{databaseID: "db_123"}
	got := client.clientWorkspaceSessionPath("ws_456", "sessions", "sess_789", "heartbeat")
	want := "/v1/client/databases/db_123/workspaces/ws_456/sessions/sess_789/heartbeat"
	if got != want {
		t.Fatalf("clientWorkspaceSessionPath = %q, want %q", got, want)
	}
}

func TestHTTPControlPlaneClientSessionPathFallsBackToWorkspaceRoute(t *testing.T) {
	t.Helper()

	client := &httpControlPlaneClient{}
	got := client.clientWorkspaceSessionPath("getting-started", "sessions", "sess_789")
	want := "/v1/client/workspaces/getting-started/sessions/sess_789"
	if got != want {
		t.Fatalf("clientWorkspaceSessionPath = %q, want %q", got, want)
	}
}
