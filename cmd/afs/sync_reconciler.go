package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/agent-filesystem/mount/client"
)

// remoteEvent is the downloader-side counterpart of LocalEvent. It is
// produced by the subscription pump for each invalidation we receive from
// other clients touching the live workspace root.
type remoteEvent struct {
	Path         string // workspace-relative POSIX path; empty means "root, full reconciliation needed"
	NeedsContent bool   // true for InvalidateOpContent
	FullSweep    bool   // true for InvalidateOpPrefix "/"
}

// reconciler is the single writer for SyncState. Watcher events, downloader
// subscription events, upload results, and download results all funnel
// through one goroutine so we never have to lock individual entries.
type reconciler struct {
	state    *stateWriter
	root     string
	workspace string
	store    *afsStore
	echo     *echoSuppressor
	conflict *conflictNamer
	ignore   *syncIgnore

	uploadCh    chan uploadOp
	downloadCh  chan downloadOp
	uploadResCh chan uploadResult
	downloadResCh chan downloadResult
	localCh     <-chan LocalEvent
	remoteCh    <-chan remoteEvent

	fs           client.Client
	maxFileBytes int64
	readonly     bool
	log          *syncLogger

	// Chunk-level delta sync.
	chunkSize      int
	chunkThreshold int

	pendingFullSweep bool
	fullSweepRequest chan struct{}

	// pendingDeletes tracks paths whose local deletion has been queued for
	// upload but not yet confirmed. buildPlan skips re-downloading these files
	// so a full reconciliation cannot reverse a pending local delete.
	pendingDeletes map[string]struct{}
}

func newReconciler(
	state *stateWriter,
	root, workspace string,
	store *afsStore,
	fs client.Client,
	echo *echoSuppressor,
	conflict *conflictNamer,
	ignore *syncIgnore,
	maxFileBytes int64,
	readonly bool,
	log *syncLogger,
	chunkSize int,
	chunkThreshold int,
) *reconciler {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	if chunkThreshold <= 0 {
		chunkThreshold = defaultChunkThreshold
	}
	return &reconciler{
		state:            state,
		root:             root,
		workspace:        workspace,
		store:            store,
		fs:               fs,
		echo:             echo,
		conflict:         conflict,
		ignore:           ignore,
		maxFileBytes:     maxFileBytes,
		readonly:         readonly,
		log:              log,
		chunkSize:        chunkSize,
		chunkThreshold:   chunkThreshold,
		uploadCh:         make(chan uploadOp, 256),
		downloadCh:       make(chan downloadOp, 256),
		uploadResCh:      make(chan uploadResult, 256),
		downloadResCh:    make(chan downloadResult, 256),
		fullSweepRequest: make(chan struct{}, 1),
		pendingDeletes:   make(map[string]struct{}),
	}
}

// uploadIn is the channel the uploader drains.
func (r *reconciler) uploadIn() <-chan uploadOp { return r.uploadCh }

// downloadIn is the channel the downloader drains.
func (r *reconciler) downloadIn() <-chan downloadOp { return r.downloadCh }

// uploadOut is where the uploader posts results.
func (r *reconciler) uploadOut() chan<- uploadResult { return r.uploadResCh }

// downloadOut is where the downloader posts results.
func (r *reconciler) downloadOut() chan<- downloadResult { return r.downloadResCh }

// fullSweepRequests is signalled by the daemon when a full reconciliation
// pass is required (subscription reconnect, prefix-of-root invalidation,
// startup completion).
func (r *reconciler) fullSweepRequests() <-chan struct{} { return r.fullSweepRequest }

// requestFullSweep posts a full-sweep request unless one is already queued.
func (r *reconciler) requestFullSweep() {
	select {
	case r.fullSweepRequest <- struct{}{}:
	default:
	}
}

