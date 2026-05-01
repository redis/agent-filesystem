# Agent Filesystem Repo Walkthrough

This guide is the "what lives where" map for the current `/Users/rowantrollope/git/agent-filesystem` working tree as of 2026-04-24.

## Scope

- This guide treats the current worktree as the source of truth.
- It covers active project-owned source, tests, docs, examples, and build/config files.
- It does not try to inventory generated or runtime-only state such as `.git/`, `.claude/`, `ui/node_modules/`, `ui/dist/`, the built `afs` and `afs-control-plane` binaries, or copied UI assets under `internal/uistatic/dist/`.
- It intentionally avoids enumerating untracked worktree-local files because that list changes quickly during active development.

## The Big Picture

AFS currently has two active implementation layers:

1. `mount/`: the Go client plus the FUSE/NFS exposure layer over Redis-backed workspace data.
2. `cmd/` + `internal/` + `ui/`: the workspace/checkpoint/control-plane product surface, where Redis stores manifests, blobs, checkpoints, and activity while AFS materializes local working copies.

Supporting areas:

- `deploy/`: deployment-specific notes and helpers.
- `sandbox/`: isolated process runner service.
- `scripts/`: local development and benchmark helpers.
- `skills/`: agent-facing skill docs.
- `tests/`: benchmark helpers and fixtures for active workspace-first flows.
- `third_party/go-nfs/`: vendored upstream NFS library used by the NFS server binary.

## Top-Level Folder Map

- `cmd/`: Go command binaries for `afs` and `afs-control-plane`.
- `deploy/`: deployment helpers and platform-specific notes.
- `docs/`: current architecture, API, and repo-organization notes.
- `examples/`: example configs and migration guides.
- `internal/`: shared workspace/control-plane packages used by the top-level commands.
- `mount/`: Go mount and NFS serving stack.
- `sandbox/`: HTTP and MCP process sandbox.
- `scripts/`: helper scripts for web-dev and benchmarks.
- `skills/`: installable agent skill docs and assets.
- `tests/`: benchmark tools and fixtures.
- `third_party/`: vendored dependencies, currently `go-nfs`.
- `ui/`: React/TanStack control-plane UI.

## Root Files

- `.gitignore`: ignores built binaries, local config, copied UI assets, Python artifacts, and editor/test junk.
- `AGENTS.md`: repo guidance for coding agents.
- `Dockerfile`: self-hosted container build for the active control-plane path.
- `LICENSE`: AGPLv3 project license.
- `Makefile`: top-level orchestration for mount helpers, command binaries, tests, web-dev, and skill installation.
- `README.md`: current product story and user-facing usage docs for the workspace-first AFS model.
- `SKILL.md`: short root-level pointer for agents using this repo.
- `example.afs.config.json`: sample CLI config for the current workspace-first flow.
- `go.mod` / `go.sum`: root Go module definition and lockfile for the workspace/control-plane app.

## Active Code Areas

### `cmd/afs/`

- CLI entrypoint, setup flow, sync lifecycle, workspace/checkpoint commands, mount backend selection, and the built-in MCP surface.

### `cmd/afs-control-plane/`

- HTTP control-plane server binary that exposes the workspace/checkpoint/session API used by the web UI.

### `internal/controlplane/`

- Service, HTTP, catalog, onboarding, auth, and Redis persistence logic for workspaces, checkpoints, sessions, and activity.

### `internal/worktree/`

- Manifest scanning, blob collection, and local materialization helpers.

### `mount/`

- The Redis-backed filesystem client plus the FUSE and NFS daemons.
- `mount/internal/client/` is the core storage adapter.
- `mount/internal/afsfs/` and `mount/internal/nfsfs/` expose that client through FUSE and NFS.

### `ui/`

- TanStack Router + React control-plane UI.
- `ui/src/foundation/api/afs.ts` is the main browser API client.
- `ui/src/routes/docs.tsx` and `ui/public/agent-guide.md` are the main end-user docs surfaces embedded in the app.

### `sandbox/`

- Isolated process runner service and CLI.

## Notes And Working Material

### `docs/`

- `docs/README.md`: index of current docs and where non-current material belongs.
- `docs/afs-control-plane-api.md`: current shared HTTP contract for the CLI and web UI.
- `docs/backlog/`: active implementation backlog notes.
- `docs/performance.md`: consolidated benchmark and performance notes.
- `docs/plans/`: longer-lived design proposals that are not yet canonical contracts.
- `docs/repo-walkthrough.md`: this file.

### Planning Material

- Active backlog notes live under `docs/backlog/`.
- Design proposals that may or may not be implemented yet live under `docs/plans/`.
- Treat both as helpful context, not as authoritative product contracts.
- Raw benchmark CSV/JSON outputs should be rerun into `/tmp` or another external artifact directory.

### `tests/`

- `tests/bench_afs_grep.py`: benchmark comparing `afs fs grep` against mounted GNU `grep`.
- `tests/create-test-memories`: shell helper that generates realistic markdown-memory trees.
- `tests/bench/` and `tests/bench_md_workloads/`: synthetic workload and benchmark programs.

## What I Would Read First

If you want the shortest path to the current architecture, read in this order:

1. `README.md`
2. `AGENTS.md`
3. `cmd/afs/main.go`
4. `internal/controlplane/store.go`
5. `internal/controlplane/service.go`
6. `internal/worktree/manifest.go`
7. `mount/internal/client/native_core.go`
8. `mount/cmd/agent-filesystem-nfs/main.go`
9. `ui/src/foundation/api/afs.ts`

That sequence explains the current product model first, then the persistence layers, then the local-surface and UI implementations.
