package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/agent-filesystem/mount/client"
)

// fullReconciler walks the local tree and the live workspace root, diffs
// observed state against the persisted SyncState, and applies changes
// directly (download files to disk, upload files to Redis) without going
// through the reconciler's channels. This avoids the deadlock that occurred
// when the old implementation dispatched ops into unbuffered channels before
// the consumer goroutines were running.
type fullReconciler struct {
	r *reconciler
}

func newFullReconciler(r *reconciler) *fullReconciler {
	return &fullReconciler{r: r}
}

// observedMeta is what the metadata-only scan collects per path. No file
// content or hashes — those are deferred to the execution phase where they're
// actually needed (and can be parallelized).
type observedMeta struct {
	kind    string // "file" | "dir" | "symlink"
	mode    uint32
	size    int64
	mtimeMs int64
	target  string // symlink target (local) or readlink result (remote)
}

// syncAction is one entry in the plan the reconciler builds during the diff
// phase, then executes in parallel during the apply phase.
type syncAction struct {
	kind       string // "download" | "upload" | "mkdir-local" | "mkdir-remote" | "delete-local" | "delete-remote" | "symlink-download" | "symlink-upload"
	path       string // workspace-relative POSIX, no leading slash
	absPath    string // absolute local path
	mode       uint32
	target     string // for symlinks
	conflict   bool
	localMeta  *observedMeta
	remoteMeta *observedMeta // carried from scan phase so exec can record mtime in state
}

const defaultParallelWorkers = 8

// ProgressFunc is called periodically during a full reconcile with
// (completed, total) counts. Used by the CLI to update the startup spinner.
type ProgressFunc func(done, total int64)

// run executes a single full reconciliation pass. On cold start (empty local
// folder, no persisted state) it uses the bulk materialize path — the same
// one `workspace run` uses — which reads the entire workspace in a handful of
// pipelined Redis calls instead of one LsLong per directory. Warm restarts
// use the metadata-diff approach to detect changes.
func (f *fullReconciler) run(ctx context.Context, onProgress ProgressFunc) error {
	if f.isColdStart() {
		return f.coldStart(ctx, onProgress)
	}
	return f.warmStart(ctx, onProgress)
}

// isColdStart returns true when the local folder is empty (or missing) and
// there is no persisted SyncState. This means we can skip the diff entirely
// and just pull everything from Redis in bulk.
func (f *fullReconciler) isColdStart() bool {
	f.r.state.mu.Lock()
	entryCount := len(f.r.state.state.Entries)
	f.r.state.mu.Unlock()
	if entryCount > 0 {
		return false
	}
	entries, err := os.ReadDir(f.r.root)
	if err != nil {
		return true // missing dir = cold start
	}
	// Ignore hidden files (.DS_Store etc) when deciding if the dir is "empty".
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			return false
		}
	}
	return true
}