// run drains all event sources until ctx is cancelled. The reconciler is the
// single writer for r.state.
func (r *reconciler) run(ctx context.Context, local <-chan LocalEvent, remote <-chan remoteEvent) {
	r.localCh = local
	r.remoteCh = remote
	for {
		select {
		case <-ctx.Done():
			close(r.uploadCh)
			close(r.downloadCh)
			return
		case ev, ok := <-r.localCh:
			if !ok {
				r.localCh = nil
				continue
			}
			r.handleLocalEvent(ctx, ev)
		case ev, ok := <-r.remoteCh:
			if !ok {
				r.remoteCh = nil
				continue
			}
			r.handleRemoteEvent(ctx, ev)
		case res := <-r.uploadResCh:
			r.handleUploadResult(ctx, res)
		case res := <-r.downloadResCh:
			r.handleDownloadResult(ctx, res)
		}
	}
}

// handleLocalEvent processes a watcher event. The reconciler reads the local
// file from disk, computes the hash, looks up the stored entry, and decides
// whether to enqueue an upload, drop as echo, or trigger conflict resolution.
func (r *reconciler) handleLocalEvent(ctx context.Context, ev LocalEvent) {
	if ev.Path == "" {
		return
	}
	if r.ignore.shouldIgnore(ev.Path, false) {
		return
	}
	abs := filepath.Join(r.root, filepath.FromSlash(ev.Path))
	info, err := os.Lstat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		r.log.LocalChange(ev.Path, "deleted")
		r.handleLocalDelete(ev.Path)
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "afs sync: lstat %s: %v\n", abs, err)
		return
	}

	// Echo suppression: if the downloader marked an expectation for this
	// path and the disk content matches, drop the event silently.
	if exp, ok := r.echo.consume(ev.Path); ok {
		if r.echoMatches(abs, info, exp) {
			return
		}
		// Echo expectation existed but disk does not match — fall through
		// and treat as a normal local event. The expectation has already
		// been consumed.
	}

	// Log only events that survived ignore + echo filtering, and skip
	// chmod-only events (permission changes are synced but not interesting
	// to the user in the interactive log).
	if ev.KindHint != "chmod" {
		r.log.LocalChange(ev.Path, ev.KindHint)
	}

	switch {
	case info.Mode()&os.ModeSymlink != 0:
		r.handleLocalSymlink(ev.Path, abs)
	case info.IsDir():
		r.handleLocalDir(ev.Path, abs, info)
	case info.Mode().IsRegular():
		r.handleLocalFile(ev.Path, abs, info)
	default:
		// Sockets, fifos, devices: ignored.
	}
}

func (r *reconciler) echoMatches(abs string, info fs.FileInfo, exp echoExpectation) bool {
	switch exp.kind {
	case "file":
		if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
			return false
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return false
		}
		return sha256Hex(data) == exp.hash
	case "symlink":
		target, err := os.Readlink(abs)
		if err != nil {
			return false
		}
		return target == exp.hash
	case "dir":
		return info.IsDir()
	case "delete":
		return false // shouldn't happen — file present means no delete echo
	}
	return false
}

func (r *reconciler) handleLocalFile(rel, abs string, info fs.FileInfo) {
	fileSize := info.Size()
	if fileSize > r.maxFileBytes {
		fmt.Fprintf(os.Stderr, "afs sync: skipping %s — %d bytes exceeds %d byte cap\n", rel, fileSize, r.maxFileBytes)
		return
	}

	r.state.mu.Lock()
	stored, hasStored := r.state.state.Entries[rel]
	r.state.mu.Unlock()

	// Chunked path: files above the threshold use streaming chunk hashes
	// so we never load the full file into memory.
	if fileSize > int64(r.chunkThreshold) {
		hashes, actualSize, err := streamChunkHashes(abs, r.chunkSize)
		if err != nil {
			fmt.Fprintf(os.Stderr, "afs sync: chunk hash %s: %v\n", abs, err)
			return
		}
		hash := compositeHash(hashes)
		if hasStored && stored.LocalHash == hash && stored.Type == "file" && stored.Mode == uint32(info.Mode()&fs.ModePerm) {
			return
		}
		dirty, _ := diffChunkManifests(stored.ChunkHashes, hashes)
		r.uploadCh <- uploadOp{
			Kind:        opUploadFile,
			Path:        rel,
			AbsPath:     abs,
			Mode:        uint32(info.Mode() & fs.ModePerm),
			LocalHash:   hash,
			StoredEntry: stored,
			HasStored:   hasStored,
			Chunked:     true,
			FileSize:    actualSize,
			ChunkSize:   r.chunkSize,
			ChunkHashes: hashes,
			DirtyChunks: dirty,
		}
		return
	}

	// Inline path: small files use full-file read (existing behavior).
	data, err := os.ReadFile(abs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "afs sync: read %s: %v\n", abs, err)
		return
	}
	hash := sha256Hex(data)
	if hasStored && stored.LocalHash == hash && stored.Type == "file" && stored.Mode == uint32(info.Mode()&fs.ModePerm) {
		return
	}
	r.uploadCh <- uploadOp{
		Kind:        opUploadFile,
		Path:        rel,
		AbsPath:     abs,
		Content:     data,
		Mode:        uint32(info.Mode() & fs.ModePerm),
		LocalHash:   hash,
		StoredEntry: stored,
		HasStored:   hasStored,
	}
}

