# RAF Phase 1 Execution Plan

Date: 2026-04-02

## Goal

Ship the smallest real RAF that feels coherent:

- `raf` is the main CLI
- Redis stores canonical saved state
- local directories are used for live execution
- `raf run`, `raf save`, `raf fork`, `raf rollback`, and `raf diff` work end to end

Phase 1 should be good enough for:

- one developer
- one machine
- one or more sessions
- explicit savepoints
- real shell and tool execution

It does not need to solve:

- live multi-writer shared editing
- FUSE-based live overlays
- distributed unsaved state
- giant monorepo optimization

## Success Criteria

By the end of Phase 1, this should work:

```bash
raf import my-repo ./repo
raf fork my-repo main fix-login
raf run my-repo --session fix-login -- bash
raf diff my-repo --session fix-login
raf save my-repo --session fix-login before-refactor
raf rollback my-repo --session fix-login before-refactor
```

And the semantics should be clear:

- live state is local
- saved state is in Redis
- savepoints are immutable
- rollback rematerializes from saved state

## Scope

### In scope

- CLI rename to `raf`
- new Redis key helpers for RAF canonical state
- local materialization directories under `~/.raf`
- import, inspect, sessions, saves
- run, diff, save, fork, rollback
- Redis audit streams
- optimistic concurrency checks on save

### Out of scope

- apply back to target directories
- Git-aware helpers
- sandbox mode
- FUSE/NFS integration for new RAF workflows
- shared live overlay mode
- large-file chunking optimization

## Implementation Strategy

The fastest safe path is:

1. keep the current CLI architecture
2. add new RAF files in `cli/` under `package main`
3. avoid a big package refactor
4. keep current `setup/up/down/status` flow, but rename it to `raf`
5. implement native-first commands beside existing codepaths

That means:

- low repo churn
- easier testing
- easier rollback if a decision changes

## Command Surface

Phase 1 commands:

```bash
raf setup
raf up
raf down
raf status

raf import <workspace> <directory>
raf inspect <workspace> [--session <name>]
raf sessions <workspace>
raf saves <workspace> --session <name>

raf run <workspace> [--session <name>] -- <command...>
raf diff <workspace> --session <name>
raf save <workspace> --session <name> [savepoint]
raf fork <workspace> <session> <new-session>
raf rollback <workspace> --session <name> <savepoint>
```

Defaults:

- default session is `main`
- `raf run` does not save automatically
- `raf rollback` discards unsaved local state after archiving it

## Rename Plan

Because there are effectively no users, do the rename early.

### Rename targets

- binary name: `rfs` -> `raf`
- config file: `rfs.config.json` -> `raf.config.json`
- state dir: `~/.rfs` -> `~/.raf`
- user-facing docs: `Agent Filesystem` -> `Redis Agent Filesystem` where appropriate

### Keep for now

- `FS.*` command family
- existing repo path and package/module names, unless they become a blocker

This keeps the rename focused on product surface first.

## File Layout

## Existing files to modify

- [cli/main.go](/Users/rowantrollope/git/agent-filesystem/cli/main.go)
- [cli/ui.go](/Users/rowantrollope/git/agent-filesystem/cli/ui.go)
- [cli/Makefile](/Users/rowantrollope/git/agent-filesystem/cli/Makefile)
- [Makefile](/Users/rowantrollope/git/agent-filesystem/Makefile)
- [README.md](/Users/rowantrollope/git/agent-filesystem/README.md)

## New files to add

Keep them all in `cli/` and in `package main` for speed:

- `cli/raf_types.go`
- `cli/raf_keys.go`
- `cli/raf_store.go`
- `cli/raf_local.go`
- `cli/raf_hash.go`
- `cli/raf_diff.go`
- `cli/raf_commands.go`
- `cli/raf_state.go`

This avoids package churn during the first implementation.

## Concrete Data Structures

## Config

