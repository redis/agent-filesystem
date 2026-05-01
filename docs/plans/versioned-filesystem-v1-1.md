# AFS Versioned Filesystem v1.1 PRD

Last reviewed: 2026-04-30.
Status: implementation in progress.

## Summary

AFS v1.1 should add versioned-filesystem behavior to the product without
turning AFS into Git.

The core product decision is:

- AFS versions filesystem state, not source-code commits.
- Every file write is recorded in a mutation journal.
- User-visible checkpoints are created at meaningful boundaries, not on every
  write.
- Checkpoint is the primary product noun for saved filesystem state.
- "Versioned filesystem" is the category and positioning, not the everyday UX
  noun.
- Git may be an import/export or code-workspace integration later, but it is
  not the canonical storage or versioning model for v1.1.

The v1.1 goal is to make agent work on Markdown, docs, memory files, plans,
assets, and arbitrary workspace state recoverable, inspectable, and easy to
review while preserving the current AFS strengths: normal local files, MCP
tools, sync/mount surfaces, Redis-backed durability, and standalone,
self-managed, and cloud-hosted operation.

## Current Implementation State

Snapshot: 2026-04-30, current local branch/worktree.

Implemented in the current branch:

- Public product language has moved to checkpoint-only UX. `version` remains a
  category/positioning word, not a CLI/API/UI noun.
- `afs cp create` supports a checkpoint description, and restore
  safety checkpoints use structured metadata.
- `afs cp list` uses active-workspace language instead of head/draft
  language.
- Manifest-level diff exists in `internal/controlplane` and is exposed through
  the workspace diff API. It compares checkpoint-to-checkpoint,
  checkpoint-to-active, and active-to-checkpoint states at the file/metadata
  level.
- `afs cp diff [workspace] <base> <target>` and
  `afs cp diff [workspace] <checkpoint> --active` are implemented.
- `afs cp show [workspace] <checkpoint>` is implemented with
  human-readable default output and `--json` for structured agent use.
- `checkpoint diff --json` is implemented for agents, while default CLI diff
  remains human-readable and includes bounded text hunks when available.
- `GET /v1/workspaces/{workspace_id}/checkpoints/{checkpoint_id}` and its
  database-scoped equivalent expose complete checkpoint detail metadata.
- Diff responses include text diffs for UTF-8 file content under 256 KiB and
  4000 combined lines; binary/oversized content returns a skip reason.
- Checkpoint audit rows and file changelog rows now dual-write to
  `workspace:events`, exposed through workspace, database, and global event
  API routes.
- The UI Checkpoints tab has compare-with-active and restore-preview flows
  backed by the diff API, including inline text hunks for supported files.
- Checkpoint rows in the UI expand to show details and changed paths.
- The workspace History tab is the single workspace-level history surface. It
  keeps the file changelog table UX and merges in non-file lifecycle events
  from the unified event stream.
- The Browse drawer has a first path-history surface that filters recent events
  for the selected path and links checkpoint-backed rows back to Checkpoints.
- Restore creates a safety checkpoint when active state has uncheckpointed
  changes, returns the safety checkpoint in the restore result, writes restore
  changelog/audit data, and publishes a root-replace invalidation.
- Sync restore no longer depends only on an async invalidation event. CLI
  restore now checks for open handles, stops the managed sync daemon, restores
  server state, removes local sync state, and restarts sync so the restored
  active workspace rematerializes cleanly.
- Root-replace invalidation is available to mount clients and FUSE cache
  invalidation.

Partially implemented:

- Restore is safe and scriptable in the CLI. A local control-plane smoke with
  an isolated Redis server and temp sync folder verifies checkpoint create,
  show, diff JSON, restore, event API rows, and sync-folder rematerialization.
- Path history is available in the UI through the existing event API's exact
  `path` filter. A dedicated indexed path-history endpoint can wait until query
  volume proves it is needed.

Not implemented yet:

