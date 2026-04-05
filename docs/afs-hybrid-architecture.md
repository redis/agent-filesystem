# AFS Hybrid Architecture

Date: 2026-04-05

## Thesis

AFS uses a hybrid workspace model:

- Redis is the canonical store for workspace metadata, checkpoints, manifests, blobs, and audit.
- Native local directories are the execution surface for shells, editors, compilers, and tests.
- `afs workspace run` runs real processes against real directories.
- `afs checkpoint create` and `afs checkpoint restore` operate against Redis-backed canonical state.

This keeps the user model small while preserving native OS ergonomics.

## Product Model

Users only need to learn:

- workspace
- checkpoint
- working copy

The working copy is local and mutable. Checkpoints are durable and immutable.

## Core Design

### Canonical state lives in Redis

Redis stores:

- workspace metadata
- checkpoint metadata
- manifests
- content-addressed blobs
- audit streams

### Active execution happens in a local directory

When a developer or agent works on a workspace, AFS creates or reuses:

- `~/.afs/workspaces/<workspace>/tree/`
- `~/.afs/workspaces/<workspace>/state.json`

That directory is usable by normal tools such as `bash`, `git`, `go test`, `pytest`, and editors.

### Checkpoints are immutable

`afs checkpoint create` records the current working copy as a new immutable checkpoint in Redis.

### Restore is rematerialization

`afs checkpoint restore` does not replay inverse operations. It archives the current local tree if needed and recreates the working copy from the selected checkpoint manifest.

## Main Flows

### Import or create

- `afs workspace create <workspace>` creates an empty workspace and initializes `initial`
- `afs workspace import <workspace> <directory>` snapshots an existing tree and initializes `initial`

### Run

`afs workspace run`:

1. resolves the workspace
2. ensures a local materialization exists
3. launches a real child process in that directory
4. auto-checkpoints changes unless `--readonly` is used

### Checkpoint create

`afs checkpoint create`:

1. scans the local working copy
2. writes changed blobs and a new manifest to Redis
3. records checkpoint metadata
4. moves the workspace head to the new checkpoint

### Checkpoint restore

`afs checkpoint restore`:

1. loads the target checkpoint manifest from Redis
2. archives the current local tree
3. rematerializes the working copy
4. moves the workspace head back to the selected checkpoint

### Clone

`afs workspace clone <workspace> <directory>` materializes the current saved head into a separate directory for export or inspection.

### Fork

`afs workspace fork <workspace> <new-workspace>` creates a new workspace whose initial head matches the source workspace's current saved head.

## Local Layout

```text
~/.afs/workspaces/<workspace>/
  state.json
  tree/
  archive/
```

## Redis Schema

```text
afs:{ws}:workspace:meta
afs:{ws}:workspace:savepoints
afs:{ws}:workspace:audit

afs:{ws}:savepoint:<sp>:meta
afs:{ws}:savepoint:<sp>:manifest

afs:{ws}:blob:<sha256>
afs:{ws}:blobref:<sha256>
```

## Why This Model

This design avoids reimplementing live filesystem behavior in Redis while still giving AFS durable checkpoints, cloneable workspaces, and auditability.
