package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/redis/agent-filesystem/mount/client"
)

// downloadOpKind enumerates the operations the downloader can apply to the
// local filesystem.
type downloadOpKind int

const (
	opDownloadFile downloadOpKind = iota + 1
	opDownloadSymlink
	opDownloadMkdir
	opDownloadDelete
	opDownloadChmod
)

// downloadOp is the work item the reconciler hands to the downloader.
type downloadOp struct {
	Kind        downloadOpKind
	Path        string // workspace-relative POSIX, no leading slash
	AbsPath     string // absolute local destination
	Mode        uint32
	Symlink     string // target, only for opDownloadSymlink
	StoredEntry SyncEntry
	HasStored   bool
	Conflict    bool // when true, downloader writes the conflict-copy first then writes remote
	// Chunked download fields (set when remote file is chunked).
	Chunked     bool
	FileSize    int64
	ChunkSize   int
	ChunkHashes []string // remote's complete manifest
	DirtyChunks []int    // indices to fetch
}

// downloadResult informs the reconciler of the outcome so it can update state.
type downloadResult struct {
	Op           downloadOp
	Err          error
	RemoteHash   string
	RemoteStat   *client.StatResult
	ConflictPath string // populated when Conflict is true and the local file was preserved
	Mode         uint32
	Size         int64
	MtimeMs      int64
	Target       string // for symlinks
}

// downloader runs in its own goroutine, draining ops from the reconciler.
type downloader struct {
	fs       client.Client
	results  chan<- downloadResult
	root     string // local workspace root
	pid      int
	conflict *conflictNamer
	echo     *echoSuppressor
	readonly bool
	log      *syncLogger
}

func newDownloader(fs client.Client, results chan<- downloadResult, root string, conflict *conflictNamer, echo *echoSuppressor, readonly bool, log *syncLogger) *downloader {
	return &downloader{
		fs:       fs,
		results:  results,
		root:     root,
		pid:      os.Getpid(),
		conflict: conflict,
		echo:     echo,
		readonly: readonly,
		log:      log,
	}
}

// run drains in until ctx is cancelled.
func (d *downloader) run(ctx context.Context, in <-chan downloadOp) {
	for {
		select {
		case <-ctx.Done():
			return
		case op, ok := <-in:
			if !ok {
				return
			}
			d.process(ctx, op)
		}
	}
}

func (d *downloader) process(ctx context.Context, op downloadOp) {
	switch op.Kind {
	case opDownloadFile:
		d.processFile(ctx, op)
	case opDownloadSymlink:
		d.processSymlink(ctx, op)
	case opDownloadMkdir:
		d.processMkdir(ctx, op)
	case opDownloadDelete:
		d.processDelete(ctx, op)
	case opDownloadChmod:
		d.processChmod(ctx, op)
	default:
		d.send(downloadResult{Op: op, Err: fmt.Errorf("unknown download op kind: %d", op.Kind)})
	}
}

func (d *downloader) processFile(ctx context.Context, op downloadOp) {
	if op.Chunked {
		d.processChunkedFile(ctx, op)
		return
	}
	remotePath := absoluteRemotePath(op.Path)
	stat, err := d.fs.Stat(ctx, remotePath)
	if err != nil && !isClientNotFound(err) {
		d.send(downloadResult{Op: op, Err: fmt.Errorf("stat remote %s: %w", op.Path, err)})
		return
	}
	if stat == nil {
		// Treat as a delete: the inode vanished between the invalidation
		// dispatch and our follow-up read.
		d.removeLocalFile(op)
		d.send(downloadResult{Op: downloadOp{Kind: opDownloadDelete, Path: op.Path, AbsPath: op.AbsPath, StoredEntry: op.StoredEntry, HasStored: op.HasStored}})
		return
	}
	if stat.Type == "dir" {
		d.processMkdir(ctx, op)
		return
	}
	if stat.Type == "symlink" {
		target, err := d.fs.Readlink(ctx, remotePath)
		if err != nil {
			d.send(downloadResult{Op: op, Err: err})
			return
		}
		op.Symlink = target
		d.processSymlink(ctx, op)
		return
	}

	data, err := d.fs.Cat(ctx, remotePath)
	if err != nil {
		d.send(downloadResult{Op: op, Err: fmt.Errorf("read remote %s: %w", op.Path, err)})
		return
	}
	hash := sha256Hex(data)

	var conflictPath string
	if op.Conflict {
		moved, err := moveLocalToConflict(d.conflict, op.AbsPath)
		if err != nil {
			d.send(downloadResult{Op: op, Err: fmt.Errorf("preserve conflict copy %s: %w", op.Path, err)})
			return
		}
		conflictPath = moved
	}

	if d.readonly {
		// Mark read-only files in readonly mode (0444).
		stat.Mode = 0o444
	}

	if err := d.atomicWriteFile(op.AbsPath, data, stat.Mode); err != nil {
		d.send(downloadResult{Op: op, Err: fmt.Errorf("write local %s: %w", op.Path, err)})
		return
	}
	d.echo.markFile(op.Path, hash)

	d.send(downloadResult{
		Op:           op,
		RemoteHash:   hash,
		RemoteStat:   stat,
		ConflictPath: conflictPath,
		Mode:         stat.Mode,
		Size:         stat.Size,
		MtimeMs:      stat.Mtime,
	})
}

