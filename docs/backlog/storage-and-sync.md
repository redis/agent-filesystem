# Storage And Sync Backlog

Last reviewed: 2026-04-24.
Status: active follow-up notes.

## Landed Baseline

- File content is stored in external Redis string keys:
  `afs:{fs}:content:{inode}` with `content_ref = "ext"` on inode metadata.
- Mount byte-range reads and writes use `GETRANGE` and `SETRANGE`.
- Sync uses chunk-level hashing and delta upload/download for files above 1 MB.
- Sync state version 2 stores chunk size and chunk hashes for chunked files.
- The default sync file-size cap is 2 GB.
- NFS create and attribute hot paths have the HSETNX create claim and batched
  `SetAttrs` optimization.

## Open Work

### Mounted Incremental Checkpoints

Goal: make `afs checkpoint create` in mounted mode scale with changed paths
rather than total workspace size.

Keep:

- full manifests per checkpoint for simple restore and browsing
- explicit checkpoints only
- restore against the live workspace root

Add:

- Redis-backed dirty state for mounted mutations
- changed-path processing during checkpoint create
- metadata that lets checkpoints reuse existing blob/content identities
- tests for file writes, metadata-only changes, deletes, renames, restores, and
  empty/no-op checkpoints

Suggested key shape:

```text
afs:{<workspace>}:workspace:dirty_state
afs:{<workspace>}:workspace:dirty_paths
afs:{<workspace>}:workspace:deleted_paths
afs:{<workspace>}:workspace:ops
```

### Large-File Verification

The storage engine now has chunked sync and external content keys. The next work
is verification and polish:

- run a real large-file sync benchmark with local and remote Redis
- exercise grow, shrink, truncate, overwrite, and sparse-ish writes
- confirm memory stays bounded at chunk-size times pipeline batch size
- decide whether any remaining full-file paths need chunk-aware behavior

### Search Benchmarks

`afs grep` now has an optional RediSearch-backed fast path for simple literal
search. Keep future search work benchmarked in two modes:

- Redis search available and index ready
- Redis search unavailable, using the native client fallback

Do not check generated benchmark directories into the repo; rerun them under
`/tmp` or another artifact directory, then summarize stable results in
`docs/performance.md`.