- Session-boundary auto-checkpoints.
- Fork review and accept/reject flow.
- CLI `afs history` and `afs path history` commands.

## Product Thesis

Agents should be able to work in a normal filesystem and users should be able
to ask:

- What changed?
- Who or what changed it?
- What did the workspace look like before that task?
- Can I compare two states?
- Can I roll back safely?
- Can I fork a line of work and choose the better result?

Those are versioned-filesystem questions. They do not require Git semantics
such as staging, commits per write, branches, remotes, rebases, tags, or pull
requests.

AFS should copy the useful versioned-filesystem ideas from Mesa-style products:
durable history, diffs across states, rollback, checkpoints, and parallel
workspace timelines. The public language should stay aligned with agent
ecosystem usage: checkpoints are saved recoverable states. AFS should not copy
a full Git/JJ-style change graph as the default user model.

## Problem

Today AFS has the right foundation:

- Redis stores workspace metadata, manifests, blobs, checkpoints, and live
  workspace roots.
- Sync and live mount make AFS feel like a normal directory.
- MCP tools let agents read and write workspace files.
- Checkpoints save explicit restore points.
- Workspace forks support parallel work.
- Activity and changelog streams record important operations.

The gap is product coherence around versioning:

- File writes update the active workspace first and may remain dirty until a
  checkpoint is created.
- Users can create checkpoints, but they do not yet have a full checkpoint
  history and compare experience.
- Diffing saved states is not a first-class API, CLI, or UI flow.
- Restore is available, but there is no strong safety flow for previewing or
  preserving the current active state before rollback.
- Changelog and checkpoint data are adjacent, but not yet presented as one
  cohesive timeline.
- Agent sessions do not naturally create reviewable checkpoint boundaries.

## Goals

1. Make AFS feel like a versioned filesystem for agent workspaces.
2. Preserve the existing workspace/checkpoint mental model.
3. Record every write in an append-only mutation history.
4. Create user-visible checkpoints only at meaningful boundaries.
5. Support fast diffs between active state and checkpoints.
6. Make restore safe by default.
7. Keep the implementation aligned with the current Redis manifest/blob model.
8. Keep standalone, self-managed, and cloud modes behaviorally consistent.
9. Avoid introducing Git-shaped product concepts unless they solve a real AFS
   problem.

## Non-Goals

- No Git server.
- No Git-compatible storage layer.
- No commits for every file write.
- No staging area.
- No default branch/bookmark/change vocabulary.
- No rebase, cherry-pick, blame, tags, remotes, or pull-request clone.
- No full CRDT or collaborative editor semantics.
- No claim of complete POSIX parity beyond what the mount layer can prove.
- No line-level merge engine in the first versioning release.

## User Model

The user-facing nouns should stay simple:

- **Workspace**: a durable filesystem tree.
- **Live state**: the current mutable workspace contents.
- **Checkpoint**: a named saved filesystem state.
- **History**: the timeline of checkpoints, restores, forks, sessions, and file
  changes.
- **Diff**: comparison between two workspace states.
- **Fork**: an isolated workspace timeline for parallel work.

Internal implementation can use richer names, but the product should avoid
asking users to learn Git-like concepts for v1.1.

## Versioning Model

### Active Writes

File writes continue to update the active workspace state immediately.

Every write should append a mutation event containing:

- workspace id
- session id when known
- agent id/name when known
- user/principal when known
- operation: put, delete, mkdir, rmdir, symlink, chmod, rename
- path and previous path for renames
- previous content hash where available
- new content hash where available
- size and delta bytes
- timestamp
- source: sync, mount, MCP, browser, import, restore, checkpoint

This is history, not a user-visible checkpoint boundary.

### Checkpoints As Saved States

A checkpoint is an immutable saved filesystem state.

Checkpoints should be enhanced with enough metadata to render a useful
checkpoint timeline:

- id
- display name
- description/message
- kind: manual, session, safety, import, fork, restore, system
- parent checkpoint id
- created by: user, agent, session, system
- created at
- file count, folder count, total bytes
- manifest hash
- source session id when applicable
- source agent id/name when applicable

Existing checkpoint commands remain valid. The UI should also use checkpoint
language so the product matches how agent tools discuss saved recoverable
state.

### Boundary Rules

AFS should not create a checkpoint for every write.

Checkpoint boundaries should be created by:

- explicit `afs cp create`
- explicit MCP `checkpoint_create`
- browser/UI "Create checkpoint"
- before destructive restore, when active state has uncheckpointed changes
- after import or quickstart seed
- at configured agent/session boundaries, when enabled
- at template or workflow milestones, when the template asks for it

The default v1.1 behavior should be conservative:

- Manual checkpoints remain the default.
- Safety checkpoints before restore are on by default.
- Session-boundary auto-checkpoints are opt-in until the UX is proven.

### Timeline Shape

The default workspace timeline is linear:

```text
initial -> checkpoint-a -> checkpoint-b -> checkpoint-c
```

Forks create a new workspace timeline:

```text
main-workspace: initial -> before-agent -> accepted-result
                         \
forked-workspace:          fork-base -> experiment-result
```

This is enough for v1.1. A DAG inside a single workspace is not required.

## Product Requirements

### P0: Checkpoint History

Users can list checkpoint history in CLI, UI, and API.

Requirements:

- Show active workspace state.
- Show whether active state has uncheckpointed changes.
- Show checkpoint kind, timestamp, actor, session, file counts, and byte totals.
- Preserve existing `checkpoint list` output.
- Add richer detail in JSON/API responses without breaking existing clients.

Acceptance criteria:

- `afs cp list` still works.
- UI workspace detail has a clear Checkpoints or History section.
- API can return checkpoint metadata without per-row Redis fanout where
  practical.

### P0: Diff Between States

Users can compare:

- checkpoint to checkpoint
- checkpoint to active state
- active state to a checkpoint
- fork base to fork result, when fork metadata is available

Initial diff levels:

1. File-level diff: added, modified, deleted, renamed when detectable.
2. Metadata diff: mode, size, symlink target.
3. Text diff for UTF-8 files under a safe size limit.
4. Binary summary for non-text or large files.

Acceptance criteria:

- API returns stable structured diff data.
- CLI can print a concise file-level diff.
- UI can render a reviewable file list and basic text diff.
- Large/binary files do not crash or block the diff path.

### P0: Safe Restore

Restore should be hard to misuse.

Requirements:

- Restoring a checkpoint overwrites active state, as today.
- If active state is dirty, AFS creates a safety checkpoint first by default.
- Restore records a lifecycle event and file change events.
- Restore UI shows a preview diff before the destructive action.
- CLI restore says exactly what will happen and which safety checkpoint was
  created.

Acceptance criteria:

- Dirty active state is not lost without a recoverable checkpoint.
- `checkpoint restore` remains scriptable.
- Restore errors are clear when the checkpoint does not exist or a conflict is
  detected.

### P0: Unified History Foundation

Checkpoint-backed versioning should build on the event-history merge plan
rather than creating a third history system.

Requirements:

- Checkpoint lifecycle events and file mutation events are queryable from one
  history surface.
- File events can be filtered out by default to avoid noisy UI.
- Session rows can expand to reveal related file changes.
- Checkpoint rows can expand to reveal changed paths.

Acceptance criteria:

- Checkpoint timeline and activity/history UI use the same backend event model.
- Existing `/activity` and `/changes` readers can remain aliases during
  migration.

### P1: Session Boundary Checkpoints

AFS can create checkpoints around agent sessions or task boundaries.

Candidate policies:

- `manual`: never create automatic checkpoints.
- `on-session-close`: checkpoint if dirty when an agent session ends.
- `on-prompt`: external orchestrator calls checkpoint at prompt boundaries.
- `on-milestone`: templates or skills call checkpoint explicitly.

