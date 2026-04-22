package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncIgnoreBaseline(t *testing.T) {
	t.Helper()
	si, err := loadSyncIgnore(t.TempDir())
	if err != nil {
		t.Fatalf("loadSyncIgnore: %v", err)
	}
	cases := []struct {
		path string
		want bool
	}{
		{".DS_Store", true},
		{"sub/.DS_Store", true},
		{"._Hidden", true},
		{".afs-sync.tmp.foo", true},
		{".afssync.tmp.bar", true},
		{".afsignore", true},
		{"README.md", false},
		{"src/main.go", false},
	}
	for _, tc := range cases {
		if got := si.shouldIgnore(tc.path, false); got != tc.want {
			t.Errorf("shouldIgnore(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestSyncIgnoreFromFile(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, afsIgnoreFilename), []byte("vendor/\n*.tmp\n"), 0o644); err != nil {
		t.Fatalf("write afsignore: %v", err)
	}
	si, err := loadSyncIgnore(root)
	if err != nil {
		t.Fatalf("loadSyncIgnore: %v", err)
	}
	if !si.shouldIgnore("vendor", true) {
		t.Errorf("vendor dir should be ignored")
	}
	if !si.shouldIgnore("vendor/foo.go", false) {
		t.Errorf("vendor/foo.go should be ignored via dir match")
	}
	if !si.shouldIgnore("scratch.tmp", false) {
		t.Errorf("scratch.tmp should be ignored via *.tmp")
	}
	if si.shouldIgnore("README.md", false) {
		t.Errorf("README.md should not be ignored")
	}
}

func TestSyncIgnoreNoFile(t *testing.T) {
	t.Helper()
	si, err := loadSyncIgnore(t.TempDir())
	if err != nil {
		t.Fatalf("loadSyncIgnore: %v", err)
	}
	if si.shouldIgnore("normal/file.txt", false) {
		t.Errorf("normal file should not be ignored without .afsignore")
	}
	// Baseline still active.
	if !si.shouldIgnore(".DS_Store", false) {
		t.Errorf(".DS_Store should always be ignored")
	}
}
