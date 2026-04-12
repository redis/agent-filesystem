# Chunked Delta Sync for Large Files

## Context

The sync system currently transfers entire files on every change. A 1-byte edit
to a 100 MB file causes the full 100 MB to be read into memory, hashed, and
uploaded to Redis — then fully downloaded on every other machine. Files above
64 MB are rejected entirely. This design adds chunk-level storage, hashing, and
delta transfer so only changed portions move across the wire, and large files
are supported without loading them fully into memory.

This builds on top of **Candidate 4** from `tasks/perf-nfs-followups.md`
(separate content key), which is a prerequisite.

---

## Phase 1: Separate Content to Its Own Redis String Key

**Goal:** Move file content from the inode HASH field to a dedicated Redis
STRING key. This unlocks `GETRANGE`/`SETRANGE`/`APPEND` for partial operations.

### Storage model change

```
Before:  afs:{fs}:inode:{id}  HASH  { type, mode, ..., content: "<bytes>" }
After:   afs:{fs}:inode:{id}  HASH  { type, mode, ..., content_ref: "ext" }
         afs:{fs}:content:{id} STRING  <raw bytes>
```

- New inode field `content_ref` = `"ext"` signals content lives in the separate key
- Absent `content_ref` (or `""`) = legacy inline mode (backward compatible)
- `size` field on the inode remains authoritative for file length

### Files to modify

**`mount/internal/client/keys.go`** — add:
```go
func (k keyBuilder) content(id string) string {
    return "afs:{" + k.fsKey + "}:content:" + id
}
```

**`mount/internal/client/native_core.go`** — modify:
- `writeFile()`: write content to `content:{id}` via `SET`, set `content_ref: "ext"` on inode HASH, remove `content` field from HASH
- `loadContentByID()`: check `content_ref`; if `"ext"`, use `GET content:{id}` instead of `HGET inode:{id} content`
- `Echo()` / `Cat()`: route through updated write/read paths
- `createFileIfMissing()`: set `content_ref: "ext"` on new inodes, write empty content key
- `Rm()`: also `DEL content:{id}` when deleting a file
- `Cp()`: copy `content:{id}` to new content key
- Publish invalidation as before (no change to invalidation model)

**`mount/internal/client/native_range.go`** — modify:
- `ReadInodeAt()`: use `GETRANGE content:{id} off end` — only transfers requested bytes over the wire (major win for large files)
- `WriteInodeAtPath()`: use `SETRANGE content:{id} off payload` for in-place writes when the file isn't growing. For growth, `APPEND` or extend first, then `SETRANGE`. Update `size`/`mtime` on inode HASH separately
- `TruncateInodeAtPath()`: `SET content:{id}` with truncated content (or `SETRANGE` + `DEL` tail via Lua for very large files)
- `loadInodeWithContentByID()`: fetch metadata via `HMGET inode:{id}`, then content via `GET content:{id}` (or pipeline both)

**`mount/internal/client/native_helpers.go`** — modify:
- `inodeFields()`: stop including `content` in the HASH fields
- Add migration helper: on first read, if `content_ref` is absent but `content` HASH field exists, migrate inline → external

**Text operations** (`Head`, `Tail`, `Lines`, `Insert`, `Replace`, `DeleteLines`, `Wc`, `Grep`):
- These all go through `loadContentByID()` which will be updated — no individual changes needed unless we want to optimize with `GETRANGE` (defer to later)

### Online migration strategy

When `loadContentByID()` is called for an inode without `content_ref`:
1. Read content from HASH field (`HGET inode:{id} content`)
2. Write to string key (`SET content:{id} <data>`)
3. Update inode: `HSET inode:{id} content_ref ext`, `HDEL inode:{id} content`
4. Return content

This migrates lazily on first access. No offline migration step needed.

---

## Phase 2: Chunk-Level Hashing and Delta Sync

**Goal:** The sync daemon detects changes at chunk granularity and only
transfers modified chunks between local disk and Redis.

### Constants and configuration

```go
const (
    defaultChunkSize      = 256 * 1024  // 256 KB per chunk
    chunkThreshold        = 1 * 1024 * 1024  // 1 MB — files below this use full-file sync (current behavior)
    defaultMaxFileSizeMB  = 2048  // 2 GB new cap (up from 64 MB)
)
```

