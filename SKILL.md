# Agent Filesystem

Agent Filesystem is now a workspace-first system for agents. Redis is still the
canonical store, but the active user surfaces are:

- `afs mcp` for agent clients
- `afs ws attach <workspace> <directory>` for a normal local working directory
- `afs config set --mode mount` before attach for a live Redis-backed mount
- explicit checkpoints via `afs cp ...`

The old direct-command storage surface is retired and should not be used as the
mental model for this repo.

## Fast Start

```bash
make commands
./afs auth login
./afs ws create demo
./afs ws attach demo ~/demo
cd ~/demo
```

## Most Useful Commands

```bash
./afs ws create <workspace>
./afs ws import <workspace> <directory>
./afs ws attach <workspace> <directory>
./afs ws detach <workspace-or-directory>
./afs ws list
./afs ws fork <workspace> <new-workspace>
./afs cp create <workspace> <name>
./afs cp list <workspace>
./afs cp restore <workspace> <name>
./afs fs grep --workspace <workspace> "pattern"
./afs config set --mode sync
./afs config set --mode mount
./afs status
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
- `docs/README.md` for the current documentation, backlog, and plan index
- `docs/repo-walkthrough.md` for the current tree layout
- `skills/agent-filesystem/SKILL.md` for the installable agent skill
