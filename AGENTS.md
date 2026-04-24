# AGENTS.md

This file provides guidance to Codex and other AI coding agents working in this repository.

**This is a living document.** When you learn a new repo-specific sharp edge, workflow rule, or recurring mistake, add it to [docs/agents/lessons-learned.md](docs/agents/lessons-learned.md).

## Quick Links

- [README.md](README.md)
- [docs/README.md](docs/README.md)
- [docs/repo-walkthrough.md](docs/repo-walkthrough.md)
- [docs/afs-control-plane-api.md](docs/afs-control-plane-api.md)
- [ui/README.md](ui/README.md)
- [docs/afs-cloud-control-plane-design.md](docs/afs-cloud-control-plane-design.md)
- [docs/backlog/storage-and-sync.md](docs/backlog/storage-and-sync.md)
- [docs/agents/lessons-learned.md](docs/agents/lessons-learned.md)

## Critical Invariants

- AFS is workspace-first. Redis is the canonical store for workspace metadata, manifests, blobs, checkpoints, and activity.
- Sync mode and live mounts are the supported local execution surfaces.
- `afs mcp` exposes the same workspace model over stdio for agent clients.
- Checkpoints are explicit. File edits change the live workspace state; they do not auto-create checkpoints.
- The canonical starter workspace name is `getting-started`. Treat it as a stable user-facing concept, not incidental seed data.
- Use `Self-managed` in user-facing copy for the control-plane-backed mode.

## Commands

```bash
# Core builds
make                # build mount helpers + afs + afs-control-plane
make mount          # build mount/agent-filesystem-mount + mount/agent-filesystem-nfs
make commands       # build afs + afs-control-plane
make test           # run Go unit tests for cmd/, deploy/, internal/, and mount/
make clean          # remove compiled artifacts

# Web/UI workflows
make web-install    # install UI dependencies into ui/
make web-build      # build the Vite UI
make web-dev        # run the control plane and UI together

# CLI lifecycle helpers
./afs setup
./afs up
./afs down
./afs status
./afs workspace import <workspace> <directory>
./afs workspace clone <workspace> <directory>
./afs checkpoint list <workspace>

# UI-only commands
cd ui && npm run dev
cd ui && npm run build
cd ui && npm run test
cd ui && npm run lint
```

## Validation By Surface

- CLI changes: run `make commands` and targeted tests under `./cmd/...`.
- Control-plane/backend changes: run `make test` or targeted tests under `./internal/...` and `./deploy/...`.
- Mount changes: run `cd mount && go test ./...`.
- UI changes: run `cd ui && npm run build` and the most relevant `npm run test` scope you can.
- Cross-surface web changes: prefer `make web-dev` to verify the control plane and Vite UI together.
- If you touch embedded UI behavior, verify with a path that rebuilds the UI assets, not just raw Go compilation.

## Embedded UI Build Rule

- `cmd/afs-control-plane` serves embedded assets from `internal/uistatic/dist`.
- Prefer `make afs-control-plane`, `make web-build`, or `make embed-ui` when UI assets matter.
- A plain `go build ./cmd/afs-control-plane` can still compile with placeholder assets and fall back to API-only behavior, so it is not sufficient verification for UI changes.

## Git & Shell Gotchas

- Quote file paths that include shell-significant characters when using `git add`, `git checkout`, or similar commands.
- In this repo, TanStack route files include `$` in filenames, for example:
  - `ui/src/routes/workspaces.$workspaceId.tsx`
  - `ui/src/routes/login.$clerkPath.tsx`
  - `ui/src/routes/signup.$clerkPath.tsx`
- In `zsh`, use quotes, for example: `git add "ui/src/routes/workspaces.$workspaceId.tsx"`.

## File Organization

- Before adding code, decide which product surface owns the behavior instead of appending to the nearest file.
- Keep CLI UX and local lifecycle behavior in `cmd/afs/`.
- Keep HTTP entrypoints in `cmd/afs-control-plane/`.
- Keep control-plane service logic in `internal/controlplane/`.
- Keep local materialization and manifest logic in `internal/worktree/`.
- Keep Redis-backed filesystem client logic in `mount/internal/client/`.
- Keep browser UI behavior in `ui/`.
- If a change introduces a distinct concern, prefer a focused colocated file over growing an already-mixed file.

## Current Repo Map

This repo has two active product layers:

- `mount/`: the inode-keyed Go client plus the FUSE and NFS exposure layer.
- `cmd/` + `internal/` + `ui/`: the workspace/checkpoint/control-plane product surface, where Redis stores manifests, blobs, savepoints, and activity while AFS materializes local working copies.

Useful supporting areas:

- `deploy/`: deployment-specific notes and helpers.
- `sandbox/`: isolated process runner.
- `scripts/`: helper scripts for local development and benchmarks.
- `skills/`: installable skill docs for agent use.
- `tests/`: benchmark helpers and fixtures for the active workspace-first surfaces.

Active backlog notes and longer design proposals live under `docs/backlog/`
and `docs/plans/`. Raw benchmark outputs should stay outside the repo.

For a file-by-file walkthrough of the current tree, read `docs/repo-walkthrough.md`.

The old Redis module, its Python integration suite, and RedisClaw have been retired and should not be treated as active architecture.

## Architecture Summary

The most important implementation seams are:

- `cmd/afs/`: CLI command surface, setup flow, sync lifecycle, local UX.
- `cmd/afs-control-plane/`: HTTP control plane binary.
- `internal/controlplane/`: workspace, checkpoint, session, catalog, and HTTP service logic.
- `internal/worktree/`: manifest scanning and local materialization helpers.
- `mount/internal/client/`: Redis-backed filesystem client used by FUSE/NFS.
- `ui/`: TanStack Router + React control-plane UI.
