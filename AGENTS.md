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
./scripts/test_harness.py        # interactive test/benchmark catalog and runner
./scripts/test_harness.py --list # print the indexed runnable entries

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
- Use `./scripts/test_harness.py` when you need to discover the current runnable suites, package tests, benchmarks, or smoke scripts before picking a validation command.

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

- Search/BM25 promotion in the Cloud UI should stay restrained and
  operational: prefer compact status text in existing workspace and monitor
  surfaces over extra badge rows or standalone promo cards.
- New UI pages must start from existing Redis UI controls and local shared
  components such as `Button`, `Table`, `Menu`, `TableHeading.SearchInput`, and
  `ui/src/foundation/tables/workspace-table.styles.ts`. Do not invent bespoke
  cards, inputs, rows, or CSS unless the needed component does not already exist
  in the project.
- Agent-profile editing should use full pages with a breadcrumb back to the
  list, not drawer-based flows, while preserving the same Filesystem / Tokens /
  Settings structure.
- The `/workspaces` UI is "Agent Workspaces" in page copy. On the list, the
  mounted storage column is "Volumes" and counts should say volume/volumes, not
  workspace/workspaces. Agent rows should use the agent/Bot icon with the
  neutral table icon treatment, not colored avatars or letter-only avatars.
- Agent Workspace editor pages should use standard input components and compact
  table-like summaries. Avoid prototype-only borderless inputs, big metric
  cards, and explanatory workspace cards in this flow.
- Mounted folder rows in the Agent Workspace editor should read as filesystem
  children under `/`, with indentation and connector lines from the root row.
- Agent Workspace editor pages should keep the editable content, tabs, body,
  and footer inside one main card below the breadcrumb.
- The Agent Workspace filesystem action is "Add Volume". It opens a multi-select
  wizard for existing volumes plus permissions, not an immediate "Add shared
  folder" shortcut.
- The Agent Workspace filesystem section should present a left-aligned
  "Volumes" section title with the "Add Volume" action aligned to the right,
  matching other table/action headers.
- Volume/detail tab bars must stay inside their card. Use short tab labels and
  horizontal tab scrolling instead of allowing tab controls to overflow or wrap.
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
- Active-agent UI fixes may need both the Agents table and the Monitor compact
  card. The Monitor page active-agents card lives in `ui/src/routes/index.tsx`
  and renders relative time in `AgentSeen`.
- Do not sort active-agent lists by `lastSeenAt`; heartbeat/time updates make
  rows jump around. Use a stable identity/display label sort for live lists.
- Template source files live under `templates/<template-id>/`. After changing
  template manifests, seed files, skills, or commands, run
  `npm run templates:generate` from `ui/` or `make templates-generate`.
- This repo has multiple nested Go modules (`mount/`, `sandbox/`,
  `third_party/go-nfs`, and `tests/bench`). For lightweight local discovery,
  prefer filesystem scanning over `go list` across the whole tree because
  `go list` can trigger dependency downloads in those nested modules.
- `afs setup` owns only the default local mode prompt. Workspace selection and
  local directory prompts belong under `afs ws mount`, not setup.
- When `afs setup` selects Live Mount, it must still save a concrete mount
  backend. `mode=mount` with `runtime.mount.backend=none` is invalid and makes
  `afs mount <workspace>` fail before workspace resolution.
- Live mount startup must treat the workspace selected by `afs mount` as an
  explicit target. Do not let CWD/single-mounted-workspace fallbacks override
  that selection during session bootstrap.
- Live NFS/FUSE mounts overlay the local directory; they do not materialize or
  clean up workspace files. Guard against mounting over populated directories
  unless the user explicitly opts in, or old sync copies will reappear after
  unmount and look like copied live-mount contents.
- If live mount startup creates the mountpoint directory, persist that fact in
  the mount registry and remove the empty mountpoint on unmount. Preserve
  user-created directories.
- `example.afs.config.json` should mirror the canonical persisted config shape:
  an empty `workspace.default` to force an explicit user choice; no root
  `currentWorkspace` or `localPath` keys.
- If a Vite UI change is present in source but `localhost:5173` still shows old
  behavior, check for duplicate Vite listeners from sibling worktrees.
  `localhost` can hit an IPv6 listener from another checkout while
  `127.0.0.1` hits the current repo.
- Control-plane import/checkpoint JSON fields typed as `map[string][]byte`
  expect base64-encoded string values. Small synthetic manifests can avoid that
  by using `ManifestEntry.inline`, which is already base64 text.
- `afs status` can show running mounts from multiple product modes/databases.
  When a command scopes to the current config, explain when a status-visible
  mount belongs to another config instead of returning a bare "does not exist".
