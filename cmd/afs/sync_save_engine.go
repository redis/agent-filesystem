package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/redis/agent-filesystem/mount/client"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"
)

// syncSaveReceipt confirms visibility in the Redis live tree. It does not
// acknowledge Redis disk persistence or a snapshot under concurrent writers.
type syncSaveReceipt struct {
	Entries     int       `json:"entries"`
	Files       int       `json:"files"`
	Bytes       int64     `json:"bytes"`
	TreeSHA256  string    `json:"tree_sha256"`
	CompletedAt time.Time `json:"completed_at"`
}

// Metadata used only for the next sync baseline is excluded from tree equality.
// Contents are hashed as whole files even when steady sync uses chunk hashes.
type syncSaveEntry struct {
	Type   string `json:"type"`
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size,omitempty"`
	Hash   string `json:"sha256,omitempty"`
	Target string `json:"target,omitempty"`

	stat          *client.StatResult
	mtimeMs       int64
	identity      string
	baselineHash  string
	chunkSize     int
	chunkHashes   []string
	hasChunks     bool
	chunksCurrent bool
}

type syncSaveTree map[string]syncSaveEntry

// saveSyncTree uses the local manifest verified by the caller after joining all
// steady-state workers. The application must stop writing. It never changes the
// local tree. A failure can leave partially applied remote writes and must not
// be treated as a save.
func saveSyncTree(ctx context.Context, r *reconciler, local syncSaveTree) (syncSaveReceipt, error) {
	var receipt syncSaveReceipt
	if err := ctx.Err(); err != nil {
		return receipt, err
	}
	if r.readonly {
		return receipt, fmt.Errorf("cannot save a read-only sync mount")
	}
	baseline := r.state.snapshot()
	remote, err := scanSyncSaveRemote(ctx, r, baseline)
	if err != nil {
		return receipt, fmt.Errorf("scan Redis tree: %w", err)
	}
	remove, write, err := planSyncSave(local, remote, baseline)
	if err != nil {
		return receipt, err
	}
	// Complete preflight before the first write, including edits during the
	// remote scan. Do not use warmStart: its plan may download or delete locally.
	check, err := scanSyncSaveLocal(ctx, r)
	if err != nil {
		return receipt, err
	}
	if err := compareSyncSaveTrees(local, check); err != nil {
		return receipt, fmt.Errorf("local tree changed during save: %w", err)
	}
	for _, rel := range remove {
		if err := removeSyncSaveEntry(ctx, r, rel, remote[rel]); err != nil {
			return receipt, fmt.Errorf("save remove %s: %w", rel, err)
		}
		delete(remote, rel)
	}
	for _, rel := range write {
		if err := writeSyncSaveEntry(ctx, r, rel, local[rel], remote[rel]); err != nil {
			return receipt, fmt.Errorf("save %s: %w", rel, err)
		}
	}
	// EchoCreate preserves any prior chunk manifest. Normalize it from the
	// saved bytes so resumed steady sync can compare compatible baselines.
	for rel, entry := range local {
		if entry.Type != "file" {
			continue
		}
		previous := remote[rel]
		if entry.Size <= int64(syncSaveChunkThreshold(r)) && !previous.hasChunks {
			continue
		}
		if sameSyncSaveContent(entry, previous) && previous.chunksCurrent {
			continue
		}
		data, err := readSyncSaveFile(ctx, filepath.Join(r.root, filepath.FromSlash(rel)), entry, r.maxFileBytes)
		if err != nil {
			return receipt, err
		}
		chunkSize, hashes := syncSaveChunks(data, r)
		snapshot, err := client.ReadFileSnapshot(ctx, r.fs, absoluteRemotePath(rel))
		if err != nil {
			return receipt, err
		}
		if snapshot.Stat == nil || sha256Hex(snapshot.Content) != entry.Hash {
			return receipt, fmt.Errorf("save chunk metadata %s: remote file changed", rel)
		}
		if err := r.fs.WriteChunks(snapshot.WriteContext(ctx), absoluteRemotePath(rel), nil, chunkSize, entry.Size, hashes); err != nil {
			return receipt, fmt.Errorf("save chunk metadata %s: %w", rel, err)
		}
	}
	verifiedRemote, err := scanSyncSaveRemote(ctx, r, baseline)
	if err != nil {
		return receipt, fmt.Errorf("verify Redis tree: %w", err)
	}
	if err := compareSyncSaveTrees(local, verifiedRemote); err != nil {
		return receipt, fmt.Errorf("Redis tree does not match save: %w", err)
	}
	verifiedLocal, err := scanSyncSaveLocal(ctx, r)
	if err != nil {
		return receipt, err
	}
	if err := compareSyncSaveTrees(local, verifiedLocal); err != nil {
		return receipt, fmt.Errorf("local tree changed during save: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return receipt, err
	}
	// Publish a baseline only after all bytes, modes and paths were verified.
	// The caller has stopped the state writer along with the old generation.
	next := cloneSyncState(baseline)
	next.Entries = make(map[string]SyncEntry, len(local))
	now := time.Now().UTC()
	for rel, entry := range verifiedLocal {
		observed := verifiedRemote[rel]
		if entry.Type == "file" && !observed.chunksCurrent {
			return receipt, fmt.Errorf("Redis chunk metadata does not match save: %s", rel)
		}
		saved := SyncEntry{
			Type: entry.Type, Mode: entry.Mode, Size: entry.Size,
			LocalHash: entry.Hash, RemoteHash: entry.Hash, Target: entry.Target,
			LocalIdentity: entry.identity, LocalMtimeMs: entry.mtimeMs,
			RemoteMtimeMs: verifiedRemote[rel].mtimeMs, LastSyncedAt: now,
			Version: next.NextVersion,
		}
		if observed.stat != nil {
			saved.RemoteRevision = observed.stat.Revision
			saved.RemoteInode = observed.stat.Inode
		}
		if observed.chunkSize > 0 {
			saved.ChunkSize, saved.ChunkHashes = observed.chunkSize, observed.chunkHashes
			saved.LocalHash, saved.RemoteHash = compositeHash(observed.chunkHashes), compositeHash(observed.chunkHashes)
		}
		next.Entries[rel] = saved
		next.NextVersion++
	}
	if err := saveSyncState(next); err != nil {
		return receipt, fmt.Errorf("persist save state: %w", err)
	}
	r.state.mu.Lock()
	r.state.state = next
	r.state.dirty = false
	r.state.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return receipt, err
	}
	encoded, err := json.Marshal(local) // encoding/json sorts map keys.
	if err != nil {
		return receipt, err
	}
	receipt = syncSaveReceipt{Entries: len(local), TreeSHA256: sha256Hex(encoded), CompletedAt: time.Now().UTC()}
	for _, entry := range local {
		if entry.Type == "file" {
			receipt.Files++
			receipt.Bytes += entry.Size
		}
	}
	return receipt, nil
}

