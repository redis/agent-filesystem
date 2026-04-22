package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

// uploadOpKind enumerates the mutations the uploader can apply to the live
// workspace root over the client.Client API.
type uploadOpKind int

const (
	opUploadFile uploadOpKind = iota + 1
	opUploadSymlink
	opUploadMkdir
	opUploadDelete
	opUploadChmod
)

// uploadOp is the work item the reconciler hands to the uploader. The
// reconciler stages content reads on the local filesystem and includes the
// hash so the uploader can detect drift between "what we read" and "what's
// remote right now" without rehashing.
type uploadOp struct {
	Kind        uploadOpKind
	Path        string // workspace-relative POSIX, no leading slash
	AbsPath     string // absolute local path (for diagnostic logging)
	Content     []byte // file body, only for non-chunked opUploadFile
	Mode        uint32
	Symlink     string // target, only for opUploadSymlink
	LocalHash   string // sha256 of Content or compositeHash for chunked
	StoredEntry SyncEntry
	HasStored   bool
	// Chunked upload fields (set when file > chunkThreshold).
	Chunked     bool
	FileSize    int64
	ChunkSize   int
	ChunkHashes []string // complete new manifest
	DirtyChunks []int    // indices of changed chunks
}

// uploadResult tells the reconciler how the upload landed so it can mark the
// SyncEntry up to date or trigger a conflict resolution loop.
type uploadResult struct {
	Op             uploadOp
	Err            error
	Conflict       bool
	RemoteHashSeen string
	RemoteStat     *client.StatResult
}

// uploader runs in its own goroutine, draining ops from the reconciler.
type uploader struct {
	fs           client.Client
	results      chan<- uploadResult
	maxFileBytes int64
	readonly     bool
	log          *syncLogger

	// Changelog emission. Zero values disable — see attachChangelog.
	rdb          *redis.Client
	storageID    string
	sessionID    string
	user         string
	agentID      string
	label        string
	agentVersion string
}

func newUploader(fs client.Client, results chan<- uploadResult, maxFileBytes int64, readonly bool, log *syncLogger) *uploader {
	if maxFileBytes <= 0 {
		maxFileBytes = 64 * 1024 * 1024
	}
	return &uploader{fs: fs, results: results, maxFileBytes: maxFileBytes, readonly: readonly, log: log}
}

// attachChangelog wires the uploader to emit one controlplane ChangeEntry per
// successful upload op. Session + workspace storage identity is baked in at
// start so each entry is attributable without extra plumbing per-op.
func (u *uploader) attachChangelog(rdb *redis.Client, storageID, sessionID, user, agentID, label, agentVersion string) {
	u.rdb = rdb
	u.storageID = storageID
	u.sessionID = sessionID
	u.user = user
	u.agentID = agentID
	u.label = label
	u.agentVersion = agentVersion
}

// emitChange writes one changelog row for op `result`. Called only when the
// upload landed successfully (no error, no conflict). Safe to call with the
// changelog unattached — it no-ops.
func (u *uploader) emitChange(ctx context.Context, r uploadResult) {
	if u.rdb == nil || u.storageID == "" || u.sessionID == "" {
		return
	}
	if r.Err != nil || r.Conflict {
		return
	}
	entry := controlplane.ChangeEntry{
		SessionID:    u.sessionID,
		AgentID:      u.agentID,
		User:         u.user,
		Label:        u.label,
		AgentVersion: u.agentVersion,
		Path:         r.Op.Path,
		Source:       controlplane.ChangeSourceAgentSync,
	}
	prevSize := int64(0)
	if r.Op.HasStored {
		prevSize = r.Op.StoredEntry.Size
		entry.PrevHash = r.Op.StoredEntry.RemoteHash
	}

	switch r.Op.Kind {
	case opUploadFile:
		entry.Op = controlplane.ChangeOpPut
		entry.ContentHash = r.Op.LocalHash
		if r.Op.Chunked {
			entry.SizeBytes = r.Op.FileSize
		} else {
			entry.SizeBytes = int64(len(r.Op.Content))
		}
		entry.DeltaBytes = entry.SizeBytes - prevSize
		entry.Mode = r.Op.Mode
	case opUploadSymlink:
		entry.Op = controlplane.ChangeOpSymlink
		entry.ContentHash = "symlink:" + r.Op.Symlink
		entry.Mode = r.Op.Mode
	case opUploadMkdir:
		entry.Op = controlplane.ChangeOpMkdir
		entry.Mode = r.Op.Mode
	case opUploadDelete:
		if r.Op.HasStored && r.Op.StoredEntry.Type == "dir" {
			entry.Op = controlplane.ChangeOpRmdir
		} else {
			entry.Op = controlplane.ChangeOpDelete
		}
		if prevSize > 0 {
			entry.DeltaBytes = -prevSize
		}
	case opUploadChmod:
		entry.Op = controlplane.ChangeOpChmod
		entry.Mode = r.Op.Mode
		entry.ContentHash = r.Op.LocalHash
	default:
		return
	}

	controlplane.WriteChangeEntries(ctx, u.rdb, u.storageID, []controlplane.ChangeEntry{entry})
}

