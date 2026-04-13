package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func itoa(i int) string { return strconv.Itoa(i) }

// Scenario 1: a file the daemon finds at startup ends up in the live root.
// This is the cheapest possible end-to-end smoke test for the upload path.
func TestSyncStartupUpload(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeLocalFile(t, "hello.txt", "world")

	env.startDaemon(t)
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "hello.txt to land remotely", func() bool {
		return env.remoteExists(t, "hello.txt")
	})
	if got := env.readRemoteFile(t, "hello.txt"); got != "world" {
		t.Fatalf("remote content = %q, want %q", got, "world")
	}
}

// Scenario 1b: a file written remotely before startup is materialized
// locally during the initial reconciliation.
func TestSyncStartupDownload(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeRemoteFile(t, "remote.md", "# remote")

	env.startDaemon(t)
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "remote.md to materialize locally", func() bool {
		return env.localExists("remote.md")
	})
	if got := env.readLocalFile(t, "remote.md"); got != "# remote" {
		t.Fatalf("local content = %q, want %q", got, "# remote")
	}
}

// Scenario 9: oversize files are explicitly refused and never reach the
// uploader's Echo path. We set a tiny cap to keep the test fast.
func TestSyncOversizedFileRefused(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)

	big := strings.Repeat("a", 1024)
	env.writeLocalFile(t, "big.bin", big)

	env.startDaemon(t, func(c *syncDaemonConfig) {
		// 100 bytes — well below the 1024-byte file.
		c.MaxFileBytes = 100
	})
	defer env.stopDaemon()

	// Wait for at least one reconcile pass to drain.
	time.Sleep(200 * time.Millisecond)
	if env.remoteExists(t, "big.bin") {
		t.Fatalf("oversize file unexpectedly uploaded")
	}
}

// Scenario 14a: deleting a local file propagates to the remote.
func TestSyncLocalDeletePropagates(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeLocalFile(t, "doomed.txt", "x")
	env.startDaemon(t)
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "doomed.txt remote", func() bool {
		return env.remoteExists(t, "doomed.txt")
	})

	abs := filepath.Join(env.localRoot, "doomed.txt")
	if err := removeFile(abs); err != nil {
		t.Fatalf("remove local: %v", err)
	}

	assertEventually(t, 3*time.Second, "remote delete", func() bool {
		return !env.remoteExists(t, "doomed.txt")
	})
}

// Scenario 16: macOS baseline ignore — .DS_Store never crosses.
func TestSyncBaselineIgnoreFilter(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeLocalFile(t, ".DS_Store", "junk")
	env.writeLocalFile(t, "real.txt", "hi")

	env.startDaemon(t)
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "real.txt remote", func() bool {
		return env.remoteExists(t, "real.txt")
	})
	if env.remoteExists(t, ".DS_Store") {
		t.Fatalf(".DS_Store should not be synced")
	}
}

// Scenario 7: an .afsignore at the root applies symmetrically.
func TestSyncAfsignoreFilter(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeLocalFile(t, ".afsignore", "secret/\n")
	env.writeLocalFile(t, "secret/key.txt", "hush")
	env.writeLocalFile(t, "public.txt", "ok")

	env.startDaemon(t)
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "public.txt remote", func() bool {
		return env.remoteExists(t, "public.txt")
	})
	if env.remoteExists(t, "secret/key.txt") {
		t.Fatalf("ignored file should not be synced")
	}
}

// Scenario 14b: deleting a remote file propagates to the local copy.
func TestSyncRemoteDeletePropagates(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeLocalFile(t, "doomed.txt", "x")

	env.startDaemon(t)
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "remote upload", func() bool {
		return env.remoteExists(t, "doomed.txt")
	})

	// Now remove via the client and verify the local copy disappears.
	if err := env.fsClient.Rm(testCtx(), absoluteRemotePath("doomed.txt")); err != nil {
		t.Fatalf("remote rm: %v", err)
	}

	assertEventually(t, 3*time.Second, "local delete", func() bool {
		return !env.localExists("doomed.txt")
	})
}