New config fields in `sync_config.go`:
- `SyncChunkSize` (default 256 KB)
- `SyncChunkThreshold` (default 1 MB)

### SyncEntry changes (`sync_state.go`)

```go
type SyncEntry struct {
    // ... existing fields unchanged ...
    ChunkSize   int      `json:"chunk_size,omitempty"`    // 0 = inline (not chunked)
    ChunkHashes []string `json:"chunk_hashes,omitempty"`  // per-chunk SHA256 hex
}
```

State version bump: `syncStateVersion = 2` with backward-compat (v1 entries
have zero-value ChunkSize, treated as inline).

### New file: `sync_chunk.go`

Chunk utility functions:

```go
// streamChunkHashes reads a file chunk-by-chunk without loading it all
// into memory. Returns per-chunk SHA256 hashes and file size.
// Memory usage: O(chunkSize), not O(fileSize).
func streamChunkHashes(path string, chunkSize int) (hashes []string, fileSize int64, err error)

// diffChunkManifests compares old and new chunk hash lists.
// Returns indices of chunks that differ (changed, added, or removed).
func diffChunkManifests(oldHashes, newHashes []string) (changed []int, truncated bool)

// readChunkFromDisk reads exactly one chunk from a file at the given index.
func readChunkFromDisk(path string, chunkIndex int, chunkSize int) ([]byte, error)

// wholeFileHash computes SHA256 of all chunk hashes concatenated.
// Used as the top-level LocalHash/RemoteHash for backward compat.
func compositeHash(chunkHashes []string) string
```

### Upload path changes

**`sync_reconciler.go`** — `handleLocalFile()`:

```
Current:
  data, _ := os.ReadFile(abs)       // full file in memory
  hash := sha256Hex(data)           // full-file hash
  → queue uploadOp{Content: data}

Proposed (file > chunkThreshold):
  hashes, size, _ := streamChunkHashes(abs, chunkSize)  // O(chunkSize) memory
  compositeH := compositeHash(hashes)
  if compositeH == stored.LocalHash { return }  // unchanged
  dirty := diffChunkManifests(stored.ChunkHashes, hashes)
  → queue uploadOp{Chunked: true, ChunkHashes: hashes, DirtyChunks: dirty, FileSize: size}

Proposed (file <= chunkThreshold):
  // Unchanged — full-file path as today
```

**`sync_uploader.go`** — `processFile()`:

```
Current:
  fs.Echo(ctx, path, op.Content)     // full file upload

Proposed (op.Chunked):
  // 1. Drift check: fetch remote chunk manifest (metadata only, no content)
  remoteMeta := fs.ChunkManifest(ctx, path)
  if remoteMeta differs from stored → conflict

  // 2. Upload only dirty chunks via pipelined SETRANGE
  pipe := rdb.Pipeline()
  for _, idx := range op.DirtyChunks {
      data := readChunkFromDisk(op.AbsPath, idx, op.ChunkSize)
      offset := int64(idx) * int64(op.ChunkSize)
      pipe.SetRange(ctx, contentKey, offset, string(data))
  }

  // 3. Handle truncation if file shrunk
  if newSize < oldSize {
      // Truncate via Lua or GET+SET of tail
  }

  // 4. Update inode metadata (size, mtime, chunk manifest)
  pipe.HSet(ctx, inodeKey, "size", op.FileSize, "mtime_ms", now,
            "chunk_hashes", marshalHashes(op.ChunkHashes))
  pipe.Exec(ctx)

  // 5. Publish invalidation (unchanged — still file-level)
```

**uploadOp struct changes:**
```go
type uploadOp struct {
    Kind        uploadOpKind
    Path        string
    AbsPath     string
    Content     []byte    // only for inline (non-chunked) files
    Mode        uint32
    Symlink     string
    LocalHash   string    // composite hash for chunked, sha256 for inline
    StoredEntry SyncEntry
    HasStored   bool
    // Chunked upload fields:
    Chunked     bool
    FileSize    int64
    ChunkSize   int
    ChunkHashes []string  // complete new manifest
    DirtyChunks []int     // indices of changed chunks (data read from AbsPath on demand)
}
```

### Download path changes

**`sync_reconciler.go`** — `handleRemoteEvent()`:

```
Current:
  → queue downloadOp (downloader fetches full file via Cat)

Proposed (chunked file):
  // Fetch remote chunk manifest (metadata only — cheap)
  remoteHashes := fs.ChunkManifest(ctx, path)
  dirty := diffChunkManifests(stored.ChunkHashes, remoteHashes)
  → queue downloadOp{Chunked: true, ChunkHashes: remoteHashes, DirtyChunks: dirty}
```

**`sync_downloader.go`** — `processFile()`:

```
Current:
  data := fs.Cat(ctx, path)          // full file download
  atomicWriteFile(absPath, data)     // full rewrite

Proposed (op.Chunked):
  // 1. Download only dirty chunks via pipelined GETRANGE
  pipe := rdb.Pipeline()
  cmds := make([]*redis.StringCmd, len(op.DirtyChunks))
  for i, idx := range op.DirtyChunks {
      offset := int64(idx) * int64(op.ChunkSize)
      end := offset + int64(op.ChunkSize) - 1
      cmds[i] = pipe.GetRange(ctx, contentKey, offset, end)
  }
  pipe.Exec(ctx)

  // 2. Patch local file at chunk offsets (in-place, no full rewrite)
  f, _ := os.OpenFile(absPath, os.O_RDWR|os.O_CREATE, mode)
  for i, idx := range op.DirtyChunks {
      offset := int64(idx) * int64(op.ChunkSize)
      f.WriteAt([]byte(cmds[i].Val()), offset)
  }

  // 3. Handle truncation if file shrunk
  if newSize < currentSize {
      f.Truncate(newSize)
  }
  f.Sync(); f.Close()

  // 4. Update echo suppressor and SyncEntry
```

**downloadOp struct changes:**
```go
type downloadOp struct {
    Kind        downloadOpKind
    Path        string
    AbsPath     string
    Mode        uint32
    Symlink     string
    StoredEntry SyncEntry
    HasStored   bool
    Conflict    bool
    // Chunked download fields:
    Chunked     bool
    FileSize    int64
    ChunkSize   int
    ChunkHashes []string  // remote's complete manifest
    DirtyChunks []int     // indices to fetch
}
```

### New client methods

Add to `Client` interface (`mount/internal/client/client.go`):

```go
// ChunkManifest returns the stored chunk hashes for a file without
// fetching content. Returns nil for inline (non-chunked) files.
ChunkManifest(ctx context.Context, path string) ([]string, int, error)

// WriteChunks writes specific chunks to a file's content key via
// pipelined SETRANGE. Updates size and mtime atomically.
WriteChunks(ctx context.Context, path string, chunks map[int][]byte,
    chunkSize int, newSize int64, hashes []string) error

// ReadChunks reads specific chunks from a file's content key via
// pipelined GETRANGE. Returns chunk data by index.
ReadChunks(ctx context.Context, path string, indices []int,
    chunkSize int) (map[int][]byte, error)
```

### Chunk manifest storage in Redis

Store chunk hashes as a JSON array in the inode HASH under field `chunk_hashes`.
For a 1 GB file with 256 KB chunks (4096 chunks), this is ~260 KB of metadata — acceptable for a HASH field.

```
afs:{fs}:inode:{id}  HASH {
    ...,
    content_ref: "ext",
    chunk_size: "262144",
    chunk_hashes: '["a1b2c3...","d4e5f6...",...]'
}
```

### Conflict detection for chunked files

The composite hash (hash of all chunk hashes) serves as LocalHash/RemoteHash
in SyncEntry, maintaining compatibility with the existing conflict detection:
- Both sides unchanged: compositeHash matches stored → no-op
- One side changed: compositeHash differs → upload or download
- Both sides changed: both differ from stored → conflict (preserve local copy)

---

## Phase 3: Raise Size Cap and Polish

- Change `defaultSyncFileSizeCapMB` from 64 to 2048 (2 GB)
- Add `SyncMaxFileSizeMB` config field
- Ensure streaming hash computation handles very large files (>1 GB) gracefully
- Memory budget: peak usage should be `O(chunkSize × pipelineBatchSize)` — at 256 KB chunks with 16-chunk batches, that's 4 MB peak per sync operation
- For pipeline batch size: upload/download dirty chunks in batches of 16 to avoid excessive pipeline size for files with many changed chunks

---

## What This Does NOT Change