// run drains in until ctx is cancelled. Each op is processed serially so the
// reconciler can rely on op-completion ordering when applying state updates.
func (u *uploader) run(ctx context.Context, in <-chan uploadOp) {
	for {
		select {
		case <-ctx.Done():
			return
		case op, ok := <-in:
			if !ok {
				return
			}
			if u.readonly {
				u.send(uploadResult{Op: op, Err: errors.New("uploader is read-only")})
				continue
			}
			u.process(ctx, op)
		}
	}
}

func (u *uploader) process(ctx context.Context, op uploadOp) {
	switch op.Kind {
	case opUploadFile:
		u.processFile(ctx, op)
	case opUploadSymlink:
		u.processSymlink(ctx, op)
	case opUploadMkdir:
		u.processMkdir(ctx, op)
	case opUploadDelete:
		u.processDelete(ctx, op)
	case opUploadChmod:
		u.processChmod(ctx, op)
	default:
		u.send(uploadResult{Op: op, Err: fmt.Errorf("unknown upload op kind: %d", op.Kind)})
	}
}

func (u *uploader) processFile(ctx context.Context, op uploadOp) {
	if op.Chunked {
		u.processChunkedFile(ctx, op)
		return
	}
	if int64(len(op.Content)) > u.maxFileBytes {
		u.send(uploadResult{Op: op, Err: fmt.Errorf("file %s is %d bytes, exceeds sync size cap of %d bytes", op.Path, len(op.Content), u.maxFileBytes)})
		return
	}

	// Drift check: if we have a stored RemoteHash, compare against the
	// current remote state. If they differ, the remote moved while we held
	// the local copy in our queue — that's a conflict.
	remotePath := absoluteRemotePath(op.Path)
	stat, statErr := u.fs.Stat(ctx, remotePath)
	if statErr != nil && !isClientNotFound(statErr) {
		u.send(uploadResult{Op: op, Err: fmt.Errorf("stat remote %s: %w", op.Path, statErr)})
		return
	}
	if stat != nil && op.HasStored && op.StoredEntry.RemoteHash != "" {
		remoteData, err := u.fs.Cat(ctx, remotePath)
		if err != nil {
			u.send(uploadResult{Op: op, Err: fmt.Errorf("read remote %s: %w", op.Path, err)})
			return
		}
		remoteHash := sha256Hex(remoteData)
		if remoteHash != op.StoredEntry.RemoteHash {
			u.send(uploadResult{Op: op, Conflict: true, RemoteHashSeen: remoteHash, RemoteStat: stat})
			return
		}
	}

	mode := op.Mode
	if mode == 0 {
		mode = 0o644
	}
	// Echo handles both create-and-write and write-existing in a single
	// round trip; the native client falls back to createFile when the
	// path is missing. We don't pre-create with CreateFile because that
	// leaves the inode briefly empty and other watchers can race against
	// the empty state.
	if err := u.fs.Echo(ctx, remotePath, op.Content); err != nil {
		u.send(uploadResult{Op: op, Err: fmt.Errorf("write remote %s: %w", op.Path, err)})
		return
	}
	// Best-effort mode sync; ignore mode-only errors so transient
	// permission errors don't block content propagation.
	_ = u.fs.Chmod(ctx, remotePath, mode)
	newStat, err := u.fs.Stat(ctx, remotePath)
	if err != nil {
		u.send(uploadResult{Op: op, Err: fmt.Errorf("post-write stat %s: %w", op.Path, err)})
		return
	}
	u.send(uploadResult{Op: op, RemoteHashSeen: op.LocalHash, RemoteStat: newStat})
}

func (u *uploader) processChunkedFile(ctx context.Context, op uploadOp) {
	remotePath := absoluteRemotePath(op.Path)

	// Drift check: compare remote chunk manifest against what we stored.
	_, remoteHashes, err := u.fs.ChunkMeta(ctx, remotePath)
	if err != nil && !isClientNotFound(err) {
		u.send(uploadResult{Op: op, Err: fmt.Errorf("chunk meta %s: %w", op.Path, err)})
		return
	}
	// If remote has chunks and they differ from our stored baseline, someone
	// else changed the file → conflict.
	if op.HasStored && op.StoredEntry.RemoteHash != "" && len(remoteHashes) > 0 {
		remoteComposite := compositeHash(remoteHashes)
		if remoteComposite != op.StoredEntry.RemoteHash {
			stat, _ := u.fs.Stat(ctx, remotePath)
			u.send(uploadResult{Op: op, Conflict: true, RemoteHashSeen: remoteComposite, RemoteStat: stat})
			return
		}
	}

	// Upload dirty chunks in batches.
	chunks := make(map[int][]byte, len(op.DirtyChunks))
	for _, idx := range op.DirtyChunks {
		data, err := readChunkFromDisk(op.AbsPath, idx, op.ChunkSize)
		if err != nil {
			u.send(uploadResult{Op: op, Err: fmt.Errorf("read chunk %d of %s: %w", idx, op.Path, err)})
			return
		}
		chunks[idx] = data
	}

	// If file doesn't exist remotely yet, create it first.
	stat, _ := u.fs.Stat(ctx, remotePath)
	if stat == nil {
		if _, _, err := u.fs.CreateFile(ctx, remotePath, op.Mode, false); err != nil && !isClientAlreadyExists(err) {
			u.send(uploadResult{Op: op, Err: fmt.Errorf("create %s: %w", op.Path, err)})
			return
		}
	}

	if err := u.fs.WriteChunks(ctx, remotePath, chunks, op.ChunkSize, op.FileSize, op.ChunkHashes); err != nil {
		u.send(uploadResult{Op: op, Err: fmt.Errorf("write chunks %s: %w", op.Path, err)})
		return
	}

	_ = u.fs.Chmod(ctx, remotePath, op.Mode)
	newStat, err := u.fs.Stat(ctx, remotePath)
	if err != nil {
		u.send(uploadResult{Op: op, Err: fmt.Errorf("post-write stat %s: %w", op.Path, err)})
		return
	}
	u.send(uploadResult{Op: op, RemoteHashSeen: op.LocalHash, RemoteStat: newStat})
}

