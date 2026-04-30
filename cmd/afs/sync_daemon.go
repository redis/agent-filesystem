package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

// syncDaemonConfig is everything the daemon needs to operate. The fields are
// grouped by concern: the workspace and root path, the Redis client, and
// optional knobs (size cap, debounce, readonly).
type syncDaemonConfig struct {
	Workspace       string
	LocalRoot       string // absolute, will be created if missing
	FS              client.Client
	Store           *afsStore
	MaxFileBytes    int64
	WatcherDebounce time.Duration
	Readonly        bool
	Interactive     bool // when true, log every file event to stderr
	// Chunk-level delta sync knobs. Zero values use defaults.
	ChunkSize      int // bytes per chunk (default 256 KB)
	ChunkThreshold int // minimum file size to enable chunked sync (default 1 MB)
	// Changelog wiring (optional): when all of Rdb, StorageID, and SessionID
	// are set, the uploader emits one changelog entry per successful op so
	// live sync writes show up in the Changes tab without requiring an
	// explicit checkpoint.
	Rdb          *redis.Client
	StorageID    string
	SessionID    string
	User         string
	AgentID      string
	Label        string
	AgentVersion string
}

// syncDaemon orchestrates the watcher, reconciler, uploader, downloader, and
// remote subscription pump goroutines for one workspace+local-path pair.
type syncDaemon struct {
	cfg syncDaemonConfig
	log *syncLogger

	stateWriter *stateWriter
	reconciler  *reconciler
	uploader    *uploader
	downloader  *downloader
	pump        *remoteSubscriptionPump
	watcher     *syncWatcher
	full        *fullReconciler
	echo        *echoSuppressor
	conflict    *conflictNamer
	ignore      *syncIgnore

	wg     sync.WaitGroup
	cancel context.CancelFunc
	done   chan struct{}
}

// newSyncDaemon initializes (but does not start) a daemon for the given
// workspace + local path. Run() does the heavy lifting.
func newSyncDaemon(cfg syncDaemonConfig) (*syncDaemon, error) {
	if cfg.FS == nil {
		return nil, errors.New("syncDaemon: nil client")
	}
	if cfg.Workspace == "" {
		return nil, errors.New("syncDaemon: empty workspace")
	}
	if cfg.LocalRoot == "" {
		return nil, errors.New("syncDaemon: empty local root")
	}
	if cfg.MaxFileBytes <= 0 {
		cfg.MaxFileBytes = 64 * 1024 * 1024
	}
	if cfg.WatcherDebounce <= 0 {
		cfg.WatcherDebounce = 100 * time.Millisecond
	}

	if err := os.MkdirAll(cfg.LocalRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create local root: %w", err)
	}
	abs, err := filepath.Abs(cfg.LocalRoot)
	if err != nil {
		return nil, err
	}
	cfg.LocalRoot = abs

	st, err := loadSyncState(cfg.Workspace)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load sync state: %w", err)
		}
		st = newSyncState(cfg.Workspace, cfg.LocalRoot)
	} else if st.LocalPath != cfg.LocalRoot {
		// Local path moved between runs; reset state to force a full
		// reconciliation rather than using stale paths.
		st = newSyncState(cfg.Workspace, cfg.LocalRoot)
	}

	ignore, err := loadSyncIgnore(cfg.LocalRoot)
	if err != nil {
		return nil, err
	}
	echo := newEchoSuppressor()
	conflict := newConflictNamer()
	stateWriter := newStateWriter(st, time.Second)
	log := newSyncLogger(cfg.Interactive)

	d := &syncDaemon{
		cfg:         cfg,
		log:         log,
		stateWriter: stateWriter,
		echo:        echo,
		conflict:    conflict,
		ignore:      ignore,
		done:        make(chan struct{}),
	}
	d.reconciler = newReconciler(stateWriter, cfg.LocalRoot, cfg.Workspace, cfg.Store, cfg.FS, echo, conflict, ignore, cfg.MaxFileBytes, cfg.Readonly, log, cfg.ChunkSize, cfg.ChunkThreshold)
	d.full = newFullReconciler(d.reconciler)
	d.uploader = newUploader(cfg.FS, d.reconciler.uploadOut(), cfg.MaxFileBytes, cfg.Readonly, log)
	if cfg.Rdb != nil && strings.TrimSpace(cfg.StorageID) != "" && strings.TrimSpace(cfg.SessionID) != "" {
		d.uploader.attachChangelog(cfg.Rdb, cfg.StorageID, cfg.SessionID, cfg.User, cfg.AgentID, cfg.Label, cfg.AgentVersion)
	}
	d.downloader = newDownloader(cfg.FS, d.reconciler.downloadOut(), cfg.LocalRoot, conflict, echo, cfg.Readonly, log)
	d.pump = newRemoteSubscriptionPump(cfg.FS, log, stateWriter)
	return d, nil
}

