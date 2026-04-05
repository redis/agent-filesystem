# Redis Agent Filesystem Roadmap

Date: 2026-04-02

## Goal

Turn Redis Agent Filesystem from "a filesystem stored in Redis" into "a branchable, inspectable, shared agent workspace layer" without giving up the shell, FUSE/NFS access, or the existing `FS.*` and MCP surfaces.

This document proposes:

- a product model,
- a command set,
- a Redis storage layout,
- overlay and whiteout semantics,
- audit logging,
- mount and sandbox integration,
- a phased implementation plan.

## Product Thesis

The right near-term product for Redis Agent Filesystem is:

Redis Agent Filesystem is a shared, queryable workspace substrate for agents and humans. Shells, containers, and microVMs execute code against it, while Redis provides persistence, branching, audit, and collaboration.

That means:

- keep `bash`,
- keep normal toolchains,
- add sessions and overlays,
- make diffs and replay first-class,
- use Redis as the coordination layer.

## Design Principles

### 1. Keep the current filesystem usable

Existing flows should keep working:

- `FS.*`
- FUSE mount
- NFS mount
- Python client
- MCP server
- `raf up/down/status/migrate`

Sessions should be additive, not a rewrite-first prerequisite.

### 2. Optimize for coding and text workflows first

The first target is:

- source repos,
- configs,
- docs,
- logs,
- memory files,
- generated patches and reports.

Do not block on perfect large-binary behavior before shipping the overlay model.

### 3. Separate compute from workspace state

Redis Agent Filesystem should not be the only answer to safe execution.
It should pair well with:

- local shell + Git worktrees,
- Docker,
- nsjail,
- microVMs,
- cloud sandboxes.

### 4. Favor explicit session semantics

Users should reason in terms of:

- workspace
- session
- savepoint
- diff
- apply
- audit log

Those are the agent-native primitives.

## Redis Design Pass

This is the recommended simplification pass for the whole product.

### One public binary

There should be one public CLI:

- `raf`

These should remain internal plumbing or power-user tools:

- mount and NFS helper daemons
- direct Redis key inspection

This mirrors Redis product design well:

- one obvious entrypoint,
- richer internals underneath,
- low user-facing ceremony.

### One public mental model

Users should only need three visible nouns:

- workspace
- session
- savepoint

Everything else is implementation detail.

In particular:

- snapshots are internal immutable objects,
- overlays are internal mechanics,
- whiteouts are internal mechanics,
- FUSE and NFS are transport details.

### Flat verbs over nested command trees

The public CLI should stay closer to a simple `raf up/down/status/migrate` style than to a very deep Git-style noun tree.

Recommended public verbs:

- `init`
- `import`
- `mount`
- `run`
- `inspect`
- `diff`
- `apply`
- `fork`
- `save`
- `rollback`
- `log`

This keeps the surface small and predictable for both humans and agents.

### Public savepoints, internal snapshots

Users care about:

- "save where I am now"
- "roll back to that point"

They do not need to care whether that is implemented with:

- materialized snapshots,
- logical snapshots,
- copy-on-write trees,
- or pointer swaps.

So the public language should be:

- `save`
- `rollback`

And the storage language can stay:

- snapshot
- base snapshot
- snapshot materialization

### Prefer pointer moves over inverse operations

The most elegant rollback design is not:

- compute inverse diffs,
- replay undo operations,
- or mutate the current tree in place.

It is:

1. create immutable savepoint snapshot
2. repoint session `base_snapshot`
3. clear session overlay

This is simpler to explain, simpler to debug, and much more Redis-like.

### Simplicity over clever dedup in phase 1

Phase 1 should favor:

- materialized savepoints,
- shallow lineage,
- explicit copy-up,
- clear key layout.

It should not block on:

- maximal deduplication,
- arbitrary ancestry chains,
- perfect O(1) rename for inherited trees.

That is the cleanest path to an elegant first product.

## Naming Recommendation

### Recommended product name

Use:

- `Redis Agent Filesystem`

Short form:

- `RAF`

Why this works:

- it keeps Redis in the name, which is the differentiator,
- it makes the agent use case explicit,
- it gives the CLI the same acronym as the product name,
- it avoids fighting directly over the `AgentFS` name,
- it is still short and command-line friendly.

### Recommended tagline

Something close to:

- `Branchable Redis-backed workspaces for agents`

That is more useful than just saying "filesystem on Redis."

### What not to do

I would avoid:

- introducing a second top-level CLI,
- leading with "snapshot filesystem" language in the product name,
- or using a name too close to `AgentFS`.

### Suggested naming stack

- Product: `Redis Agent Filesystem`
- Short name: `RAF`
- CLI: `raf`
- Redis command family: keep `FS.*`
- Mount/NFS binaries: internal implementation details

That is the lowest-friction naming scheme.

## Product Model

### Workspace

A workspace is the top-level logical unit for one codebase or one persistent agent state domain.

Examples:

- `my-repo`
- `agent-memory`
- `customer-support-bot`

### Session

A session is a writable overlay on top of a snapshot.

Use cases:

- one agent task,
- one human debugging branch,
- one experiment or patchset,
- one multi-agent shared workspace.

### Savepoint

A savepoint is the public rollback primitive for a session.

Use cases:

- checkpoint before a risky edit,
- restore a known-good state,
- create a stable base for a fork,
- preserve a successful run before more experimentation.

Internally, a savepoint maps to an immutable snapshot.

### Audit Stream

The audit stream records filesystem and runtime events associated with a workspace and session.

Use cases:

- timeline inspection,
- debugging,
- compliance,
- usage analytics,
- replay.

## User Experience

## Recommended Public CLI

This is the recommended low-surface-area command set.

### Core commands

```bash
raf init <workspace>
raf import <workspace> <directory>
raf mount <workspace> [--session <name>] <mountpoint>
raf run <workspace> [--session <name>] -- <command...>
raf inspect <workspace> [--session <name>]
raf diff <workspace> --session <name>
raf apply <workspace> --session <name> --to-dir <path>
raf fork <workspace> <session> <new-session>
raf save <workspace> --session <name> [savepoint]
raf rollback <workspace> --session <name> <savepoint>
raf log <workspace> [--session <name>]
```

### Optional management commands

If we need explicit listing and cleanup, keep them few and obvious:

```bash
raf sessions <workspace>
raf saves <workspace> --session <name>
raf drop <workspace> --session <name>
```

### Why this is the right CLI shape

- It keeps one public binary.
- It uses flat verbs instead of deep subcommand trees.
- It maps to user intent, not storage internals.
- It is easy for an agent to infer.
- It leaves room for richer internals later.

### One CLI controls settings

All user-facing configuration should flow through `raf`.

That means:

- `raf setup` remains the interactive entrypoint,
- `raf.config.json` remains the persisted config surface,
- advanced flags hang off `raf`, not separate helper tools,
- mount/NFS binaries are implementation details invoked by `raf`.

Examples of settings that should stay under one roof:

- Redis connection
- mount backend
- default workspace
- default session
- read-only mode
- allow-other
- sandbox mode or runtime backend

### Behavioral rules

- `mount` without `--session` mounts the default session, normally `main`.
- `run` is the reference shell or agent entrypoint.
- `save` creates a rollback point for a session.
- `rollback` restores a session to a savepoint.
- `fork` creates a new writable session from a savepoint-backed state.

### Default session

Every workspace should have a default writable session:

- `main`

This keeps the mental model simple:

- import workspace
- mount workspace
- save when needed
- create or fork named sessions only for isolation

That is much easier to explain than requiring users to create a session before doing anything.

## Recommended MVP User Stories

### Story 1: Safe coding session over a clean repo

1. `raf import my-repo ./repo`
2. `raf fork my-repo main fix-login`
3. `raf mount my-repo --session fix-login /tmp/mnt`
4. Run agent or shell against `/tmp/mnt`
5. `raf diff my-repo --session fix-login`
6. `raf apply my-repo --session fix-login --to-dir ./repo`

### Story 2: Human joins a live agent session

1. Agent runs in session `fix-login`
2. Human opens another terminal and mounts the same session
3. Human inspects files or runs tests
4. `raf log my-repo --session fix-login` shows commands and file operations

### Story 3: Fork before risky work

1. `raf save my-repo --session fix-login pre-refactor`
2. `raf fork my-repo fix-login try-alt-approach`
3. Agent runs in `try-alt-approach`
4. Compare `diff` between sessions
5. Apply or discard

### Story 4: Roll back cleanly

1. `raf save my-repo --session fix-login known-good`
2. Agent makes a mess
3. `raf rollback my-repo --session fix-login known-good`
4. Session is restored without replaying inverse operations

## Redis Storage Model

This section proposes the Redis key layout for workspaces, snapshots, sessions, and audit streams.

The design goal is:

- preserve the current key style,
- avoid a mandatory full data-model rewrite in phase 1,
- leave room for inode IDs and chunking later.