// coldStart pulls the entire workspace from Redis using the bulk manifest
// path (buildManifestFromWorkspaceRoot + materializeManifestToDirectory).
// This is the same path `workspace run` uses and reads the full tree in
// a handful of pipelined HMGet/HGetAll calls — dramatically faster than
// one LsLong per directory over WAN.
func (f *fullReconciler) coldStart(ctx context.Context, onProgress ProgressFunc) error {
	if f.r.store == nil || f.r.store.rdb == nil {
		return fmt.Errorf("cold start requires a store with Redis connection")
	}

	fsKey := workspaceRedisKey(f.r.workspace)
	meta, err := f.r.store.getWorkspaceMeta(ctx, f.r.workspace)
	if err != nil {
		return fmt.Errorf("get workspace meta: %w", err)
	}

	m, blobs, stats, err := buildManifestFromWorkspaceRootWithProgress(ctx, f.r.store.rdb, fsKey, f.r.workspace, meta.HeadSavepoint, onProgress)
	if err != nil {
		return fmt.Errorf("build manifest: %w", err)
	}

	if onProgress != nil {
		onProgress(0, int64(stats.FileCount+stats.DirCount))
	}

	var done int64
	matStats, err := materializeManifestToDirectory(f.r.root, m, func(blobID string) ([]byte, error) {
		data, ok := blobs[blobID]
		if !ok {
			return nil, fmt.Errorf("blob %q missing during cold start materialize", blobID)
		}
		return data, nil
	}, manifestMaterializeOptions{
		preserveMetadata: true,
		onProgress: func(p importStats) {
			done = int64(p.Files + p.Dirs + p.Symlinks)
			if onProgress != nil {
				onProgress(done, int64(stats.FileCount+stats.DirCount))
			}
		},
	})
	if err != nil {
		return fmt.Errorf("materialize: %w", err)
	}

	// Build SyncState from the materialized manifest so warm restarts can
	// diff against it without re-reading content.
	now := time.Now().UTC()
	f.r.state.mu.Lock()
	for path, entry := range m.Entries {
		rel := strings.TrimPrefix(path, "/")
		if rel == "" {
			continue // skip root dir entry
		}
		se := SyncEntry{
			Type:          entry.Type,
			Mode:          entry.Mode,
			Size:          entry.Size,
			RemoteMtimeMs: entry.MtimeMs,
			LastSyncedAt:  now,
		}
		switch entry.Type {
		case "file":
			// Compute hash from the inline/blob content so warm restart can compare.
			data, _ := manifestEntryData(entry, func(blobID string) ([]byte, error) {
				d, ok := blobs[blobID]
				if !ok {
					return nil, fmt.Errorf("blob %q missing", blobID)
				}
				return d, nil
			})
			if data != nil {
				se.LocalHash = sha256Hex(data)
				se.RemoteHash = se.LocalHash
			}
			// Get local mtime from the file we just wrote.
			abs := filepath.Join(f.r.root, filepath.FromSlash(rel))
			if fi, statErr := os.Stat(abs); statErr == nil {
				se.LocalMtimeMs = fi.ModTime().UnixMilli()
			}
		case "symlink":
			se.Target = entry.Target
		}
		f.r.state.state.Entries[rel] = se
	}
	f.r.state.dirty = true
	f.r.state.mu.Unlock()
	f.r.state.markDirty()

	_ = matStats // used via the progress callback
	return nil
}

// warmStart diffs local vs remote metadata and syncs only what changed.
func (f *fullReconciler) warmStart(ctx context.Context, onProgress ProgressFunc) error {
	local, err := f.scanLocalMeta()
	if err != nil {
		return fmt.Errorf("scan local: %w", err)
	}
	remote, err := f.scanRemoteMeta(ctx, onProgress)
	if err != nil {
		return fmt.Errorf("scan remote: %w", err)
	}

	plan := f.buildPlan(local, remote)
	if len(plan) == 0 {
		return nil
	}
	return f.executePlan(ctx, plan, onProgress)
}

// scanLocalMeta walks the local tree collecting only stat information —
// no ReadFile, no hashing. This is O(syscalls) not O(bytes).
func (f *fullReconciler) scanLocalMeta() (map[string]observedMeta, error) {
	out := make(map[string]observedMeta)
	root := f.r.root
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if p == root {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if f.r.ignore.shouldIgnoreEntry(p, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return nil
			}
			out[rel] = observedMeta{kind: "symlink", target: target, mtimeMs: info.ModTime().UnixMilli()}
			return nil
		}
		if d.IsDir() {
			out[rel] = observedMeta{kind: "dir", mode: uint32(info.Mode() & fs.ModePerm), mtimeMs: info.ModTime().UnixMilli()}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		out[rel] = observedMeta{
			kind:    "file",
			mode:    uint32(info.Mode() & fs.ModePerm),
			size:    info.Size(),
			mtimeMs: info.ModTime().UnixMilli(),
		}
		return nil
	})
	return out, err
}

// scanRemoteMeta walks the live workspace root using LsLong only — no Cat().
// This is one LsLong RPC per directory, proportional to directory count not
// file count. For symlinks we also call Readlink (one extra RPC per symlink).
// No timeout — large workspaces (45 GB+) can have thousands of directories
// and the walk legitimately takes minutes on WAN. The parent context handles
// cancellation (Ctrl-C).
func (f *fullReconciler) scanRemoteMeta(ctx context.Context, onProgress ProgressFunc) (map[string]observedMeta, error) {
	out := make(map[string]observedMeta)
	var scanned int64
	report := func() {
		scanned++
		if onProgress != nil {
			onProgress(scanned, -1) // -1 = total unknown during scan
		}
	}
	if err := f.scanRemoteDirMeta(ctx, "/", out, report); err != nil {
		if isClientNotFound(err) {
			return out, nil
		}
		return nil, err
	}
	return out, nil
}

