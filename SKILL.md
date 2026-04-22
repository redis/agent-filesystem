# Agent Filesystem

Agent Filesystem is now a workspace-first system for agents. Redis is still the
canonical store, but the active user surfaces are:

- `afs mcp` for agent clients
- `afs up --mode sync` for a normal local working directory
- `afs up --mode mount` for a live Redis-backed mount
- explicit checkpoints via `afs checkpoint ...`

The old direct-command storage surface is retired and should not be used as the
mental model for this repo.

## Fast Start

```bash
make commands
./afs setup
./afs workspace create demo
./afs workspace use demo
./afs up --mode sync
cd ~/afs
```

## Most Useful Commands

```bash
./afs workspace create <workspace>
./afs workspace import <workspace> <directory>
./afs workspace use <workspace>
./afs workspace list
./afs workspace fork <workspace> <new-workspace>
./afs workspace clone <workspace> <directory>
./afs checkpoint create <workspace> <name>
./afs checkpoint list <workspace>
./afs checkpoint restore <workspace> <name>
./afs grep --workspace <workspace> "pattern"
./afs up --mode sync
./afs up --mode mount
./afs status
./afs down
./afs mcp
```

## Working Model

- Redis stores the live workspace state plus checkpoint history.
- Sync mode gives you a real local directory that is reconciled with Redis.
- Mount mode exposes the live workspace directly through NFS/FUSE.
- MCP talks to the same workspace model without requiring a local directory.
- File edits update the live workspace immediately; create checkpoints
  explicitly when you want a durable restore point.

## Read Next

- `README.md` for the current product story and setup flow
- `docs/repo-walkthrough.md` for the current tree layout
- `skills/agent-filesystem/SKILL.md` for the installable agent skill
