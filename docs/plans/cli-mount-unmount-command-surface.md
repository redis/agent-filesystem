# AFS CLI Mount/Unmount Command Surface Plan

Last reviewed: 2026-04-30.
Status: draft for review.

## Summary

AFS should make the local lifecycle feel like mounting a durable remote
workspace to a normal local directory, then unmounting it when work is done.

The accepted vocabulary is `afs ws mount` and `afs ws unmount`. The
user-facing model is:

- A workspace is a durable remote filesystem tree.
- A mounted directory is the local working surface for that workspace.
- Unmount stops AFS from managing the local surface.
- Deleting local files is an explicit opt-in, not the default unmount behavior.

This plan focuses first on the CLI command surface and the transition to
mount/unmount language. Remote filesystem inspection, tags, richer logs, and
multi-mount support are follow-up surfaces.

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
afs ws mount [workspace] [directory]
afs ws unmount
afs ws import [--force] [--mount-at-source] [--database <database>] <workspace> <directory>
afs status
afs log
afs fs grep
```

Important current behavior:

- `afs ws mount` starts the local surface for an explicit workspace.
- `afs ws unmount` stops managing the mounted local folder.
- Sync-mode unmount preserves local files by default.
- Import with `--mount-at-source` mounts the source directory after import.
- The mount registry supports multiple workspace/folder mounts.

## Target Command Shape

Recommended canonical commands:

```bash
afs ws list
afs ws create <workspace>
afs ws delete <workspace>
afs ws mount <workspace> <directory> [--verbose]
afs ws unmount <directory> [--delete]
```

`ws` is the documented workspace command group. Root-level workspace shortcuts
may exist for compatibility, but they are not documented.

## Mount Semantics

`afs ws mount <workspace> <directory>` should:

1. Resolve the workspace by stable workspace identity.
2. Resolve the target directory to an absolute path.
3. Create the directory if it does not exist.
4. Start the local AFS runtime for that workspace and path.
5. Reconcile local and remote state.
6. Print a concise operation summary.
7. Persist runtime state so `afs status` and `afs ws unmount` can describe the
   mount.

For the first slice, mount should target sync mode. Mount mode can remain an
advanced configuration or a later `--mode mount` option after unmount semantics
are safe and obvious.

Suggested output:

```text
Mounted workspace agent1
path    /Users/example/agent-1-memories
mode    sync
files   42 scanned, 12 uploaded, 8 downloaded, 1 skipped

Unmount with: afs ws unmount /Users/example/agent-1-memories
```

`--verbose` should print a stable operation table:

```text
OP  PATH                                      DETAILS
I   memories/entities/products.md
D   memories/episodes/20260429/events.jsonl
S   some-enormous-file                       limit=2048MiB
```

Operation codes for mount/sync output:

- `I`: imported local path into workspace.
- `D`: downloaded remote path locally.
- `U`: uploaded local change after mount.
- `DC`: download conflict, local copy preserved.
- `UC`: upload conflict, remote copy preserved.
- `S`: skipped path with reason.

## Unmount Semantics

`afs ws unmount <directory>` should:

1. Find the active runtime state for the directory.
2. Stop the daemon or unmount the mounted surface.
3. Close the managed workspace session.
4. Remove AFS runtime metadata for that mount.
5. Preserve the local directory by default.
6. Delete the local directory only when `--delete` is present.

Default output:

```text
Unmounted workspace agent1
path    /Users/example/agent-1-memories
local   preserved
```

Destructive output:

```text
Unmounted workspace agent1
path    /Users/example/agent-1-memories
local   deleted
```

`--delete` should fail unless the path is known to be the mounted AFS path in
runtime state. It must not delete an arbitrary user-supplied directory.

## `up` And `down` Transition

Phase 1:

- Add `mount` and `unmount`.
- Update root help examples and install/onboarding copy to use
  `mount`/`unmount`.
- Keep `up` and `down` working.
- Make `up` print a short hint that `mount` is now the preferred command.
- Make `down` use the new safe unmount behavior and preserve local files unless
  `--delete` is passed.

Phase 2:

- Remove `up` and `down` from prominent examples.
- Keep them in the reference as compatibility commands for one release cycle.
- Re-evaluate whether to remove them from routing after docs, onboarding, and
  installer copy no longer depend on them.

Important compatibility decision: if `down` remains routed, it should behave as
`unmount` rather than preserving the old delete-by-default behavior.

## First Implementation Slice

Goal: ship the command vocabulary and safe unmount behavior without trying to
solve the whole remote-filesystem UX at once.

Tasks:

1. Add routing for `afs ws mount` and `afs ws unmount`.
2. Keep `afs ws` as the canonical documented workspace command group.
3. Keep root-level shortcuts hidden from help/docs.
4. Implement `cmdMountArgs` as the new lifecycle wrapper.
5. Implement `cmdUnmountArgs` around a shared unmount function with
   `deleteLocal=false` by default.
6. Change sync shutdown to preserve the local directory unless `deleteLocal` is
   true.
7. Change mount shutdown to preserve mountpoints unless `deleteLocal` is true,
   while still restoring archived source directories for mount-at-source flows.
8. Update status and ready output from lifecycle language to
   "mounted/unmount" language.
9. Update root help, workspace help, setup next steps, installer next steps,
   UI connect-agent copy, and CLI reference docs.
10. Add tests for:
    - mount parses workspace and directory.
    - unmount preserves sync directory by default.
    - unmount `--delete` removes only the active mounted path.
    - `down` preserves by default after the transition.
    - help surfaces prefer mount/unmount.

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

Multi-mount:

- Support multiple mounted workspaces per machine.
- Replace the single runtime state file with mount records keyed by local
  path and workspace ID.
- Update `afs status` to list all mounts.

## Non-Goals For First Slice

- No multi-mount daemon supervisor.
- No new remote filesystem command group yet.
- No checkpoint tag implementation yet.
- No database-first CLI UX.
- No full rename of internal sync/mount implementation symbols.
- No hidden aliases.

## Open Questions

1. Should `mount` always default to sync mode, with mount mode reserved for
   explicit configuration?
2. Should `unmount --delete` require an interactive confirmation on TTY, or is
   the flag explicit enough?
5. Should `up` and `down` be removed after one release, or kept indefinitely as
   compatibility commands with mount/unmount wording?