func (f *fullReconciler) scanRemoteDirMeta(ctx context.Context, dir string, out map[string]observedMeta, onEntry func()) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	entries, err := f.r.fs.LsLong(ctx, dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		full := joinRemote(dir, e.Name)
		rel := strings.TrimPrefix(full, "/")
		if f.r.ignore.shouldIgnore(rel, e.Type == "dir") {
			continue
		}
		if onEntry != nil {
			onEntry()
		}
		switch e.Type {
		case "dir":
			out[rel] = observedMeta{kind: "dir", mode: e.Mode, mtimeMs: e.Mtime}
			if err := f.scanRemoteDirMeta(ctx, full, out, onEntry); err != nil {
				return err
			}
		case "symlink":
			target, err := f.r.fs.Readlink(ctx, full)
			if err != nil {
				continue
			}
			out[rel] = observedMeta{kind: "symlink", target: target, mtimeMs: e.Mtime}
		case "file":
			out[rel] = observedMeta{kind: "file", mode: e.Mode, size: e.Size, mtimeMs: e.Mtime}
		}
	}
	return nil
}

// buildPlan diffs local vs remote vs persisted state and produces a list of
// actions. No I/O happens here — just decisions based on metadata.
func (f *fullReconciler) buildPlan(local, remote map[string]observedMeta) []syncAction {
	all := make(map[string]struct{}, len(local)+len(remote))
	for k := range local {
		all[k] = struct{}{}
	}
	for k := range remote {
		all[k] = struct{}{}
	}

	// Sort directories before files so mkdir actions run first in the
	// parallel phase (the worker pool creates parents before writing children).
	var plan []syncAction
	for path := range all {
		l, lok := local[path]
		r, rok := remote[path]

		f.r.state.mu.Lock()
		stored, hasStored := f.r.state.state.Entries[path]
		f.r.state.mu.Unlock()

		abs := filepath.Join(f.r.root, filepath.FromSlash(path))

		switch {
		case lok && !rok:
			// Local-only → upload to remote.
			plan = append(plan, f.planUpload(path, abs, l, stored, hasStored)...)
		case !lok && rok:
			// Remote-only → download to local.
			plan = append(plan, f.planDownload(path, abs, r, stored, hasStored, false))
		case lok && rok:
			// Both present. Check if they match using metadata (size+mtime
			// for files, target for symlinks). Only go deeper if they differ.
			if metaMatch(l, r, stored, hasStored) {
				f.refreshStateMeta(path, l, r)
				continue
			}
			// Possible conflict: both sides diverged from stored state?
			conflict := false
			if hasStored && stored.LocalHash != "" {
				// We can't determine conflict purely from metadata — the
				// actual content hashes are needed. Mark as conflict candidate
				// only if local metadata also changed from stored.
				localChanged := l.size != stored.Size || l.mtimeMs != stored.LocalMtimeMs
				remoteChanged := r.size != stored.Size || r.mtimeMs != stored.RemoteMtimeMs
				if localChanged && remoteChanged {
					conflict = true
				}
			}
			plan = append(plan, f.planDownload(path, abs, r, stored, hasStored, conflict))
		}
	}
	return plan
}

func (f *fullReconciler) planUpload(path, abs string, l observedMeta, stored SyncEntry, hasStored bool) []syncAction {
	switch l.kind {
	case "dir":
		return []syncAction{{kind: "mkdir-remote", path: path, absPath: abs, mode: l.mode}}
	case "symlink":
		return []syncAction{{kind: "symlink-upload", path: path, absPath: abs, target: l.target}}
	case "file":
		return []syncAction{{kind: "upload", path: path, absPath: abs, mode: l.mode, localMeta: &l}}
	}
	return nil
}

func (f *fullReconciler) planDownload(path, abs string, r observedMeta, stored SyncEntry, hasStored bool, conflict bool) syncAction {
	rm := r // copy so we can take address
	switch r.kind {
	case "dir":
		return syncAction{kind: "mkdir-local", path: path, absPath: abs, mode: r.mode, remoteMeta: &rm}
	case "symlink":
		return syncAction{kind: "symlink-download", path: path, absPath: abs, target: r.target, conflict: conflict, remoteMeta: &rm}
	default: // file
		return syncAction{kind: "download", path: path, absPath: abs, mode: r.mode, conflict: conflict, remoteMeta: &rm}
	}
}