Recommended v1.1 default:

- `manual` plus restore safety checkpoints.
- Expose session auto-checkpoints as opt-in configuration.

Acceptance criteria:

- Users can enable auto-checkpointing per workspace or token/profile.
- Auto-checkpoints have recognizable names and metadata.
- No per-write checkpoint noise.

### P1: Fork Review Flow

Workspace forks should become the parallel-work primitive, not Git-like
branches.

Requirements:

- Fork metadata records source workspace and base checkpoint.
- UI can compare fork result to fork base.
- Users can keep, delete, or manually apply fork results.
- A first "accept fork result" flow may replace parent active state from fork
  result only after an explicit preview and safety checkpoint.

Non-goal for v1.1:

- Automatic three-way merge across forked workspaces.

Acceptance criteria:

- Parallel agents can work in separate workspaces.
- The user can compare outputs and choose a winner.
- No new branch/bookmark model is needed.

### P2: Path History

Users can inspect path-level history.

Requirements:

- For a selected path, show recent mutation events.
- Show the agent/session/user that last touched the path.
- Link path events back to checkpoints and sessions where possible.

Acceptance criteria:

- Path history works for common Markdown/doc workflows.
- Query cost is bounded by an index or a documented scan limit.

## API Shape

Keep checkpoint as the public API noun for v1.1.

Potential endpoints:

```text
GET  /v1/workspaces/{workspace_id}/checkpoints
GET  /v1/workspaces/{workspace_id}/checkpoints/{checkpoint_id}
POST /v1/workspaces/{workspace_id}/checkpoints
GET  /v1/workspaces/{workspace_id}/diff?base=<id|active>&target=<id|active>
POST /v1/workspaces/{workspace_id}:restore
GET  /v1/workspaces/{workspace_id}/events
GET  /v1/workspaces/{workspace_id}/events?path=/notes/foo.md
```

No `/versions` alias is planned for the first release. It adds another noun
without improving the agent-facing model.

A dedicated `/paths/history` endpoint is not required for the first UI slice.
The existing event endpoint already accepts `path`, `session_id`, `kind`, and
time bounds. Add a path-history-specific endpoint only if we need a different
indexing strategy or response shape.

## CLI Shape

Do not remove existing checkpoint commands.

P0 additions:

```bash
afs cp diff [workspace] <base> <target>
afs cp diff [workspace] <checkpoint> --active
afs cp show [workspace] <checkpoint>
```

P1 additions:

```bash
afs history [workspace] [--files] [--session <id>] [--since <duration>]
afs path history [workspace] <path>
```

Recommendation for v1.1:

- Keep `checkpoint` in the CLI.
- Do not add `afs version` aliases.
- Explain checkpoint as a saved filesystem state in help text.
- Keep history/path-history CLI commands behind a real agent workflow need; the
  API/UI event stream now covers the first review loop.

## UI Shape

Workspace detail should gain a cohesive checkpointing surface.

### Checkpoints Tab

Shows:

- active state status: clean or dirty
- active workspace status
- checkpoint list
- kind, actor, time, file count, byte count
- actions: view, diff, restore

### Diff View

Shows:

- base and target selectors
- changed file list
- text diff for supported files
- binary/large-file summary
- restore or checkpoint actions when relevant

### Restore Flow

Shows:

- warning that restore updates active state
- preview of changes from active to target checkpoint
- safety checkpoint name when dirty state exists
- explicit confirmation

### History Tab Alignment

The existing event-history merge should feed the checkpoint timeline where
possible. Checkpoint rows should expand into file changes, and session rows
should expand into the file events they produced.

For the current v1.1 UI slice:

- Workspace detail uses a visible **History** tab. It merges file changelog rows
  with non-file lifecycle rows from `workspace:events`.
- The visible **Changelog** tab is removed. The route/search value can remain
  `changes` for compatibility, and legacy `activity` links should normalize to
  the same History view.
