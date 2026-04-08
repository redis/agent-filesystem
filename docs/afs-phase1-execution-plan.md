# AFS Phase 1 Execution Plan

Date: 2026-04-05

## Goal

Ship the smallest real AFS that feels coherent:

- `afs` is the main CLI
- Redis stores canonical saved state
- local directories are used for live execution
- `afs workspace run`, `afs workspace clone`, `afs workspace fork`, `afs checkpoint create`, and `afs checkpoint restore` work end to end

## Scope

### In scope

- CLI rename to `afs`
- workspace CRUD
- checkpoint create, list, and restore
- local materialization under `~/.afs`
- Redis audit streams
- optimistic concurrency checks on restore and save

### Out of scope

- `afs apply`
- public diff commands
- Git-aware helpers
- shared live multi-writer editing
- FUSE/NFS integration for the new AFS workflow

## Delivered Model

Phase 1 is workspace-first:

- each workspace has one live working copy
- each workspace has one saved head checkpoint
- checkpoints are immutable
- `workspace fork` is the way to branch into parallel work

## Command Surface

```bash
afs setup
afs up
afs down
afs status

afs workspace create <workspace>
afs workspace list
afs workspace current
afs workspace use <workspace>
afs workspace import <workspace> <directory>
afs workspace run [workspace] [--readonly] -- <command...>
afs workspace clone <workspace> <directory>
afs workspace fork <workspace> <new-workspace>
afs workspace delete <workspace>...

afs checkpoint list <workspace>
afs checkpoint create <workspace> [checkpoint]
afs checkpoint restore <workspace> <checkpoint>
```

## Data Structures

### Config

AFS config keeps:

- Redis connection settings
- `workRoot`
- current workspace selection
- mount/runtime compatibility settings for legacy flows

### Local state

Local workspace state stores:

- `workspace`
- `head_savepoint`
- `dirty`
- `materialized_at`
- `last_scan_at`

Stored at:

```text
~/.afs/workspaces/<workspace>/state.json
```

The working tree lives at:

```text
~/.afs/workspaces/<workspace>/tree/
```

### Redis keys

```text
afs:{ws}:workspace:meta
afs:{ws}:workspace:savepoints
afs:{ws}:workspace:audit

afs:{ws}:savepoint:<sp>:meta
afs:{ws}:savepoint:<sp>:manifest

afs:{ws}:blob:<sha256>
afs:{ws}:blobref:<sha256>
```

## Flow Summary

### Create/import

- initialize workspace metadata
- write `initial` checkpoint

### Run

- materialize from the live workspace root if needed
- launch a real process in `tree/`
- sync changes back into the live workspace root unless `--readonly` is used

### Restore

- verify the workspace head
- archive the current tree if present
- rematerialize from the selected checkpoint

### Fork

- copy the current saved head into a new workspace

## Remaining Work

- hosted control-plane implementation
- richer browser/tree APIs
- diff/apply flows
- blob GC and checkpoint retention policies