// metaMatch decides whether local and remote are equivalent using only
// metadata (no content read). For files we compare size and check if both
// sides match the stored state. For cold start (no stored state) where both
// sides have matching size+mtime, we assume they're in sync.
func metaMatch(l, r observedMeta, stored SyncEntry, hasStored bool) bool {
	if l.kind != r.kind {
		return false
	}
	switch l.kind {
	case "dir":
		return true
	case "symlink":
		return l.target == r.target
	case "file":
		if l.size != r.size {
			return false
		}
		// If we have stored state and both sides match it, they're in sync.
		if hasStored && stored.Size == l.size {
			if l.mtimeMs == stored.LocalMtimeMs && r.mtimeMs == stored.RemoteMtimeMs {
				return true
			}
		}
		// Cold start or no stored mtime: same size + same remote mtime as
		// stored = probably unchanged. We accept a false-positive here
		// (skipping a file that changed to the exact same size) because the
		// alternative is reading every file on every startup.
		if l.size == r.size && l.mtimeMs == r.mtimeMs {
			return true
		}
		return false
	}
	return false
}

// executePlan runs the planned actions with a bounded worker pool.
func (f *fullReconciler) executePlan(ctx context.Context, plan []syncAction, onProgress ProgressFunc) error {
	// Separate directory creations from file ops. Dirs must happen first
	// (serially) so parent directories exist before child file writes.
	var dirActions, fileActions []syncAction
	for _, a := range plan {
		if a.kind == "mkdir-local" || a.kind == "mkdir-remote" {
			dirActions = append(dirActions, a)
		} else {
			fileActions = append(fileActions, a)
		}
	}

	total := int64(len(plan))
	var done atomic.Int64

	report := func() {
		if onProgress != nil {
			onProgress(done.Load(), total)
		}
	}

	// Phase 1: directories (serial, fast).
	for _, a := range dirActions {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		f.executeAction(ctx, a)
		done.Add(1)
		report()
	}

	// Phase 2: files + symlinks (parallel).
	sem := make(chan struct{}, defaultParallelWorkers)
	var mu sync.Mutex
	var firstErr error

	var wg sync.WaitGroup
	for _, a := range fileActions {
		if ctx.Err() != nil {
			break
		}
		mu.Lock()
		if firstErr != nil {
			mu.Unlock()
			break
		}
		mu.Unlock()

		wg.Add(1)
		sem <- struct{}{}
		go func(action syncAction) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := f.executeAction(ctx, action); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
			done.Add(1)
			report()
		}(a)
	}
	wg.Wait()
	f.r.state.markDirty()
	return firstErr
}

// executeAction applies one planned action directly (no channel dispatch).
func (f *fullReconciler) executeAction(ctx context.Context, a syncAction) error {
	switch a.kind {
	case "mkdir-local":
		return f.execMkdirLocal(a)
	case "mkdir-remote":
		return f.execMkdirRemote(ctx, a)
	case "download":
		return f.execDownload(ctx, a)
	case "upload":
		return f.execUpload(ctx, a)
	case "symlink-download":
		return f.execSymlinkDownload(ctx, a)
	case "symlink-upload":
		return f.execSymlinkUpload(ctx, a)
	default:
		return fmt.Errorf("unknown action kind: %s", a.kind)
	}
}

func (f *fullReconciler) execMkdirLocal(a syncAction) error {
	if err := os.MkdirAll(a.absPath, 0o755); err != nil {
		return err
	}
	f.r.echo.markDir(a.path)
	f.updateState(a.path, SyncEntry{
		Type:         "dir",
		Mode:         a.mode,
		LastSyncedAt: time.Now().UTC(),
	})
	return nil
}

func (f *fullReconciler) execMkdirRemote(ctx context.Context, a syncAction) error {
	remotePath := absoluteRemotePath(a.path)
	if err := f.r.fs.Mkdir(ctx, remotePath); err != nil && !isClientAlreadyExists(err) {
		return fmt.Errorf("mkdir remote %s: %w", a.path, err)
	}
	f.updateState(a.path, SyncEntry{
		Type:         "dir",
		Mode:         a.mode,
		LastSyncedAt: time.Now().UTC(),
	})
	return nil
}

