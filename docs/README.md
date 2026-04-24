# Documentation Index

Last reviewed: 2026-04-24.

This directory is for durable repo documentation: product contracts,
architecture notes, operating guides, and consolidated performance findings.
It is the single root for docs, active backlog notes, and longer design plans.

## Current References

- `afs-control-plane-api.md` - the shared workspace/checkpoint/file HTTP contract.
- `afs-cloud-control-plane-design.md` - active hosted control-plane architecture.
- `afs-cloud-control-plane-byodb-design.md` - external database and private data-plane design.
- `performance.md` - consolidated benchmark and performance notes.
- `repo-walkthrough.md` - current repo map and suggested read order.
- `../AGENTS.md` - repo-specific guidance and sharp edges for coding agents.
- `agents/lessons-learned.md` - repo-specific sharp edges that agents should preserve.

## Planning And Backlog

- `backlog/storage-and-sync.md` - active storage, sync, mounted checkpoint, and
  large-file follow-up backlog.
- `plans/event-history-merge.md` - plan to merge audit and changelog streams.
- `plans/observability.md` - observability research and milestone notes.

## Removed From Active Roots

The old phase-one, hybrid, cloud-plus-standalone, and cloud execution-plan notes
were removed because they described completed slices or retired `workspace run`
and `direct` flows. Use git history only if you need that context, and verify
every command against the current CLI before copying it back into active docs.

The old top-level `tasks/` and `plans/` directories were removed or
consolidated here so there is one master documentation root.
