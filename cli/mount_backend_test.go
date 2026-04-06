package main

import (
	"strings"
	"testing"
)

func TestDarwinNFSMountOptionsDisableCachingAndUseSyncWrites(t *testing.T) {
	t.Helper()

	opts := darwinNFSMountOptions(20490)
	for _, want := range []string{
		"vers=3",
		"tcp",
		"port=20490",
		"mountport=20490",
		"nolocks",
		"noac",
		"nonegnamecache",
		"sync",
	} {
		if !strings.Contains(opts, want) {
			t.Fatalf("darwinNFSMountOptions() = %q, want substring %q", opts, want)
		}
	}
}
