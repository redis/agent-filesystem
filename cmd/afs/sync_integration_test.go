package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/mount/client"
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

// Scenario 1c: the very first sync should refuse to merge a populated local
// directory into an already-populated remote workspace with no saved sync
// state. This catches accidental sync roots like ~/.codex.
func TestSyncStartupRejectsAmbiguousMerge(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeLocalFile(t, "local-only.txt", "hello")
	env.writeRemoteFile(t, "remote-only.txt", "world")

	daemonClient := client.New(env.rdb, env.mountKey)
	cfg := syncDaemonConfig{
		Workspace:       env.workspace,
		LocalRoot:       env.localRoot,
		FS:              daemonClient,
		Store:           env.store,
		MaxFileBytes:    16 * 1024 * 1024,
		WatcherDebounce: 20 * time.Millisecond,
	}
	d, err := newSyncDaemon(cfg)
	if err != nil {
		t.Fatalf("newSyncDaemon: %v", err)
	}
	if err := d.Start(context.Background()); err == nil {
		t.Fatal("Start() unexpectedly succeeded for ambiguous first sync")
	} else if !strings.Contains(err.Error(), "Mount blocked for workspace") {
		t.Fatalf("Start() error = %q, want ambiguous first sync rejection", err)
	}
}

func TestSyncStartupAllowsIdenticalPopulatedTrees(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeLocalFile(t, "shared.txt", "same")
	env.writeRemoteFile(t, "shared.txt", "same")

	daemonClient := client.New(env.rdb, env.mountKey)
	cfg := syncDaemonConfig{
		Workspace:       env.workspace,
		LocalRoot:       env.localRoot,
		FS:              daemonClient,
		Store:           env.store,
		MaxFileBytes:    16 * 1024 * 1024,
		WatcherDebounce: 20 * time.Millisecond,
	}
	d, err := newSyncDaemon(cfg)
	if err != nil {
		t.Fatalf("newSyncDaemon: %v", err)
	}
	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start() with identical populated trees: %v", err)
	}
	defer d.Stop()

	if got := env.readLocalFile(t, "shared.txt"); got != "same" {
		t.Fatalf("local content = %q, want %q", got, "same")
	}
	if got := env.readRemoteFile(t, "shared.txt"); got != "same" {
		t.Fatalf("remote content = %q, want %q", got, "same")
	}
}

func TestMountReconcileAllowsApprovedSafeUnion(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeLocalFile(t, "local-only.txt", "hello")
	env.writeRemoteFile(t, "remote-only.txt", "world")

	daemonClient := client.New(env.rdb, env.mountKey)
	cfg := syncDaemonConfig{
		Workspace:       env.workspace,
		LocalRoot:       env.localRoot,
		FS:              daemonClient,
		Store:           env.store,
		MaxFileBytes:    16 * 1024 * 1024,
		WatcherDebounce: 20 * time.Millisecond,
	}
	d, err := newSyncDaemon(cfg)
	if err != nil {
		t.Fatalf("newSyncDaemon: %v", err)
	}
	plan, err := buildMountReconcilePlan(context.Background(), d)
	if err != nil {
		t.Fatalf("buildMountReconcilePlan: %v", err)
	}
	if plan.ImportCount != 1 || plan.DownloadCount != 1 || plan.ConflictCount != 0 {
		t.Fatalf("plan counts = import:%d download:%d conflict:%d, want 1/1/0", plan.ImportCount, plan.DownloadCount, plan.ConflictCount)
	}
	if !plan.requiresConfirmation() {
		t.Fatal("requiresConfirmation() = false, want true for populated local safe union")
	}
	approveMountReconcilePlan(d, plan)
	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start() after approved mount plan: %v", err)
	}
	env.daemon = d
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "local-only.txt to upload", func() bool {
		return env.remoteExists(t, "local-only.txt")
	})
	assertEventually(t, 3*time.Second, "remote-only.txt to download", func() bool {
		return env.localExists("remote-only.txt")
	})
	if got := env.readRemoteFile(t, "local-only.txt"); got != "hello" {
		t.Fatalf("remote local-only.txt = %q, want hello", got)
	}
	if got := env.readLocalFile(t, "remote-only.txt"); got != "world" {
		t.Fatalf("local remote-only.txt = %q, want world", got)
	}
}

func TestMountReconcileReportsOfflineLocalCreate(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	if err := env.fsClient.Mkdir(context.Background(), "/empty"); err != nil && !isClientAlreadyExists(err) {
		t.Fatalf("Mkdir /empty: %v", err)
	}

	env.startDaemon(t)
	assertEventually(t, 3*time.Second, "empty folder to materialize", func() bool {
		return env.localExists("empty")
	})
	env.stopDaemon()

	env.writeLocalFile(t, "hello.txt", "hello")
	daemonClient := client.New(env.rdb, env.mountKey)
	cfg := syncDaemonConfig{
		Workspace:       env.workspace,
		LocalRoot:       env.localRoot,
		FS:              daemonClient,
		Store:           env.store,
		MaxFileBytes:    16 * 1024 * 1024,
		WatcherDebounce: 20 * time.Millisecond,
	}
	d, err := newSyncDaemon(cfg)
	if err != nil {
		t.Fatalf("newSyncDaemon: %v", err)
	}
	plan, err := buildMountReconcilePlan(context.Background(), d)
	if err != nil {
		t.Fatalf("buildMountReconcilePlan: %v", err)
	}
	if plan.StateCount == 0 {
		t.Fatal("StateCount = 0, want existing sync baseline")
	}
	if plan.UploadCount != 1 || plan.DownloadCount != 0 || plan.ConflictCount != 0 {
		t.Fatalf("plan counts = upload:%d download:%d conflict:%d, want 1/0/0", plan.UploadCount, plan.DownloadCount, plan.ConflictCount)
	}
	if len(plan.Operations) != 1 || plan.Operations[0].Code != "U" || plan.Operations[0].Path != "hello.txt" {
		t.Fatalf("operations = %#v, want one upload for hello.txt", plan.Operations)
	}

	approveMountReconcilePlan(d, plan)
	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start() after approved offline local create: %v", err)
	}
	env.daemon = d
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "hello.txt to upload", func() bool {
		return env.remoteExists(t, "hello.txt")
	})
	if got := env.readRemoteFile(t, "hello.txt"); got != "hello" {
		t.Fatalf("remote hello.txt = %q, want hello", got)
	}
}

