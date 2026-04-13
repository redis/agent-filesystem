package main

import (
	"net"
	"strings"
	"testing"
)

func TestDarwinNFSMountOptionsKeepSyncWritesWithoutDisablingPositiveAttrCaching(t *testing.T) {
	t.Helper()

	opts := darwinNFSMountOptions(20490)
	for _, want := range []string{
		"vers=3",
		"tcp",
		"port=20490",
		"mountport=20490",
		"nolocks",
		"nonegnamecache",
		"sync",
	} {
		if !strings.Contains(opts, want) {
			t.Fatalf("darwinNFSMountOptions() = %q, want substring %q", opts, want)
		}
	}
	if strings.Contains(opts, "noac") {
		t.Fatalf("darwinNFSMountOptions() = %q, should not disable positive attr caching", opts)
	}
}

func TestPrepareRuntimeMountConfigFallsBackToFreeNFSPort(t *testing.T) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() returned error: %v", err)
	}
	defer listener.Close()

	occupiedPort := listener.Addr().(*net.TCPAddr).Port
	cfg := config{}
	cfg.MountBackend = mountBackendNFS
	cfg.NFSHost = "127.0.0.1"
	cfg.NFSPort = occupiedPort

	prepared, err := prepareRuntimeMountConfig(cfg, mountBackendNFS)
	if err != nil {
		t.Fatalf("prepareRuntimeMountConfig() returned error: %v", err)
	}
	if prepared.NFSPort == occupiedPort {
		t.Fatalf("prepareRuntimeMountConfig() kept occupied port %d", occupiedPort)
	}
	if prepared.NFSHost != "127.0.0.1" {
		t.Fatalf("prepareRuntimeMountConfig() host = %q, want %q", prepared.NFSHost, "127.0.0.1")
	}
}
