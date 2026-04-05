# Share Codex State Across Computers with Agent Filesystem

This guide shows how to migrate `~/.codex` into Agent Filesystem on one computer, then mount that same shared state on other computers so Codex keeps the same memory and settings everywhere.

Use this when:

- Codex stores local state in `~/.codex`
- you want the same state across multiple machines
- you usually use one machine at a time and want to resume cleanly when you switch

Important behavior:

- `./raf migrate ~/.codex` imports the directory into Redis
- the original directory is renamed to `~/.codex.archive`
- Agent Filesystem is mounted back at `~/.codex`
- the Redis key becomes `.codex`, because `rfs migrate` uses the source directory basename
- if `~/.codex/.rfsignore` exists, matching files and directories are skipped during import

## Recommended exclusions

Before migrating, create `~/.codex/.rfsignore` to exclude machine-local or high-churn state you do not want to sync.

Suggested starting point:

```gitignore
# High-churn caches
cache/
tmp/

# Local checkout state
worktrees/

# Local logs and temp files
logs/
*.log
*.tmp
*.pid
*.sock
```

`worktrees/` is a good default exclusion. It is usually large, machine-local, and likely to cause confusion if multiple computers treat it as shared state. Only remove that exclusion if you explicitly want Codex's local worktree state to roam too.

Because `.rfsignore` uses `.gitignore`-style rules, you can also re-include a specific file with `!`, for example:

```gitignore
*.log
!logs/important.log
```

## Machine 1: migrate the existing `~/.codex`

Build `agent-filesystem`:

```bash
cd /path/to/agent-filesystem
make
```

Set up `rfs` against the shared Redis instance:

```bash
./raf setup
```

Use your shared Redis host, password, and DB during setup. The mountpoint chosen during setup is not important for the migration step because `./raf migrate ~/.codex` will mount back at `~/.codex`.

Create or review the ignore file:

```bash
cat > ~/.codex/.rfsignore <<'EOF'
cache/
tmp/
worktrees/
logs/
*.log
*.tmp
*.pid
*.sock
EOF
```

Then migrate:

```bash
./raf migrate ~/.codex
```

Verify:

```bash
./raf status
ls -la ~/.codex
```

## Machine 2 and later: mount the same shared Codex state

On each additional computer:

1. Build `agent-filesystem`.
2. Point `rfs` at the same shared Redis instance.
3. Use the same Redis key, `.codex`.
4. Mount it at the local path `~/.codex`.

Back up any existing local Codex directory first:

```bash
if [ -d ~/.codex ]; then mv ~/.codex ~/.codex.local-backup; fi
mkdir -p ~/.codex
```

Create `rfs.config.json` next to the `rfs` binary:

```json
{
  "useExistingRedis": true,
  "redisAddr": "YOUR_SHARED_REDIS_HOST:6379",
  "redisPassword": "",
  "redisDB": 0,
  "redisKey": ".codex",
  "mountpoint": "/Users/YOUR_USER/.codex",
  "mountBackend": "auto",
  "readOnly": false,
  "allowOther": false,
  "redisServerBin": "",
  "modulePath": "",
  "mountBin": "",
  "nfsBin": "",
  "nfsHost": "127.0.0.1",
  "nfsPort": 20490,
  "redisLog": "/tmp/rfs-redis.log",
  "mountLog": "/tmp/rfs-mount.log"
}
```

Start the mount:

```bash
./raf up
./raf status
ls -la ~/.codex
```

## Agent checklist

If you want an agent to perform this, the agent should:

1. Confirm Codex is not currently running on the machines involved.
2. Recommend creating `~/.codex/.rfsignore` before migration.
3. Suggest excluding `worktrees/` by default, unless the user explicitly wants local checkout state shared.
4. Build `agent-filesystem` with `make`.
5. Configure `rfs` to use the shared Redis instance.
6. On the first machine, run `./raf migrate ~/.codex`.
7. On later machines, back up any existing `~/.codex`, configure `redisKey` as `.codex`, then run `./raf up`.
8. Verify that the same Codex files appear on every machine.

## Rollback

Undo on the first computer:

```bash
./raf down
rm -rf ~/.codex
mv ~/.codex.archive ~/.codex
```

Undo on a later computer:

```bash
./raf down
rm -rf ~/.codex
mv ~/.codex.local-backup ~/.codex
```
