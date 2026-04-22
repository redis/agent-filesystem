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
- Searchable workspaces where `afs grep` or MCP file tools are useful

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
./afs workspace use my-project
./afs up --mode sync
cd ~/afs
```

### 3. Live mount mode
Use `./afs up --mode mount` when you need the workspace exposed directly as a
mount rather than through the sync daemon.

## Common Flows

### Create or import a workspace
```bash
./afs workspace create my-project
./afs workspace import my-project ./existing-dir
./afs workspace use my-project
```

### Start working locally
```bash
./afs up --mode sync
cd ~/afs
```

### Search a workspace
```bash
./afs grep --workspace my-project "TODO auth"
./afs grep --workspace my-project --path /src -E "timeout|retry"
```

### Save and restore stable points
```bash
./afs checkpoint create my-project before-refactor
./afs checkpoint list my-project
./afs checkpoint restore my-project before-refactor
```

### Fork work for a second line of effort
```bash
./afs workspace fork my-project my-project-experiment
```

## Key Points

- Redis is the source of truth for the live workspace and checkpoint history.
- Sync mode gives you a normal local directory; mount mode exposes the live
  workspace directly.
- `afs mcp` and the CLI operate on the same workspace model.
- File edits change the live workspace immediately.
- Create checkpoints explicitly when you want a restore point.
- `.afsignore` controls what gets imported from an existing local directory.