## Naming

Use the current `fsKey` as the workspace key.

Example:

- workspace key: `my-repo`
- Redis hashtag root: `raf:{my-repo}:...`

## Top-Level Metadata

```text
raf:{ws}:workspace:meta
raf:{ws}:workspace:audit
raf:{ws}:workspace:heads
```

Suggested contents:

- `workspace:meta`
  - format version
  - created_at
  - default_session=`main`
  - default_snapshot
  - session_count
  - snapshot_count
- `workspace:audit`
  - Redis Stream for all workspace events
- `workspace:heads`
  - optional map of named heads like `main -> snapshot-123`

## Snapshot Keys

Snapshots remain an internal storage primitive, even if the public CLI exposes savepoints instead.

Initial design:

```text
raf:{ws}:snapshot:<snap>:meta
raf:{ws}:snapshot:<snap>:inode:<path>
raf:{ws}:snapshot:<snap>:children:<path>
raf:{ws}:snapshot:<snap>:info
```

This deliberately mirrors the current standard-key backend so the first implementation can reuse a lot of code.

Snapshot metadata:

- `id`
- `name` or user-visible savepoint label
- `created_at`
- `source_session`
- `source_snapshot`
- `materialized=true`

### Why materialized snapshots first?

Because they are simple, compatible with the existing path-keyed model, and make the first session implementation easier.

Later we can optimize snapshots with:

- deduplicated blobs,
- inode IDs,
- copy-on-write snapshot trees.

## Session Keys

```text
raf:{ws}:session:<sid>:meta
raf:{ws}:session:<sid>:inode:<path>
raf:{ws}:session:<sid}:children:add:<path>
raf:{ws}:session:<sid}:children:del:<path>
raf:{ws}:session:<sid}:whiteout:<path>
raf:{ws}:session:<sid}:info
raf:{ws}:session:<sid}:audit
```

Session metadata fields:

- `id`
- `name`
- `base_snapshot`
- `created_at`
- `updated_at`
- `status` active, archived, deleted
- `owner`
- `mount_count`
- `mode` readwrite or readonly

### Recommended lineage rule

For elegance, each session should point to exactly one base snapshot.

Do not support arbitrary deep parent chains in phase 1.

That means:

- `fork` first materializes the source effective state into a snapshot
- the new session points to that snapshot
- `rollback` repoints the existing session to a savepoint snapshot and clears overlay keys

This is simpler than multi-hop lineage walking and easier to debug.

### Overlay semantics

Session data stores only changes relative to its base snapshot.

Overlay entries can represent:

- created files
- modified files
- created directories
- modified metadata
- created symlinks
- deleted paths via whiteouts
- directory membership deltas

## Whiteout Model

Whiteouts are required because a path may exist in the base snapshot but be deleted in the session.

Suggested representation:

```text
SET raf:{ws}:session:<sid>:whiteout:<path> "1"
```

Or equivalently a zero-field HASH/string marker.

Behavior:

- if a whiteout exists at `path`, the merged view reports `ENOENT`,
- if a whiteout exists for a directory, all descendant paths under it are hidden unless explicitly recreated in the session,
- deleting a session-created path removes the overlay inode and records a whiteout only if the path existed in the base snapshot.

### Directory merge state

Directory listings need explicit add/delete sets:

- `children:add:<dir>` holds names introduced by the session
- `children:del:<dir>` holds names hidden from the base snapshot

Merged listing for directory `D`:

1. load base children from snapshot
2. union overlay-added children
3. subtract overlay-deleted children
4. subtract names whose full child path is whiteouted
5. sort output

This is enough for a correct MVP without redesigning the whole inode model.

## Read Path Resolution

For session `<sid>` and path `P`:

1. Normalize `P`
2. If `session:whiteout:P` exists, return not found
3. If `session:inode:P` exists, return overlay inode
4. Otherwise read `snapshot:inode:P` from the base snapshot

For directory listing:

1. Resolve the effective directory inode
2. Load base children
3. Load `children:add`
4. Load `children:del`
5. Hide whiteouted entries
6. Return the merged list

This should be implemented in a new session-aware client backend before touching FUSE behavior.

## Write Path Semantics

### Create file

When creating `/a/b.txt`:

1. ensure parent directories exist in merged view
2. write `session:inode:/a/b.txt`
3. add `b.txt` to `children:add:/a`
4. remove `b.txt` from `children:del:/a` if present
5. remove any whiteout at `/a/b.txt`
6. emit audit event

