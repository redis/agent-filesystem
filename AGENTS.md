# AGENTS.md

This file provides guidance to Codex and other AI coding agents working in this repository.

**This is a living document.** When you learn a new repo-specific sharp edge,
workflow rule, or recurring mistake, add it here under the most relevant
section. Do not create a separate lessons file.

## Quick Links

- [README.md](README.md)
- [docs/README.md](docs/README.md)
- [docs/internals/repo-walkthrough.md](docs/internals/repo-walkthrough.md)
- [docs/reference/control-plane-api.md](docs/reference/control-plane-api.md)
- [ui/README.md](ui/README.md)
- [docs/internals/cloud.md](docs/internals/cloud.md)
- [plans/README.md](plans/README.md)
- [plans/future-work.md](plans/future-work.md)

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
./afs status
./afs ws mount <workspace> <directory>
./afs ws unmount <workspace-or-directory>
./afs ws import <workspace> <directory>
./afs cp list <workspace>

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

## Documentation Organization

- Keep `docs/` simple: `guides/`, `reference/`, and `internals/`.
- `docs/` is for current app/repo truth only. Do not use it for active plans,
  stale proposals, backlog trackers, or "maybe later" notes.
- Current user-facing and agent-facing docs live in `docs/guides/`.
- Current CLI/API/SDK/MCP contracts live in `docs/reference/`.
- Current architecture, repo map, and performance notes live in
  `docs/internals/`.
- Future work and active implementation planning live under root `plans/`.
- Do not reintroduce `docs/plans/`, `docs/proposals/`, `docs/backlog/`, or
  top-level `tasks/`.
- When work lands, update the current docs and remove stale notes from
  `plans/future-work.md` or the relevant active plan.
- Accepted architecture decisions belong in `docs/internals/decisions/` as
  short ADRs with status, context, decision, and consequences.
- Raw benchmark output belongs outside the repo, usually under `/tmp`; summarize
  durable conclusions in `docs/internals/performance.md`.
- If UI docs links point at GitHub markdown files, keep them in sync with the
  `docs/guides/` and `docs/reference/` paths.

## Planning Artifacts

- Root `plans/` is the canonical place for Codex, Claude, and human
  implementation plans.
- Active plans live directly under `plans/<slug>.md`.
- Completed, cancelled, and superseded plans move to
  `plans/archive/YYYY-MM-DD-<slug>.md`.
- Keep `plans/future-work.md` for known work that is not actively being
  implemented.
- Active plans must track status, owner, created/updated dates, goal, scope,
  checklist, what is in flight, what remains, decisions/blockers, and
  verification.
- Update the active plan as work progresses. It should be possible to resume the
  task from the plan without replaying the chat.
- Before archiving a completed plan, add the result and verification evidence.
- Plans are not product documentation. If a plan changes current behavior,
  update `docs/` separately.

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

Future work and active plans live under root `plans/`. Raw benchmark outputs
should stay outside the repo.

For a file-by-file walkthrough of the current tree, read
`docs/internals/repo-walkthrough.md`.

The old Redis module, its Python integration suite, and RedisClaw have been retired and should not be treated as active architecture.

## Architecture Summary

The most important implementation seams are:

- `cmd/afs/`: CLI command surface, setup flow, sync lifecycle, local UX.
- `cmd/afs-control-plane/`: HTTP control plane binary.
- `internal/controlplane/`: workspace, checkpoint, session, catalog, and HTTP service logic.
- `internal/worktree/`: manifest scanning and local materialization helpers.
- `mount/internal/client/`: Redis-backed filesystem client used by FUSE/NFS.
- `ui/`: TanStack Router + React control-plane UI.

## Lessons Learned

- Tenant-scoped client routes must run through the same auth middleware as admin
  routes before they resolve workspace names. Otherwise bearer tokens do not
  attach an auth subject and duplicate workspace-name errors can expose
  cross-tenant identifiers.
- Auth commands belong under `afs auth`; keep login/logout/status under that
  family in help text, docs, and install scripts.
- Plain `afs auth login` should ask Cloud vs Self-managed before opening a
  browser login. Keep `--cloud`, `--self-hosted`, and token handoff
  noninteractive for scripted install paths.
- Benchmark helpers that open the Redis filesystem client directly must resolve
  the workspace storage ID after import. New imports use opaque workspace IDs,
  so using the human workspace name can silently point at an empty namespace.
- Build versions must use the AFS product tag namespace. Keep SDK tag names out
  of `git describe` paths for CLI/control-plane releases.
- Sync-mode file writes only reach the user-facing changelog when the daemon has
  a tracked workspace session id. If the UI shows no active agents, inspect
  session creation before debugging uploader logic.
- Local mount state and control-plane agent sessions are different views:
  `~/.afs/mounts.json` is what `afs status` and `afs ws unmount` can manage
  locally; `/v1/agents` shows fresh session heartbeats.
- Browser/UI `draft_state` must come from the live workspace root dirty marker
  when it exists, not only `WorkspaceMeta.DirtyHint`.
- Remounting a workspace to an empty path with prior sync state is ambiguous.
  Treat a missing local root as a fresh mount; require explicit destructive
  confirmation before propagating local absence as remote deletes.
- When a mount error names a path like `rm remote docs`, that path is a
  workspace-relative entry under the selected mount root, not a separate local
  mount target.
- TanStack route files should only export their `Route`. Move shared route UI
  into `ui/src/features/` instead of importing from another route file.
- Template source files live under `templates/<template-id>/`. After changing
  template manifests, seed files, skills, or commands, run
  `npm run templates:generate` from `ui/` or `make templates-generate`.