Extend the existing config in [cli/main.go](/Users/rowantrollope/git/agent-filesystem/cli/main.go#L23) with RAF-native fields:

```go
type config struct {
    UseExistingRedis bool   `json:"useExistingRedis"`
    RedisAddr        string `json:"redisAddr"`
    RedisUsername    string `json:"redisUsername"`
    RedisPassword    string `json:"redisPassword"`
    RedisDB          int    `json:"redisDB"`
    RedisTLS         bool   `json:"redisTLS"`

    WorkRoot         string `json:"workRoot"`
    DefaultWorkspace string `json:"defaultWorkspace"`
    DefaultSession   string `json:"defaultSession"`
    RuntimeMode      string `json:"runtimeMode"` // host or sandbox

    RedisLog         string `json:"redisLog"`
    MountLog         string `json:"mountLog"`

    // keep existing mount/backend fields for compatibility
}
```

Recommended defaults:

- `WorkRoot = ~/.raf/workspaces`
- `DefaultSession = main`
- `RuntimeMode = host`

## Local session state

```go
type rafLocalState struct {
    Version           int       `json:"version"`
    Workspace         string    `json:"workspace"`
    Session           string    `json:"session"`
    HeadSavepoint     string    `json:"head_savepoint"`
    Dirty             bool      `json:"dirty"`
    MaterializedAt    time.Time `json:"materialized_at"`
    LastScanAt        time.Time `json:"last_scan_at"`
    ArchivedAt        time.Time `json:"archived_at,omitempty"`
}
```

Stored at:

```text
~/.raf/workspaces/<workspace>/sessions/<session>/state.json
```

Actual files live at:

```text
~/.raf/workspaces/<workspace>/sessions/<session>/tree/
```

Using `tree/` keeps metadata out of the user's working directory.

## Workspace metadata

```go
type workspaceMeta struct {
    Version          int       `json:"version"`
    Name             string    `json:"name"`
    CreatedAt        time.Time `json:"created_at"`
    DefaultSession   string    `json:"default_session"`
    DefaultSavepoint string    `json:"default_savepoint"`
}
```

## Session metadata

```go
type sessionMeta struct {
    Version                 int       `json:"version"`
    Workspace               string    `json:"workspace"`
    Name                    string    `json:"name"`
    HeadSavepoint           string    `json:"head_savepoint"`
    CreatedAt               time.Time `json:"created_at"`
    UpdatedAt               time.Time `json:"updated_at"`
    DirtyHint               bool      `json:"dirty_hint"`
    LastMaterializedAt      time.Time `json:"last_materialized_at"`
    LastKnownMaterializedAt string    `json:"last_materialized_host,omitempty"`
}
```

## Savepoint metadata

```go
type savepointMeta struct {
    Version         int       `json:"version"`
    ID              string    `json:"id"`
    Name            string    `json:"name"`
    Workspace       string    `json:"workspace"`
    Session         string    `json:"session"`
    ParentSavepoint string    `json:"parent_savepoint,omitempty"`
    ManifestHash    string    `json:"manifest_hash"`
    CreatedAt       time.Time `json:"created_at"`
    FileCount       int       `json:"file_count"`
    DirCount        int       `json:"dir_count"`
    TotalBytes      int64     `json:"total_bytes"`
}
```

## Manifest

For Phase 1, keep the manifest simple and whole.

```go
type manifest struct {
    Version   int                      `json:"version"`
    Workspace string                   `json:"workspace"`
    Savepoint string                   `json:"savepoint"`
    Entries   map[string]manifestEntry `json:"entries"`
}

type manifestEntry struct {
    Type    string `json:"type"` // file, dir, symlink
    Mode    uint32 `json:"mode"`
    MtimeMs int64  `json:"mtime_ms"`
    Size    int64  `json:"size"`
    BlobID  string `json:"blob_id,omitempty"`
    Inline  string `json:"inline,omitempty"` // base64 for small files
    Target  string `json:"target,omitempty"` // symlink target
}
```

### Why one whole manifest?

Because it is the lowest-complexity Phase 1 design.

For code and text repos, a manifest JSON blob is usually tractable.
We can normalize or shard it later if needed.

## Blob strategy

Use content-addressed blobs with `sha256`.

```go
type blobRef struct {
    BlobID    string    `json:"blob_id"`
    Size      int64     `json:"size"`
    RefCount  int64     `json:"ref_count"`
    CreatedAt time.Time `json:"created_at"`
}
```

Blob rules:

- small files may be inlined in manifest under a threshold like 4 KiB
- larger files stored in `raf:{ws}:blob:<sha256>`
- dedup within a workspace

This is simple and good enough.

## Redis Key Schema

Use:

```text
raf:{workspace}:...
```

Concrete keys:

```text
raf:{ws}:workspace:meta
raf:{ws}:workspace:sessions
raf:{ws}:workspace:savepoints
raf:{ws}:workspace:audit

raf:{ws}:session:<sid>:meta
raf:{ws}:session:<sid>:audit

raf:{ws}:savepoint:<sp>:meta
raf:{ws}:savepoint:<sp>:manifest

raf:{ws}:blob:<blobid>
raf:{ws}:blobref:<blobid>
```

Keep these helpers in `cli/raf_keys.go`.

## Command Behavior Details

## `raf import`

Algorithm:

1. verify workspace does not exist unless `--force`
2. scan source directory
3. build a manifest from disk
4. upload blobs
5. create savepoint `initial`
6. create session `main` pointing at `initial`
7. create local materialization at `~/.raf/workspaces/<ws>/sessions/main/tree`
8. write local state as clean
9. emit audit event

Notes:

- skip `.git` by default only if we decide to; otherwise import everything
- `.rafignore` can wait until Phase 2

## `raf run`

Algorithm:

1. resolve workspace and session
2. fetch session meta from Redis
3. ensure local materialization exists
4. if no local materialization, materialize from `head_savepoint`
5. if local materialization exists but local state head differs from Redis head:
   - if local is clean, rematerialize
   - if local is dirty, error with conflict
6. spawn child process with:
   - `cmd.Dir = .../tree`
   - inherited env
   - inherited or streamed stdio
7. when process exits:
   - mark local state dirty if exit path may have changed files
   - emit `run_exit`

Important simplification:

- for Phase 1, always mark dirty after `raf run`
- a later optimization can avoid this if command is known read-only

## `raf diff`

Phase 1 output:

- summary only

Example:

```text
M src/auth.go
A notes/todo.md
D tmp/debug.log
```

Algorithm:

1. load local state
2. load head savepoint manifest
3. walk local `tree/`
4. compare path/type/size/mtime and hash when needed
5. print changed paths

Patch output can wait.

## `raf save`

Algorithm:

1. load local state
2. load Redis session meta
3. verify `local.HeadSavepoint == redisSession.HeadSavepoint`
4. if not equal:
   - if local is clean, rematerialize and no-op
   - if local is dirty, return conflict and suggest `raf fork`
5. scan local directory
6. compare against head manifest
7. upload new blobs
8. construct new manifest
9. create new savepoint metadata
10. atomically move session head to new savepoint
11. update local state head and mark clean
12. emit audit event

### Atomic head update

Use Redis optimistic concurrency:

- `WATCH` session meta key
- verify current `head_savepoint`
- `MULTI`
- write savepoint meta and manifest
- update session meta head
- `EXEC`

If `EXEC` fails, report save conflict.

This is enough for Phase 1 and keeps the concurrency model very simple.

## `raf fork`

Algorithm:

1. load source session meta
2. create destination session meta with same `head_savepoint`
3. add destination to workspace session set
4. do not materialize immediately
5. emit audit event

This is cheap and elegant.

## `raf rollback`

Algorithm:

1. verify target savepoint exists
2. archive current local directory if present to `~/.raf/archive/...`
3. atomically update session head to target savepoint
4. rematerialize local directory from target savepoint
5. mark local state clean
6. emit audit event

### Safety rule

Do not delete dirty local state outright in Phase 1.
Archive it first.

That gives us safer rollback with simple implementation.

## `raf inspect`

Behavior:

- workspace mode
  - show session count
  - default session
  - latest savepoints
  - total blobs and bytes
- session mode
  - show head savepoint
  - local materialization path
  - dirty/clean state
  - last materialized time

## `raf sessions`

Behavior:

- list session names and head savepoint names

## `raf saves`

Behavior:

- list savepoints for a session in reverse chronological order

## Local Materialization Rules

Directory layout:

```text
~/.raf/
  state.json
  workspaces/
    my-repo/
      sessions/
        main/
          state.json
          tree/
        fix-login/
          state.json
          tree/
      archive/
```

Rules:

- only `tree/` is used as cwd for commands
- state file sits outside `tree/`
- rollback archives old `tree/` under workspace `archive/`

## Materialize Function

Create a single helper in `cli/raf_local.go`:

```go
func materializeSession(ctx context.Context, store *rafStore, ws, session string) error
```

Responsibilities:

- load session head savepoint
- fetch manifest
- create clean `tree/`
- write files
- write local state

For Phase 1, full rematerialization is acceptable.

## Dirty-State Rules

Phase 1 dirty-state policy:

- after any `raf run`, mark session dirty
- after `raf save`, mark clean
- after `raf rollback`, mark clean
- after `raf import`, mark clean

This is intentionally conservative.

We can improve later with:

- stat-based dirty detection
- read-only command hints
- file watcher integration

## Build and Rename Steps

Concrete repo changes:

1. update [cli/Makefile](/Users/rowantrollope/git/agent-filesystem/cli/Makefile) to build `raf`
2. update [Makefile](/Users/rowantrollope/git/agent-filesystem/Makefile) to place `raf` at repo root
3. update usage strings in [cli/main.go](/Users/rowantrollope/git/agent-filesystem/cli/main.go)
4. rename config/state defaults to `raf.config.json` and `~/.raf`
5. keep existing current logic for `setup/up/down/status` as much as possible

This rename should happen before adding user-facing new commands.

## Recommended Coding Sequence

## Slice 0: Rename surface

- rename CLI binary references to `raf`
- rename config and state paths
- update help text and status output

This gets the product name aligned before deeper work.

## Slice 1: Redis store primitives

- add `raf_types.go`
- add `raf_keys.go`
- add `raf_store.go`
- implement workspace/session/savepoint CRUD
- implement audit helper

No command wiring yet beyond a tiny smoke test path.

## Slice 2: Local materialization

- add `raf_local.go`
- implement local paths and state file helpers
- implement `materializeSession`
- implement local archive helper

This is the foundation for `run` and `rollback`.

## Slice 3: `raf import`, `inspect`, `sessions`, `saves`

- wire command parsing in [cli/main.go](/Users/rowantrollope/git/agent-filesystem/cli/main.go)
- implement workspace bootstrap and listing

This gives visibility into the stored model early.

## Slice 4: `raf run`

- implement command launch with `cmd.Dir = tree/`
- record audit events
- mark dirty after process exit

This is the first end-to-end “real app” moment.

## Slice 5: `raf save` and `raf diff`

- implement hashing and manifest comparison
- implement optimistic save
- implement summary diff

This is the highest-risk correctness slice.

## Slice 6: `raf fork` and `raf rollback`

- implement session branching
- implement rollback by rematerialization
- archive local dirty state before rollback

This completes the branch/savepoint story.

## Testing Plan

Add Go tests alongside the new files.

Priority tests:

- key generation
- manifest hashing stability
- import builds expected manifest
- save with no changes
- save with file modifications
- save conflict when session head changed remotely
- fork reuses head savepoint
- rollback rematerializes expected files
- run uses correct cwd

Test style:

- unit tests for helpers
- integration tests against temporary directories and a real Redis instance if available

## Major Risks

### 1. Save conflicts are confusing

Mitigation:

- use a strict compare-and-set on `head_savepoint`
- when conflict occurs, show a very explicit message:
  - session moved remotely
  - your local copy is dirty
  - use `raf fork` or inspect/save elsewhere

### 2. Manifest JSON gets large

Mitigation:

- acceptable in Phase 1
- optimize later with compression or sharding if needed

### 3. Rollback destroys local unsaved state

Mitigation:

- archive local tree before rollback
- expose archive path in output

### 4. Rename scope balloons

Mitigation:

- rename product surface first
- keep internal Go module/repo names unchanged until later

## What We Should Not Do Yet

Do not start with:

- full repo-wide package/module rename
- live Redis filesystem overlays
- FUSE integration for new RAF workflows
- sandbox mode
- advanced patch diff output
- perfect dirty detection

Those will slow us down and blur the model.

## Recommended First PR Shape

The first execution PR should ideally contain:

1. `raf` binary rename
2. config/state rename
3. Redis key helpers and store types
4. local materialization helpers
5. `raf import`
6. `raf inspect`
7. `raf sessions`
8. `raf run`

That is a very strong vertical slice.

Then the next PR can add:

1. `raf diff`
2. `raf save`
3. `raf fork`
4. `raf rollback`

## Bottom Line

Phase 1 should bias toward one thing above all else:

make RAF feel obvious.

That means:

- native directories for execution
- Redis for saved truth
- explicit savepoints
- cheap forks
- deterministic rollback
- one CLI with flat verbs

If we keep that discipline, execution can start soon without painting the product into a corner.