func (r *reconciler) handleLocalSymlink(rel, abs string) {
	target, err := os.Readlink(abs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "afs sync: readlink %s: %v\n", abs, err)
		return
	}
	r.state.mu.Lock()
	stored, hasStored := r.state.state.Entries[rel]
	r.state.mu.Unlock()
	if hasStored && stored.Type == "symlink" && stored.Target == target {
		return
	}
	r.uploadCh <- uploadOp{
		Kind:        opUploadSymlink,
		Path:        rel,
		AbsPath:     abs,
		Symlink:     target,
		StoredEntry: stored,
		HasStored:   hasStored,
	}
}

func (r *reconciler) handleLocalDir(rel, abs string, info fs.FileInfo) {
	r.state.mu.Lock()
	stored, hasStored := r.state.state.Entries[rel]
	r.state.mu.Unlock()
	if hasStored && stored.Type == "dir" {
		return
	}
	r.uploadCh <- uploadOp{
		Kind:        opUploadMkdir,
		Path:        rel,
		AbsPath:     abs,
		Mode:        uint32(info.Mode() & fs.ModePerm),
		StoredEntry: stored,
		HasStored:   hasStored,
	}
}

func (r *reconciler) handleLocalDelete(rel string) {
	r.state.mu.Lock()
	stored, hasStored := r.state.state.Entries[rel]
	r.state.mu.Unlock()
	if !hasStored {
		return
	}
	r.pendingDeletes[rel] = struct{}{}
	r.uploadCh <- uploadOp{
		Kind:        opUploadDelete,
		Path:        rel,
		AbsPath:     filepath.Join(r.root, filepath.FromSlash(rel)),
		StoredEntry: stored,
		HasStored:   true,
	}
}

