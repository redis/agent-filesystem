# AFS Versioned Filesystem v1.1 PRD

Last reviewed: 2026-04-29.
Status: draft for review.

## Summary

AFS v1.1 should add versioned-filesystem behavior to the product without
turning AFS into Git.

The core product decision is:

- AFS versions filesystem state, not source-code commits.
- Every file write is recorded in a mutation journal.
- User-visible versions are created at meaningful boundaries, not on every
  write.
- Checkpoints remain the familiar CLI concept and become the user-facing form
  of saved filesystem versions.
- Git may be an import/export or code-workspace integration later, but it is
  not the canonical storage or versioning model for v1.1.

The v1.1 goal is to make agent work on Markdown, docs, memory files, plans,
assets, and arbitrary workspace state recoverable, inspectable, and easy to
review while preserving the current AFS strengths: normal local files, MCP
tools, sync/mount surfaces, Redis-backed durability, and standalone,
self-managed, and cloud-hosted operation.

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
workspace timelines. AFS should not copy a full Git/JJ-style change graph as
the default user model.

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

- File writes update the live workspace first and may remain dirty until a
  checkpoint is created.
- Users can create checkpoints, but they do not yet have a full "version
  history" experience.
- Diffing saved states is not a first-class API, CLI, or UI flow.
- Restore is available, but there is no strong safety flow for previewing or
  preserving the current live state before rollback.
- Changelog and checkpoint data are adjacent, but not yet presented as one
  cohesive timeline.
- Agent sessions do not naturally create reviewable version boundaries.

## Goals

1. Make AFS feel like a versioned filesystem for agent workspaces.
2. Preserve the existing workspace/checkpoint mental model.
3. Record every write in an append-only mutation history.
4. Create user-visible versions only at meaningful boundaries.
5. Support fast diffs between live state, checkpoints, and saved versions.
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
- **Checkpoint**: a named saved version of the workspace.
- **History**: the timeline of checkpoints, restores, forks, sessions, and file
  changes.
- **Diff**: comparison between two workspace states.
- **Fork**: an isolated workspace timeline for parallel work.

Internal implementation can use richer names, but the product should avoid
asking users to learn Git-like concepts for v1.1.

## Versioning Model

### Live Writes

File writes continue to update the live workspace state immediately.

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

This is history, not a user-visible version boundary.

### Checkpoints As Versions

A checkpoint is an immutable saved filesystem state.

Checkpoints should be enhanced with enough metadata to render a version
timeline:

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

Existing checkpoint commands remain valid. UI may label the section "Versions"
while still using checkpoint semantics under the hood.

### Boundary Rules

AFS should not create a checkpoint for every write.

Version boundaries should be created by:

- explicit `afs checkpoint create`
- explicit MCP `checkpoint_create`
- browser/UI "Create checkpoint"
- before destructive restore, when live state has uncheckpointed changes
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

### P0: Version History

Users can list checkpoint/version history in CLI, UI, and API.

Requirements:

- Show current head.
- Show whether live state is dirty relative to head.
- Show version kind, timestamp, actor, session, file counts, and byte totals.
- Preserve existing `checkpoint list` output.
- Add richer detail in JSON/API responses without breaking existing clients.

Acceptance criteria:

- `afs checkpoint list` still works.
- UI workspace detail has a clear Versions or History section.
- API can return version metadata without per-row Redis fanout where practical.

### P0: Diff Between States

Users can compare:

- checkpoint to checkpoint
- checkpoint to live state
- live state to current head
- fork base to fork head, when fork metadata is available

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

- Restoring a checkpoint overwrites live state, as today.
- If live state is dirty, AFS creates a safety checkpoint first by default.
- Restore records a lifecycle event and file change events.
- Restore UI shows a preview diff before the destructive action.
- CLI restore says exactly what will happen and which safety checkpoint was
  created.

Acceptance criteria:

- Dirty live state is not lost without a recoverable checkpoint.
- `checkpoint restore` remains scriptable.
- Restore errors are clear when the checkpoint does not exist or a conflict is
  detected.

### P0: Unified History Foundation

Versioning should build on the event-history merge plan rather than creating a
third history system.

Requirements:

- Checkpoint/version lifecycle events and file mutation events are queryable
  from one history surface.
- File events can be filtered out by default to avoid noisy UI.
- Session rows can expand to reveal related file changes.
- Checkpoint rows can expand to reveal changed paths.

Acceptance criteria:

- Version timeline and activity/history UI use the same backend event model.
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
- UI can compare fork head to fork base.
- Users can keep, delete, or manually apply fork results.
- A first "accept fork result" flow may replace parent live state from fork
  head only after an explicit preview and safety checkpoint.

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

Prefer adding version-oriented aliases while preserving checkpoint routes.

Potential endpoints:

```text
GET  /v1/workspaces/{workspace_id}/versions
GET  /v1/workspaces/{workspace_id}/versions/{version_id}
POST /v1/workspaces/{workspace_id}/versions
GET  /v1/workspaces/{workspace_id}/diff?base=<id|head|live>&head=<id|head|live>
POST /v1/workspaces/{workspace_id}:restore
GET  /v1/workspaces/{workspace_id}/events
GET  /v1/workspaces/{workspace_id}/paths/history?path=/notes/foo.md
```

