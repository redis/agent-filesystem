---
name: codex-settings-sync
description: Use when the user wants to migrate Codex state in ~/.codex into Agent Filesystem and mount the same shared Codex memory/settings across multiple computers. Recommends a .afsignore before migration and defaults to excluding worktrees, caches, logs, and temporary files.
---

# Codex Settings Sync

Use this skill when the goal is to share Codex state across machines by moving `~/.codex` into Agent Filesystem, then mounting that same Redis-backed volume on other computers.

## Default stance

- Recommend a root `~/.codex/.afsignore` before migration.
- Default to excluding `worktrees/`.
- Treat `cache/`, `tmp/`, `logs/`, `*.log`, `*.tmp`, `*.pid`, and `*.sock` as good default exclusions.
- If the user appears to care about restoring local checkout state inside Codex, call out `worktrees/` as a choice point before migrating.

Open the bundled starter ignore file at [assets/.afsignore](assets/.afsignore) and adapt it to the user's needs.

## Migration workflow

1. Ask the user to stop Codex on the machines involved, or verify that it is already closed.
2. Ensure `agent-filesystem` is built with `make`.
3. Configure `afs` to point at the shared Redis instance.
4. Create or update `~/.codex/.afsignore` before migration.
5. On the source machine, run `./afs ws import --attach-at-source .codex ~/.codex`.
6. Explain that the imported workspace is attached at `~/.codex`, and the workspace name is `.codex`.
7. On each additional machine, move aside any existing `~/.codex`, choose mount mode with `./afs config set --mode mount` if you want a live mount there, then run `./afs ws attach .codex ~/.codex`.
8. Verify with `./afs status` and `ls -la ~/.codex`.

## Secondary machine config

Point the CLI at the same control plane or Redis database. Then run
`./afs config set --mode mount` for live mount mode, or
`./afs config set --mode sync` for sync mode, before attaching `.codex` at
that machine's `~/.codex`.

## Notes to surface

- `afs ws import` honors `~/.codex/.afsignore` if present.
- `.afsignore` uses `.gitignore`-style pattern syntax.
- Excluding a directory like `worktrees/` is usually safer than syncing it.
- Avoid using the same shared `~/.codex` from multiple active computers at the same time.
- Keep any `.local-backup` directories until the setup is stable.

## Rollback

Source machine rollback:

```bash
./afs ws detach ~/.codex
```

Secondary machine rollback:

```bash
./afs ws detach ~/.codex
rm -rf ~/.codex
mv ~/.codex.local-backup ~/.codex
```