- Browse file drawers expose a compact Path history panel for the selected
  path, backed by `events?path=...`.
- The global Activity page keeps Changelog first and exposes Events as the
  broader operational feed.

## MCP Behavior

MCP file tools should continue to write active state.

Checkpointing behavior:

- `checkpoint_create` creates a saved-state boundary.
- File mutation tools append file events.
- Workspace profiles that include checkpoint permission can create explicit
  task or milestone checkpoints.
- Future opt-in policy can checkpoint on session close.

Do not create a checkpoint for every MCP file write.

## Storage And Implementation Notes

Use the current Redis-backed manifest/blob/checkpoint model as the foundation.

Likely implementation approach:

1. Extend checkpoint metadata rather than inventing a separate version store.
2. Add richer checkpoint DTOs in `internal/controlplane`.
3. Add diff service methods that compare manifests.
4. Add text diff generation with size and binary guards.
5. Add safety-checkpoint behavior around restore.
6. Connect checkpoint lifecycle events to the unified events plan.
7. Add UI/API/CLI surfaces after backend contracts are stable.

Potential metadata additions:

```go
Kind           string
ParentID       string
Description    string
Source         string
SessionID      string
AgentID        string
AgentName      string
CreatedBy      string
BaseCheckpoint string
```

Potential Redis keys can remain mostly unchanged:

```text
afs:{ws}:workspace:meta
afs:{ws}:savepoints
afs:{ws}:savepoint:{id}:meta
afs:{ws}:savepoint:{id}:manifest
afs:{ws}:workspace:events
afs:{ws}:path:last:{path}
```

This keeps the v1.1 implementation close to the current code and avoids a
storage rewrite.

## Rollout Plan

### Phase 0: Contract Review - mostly complete

- [x] Review this PRD.
- [x] Confirm checkpoint-only public naming.
- [x] Decide default auto-checkpoint policy: manual by default, safety
  checkpoint before restore.
- [x] Decide restore safety behavior: default-on safety checkpoint plus explicit
  restore action.
- [ ] Final pass on CLI/UI confirmation wording after end-to-end restore smoke
  testing.

### Phase 1: Backend Checkpoint DTOs And Diff - complete

- [x] Add complete richer checkpoint DTOs backed by existing savepoints.
- [x] Add manifest diff service method.
- [x] Add API route for diff.
- [x] Add tests for checkpoint-to-checkpoint and checkpoint-to-active diff.
- [x] Add text diff generation with binary and size guards.

### Phase 2: CLI Diff And Show - complete

- [x] Add `checkpoint show`.
- [x] Add `checkpoint diff`.
- [x] Keep output concise and scriptable.
- [x] Add JSON output for `checkpoint diff` and `checkpoint show`.

### Phase 3: Safe Restore - mostly complete

- [x] Detect dirty active state before restore.
- [x] Create safety checkpoint by default.
- [x] Emit audit/event rows and restore changelog rows.
- [x] Publish root-replace invalidation after restore.
- [x] Coordinate CLI sync restore by stopping/restarting the managed sync
  daemon and rematerializing the local folder.
- [x] Update CLI and API tests.
- [x] Run a final manual restore smoke test against a local control plane and
  isolated temp sync folder.

### Phase 4: UI Checkpoints And Diff - complete

- [x] Redesign the existing Checkpoints tab around checkpoint history.
- [x] Add file-level diff view.
- [x] Add restore preview flow.
- [x] Add text diff view for supported content diffs.
- [x] Run browser QA against the Redis UI shell for checkpoint expand/collapse
  and diff flows.

### Phase 5: History Integration - mostly complete

- [x] Wire checkpoint and file rows into unified event storage/API.
- [x] Add expandable checkpoint rows.
- [x] Move workspace detail History to the unified event API.
- [x] Add first path-history UI using the existing event `path` filter.
- [ ] Add CLI history/path-history only if the UI/API loop exposes a concrete
  agent workflow need.