func syncSaveIgnored(r *reconciler, rel string, isDir bool) bool {
	return isSyncControlPath(rel) || isBaselineIgnored(rel) || r.ignore.shouldIgnore(rel, isDir)
}

func scanSyncSaveLocal(ctx context.Context, r *reconciler) (syncSaveTree, error) {
	root, err := os.Lstat(r.root)
	if err != nil {
		return nil, err
	}
	if !root.IsDir() {
		return nil, fmt.Errorf("local root is not a directory")
	}
	tree := make(syncSaveTree)
	err = filepath.WalkDir(r.root, func(abs string, dir fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if abs == r.root {
			return nil
		}
		rel, err := filepath.Rel(r.root, abs)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if syncSaveIgnored(r, rel, dir.IsDir()) {
			if dir.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := os.Lstat(abs)
		if err != nil {
			return err
		}
		entry := syncSaveEntry{Mode: syncSaveMode(info.Mode()), mtimeMs: info.ModTime().UnixMilli(), identity: localFileIdentity(info)}
		switch {
		case info.IsDir():
			entry.Type = "dir"
		case info.Mode()&os.ModeSymlink != 0:
			entry.Type = "symlink"
			entry.Target, err = os.Readlink(abs)
		case info.Mode().IsRegular():
			entry.Type, entry.Size = "file", info.Size()
			entry.Hash, err = hashSyncSaveFile(ctx, abs, info, r.maxFileBytes)
		default:
			return fmt.Errorf("unsupported local entry %s (%s)", rel, info.Mode().Type())
		}
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		tree[rel] = entry
		return nil
	})
	return tree, err
}

func syncSaveMode(mode fs.FileMode) uint32 {
	bits := uint32(mode.Perm())
	if mode&fs.ModeSetuid != 0 {
		bits |= 0o4000
	}
	if mode&fs.ModeSetgid != 0 {
		bits |= 0o2000
	}
	if mode&fs.ModeSticky != 0 {
		bits |= 0o1000
	}
	return bits
}

// Hash through a bounded buffer, checking cancellation between reads. Validate
// the opened file and the path afterward so replacements cannot be certified.
func hashSyncSaveFile(ctx context.Context, abs string, before fs.FileInfo, limit int64) (string, error) {
	if before.Size() < 0 || before.Size() > limit {
		return "", fmt.Errorf("%d bytes exceeds %d byte cap", before.Size(), limit)
	}
	file, err := os.OpenFile(abs, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !sameSyncSaveFileInfo(before, opened) {
		return "", fmt.Errorf("file changed during save")
	}
	hash := sha256.New()
	buffer := make([]byte, 64*1024)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := file.Read(buffer)
		size += int64(n)
		if size > limit {
			return "", fmt.Errorf("file grew beyond %d byte cap", limit)
		}
		_, _ = hash.Write(buffer[:n])
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	after, err := os.Lstat(abs)
	if err != nil {
		return "", err
	}
	if size != before.Size() || !sameSyncSaveFileInfo(before, after) {
		return "", fmt.Errorf("file changed during save")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sameSyncSaveFileInfo(a, b fs.FileInfo) bool {
	return b.Mode().IsRegular() && os.SameFile(a, b) && a.Size() == b.Size() && a.Mode() == b.Mode() && a.ModTime().Equal(b.ModTime())
}

func readSyncSaveFile(ctx context.Context, abs string, expected syncSaveEntry, limit int64) ([]byte, error) {
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != expected.Size || syncSaveMode(info.Mode()) != expected.Mode || localFileIdentity(info) != expected.identity {
		return nil, fmt.Errorf("local file changed during save")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(abs, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !sameSyncSaveFileInfo(info, opened) {
		return nil, fmt.Errorf("local file changed during save")
	}
	data, err := io.ReadAll(io.LimitReader(syncSaveContextReader{ctx: ctx, reader: file}, limit+1))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(data)) != expected.Size || sha256Hex(data) != expected.Hash {
		return nil, fmt.Errorf("local file changed during save")
	}
	return data, nil
}

type syncSaveContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r syncSaveContextReader) Read(buf []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buf[:min(len(buf), 64*1024)])
}

func scanSyncSaveRemote(ctx context.Context, r *reconciler, baseline *SyncState) (syncSaveTree, error) {
	r.fs.InvalidateCache()
	root, err := r.fs.Stat(ctx, "/")
	if err != nil {
		return nil, err
	}
	if root == nil || root.Type != "dir" {
		return nil, fmt.Errorf("Redis root is not a directory")
	}
	tree := make(syncSaveTree)
	var walk func(string) error
	walk = func(dir string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := r.fs.LsLong(ctx, dir)
		if err != nil {
			return err
		}
		for _, item := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if item.Name == "" || item.Name == "." || item.Name == ".." || strings.Contains(item.Name, "/") {
				return fmt.Errorf("invalid Redis entry name %q", item.Name)
			}
			rel := strings.TrimPrefix(path.Join(dir, item.Name), "/")
			if syncSaveIgnored(r, rel, item.Type == "dir") {
				continue
			}
			if _, exists := tree[rel]; exists {
				return fmt.Errorf("duplicate Redis entry %s", rel)
			}
			entry := syncSaveEntry{Type: item.Type, Mode: item.Mode, mtimeMs: item.Mtime}
			switch item.Type {
			case "dir":
				entry.stat, err = r.fs.Stat(ctx, absoluteRemotePath(rel))
				if err != nil {
					return err
				}
				tree[rel] = entry
				if err := walk(absoluteRemotePath(rel)); err != nil {
					return err
				}
			case "symlink":
				entry.stat, err = r.fs.Stat(ctx, absoluteRemotePath(rel))
				if err != nil {
					return err
				}
				entry.Target, err = r.fs.Readlink(ctx, absoluteRemotePath(rel))
			case "file":
				if item.Size < 0 || item.Size > r.maxFileBytes {
					return fmt.Errorf("Redis file %s exceeds %d byte cap", rel, r.maxFileBytes)
				}
				var data []byte
				var snapshot client.FileSnapshot
				snapshot, err = client.ReadFileSnapshot(ctx, r.fs, absoluteRemotePath(rel))
				if err == nil && snapshot.Stat == nil {
					return fmt.Errorf("Redis file %s vanished during scan", rel)
				}
				data = snapshot.Content
				entry.stat = snapshot.Stat
				if snapshot.Stat != nil {
					entry.Mode = snapshot.Stat.Mode
					entry.mtimeMs = snapshot.Stat.Mtime
				}
				if err == nil {
					if int64(len(data)) != item.Size {
						return fmt.Errorf("Redis file %s changed during scan", rel)
					}
					entry.Size, entry.Hash = int64(len(data)), sha256Hex(data)
					entry.chunkSize, entry.chunkHashes = syncSaveChunks(data, r)
					var storedSize int
					var storedHashes []string
					storedSize, storedHashes, err = r.fs.ChunkMeta(ctx, absoluteRemotePath(rel))
					entry.hasChunks = storedSize != 0 || len(storedHashes) != 0
					entry.chunksCurrent = storedSize == entry.chunkSize && slices.Equal(storedHashes, entry.chunkHashes)
					if stored := baseline.Entries[rel]; stored.ChunkSize > 0 {
						var hashes []string
						for start := 0; start < len(data); start += stored.ChunkSize {
							end := min(start+stored.ChunkSize, len(data))
							hashes = append(hashes, sha256Hex(data[start:end]))
						}
						entry.baselineHash = compositeHash(hashes)
					}
				}
			default:
				return fmt.Errorf("unsupported Redis entry %s (%s)", rel, item.Type)
			}
			if err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
			tree[rel] = entry
		}
		return nil
	}
	err = walk("/")
	return tree, err
}

func syncSaveChunkThreshold(r *reconciler) int {
	if r.chunkThreshold > 0 {
		return r.chunkThreshold
	}
	return defaultChunkThreshold
}

func syncSaveChunks(data []byte, r *reconciler) (int, []string) {
	if len(data) <= syncSaveChunkThreshold(r) {
		return 0, nil
	}
	chunkSize := r.chunkSize
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	var hashes []string
	for start := 0; start < len(data); start += chunkSize {
		hashes = append(hashes, sha256Hex(data[start:min(start+chunkSize, len(data))]))
	}
	return chunkSize, hashes
}

// Reject all inbound changes before any writes. Comparing actual remote bytes
// with both intended bytes and the baseline also permits retry after a previous
// save applied content but failed before completion. Chunk manifests alone are
// insufficient because whole-file writes can leave older chunk metadata behind.
func planSyncSave(local, remote syncSaveTree, baseline *SyncState) (remove, write []string, err error) {
	for rel, observed := range remote {
		intended, exists := local[rel]
		stored, tracked := baseline.Entries[rel]
		if exists && equalSyncSaveEntry(intended, observed) {
			continue
		}
		unchanged := tracked && syncSaveMatchesBaseline(observed, stored)
		if !exists {
			if !unchanged {
				return nil, nil, fmt.Errorf("save conflict at %s: remote-only entry is untracked or changed", rel)
			}
			remove = append(remove, rel)
			continue
		}
		if !unchanged {
			// Content and mode can each already equal the intended value after
			// a partial apply, but any third value requires conflict handling.
			contentOK := sameSyncSaveContent(intended, observed) || (tracked && syncSaveBaselineContent(observed, stored))
			modeOK := observed.Mode == intended.Mode || (tracked && observed.Type == stored.Type && observed.Mode == syncSaveBaselineMode(stored))
			if !contentOK || !modeOK {
				return nil, nil, fmt.Errorf("save conflict at %s: Redis differs from the stored baseline", rel)
			}
		}
		if intended.Type != observed.Type || intended.Type == "symlink" {
			remove = append(remove, rel)
		}
		write = append(write, rel)
	}
	for rel := range local {
		if _, exists := remote[rel]; !exists {
			write = append(write, rel)
		}
	}
	// Delete children before parents; create parents before their children.
	sort.Slice(remove, func(i, j int) bool { return syncSavePathBefore(remove[j], remove[i]) })
	sort.Slice(write, func(i, j int) bool { return syncSavePathBefore(write[i], write[j]) })
	return remove, write, nil
}

func syncSavePathBefore(a, b string) bool {
	if x, y := strings.Count(a, "/"), strings.Count(b, "/"); x != y {
		return x < y
	}
	return a < b
}

func syncSaveBaselineMode(stored SyncEntry) uint32 {
	if stored.Type == "symlink" && stored.Mode == 0 {
		return 0o777
	}
	return stored.Mode
}

func syncSaveBaselineContent(entry syncSaveEntry, stored SyncEntry) bool {
	if entry.Type != stored.Type {
		return false
	}
	switch entry.Type {
	case "dir":
		return true
	case "symlink":
		return entry.Target == stored.Target
	case "file":
		return stored.RemoteHash != "" && entry.Size == stored.Size && (entry.Hash == stored.RemoteHash || entry.baselineHash == stored.RemoteHash)
	}
	return false
}

func syncSaveMatchesBaseline(entry syncSaveEntry, stored SyncEntry) bool {
	return entry.Mode == syncSaveBaselineMode(stored) && syncSaveBaselineContent(entry, stored)
}

func sameSyncSaveContent(a, b syncSaveEntry) bool {
	return a.Type == b.Type && a.Size == b.Size && a.Hash == b.Hash && a.Target == b.Target
}

func equalSyncSaveEntry(a, b syncSaveEntry) bool {
	return a.Mode == b.Mode && sameSyncSaveContent(a, b)
}

func compareSyncSaveTrees(expected, actual syncSaveTree) error {
	for rel, want := range expected {
		got, exists := actual[rel]
		if !exists {
			return fmt.Errorf("missing %s", rel)
		}
		if !equalSyncSaveEntry(want, got) {
			return fmt.Errorf("changed %s", rel)
		}
	}
	for rel := range actual {
		if _, exists := expected[rel]; !exists {
			return fmt.Errorf("extra %s", rel)
		}
	}
	return nil
}