### Modify inherited file

When editing a file that exists only in the base snapshot:

1. load base inode and content
2. write a copy-up inode into `session:inode:<path>`
3. mutate the session inode
4. emit audit event

This is expensive but acceptable for phase 1.

### Delete file

When deleting `/a/b.txt`:

1. if `session:inode:/a/b.txt` exists, remove it
2. if file exists in base snapshot, write `whiteout:/a/b.txt`
3. add `b.txt` to `children:del:/a`
4. remove `b.txt` from `children:add:/a` if this was session-created
5. emit audit event

### Rename or move

This is the hardest operation in the phase 1 model.

Phase 1 proposal:

- support file rename first,
- support directory rename by materializing the subtree into the session overlay,
- explicitly reject moves into the same subtree,
- document that inherited subtree rename is O(number of descendants).

Phase 2 proposal:

- move to inode IDs so subtree rename updates directory entries rather than path-keyed inodes.

## Save and Rollback Model

This is the key simplification.

### `raf save`

Public meaning:

- freeze the current effective state of a session under a savepoint name

Internal behavior:

1. materialize the effective session tree into a new immutable snapshot
2. record savepoint metadata pointing to that snapshot
3. leave the current session untouched

### `raf rollback`

Public meaning:

- restore a session to a previously saved state

Internal behavior:

1. find the snapshot associated with the savepoint
2. update `session:meta.base_snapshot`
3. delete the current session overlay keys
4. emit one rollback audit event

This is cleaner than replaying inverse operations and makes rollback semantics very easy to explain.

### `raf fork`

Public meaning:

- create a new session from the current state of another session

Internal behavior:

1. materialize the source effective state into a snapshot
2. create a new session with `base_snapshot` pointing to it
3. start with an empty overlay

This gives a very clean "copy the current branch state" behavior.

## Diff Model

`raf diff` should not depend on Git. It should compare session overlay against the base snapshot.

Output modes:

- `summary`
- `patch`
- `json`

Example summary:

```text
M /src/auth.go
A /docs/notes.md
D /tmp/debug.log
R /old/path.txt -> /new/path.txt
```

Patch mode:

- unified diffs for text files
- binary marker for non-text content

JSON mode:

- machine-readable event list for UIs, MCP, or future APIs

## Apply Model

`raf apply` should support:

- apply to local directory
- apply to mounted repo
- optional Git-aware mode

Suggested flags:

```bash
raf apply my-repo --session fix-login --to-dir ./repo
raf apply my-repo --session fix-login --to-dir ./repo --git-stage
raf apply my-repo --session fix-login --to-dir ./repo --check-clean
```

Behavior:

- create/update/delete files to match the session diff
- preserve mode and timestamps when practical
- optionally refuse if target directory has conflicting local changes

This lets Redis Agent Filesystem complement Git worktrees instead of competing with them.

## Audit Log Design

Audit is one of the best ways for Redis Agent Filesystem to beat plain volumes and plain bash.

Use Redis Streams:

```text
raf:{ws}:workspace:audit
raf:{ws}:session:<sid>:audit
```

Each file operation emits an event.

Suggested fields:

- `ts_ms`
- `workspace`
- `session`
- `actor_type` human, agent, system
- `actor_id`
- `op`
- `path`
- `path2` for rename/move
- `bytes`
- `mode`
- `uid`
- `gid`
- `result` ok or error
- `error`

Example ops:

- `create`
- `write`
- `append`
- `truncate`
- `mkdir`
- `unlink`
- `rmdir`
- `rename`
- `chmod`
- `chown`
- `utimens`
- `mount`
- `unmount`
- `process_launch`
- `process_exit`

### Why Streams?

Because they give:

- append-only timelines,
- consumer groups for UIs or dashboards,
- replay,
- trimming policies,
- cheap operational analytics.

## Mount Integration

## New client type

Add a new client constructor:

```go
client.NewSession(rdb, workspace, sessionID)
```

This client implements the same `Client` interface as the current native backend, but reads through the overlay and base snapshot.

That means the FUSE layer can stay mostly unchanged.

Relevant existing surface:

- [mount/internal/client/client.go](/Users/rowantrollope/git/redis-fs/mount/internal/client/client.go)
- [mount/internal/client/native.go](/Users/rowantrollope/git/redis-fs/mount/internal/client/native.go)
- [mount/internal/client/keys.go](/Users/rowantrollope/git/redis-fs/mount/internal/client/keys.go)

## CLI and daemon flags

Add support for:

```bash
redis-fs-mount --workspace my-repo --session fix-login /mnt
redis-fs-nfs --workspace my-repo --session fix-login ...
```

Or equivalently:

```bash
redis-fs-mount --key my-repo --session fix-login /mnt
```

Then layer that into:

```bash
raf mount my-repo --session fix-login /mnt
raf up --session fix-login
```

Relevant existing code:

- [cli/main.go](/Users/rowantrollope/git/redis-fs/cli/main.go)
- [cli/mount_backend.go](/Users/rowantrollope/git/redis-fs/cli/mount_backend.go)
- [mount/internal/redisfs/fs.go](/Users/rowantrollope/git/redis-fs/mount/internal/redisfs/fs.go)

## Cache invalidation

The current mount layer already has path and subtree invalidation helpers.

That is enough for a phase 1 session mount, but cache keys must be scoped to:

- workspace
- session
- path

Otherwise two sessions mounted concurrently could poison each other's cache behavior.

## Sandbox Integration

The sandbox does not need a redesign.

The important change is to launch processes against a mounted session path and emit process lifecycle events into the session audit stream.

Suggested additions:

- `sandbox_launch` accepts `workspace` and `session`
- the sandbox mounts or binds the session view at `/workspace`
- process start and exit are audited

Relevant existing code:

- [sandbox/internal/api/mcp_tools.go](/Users/rowantrollope/git/redis-fs/sandbox/internal/api/mcp_tools.go)
- [redisclaw/redisclaw/agent.py](/Users/rowantrollope/git/redis-fs/redisclaw/redisclaw/agent.py)

This also creates a good path for a "just-bash" reference runtime:

- `raf run my-repo --session fix-login -- bash`

## Implementation Phases

## Phase 0: Clarify product and data boundaries

Goal:

- define workspaces, sessions, savepoints, and audit streams in docs and code constants.

Tasks:

- add `workspace` and `session` terminology to README
- add versioned key naming constants
- decide whether "workspace" maps 1:1 with current `redisKey`

Acceptance:

- docs consistently describe the new model
- no user-visible behavior changes yet

## Phase 1: Session metadata and audit streams

Goal:

- ship inspectable sessions before shipping merged mounts.

Tasks:

- implement workspace/session/snapshot metadata records
- add flat CLI verbs like `inspect`, `sessions`, and `drop`
- add Redis Streams audit helpers
- emit audit events for existing native client writes

Code touchpoints:

- [cli/main.go](/Users/rowantrollope/git/redis-fs/cli/main.go)
- `mount/internal/client`
- Python client wrappers

Acceptance:

- users can create and inspect sessions
- all writes against a session-aware client emit audit events
- session metadata survives process restarts

## Phase 2: Read-only session overlay

Goal:

- implement merged reads against snapshot + overlay.

Tasks:

- add `sessionClient`
- implement `Stat`, `Cat`, `Ls`, `LsLong`, `Find`, `Tree`, `Readlink`
- implement whiteout and children delta resolution
- add unit tests for merged listings and whiteouts

Acceptance:

- a session can shadow or hide files from its base snapshot
- merged `ls` and `cat` behave correctly
- FUSE can mount a read-only session

## Phase 3: Writable session overlay

Goal:

- make sessions usable for real agent work.

Tasks:

- implement `Echo`, `Mkdir`, `Rm`, `Touch`, `Chmod`, `Chown`, `Truncate`, `Utimens`
- implement copy-up for inherited files
- implement file rename and limited directory rename
- invalidate session-scoped caches on writes

Acceptance:

- agents can create, modify, and delete files in a session
- base snapshot remains unchanged
- diffs are accurate after writes

## Phase 4: Diff, apply, and savepoint lifecycle

Goal:

- make sessions operationally useful.

Tasks:

- implement `raf diff`
- implement `raf apply`
- implement `raf save` and `raf rollback`
- implement text patch output and JSON summaries

Acceptance:

- a user can take a session and apply it back to a real directory
- savepoints behave predictably and rollback is pointer-based

## Phase 5: Runtime helpers and multi-agent workflows

Goal:

- make the agent workflow feel native.

Tasks:

- add `raf run`
- integrate sandbox process events with audit streams
- support shared mounted sessions and read-only follower sessions
- expose log and timeline views in CLI and MCP

Acceptance:

- agent and human can join the same session
- commands and file changes appear in a coherent timeline

## Phase 6: Data-model evolution

Goal:

- fix the known long-term scaling constraints.

Tasks:

- move toward inode-ID based namespace
- add chunked file storage
- add ranged read/write APIs
- add integrity and repair commands

This phase aligns directly with the existing backlog:

- [BACKLOG.md](/Users/rowantrollope/git/redis-fs/BACKLOG.md#L82)
- [BACKLOG.md](/Users/rowantrollope/git/redis-fs/BACKLOG.md#L88)
- [BACKLOG.md](/Users/rowantrollope/git/redis-fs/BACKLOG.md#L101)

## Recommended First Slice

If we want the highest-leverage first implementation, I would do this:

### Slice A

- session metadata
- workspace metadata
- audit stream helper
- `raf inspect`, `raf sessions`, and `raf drop`

Why:

- visible product progress,
- low risk,
- no FUSE complexity yet,
- lays the foundation for everything else.

### Slice B

- read-only overlay client
- whiteouts
- merged directory listing
- session mount in read-only mode

Why:

- validates the overlay model,
- surfaces path-resolution issues early,
- keeps writes and rename complexity out of the first mount milestone.

### Slice C

- writable overlay for file create/update/delete
- `raf diff`

Why:

- this is the first moment the system becomes truly useful for agents.

## Open Questions

### 1. Should sessions allow deep parent chains?

Recommendation:

- phase 1: no
- each session points to one base snapshot
- `fork` materializes the source effective state into a fresh snapshot

This avoids expensive multi-hop path resolution in the first implementation.

### 2. Should snapshots be materialized or logical?

Recommendation:

- phase 1: materialized snapshots
- phase 2+: logical snapshots with deduplicated content

### 3. How much rename support should phase 1 have?

Recommendation:

- files: yes
- small directories: yes, by materialization
- large inherited trees: document as expensive

### 4. How should conflicts be handled in `apply`?

Recommendation:

- default: fail if target file contents differ from expected base
- allow `--force` later

### 5. Is Redis memory cost acceptable?

Recommendation:

- yes for text/code first
- add quotas and chunked blobs before broad binary/media positioning

## How This Relates to "Just Bash"

The right answer is not "replace bash with filesystem APIs."

The right answer is:

- keep bash,
- keep real tools,
- mount a better workspace under bash.

Redis Agent Filesystem should explicitly support this workflow:

1. fork or choose session
2. mount session
3. run bash or agent against it
4. inspect diff and audit log
5. apply back to Git worktree

That gives the ergonomics of a real shell and the control plane of an agent-native workspace system.

## Proposed Near-Term Engineering Backlog

1. Add workspace/session metadata package and Redis key constants.
2. Add audit stream writer and wire it into current write paths.
3. Add `raf inspect`, `raf sessions`, `raf saves`, and `raf drop`.
4. Add session-aware client with read-only merged resolution.
5. Add read-only FUSE mount for sessions.
6. Add writable overlay for file create/update/delete.
7. Add `raf diff`.
8. Add `raf apply`.
9. Add `raf save`, `raf rollback`, and internal savepoint snapshots.
10. Add `raf run` as the reference shell/agent entrypoint.

## File Touchpoints for Phase 1

The most likely first code touchpoints are:

- [cli/main.go](/Users/rowantrollope/git/redis-fs/cli/main.go)
- [cli/mount_backend.go](/Users/rowantrollope/git/redis-fs/cli/mount_backend.go)
- [mount/internal/client/client.go](/Users/rowantrollope/git/redis-fs/mount/internal/client/client.go)
- [mount/internal/client/native.go](/Users/rowantrollope/git/redis-fs/mount/internal/client/native.go)
- [mount/internal/client/keys.go](/Users/rowantrollope/git/redis-fs/mount/internal/client/keys.go)
- [mount/internal/redisfs/fs.go](/Users/rowantrollope/git/redis-fs/mount/internal/redisfs/fs.go)
- [sandbox/internal/api/mcp_tools.go](/Users/rowantrollope/git/redis-fs/sandbox/internal/api/mcp_tools.go)
- [redisclaw/redisclaw/agent.py](/Users/rowantrollope/git/redis-fs/redisclaw/redisclaw/agent.py)

## Bottom Line

The shortest path to a serious competitive answer is:

- do not build a new generic sandbox platform from scratch,
- do not abandon shell-first workflows,
- add sessions, savepoints, overlays, diffs, and audit on top of the existing Redis-backed filesystem.

That would make Redis Agent Filesystem meaningfully closer to an AgentFS-class product while still preserving the shared-state and multi-access strengths that make Redis interesting in the first place.