func TestMountReconcileReportsSamePathConflict(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeLocalFile(t, "shared.txt", "local")
	env.writeRemoteFile(t, "shared.txt", "remote")

	daemonClient := client.New(env.rdb, env.mountKey)
	cfg := syncDaemonConfig{
		Workspace:       env.workspace,
		LocalRoot:       env.localRoot,
		FS:              daemonClient,
		Store:           env.store,
		MaxFileBytes:    16 * 1024 * 1024,
		WatcherDebounce: 20 * time.Millisecond,
	}
	d, err := newSyncDaemon(cfg)
	if err != nil {
		t.Fatalf("newSyncDaemon: %v", err)
	}
	plan, err := buildMountReconcilePlan(context.Background(), d)
	if err != nil {
		t.Fatalf("buildMountReconcilePlan: %v", err)
	}
	if plan.ConflictCount != 1 || plan.ImportCount != 0 || plan.DownloadCount != 0 {
		t.Fatalf("plan counts = conflict:%d import:%d download:%d, want 1/0/0", plan.ConflictCount, plan.ImportCount, plan.DownloadCount)
	}
	if len(plan.Operations) != 1 || plan.Operations[0].Code != "C" || plan.Operations[0].Path != "shared.txt" {
		t.Fatalf("operations = %#v, want one conflict for shared.txt", plan.Operations)
	}
}

func TestSyncImportMountUnmountRemountLocalChangeMatrix(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)

	sourceDir := t.TempDir()
	writeTreeFile(t, sourceDir, "docs/readme.md", "readme v1\n")
	writeTreeFile(t, sourceDir, "docs/remove-while-mounted.txt", "mounted delete\n")
	writeTreeFile(t, sourceDir, "keep/remove-while-unmounted.txt", "unmounted delete\n")
	writeTreeFile(t, sourceDir, "tree/remove-me/file.txt", "remove subtree\n")
	mkdirTreeDir(t, sourceDir, "empty-folder")
	importDirectoryIntoSyncWorkspace(t, env, sourceDir)

	env.startDaemon(t)
	assertEventually(t, 3*time.Second, "imported tree to materialize locally", func() bool {
		return env.localExists("docs/readme.md") &&
			env.localExists("docs/remove-while-mounted.txt") &&
			env.localExists("empty-folder")
	})

	if err := os.Remove(filepath.Join(env.localRoot, "docs", "remove-while-mounted.txt")); err != nil {
		t.Fatalf("remove mounted file: %v", err)
	}
	assertEventually(t, 3*time.Second, "mounted local delete to remove remote file", func() bool {
		return !env.remoteExists(t, "docs/remove-while-mounted.txt")
	})
	env.stopDaemon()

	env.writeLocalFile(t, "unmounted-added.txt", "added while unmounted\n")
	env.writeLocalFile(t, "unmounted/new-folder/nested.txt", "nested add while unmounted\n")
	env.writeLocalFile(t, "docs/readme.md", "readme v2\n")
	removeTreePath(t, env.localRoot, "keep/remove-while-unmounted.txt")
	removeTreePath(t, env.localRoot, "tree/remove-me")
	removeTreePath(t, env.localRoot, "empty-folder")
	mkdirTreeDir(t, env.localRoot, "new-empty-folder")

	d := newTestMountDaemon(t, env)
	plan, err := buildMountReconcilePlan(context.Background(), d)
	if err != nil {
		t.Fatalf("buildMountReconcilePlan: %v", err)
	}
	if plan.StateCount == 0 {
		t.Fatal("StateCount = 0, want existing sync baseline")
	}
	if plan.ConflictCount != 0 {
		t.Fatalf("ConflictCount = %d, want 0; operations = %#v", plan.ConflictCount, plan.Operations)
	}
	requireMountOp(t, plan, "U", "unmounted-added.txt")
	requireMountOp(t, plan, "U", "unmounted")
	requireMountOp(t, plan, "U", "unmounted/new-folder")
	requireMountOp(t, plan, "U", "unmounted/new-folder/nested.txt")
	requireMountOp(t, plan, "U", "docs/readme.md")
	requireMountOp(t, plan, "U", "new-empty-folder")
	requireMountOp(t, plan, "DR", "keep/remove-while-unmounted.txt")
	requireMountOp(t, plan, "DR", "tree/remove-me/file.txt")
	requireMountOp(t, plan, "DR", "tree/remove-me")
	requireMountOp(t, plan, "DR", "empty-folder")

	approveMountReconcilePlan(d, plan)
	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start() after approved local unmounted matrix: %v", err)
	}
	env.daemon = d
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "local unmounted matrix to converge", func() bool {
		return localRemoteTreesEqual(t, env)
	})
	assertTreeHasFile(t, env, "docs/readme.md", "readme v2\n")
	assertTreeHasFile(t, env, "unmounted-added.txt", "added while unmounted\n")
	assertTreeHasFile(t, env, "unmounted/new-folder/nested.txt", "nested add while unmounted\n")
	assertTreeHasDir(t, env, "new-empty-folder")
	assertTreeMissing(t, env, "docs/remove-while-mounted.txt")
	assertTreeMissing(t, env, "keep/remove-while-unmounted.txt")
	assertTreeMissing(t, env, "tree/remove-me")
	assertTreeMissing(t, env, "empty-folder")
}