// Start kicks off all goroutines and returns. The initial reconciliation
// (cold-start hydration) runs inline with a progress callback so the caller
// can show a live spinner. Once Start returns, the folder is fully populated
// and the steady-state goroutines are running.
func (d *syncDaemon) Start(ctx context.Context) error {
	return d.StartWithProgress(ctx, nil)
}

// StartWithProgress is like Start but accepts an optional progress callback
// that receives (done, total) file counts during the initial reconciliation.
func (d *syncDaemon) StartWithProgress(ctx context.Context, onProgress ProgressFunc) error {
	return d.start(ctx, onProgress, false)
}

// StartSteadyStateOnly skips the initial reconcile and goes straight to the
// steady-state goroutines. Used by the background _sync-daemon child process
// when the parent already did the reconcile moments ago.
func (d *syncDaemon) StartSteadyStateOnly(ctx context.Context) error {
	return d.start(ctx, nil, true)
}

func (d *syncDaemon) start(ctx context.Context, onProgress ProgressFunc, skipReconcile bool) error {
	if d.cancel != nil {
		return errors.New("syncDaemon: already started")
	}
	dctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	if !skipReconcile {
		if err := d.validateInitialSyncSafety(dctx); err != nil {
			cancel()
			return err
		}
		if err := d.full.run(dctx, onProgress); err != nil {
			cancel()
			return fmt.Errorf("initial reconcile: %w", err)
		}
	}

	// Install the watcher AFTER the reconcile. The cold-start path calls
	// materializeManifestToDirectory which does RemoveAll + MkdirAll on
	// the target directory — that would invalidate any fsnotify watches
	// installed earlier. Installing after guarantees the watches land on
	// the final directory tree.
	w, err := newSyncWatcher(d.cfg.LocalRoot, d.ignore, d.cfg.WatcherDebounce)
	if err != nil {
		cancel()
		return fmt.Errorf("watcher: %w", err)
	}
	d.watcher = w

	// Steady-state goroutines.
	stateStop := make(chan struct{})
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.stateWriter.run(stateStop)
	}()
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		<-dctx.Done()
		close(stateStop)
	}()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		w.run(dctx)
	}()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.uploader.run(dctx, d.reconciler.uploadIn())
	}()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.downloader.run(dctx, d.reconciler.downloadIn())
	}()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		err := d.pump.run(dctx, func() {
			d.reconciler.requestFullSweep()
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "afs sync: subscription pump exited: %v\n", err)
		}
	}()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.reconciler.run(dctx, w.Events(), d.pump.events())
	}()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		for {
			select {
			case <-dctx.Done():
				return
			case <-d.reconciler.fullSweepRequests():
				if err := d.full.run(dctx, nil); err != nil && !errors.Is(err, context.Canceled) {
					fmt.Fprintf(os.Stderr, "afs sync: full reconcile failed: %v\n", err)
				}
			case <-d.reconciler.rootReplaceRequests():
				d.reconciler.suppressLocalEventsDuringRestore(true)
				if err := d.full.replaceFromRemote(dctx, nil); err != nil {
					d.reconciler.suppressLocalEventsDuringRestore(false)
					if !errors.Is(err, context.Canceled) {
						fmt.Fprintf(os.Stderr, "afs sync: checkpoint restore local replace failed: %v\n", err)
					}
					continue
				}
				if d.watcher != nil {
					if err := d.watcher.resetRecursive(d.cfg.LocalRoot); err != nil {
						fmt.Fprintf(os.Stderr, "afs sync: refresh watcher after restore: %v\n", err)
					}
				}
				time.AfterFunc(2*d.cfg.WatcherDebounce, func() {
					d.reconciler.suppressLocalEventsDuringRestore(false)
				})
			}
		}
	}()

	go func() {
		d.wg.Wait()
		if d.watcher != nil {
			_ = d.watcher.Close()
		}
		d.stateWriter.flushNow()
		close(d.done)
	}()

	return nil
}

