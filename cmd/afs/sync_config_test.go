package main

import (
	"testing"
)

func TestEffectiveModeDefaults(t *testing.T) {
	t.Helper()
	cases := []struct {
		name  string
		cfg   config
		want  string
		err   bool
	}{
		{"empty defaults to sync", config{}, modeSync, false},
		{"explicit sync", config{Mode: "sync"}, modeSync, false},
		{"explicit mount", config{Mode: "mount"}, modeMount, false},
		{"explicit none", config{Mode: "none"}, modeNone, false},
		{"garbage errors", config{Mode: "garbage"}, "", true},
		{
			name: "legacy mount config (empty mode + mountpoint + backend)",
			cfg:  config{MountBackend: "nfs", Mountpoint: "/tmp/afs"},
			want: modeMount,
		},
		{
			name: "legacy config with explicit sync path prefers sync",
			cfg:  config{MountBackend: "nfs", Mountpoint: "/tmp/afs", SyncLocalPath: "/tmp/afs"},
			want: modeSync,
		},
	}
	for _, tc := range cases {
		got, err := effectiveMode(tc.cfg)
		if tc.err {
			if err == nil {
				t.Errorf("%s: effectiveMode(%+v): expected error", tc.name, tc.cfg)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: effectiveMode(%+v): %v", tc.name, tc.cfg, err)
		}
		if got != tc.want {
			t.Errorf("%s: effectiveMode(%+v) = %q, want %q", tc.name, tc.cfg, got, tc.want)
		}
	}
}

func TestSyncSizeCapBytesDefault(t *testing.T) {
	t.Helper()
	if got := syncSizeCapBytes(config{}); got != int64(defaultSyncFileSizeCapMB)*1024*1024 {
		t.Fatalf("syncSizeCapBytes default = %d, want %d", got, int64(defaultSyncFileSizeCapMB)*1024*1024)
	}
	if got := syncSizeCapBytes(config{SyncFileSizeCapMB: 8}); got != 8*1024*1024 {
		t.Fatalf("syncSizeCapBytes(8) = %d, want %d", got, 8*1024*1024)
	}
}

func TestEchoSuppressorMarkConsume(t *testing.T) {
	t.Helper()
	e := newEchoSuppressor()
	e.markFile("foo", "deadbeef")
	if _, ok := e.consume("foo"); !ok {
		t.Fatalf("expected echo expectation present")
	}
	if _, ok := e.consume("foo"); ok {
		t.Fatalf("echo expectation should be one-shot")
	}
	e.markSymlink("link", "/tmp/x")
	got, ok := e.consume("link")
	if !ok || got.kind != "symlink" || got.hash != "/tmp/x" {
		t.Fatalf("symlink expectation mismatch: %+v ok=%v", got, ok)
	}
}