Existing checkpoint endpoints remain supported:

```text
GET  /v1/workspaces/{workspace_id}/checkpoints
POST /v1/workspaces/{workspace_id}/checkpoints
POST /v1/workspaces/{workspace_id}:restore
```

The version endpoints can initially be thin aliases over checkpoint service
methods with richer DTO names.

## CLI Shape

Do not remove existing checkpoint commands.

P0 additions:

```bash
afs checkpoint diff [workspace] <base> <head>
afs checkpoint diff [workspace] <checkpoint> --live
afs checkpoint show [workspace] <checkpoint>
```

P1 additions:

```bash
afs history [workspace] [--files] [--session <id>] [--since <duration>]
afs path history [workspace] <path>
```

Open naming question:

- Should the CLI introduce `afs version ...` as a friendlier alias, or should
  it keep the existing `checkpoint` language to avoid expanding the command
  surface?

Recommendation for v1.1:

- Keep `checkpoint` in the CLI.
- Use "Versions" in the UI where it helps product comprehension.

## UI Shape

Workspace detail should gain a cohesive versioning surface.

### Versions Tab

Shows:

- current live state status: clean or dirty
- current head checkpoint
- checkpoint/version list
- kind, actor, time, file count, byte count
- actions: view, diff, restore

### Diff View

Shows:

- base and head selectors
- changed file list
- text diff for supported files
- binary/large-file summary
- restore or checkpoint actions when relevant

### Restore Flow

Shows:

- warning that restore updates live state
- preview of changes from live to target checkpoint
- safety checkpoint name when dirty state exists
- explicit confirmation

### History Tab Alignment

The existing event-history merge should feed the version timeline where
possible. Checkpoint rows should expand into file changes, and session rows
should expand into the file events they produced.

## MCP Behavior

MCP file tools should continue to write live state.

Versioning behavior:

- `checkpoint_create` creates a version boundary.
- File mutation tools append file events.
- Workspace profiles that include checkpoint permission can create explicit
  task or milestone checkpoints.
- Future opt-in policy can checkpoint on session close.

Do not create a checkpoint for every MCP file write.

## Storage And Implementation Notes

Use the current Redis-backed manifest/blob/checkpoint model as the foundation.

Likely implementation approach:

1. Extend checkpoint metadata rather than inventing a separate version store.
2. Add richer version DTOs in `internal/controlplane`.
3. Add diff service methods that compare manifests.
4. Add text diff generation with size and binary guards.
5. Add safety-checkpoint behavior around restore.
6. Connect version lifecycle events to the unified events plan.
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

### Phase 0: Contract Review

- Review this PRD.
- Decide CLI naming: checkpoint-only vs version aliases.
- Decide default auto-checkpoint policy.
- Decide restore safety behavior and confirmation wording.

### Phase 1: Backend Version DTOs And Diff

- Add version DTOs backed by existing checkpoints.
- Add manifest diff service method.
- Add API route for diff.
- Add tests for checkpoint-to-checkpoint and checkpoint-to-live diff.
- Add binary and size guards.

### Phase 2: CLI Diff And Show

- Add `checkpoint show`.
- Add `checkpoint diff`.
- Keep output concise and scriptable.
- Add JSON output if current command conventions support it.

### Phase 3: Safe Restore

- Detect dirty live state before restore.
- Create safety checkpoint by default.
- Emit clear audit/event rows.
- Update CLI and API tests.

### Phase 4: UI Versions And Diff

- Add Versions tab or section.
- Add diff view.
- Add restore preview flow.
- Keep layout aligned with current Redis UI shell.

### Phase 5: History Integration

- Wire checkpoint/version rows into unified history.
- Add expandable checkpoint rows.
- Add path-history follow-up if event indexing is ready.

### Phase 6: Fork Review

- Add fork base metadata.
- Add compare fork to source base.
- Add explicit accept/reject/delete UX.
- Defer automatic merge until there is strong product demand.

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

### Risk: Too Many Versions

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

### Risk: Confusing Checkpoint Vs Version Language

Mitigation:

- Treat checkpoint as the CLI/storage noun.
- Treat version as the UI/product framing.
- Do not introduce more nouns until needed.

## Open Questions For Review

1. Should the CLI add `afs version` aliases, or should v1.1 keep only
   `checkpoint` commands?
2. Should safety checkpoints before restore be mandatory, default-on with
   `--no-safety-checkpoint`, or prompted?
3. Should session-close auto-checkpoints ship in v1.1, or wait until after
   manual version history and diff are live?
4. What is the first text-diff size limit?
5. Should fork acceptance replace parent live state in v1.1, or should fork
   review stop at compare/delete/manual copy?
6. Should path history be part of v1.1, or a follow-up after unified events?

## Recommended V1.1 Cut

Ship this first:

1. Version list backed by checkpoints.
2. Checkpoint detail metadata.
3. Diff between checkpoint/live states.
4. CLI `checkpoint diff`.
5. UI Versions tab and diff view.
6. Safety checkpoint before restore.
7. Event/history integration for checkpoint rows.

Defer:

- Git integration.
- Custom branch/bookmark model.
- Automatic merge.
- Per-write versions.
- Full path-history UI.