func (d *downloader) processChunkedFile(ctx context.Context, op downloadOp) {
	remotePath := absoluteRemotePath(op.Path)
	stat, err := d.fs.Stat(ctx, remotePath)
	if err != nil && !isClientNotFound(err) {
		d.send(downloadResult{Op: op, Err: fmt.Errorf("stat remote %s: %w", op.Path, err)})
		return
	}
	if stat == nil {
		d.removeLocalFile(op)
		d.send(downloadResult{Op: downloadOp{Kind: opDownloadDelete, Path: op.Path, AbsPath: op.AbsPath, StoredEntry: op.StoredEntry, HasStored: op.HasStored}})
		return
	}

	// Fetch dirty chunks from remote.
	chunkData, err := d.fs.ReadChunks(ctx, remotePath, op.DirtyChunks, op.ChunkSize)
	if err != nil {
		d.send(downloadResult{Op: op, Err: fmt.Errorf("read chunks %s: %w", op.Path, err)})
		return
	}

	var conflictPath string
	if op.Conflict {
		moved, err := moveLocalToConflict(d.conflict, op.AbsPath)
		if err != nil {
			d.send(downloadResult{Op: op, Err: fmt.Errorf("preserve conflict copy %s: %w", op.Path, err)})
			return
		}
		conflictPath = moved
	}

	mode := stat.Mode
	if d.readonly {
		mode = 0o444
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(op.AbsPath), 0o755); err != nil {
		d.send(downloadResult{Op: op, Err: err})
		return
	}

	// Patch the local file at chunk offsets.
	f, err := os.OpenFile(op.AbsPath, os.O_RDWR|os.O_CREATE, fs.FileMode(mode&0o7777))
	if err != nil {
		d.send(downloadResult{Op: op, Err: fmt.Errorf("open %s: %w", op.Path, err)})
		return
	}
	for idx, data := range chunkData {
		offset := int64(idx) * int64(op.ChunkSize)
		if _, err := f.WriteAt(data, offset); err != nil {
			_ = f.Close()
			d.send(downloadResult{Op: op, Err: fmt.Errorf("write chunk %d of %s: %w", idx, op.Path, err)})
			return
		}
	}
	if op.FileSize > 0 {
		if err := f.Truncate(op.FileSize); err != nil {
			_ = f.Close()
			d.send(downloadResult{Op: op, Err: fmt.Errorf("truncate %s: %w", op.Path, err)})
			return
		}
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		d.send(downloadResult{Op: op, Err: err})
		return
	}
	_ = f.Close()
	_ = os.Chmod(op.AbsPath, fs.FileMode(mode&0o7777))

	hash := compositeHash(op.ChunkHashes)
	d.echo.markFile(op.Path, hash)

	d.send(downloadResult{
		Op:           op,
		RemoteHash:   hash,
		RemoteStat:   stat,
		ConflictPath: conflictPath,
		Mode:         mode,
		Size:         op.FileSize,
		MtimeMs:      stat.Mtime,
	})
}

