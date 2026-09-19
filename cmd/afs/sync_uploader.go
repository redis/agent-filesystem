package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

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
	opUploadRename
)

// uploadOp is the work item the reconciler hands to the uploader. The
// reconciler stages content reads on the local filesystem and includes the
// hash so the uploader can detect drift between "what we read" and "what's
// remote right now" without rehashing.
type uploadOp struct {
	Kind          uploadOpKind
	Path          string // workspace-relative POSIX, no leading slash
	PrevPath      string // previous workspace-relative POSIX path for renames
	AbsPath       string // absolute local path (for diagnostic logging)
	Content       []byte // file body, only for non-chunked opUploadFile
	Mode          uint32
	Symlink       string // target, only for opUploadSymlink
	LocalHash     string // sha256 of Content or compositeHash for chunked
	LocalIdentity string
	LocalMtimeMs  int64 // local metadata captured when the content was staged
	StoredEntry   SyncEntry
	HasStored     bool
	Tracked       bool   // reconciler keeps this operation pending through result application
	RenameVersion uint64 // provisional destination baseline installed when a rename was staged
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
	Skipped        bool // queued local content changed; reconcile again without updating the baseline
	RemoteHashSeen string
	RemoteStat     *client.StatResult
}

// uploader runs in its own goroutine, draining ops from the reconciler.
type uploader struct {
	stopCh         <-chan struct{}
	stoppedResults []uploadResult
	runContext     context.Context
	fs             client.Client
	results        chan<- uploadResult
	maxFileBytes   int64
	readonly       bool
	log            *syncLogger

	// Changelog emission. Zero values disable — see mountChangelog.
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

func (u *uploader) mountChangelog(rdb *redis.Client, storageID, sessionID, user, agentID, label, agentVersion string) {
	u.attachChangelog(rdb, storageID, sessionID, user, agentID, label, agentVersion)
}

// emitChange writes one changelog row for op `result`. Called only when the
// upload landed successfully (no error, no conflict). Safe to call with the
// changelog unmounted — it no-ops.
func (u *uploader) emitChange(ctx context.Context, r uploadResult) {
	if u.rdb == nil || u.storageID == "" || u.sessionID == "" {
		return
	}
	if r.Err != nil || r.Conflict || r.Skipped {
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
	case opUploadRename:
		entry.Op = controlplane.ChangeOpRename
		entry.PrevPath = r.Op.PrevPath
		entry.Mode = r.Op.Mode
		entry.ContentHash = r.Op.LocalHash
	default:
		return
	}
	if version := u.recordVersionMutation(ctx, r); version != nil {
		entry.FileID = version.FileID
		entry.VersionID = version.VersionID
	}

	controlplane.WriteChangeEntries(ctx, u.rdb, u.storageID, []controlplane.ChangeEntry{entry})
}

func (u *uploader) recordVersionMutation(ctx context.Context, r uploadResult) *controlplane.FileVersion {
	beforePath := absoluteRemotePath(r.Op.Path)
	if r.Op.Kind == opUploadRename && strings.TrimSpace(r.Op.PrevPath) != "" {
		beforePath = absoluteRemotePath(r.Op.PrevPath)
	}
	before := versionedSnapshotFromSyncEntry(beforePath, r.Op.StoredEntry, r.Op.HasStored)
	after, shouldTrack, err := versionedSnapshotFromUploadResult(r)
	if err != nil {
		if u.log != nil {
			u.log.Err("version history", err.Error())
		}
		return nil
	}
	if !shouldTrack {
		return nil
	}
	version, err := controlplane.NewStore(u.rdb).RecordFileVersionMutation(ctx, u.storageID, before, after, controlplane.FileVersionMutationMetadata{
		Source:    controlplane.ChangeSourceAgentSync,
		SessionID: u.sessionID,
		AgentID:   u.agentID,
		User:      u.user,
	})
	if err != nil {
		if u.log != nil {
			u.log.Err("version history", err.Error())
		}
		return nil
	}
	return version
}

func versionedSnapshotFromSyncEntry(path string, entry SyncEntry, hasStored bool) controlplane.VersionedFileSnapshot {
	if !hasStored || entry.Type == "dir" {
		return controlplane.VersionedFileSnapshot{Path: path}
	}
	snapshot := controlplane.VersionedFileSnapshot{
		Path:   path,
		Exists: !entry.Deleted,
		Mode:   entry.Mode,
	}
	switch entry.Type {
	case "symlink":
		snapshot.Kind = "symlink"
		snapshot.Target = entry.Target
		snapshot.ContentHash = entry.RemoteHash
		snapshot.SizeBytes = int64(len(entry.Target))
	default:
		snapshot.Kind = "file"
		snapshot.ContentHash = entry.RemoteHash
		snapshot.BlobID = entry.RemoteHash
		snapshot.SizeBytes = entry.Size
	}
	return snapshot
}

func versionedSnapshotFromUploadResult(r uploadResult) (controlplane.VersionedFileSnapshot, bool, error) {
	path := absoluteRemotePath(r.Op.Path)
	switch r.Op.Kind {
	case opUploadFile:
		mode := r.Op.Mode
		if mode == 0 {
			mode = 0o644
		}
		snapshot := controlplane.VersionedFileSnapshot{
			Path:        path,
			Exists:      true,
			Kind:        "file",
			Mode:        mode,
			ContentHash: r.Op.LocalHash,
			BlobID:      r.Op.LocalHash,
		}
		if r.Op.Chunked {
			data := r.Op.Content
			var err error
			if data == nil {
				data, err = os.ReadFile(r.Op.AbsPath)
			}
			if err == nil && compositeHash(uploadChunkHashes(data, r.Op.ChunkSize)) != r.Op.LocalHash {
				err = errors.New("chunked history source changed after upload")
			}
			if err != nil {
				return controlplane.VersionedFileSnapshot{}, true, fmt.Errorf("read chunked upload %s: %w", r.Op.AbsPath, err)
			}
			snapshot.Content = data
			snapshot.SizeBytes = int64(len(data))
		} else {
			snapshot.Content = append([]byte(nil), r.Op.Content...)
			snapshot.SizeBytes = int64(len(snapshot.Content))
		}
		if snapshot.SizeBytes == 0 && r.RemoteStat != nil {
			snapshot.SizeBytes = r.RemoteStat.Size
		}
		return snapshot, true, nil
	case opUploadSymlink:
		return controlplane.VersionedFileSnapshot{
			Path:   path,
			Exists: true,
			Kind:   "symlink",
			Mode:   r.Op.Mode,
			Target: r.Op.Symlink,
		}, true, nil
	case opUploadDelete:
		if r.Op.HasStored && r.Op.StoredEntry.Type == "dir" {
			return controlplane.VersionedFileSnapshot{Path: path}, false, nil
		}
		return controlplane.VersionedFileSnapshot{Path: path}, true, nil
	case opUploadRename:
		if !r.Op.HasStored {
			return controlplane.VersionedFileSnapshot{}, false, nil
		}
		snapshot := versionedSnapshotFromSyncEntry(path, r.Op.StoredEntry, true)
		snapshot.Path = path
		snapshot.Exists = true
		if snapshot.Mode == 0 {
			snapshot.Mode = r.Op.Mode
		}
		return snapshot, true, nil
	case opUploadChmod:
		before := versionedSnapshotFromSyncEntry(path, r.Op.StoredEntry, r.Op.HasStored)
		if !before.Exists {
			return controlplane.VersionedFileSnapshot{}, false, nil
		}
		after := before
		after.Mode = r.Op.Mode
		return after, true, nil
	default:
		return controlplane.VersionedFileSnapshot{}, false, nil
	}
}

// run drains in until ctx is cancelled. Each op is processed serially so the
// reconciler can rely on op-completion ordering when applying state updates.
func (u *uploader) run(ctx context.Context, in <-chan uploadOp) {
	if u.runContext == nil {
		u.runContext = ctx
	}
	for {
		select {
		case <-ctx.Done():
			return
		case op, ok := <-in:
			if !ok || ctx.Err() != nil {
				return
			}
			if u.readonly {
				u.send(uploadResult{Op: op, Err: errors.New("uploader is read-only")})
				continue
			}
			u.process(u.runContext, op)
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
	case opUploadRename:
		u.processRename(ctx, op)
	default:
		u.send(uploadResult{Op: op, Err: fmt.Errorf("unknown upload op kind: %d", op.Kind)})
	}
}

func (u *uploader) processFile(ctx context.Context, op uploadOp) {
	if op.Chunked {
		u.processChunkedFile(ctx, op)
		return
	}
	if !u.queuedFileCurrent(op) {
		return
	}
	if int64(len(op.Content)) > u.maxFileBytes {
		u.send(uploadResult{Op: op, Err: fmt.Errorf("file %s is %d bytes, exceeds sync size cap of %d bytes", op.Path, len(op.Content), u.maxFileBytes)})
		return
	}
	snapshot, ok := u.uploadSnapshot(ctx, op)
	if !ok {
		return
	}
	if snapshot.Stat != nil {
		remoteHash := sha256Hex(snapshot.Content)
		if remoteHash == op.LocalHash {
			u.finishMatchingSnapshot(ctx, op, snapshot)
			return
		}
		matches := remoteHash == op.StoredEntry.RemoteHash
		if !matches && op.StoredEntry.ChunkSize > 0 {
			matches = compositeHash(uploadChunkHashes(snapshot.Content, op.StoredEntry.ChunkSize)) == op.StoredEntry.RemoteHash
		}
		if op.HasStored && op.StoredEntry.RemoteHash != "" && !matches {
			u.send(uploadResult{Op: op, Conflict: true, RemoteHashSeen: remoteHash, RemoteStat: snapshot.Stat})
			return
		}
	}
	if !u.queuedFileCurrent(op) {
		return
	}
	mode := op.Mode
	if mode == 0 {
		mode = 0o644
	}
	var result client.MutationResult
	writeCtx := client.WithMutationResult(client.WithFileMode(snapshot.WriteContext(ctx), mode), &result)
	if err := u.fs.Echo(writeCtx, snapshot.Path, op.Content); err != nil {
		u.uploadWriteError(op, err)
		return
	}
	u.finishCommittedUpload(ctx, op, mode, result)
}

func (u *uploader) uploadSnapshot(ctx context.Context, op uploadOp) (client.FileSnapshot, bool) {
	snapshot, err := client.ReadFileSnapshot(ctx, u.fs, absoluteRemotePath(op.Path))
	if err != nil {
		u.uploadWriteError(op, err)
		return snapshot, false
	}
	return snapshot, u.queuedFileCurrent(op)
}

func (u *uploader) uploadWriteError(op uploadOp, err error) {
	if errors.Is(err, client.ErrWriteConflict) {
		u.send(uploadResult{Op: op, Conflict: true})
		return
	}
	u.send(uploadResult{Op: op, Err: fmt.Errorf("publish remote %s: %w", op.Path, err)})
}

func (u *uploader) processChunkedFile(ctx context.Context, op uploadOp) {
	if !u.queuedFileCurrent(op) {
		return
	}
	snapshot, ok := u.uploadSnapshot(ctx, op)
	if !ok {
		return
	}
	if snapshot.Stat != nil {
		remoteHash := sha256Hex(snapshot.Content)
		remoteComposite := compositeHash(uploadChunkHashes(snapshot.Content, op.ChunkSize))
		if remoteComposite == op.LocalHash {
			u.finishMatchingSnapshot(ctx, op, snapshot)
			return
		}
		baselineComposite := remoteComposite
		if op.StoredEntry.ChunkSize > 0 && op.StoredEntry.ChunkSize != op.ChunkSize {
			baselineComposite = compositeHash(uploadChunkHashes(snapshot.Content, op.StoredEntry.ChunkSize))
		}
		if op.HasStored && op.StoredEntry.RemoteHash != "" && remoteHash != op.StoredEntry.RemoteHash && baselineComposite != op.StoredEntry.RemoteHash {
			u.send(uploadResult{Op: op, Conflict: true, RemoteHashSeen: remoteComposite, RemoteStat: snapshot.Stat})
			return
		}
	}
	dirtyChunks := op.DirtyChunks
	if snapshot.Stat == nil {
		dirtyChunks = make([]int, len(op.ChunkHashes))
		for i := range dirtyChunks {
			dirtyChunks[i] = i
		}
	}
	chunks := make(map[int][]byte, len(dirtyChunks))
	for _, idx := range dirtyChunks {
		if idx < 0 || idx >= len(op.ChunkHashes) {
			u.send(uploadResult{Op: op, Err: errors.New("invalid dirty chunk index")})
			return
		}
		data, err := readChunkFromDisk(op.AbsPath, idx, op.ChunkSize)
		if err != nil {
			u.uploadWriteError(op, err)
			return
		}
		// These frozen buffers, not another disk read, are the publication payload.
		if sha256Hex(data) != op.ChunkHashes[idx] {
			u.send(uploadResult{Op: op, Skipped: true})
			return
		}
		chunks[idx] = data
	}
	if !u.queuedFileCurrent(op) {
		return
	}
	// Retain the exact full candidate for history. Unchanged chunks come from
	// the verified snapshot; changed chunks are the frozen buffers just checked.
	if op.FileSize < 0 || op.FileSize > u.maxFileBytes {
		u.uploadWriteError(op, errors.New("chunked file exceeds size cap"))
		return
	}
	candidate := make([]byte, op.FileSize)
	copy(candidate, snapshot.Content)
	for idx, data := range chunks {
		off := int64(idx) * int64(op.ChunkSize)
		if off < 0 || off+int64(len(data)) > op.FileSize {
			u.uploadWriteError(op, errors.New("chunk outside file"))
			return
		}
		copy(candidate[off:], data)
	}
	if compositeHash(uploadChunkHashes(candidate, op.ChunkSize)) != op.LocalHash {
		u.send(uploadResult{Op: op, Skipped: true})
		return
	}
	op.Content = candidate
	mode := op.Mode
	if mode == 0 {
		mode = 0o644
	}
	var result client.MutationResult
	writeCtx := client.WithMutationResult(client.WithFileMode(snapshot.WriteContext(ctx), mode), &result)
	// WriteChunks stages and claims an absent file in a single publication.
	// Pre-creating an empty inode would expose partial state and lose ExpectAbsent.
	if err := u.fs.WriteChunks(writeCtx, snapshot.Path, chunks, op.ChunkSize, op.FileSize, op.ChunkHashes); err != nil {
		u.uploadWriteError(op, err)
		return
	}
	u.finishCommittedUpload(ctx, op, mode, result)
}

func (u *uploader) finishCommittedUpload(ctx context.Context, op uploadOp, mode uint32, result client.MutationResult) {
	if result.Stat != nil {
		// Never attach a later Stat to our older content hash. It may be a peer's write.
		u.send(uploadResult{Op: op, RemoteHashSeen: op.LocalHash, RemoteStat: result.Stat})
		return
	}
	// Alternate Client implementations can omit result capture. Verify their
	// content before attempting a revision-guarded mode update.
	snapshot, err := client.ReadFileSnapshot(ctx, u.fs, absoluteRemotePath(op.Path))
	if err != nil {
		u.uploadWriteError(op, err)
		return
	}
	hash := sha256Hex(snapshot.Content)
	if op.Chunked {
		hash = compositeHash(uploadChunkHashes(snapshot.Content, op.ChunkSize))
	}
	if snapshot.Stat == nil || hash != op.LocalHash {
		u.send(uploadResult{Op: op, Conflict: true})
		return
	}
	op.Mode = mode
	u.finishMatchingSnapshot(ctx, op, snapshot)
}

func (u *uploader) finishMatchingSnapshot(ctx context.Context, op uploadOp, snapshot client.FileSnapshot) {
	stat := snapshot.Stat
	mode := op.Mode
	if mode == 0 {
		mode = 0o644
	}
	if !u.queuedFileCurrent(op) {
		return
	}
	if op.HasStored && op.StoredEntry.Mode != 0 && stat.Mode != op.StoredEntry.Mode && stat.Mode != mode {
		u.send(uploadResult{Op: op, Skipped: mode == op.StoredEntry.Mode, Conflict: mode != op.StoredEntry.Mode})
		return
	}
	if stat.Mode == mode {
		u.send(uploadResult{Op: op, RemoteHashSeen: op.LocalHash, RemoteStat: stat})
		return
	}
	var result client.MutationResult
	writeCtx := client.WithMutationResult(snapshot.WriteContext(ctx), &result)
	if err := u.fs.Chmod(writeCtx, snapshot.Path, mode); err != nil {
		u.uploadWriteError(op, err)
		return
	}
	if result.Stat != nil {
		u.send(uploadResult{Op: op, RemoteHashSeen: op.LocalHash, RemoteStat: result.Stat})
		return
	}
	after, err := client.ReadFileSnapshot(ctx, u.fs, snapshot.Path)
	if err != nil {
		u.uploadWriteError(op, err)
		return
	}
	if after.Stat == nil || after.Stat.Mode != mode || !bytes.Equal(after.Content, snapshot.Content) {
		u.send(uploadResult{Op: op, Conflict: true})
		return
	}
	u.send(uploadResult{Op: op, RemoteHashSeen: op.LocalHash, RemoteStat: after.Stat})
}

func (u *uploader) finishMatchingFileUpload(ctx context.Context, op uploadOp, remotePath string, mode uint32) {
	snapshot, err := client.ReadFileSnapshot(ctx, u.fs, remotePath)
	if err != nil {
		u.uploadWriteError(op, err)
		return
	}
	if snapshot.Stat == nil {
		u.send(uploadResult{Op: op, Skipped: true})
		return
	}
	op.Mode = mode
	u.finishMatchingSnapshot(ctx, op, snapshot)
}

func (u *uploader) queuedFileCurrent(op uploadOp) bool {
	// Older callers did not capture metadata with the staged content.
	if op.LocalMtimeMs == 0 {
		return true
	}
	info, err := os.Lstat(op.AbsPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		u.send(uploadResult{Op: op, Err: fmt.Errorf("stat queued file %s: %w", op.Path, err)})
		return false
	}
	size := int64(len(op.Content))
	if op.Chunked {
		size = op.FileSize
	}
	if err != nil || !info.Mode().IsRegular() || info.ModTime().UnixMilli() != op.LocalMtimeMs || info.Size() != size || uint32(info.Mode().Perm()) != op.Mode || op.LocalIdentity != "" && localFileIdentity(info) != op.LocalIdentity {
		u.send(uploadResult{Op: op, Skipped: true})
		return false
	}
	if !op.Chunked {
		data, readErr := os.ReadFile(op.AbsPath)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			u.send(uploadResult{Op: op, Err: fmt.Errorf("read queued file %s: %w", op.Path, readErr)})
			return false
		}
		if readErr != nil || !bytes.Equal(data, op.Content) {
			u.send(uploadResult{Op: op, Skipped: true})
			return false
		}
	}
	return true
}

func uploadChunkHashes(data []byte, chunkSize int) []string {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	var hashes []string
	for offset := 0; offset < len(data); offset += chunkSize {
		end := min(offset+chunkSize, len(data))
		hashes = append(hashes, sha256Hex(data[offset:end]))
	}
	return hashes
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
	if op.AbsPath != "" {
		_, err := os.Lstat(op.AbsPath)
		if err == nil {
			u.send(uploadResult{Op: op, Skipped: true})
			return
		}
		if !errors.Is(err, os.ErrNotExist) {
			u.send(uploadResult{Op: op, Err: fmt.Errorf("stat queued delete %s: %w", op.Path, err)})
			return
		}
	}
	remotePath := absoluteRemotePath(op.Path)
	if err := u.fs.Rm(ctx, remotePath); err != nil && !errors.Is(err, redis.Nil) && !isClientNotFound(err) {
		u.send(uploadResult{Op: op, Err: fmt.Errorf("rm remote %s: %w", op.Path, err)})
		return
	}
	u.send(uploadResult{Op: op})
}

func (u *uploader) processRename(ctx context.Context, op uploadOp) {
	if strings.TrimSpace(op.PrevPath) == "" {
		u.send(uploadResult{Op: op, Err: errors.New("rename upload missing previous path")})
		return
	}
	srcPath := absoluteRemotePath(op.PrevPath)
	dstPath := absoluteRemotePath(op.Path)
	if err := u.fs.Rename(ctx, srcPath, dstPath, 0); err != nil {
		if errors.Is(err, redis.Nil) || isClientNotFound(err) {
			// A source or destination parent can disappear before a queued
			// rename executes. Reconcile the current local destination rather
			// than retaining a baseline for a rename that did not complete.
			u.send(uploadResult{Op: op, Skipped: true})
			return
		}
		u.send(uploadResult{Op: op, Err: fmt.Errorf("rename remote %s -> %s: %w", op.PrevPath, op.Path, err)})
		return
	}
	stat, err := u.fs.Stat(ctx, dstPath)
	if err != nil {
		u.send(uploadResult{Op: op, Err: fmt.Errorf("post-rename stat %s: %w", op.Path, err)})
		return
	}
	u.send(uploadResult{Op: op, RemoteStat: stat})
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
	// helper is no-op when changelog wiring is unmounted.
	changeCtx := u.runContext
	if changeCtx == nil {
		changeCtx = context.Background()
	}
	u.emitChange(changeCtx, r)
	if u.results == nil {
		return
	}
	select {
	case u.results <- r:
	case <-u.stopCh:
		// Preserve the final result for save after every old worker joins.
		u.stoppedResults = append(u.stoppedResults, r)
	}
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
