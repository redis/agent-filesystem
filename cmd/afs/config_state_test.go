package main

import "testing"

func TestEnsureAgentIdentitySeedsMissingAgentFields(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	changed, err := ensureAgentIdentity(&cfg)
	if err != nil {
		t.Fatalf("ensureAgentIdentity() returned error: %v", err)
	}
	if !changed {
		t.Fatal("ensureAgentIdentity() changed = false, want true")
	}
	if cfg.ID == "" {
		t.Fatal("expected agent id to be populated")
	}
	if cfg.Name == "" {
		t.Fatal("expected agent name to be populated")
	}
}
