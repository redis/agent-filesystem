package main

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempHome sets HOME (and on macOS, ensures stateDir() returns a fresh
// per-test temp directory) so writing sync state never escapes the test
// sandbox.
func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestSyncStateRoundTrip(t *testing.T) {
	t.Helper()

	withTempHome(t)

	st := newSyncState("repo", "/tmp/repo")
	st.Entries["docs/README.md"] = SyncEntry{
		Type:       "file",
		Mode:       0o644,
		Size:       42,
		LocalHash:  "abc",
		RemoteHash: "abc",
	}
	if err := saveSyncState(st); err != nil {
		t.Fatalf("saveSyncState: %v", err)
	}

	loaded, err := loadSyncState("repo")
	if err != nil {
		t.Fatalf("loadSyncState: %v", err)
	}
	if loaded.Workspace != "repo" {
		t.Fatalf("workspace = %q, want repo", loaded.Workspace)
	}
	if loaded.LocalPath != "/tmp/repo" {
		t.Fatalf("LocalPath = %q, want /tmp/repo", loaded.LocalPath)
	}
	entry, ok := loaded.Entries["docs/README.md"]
	if !ok {
		t.Fatalf("entry missing after round trip")
	}
	if entry.LocalHash != "abc" || entry.Size != 42 || entry.Mode != 0o644 {
		t.Fatalf("entry mismatch: %+v", entry)
	}
}

func TestSyncStateAtomicWrite(t *testing.T) {
	t.Helper()
	withTempHome(t)

	st := newSyncState("repo", "/tmp/repo")
	if err := saveSyncState(st); err != nil {
		t.Fatalf("saveSyncState: %v", err)
	}
	// No leftover .tmp file in the sync state dir.
	entries, err := os.ReadDir(syncStateDir())
	if err != nil {
		t.Fatalf("read sync state dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) == ".tmp" || filepath.Base(name) != "repo.json" {
			if name != "repo.json" {
				t.Fatalf("unexpected file in sync state dir: %s", name)
			}
		}
	}
}

func TestSyncStateLoadMissing(t *testing.T) {
	t.Helper()
	withTempHome(t)

	if _, err := loadSyncState("nope"); !os.IsNotExist(err) {
		t.Fatalf("loadSyncState(nope) err = %v, want IsNotExist", err)
	}
}

func TestSyncStateRemove(t *testing.T) {
	t.Helper()
	withTempHome(t)

	st := newSyncState("repo", "/tmp/repo")
	if err := saveSyncState(st); err != nil {
		t.Fatalf("saveSyncState: %v", err)
	}
	if err := removeSyncState("repo"); err != nil {
		t.Fatalf("removeSyncState: %v", err)
	}
	if _, err := os.Stat(syncStatePath("repo")); !os.IsNotExist(err) {
		t.Fatalf("state file still present: %v", err)
	}
	// Removing a missing state file is a no-op.
	if err := removeSyncState("repo"); err != nil {
		t.Fatalf("removeSyncState (idempotent): %v", err)
	}
}
