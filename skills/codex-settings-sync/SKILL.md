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
5. On the source machine, run `./afs workspace import --mount-at-source .codex ~/.codex`, then `./afs workspace use .codex`.
6. Explain that the original directory becomes `~/.codex.pre-afs`, the imported workspace is mounted at `~/.codex`, and the workspace name is `.codex`.
7. On each additional machine, move aside any existing `~/.codex`, configure the current workspace as `.codex`, set `localPath` to that machine's `~/.codex`, choose mount mode if you want a live mount there, then run `./afs up`.
8. Verify with `./afs status` and `ls -la ~/.codex`.

## Secondary machine config

Use a config like:

```json
{
  "redis": {
    "addr": "YOUR_SHARED_REDIS_HOST:6379",
    "password": "",
    "db": 0
  },
  "mode": "mount",
  "currentWorkspace": ".codex",
  "localPath": "/Users/YOUR_USER/.codex",
  "mount": {
    "backend": "nfs",
    "readOnly": false,
    "allowOther": false,
    "mountBin": "",
    "nfsBin": "",
    "nfsHost": "127.0.0.1",
    "nfsPort": 20490
  },
  "logs": {
    "mount": "/tmp/afs-mount.log",
    "sync": "/tmp/afs-sync.log"
  },
  "sync": {
    "fileSizeCapMB": 2048
  }
}
```

For sync mode instead, keep `"mode": "sync"` and set `"mount": { "backend": "none" }`.

## Notes to surface

- `afs workspace import` honors `~/.codex/.afsignore` if present.
- `.afsignore` uses `.gitignore`-style pattern syntax.
- Excluding a directory like `worktrees/` is usually safer than syncing it.
- Avoid using the same shared `~/.codex` from multiple active computers at the same time.
- Keep `~/.codex.pre-afs` and any `.local-backup` directories until the setup is stable.

## Rollback

Source machine rollback:

```bash
./afs down
rm -rf ~/.codex
mv ~/.codex.pre-afs ~/.codex
```

Secondary machine rollback:

```bash
./afs down
rm -rf ~/.codex
mv ~/.codex.local-backup ~/.codex
```