- [ ] Revisit path indexing if exact path filtering becomes too expensive.

### Phase 6: Fork Review - not started

- [ ] Add fork base metadata.
- [ ] Add compare fork to source base.
- [ ] Add explicit accept/reject/delete UX.
- [ ] Defer automatic merge until there is strong product demand.

## Success Metrics

Product:

- Users can explain how to recover from a bad agent run.
- Users can compare before/after states without leaving AFS.
- Agents can work on Markdown/doc workspaces without Git ceremony.
- Restore feels safe instead of scary.

Operational:

- Checkpoint creation scales with changed paths where dirty-path tracking is
  available.
- Diff API latency is acceptable for typical Markdown/doc workspaces.
- Large/binary files do not dominate diff latency.
- Existing checkpoint and restore tests continue to pass.

Adoption:

- More workspaces have recent checkpoints.
- Users use diff before restore.
- Session or milestone checkpointing is enabled in templates where it makes
  sense.

## Risks

### Risk: Rebuilding Git

Mitigation:

- Keep timeline linear inside a workspace for v1.1.
- Use workspace forks for parallelism.
- Do not add branches/bookmarks/rebase/merge unless a concrete AFS workflow
  requires them.

### Risk: Too Many Checkpoints

Mitigation:

- Do not checkpoint per write.
- Keep automatic checkpointing opt-in.
- Add retention/cleanup policy later if needed.

### Risk: Restore Data Loss

Mitigation:

- Create safety checkpoint before restore when dirty.
- Show preview diff in UI.
- Keep restore events append-only.

### Risk: Diff Cost On Large Workspaces

Mitigation:

- Start with manifest-level diff.
- Generate content diffs only for text files under a limit.
- Use dirty-path metadata as it lands.

### Risk: Confusing Checkpoint And Versioning Language

Mitigation:

- Treat checkpoint as the CLI, API, MCP, UI, and docs noun.
- Treat versioned filesystem as product/category positioning.
- Do not introduce more nouns until needed.

## Open Questions For Review

Resolved during implementation:

- Public naming is checkpoint-only for CLI, API, MCP, UI, and docs.
- Safety checkpoints before restore are default-on.
- Automatic session checkpoints remain opt-in.
- Text diff limits are 256 KiB and 4000 combined lines for v1.1.
- `checkpoint diff` and `checkpoint show` default to human-readable output and
  support `--json` for agents.

Still open:

1. Should session-close auto-checkpoints ship in v1.1, or wait until after
   manual checkpoint history and diff are live?
2. Should fork acceptance replace parent active state in v1.1, or should fork
   review stop at compare/delete/manual copy?
3. Should history/path-history CLI commands ship in v1.1, or wait until a real
   agent/operator workflow needs them?

## Recommended V1.1 Cut

Already covered in the current branch:

1. Checkpoint history backed by existing savepoints.
2. Manifest-level diff between checkpoint and active states.
3. CLI `checkpoint diff`.
4. UI Checkpoints tab redesign and file-level diff/restore preview.
5. Safety checkpoint before restore.
6. Root-replace invalidation and local sync rematerialization after restore.
7. Checkpoint detail API and CLI `checkpoint show`.
8. Human-readable default plus `--json` for checkpoint diff/show.
9. Bounded text diff hunks for API/CLI.
10. Unified event stream API for checkpoint and file rows.

Remaining before calling v1.1 complete:

1. Run browser-level visual QA for the new History tab and file drawer Path
   history panel.
2. Decide whether the CLI needs `afs history` / `afs path history` in v1.1 or
   whether the event API plus UI covers the first release.

Defer:

- Git integration.
- Custom branch/bookmark model.
- Automatic merge.
- Per-write checkpoints.
- Session-boundary auto-checkpoints unless a concrete workflow needs them.
- Fork accept/reject flow unless the fork review work becomes urgent.
- Full path-history UI.
