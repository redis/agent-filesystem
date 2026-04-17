package main

import "testing"

func TestDefaultListenAddrUsesPortEnv(t *testing.T) {
	t.Setenv("PORT", "3000")

	if got := defaultListenAddr(); got != ":3000" {
		t.Fatalf("defaultListenAddr() = %q, want :3000", got)
	}
}

func TestDefaultListenAddrFallsBackToLocalhost(t *testing.T) {
	t.Setenv("PORT", "")

	if got := defaultListenAddr(); got != "127.0.0.1:8091" {
		t.Fatalf("defaultListenAddr() = %q, want 127.0.0.1:8091", got)
	}
}

func TestDefaultConfigPathUsesEnvOverride(t *testing.T) {
	t.Setenv(controlPlaneConfigPathEnvVar, "/tmp/afs.config.json")

	if got := defaultConfigPath(); got != "/tmp/afs.config.json" {
		t.Fatalf("defaultConfigPath() = %q, want /tmp/afs.config.json", got)
	}
}