// Stop cancels the daemon context and waits for all goroutines to drain.
func (d *syncDaemon) Stop() {
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
	<-d.done
}

// Snapshot returns a copy of the current sync state for status reporting.
func (d *syncDaemon) Snapshot() *SyncState {
	return d.stateWriter.snapshot()
}

// Run is a blocking helper that starts the daemon and waits for ctx to be
// cancelled. Used by `afs up` foreground mode and tests.
func (d *syncDaemon) Run(ctx context.Context) error {
	if err := d.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	d.Stop()
	return ctx.Err()
}

// validateInitialSyncSafety blocks the "merge a populated local folder into an
// already-populated remote workspace" footgun on the very first sync, before we
// have any persisted state to disambiguate intent.
func (d *syncDaemon) validateInitialSyncSafety(ctx context.Context) error {
	if d == nil || d.stateWriter == nil {
		return nil
	}
	if snap := d.stateWriter.snapshot(); snap != nil && len(snap.Entries) > 0 {
		return nil
	}

	localHasEntries, err := d.hasSyncableLocalEntries()
	if err != nil {
		return fmt.Errorf("check local sync root: %w", err)
	}
	if !localHasEntries {
		return nil
	}

	remoteHasEntries, err := d.hasSyncableRemoteEntries(ctx)
	if err != nil {
		return fmt.Errorf("check remote workspace: %w", err)
	}
	if !remoteHasEntries {
		return nil
	}

	return fmt.Errorf(
		"refusing first sync for workspace %q: local path %q is already populated and the remote workspace is not empty\nUse an empty directory for `afs up`, or use `afs workspace clone %s <directory>` to download the existing workspace first",
		d.cfg.Workspace,
		d.cfg.LocalRoot,
		d.cfg.Workspace,
	)
}

func (d *syncDaemon) hasSyncableLocalEntries() (bool, error) {
	entries, err := os.ReadDir(d.cfg.LocalRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	for _, entry := range entries {
		if d.ignoreEntry(entry.Name(), entry) {
			continue
		}
		return true, nil
	}
	return false, nil
}

func (d *syncDaemon) hasSyncableRemoteEntries(ctx context.Context) (bool, error) {
	entries, err := d.cfg.FS.LsLong(ctx, "/")
	if err != nil {
		if isClientNotFound(err) {
			return false, nil
		}
		return false, err
	}
	for _, entry := range entries {
		if d.ignore == nil {
			return true, nil
		}
		if d.ignore.shouldIgnore(entry.Name, entry.Type == "dir") {
			continue
		}
		return true, nil
	}
	return false, nil
}

func (d *syncDaemon) ignoreEntry(name string, entry fs.DirEntry) bool {
	if d.ignore == nil {
		return false
	}
	return d.ignore.shouldIgnore(strings.TrimPrefix(filepath.ToSlash(name), "/"), entry.IsDir())
}
