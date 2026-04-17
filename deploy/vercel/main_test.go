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