// Scenario 3: stop the daemon, modify both local and remote, restart, expect
// the conflict resolution path to fire (remote wins, local preserved as
// .conflict-*).
func TestSyncRestartConflictPath(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeLocalFile(t, "shared.txt", "v0")

	env.startDaemon(t)
	assertEventually(t, 3*time.Second, "v0 remote", func() bool {
		return env.remoteExists(t, "shared.txt") && env.readRemoteFile(t, "shared.txt") == "v0"
	})
	env.stopDaemon()

	// Mutate both sides while no daemon is running.
	env.writeLocalFile(t, "shared.txt", "local-after")
	if err := env.fsClient.Echo(testCtx(), absoluteRemotePath("shared.txt"), []byte("remote-after")); err != nil {
		t.Fatalf("remote echo: %v", err)
	}

	env.startDaemon(t)
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "remote-after to land locally", func() bool {
		return env.localExists("shared.txt") && env.readLocalFile(t, "shared.txt") == "remote-after"
	})

	// And a conflict copy should exist.
	matches, err := filepath.Glob(filepath.Join(env.localRoot, "shared.txt.conflict-*"))
	if err != nil {
		t.Fatalf("glob conflict copies: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected at least one conflict copy")
	}
}

// Scenario 15: empty files round trip in both directions.
func TestSyncEmptyFile(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeLocalFile(t, "empty.txt", "")
	env.startDaemon(t)
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "empty file remote", func() bool {
		return env.remoteExists(t, "empty.txt")
	})
	if got := env.readRemoteFile(t, "empty.txt"); got != "" {
		t.Fatalf("remote content = %q, want empty", got)
	}
}

// Scenario 1 (burst variant): a batch of files written before startup all
// land remotely, and the steady-state has no spurious echo loops.
func TestSyncStartupUploadBurst(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	for i := 0; i < 10; i++ {
		env.writeLocalFile(t, filepath.Join("burst", filepath.Clean("file"+itoa(i)+".txt")), "v"+itoa(i))
	}

	env.startDaemon(t)
	defer env.stopDaemon()

	for i := 0; i < 10; i++ {
		name := "burst/file" + itoa(i) + ".txt"
		assertEventually(t, 3*time.Second, name+" remote", func() bool {
			return env.remoteExists(t, name)
		})
	}

	// State should contain all 10 entries plus the burst dir.
	snap := env.daemon.Snapshot()
	count := 0
	for path := range snap.Entries {
		if filepath.Dir(path) == "burst" {
			count++
		}
	}
	if count != 10 {
		t.Fatalf("expected 10 file entries under burst/, got %d", count)
	}
}

// Scenario 5: editor atomic-replace pattern (write to temp, rename over real
// file). The reconciler should eventually publish "real" without leaking
// "real.swp" or any tmp variants.
func TestSyncAtomicReplacePattern(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeLocalFile(t, "config.toml", "v1")
	env.startDaemon(t)
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "v1 remote", func() bool {
		return env.remoteExists(t, "config.toml") && env.readRemoteFile(t, "config.toml") == "v1"
	})

	// Editor pattern: write to a sibling, fsync, rename over destination.
	tmp := filepath.Join(env.localRoot, ".config.toml.swp")
	if err := os.WriteFile(tmp, []byte("v2"), 0o644); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	if err := os.Rename(tmp, filepath.Join(env.localRoot, "config.toml")); err != nil {
		t.Fatalf("rename: %v", err)
	}

	assertEventually(t, 3*time.Second, "v2 remote", func() bool {
		return env.remoteExists(t, "config.toml") && env.readRemoteFile(t, "config.toml") == "v2"
	})
	// The .swp temp file is gone locally and never reached remote.
	if env.remoteExists(t, ".config.toml.swp") {
		t.Fatalf(".swp leaked to remote")
	}
}

// Regression: the old fullReconciler dispatched ops into 256-capacity channels
// before consumer goroutines were running. A workspace with >256 entries
// blocked forever. This test creates 500 remote files and verifies they all
// materialize locally within a reasonable timeout.
func TestSyncColdStartWith500Files(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)

	// Seed 500 files directly in the live workspace.
	for i := 0; i < 500; i++ {
		name := "file" + itoa(i) + ".txt"
		env.writeRemoteFile(t, name, "content-"+itoa(i))
	}

	env.startDaemon(t)
	defer env.stopDaemon()

	// All 500 should arrive locally. Give generous timeout because miniredis
	// is in-process and much faster than WAN, but the parallelism still
	// exercises the worker pool.
	assertEventually(t, 10*time.Second, "500 files locally", func() bool {
		for i := 0; i < 500; i++ {
			if !env.localExists("file" + itoa(i) + ".txt") {
				return false
			}
		}
		return true
	})

	// Spot-check content.
	if got := env.readLocalFile(t, "file0.txt"); got != "content-0" {
		t.Fatalf("file0.txt = %q, want %q", got, "content-0")
	}
	if got := env.readLocalFile(t, "file499.txt"); got != "content-499" {
		t.Fatalf("file499.txt = %q, want %q", got, "content-499")
	}
}

