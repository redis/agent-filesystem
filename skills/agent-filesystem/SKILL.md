---
name: agent-filesystem
description: "Use when agents need persistent shared storage, when saving or restoring workspace state, or when coordinating file access across multiple agents and machines. Creates Redis-backed workspaces, checkpoints and restores agent state, mounts shared filesystems locally, searches workspace contents, and forks workspaces for parallel work."
---

# Agent Filesystem

AFS is a workspace system for agents, backed by Redis. Use it when you want a
durable workspace that still feels like normal files and directories, with
explicit checkpoints and easy movement between MCP, sync mode, and live mounts.

## When to Use This Skill

**Use for:**
- Persistent agent workspaces that survive across sessions and machines
- Code, docs, or shared state that should live in a normal directory backed by Redis
- Saving and restoring workspace snapshots with explicit checkpoints
- Searching workspace contents with `afs fs grep` or MCP file tools
- Forking a workspace to run parallel experiments without losing the original

**Avoid for:**
- Large build output, media, or disposable artifacts
- Workflows that assume checkpoints happen automatically
- Old direct-command / `redis-cli` examples from module-era docs

## Preferred Interfaces

### 1. `afs mcp`
Use `afs mcp` when the agent can talk over MCP and does not need a local
directory.

### 2. Sync mode + `afs` CLI
Use sync mode when the agent or user wants a real local directory:

```bash
./afs ws mount my-project ~/my-project
cd ~/my-project
```

### 3. Live mount mode
Use `./afs config set --mode mount` before mounting when you need the
workspace exposed directly as a mount rather than through the sync daemon.

## Common Flows

### Create or import a workspace
```bash
./afs ws create my-project
./afs ws import my-project ./existing-dir
./afs ws list                          # verify the workspace exists
```

### Start working locally
```bash
./afs ws mount my-project ~/my-project
./afs status                           # verify the mount is active
cd ~/my-project
```

### Search a workspace
```bash
./afs fs grep --workspace my-project "TODO auth"
./afs fs grep --workspace my-project --path /src -E "timeout|retry"
```

### Save and restore stable points
```bash
./afs cp create my-project before-refactor
./afs cp list my-project               # verify the checkpoint was saved
./afs cp restore my-project before-refactor
./afs cp list my-project               # confirm the restore completed
```

### Fork work for a second line of effort
```bash
./afs ws fork my-project my-project-experiment
./afs ws list                          # verify the fork appears
```

## Key Points

- Redis is the source of truth for the live workspace and checkpoint history.
- Sync mode gives you a normal local directory; mount mode exposes the live
  workspace directly.
- `afs mcp` and the CLI operate on the same workspace model.
- File edits change the live workspace immediately.
- Create checkpoints explicitly when you want a restore point.
- `.afsignore` controls what gets imported from an existing local directory.

## Further Reading

- `docs/guides/agent-filesystem.md` — agent-facing usage guide
- `docs/reference/cli.md` — full CLI command reference
- `docs/reference/mcp.md` — MCP tool reference for agent integrations
