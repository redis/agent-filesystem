package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

// syncTestEnv is the lightweight fixture every sync integration test uses.
// It bundles a miniredis-backed control plane store, a live workspace
// initialized for "repo", a local sync root, and a syncDaemon ready to
// Start(ctx). The test caller is responsible for invoking Start and Stop.
type syncTestEnv struct {
	t         *testing.T
	mr        *miniredis.Miniredis
	rdb       *redis.Client
	cp        *controlplane.Store
	store     *afsStore
	fsClient  client.Client
	workspace string
	localRoot string
	mountKey  string
	daemon    *syncDaemon
	cleanup   []func()
}

func newSyncTestEnv(t *testing.T) *syncTestEnv {
	t.Helper()
	withTempHome(t)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cp := controlplane.NewStore(rdb)
	ctx := context.Background()
	if err := cp.PutWorkspaceMeta(ctx, controlplane.WorkspaceMeta{Name: "repo", HeadSavepoint: "initial"}); err != nil {
		t.Fatalf("PutWorkspaceMeta: %v", err)
	}
	emptyManifest := controlplane.Manifest{
		Workspace: "repo",
		Savepoint: "initial",
		Entries: map[string]controlplane.ManifestEntry{
			"/": {Type: "dir", Mode: 0o755},
		},
	}
	if err := controlplane.SyncWorkspaceRoot(ctx, cp, "repo", emptyManifest); err != nil {
		t.Fatalf("SyncWorkspaceRoot: %v", err)
	}
	mountKey := controlplane.WorkspaceFSKey("repo")
	fs := client.New(rdb, mountKey)

	store := newAFSStore(rdb)
	localRoot := t.TempDir()

	env := &syncTestEnv{
		t:         t,
		mr:        mr,
		rdb:       rdb,
		cp:        cp,
		store:     store,
		fsClient:  fs,
		workspace: "repo",
		localRoot: localRoot,
		mountKey:  mountKey,
	}
	t.Cleanup(env.shutdown)
	return env
}

// startDaemon constructs and starts a fresh sync daemon. Tests call this
// after seeding initial files so the startup reconciliation has something to
// converge.
//
// The daemon gets its OWN client instance (not the helper's e.fsClient) so
// the native client's origin-dedup filter doesn't accidentally drop events
// the test publishes via e.fsClient.
func (e *syncTestEnv) startDaemon(t *testing.T, opts ...func(*syncDaemonConfig)) *syncDaemon {
	t.Helper()
	daemonClient := client.New(e.rdb, e.mountKey)
	cfg := syncDaemonConfig{
		Workspace:      e.workspace,
		LocalRoot:      e.localRoot,
		FS:             daemonClient,
		Store:          e.store,
		MaxFileBytes:   16 * 1024 * 1024,
		WatcherDebounce: 20 * time.Millisecond,
	}
	for _, o := range opts {
		o(&cfg)
	}
	d, err := newSyncDaemon(cfg)
	if err != nil {
		t.Fatalf("newSyncDaemon: %v", err)
	}
	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("daemon.Start: %v", err)
	}
	e.daemon = d
	return d
}

func (e *syncTestEnv) stopDaemon() {
	if e.daemon != nil {
		e.daemon.Stop()
		e.daemon = nil
	}
}

func (e *syncTestEnv) shutdown() {
	e.stopDaemon()
	for _, fn := range e.cleanup {
		fn()
	}
}

// writeLocalFile writes a file under the local root and returns its absolute
// path.
func (e *syncTestEnv) writeLocalFile(t *testing.T, rel, content string) string {
	t.Helper()
	abs := filepath.Join(e.localRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return abs
}

// writeRemoteFile writes a file directly into the live workspace via the
// client interface.
func (e *syncTestEnv) writeRemoteFile(t *testing.T, rel, content string) {
	t.Helper()
	ctx := context.Background()
	full := absoluteRemotePath(rel)
	if _, _, err := e.fsClient.CreateFile(ctx, full, 0o644, false); err != nil && !isClientAlreadyExists(err) {
		t.Fatalf("CreateFile %s: %v", rel, err)
	}
	if err := e.fsClient.Echo(ctx, full, []byte(content)); err != nil {
		t.Fatalf("Echo %s: %v", rel, err)
	}
}

// readRemoteFile is a convenience wrapper around client.Cat for assertions.
func (e *syncTestEnv) readRemoteFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := e.fsClient.Cat(context.Background(), absoluteRemotePath(rel))
	if err != nil {
		t.Fatalf("Cat %s: %v", rel, err)
	}
	return string(data)
}

// remoteExists returns true if Stat reports the path exists in the live root.
// The native client returns (nil, nil) for missing paths in some code paths,
// so we treat both error and nil-stat as "absent".
func (e *syncTestEnv) remoteExists(t *testing.T, rel string) bool {
	t.Helper()
	stat, err := e.fsClient.Stat(context.Background(), absoluteRemotePath(rel))
	if err != nil {
		if isClientNotFound(err) {
			return false
		}
		t.Fatalf("Stat %s: %v", rel, err)
	}
	return stat != nil
}

// localExists returns true if the file is present on disk.
func (e *syncTestEnv) localExists(rel string) bool {
	_, err := os.Stat(filepath.Join(e.localRoot, filepath.FromSlash(rel)))
	return err == nil
}

// readLocalFile reads a file from the local root.
func (e *syncTestEnv) readLocalFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(e.localRoot, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read local %s: %v", rel, err)
	}
	return string(data)
}

// assertEventually polls until cond returns true or the timeout fires. Used
// for assertions about asynchronous propagation through the sync pipeline.
func assertEventually(t *testing.T, timeout time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("eventually: %s", msg)
}

// removeFile is a tiny wrapper so tests don't import "os" just for one call.
func removeFile(abs string) error {
	return os.Remove(abs)
}

// testCtx returns a context.Background equivalent. Helper exists so tests
// don't need to import context everywhere.
func testCtx() context.Context {
	return context.Background()
}