func (f *fullReconciler) execDownload(ctx context.Context, a syncAction) error {
	remotePath := absoluteRemotePath(a.path)
	// Use a per-file timeout to prevent a single slow Redis call from
	// blocking the entire cold start. 30s is generous for any individual
	// file (even multi-MB on WAN).
	catCtx, catCancel := context.WithTimeout(ctx, 30*time.Second)
	defer catCancel()
	data, err := f.r.fs.Cat(catCtx, remotePath)
	if err != nil {
		if isClientNotFound(err) {
			return nil // vanished between scan and download
		}
		return fmt.Errorf("download %s: %w", a.path, err)
	}
	hash := sha256Hex(data)

	if a.conflict {
		if _, err := moveLocalToConflict(f.r.conflict, a.absPath); err != nil {
			fmt.Fprintf(os.Stderr, "afs sync: conflict copy %s: %v\n", a.path, err)
		}
	}

	mode := a.mode
	if mode == 0 {
		mode = 0o644
	}
	if f.r.readonly {
		mode = 0o444
	}
	if err := atomicWriteFileStandalone(a.absPath, data, mode, os.Getpid()); err != nil {
		return fmt.Errorf("write %s: %w", a.path, err)
	}
	f.r.echo.markFile(a.path, hash)

	// Record both mtimes so the next startup's metaMatch can skip unchanged
	// files without re-reading content. Local mtime comes from the file we
	// just wrote; remote mtime comes from the scan phase.
	var localMtimeMs, remoteMtimeMs int64
	if fi, err := os.Stat(a.absPath); err == nil {
		localMtimeMs = fi.ModTime().UnixMilli()
	}
	if a.remoteMeta != nil {
		remoteMtimeMs = a.remoteMeta.mtimeMs
	}
	f.updateState(a.path, SyncEntry{
		Type:          "file",
		Mode:          mode,
		Size:          int64(len(data)),
		LocalHash:     hash,
		RemoteHash:    hash,
		LocalMtimeMs:  localMtimeMs,
		RemoteMtimeMs: remoteMtimeMs,
		LastSyncedAt:  time.Now().UTC(),
	})
	return nil
}