// Test that warm restart (with existing SyncState) skips unchanged files
// by comparing size+mtime, and only downloads changed ones.
func TestSyncWarmRestartSkipsUnchanged(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)

	env.writeRemoteFile(t, "stable.txt", "original")
	env.writeRemoteFile(t, "changing.txt", "v1")

	env.startDaemon(t)
	assertEventually(t, 3*time.Second, "initial sync", func() bool {
		return env.localExists("stable.txt") && env.localExists("changing.txt")
	})
	env.stopDaemon()

	// Modify only changing.txt remotely.
	if err := env.fsClient.Echo(testCtx(), absoluteRemotePath("changing.txt"), []byte("v2")); err != nil {
		t.Fatalf("remote echo: %v", err)
	}

	// Restart daemon. stable.txt should be skipped (size+mtime match state),
	// changing.txt should be re-downloaded.
	env.startDaemon(t)
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "v2 downloaded", func() bool {
		return env.localExists("changing.txt") && env.readLocalFile(t, "changing.txt") == "v2"
	})
	// stable.txt should still be the original.
	if got := env.readLocalFile(t, "stable.txt"); got != "original" {
		t.Fatalf("stable.txt = %q, want %q", got, "original")
	}
}

// Test that the change stream journal enables catch-up after an offline
// period. The daemon writes to the stream on every mutation; after restart,
// it reads from the saved cursor and replays missed changes.
func TestSyncStreamCatchUpAfterOffline(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)

	// Start daemon, create a file, wait for it to sync.
	env.writeLocalFile(t, "before.txt", "v1")
	env.startDaemon(t)
	assertEventually(t, 3*time.Second, "before.txt remote", func() bool {
		return env.remoteExists(t, "before.txt")
	})

	// Let the stream cursor get persisted.
	time.Sleep(200 * time.Millisecond)
	env.stopDaemon()

	// While the daemon is stopped, write a file directly to Redis
	// (simulating another client). The native client's Echo will XADD
	// to the changes stream automatically.
	env.writeRemoteFile(t, "missed.txt", "offline-change")

	// Restart the daemon. It should catch up from the stream and download
	// the missed file without needing a full remote scan.
	env.startDaemon(t)
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "missed.txt via stream catch-up", func() bool {
		return env.localExists("missed.txt") && env.readLocalFile(t, "missed.txt") == "offline-change"
	})
}

// Regression: deleting a local file and then triggering a full reconciliation
// (e.g. from a subscription reconnect) before the watcher debounce fires
// would re-download the file. The fullReconciler should detect that a
// previously-synced file is missing locally and propagate the delete to
// remote rather than re-downloading.
func TestSyncLocalDeleteSurvivesFullReconcile(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeLocalFile(t, "vanish.txt", "goodbye")

	env.startDaemon(t)
	defer env.stopDaemon()

	// Wait for the file to be fully synced.
	assertEventually(t, 3*time.Second, "vanish.txt remote", func() bool {
		return env.remoteExists(t, "vanish.txt")
	})

	// Delete the file locally.
	abs := filepath.Join(env.localRoot, "vanish.txt")
	if err := removeFile(abs); err != nil {
		t.Fatalf("remove local: %v", err)
	}

	// Immediately force a full reconciliation — simulates a subscription
	// reconnect or activity from a second system that races with the
	// watcher debounce.
	env.daemon.reconciler.requestFullSweep()

	// The file must stay deleted locally and eventually disappear from remote.
	assertEventually(t, 5*time.Second, "remote delete after full reconcile", func() bool {
		return !env.remoteExists(t, "vanish.txt")
	})
	// Verify the file did not reappear locally.
	if env.localExists("vanish.txt") {
		t.Fatalf("vanish.txt reappeared locally after delete + full reconcile")
	}
}

// Regression: stopping the daemon, deleting a file, then restarting should
// propagate the delete to remote — not re-download the file.
func TestSyncOfflineDeletePropagatesOnRestart(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeLocalFile(t, "offline.txt", "will-die")

	env.startDaemon(t)
	assertEventually(t, 3*time.Second, "offline.txt remote", func() bool {
		return env.remoteExists(t, "offline.txt")
	})
	env.stopDaemon()

	// Delete while daemon is stopped.
	abs := filepath.Join(env.localRoot, "offline.txt")
	if err := removeFile(abs); err != nil {
		t.Fatalf("remove: %v", err)
	}

	// Restart — warm start should detect the local delete and propagate it.
	env.startDaemon(t)
	defer env.stopDaemon()

	assertEventually(t, 5*time.Second, "remote delete after restart", func() bool {
		return !env.remoteExists(t, "offline.txt")
	})
	if env.localExists("offline.txt") {
		t.Fatalf("offline.txt reappeared locally after offline delete + restart")
	}
}