- **Invalidation model**: Still file-level (`InvalidateOpContent`). Chunk-level invalidation is unnecessary — manifest comparison is cheap.
- **Small files (<1 MB)**: Continue using the current full-file inline sync path. No overhead for the common case.
- **FUSE mount layer**: Gets GETRANGE/SETRANGE from Phase 1 for free. No chunk awareness needed in FUSE — it already operates at byte offsets.
- **Cold-start sync**: Bulk materialize still works — new files download all chunks (no manifest to diff against), but in batches instead of one giant transfer.

---

## Performance Characteristics

| Scenario | Before | After |
|---|---|---|
| 1 GB file, 1-byte change, upload | 1 GB read + hash + transfer | 256 KB read + hash + transfer |
| 1 GB file, 1-byte change, download | 1 GB transfer + write | 256 KB transfer + 256 KB write |
| 1 GB file, full rewrite | Rejected (>64 MB cap) | 1 GB streamed in 256 KB chunks |
| 100 KB file, any change | ~100 KB (unchanged) | ~100 KB (unchanged, below threshold) |
| Memory usage for 1 GB sync | >2 GB (file + remote copy) | ~4 MB (chunk buffer) |

---

## Implementation Order

1. **Phase 1a**: Add `content(id)` key builder, update `loadContentByID` and `writeFile` to use external content key for new files
2. **Phase 1b**: Online migration path for existing files (lazy on first access)
3. **Phase 1c**: Update `ReadInodeAt` → `GETRANGE`, `WriteInodeAtPath` → `SETRANGE`
4. **Phase 1d**: Update `Rm`, `Cp`, `Rename` to handle content keys
5. **Phase 2a**: `sync_chunk.go` — streaming hash, manifest diff utilities
6. **Phase 2b**: SyncEntry chunk fields, reconciler chunk-aware change detection
7. **Phase 2c**: Uploader delta upload via `WriteChunks`
8. **Phase 2d**: Downloader delta download via `ReadChunks` + local patching
9. **Phase 3**: Raise size cap, add batched pipeline limits

## Key Files

| File | Changes |
|---|---|
| `mount/internal/client/keys.go` | Add `content(id)` key |
| `mount/internal/client/native_core.go` | External content read/write, migration |
| `mount/internal/client/native_range.go` | GETRANGE/SETRANGE for partial I/O |
| `mount/internal/client/native_helpers.go` | Remove content from inode fields |
| `mount/internal/client/client.go` | New ChunkManifest/WriteChunks/ReadChunks methods |
| `cmd/afs/sync_chunk.go` | **New** — chunk utilities |
| `cmd/afs/sync_state.go` | ChunkSize/ChunkHashes on SyncEntry |
| `cmd/afs/sync_config.go` | New chunk config fields |
| `cmd/afs/sync_reconciler.go` | Chunk-aware change detection |
| `cmd/afs/sync_uploader.go` | Delta upload |
| `cmd/afs/sync_downloader.go` | Delta download + local patching |
| `cmd/afs/sync_daemon.go` | Wire up chunk config |

## Verification

1. **Unit tests**: `sync_chunk_test.go` — test `streamChunkHashes`, `diffChunkManifests`, round-trip chunk read/write
2. **Integration tests**: Extend `sync_integration_test.go` with:
   - Large file upload (above chunk threshold) → verify only dirty chunks transferred
   - Large file download → verify local file patched correctly
   - File growth/shrink across chunk boundaries
   - Conflict detection with chunked files
   - Migration: inline file → chunked after growth
3. **FUSE tests**: Verify `ReadInodeAt` with GETRANGE returns correct data at arbitrary offsets
4. **Manual test**: Create a >100 MB file, sync, modify 1 byte, verify only one chunk transfers (observe Redis traffic with `MONITOR` or `AFS_NFS_OPSTATS`)

## Future enhancements (not in scope)

- **Content-defined chunking (CDC)**: Fixed-size chunks shift all hashes on insertions at the beginning. CDC (rolling hash like FastCDC) handles insertions gracefully but adds complexity. Worth considering if users frequently insert data at the start of large files.
- **Compression**: Compress chunks before storage/transfer. Trivial to add since each chunk is an independent unit.
- **Parallel chunk transfers**: Upload/download chunks concurrently with a worker pool. The pipelined GETRANGE/SETRANGE already batch well, but for very large files with many dirty chunks, parallel goroutines could help.
- **Chunk-level invalidation**: Broadcast which chunks changed so peers skip the manifest comparison. Overkill for now since manifest comparison is cheap.
