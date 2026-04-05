# RAF Hybrid Architecture

Date: 2026-04-02

## Thesis

The cleanest RAF design is:

- Redis is the canonical store for workspaces, sessions, savepoints, manifests, and audit.
- Native local directories are the execution surface for shells, editors, compilers, and tests.
- `raf run` runs real processes against real directories.
- `raf save`, `raf fork`, and `raf rollback` operate against Redis-backed canonical state.

This gives RAF:

- native OS ergonomics,
- a simple CLI,
- one consistent branching and rollback model,
- remote durability when Redis is remote and managed,
- an optional path to shared/live collaboration later.

## Why This Is Better Than "Filesystem Fully In Redis"

Putting all live file reads and writes on Redis can work, but it pushes RAF into reimplementing too much filesystem behavior:

- rename edge cases,
- range I/O,
- large file chunking,
- inode identity,
- cache invalidation,
- FUSE correctness,
- performance tuning for build/test loops.

By contrast, this hybrid model lets the operating system do what it is good at:

- local file I/O,
- process execution,
- shell compatibility,
- editor compatibility,
- build tool compatibility.

And it lets Redis do what Redis is good at:

- durable centralized metadata,
- manifests and blobs,
- savepoints and branching,
- audit streams,
- sharing and synchronization,
- TTLs, GC, and coordination.

## Product Model

Users only need to learn:

- workspace
- session
- savepoint

Under the hood, RAF maintains:

- local materializations on disk
- canonical state in Redis

The user does not have to care which one is being touched at any given moment.

## Core Design

### Canonical state lives in Redis

Redis stores:

- workspace metadata
- session metadata
- savepoint metadata
- file manifests
- file blobs or chunks
- audit streams
- optional locks and sync state

### Active execution happens in a local directory

When a developer or agent works on a session, RAF creates or reuses a local materialization:

- a real directory on disk
- normal files
- normal subdirectories
- usable by `bash`, `python3`, `git`, `pytest`, editors, and build tools

### Savepoints are immutable

`raf save` records the exact effective filesystem state into Redis as an immutable savepoint.

### Rollback is rematerialization

`raf rollback` does not replay inverse operations.
It throws away or archives the current local materialization and recreates it from the selected savepoint.

This is the most elegant semantic model.

## Mental Model

The simplest way to explain RAF is:

1. A session is your branch.
2. The branch runs locally in a real directory.
3. Redis stores its canonical checkpoints and history.
4. Savepoint restores come from Redis, not from undo logs.

That gives the user a branch-like experience without requiring them to think in Git or overlayfs internals.

## Public CLI Semantics

### `raf import`

```bash
raf import my-repo ./repo
```

Meaning:

- create workspace `my-repo`
- scan `./repo`
- upload canonical file state into Redis
- create default session `main`
- create initial savepoint, usually `imported` or `initial`
- optionally create a local materialization pointer for `main`

### `raf run`

```bash
raf run my-repo --session fix-login -- pytest -q
raf run my-repo --session fix-login -- bash
```

Meaning:

1. resolve the workspace and session
2. ensure a local materialization exists
3. if missing, materialize from the session's current head savepoint
4. set process cwd to that local directory
5. launch the command as a real child process
6. stream stdio
7. record process lifecycle in audit log

`raf run` does not save automatically unless that becomes an explicit mode.

### `raf save`

```bash
raf save my-repo --session fix-login before-refactor
```

Meaning:

1. scan the local materialization
2. compare it to the session's current canonical base
3. upload changed files to Redis
4. create a new immutable savepoint manifest
5. move the session head to that savepoint
6. record an audit event

This is effectively "commit without Git."

### `raf fork`

```bash
raf fork my-repo fix-login try-alt-approach
```

Meaning:

1. resolve the source session head savepoint
2. create a new session pointing to that same savepoint
3. create a fresh local materialization on demand

This is cheap because the fork points at canonical saved state, not at copied live directories.

### `raf rollback`

```bash
raf rollback my-repo --session fix-login before-refactor
```

Meaning:

1. find the target savepoint in Redis
2. move the session head back to that savepoint
3. discard or archive the current local materialization
4. recreate local files from the target savepoint
5. record one rollback event

This is clean, deterministic, and easy to explain.

### `raf diff`

```bash
raf diff my-repo --session fix-login
```

