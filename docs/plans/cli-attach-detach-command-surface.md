# AFS CLI Attach/Detach Command Surface Plan

Last reviewed: 2026-04-30.
Status: draft for review.

## Summary

AFS should make the local lifecycle feel like attaching a durable remote
workspace to a normal local directory, then detaching it when work is done.

The accepted vocabulary is `afs ws attach` and `afs ws detach`. The
user-facing model is:

- A workspace is a durable remote filesystem tree.
- An attached directory is the local working surface for that workspace.
- Detach stops AFS from managing the local surface.
- Deleting local files is an explicit opt-in, not the default detach behavior.

This plan focuses first on the CLI command surface and the transition to
attach/detach language. Remote filesystem inspection, tags, richer logs, and
multi-attach support are follow-up surfaces.

## Product Principles

1. Prefer user-goal verbs over implementation nouns.
2. Keep `workspace` as the canonical product noun.
3. Do not make users understand databases for normal workspace operations.
4. Do not hide obscure aliases. If `ws` exists, it must be documented as a real
   command group.
5. Preserve local files by default. A destructive local cleanup must require an
   explicit `--delete`.
6. Keep output boring and scriptable for lifecycle commands.
7. Keep implementation names such as sync, mount, session, Redis key, and
   database ID out of the happy-path CLI unless they are needed for debugging.

## Current State

The target CLI surface has:

```bash
afs ws attach [workspace] [directory]
afs ws detach
afs ws import [--force] [--attach-at-source] [--database <database>] <workspace> <directory>
afs status
afs log
afs fs grep
```

Important current behavior:

- `afs ws attach` starts the local surface for an explicit workspace.
- `afs ws detach` stops managing the attached local folder.
- Sync-mode detach preserves local files by default.
- Import with `--attach-at-source` attaches the source directory after import.
- The attachment registry supports multiple workspace/folder attachments.

## Target Command Shape

Recommended canonical commands:

```bash
afs ws list
afs ws create <workspace>
afs ws delete <workspace>
afs ws attach <workspace> <directory> [--verbose]
afs ws detach <directory> [--delete]
```

`ws` is the documented workspace command group. Root-level workspace shortcuts
may exist for compatibility, but they are not documented.

## Attach Semantics

`afs ws attach <workspace> <directory>` should:

1. Resolve the workspace by stable workspace identity.
2. Resolve the target directory to an absolute path.
3. Create the directory if it does not exist.
4. Start the local AFS runtime for that workspace and path.
5. Reconcile local and remote state.
6. Print a concise operation summary.
7. Persist runtime state so `afs status` and `afs ws detach` can describe the
   attachment.

For the first slice, attach should target sync mode. Mount mode can remain an
advanced configuration or a later `--mode mount` option after detach semantics
are safe and obvious.

Suggested output:

```text
Attached workspace agent1
path    /Users/example/agent-1-memories
mode    sync
files   42 scanned, 12 uploaded, 8 downloaded, 1 skipped

Detach with: afs ws detach /Users/example/agent-1-memories
```

`--verbose` should print a stable operation table:

```text
OP  PATH                                      DETAILS
I   memories/entities/products.md
D   memories/episodes/20260429/events.jsonl
S   some-enormous-file                       limit=2048MiB
```

Operation codes for attach/sync output:

- `I`: imported local path into workspace.
- `D`: downloaded remote path locally.
- `U`: uploaded local change after attach.
- `DC`: download conflict, local copy preserved.
- `UC`: upload conflict, remote copy preserved.
- `S`: skipped path with reason.

## Detach Semantics

`afs ws detach <directory>` should:

1. Find the active runtime state for the directory.
2. Stop the daemon or unmount the mounted surface.
3. Close the managed workspace session.
4. Remove AFS runtime metadata for that attachment.
5. Preserve the local directory by default.
6. Delete the local directory only when `--delete` is present.

Default output:

```text
Detached workspace agent1
path    /Users/example/agent-1-memories
local   preserved
```

Destructive output:

```text
Detached workspace agent1
path    /Users/example/agent-1-memories
local   deleted
```

`--delete` should fail unless the path is known to be the attached AFS path in
runtime state. It must not delete an arbitrary user-supplied directory.

## `up` And `down` Transition

Phase 1:

- Add `attach` and `detach`.
- Update root help examples and install/onboarding copy to use
  `attach`/`detach`.
- Keep `up` and `down` working.
- Make `up` print a short hint that `attach` is now the preferred command.
- Make `down` use the new safe detach behavior and preserve local files unless
  `--delete` is passed.

Phase 2:

- Remove `up` and `down` from prominent examples.
- Keep them in the reference as compatibility commands for one release cycle.
- Re-evaluate whether to remove them from routing after docs, onboarding, and
  installer copy no longer depend on them.

Important compatibility decision: if `down` remains routed, it should behave as
`detach` rather than preserving the old delete-by-default behavior.

## First Implementation Slice

Goal: ship the command vocabulary and safe detach behavior without trying to
solve the whole remote-filesystem UX at once.

Tasks:

1. Add routing for `afs ws attach` and `afs ws detach`.
2. Keep `afs ws` as the canonical documented workspace command group.
3. Keep root-level shortcuts hidden from help/docs.
4. Implement `cmdAttachArgs` as the new lifecycle wrapper.
5. Implement `cmdDetachArgs` around a shared detach function with
   `deleteLocal=false` by default.
6. Change sync shutdown to preserve the local directory unless `deleteLocal` is
   true.
7. Change mount shutdown to preserve mountpoints unless `deleteLocal` is true,
   while still restoring archived source directories for attach-at-source flows.
8. Update status and ready output from lifecycle language to
   "attached/detach" language.
9. Update root help, workspace help, setup next steps, installer next steps,
   UI connect-agent copy, and CLI reference docs.
10. Add tests for:
    - attach parses workspace and directory.
    - detach preserves sync directory by default.
    - detach `--delete` removes only the active attached path.
    - `down` preserves by default after the transition.
    - help surfaces prefer attach/detach.

## Files Likely To Change

- `cmd/afs/main.go`
- `cmd/afs/service_lifecycle.go`
- `cmd/afs/sync_lifecycle.go`
- `cmd/afs/config_commands.go`
- `cmd/afs/workspace_checkpoint_commands.go`
- `cmd/afs/afs_surface_test.go`
- `cmd/afs/config_commands_test.go`
- `docs/cli-reference.md`
- `docs/agent-filesystem.md`
- `ui/src/components/connect-agent-banner.tsx`
- `ui/src/features/agents/AgentSetupGuide.tsx`

## Follow-Up Surfaces

Remote filesystem inspection:

```bash
afs fs -w <workspace[@checkpoint]> ls
afs fs -w <workspace[@checkpoint]> cat <path>
afs fs -w <workspace[@checkpoint]> find . -name '*.md' -print
afs fs -w <workspace[@checkpoint]> grep <pattern> .
```

History and logs:

```bash
afs log -w <workspace>
afs log -w <workspace> --limit 200
```

Checkpoint labels:

```bash
afs tag create <workspace>@<checkpoint-or-timestamp> <name>
afs tag list <workspace>
afs tag delete <workspace> <name>
```

Multi-attach:

- Support multiple attached workspaces per machine.
- Replace the single runtime state file with attachment records keyed by local
  path and workspace ID.
- Update `afs status` to list all attachments.

## Non-Goals For First Slice

- No multi-attach daemon supervisor.
- No new remote filesystem command group yet.
- No checkpoint tag implementation yet.
- No database-first CLI UX.
- No full rename of internal sync/mount implementation symbols.
- No hidden aliases.

## Open Questions

1. Should `attach` always default to sync mode, with mount mode reserved for
   explicit configuration?
2. Should `detach --delete` require an interactive confirmation on TTY, or is
   the flag explicit enough?
5. Should `up` and `down` be removed after one release, or kept indefinitely as
   compatibility commands with attach/detach wording?