- Top-level filesystem shortcuts like `afs grep` and `afs query` must route
  through the `afs fs` command path. Do not send them to older local-only
  helpers, or Cloud/Self-managed users will see local Redis errors instead of
  control-plane behavior.
- Top-level filesystem shortcut help must say shortcuts use the "default"
  workspace and that explicit targeting requires `afs fs <workspace> <command>`.
- CLI error rendering must preserve help and usage blocks literally. Do not
  sentence-format, title-case, or append punctuation to command names, flags,
  examples, or subcommand lists.
- Sync mount cold-start in Cloud-managed mode must hydrate from the workspace
  session's storage key/head checkpoint. Do not require direct Redis workspace
  metadata lookup by display name; cloud session Redis may expose the live root
  while metadata lives behind the control plane.
- Preserve the two-command search UX: `grep` is exact text evidence and `query`
  is the powerful ranked retrieval command. `query` defaults to hybrid + rerank,
  supports `lex:`, `vec:`, `hyde:`, and `intent:` documents, and uses
  `--keyword` / `--semantic` for narrower modes. Do not reintroduce public
  `search` or `vsearch` commands unless the product direction changes again.
- On the Volume details page, the merged file/lifecycle timeline is titled
  `History`. Do not expose `Changelog` as a peer tab or section title there.
- Semantic query embeddings must use a real provider. QMD uses a local GGUF
  embedding model with explicit query/document formatting; deterministic hash
  embeddings are acceptable only as test doubles, never as product behavior.
- Pure local embedding model work means downloading and caching the GGUF
  artifact itself. Do not substitute Ollama or another local service shim when
  the requirement is "pure GGUF".
- Local GGUF embeddings are an explicit provider override using a managed Node
  helper with `node-llama-cpp`, matching QMD's model lifecycle. Do not swap this
  back to per-call `llama-embedding` process invocations.
- Semantic query provider/model settings are global runtime settings, not
  workspace config. Embeddings should be treated as on by default; if provider
  runtime dependencies are missing, return/report unavailable status without
  hard-failing normal query flows.
- `AFS_EMBED_MODEL`, `AFS_EMBED_PROVIDER`, `AFS_EMBED_DIMENSIONS`, and
  `OPENAI_API_KEY` are read by the control-plane process. CLI help and
  troubleshooting copy must not imply that setting them only on an `afs query`
  invocation changes an already-running control plane.
- Semantic embedding backfill must batch provider requests. Query chunks are
  individually capped, but a large workspace can still exceed OpenAI's total
  tokens-per-request limit if every pending chunk is embedded at once.
- Semantic query can take longer than normal control-plane calls on first
  provider backfill. Keep query HTTP client timeouts separate from quick
  metadata/status calls so first-run embedding work does not fail at 30s.
- Semantic query must not backfill embeddings as a side effect. Imports should
  start embedding creation in the control plane when the global provider is
  available, and existing workspaces should use an explicit query index create
  path for embedding backfill.
- Workspace file/query CLI calls use resolved workspace routes under
  `/v1/workspaces/<id>/...`; when adding a scoped database route, add the
  matching resolved route and a regression test for workspace IDs.
- `afs ws mount <workspace>` must resolve the Agent Workspace manifest before
  prompting for a local folder. Missing manifests should produce an Agent
  Workspace error with `afs ws list`/`afs ws create` guidance, not a bare
  file-not-found error.
- Root `afs mount` and `afs unmount` are Agent Workspace shortcuts. They should
  behave like `afs ws mount`/`afs ws unmount`, including no-arg prompts that
  list Agent Workspaces, not raw volumes. Direct volume mounting remains under
  `afs vol mount`/`afs vol unmount`.
- `afs ws mount` should print one Agent Workspace summary. Suppress child
  `Volume mounted` sections from the underlying per-volume mounts and show the
  actual local mounted volume paths with read-only/read-write permissions and
  file counts. Do not display logical `/workspace/volume` paths as if they are
  filesystem roots.
- The root `afs help` screen should stay concise. Do not reintroduce a
  `Common Flows` section there.
- `afs status` must aggregate child volume mount records tagged with
  `agent_workspace_root` into one mounted Agent Workspace row. Do not list those
  child volumes under Mounted workspaces; direct volume mounts belong in a
  separate Mounted volumes section.
- Do not restart the user's running control plane automatically. Rebuild
  binaries when needed, but let the user restart `afs-control-plane`.
- Self-managed `afs auth login --access-token` must preserve the CLI bearer
  token. Do not normalize self-managed access-token logins into unauthenticated
  local-control-plane access, or workspace-scoped mount tokens can list and
  mount workspaces outside their scope.
- `afs tokens create --workspace <name>` is for Agent Workspace mounts. It must
  mint against the Agent Workspace manifest route and authorize sessions only
  for volumes attached to that manifest; do not route this command through the
  lower-level volume workspace token path.
