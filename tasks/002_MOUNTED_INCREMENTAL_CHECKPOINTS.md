# Task 002: Mounted Incremental Checkpoint Design

## Overview

Design a mounted-mode checkpoint architecture that:

- makes `afs checkpoint create` scale with changed paths instead of full workspace size
- avoids touching the local `~/.afs` worktree outside `afs workspace run`
- preserves immutable savepoints and simple restore semantics
- improves large-file behavior, especially for SQLite-style database files

This design is intentionally optimized for the primary workflow: AFS running in mounted mode against the live Redis-backed workspace root.

## Problem Statement

The current mounted checkpoint path is still too expensive for large workspaces and large mutable files:

1. `checkpoint create` rebuilds a full manifest by scanning the entire live workspace tree.
2. Each file is re-read during checkpoint creation, even when only a few files changed.
3. Large files such as the Claude Code database still pay a full read/hash/save cost at checkpoint time.
4. Local materialization under `~/.afs` should not be part of the mounted checkpoint path. The local worktree should exist only for `afs workspace run`.

## Current Behavior

### Savepoint Format

Each savepoint stores:

- `SavepointMeta`, including `ParentSavepoint`
- a full `Manifest` containing every path in the workspace
- content-addressed whole-file blobs

This is visible in:

- `internal/controlplane/store.go`
- `internal/controlplane/service.go`

### Mounted Checkpoint Flow

The current mounted save path:

1. walks the live workspace tree
2. stats each path
3. cats each file
4. builds a full manifest
5. writes blobs and savepoint metadata

The key code path is:

- `cmd/afs/workspace_mount_bridge.go`

### Important Constraint

The mounted live filesystem currently stores file content inline in inode state. File writes and range writes rewrite whole-file content in Redis. This means that even if checkpoint storage becomes incremental, large mutable files will still be expensive unless the live file representation also changes.

## Design Decision

Do **not** build checkpointing around persisted text diffs or patch chains.

Instead:

1. keep full manifests per savepoint for simple restore and browse
2. add a live dirty journal so checkpoint creation only processes changed paths
3. reuse blob references for unchanged files
4. add a second-phase chunked large-file design for database workloads

This keeps restore simple while removing the most expensive checkpoint-time work.

## Goals

- `checkpoint create` in mounted mode should avoid full-tree rescans when only a subset of files changed
- mounted checkpoints should not materialize or synchronize the local `~/.afs` worktree
- unchanged files should not be re-read at checkpoint time
- metadata-only changes should avoid file content work
- savepoint restore should operate on the live workspace root only
- large mutable files should have a path toward incremental persistence

## Non-Goals

- replacing savepoints with delta-only restore chains
- introducing local checkout state into mounted checkpoint flow
- solving all large binary file performance problems in the first phase
- changing `afs workspace run` semantics

## Proposed Architecture

### 1. Keep Full Manifests Per Savepoint

Each savepoint should continue to store a complete manifest.

Why:

- restore remains direct and simple
- browsing any checkpoint stays fast
- manifest storage is not the main bottleneck
- failure recovery stays easier than with patch-chain reconstruction

`ParentSavepoint` remains lineage metadata, not a replay chain.

### 2. Add Live Dirty-State Tracking

Introduce a Redis-backed dirty journal for the mounted live workspace.

Proposed keys:

- `afs:{<ws>}:workspace:dirty_state`
- `afs:{<ws>}:workspace:dirty_paths`
- `afs:{<ws>}:workspace:deleted_paths`
- `afs:{<ws>}:workspace:ops` (optional debug/audit stream)

Suggested contents:

#### `dirty_state` hash

- `base_savepoint`: savepoint the dirty live state started from
- `dirty_since_ms`: first dirty timestamp
- `last_mutation_ms`: latest mutation timestamp

#### `dirty_paths` hash

Keyed by manifest path, with a small JSON or flat string payload describing:

- `kind`: `upsert`
- `inode_id`
- `content_dirty`
- `meta_dirty`
- `type`

#### `deleted_paths` hash or set

Contains removed manifest paths. Directory deletes may represent subtree tombstones.

#### `ops` stream

Optional. Useful for debugging and observability, but not required for restore.