func TestSyncImportMountUnmountRemountRemoteChangeMatrix(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)

	sourceDir := t.TempDir()
	writeTreeFile(t, sourceDir, "docs/readme.md", "readme v1\n")
	writeTreeFile(t, sourceDir, "local/remove-after-remote-delete.txt", "remote delete\n")
	writeTreeFile(t, sourceDir, "remote-empty/remove-me/.keep", "keeps parent non-empty for import\n")
	mkdirTreeDir(t, sourceDir, "remote-empty/delete-empty")
	importDirectoryIntoSyncWorkspace(t, env, sourceDir)

	env.startDaemon(t)
	assertEventually(t, 3*time.Second, "initial remote matrix tree locally", func() bool {
		return env.localExists("docs/readme.md") &&
			env.localExists("local/remove-after-remote-delete.txt") &&
			env.localExists("remote-empty/delete-empty")
	})
	env.stopDaemon()

	env.writeRemoteFile(t, "remote-added.txt", "remote add\n")
	env.writeRemoteFile(t, "remote/new-folder/nested.txt", "remote nested add\n")
	env.writeRemoteFile(t, "docs/readme.md", "readme remote v2\n")
	if err := env.fsClient.Mkdir(context.Background(), "/remote-new-empty"); err != nil && !isClientAlreadyExists(err) {
		t.Fatalf("Mkdir /remote-new-empty: %v", err)
	}
	removeRemotePath(t, env, "local/remove-after-remote-delete.txt")
	removeRemotePath(t, env, "remote-empty/delete-empty")

	d := newTestMountDaemon(t, env)
	plan, err := buildMountReconcilePlan(context.Background(), d)
	if err != nil {
		t.Fatalf("buildMountReconcilePlan: %v", err)
	}
	if plan.StateCount == 0 {
		t.Fatal("StateCount = 0, want existing sync baseline")
	}
	if plan.ConflictCount != 0 {
		t.Fatalf("ConflictCount = %d, want 0; operations = %#v", plan.ConflictCount, plan.Operations)
	}
	requireMountOp(t, plan, "D", "remote-added.txt")
	requireMountOp(t, plan, "D", "remote")
	requireMountOp(t, plan, "D", "remote/new-folder")
	requireMountOp(t, plan, "D", "remote/new-folder/nested.txt")
	requireMountOp(t, plan, "D", "docs/readme.md")
	requireMountOp(t, plan, "D", "remote-new-empty")
	requireMountOp(t, plan, "DL", "local/remove-after-remote-delete.txt")
	requireMountOp(t, plan, "DL", "remote-empty/delete-empty")

	approveMountReconcilePlan(d, plan)
	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start() after approved remote unmounted matrix: %v", err)
	}
	env.daemon = d
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "remote unmounted matrix to converge", func() bool {
		return localRemoteTreesEqual(t, env)
	})
	assertTreeHasFile(t, env, "docs/readme.md", "readme remote v2\n")
	assertTreeHasFile(t, env, "remote-added.txt", "remote add\n")
	assertTreeHasFile(t, env, "remote/new-folder/nested.txt", "remote nested add\n")
	assertTreeHasDir(t, env, "remote-new-empty")
	assertTreeMissing(t, env, "local/remove-after-remote-delete.txt")
	assertTreeMissing(t, env, "remote-empty/delete-empty")
}

func TestSyncRemountReportsUnmountedConflictMatrix(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)

	sourceDir := t.TempDir()
	writeTreeFile(t, sourceDir, "both-change.txt", "base\n")
	writeTreeFile(t, sourceDir, "local-delete-remote-change.txt", "base\n")
	importDirectoryIntoSyncWorkspace(t, env, sourceDir)

	env.startDaemon(t)
	assertEventually(t, 3*time.Second, "conflict baseline locally", func() bool {
		return env.localExists("both-change.txt") && env.localExists("local-delete-remote-change.txt")
	})
	env.stopDaemon()

	env.writeLocalFile(t, "both-change.txt", "local v2\n")
	if err := env.fsClient.Echo(context.Background(), "/both-change.txt", []byte("remote v2\n")); err != nil {
		t.Fatalf("remote echo both-change.txt: %v", err)
	}
	removeTreePath(t, env.localRoot, "local-delete-remote-change.txt")
	if err := env.fsClient.Echo(context.Background(), "/local-delete-remote-change.txt", []byte("remote changed after local delete\n")); err != nil {
		t.Fatalf("remote echo local-delete-remote-change.txt: %v", err)
	}

	d := newTestMountDaemon(t, env)
	plan, err := buildMountReconcilePlan(context.Background(), d)
	if err != nil {
		t.Fatalf("buildMountReconcilePlan: %v", err)
	}
	if plan.ConflictCount != 2 {
		t.Fatalf("ConflictCount = %d, want 2; operations = %#v", plan.ConflictCount, plan.Operations)
	}
	requireMountOp(t, plan, "C", "both-change.txt")
	requireMountOp(t, plan, "C", "local-delete-remote-change.txt")
}

func TestMountReconcileCompletesPendingTombstoneDelete(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeRemoteFile(t, "leftover.txt", "remote still present\n")

	st := newSyncState(env.workspace, env.localRoot)
	st.Entries["leftover.txt"] = SyncEntry{
		Type:         "file",
		Size:         int64(len("remote still present\n")),
		Deleted:      true,
		LastSyncedAt: time.Now().UTC(),
	}
	if err := saveSyncState(st); err != nil {
		t.Fatalf("saveSyncState() returned error: %v", err)
	}

	d := newTestMountDaemon(t, env)
	plan, err := buildMountReconcilePlan(context.Background(), d)
	if err != nil {
		t.Fatalf("buildMountReconcilePlan: %v", err)
	}
	if plan.DownloadCount != 0 || plan.DeleteRemoteCount != 1 || plan.ConflictCount != 0 {
		t.Fatalf("plan counts = download:%d deleteRemote:%d conflict:%d, want 0/1/0; operations = %#v", plan.DownloadCount, plan.DeleteRemoteCount, plan.ConflictCount, plan.Operations)
	}
	if plan.StateLiveCount != 0 || plan.StateDeletedCount != 1 {
		t.Fatalf("state counts = live:%d deleted:%d, want 0/1", plan.StateLiveCount, plan.StateDeletedCount)
	}
	requireMountOp(t, plan, "DR", "leftover.txt")

	approveMountReconcilePlan(d, plan)
	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start() after approved pending tombstone delete: %v", err)
	}
	env.daemon = d
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "pending tombstone delete to remove remote", func() bool {
		return !env.remoteExists(t, "leftover.txt")
	})
	if env.localExists("leftover.txt") {
		t.Fatalf("leftover.txt was downloaded locally despite tombstone")
	}
}

