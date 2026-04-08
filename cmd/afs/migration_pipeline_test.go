package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseRedisMemoryInfo(t *testing.T) {
	info := strings.Join([]string{
		"# Memory",
		"used_memory:2097152",
		"maxmemory:8388608",
	}, "\n")

	capacity := parseRedisMemoryInfo(info)
	if capacity.UsedMemory != 2097152 {
		t.Fatalf("used_memory = %d, want 2097152", capacity.UsedMemory)
	}
	if capacity.MaxMemory != 8388608 {
		t.Fatalf("maxmemory = %d, want 8388608", capacity.MaxMemory)
	}
}

func TestEstimateMigrationRequiredBytes(t *testing.T) {
	stats := importStats{
		Files:    10,
		Dirs:     4,
		Symlinks: 2,
		Bytes:    4096,
	}

	got := estimateMigrationRequiredBytes(stats)
	want := int64((4096 + 10*1024 + 4*512 + 2*512) * 5 / 4)
	if got != want {
		t.Fatalf("estimateMigrationRequiredBytes() = %d, want %d", got, want)
	}
}

func TestIsRedisOOM(t *testing.T) {
	if !isRedisOOM(assertErr("OOM command not allowed when used memory > 'maxmemory'")) {
		t.Fatal("expected OOM error to be detected")
	}
	if isRedisOOM(assertErr("WRONGTYPE operation against a key holding the wrong kind of value")) {
		t.Fatal("did not expect non-OOM error to match")
	}
}

func TestFormatMigrationETA(t *testing.T) {
	eta := formatMigrationETA(50, 100, 10*time.Second)
	if eta != "10s" {
		t.Fatalf("formatMigrationETA() = %q, want 10s", eta)
	}
}

func assertErr(msg string) error {
	return testError(msg)
}

type testError string

func (e testError) Error() string {
	return string(e)
}