func (r *reconciler) handleRemoteEvent(ctx context.Context, ev remoteEvent) {
	if ev.FullSweep {
		r.requestFullSweep()
		return
	}
	if ev.Path == "" {
		return
	}
	rel := strings.TrimPrefix(ev.Path, "/")
	if rel == "" {
		r.requestFullSweep()
		return
	}
	if r.ignore.shouldIgnore(rel, false) {
		return
	}
	abs := filepath.Join(r.root, filepath.FromSlash(rel))
	r.state.mu.Lock()
	stored, hasStored := r.state.state.Entries[rel]
	r.state.mu.Unlock()

	stat, err := r.fs.Stat(ctx, absoluteRemotePath(rel))
	if err != nil && !isClientNotFound(err) {
		fmt.Fprintf(os.Stderr, "afs sync: stat remote %s: %v\n", rel, err)
		return
	}
	if stat == nil {
		if hasStored {
			r.log.RemoteChange(rel, "deleted")
			r.downloadCh <- downloadOp{
				Kind:        opDownloadDelete,
				Path:        rel,
				AbsPath:     abs,
				StoredEntry: stored,
				HasStored:   true,
			}
		}
		return
	}

	// Detect potential conflict: local file diverged from stored hash AND
	// remote also moved off stored hash.
	conflict := r.detectConflict(rel, abs, stored, hasStored)
	r.log.RemoteChange(rel, stat.Type+" changed")

	switch stat.Type {
	case "dir":
		r.downloadCh <- downloadOp{
			Kind:        opDownloadMkdir,
			Path:        rel,
			AbsPath:     abs,
			Mode:        stat.Mode,
			StoredEntry: stored,
			HasStored:   hasStored,
		}
	case "symlink":
		target, err := r.fs.Readlink(ctx, absoluteRemotePath(rel))
		if err != nil {
			fmt.Fprintf(os.Stderr, "afs sync: readlink remote %s: %v\n", rel, err)
			return
		}
		r.downloadCh <- downloadOp{
			Kind:        opDownloadSymlink,
			Path:        rel,
			AbsPath:     abs,
			Symlink:     target,
			StoredEntry: stored,
			HasStored:   hasStored,
			Conflict:    conflict,
		}
	case "file":
		// Check if the remote file has chunk metadata for delta download.
		chunkSize, remoteHashes, chunkErr := r.fs.ChunkMeta(ctx, absoluteRemotePath(rel))
		if chunkErr == nil && chunkSize > 0 && len(remoteHashes) > 0 {
			dirty, _ := diffChunkManifests(stored.ChunkHashes, remoteHashes)
			r.downloadCh <- downloadOp{
				Kind:        opDownloadFile,
				Path:        rel,
				AbsPath:     abs,
				Mode:        stat.Mode,
				StoredEntry: stored,
				HasStored:   hasStored,
				Conflict:    conflict,
				Chunked:     true,
				FileSize:    stat.Size,
				ChunkSize:   chunkSize,
				ChunkHashes: remoteHashes,
				DirtyChunks: dirty,
			}
		} else {
			r.downloadCh <- downloadOp{
				Kind:        opDownloadFile,
				Path:        rel,
				AbsPath:     abs,
				Mode:        stat.Mode,
				StoredEntry: stored,
				HasStored:   hasStored,
				Conflict:    conflict,
			}
		}
	}
}

// detectConflict returns true when both sides moved off the stored hash. If
// the local file is missing or the stored entry is empty, no conflict is
// possible (it's just a download).
func (r *reconciler) detectConflict(rel, abs string, stored SyncEntry, hasStored bool) bool {
	if !hasStored {
		return false
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(abs)
		if err != nil {
			return false
		}
		return stored.Type == "symlink" && stored.Target != target
	}
	if !info.Mode().IsRegular() {
		return false
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return false
	}
	hash := sha256Hex(data)
	return stored.LocalHash != "" && hash != stored.LocalHash
}

func (r *reconciler) handleUploadResult(ctx context.Context, res uploadResult) {
	if res.Op.Kind == opUploadDelete {
		delete(r.pendingDeletes, res.Op.Path)
	}
	if res.Err != nil {
		r.log.Err("upload "+res.Op.Path, res.Err.Error())
		return
	}
	if res.Conflict {
		// Remote diverged. The remote-wins resolution is to download the
		// remote version, push the local copy aside, and create an
		// auto-checkpoint.
		r.downloadCh <- downloadOp{
			Kind:        opDownloadFile,
			Path:        res.Op.Path,
			AbsPath:     res.Op.AbsPath,
			StoredEntry: res.Op.StoredEntry,
			HasStored:   res.Op.HasStored,
			Conflict:    true,
		}
		go triggerConflictCheckpoint(ctx, r.store, r.workspace)
		return
	}
	now := time.Now().UTC()
	r.state.mu.Lock()
	switch res.Op.Kind {
	case opUploadFile:
		size := int64(len(res.Op.Content))
		if res.Op.Chunked {
			size = res.Op.FileSize
		}
		entry := SyncEntry{
			Type:          "file",
			Mode:          res.Op.Mode,
			Size:          size,
			LocalHash:     res.Op.LocalHash,
			RemoteHash:    res.Op.LocalHash,
			LastSyncedAt:  now,
			ChunkSize:     res.Op.ChunkSize,
			ChunkHashes:   res.Op.ChunkHashes,
		}
		if res.RemoteStat != nil {
			entry.RemoteMtimeMs = res.RemoteStat.Mtime
			entry.LocalMtimeMs = res.RemoteStat.Mtime
		}
		r.state.state.Entries[res.Op.Path] = entry
	case opUploadSymlink:
		entry := SyncEntry{
			Type:         "symlink",
			Target:       res.Op.Symlink,
			LastSyncedAt: now,
		}
		if res.RemoteStat != nil {
			entry.RemoteMtimeMs = res.RemoteStat.Mtime
			entry.LocalMtimeMs = res.RemoteStat.Mtime
		}
		r.state.state.Entries[res.Op.Path] = entry
	case opUploadMkdir:
		r.state.state.Entries[res.Op.Path] = SyncEntry{
			Type:         "dir",
			Mode:         res.Op.Mode,
			LastSyncedAt: now,
		}
	case opUploadDelete:
		delete(r.state.state.Entries, res.Op.Path)
	case opUploadChmod:
		if entry, ok := r.state.state.Entries[res.Op.Path]; ok {
			entry.Mode = res.Op.Mode
			entry.LastSyncedAt = now
			r.state.state.Entries[res.Op.Path] = entry
		}
	}
	r.state.dirty = true
	r.state.mu.Unlock()
	r.state.markDirty()

	switch res.Op.Kind {
	case opUploadFile:
		r.log.Upload(res.Op.Path)
	case opUploadSymlink:
		r.log.Symlink(res.Op.Path, res.Op.Symlink, "upload")
	case opUploadMkdir:
		r.log.Mkdir(res.Op.Path, "upload")
	case opUploadDelete:
		r.log.Delete(res.Op.Path, "upload")
	}
}