## Inode Metadata Additions

Extend live inode metadata with checkpoint-relevant fields:

- `content_sha256`
- `last_saved_blob_id`
- `last_saved_savepoint`
- `content_kind`

`content_kind` can initially be:

- `inline`
- `blob`

Later it can support:

- `chunked`

These fields allow checkpoint creation to reuse already-known content identity for changed files without rescanning the entire workspace.

## Mounted Write Path Changes

Every mounted mutation should mark dirty state as part of the same logical mutation.

Touch points include:

- `mount/internal/client/native_core.go`
- `mount/internal/client/native_range.go`
- `mount/internal/client/native_text.go`
- `mount/internal/client/native_helpers.go`

### Required Mutation Semantics

#### File content writes

For:

- `Echo`
- `EchoCreate`
- `EchoAppend`
- `WriteInodeAt`
- `TruncateInode`
- `Insert`
- `Replace`
- `DeleteLines`

Do:

- mark path dirty
- set `content_dirty = true`
- update `content_sha256`
- eagerly persist large-file blobs when profitable

#### Metadata-only changes

For:

- `Touch`
- `Chmod`
- `Chown`
- `Utimens`

Do:

- mark path dirty
- set `meta_dirty = true`
- avoid blob work

#### Create operations

For:

- `CreateFile`
- `Mkdir`
- `Ln`

Do:

- mark new path dirty

#### Delete operations

For:

- `Rm`

Do:

- mark path deleted
- remove any matching dirty-path entry

#### Rename and Move

For:

- `Rename`
- `Mv`

Do:

- mark old path deleted
- mark new path dirty

For directory renames in phase 1, it is acceptable to enumerate the renamed subtree and mark descendant paths dirty. That is more expensive than ideal, but still far cheaper than rescanning the whole workspace on every checkpoint.

## Checkpoint Create Flow

### Phase 1 Flow

1. Read workspace metadata and expected head savepoint.
2. Read `dirty_state`.
3. If there is no dirty state, checkpoint is a no-op.
4. If `dirty_state.base_savepoint != expected head`, fall back to full manifest rebuild or fail with a conflict.
5. Load the head manifest once.
6. Clone the head manifest in memory.
7. Apply `deleted_paths` to remove entries.
8. Apply `dirty_paths` to refresh only changed entries from the live workspace.
9. For dirty files:
   - reuse `last_saved_blob_id` when content hash matches
   - otherwise reference a blob already sealed during mutation
   - inline small files directly in the manifest
10. Persist:
   - new savepoint meta
   - full new manifest
   - any new blob refs
   - updated workspace head
11. Clear dirty journal state atomically.

### Fallback Behavior

If dirty journal state is missing, corrupt, or inconsistent:

- fall back to the current full manifest rebuild path
- emit an audit event
- repair dirty state after the checkpoint completes

This keeps rollout safe.

## Blob Strategy

### Short-Term

Keep current whole-file content-addressed blobs for savepoints.

Improve behavior by shifting blob creation from checkpoint time to mutation time for large files:

- when a mounted write produces a large file body, compute `content_sha256`
- `SETNX` the blob immediately
- store `last_saved_blob_id` or `content_sha256` on the inode

Then checkpoint creation can reference the blob directly without re-reading the file body from Redis.

### Why This Helps

Today a large file can be fully read during checkpoint creation just to produce the same blob ID that could have been known at write time.

Eager blob sealing turns checkpoint work into metadata assembly for most changed files.

## Large Mutable Files

### Problem

Large DB-like files are still expensive even with dirty-path journaling because the live mount representation currently rewrites full-file content for range updates.

### Phase 2 Design: Chunked File Storage

Introduce a chunked representation for large mutable files.

Suggested model:

- small files continue using inline inode `content`
- large files switch to chunked storage above a threshold
- inode metadata stores:
  - file size
  - chunk size
  - ordered chunk references
  - chunk root hash or manifest

Range writes then:

- rewrite only affected chunks
- reuse unchanged chunks
- update file metadata only for touched ranges

Checkpoint creation then:

- references the file's chunk-root identity
- does not need to rebuild a monolithic blob for the entire file

This is the real long-term answer for SQLite-like checkpoint performance.