func (u *uploader) processSymlink(ctx context.Context, op uploadOp) {
	remotePath := absoluteRemotePath(op.Path)
	// Best-effort delete first; Ln on existing path returns an error.
	if existing, err := u.fs.Stat(ctx, remotePath); err == nil && existing != nil {
		if rmErr := u.fs.Rm(ctx, remotePath); rmErr != nil {
			u.send(uploadResult{Op: op, Err: fmt.Errorf("replace symlink %s: %w", op.Path, rmErr)})
			return
		}
	}
	if err := u.fs.Ln(ctx, op.Symlink, remotePath); err != nil {
		u.send(uploadResult{Op: op, Err: fmt.Errorf("create symlink %s: %w", op.Path, err)})
		return
	}
	stat, _ := u.fs.Stat(ctx, remotePath)
	u.send(uploadResult{Op: op, RemoteStat: stat})
}

func (u *uploader) processMkdir(ctx context.Context, op uploadOp) {
	remotePath := absoluteRemotePath(op.Path)
	if err := u.fs.Mkdir(ctx, remotePath); err != nil {
		// If it already exists we treat as success (the live root may have
		// the dir from a prior run).
		if !isClientAlreadyExists(err) {
			u.send(uploadResult{Op: op, Err: fmt.Errorf("mkdir remote %s: %w", op.Path, err)})
			return
		}
	}
	stat, _ := u.fs.Stat(ctx, remotePath)
	u.send(uploadResult{Op: op, RemoteStat: stat})
}

func (u *uploader) processDelete(ctx context.Context, op uploadOp) {
	remotePath := absoluteRemotePath(op.Path)
	if err := u.fs.Rm(ctx, remotePath); err != nil && !isClientNotFound(err) {
		u.send(uploadResult{Op: op, Err: fmt.Errorf("rm remote %s: %w", op.Path, err)})
		return
	}
	u.send(uploadResult{Op: op})
}

func (u *uploader) processChmod(ctx context.Context, op uploadOp) {
	remotePath := absoluteRemotePath(op.Path)
	if err := u.fs.Chmod(ctx, remotePath, op.Mode); err != nil {
		u.send(uploadResult{Op: op, Err: fmt.Errorf("chmod remote %s: %w", op.Path, err)})
		return
	}
	stat, _ := u.fs.Stat(ctx, remotePath)
	u.send(uploadResult{Op: op, RemoteStat: stat})
}

func (u *uploader) send(r uploadResult) {
	// Emit the changelog entry before forwarding the result so that a
	// blocked reconciler channel doesn't stall the changelog write. The
	// helper is no-op when changelog wiring is unattached.
	u.emitChange(context.Background(), r)
	if u.results == nil {
		return
	}
	u.results <- r
}

// absoluteRemotePath converts a workspace-relative POSIX path to the
// absolute form expected by client.Client (always rooted at "/"). Empty
// strings collapse to "/".
func absoluteRemotePath(rel string) string {
	if rel == "" || rel == "." {
		return "/"
	}
	if rel[0] == '/' {
		return rel
	}
	return "/" + rel
}

// isClientNotFound is a deliberately string-matching helper. The native client
// returns plain errors for "no such inode" / "no such path"; we don't want to
// import the internal package just to type-assert. Adjust the matched
// substrings if the client changes its error format.
func isClientNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsAny(msg, "no such file", "not found", "ENOENT", "does not exist")
}

func isClientAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsAny(msg, "exists", "EEXIST")
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub == "" {
			continue
		}
		if indexOfFold(s, sub) >= 0 {
			return true
		}
	}
	return false
}

// indexOfFold is a small case-insensitive substring search. We avoid
// strings.EqualFold/ToLower allocations on the hot path.
func indexOfFold(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			c1 := s[i+j]
			c2 := sub[j]
			if 'A' <= c1 && c1 <= 'Z' {
				c1 += 'a' - 'A'
			}
			if 'A' <= c2 && c2 <= 'Z' {
				c2 += 'a' - 'A'
			}
			if c1 != c2 {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