func (r *reconciler) handleDownloadResult(ctx context.Context, res downloadResult) {
	if res.Err != nil {
		r.log.Err("download "+res.Op.Path, res.Err.Error())
		return
	}
	// If a local delete is in-flight for this path, discard the download
	// result — the file was written to disk by the downloader but the user
	// already deleted it. Remove the re-created file immediately so it does
	// not reappear, and let the pending upload carry the delete to Redis.
	if _, pending := r.pendingDeletes[res.Op.Path]; pending {
		abs := filepath.Join(r.root, filepath.FromSlash(res.Op.Path))
		_ = os.Remove(abs)
		return
	}
	if res.ConflictPath != "" {
		r.log.Conflict(res.Op.Path, res.ConflictPath)
		go triggerConflictCheckpoint(ctx, r.store, r.workspace)
	}
	now := time.Now().UTC()
	r.state.mu.Lock()
	switch res.Op.Kind {
	case opDownloadFile:
		entry := SyncEntry{
			Type:          "file",
			Mode:          res.Mode,
			Size:          res.Size,
			LocalHash:     res.RemoteHash,
			RemoteHash:    res.RemoteHash,
			LocalMtimeMs:  res.MtimeMs,
			RemoteMtimeMs: res.MtimeMs,
			LastSyncedAt:  now,
			ChunkSize:     res.Op.ChunkSize,
			ChunkHashes:   res.Op.ChunkHashes,
		}
		r.state.state.Entries[res.Op.Path] = entry
	case opDownloadSymlink:
		r.state.state.Entries[res.Op.Path] = SyncEntry{
			Type:         "symlink",
			Target:       res.Target,
			LastSyncedAt: now,
		}
	case opDownloadMkdir:
		entry := SyncEntry{
			Type:         "dir",
			Mode:         res.Op.Mode,
			LastSyncedAt: now,
		}
		r.state.state.Entries[res.Op.Path] = entry
	case opDownloadDelete:
		delete(r.state.state.Entries, res.Op.Path)
	case opDownloadChmod:
		if entry, ok := r.state.state.Entries[res.Op.Path]; ok {
			entry.Mode = res.Mode
			entry.LastSyncedAt = now
			r.state.state.Entries[res.Op.Path] = entry
		}
	}
	r.state.dirty = true
	r.state.mu.Unlock()
	r.state.markDirty()

	switch res.Op.Kind {
	case opDownloadFile:
		r.log.Download(res.Op.Path)
	case opDownloadSymlink:
		r.log.Symlink(res.Op.Path, res.Target, "download")
	case opDownloadMkdir:
		r.log.Mkdir(res.Op.Path, "download")
	case opDownloadDelete:
		r.log.Delete(res.Op.Path, "download")
	}
}
