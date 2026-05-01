---
name: agent-filesystem
description: Persistent Redis-backed workspaces for agents. Use via `afs mcp`, the `afs` CLI, sync mode, live mounts, and explicit checkpoints.
---

# Agent Filesystem

AFS is a workspace system for agents, backed by Redis. Use it when you want a
durable workspace that still feels like normal files and directories, with
explicit checkpoints and easy movement between MCP, sync mode, and live mounts.

## When to Use This Skill

**Use for:**
- Persistent agent workspaces
- Code or docs that should live in a normal directory
- Shared notes/config/state that benefit from checkpoints and forks
- Searchable workspaces where `afs fs grep` or MCP file tools are useful

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
```

### Start working locally
```bash
./afs ws mount my-project ~/my-project
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
./afs cp list my-project
./afs cp restore my-project before-refactor
```

### Fork work for a second line of effort
```bash
./afs ws fork my-project my-project-experiment
```

## Key Points

- Redis is the source of truth for the live workspace and checkpoint history.
- Sync mode gives you a normal local directory; mount mode exposes the live
  workspace directly.
- `afs mcp` and the CLI operate on the same workspace model.
- File edits change the live workspace immediately.
- Create checkpoints explicitly when you want a restore point.
- `.afsignore` controls what gets imported from an existing local directory.