Meaning:

- compare the current local materialization to the session head savepoint
- show unsaved local changes

Optional future mode:

- compare two savepoints
- compare two sessions

### `raf apply`

```bash
raf apply my-repo --session fix-login --to-dir ./repo
```

Meaning:

- copy the current local materialization, or the current head savepoint, back into a target directory
- optionally check for conflicts
- optionally stage with Git

## Two States: Saved and Dirty

This architecture is easiest if RAF explicitly tracks two states per session:

- saved state
- dirty local state

### Saved state

This is the canonical session head in Redis.

It is:

- immutable at the savepoint level
- branchable
- rollbackable
- remotely inspectable

### Dirty local state

This is whatever currently exists in the local materialization but has not yet been saved.

This is:

- local only by default
- crash-recoverable if we keep local metadata
- visible in `raf diff`

This mirrors how developers already think:

- saved checkpoint
- unsaved working tree

## Redis Schema

All keys below use one Redis hashtag root:

```text
raf:{workspace}:...
```

Example:

```text
raf:{my-repo}:workspace:meta
```

## Workspace metadata

```text
raf:{ws}:workspace:meta
raf:{ws}:workspace:audit
raf:{ws}:workspace:sessions
raf:{ws}:workspace:savepoints
```

Suggested `workspace:meta` fields:

- `format_version`
- `created_at`
- `default_session`
- `default_savepoint`
- `session_count`
- `savepoint_count`
- `blob_count`
- `total_bytes`

`workspace:sessions`

- set of session names

`workspace:savepoints`

- set of savepoint ids or names

`workspace:audit`

- Redis Stream for workspace-wide events

## Session metadata

```text
raf:{ws}:session:<sid>:meta
raf:{ws}:session:<sid>:audit
raf:{ws}:session:<sid>:locks
```

Suggested fields:

- `id`
- `name`
- `head_savepoint`
- `created_at`
- `updated_at`
- `owner`
- `status`
- `local_materialization_path`
- `local_materialization_state`
- `dirty`
- `last_saved_at`

Important rule:

- every session points to exactly one canonical saved head
- dirty changes live locally until saved

That keeps the model very simple.

## Savepoint metadata

```text
raf:{ws}:savepoint:<sp>:meta
raf:{ws}:savepoint:<sp>:manifest
```

Suggested `meta` fields:

- `id`
- `name`
- `created_at`
- `workspace`
- `session`
- `parent_savepoint`
- `root_manifest_hash`
- `file_count`
- `dir_count`
- `total_bytes`

`manifest`

- mapping of path -> file entry metadata

Each entry contains:

- file type
- mode
- uid
- gid
- mtime
- size
- blob id or inline small content
- symlink target if needed

## Blob store

```text
raf:{ws}:blob:<blobid>
raf:{ws}:blobref:<blobid>
```

Suggested behavior:

- content-addressed by hash
- deduplicated across savepoints within a workspace
- small files may be stored inline at first
- large files can be chunked later

`blobref` can hold:

- reference count
- size
- created_at

This avoids rewriting identical content across many savepoints.

## Audit streams

```text
raf:{ws}:workspace:audit
raf:{ws}:session:<sid>:audit
```

Fields:

- `ts_ms`
- `workspace`
- `session`
- `actor_type`
- `actor_id`
- `op`
- `path`
- `path2`
- `result`
- `error`
- `command`
- `cwd`

Examples:

- `run_start`
- `run_exit`
- `save`
- `rollback`
- `fork`
- `materialize`
- `apply`

## Local Materialization Model

This is the native execution side.

Suggested default location:

```text
~/.raf/workspaces/<workspace>/sessions/<session>/
```

Local metadata:

```text
~/.raf/workspaces/<workspace>/sessions/<session>/.raf-local.json
```

Fields:

- `workspace`
- `session`
- `head_savepoint`
- `materialized_at`
- `dirty`
- `last_scan_at`

### Materialize

Materialization means:

1. create or clean the local session directory
2. fetch the session head savepoint manifest from Redis
3. write files to disk
4. restore metadata where practical
5. mark local state clean

This can be full materialization for MVP.

Later it can be optimized with:

- lazy fetch
- hardlink/reflink cache
- sparse materialization

### Detect dirty state

For MVP, dirty detection can be simple:

