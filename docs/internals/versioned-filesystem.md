# Versioned Filesystem

Last reviewed: 2026-05-02.

AFS versions filesystem state, not source-code commits. Checkpoints are the
user-facing saved-state primitive.

## Current Model

- **Workspace**: durable filesystem tree.
- **Live state**: current mutable workspace contents.
- **Checkpoint**: immutable saved filesystem state.
- **History**: timeline of checkpoints, restores, sessions, and file changes.
- **Diff**: comparison between two workspace states.
- **Fork**: isolated workspace timeline for parallel work.

AFS is not Git. It does not expose staging, commits for every write, remotes,
rebases, blame, or branch workflows as the default product model.

## Implemented

- Manual checkpoints through CLI, UI, MCP, and API.
- Checkpoint descriptions and structured metadata.
- Checkpoint detail APIs.
- Checkpoint-to-checkpoint and checkpoint-to-live diffs.
- Bounded text hunks for supported UTF-8 files.
- Safe restore that creates a safety checkpoint when active state has
  uncheckpointed changes.
- Restore invalidation/rematerialization for sync clients.
- Unified workspace History surface backed by `workspace:events`.
- File history UI in the Browse drawer through event path filtering.
- File versioning policy APIs, CLI/MCP tools, and workspace settings UI.
- Versioned file read/diff/restore/undelete surfaces.

## Open Work

Keep future items in [plans/future-work.md](../../plans/future-work.md). The main known follow
ups are:

- session-boundary auto-checkpoints
- fork review and accept/reject flow
- CLI `afs history` and `afs path history`
- indexed path-history endpoint if current event filtering becomes too slow

## Product Rules

- File writes update live state immediately.
- AFS records file mutations in append-only history.
- AFS does not create a user-visible checkpoint for every write.
- Restore creates a new event and may create a safety checkpoint; it does not
  rewrite history.
- Older file versions are accessed explicitly through APIs/tools, never through
  the normal live filesystem path.