func (f *fullReconciler) execUpload(ctx context.Context, a syncAction) error {
	data, err := os.ReadFile(a.absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if int64(len(data)) > f.r.maxFileBytes {
		fmt.Fprintf(os.Stderr, "afs sync: skipping %s — %d bytes exceeds %d byte cap\n", a.path, len(data), f.r.maxFileBytes)
		return nil
	}
	hash := sha256Hex(data)
	remotePath := absoluteRemotePath(a.path)
	if err := f.r.fs.Echo(ctx, remotePath, data); err != nil {
		return fmt.Errorf("upload %s: %w", a.path, err)
	}
	mode := a.mode
	if mode == 0 {
		mode = 0o644
	}
	_ = f.r.fs.Chmod(ctx, remotePath, mode)

	var localMtimeMs int64
	if fi, err := os.Stat(a.absPath); err == nil {
		localMtimeMs = fi.ModTime().UnixMilli()
	}
	f.updateState(a.path, SyncEntry{
		Type:          "file",
		Mode:          mode,
		Size:          int64(len(data)),
		LocalHash:     hash,
		RemoteHash:    hash,
		LocalMtimeMs:  localMtimeMs,
		RemoteMtimeMs: localMtimeMs, // best estimate without a Stat RPC; close enough for skip logic
		LastSyncedAt:  time.Now().UTC(),
	})
	return nil
}

func (f *fullReconciler) execSymlinkDownload(ctx context.Context, a syncAction) error {
	target := a.target
	if target == "" {
		remotePath := absoluteRemotePath(a.path)
		t, err := f.r.fs.Readlink(ctx, remotePath)
		if err != nil {
			return err
		}
		target = t
	}
	if err := os.MkdirAll(filepath.Dir(a.absPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Lstat(a.absPath); err == nil {
		_ = os.Remove(a.absPath)
	}
	if err := os.Symlink(target, a.absPath); err != nil {
		return err
	}
	f.r.echo.markSymlink(a.path, target)
	f.updateState(a.path, SyncEntry{
		Type:         "symlink",
		Target:       target,
		LastSyncedAt: time.Now().UTC(),
	})
	return nil
}

func (f *fullReconciler) execSymlinkUpload(ctx context.Context, a syncAction) error {
	remotePath := absoluteRemotePath(a.path)
	if existing, err := f.r.fs.Stat(ctx, remotePath); err == nil && existing != nil {
		_ = f.r.fs.Rm(ctx, remotePath)
	}
	if err := f.r.fs.Ln(ctx, a.target, remotePath); err != nil {
		return fmt.Errorf("symlink upload %s: %w", a.path, err)
	}
	f.updateState(a.path, SyncEntry{
		Type:         "symlink",
		Target:       a.target,
		LastSyncedAt: time.Now().UTC(),
	})
	return nil
}

func (f *fullReconciler) updateState(path string, entry SyncEntry) {
	f.r.state.mu.Lock()
	f.r.state.state.Entries[path] = entry
	f.r.state.dirty = true
	f.r.state.mu.Unlock()
}

func (f *fullReconciler) refreshStateMeta(rel string, l, r observedMeta) {
	now := time.Now().UTC()
	f.r.state.mu.Lock()
	defer f.r.state.mu.Unlock()
	f.r.state.state.Entries[rel] = SyncEntry{
		Type:          l.kind,
		Mode:          l.mode,
		Size:          l.size,
		LocalMtimeMs:  l.mtimeMs,
		RemoteMtimeMs: r.mtimeMs,
		Target:        targetFromMeta(l, r),
		LastSyncedAt:  now,
	}
	f.r.state.dirty = true
}

func targetFromMeta(l, r observedMeta) string {
	if l.kind == "symlink" {
		return l.target
	}
	if r.kind == "symlink" {
		return r.target
	}
	return ""
}

func joinRemote(dir, name string) string {
	if dir == "" || dir == "/" {
		return "/" + name
	}
	if strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}

// atomicWriteFileStandalone is the free-function counterpart of
// downloader.atomicWriteFile. Used by the full reconciler (which doesn't
// have a downloader instance during startup).
func atomicWriteFileStandalone(absPath string, data []byte, mode uint32, pid int) error {
	if mode == 0 {
		mode = 0o644
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return err
	}
	suffix := hex.EncodeToString(buf[:])
	base := filepath.Base(absPath)
	dir := filepath.Dir(absPath)
	tmpName := filepath.Join(dir, "."+base+".afssync.tmp."+fmt.Sprintf("%d.%s", pid, suffix))
	f, err := os.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_TRUNC, fs.FileMode(mode&0o7777))
	if err != nil {
		return err
	}
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, absPath); err != nil {
		cleanup()
		return err
	}
	_ = os.Chmod(absPath, fs.FileMode(mode&0o7777))
	return nil
}

// remoteSubscriptionPump runs in its own goroutine, translating client
// invalidation events into remoteEvents the reconciler understands.
type remoteSubscriptionPump struct {
	fs  client.Client
	out chan remoteEvent
}

func newRemoteSubscriptionPump(fs client.Client) *remoteSubscriptionPump {
	return &remoteSubscriptionPump{fs: fs, out: make(chan remoteEvent, 256)}
}

func (p *remoteSubscriptionPump) events() <-chan remoteEvent { return p.out }

func (p *remoteSubscriptionPump) run(ctx context.Context, onReconnect func()) error {
	handler := func(ev client.InvalidateEvent) {
		switch ev.Op {
		case client.InvalidateOpContent:
			for _, path := range ev.Paths {
				p.send(remoteEvent{Path: path, NeedsContent: true})
			}
		case client.InvalidateOpInode:
			for _, path := range ev.Paths {
				p.send(remoteEvent{Path: path})
			}
		case client.InvalidateOpDir:
			for _, path := range ev.Paths {
				p.send(remoteEvent{Path: path})
			}
		case client.InvalidateOpPrefix:
			for _, path := range ev.Paths {
				if path == "/" || path == "" {
					p.send(remoteEvent{FullSweep: true})
					if onReconnect != nil {
						onReconnect()
					}
					return
				}
				p.send(remoteEvent{Path: path})
			}
		}
	}
	return p.fs.SubscribeInvalidations(ctx, handler)
}

func (p *remoteSubscriptionPump) send(ev remoteEvent) {
	select {
	case p.out <- ev:
	default:
		select {
		case p.out <- remoteEvent{FullSweep: true}:
		default:
		}
	}
}