func TestMountReconcileMissingLocalRootDownloadsInsteadOfRemoteDelete(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeRemoteFile(t, "README.md", "restored\n")

	st := newSyncState(env.workspace, env.localRoot)
	st.Entries["README.md"] = SyncEntry{
		Type:         "file",
		Size:         int64(len("restored\n")),
		LastSyncedAt: time.Now().UTC(),
	}
	if err := saveSyncState(st); err != nil {
		t.Fatalf("saveSyncState() returned error: %v", err)
	}
	if err := os.RemoveAll(env.localRoot); err != nil {
		t.Fatalf("RemoveAll(localRoot) returned error: %v", err)
	}
	localSnapshot, err := inspectMountLocalRoot(env.localRoot)
	if err != nil {
		t.Fatalf("inspectMountLocalRoot() returned error: %v", err)
	}
	if localSnapshot.Exists || localSnapshot.EntryCount != 0 {
		t.Fatalf("localSnapshot = %#v, want missing empty root", localSnapshot)
	}

	d := newTestMountDaemon(t, env)
	plan, err := buildMountReconcilePlan(context.Background(), d)
	if err != nil {
		t.Fatalf("buildMountReconcilePlan: %v", err)
	}
	if !mountPlanDeletesRemoteFromEmptyLocal(plan, localSnapshot) {
		t.Fatalf("plan should be recognized as empty-local remote delete: %#v", plan)
	}

	resetMountSyncState(d)
	plan, err = buildMountReconcilePlan(context.Background(), d)
	if err != nil {
		t.Fatalf("buildMountReconcilePlan after reset: %v", err)
	}
	if plan.DownloadCount != 1 || plan.DeleteRemoteCount != 0 || plan.ConflictCount != 0 {
		t.Fatalf("plan counts = download:%d deleteRemote:%d conflict:%d, want 1/0/0; operations = %#v", plan.DownloadCount, plan.DeleteRemoteCount, plan.ConflictCount, plan.Operations)
	}
	requireMountOp(t, plan, "D", "README.md")

	approveMountReconcilePlan(d, plan)
	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start() after empty-root mount reset: %v", err)
	}
	env.daemon = d
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "README.md to download locally", func() bool {
		return env.localExists("README.md")
	})
	if got := env.readLocalFile(t, "README.md"); got != "restored\n" {
		t.Fatalf("local README.md = %q, want restored", got)
	}
	if got := env.readRemoteFile(t, "README.md"); got != "restored\n" {
		t.Fatalf("remote README.md = %q, want restored", got)
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

func TestSyncHistoryDeleteRecordsTombstone(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	ctx := context.Background()
	if err := env.cp.PutWorkspaceVersioningPolicy(ctx, env.workspace, controlplane.WorkspaceVersioningPolicy{
		Mode: controlplane.WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy: %v", err)
	}
	meta, err := env.store.getWorkspaceMeta(ctx, env.workspace)
	if err != nil {
		t.Fatalf("getWorkspaceMeta: %v", err)
	}
	storageID := controlplane.WorkspaceStorageID(meta)
	service := controlPlaneServiceFromStore(defaultConfig(), env.store)
	env.startDaemon(t, func(c *syncDaemonConfig) {
		c.Rdb = env.rdb
		c.StorageID = storageID
		c.SessionID = "sess-history-delete"
		c.AgentID = "agt-history-delete"
		c.Label = "sync/history"
		c.AgentVersion = "test"
	})
	defer env.stopDaemon()

	abs := env.writeLocalFile(t, "deleted.txt", "first\nsecond\n")
	assertEventually(t, 3*time.Second, "deleted.txt to land remotely", func() bool {
		return env.remoteExists(t, "deleted.txt")
	})

	if err := removeFile(abs); err != nil {
		t.Fatalf("remove local deleted.txt: %v", err)
	}
	assertEventually(t, 3*time.Second, "deleted.txt remote delete", func() bool {
		return !env.remoteExists(t, "deleted.txt")
	})

	var history controlplane.FileHistoryResponse
	assertEventually(t, 3*time.Second, "deleted.txt history tombstone", func() bool {
		var getErr error
		history, getErr = service.GetFileHistory(ctx, env.workspace, "/deleted.txt", false)
		if getErr != nil || len(history.Lineages) != 1 || len(history.Lineages[0].Versions) != 2 {
			return false
		}
		lineage := history.Lineages[0]
		return lineage.State == controlplane.FileLineageStateDeleted &&
			lineage.Versions[0].Op == controlplane.ChangeOpPut &&
			lineage.Versions[1].Op == controlplane.ChangeOpDelete
	})

	if _, err := env.cp.ResolveLiveFileLineageByPath(ctx, env.workspace, "/deleted.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ResolveLiveFileLineageByPath(/deleted.txt) error = %v, want os.ErrNotExist", err)
	}
}

func TestCmdFileUndeleteCoordinatesWithActiveSyncDaemon(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	ctx := context.Background()
	if err := env.cp.PutWorkspaceVersioningPolicy(ctx, env.workspace, controlplane.WorkspaceVersioningPolicy{
		Mode: controlplane.WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy: %v", err)
	}
	meta, err := env.store.getWorkspaceMeta(ctx, env.workspace)
	if err != nil {
		t.Fatalf("getWorkspaceMeta: %v", err)
	}
	storageID := controlplane.WorkspaceStorageID(meta)
	service := controlPlaneServiceFromStore(defaultConfig(), env.store)

	env.startDaemon(t, func(c *syncDaemonConfig) {
		c.Rdb = env.rdb
		c.StorageID = storageID
		c.SessionID = "sess-history-undelete"
		c.AgentID = "agt-history-undelete"
		c.Label = "sync/history"
		c.AgentVersion = "test"
	})
	defer env.stopDaemon()

	abs := env.writeLocalFile(t, "undelete.txt", "bring me back\n")
	assertEventually(t, 3*time.Second, "undelete.txt to land remotely", func() bool {
		return env.remoteExists(t, "undelete.txt")
	})

	if err := removeFile(abs); err != nil {
		t.Fatalf("remove local undelete.txt: %v", err)
	}
	assertEventually(t, 3*time.Second, "undelete.txt remote delete", func() bool {
		return !env.remoteExists(t, "undelete.txt")
	})

	oldCfgPathOverride := cfgPathOverride
	cfgPathOverride = filepath.Join(t.TempDir(), "afs.config.json")
	t.Cleanup(func() {
		cfgPathOverride = oldCfgPathOverride
	})

	cfg := defaultConfig()
	cfg.ProductMode = productModeLocal
	cfg.Mode = modeSync
	cfg.RedisAddr = env.mr.Addr()
	cfg.RedisDB = 0
	cfg.LocalPath = env.localRoot
	cfg.CurrentWorkspace = env.workspace
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig() returned error: %v", err)
	}

	st := state{
		ProductMode:      productModeLocal,
		RedisAddr:        env.mr.Addr(),
		RedisDB:          0,
		CurrentWorkspace: env.workspace,
		LocalPath:        env.localRoot,
		Mode:             modeSync,
		SyncPID:          os.Getpid(),
	}
	if err := saveState(st); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	if err := cmdFile([]string{"file", "undelete", "repo", "/undelete.txt"}); err != nil {
		t.Fatalf("cmdFile(undelete) returned error: %v", err)
	}

	assertEventually(t, 3*time.Second, "undelete.txt restored locally and remotely", func() bool {
		return env.localExists("undelete.txt") && env.remoteExists(t, "undelete.txt")
	})
	if got := env.readLocalFile(t, "undelete.txt"); got != "bring me back\n" {
		t.Fatalf("local content after undelete = %q, want %q", got, "bring me back\n")
	}
	if got := env.readRemoteFile(t, "undelete.txt"); got != "bring me back\n" {
		t.Fatalf("remote content after undelete = %q, want %q", got, "bring me back\n")
	}

	history, err := service.GetFileHistory(ctx, env.workspace, "/undelete.txt", false)
	if err != nil {
		t.Fatalf("GetFileHistory(/undelete.txt) returned error: %v", err)
	}
	if len(history.Lineages) != 1 {
		t.Fatalf("len(history.Lineages) = %d, want 1", len(history.Lineages))
	}
	versions := history.Lineages[0].Versions
	if len(versions) != 3 {
		t.Fatalf("len(versions) = %d, want 3", len(versions))
	}
	if got := versions[2].Op; got != controlplane.ChangeOpPut {
		t.Fatalf("latest undelete op = %q, want %q", got, controlplane.ChangeOpPut)
	}
	if got := versions[2].Source; got != controlplane.ChangeSourceVersionUndelete {
		t.Fatalf("latest undelete source = %q, want %q", got, controlplane.ChangeSourceVersionUndelete)
	}
}

func TestSyncHistoryRenamePreservesLineage(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	ctx := context.Background()
	if err := env.cp.PutWorkspaceVersioningPolicy(ctx, env.workspace, controlplane.WorkspaceVersioningPolicy{
		Mode: controlplane.WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy: %v", err)
	}
	meta, err := env.store.getWorkspaceMeta(ctx, env.workspace)
	if err != nil {
		t.Fatalf("getWorkspaceMeta: %v", err)
	}
	storageID := controlplane.WorkspaceStorageID(meta)
	service := controlPlaneServiceFromStore(defaultConfig(), env.store)
	env.startDaemon(t, func(c *syncDaemonConfig) {
		c.Rdb = env.rdb
		c.StorageID = storageID
		c.SessionID = "sess-history-rename"
		c.AgentID = "agt-history-rename"
		c.Label = "sync/history"
		c.AgentVersion = "test"
	})
	defer env.stopDaemon()

	oldAbs := env.writeLocalFile(t, "history.txt", "rename me\n")
	assertEventually(t, 3*time.Second, "history.txt to land remotely", func() bool {
		return env.remoteExists(t, "history.txt")
	})

	newAbs := filepath.Join(env.localRoot, "renamed.txt")
	if err := os.Rename(oldAbs, newAbs); err != nil {
		t.Fatalf("os.Rename(history.txt -> renamed.txt): %v", err)
	}
	if err := os.WriteFile(newAbs, []byte("rename me\nand update me\n"), 0o644); err != nil {
		t.Fatalf("rewrite renamed.txt: %v", err)
	}

	assertEventually(t, 3*time.Second, "renamed.txt remote rename", func() bool {
		return env.remoteExists(t, "renamed.txt") && !env.remoteExists(t, "history.txt")
	})
	assertEventually(t, 3*time.Second, "renamed.txt remote content update", func() bool {
		return env.readRemoteFile(t, "renamed.txt") == "rename me\nand update me\n"
	})

	var oldHistory controlplane.FileHistoryResponse
	assertEventually(t, 3*time.Second, "old path rename history", func() bool {
		var getErr error
		oldHistory, getErr = service.GetFileHistory(ctx, env.workspace, "/history.txt", false)
		if getErr != nil || len(oldHistory.Lineages) != 1 || len(oldHistory.Lineages[0].Versions) != 2 {
			return false
		}
		return oldHistory.Lineages[0].Versions[0].Op == controlplane.ChangeOpPut &&
			oldHistory.Lineages[0].Versions[1].Op == controlplane.ChangeOpRename &&
			oldHistory.Lineages[0].Versions[1].PrevPath == "/history.txt" &&
			oldHistory.Lineages[0].Versions[1].Path == "/renamed.txt"
	})

	newHistory, err := service.GetFileHistory(ctx, env.workspace, "/renamed.txt", false)
	if err != nil {
		t.Fatalf("GetFileHistory(/renamed.txt) returned error: %v", err)
	}
	if len(newHistory.Lineages) != 1 {
		t.Fatalf("len(newHistory.Lineages) = %d, want 1", len(newHistory.Lineages))
	}
	if got := newHistory.Lineages[0].Versions[len(newHistory.Lineages[0].Versions)-1].Op; got != controlplane.ChangeOpPut {
		t.Fatalf("latest renamed.txt op = %q, want %q", got, controlplane.ChangeOpPut)
	}
	if newHistory.Lineages[0].FileID != oldHistory.Lineages[0].FileID {
		t.Fatalf("renamed file_id = %q, want %q", newHistory.Lineages[0].FileID, oldHistory.Lineages[0].FileID)
	}
}

func TestSyncHistoryExistingTrackedFileSeedsBaseline(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	ctx := context.Background()
	if err := env.cp.PutWorkspaceVersioningPolicy(ctx, env.workspace, controlplane.WorkspaceVersioningPolicy{
		Mode: controlplane.WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy: %v", err)
	}
	meta, err := env.store.getWorkspaceMeta(ctx, env.workspace)
	if err != nil {
		t.Fatalf("getWorkspaceMeta: %v", err)
	}
	storageID := controlplane.WorkspaceStorageID(meta)

	env.writeRemoteFile(t, "existing.txt", "before\n")
	env.startDaemon(t, func(c *syncDaemonConfig) {
		c.Rdb = env.rdb
		c.StorageID = storageID
		c.SessionID = "sess-history-existing"
		c.AgentID = "agt-history-existing"
		c.Label = "sync/history-existing"
		c.AgentVersion = "test"
	})
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "existing.txt local materialized", func() bool {
		return env.localExists("existing.txt") && env.readLocalFile(t, "existing.txt") == "before\n"
	})

	env.writeLocalFile(t, "existing.txt", "after\n")

	assertEventually(t, 3*time.Second, "existing.txt updated remotely", func() bool {
		return env.remoteExists(t, "existing.txt") && env.readRemoteFile(t, "existing.txt") == "after\n"
	})

	service := controlPlaneServiceFromStore(defaultConfig(), env.store)
	var history controlplane.FileHistoryResponse
	assertEventually(t, 3*time.Second, "existing.txt history", func() bool {
		var getErr error
		history, getErr = service.GetFileHistory(ctx, env.workspace, "/existing.txt", false)
		return getErr == nil && len(history.Lineages) == 1 && len(history.Lineages[0].Versions) == 2
	})

	if got := history.Lineages[0].Versions[0].Op; got != controlplane.ChangeOpPut {
		t.Fatalf("baseline op = %q, want %q", got, controlplane.ChangeOpPut)
	}
	if got := history.Lineages[0].Versions[1].Op; got != controlplane.ChangeOpPut {
		t.Fatalf("latest op = %q, want %q", got, controlplane.ChangeOpPut)
	}
	if got := history.Lineages[0].Versions[0].ContentHash; got != sha256Hex([]byte("before\n")) {
		t.Fatalf("baseline content hash = %q, want %q", got, sha256Hex([]byte("before\n")))
	}
	if got := history.Lineages[0].Versions[1].ContentHash; got != sha256Hex([]byte("after\n")) {
		t.Fatalf("latest content hash = %q, want %q", got, sha256Hex([]byte("after\n")))
	}
}

func TestSyncHistoryExistingTrackedFileAfterPolicyEnabledLate(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	ctx := context.Background()
	meta, err := env.store.getWorkspaceMeta(ctx, env.workspace)
	if err != nil {
		t.Fatalf("getWorkspaceMeta: %v", err)
	}
	storageID := controlplane.WorkspaceStorageID(meta)

	env.writeRemoteFile(t, "existing.txt", "before\n")
	env.startDaemon(t, func(c *syncDaemonConfig) {
		c.Rdb = env.rdb
		c.StorageID = storageID
		c.SessionID = "sess-history-existing-late"
		c.AgentID = "agt-history-existing-late"
		c.Label = "sync/history-existing-late"
		c.AgentVersion = "test"
	})
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "existing.txt local materialized", func() bool {
		return env.localExists("existing.txt") && env.readLocalFile(t, "existing.txt") == "before\n"
	})

	if err := env.cp.PutWorkspaceVersioningPolicy(ctx, env.workspace, controlplane.WorkspaceVersioningPolicy{
		Mode: controlplane.WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy late: %v", err)
	}

	env.writeLocalFile(t, "existing.txt", "after\n")

	assertEventually(t, 3*time.Second, "existing.txt updated remotely", func() bool {
		return env.remoteExists(t, "existing.txt") && env.readRemoteFile(t, "existing.txt") == "after\n"
	})

	service := controlPlaneServiceFromStore(defaultConfig(), env.store)
	var history controlplane.FileHistoryResponse
	assertEventually(t, 3*time.Second, "existing.txt late-enabled history", func() bool {
		var getErr error
		history, getErr = service.GetFileHistory(ctx, env.workspace, "/existing.txt", false)
		return getErr == nil && len(history.Lineages) == 1 && len(history.Lineages[0].Versions) == 2
	})

	if got := history.Lineages[0].Versions[0].ContentHash; got != sha256Hex([]byte("before\n")) {
		t.Fatalf("baseline content hash = %q, want %q", got, sha256Hex([]byte("before\n")))
	}
	if got := history.Lineages[0].Versions[1].ContentHash; got != sha256Hex([]byte("after\n")) {
		t.Fatalf("latest content hash = %q, want %q", got, sha256Hex([]byte("after\n")))
	}
}

func TestSyncHistorySymlinkDeleteProducesTombstone(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	ctx := context.Background()
	if err := env.cp.PutWorkspaceVersioningPolicy(ctx, env.workspace, controlplane.WorkspaceVersioningPolicy{
		Mode: controlplane.WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy: %v", err)
	}
	meta, err := env.store.getWorkspaceMeta(ctx, env.workspace)
	if err != nil {
		t.Fatalf("getWorkspaceMeta: %v", err)
	}
	storageID := controlplane.WorkspaceStorageID(meta)
	service := controlPlaneServiceFromStore(defaultConfig(), env.store)

	env.startDaemon(t, func(c *syncDaemonConfig) {
		c.Rdb = env.rdb
		c.StorageID = storageID
		c.SessionID = "sess-history-symlink-delete"
		c.AgentID = "agt-history-symlink-delete"
		c.Label = "sync/history-symlink-delete"
		c.AgentVersion = "test"
	})
	defer env.stopDaemon()

	targetAbs := env.writeLocalFile(t, "target.txt", "alpha\n")
	assertEventually(t, 3*time.Second, "target.txt remote", func() bool {
		return env.remoteExists(t, "target.txt")
	})

	linkAbs := filepath.Join(env.localRoot, "link.txt")
	if err := os.Symlink(filepath.Base(targetAbs), linkAbs); err != nil {
		t.Fatalf("os.Symlink(link.txt): %v", err)
	}

	assertEventually(t, 3*time.Second, "link.txt symlink history", func() bool {
		history, getErr := service.GetFileHistory(ctx, env.workspace, "/link.txt", false)
		return getErr == nil && len(history.Lineages) == 1 && len(history.Lineages[0].Versions) == 1
	})

	if err := os.Remove(linkAbs); err != nil {
		t.Fatalf("os.Remove(link.txt): %v", err)
	}

	var history controlplane.FileHistoryResponse
	assertEventually(t, 3*time.Second, "link.txt deleted history", func() bool {
		var getErr error
		history, getErr = service.GetFileHistory(ctx, env.workspace, "/link.txt", false)
		return getErr == nil &&
			len(history.Lineages) == 1 &&
			history.Lineages[0].State == controlplane.FileLineageStateDeleted &&
			len(history.Lineages[0].Versions) == 2 &&
			history.Lineages[0].Versions[1].Op == controlplane.ChangeOpDelete
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

func TestSyncCreateExclusiveRequestCreatesRemoteAndLocal(t *testing.T) {
	t.Helper()

	env := newSyncTestEnv(t)
	env.startDaemon(t)
	defer env.stopDaemon()

	requestID := "create-lock"
	request := syncControlRequest{
		Version:   syncControlVersion,
		Operation: syncControlOpCreateExclusive,
		Path:      "/locks/deploy.lock",
		Content:   "agent-a\n",
	}
	if err := writeSyncControlJSON(syncControlRequestPath(env.localRoot, requestID), request, 0o600); err != nil {
		t.Fatalf("writeSyncControlJSON(request) returned error: %v", err)
	}

	resultPath := syncControlResultPath(env.localRoot, requestID)
	assertEventually(t, 3*time.Second, "sync control result", func() bool {
		_, err := os.Stat(resultPath)
		return err == nil
	})

	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("ReadFile(result) returned error: %v", err)
	}
	var result syncControlResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(result) returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("result = %+v, want success", result)
	}

	assertEventually(t, 3*time.Second, "local lock file", func() bool {
		return env.localExists("locks/deploy.lock")
	})
	if got := env.readLocalFile(t, "locks/deploy.lock"); got != "agent-a\n" {
		t.Fatalf("local content = %q, want %q", got, "agent-a\n")
	}
	if got := env.readRemoteFile(t, "locks/deploy.lock"); got != "agent-a\n" {
		t.Fatalf("remote content = %q, want %q", got, "agent-a\n")
	}
}

func TestCmdFileCreateExclusiveRoundTrip(t *testing.T) {
	t.Helper()

	env := newSyncTestEnv(t)
	env.startDaemon(t)
	defer env.stopDaemon()

	oldCfgPathOverride := cfgPathOverride
	cfgPathOverride = filepath.Join(t.TempDir(), "afs.config.json")
	t.Cleanup(func() {
		cfgPathOverride = oldCfgPathOverride
	})

	cfg := defaultConfig()
	cfg.ProductMode = productModeLocal
	cfg.Mode = modeSync
	cfg.RedisAddr = env.mr.Addr()
	cfg.RedisDB = 0
	cfg.LocalPath = env.localRoot
	cfg.CurrentWorkspace = env.workspace
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig() returned error: %v", err)
	}

	st := state{
		ProductMode:      productModeLocal,
		RedisAddr:        env.mr.Addr(),
		RedisDB:          0,
		CurrentWorkspace: env.workspace,
		LocalPath:        env.localRoot,
		Mode:             modeSync,
		SyncPID:          os.Getpid(),
	}
	if err := saveState(st); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	if err := cmdFS([]string{"fs", "create-exclusive", "--content", "agent-b\n", "/tasks/002.claim"}); err != nil {
		t.Fatalf("cmdFS(create-exclusive) returned error: %v", err)
	}
	assertEventually(t, 3*time.Second, "remote 002.claim", func() bool {
		return env.remoteExists(t, "tasks/002.claim")
	})
	if got := env.readRemoteFile(t, "tasks/002.claim"); got != "agent-b\n" {
		t.Fatalf("remote content = %q, want %q", got, "agent-b\n")
	}

	if err := cmdFS([]string{"fs", "create-exclusive", "/tasks/002.claim"}); err == nil {
		t.Fatal("second cmdFS(create-exclusive) should fail, got success")
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

func TestSyncRootReplaceRematerializesLocalTree(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)

	orig := checkOpenHandlesUnderPath
	checkOpenHandlesUnderPath = func(root string) ([]openFileHandle, error) {
		return nil, nil
	}
	t.Cleanup(func() { checkOpenHandlesUnderPath = orig })

	env.writeLocalFile(t, "current.txt", "current")
	env.startDaemon(t)
	defer env.stopDaemon()

	assertEventually(t, 3*time.Second, "current.txt remote", func() bool {
		return env.remoteExists(t, "current.txt")
	})

	ctx := testCtx()
	restored := []byte("restored")
	if err := env.cp.PutWorkspaceMeta(ctx, controlplane.WorkspaceMeta{Name: env.workspace, HeadSavepoint: "restored"}); err != nil {
		t.Fatalf("PutWorkspaceMeta(restored): %v", err)
	}
	if err := controlplane.SyncWorkspaceRoot(ctx, env.cp, env.workspace, controlplane.Manifest{
		Workspace: env.workspace,
		Savepoint: "restored",
		Entries: map[string]controlplane.ManifestEntry{
			"/":             {Type: "dir", Mode: 0o755},
			"/restored.txt": {Type: "file", Mode: 0o644, Size: int64(len(restored)), Inline: base64.StdEncoding.EncodeToString(restored)},
		},
	}); err != nil {
		t.Fatalf("SyncWorkspaceRoot(restored): %v", err)
	}
	if err := client.PublishInvalidation(ctx, env.rdb, env.mountKey, client.InvalidateEvent{
		Origin: "control-plane",
		Op:     client.InvalidateOpRootReplace,
		Paths:  []string{"/"},
	}); err != nil {
		t.Fatalf("PublishInvalidation(root replace): %v", err)
	}

	assertEventually(t, 5*time.Second, "restored tree to materialize", func() bool {
		return env.localExists("restored.txt") && env.readLocalFile(t, "restored.txt") == "restored" && !env.localExists("current.txt")
	})
	if env.remoteExists(t, "current.txt") {
		t.Fatalf("current.txt was re-uploaded after root replace")
	}
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

// Regression: when another system deletes a file from Redis, a full
// reconciliation on this system would see the file locally but not remotely
// and re-upload it — undoing the remote delete. The fullReconciler must
// detect that a previously-synced file vanished from remote and delete
// it locally instead.
func TestSyncRemoteDeleteNotReuploadedByFullReconcile(t *testing.T) {
	t.Helper()
	env := newSyncTestEnv(t)
	env.writeLocalFile(t, "shared.txt", "hello")

	env.startDaemon(t)
	defer env.stopDaemon()

	// Wait for the file to be fully synced to remote.
	assertEventually(t, 3*time.Second, "shared.txt remote", func() bool {
		return env.remoteExists(t, "shared.txt")
	})

	// Simulate another system deleting from Redis directly.
	if err := env.fsClient.Rm(testCtx(), absoluteRemotePath("shared.txt")); err != nil {
		t.Fatalf("remote rm: %v", err)
	}

	// Force a full reconciliation — simulates a subscription reconnect.
	env.daemon.reconciler.requestFullSweep()

	// The file must be deleted locally, NOT re-uploaded to remote.
	assertEventually(t, 5*time.Second, "local delete propagated from remote", func() bool {
		return !env.localExists("shared.txt")
	})
	// Verify it didn't get re-uploaded.
	if env.remoteExists(t, "shared.txt") {
		t.Fatalf("shared.txt was re-uploaded after remote delete + full reconcile")
	}
}

func importDirectoryIntoSyncWorkspace(t *testing.T, env *syncTestEnv, sourceDir string) {
	t.Helper()
	m := seedWorkspaceFromDirectory(t, env.store, env.workspace, "initial", sourceDir)
	if err := env.store.syncWorkspaceRoot(context.Background(), env.workspace, m); err != nil {
		t.Fatalf("syncWorkspaceRoot: %v", err)
	}
	env.fsClient.InvalidateCache()
}

func newTestMountDaemon(t *testing.T, env *syncTestEnv) *syncDaemon {
	t.Helper()
	daemonClient := client.New(env.rdb, env.mountKey)
	cfg := syncDaemonConfig{
		Workspace:       env.workspace,
		LocalRoot:       env.localRoot,
		FS:              daemonClient,
		Store:           env.store,
		MaxFileBytes:    16 * 1024 * 1024,
		WatcherDebounce: 20 * time.Millisecond,
	}
	d, err := newSyncDaemon(cfg)
	if err != nil {
		t.Fatalf("newSyncDaemon: %v", err)
	}
	return d
}

func writeTreeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func mkdirTreeDir(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
}

func removeTreePath(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("remove %s: %v", rel, err)
	}
}

func removeRemotePath(t *testing.T, env *syncTestEnv, rel string) {
	t.Helper()
	if err := env.fsClient.Rm(context.Background(), absoluteRemotePath(rel)); err != nil && !isClientNotFound(err) {
		t.Fatalf("remote rm %s: %v", rel, err)
	}
	env.fsClient.InvalidateCache()
}

func requireMountOp(t *testing.T, plan mountReconcilePlan, code, path string) {
	t.Helper()
	for _, op := range plan.Operations {
		if op.Code == code && op.Path == path {
			return
		}
	}
	t.Fatalf("missing mount operation %s %s; operations = %#v", code, path, plan.Operations)
}

func localRemoteTreesEqual(t *testing.T, env *syncTestEnv) bool {
	t.Helper()
	local := localTreeSnapshot(t, env.localRoot)
	remote := remoteTreeSnapshot(t, env)
	if reflect.DeepEqual(local, remote) {
		return true
	}
	t.Logf("tree mismatch:\n%s", treeSnapshotDiff(local, remote))
	return false
}

func assertTreeHasFile(t *testing.T, env *syncTestEnv, rel, content string) {
	t.Helper()
	local := localTreeSnapshot(t, env.localRoot)
	remote := remoteTreeSnapshot(t, env)
	key := "file:" + filepath.ToSlash(rel)
	if got := local[key]; got != content {
		t.Fatalf("local %s = %q, want %q", rel, got, content)
	}
	if got := remote[key]; got != content {
		t.Fatalf("remote %s = %q, want %q", rel, got, content)
	}
}

func assertTreeHasDir(t *testing.T, env *syncTestEnv, rel string) {
	t.Helper()
	local := localTreeSnapshot(t, env.localRoot)
	remote := remoteTreeSnapshot(t, env)
	key := "dir:" + filepath.ToSlash(rel)
	if _, ok := local[key]; !ok {
		t.Fatalf("local directory %s missing", rel)
	}
	if _, ok := remote[key]; !ok {
		t.Fatalf("remote directory %s missing", rel)
	}
}

func assertTreeMissing(t *testing.T, env *syncTestEnv, rel string) {
	t.Helper()
	local := localTreeSnapshot(t, env.localRoot)
	remote := remoteTreeSnapshot(t, env)
	rel = filepath.ToSlash(rel)
	for _, prefix := range []string{"dir:", "file:", "symlink:"} {
		if _, ok := local[prefix+rel]; ok {
			t.Fatalf("local %s still present", rel)
		}
		if _, ok := remote[prefix+rel]; ok {
			t.Fatalf("remote %s still present", rel)
		}
	}
}

func localTreeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			out["symlink:"+rel] = target
			return nil
		}
		if d.IsDir() {
			out["dir:"+rel] = ""
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out["file:"+rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("local tree snapshot: %v", err)
	}
	return out
}

func remoteTreeSnapshot(t *testing.T, env *syncTestEnv) map[string]string {
	t.Helper()
	env.fsClient.InvalidateCache()
	out := make(map[string]string)
	var walk func(string)
	walk = func(dir string) {
		entries, err := env.fsClient.LsLong(context.Background(), dir)
		if err != nil {
			if isClientNotFound(err) {
				return
			}
			t.Fatalf("remote ls %s: %v", dir, err)
		}
		for _, entry := range entries {
			full := joinRemote(dir, entry.Name)
			rel := strings.TrimPrefix(full, "/")
			switch entry.Type {
			case "dir":
				out["dir:"+rel] = ""
				walk(full)
			case "file":
				data, err := env.fsClient.Cat(context.Background(), full)
				if err != nil {
					t.Fatalf("remote cat %s: %v", full, err)
				}
				out["file:"+rel] = string(data)
			case "symlink":
				target, err := env.fsClient.Readlink(context.Background(), full)
				if err != nil {
					t.Fatalf("remote readlink %s: %v", full, err)
				}
				out["symlink:"+rel] = target
			default:
				t.Fatalf("remote %s has unsupported type %q", full, entry.Type)
			}
		}
	}
	walk("/")
	return out
}

func treeSnapshotDiff(local, remote map[string]string) string {
	keys := make(map[string]struct{}, len(local)+len(remote))
	for key := range local {
		keys[key] = struct{}{}
	}
	for key := range remote {
		keys[key] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)

	var b strings.Builder
	for _, key := range ordered {
		l, lok := local[key]
		r, rok := remote[key]
		switch {
		case !lok:
			fmt.Fprintf(&b, "+ remote %s=%q\n", key, r)
		case !rok:
			fmt.Fprintf(&b, "- local %s=%q\n", key, l)
		case l != r:
			fmt.Fprintf(&b, "~ %s local=%q remote=%q\n", key, l, r)
		}
	}
	return b.String()
}