## Restore Semantics

Restore should remain canonical and mounted-only:

1. move workspace head to selected savepoint
2. materialize the selected manifest into the live workspace root
3. clear dirty journal state

Restore should **not** materialize the local `~/.afs` worktree unless the user explicitly runs `afs workspace run`.

## `workspace run` Interaction

`afs workspace run` remains the only command that should:

- materialize a local working directory under `~/.afs`
- run a real process in that directory
- sync changes back into the live workspace root on exit

Checkpoint creation and restore in mounted mode should operate entirely on canonical Redis-backed state.

## Control Plane Changes

### SaveCheckpoint

`SaveCheckpoint` should accept a prebuilt manifest assembled from dirty-state-aware mounted logic.

This is already compatible with the current separation between:

- manifest assembly
- savepoint persistence

### Dirty Status

`WorkspaceMeta.DirtyHint` should become advisory.

Preferred source of truth:

- dirty journal presence for mounted live workspaces

This avoids expensive diffing against the head manifest just to answer "is this workspace dirty?"

## Redis Key Summary

### Existing savepoint storage

- `afs:{ws}:savepoint:<sp>:meta`
- `afs:{ws}:savepoint:<sp>:manifest`
- `afs:{ws}:blob:<blobID>`
- `afs:{ws}:blobref:<blobID>`

### New mounted dirty-state storage

- `afs:{ws}:workspace:dirty_state`
- `afs:{ws}:workspace:dirty_paths`
- `afs:{ws}:workspace:deleted_paths`
- `afs:{ws}:workspace:ops`

## Rollout Plan

### Phase 1: Incremental Mounted Checkpoints

- add dirty-state keys
- mark dirty paths during mounted mutations
- build new manifests by patching the head manifest
- keep full savepoint manifests
- remove all non-`workspace run` local worktree sync assumptions

Expected result:

- checkpoint cost scales roughly with changed path count
- no `~/.afs` writes during mounted checkpoint operations

### Phase 1.5: Eager Blob Sealing

- compute file content hash during mounted writes
- seal large blobs at write time
- checkpoint creation references existing blobs

Expected result:

- large changed files avoid redundant checkpoint-time blob generation

### Phase 2: Chunked Large Files

- add chunked live file representation for large mutable files
- update read/write paths for chunk-aware operation
- teach savepoints to reference chunk-root identities

Expected result:

- range writes no longer rewrite entire large files
- database-style workloads become practical

## Risks

### Dirty Journal Drift

If mutation code updates live inode state but fails to update dirty journal state, checkpoints could miss changes.

Mitigation:

- queue dirty marks in the same Redis pipeline/transaction as inode mutation
- keep a safe fallback to full manifest rebuild

### Rename Complexity

Directory renames are harder than simple file upserts.

Mitigation:

- phase 1 may enumerate subtree descendants during rename
- optimize later if rename-heavy workloads justify it

### Partial Rollout

If some mutation paths do not mark dirty state, behavior becomes inconsistent.

Mitigation:

- audit every mounted mutation entry point
- add focused tests for each mutation family

## Testing Plan

Add tests for:

- file create/update/delete without full-tree checkpoint scan
- metadata-only changes
- directory create/delete/rename
- symlink create/rename/delete
- checkpoint create with no local `~/.afs` tree present
- checkpoint restore with no local rematerialization
- fallback to full rebuild when dirty state is missing
- large file eager blob sealing
- chunked large-file range writes in phase 2

## Success Criteria

- mounted `checkpoint create` does not scan or read unchanged files
- mounted checkpoint and restore do not touch local `~/.afs` state
- unchanged file blobs are reused across savepoints
- dirty status can be answered without manifest rescans
- large DB-like files no longer require full checkpoint-time rereads once eager sealing is in place
- chunked large-file support removes whole-file rewrite behavior for range writes in phase 2

## Recommendation

Implement phase 1 first.

That is the smallest change that meaningfully improves mounted checkpoint performance while preserving current savepoint semantics.

After phase 1 lands, phase 1.5 should address the worst large-file checkpoint cost by sealing blobs at mutation time. If large mutable database files remain a major workload, phase 2 should introduce chunked live-file storage.
