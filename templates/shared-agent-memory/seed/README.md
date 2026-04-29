# Shared Agent Memory

A shared long-term memory for agents across your team. Every agent that
connects (Claude Code, Codex, Cursor, or any MCP client) reads from and
writes to the same memory, backed by Redis through Agent Filesystem.

## Layout

- `shared-memory/index.md` — curated rollup of all learnings, newest first.
- `shared-memory/entries/YYYY-MM-DD-<slug>.md` — one file per learning.
- `AGENTS.md` — the protocol every agent should follow when using this workspace.

## Why it's interesting

Redis sits behind the workspace, so reads are sub-millisecond and every
agent connected to this workspace sees writes immediately. Nothing to
sync, nothing to pull — just a filesystem that happens to be shared.

## Getting started

See `AGENTS.md` for the read/write protocol agents should follow.