func (d *downloader) processSymlink(ctx context.Context, op downloadOp) {
	if op.Symlink == "" {
		remotePath := absoluteRemotePath(op.Path)
		target, err := d.fs.Readlink(ctx, remotePath)
		if err != nil {
			d.send(downloadResult{Op: op, Err: err})
			return
		}
		op.Symlink = target
	}
	if err := os.MkdirAll(filepath.Dir(op.AbsPath), 0o755); err != nil {
		d.send(downloadResult{Op: op, Err: err})
		return
	}
	if _, err := os.Lstat(op.AbsPath); err == nil {
		_ = os.Remove(op.AbsPath)
	}
	if err := os.Symlink(op.Symlink, op.AbsPath); err != nil {
		d.send(downloadResult{Op: op, Err: err})
		return
	}
	d.echo.markSymlink(op.Path, op.Symlink)
	d.send(downloadResult{Op: op, Target: op.Symlink})
}

func (d *downloader) processMkdir(ctx context.Context, op downloadOp) {
	if err := os.MkdirAll(op.AbsPath, 0o755); err != nil {
		d.send(downloadResult{Op: op, Err: fmt.Errorf("mkdir local %s: %w", op.Path, err)})
		return
	}
	d.echo.markDir(op.Path)
	d.send(downloadResult{Op: op})
}

func (d *downloader) processDelete(ctx context.Context, op downloadOp) {
	d.removeLocalFile(op)
	d.send(downloadResult{Op: op})
}

func (d *downloader) removeLocalFile(op downloadOp) {
	info, err := os.Lstat(op.AbsPath)
	if err != nil {
		return
	}
	d.echo.markDelete(op.Path)
	if info.IsDir() {
		_ = os.RemoveAll(op.AbsPath)
		return
	}
	_ = os.Remove(op.AbsPath)
}

func (d *downloader) processChmod(ctx context.Context, op downloadOp) {
	if err := os.Chmod(op.AbsPath, fs.FileMode(op.Mode&0o7777)); err != nil {
		d.send(downloadResult{Op: op, Err: err})
		return
	}
	d.send(downloadResult{Op: op, Mode: op.Mode})
}

// atomicWriteFile writes content into a sibling temp file and renames it
// over the destination so concurrent readers (like our own watcher) see
// either the old or the new file but never a partial. The temp filename
// embeds .afssync.tmp so the baseline ignore filter drops the watcher event.
func (d *downloader) atomicWriteFile(absPath string, data []byte, mode uint32) error {
	return writeAtomicFile(absPath, data, mode)
}

func (d *downloader) send(r downloadResult) {
	if d.results == nil {
		return
	}
	d.results <- r
}

func randomSuffix() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// echoSuppressor is the deterministic, hash-based mechanism we use to drop
// fsnotify events that are echoes of our own downloader writes. The
// downloader stamps the expected post-rename hash before the rename; the
// reconciler consults this on every local event and ignores it when the
// observed disk content matches.
type echoSuppressor struct {
	mu      sync.Mutex
	pending map[string]echoExpectation
}

type echoExpectation struct {
	kind string // "file" | "symlink" | "dir" | "delete"
	hash string // sha256 hex for files; symlink target for symlinks
}

func newEchoSuppressor() *echoSuppressor {
	return &echoSuppressor{pending: make(map[string]echoExpectation)}
}

func (e *echoSuppressor) markFile(rel, hash string) {
	e.set(rel, echoExpectation{kind: "file", hash: hash})
}

func (e *echoSuppressor) markSymlink(rel, target string) {
	e.set(rel, echoExpectation{kind: "symlink", hash: target})
}

func (e *echoSuppressor) markDir(rel string) {
	e.set(rel, echoExpectation{kind: "dir"})
}

func (e *echoSuppressor) markDelete(rel string) {
	e.set(rel, echoExpectation{kind: "delete"})
}

func (e *echoSuppressor) set(rel string, exp echoExpectation) {
	e.mu.Lock()
	if e.pending == nil {
		e.pending = make(map[string]echoExpectation)
	}
	e.pending[rel] = exp
	e.mu.Unlock()
}

// consume returns the expectation for rel and removes it. Returns ok==false
// if there is no pending echo. Use this in the reconciler before reading the
// local file from disk: if ok and the on-disk hash matches, drop the event.
func (e *echoSuppressor) consume(rel string) (echoExpectation, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	exp, ok := e.pending[rel]
	if ok {
		delete(e.pending, rel)
	}
	return exp, ok
}
