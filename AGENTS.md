# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Build & Test

```bash
make                # build mount/agent-filesystem-mount + mount/agent-filesystem-nfs + afs + afs-control-plane
make mount          # build mount/agent-filesystem-mount + mount/agent-filesystem-nfs
make commands       # build afs + afs-control-plane
make test           # run Go unit tests for cmd/, deploy/, internal/, and mount/
make clean          # remove compiled artifacts

# CLI lifecycle helper:
./afs setup
./afs up
./afs down
./afs status
./afs workspace import <workspace> <directory>
./afs workspace clone <workspace> <directory>
./afs checkpoint list <workspace>
```

## Current Repo Map

This repo now has two active product layers:

- `mount/`: the inode-keyed Go client plus the FUSE and NFS exposure layer.
- `cmd/` + `internal/` + `ui/`: the workspace/checkpoint/control-plane product surface, where Redis stores manifests/blobs/savepoints and AFS materializes local working copies.

Useful supporting areas:

- `deploy/`: deployment-specific notes and helpers.
- `plans/` and `tasks/`: working design notes, backlog fragments, and benchmark outputs.
- `sandbox/`: isolated process runner.
- `scripts/`: helper scripts for local development and benchmarks.
- `skills/`: installable skill docs for agent use.
- `tests/`: benchmark helpers and fixtures for the active workspace-first surfaces.

For a file-by-file walkthrough of the current tree, read `docs/repo-walkthrough.md`.

The old Redis module, its Python integration suite, and RedisClaw have been retired and should not be treated as active architecture.

## Active Architecture

AFS is now workspace-first:

- Redis is the canonical store for workspace metadata, manifests, blobs, checkpoints, and activity.
- Sync mode and live mounts are the supported local execution surfaces.
- `afs mcp` exposes the same workspace model over stdio for agent clients.
- Checkpoints are explicit. File edits change the live workspace state; they do not auto-create checkpoints.

The most important implementation seams are:

- `cmd/afs/`: CLI command surface, setup flow, sync lifecycle, local UX.
- `cmd/afs-control-plane/`: HTTP control plane binary.
- `internal/controlplane/`: workspace, checkpoint, session, catalog, and HTTP service logic.
- `internal/worktree/`: manifest scanning and local materialization helpers.
- `mount/internal/client/`: Redis-backed filesystem client used by FUSE/NFS.
- `ui/`: TanStack Router + React control-plane UI.
