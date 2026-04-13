# AFS Storage & Sync Roadmap

Consolidated from `chunked-delta-sync.md`, `perf-nfs-followups.md`,
`002_MOUNTED_INCREMENTAL_CHECKPOINTS.md`, and Syncthing comparison analysis.

---

## The One Thing That Unlocks Everything

**Separate content key** — move file bytes from the inode HASH field to a
dedicated Redis STRING `afs:{fs}:content:{id}`.

This is the prerequisite for:
- NFS/FUSE partial I/O via `GETRANGE`/`SETRANGE`/`APPEND` (major perf win)
- Chunk-level delta sync (only dirty blocks transfer)
- Large file support (>64 MB)

**Do this first. Everything else follows.**

---

## Execution Order

### Phase 1: Separate Content Key

See `chunked-delta-sync.md` Phase 1 for full spec.

| Step | What | Files |
|---|---|---|
| 1a | Key builder + read/write path | `keys.go`, `native_core.go`, `native_helpers.go` |
| 1b | Lazy online migration (first-access) | `native_helpers.go` |
| 1c | `GETRANGE`/`SETRANGE` for partial I/O | `native_range.go` |
| 1d | Rm/Cp/Rename handle content keys | `native_core.go`, `native_walk.go` |

### Phase 2: Chunk-Level Delta Sync

See `chunked-delta-sync.md` Phase 2 for full spec.

| Step | What | Files |
|---|---|---|
| 2a | Chunk utilities (stream hash, diff, read) | `sync_chunk.go` (new) |
| 2b | SyncEntry chunk fields + reconciler | `sync_state.go`, `sync_config.go`, `sync_reconciler.go` |
| 2c | Uploader delta upload via pipelined SETRANGE | `sync_uploader.go`, `client.go` |
| 2d | Downloader delta download + local patching | `sync_downloader.go`, `client.go` |

### Phase 3: Raise Size Cap

- `defaultSyncFileSizeCapMB`: 64 → 2048
- Pipeline batching: 16 chunks per batch (~4 MB peak memory)

---

## Syncthing Comparison: What We're Skipping

The Syncthing analysis surfaced features that don't apply to our model:

| Feature | Why we skip it |
|---|---|
| Version vectors / causal ordering | AFS is 1-folder ↔ 1-Redis. Hash-based conflict detection is correct. |
| Per-file versioning (.stversions) | Workspace checkpoints already cover this. |
| P2P transport / device trust | Entirely different architecture. |
| Ignore engine v2 | Current .afsignore works. Low priority polish. |
| Lua EVALSHA writes | Incompatible with separate content key. |

**Bottom line:** the only Syncthing-inspired work worth doing is the block-level
content engine, which is already Phase 2 of `chunked-delta-sync.md`. The rest
is either unnecessary or premature for our architecture.

---

## Already Done

- HSETNX CreateFile (perf-nfs Candidate 1)
- Batched SetAttrs + no-op skip (perf-nfs Candidates 2+7)
- Sync daemon (watcher/uploader/downloader/reconciler)
- Conflict handling + durable stream catch-up + echo suppression

## Deferred (independent, do anytime)

- Incremental checkpoints / dirty journal (`002_MOUNTED_INCREMENTAL_CHECKPOINTS.md`)
- Agent skill (`001_AGENT_SKILL.md`)
- fs_usage regression traces (perf-nfs Candidate 6)
