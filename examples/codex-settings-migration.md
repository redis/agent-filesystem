# Share Codex State Across Computers with Redis Agent Filesystem

This guide shows how to put `~/.codex` into Redis Agent Filesystem on one computer, then mount that same shared state on other computers so Codex keeps the same memory and settings everywhere.

Use this when:

- Codex stores local state in `~/.codex`
- you want the same state across multiple machines
- you usually use one machine at a time and want to resume cleanly when you switch

## Recommended exclusions

Before importing, create `~/.codex/.afsignore` to exclude machine-local or high-churn state you do not want to sync.

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

`worktrees/` is a good default exclusion. It is usually large, machine-local, and likely to cause confusion if multiple computers treat it as shared state.

Because `.afsignore` uses `.gitignore`-style rules, you can also re-include a specific file with `!`, for example:

```gitignore
*.log
!logs/important.log
```

## Machine 1: import the existing `~/.codex`

Build Redis Agent Filesystem:

```bash
cd /path/to/agent-filesystem
make
```

Run setup and point AFS at your shared Redis instance.

Important setup choices:

- choose your shared Redis host, password, and DB
- choose workspace name `.codex`
- choose mountpoint `~/.codex`

Create or review the ignore file:

```bash
cat > ~/.codex/.afsignore <<'EOF'
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

Import the existing directory into the `.codex` workspace and replace it in place with an AFS-managed copy:

```bash
./afs workspace import --clone-at-source .codex ~/.codex
./afs workspace use .codex
./afs up
```

What that does:

- imports `~/.codex` into the workspace `.codex`
- moves your original directory to `~/.codex.pre-afs`
- materializes the imported workspace back at `~/.codex`
- mounts the current workspace at `~/.codex`

Verify:

```bash
./afs status
ls -la ~/.codex
```

## Machine 2 and later: mount the same shared Codex state

On each additional computer:

1. Build `agent-filesystem`.
2. Run `./afs setup`.
3. Point it at the same shared Redis instance.
4. Choose current workspace `.codex`.
5. Choose mountpoint `~/.codex`.

Back up any existing local Codex directory first:

```bash
if [ -d ~/.codex ]; then mv ~/.codex ~/.codex.local-backup; fi
mkdir -p ~/.codex
```

Then mount the shared workspace:

```bash
./afs up
./afs status
ls -la ~/.codex
```

## Agent checklist

If you want an agent to perform this, the agent should:

1. Confirm Codex is not currently running on the machines involved.
2. Recommend creating `~/.codex/.afsignore` before import.
3. Suggest excluding `worktrees/` by default, unless the user explicitly wants local checkout state shared.
4. Build `agent-filesystem` with `make`.
5. On the first machine, run `./afs setup`, then `./afs workspace import --clone-at-source .codex ~/.codex`, then `./afs workspace use .codex`, then `./afs up`.
6. On later machines, back up any existing `~/.codex`, run `./afs setup` with workspace `.codex` and mountpoint `~/.codex`, then run `./afs up`.
7. Verify that the same Codex files appear on every machine.

## Rollback

Undo on the first computer:

```bash
./afs down
rm -rf ~/.codex
mv ~/.codex.pre-afs ~/.codex
```

Undo on a later computer:

```bash
./afs down
rm -rf ~/.codex
mv ~/.codex.local-backup ~/.codex
```
