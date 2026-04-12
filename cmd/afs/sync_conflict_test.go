package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConflictPathFormat(t *testing.T) {
	t.Helper()
	n := newConflictNamer()
	out := n.conflictPath("/tmp/foo/bar.txt")
	if !strings.HasPrefix(filepath.Base(out), "bar.txt.conflict-") {
		t.Fatalf("unexpected conflict basename: %s", out)
	}
	if filepath.Dir(out) != "/tmp/foo" {
		t.Fatalf("conflict path moved out of source dir: %s", out)
	}
}

func TestConflictNameUniqueOnRapidCalls(t *testing.T) {
	t.Helper()
	n := newConflictNamer()
	a := n.conflictPath("/tmp/file")
	b := n.conflictPath("/tmp/file")
	if a == b {
		t.Fatalf("conflict names collided: %s == %s", a, b)
	}
}

func TestSanitizeConflictHost(t *testing.T) {
	t.Helper()
	cases := map[string]string{
		"my-host.example.com": "my-host.example.com",
		"weird/host":          "weird-host",
		"":                    "unknown",
		"a b c":               "a-b-c",
	}
	for input, want := range cases {
		got := sanitizeConflictHost(input)
		if got != want {
			t.Errorf("sanitize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMoveLocalToConflict(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "foo.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	n := newConflictNamer()
	moved, err := moveLocalToConflict(n, src)
	if err != nil {
		t.Fatalf("moveLocalToConflict: %v", err)
	}
	if moved == "" {
		t.Fatalf("moveLocalToConflict returned empty path")
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still present after move: %v", err)
	}
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("conflict copy missing: %v", err)
	}
}

func TestMoveLocalToConflictMissingSource(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "ghost.txt")
	n := newConflictNamer()
	moved, err := moveLocalToConflict(n, src)
	if err != nil {
		t.Fatalf("moveLocalToConflict: %v", err)
	}
	if moved != "" {
		t.Fatalf("expected empty conflict path for missing source, got %q", moved)
	}
}