- explicit dirty bit after `raf run`
- refreshed on `raf save`

A stronger version can:

- rescan mtimes and sizes
- recompute hashes on changed files only

### Why this is good

This means RAF can always run native tools without requiring a custom live filesystem backend.

## Execution Modes

## Host mode

`raf run` in host mode:

1. ensure local materialization
2. `exec.Command(...)`
3. `cmd.Dir = localSessionPath`
4. inherit or stream stdio

This is the easiest mode to ship.

## Sandbox mode

`raf run` in sandbox mode:

1. ensure local materialization
2. bind-mount or copy the local session directory into a container/jail/VM
3. set cwd to `/workspace`
4. run the command in the sandbox
5. sync changed files back to the local materialization if needed

This keeps the same semantics while adding safety.

The key point is that Redis is still the canonical saved state in both modes.

## Save Algorithm

MVP save algorithm:

1. load session head savepoint manifest from Redis
2. walk the local session directory
3. compare current files against head manifest using path + size + mtime and hash when needed
4. upload changed blobs to Redis
5. build a new manifest
6. write savepoint metadata
7. update session head to the new savepoint
8. mark local state clean

Deletes:

- if a path exists in old manifest but not on disk, omit it from the new manifest

That means savepoints are full immutable filesystem views, even if stored deduplicated by blob.

## Fork Algorithm

MVP fork algorithm:

1. resolve source session head savepoint
2. create new session metadata with same `head_savepoint`
3. do not materialize immediately unless requested

This is very cheap and elegant.

## Rollback Algorithm

MVP rollback algorithm:

1. verify target savepoint exists
2. update `session:meta.head_savepoint`
3. archive or delete current local directory
4. materialize target savepoint into fresh local directory
5. mark clean

This is much simpler than trying to undo commands one by one.

## Diff Algorithm

There are really two useful diffs:

### Dirty diff

Compare:

- local materialization
- session head savepoint

Used by:

- `raf diff`

### Saved diff

Compare:

- savepoint A
- savepoint B

Used later for:

- history
- PR-like review
- auditing

## Native-First Benefits

This design makes the CLI implementation dramatically easier:

- `raf run` is just process execution with a real cwd
- editors and shells work natively
- no FUSE required for MVP
- no live overlay filesystem required for MVP
- local debugging is straightforward

It also preserves the big value proposition:

- savepoints are centralized
- rollback is deterministic
- forks are cheap
- canonical state can be remote and backed up

## What This Does Not Solve Yet

This model is elegant, but it does not automatically give:

- live multi-writer shared editing
- true concurrent shared directories
- instant remote visibility of unsaved changes
- efficient giant monorepo cloning without optimization

That is okay.

Those are advanced capabilities and should not block the cleanest first product.

## Future Shared Mode

If you later want live shared mode, RAF can add it without changing the public model.

Users still say:

- workspace
- session
- savepoint

Under the hood, shared mode can use:

- Redis-backed live overlay
- FUSE or NFS projection
- optimistic locking
- live session streams

But that should be an extension, not the default.

## Suggested MVP

If I were implementing RAF from here, I would ship this order:

### Phase 1

- `raf import`
- `raf inspect`
- `raf sessions`
- `raf run`
- `raf diff`
- `raf save`
- `raf fork`
- `raf rollback`

Backed by:

- local materialization directories
- Redis savepoint manifests
- Redis blob store
- Redis audit streams

### Phase 2

- `raf apply`
- `raf saves`
- optional Git-aware helpers
- sandbox mode for `raf run`

### Phase 3

- smarter dedup
- lazy materialization
- large file chunking
- shared live mode

## Recommended Defaults

### Default Redis role

Redis is the source of truth for saved state.

### Default execution role

The local OS filesystem is the source of truth for active unsaved state.

### Default CLI behavior

- `raf run` does not save automatically
- `raf save` is explicit
- `raf rollback` discards unsaved local changes unless asked to preserve them

These defaults are understandable and safe.

## Bottom Line

If RAF always uses Redis for save, fork, rollback, audit, and canonical state, while using native directories for active execution, you get the best of both worlds:

- native process and tool compatibility
- remote durable session history
- clean savepoint semantics
- a small understandable CLI
- a path to shared mode later

This is the most elegant RAF architecture I can see right now.
