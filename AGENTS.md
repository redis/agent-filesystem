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

- One workspace owns one file tree and its checkpoints. Redis is the canonical
  store for workspace metadata, manifests, blobs, checkpoints, and activity.
- The former volume is the workspace; preserve its storage ID. Composition
  records are migration evidence, not public workspaces. `/v2` composition
  routes retire with HTTP 410 and must never reinterpret composition IDs.
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
make templates-generate  # regenerate UI template data from templates/

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
- Template changes: run `make templates-generate`, then the relevant UI build/test command if generated template data changes.
- TypeScript SDK changes: run `npm --prefix sdk/typescript run check` and `npm --prefix sdk/typescript test`.
- Python SDK changes: run `PYTHONPATH=sdk/python/src python3 -m unittest discover -s sdk/python/tests`.
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
- `examples/`: live examples, sample configs, and reference plugin packages.
- `plugins/`: installable client plugin packages, including the baseline AFS plugin.
- `sandbox/`: isolated process runner.
- `sdk/`: TypeScript and Python SDKs for the control plane and agent filesystem mounts.
- `scripts/`: helper scripts for local development and benchmarks.
- `skills/`: installable skill docs for agent use.
- `templates/`: workspace templates, seed files, skills, commands, and manifests that feed generated UI template data.
- `tests/`: benchmark helpers and fixtures for the active workspace-first surfaces.
- `third_party/go-nfs/`: vendored NFS library used by the NFS server binary.

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
- `internal/mcptools/` and `internal/mcpproto/`: shared MCP contracts and stdio/hosted tool support.
- `internal/queryindex/`, `internal/querysearch/`, `internal/queryvector/`, and
  `internal/queryembedding/`: keyword query projection, fallback ranking,
  semantic retrieval, and embedding provider/runtime logic.
- `internal/worktree/`: manifest scanning and local materialization helpers.
- `mount/internal/client/`: Redis-backed filesystem client used by FUSE/NFS.
- `sdk/`: TypeScript and Python agent-facing SDKs.
- `ui/`: TanStack Router + React control-plane UI.

## Lessons Learned

- Sync must carry the revision from its verified remote snapshot through the
  Redis mutation and record the returned commit stat. A post-write Stat can
  belong to a peer, and a separate unguarded chmod can modify that peer's edit.
  Compare stored revisions as well as timestamps during recovery scans.
- Redis COPY must use the client's selected database; hard-coding database zero
  can move staging outside the workspace and leave a partial delta candidate.

- Read-time materialization and MCP refresh must not clear the live dirty
  marker or rewrite a previously fetched metadata record. Peer checkpoints
  and writes may have moved the tree; only the guarded live checkpoint save
  may acknowledge its captured generation and journal position.
- Exact grep must bypass the legacy inode search projection for trees with a
  generation marker. Current filesystem clients do not maintain those grep
  fields; an old ready marker can miss edits, and unguarded backfill can
  recreate an inode after restore. Use the published-tree scan until the
  projection has its own guarded revision protocol.
- After save stops a sync generation, a failed save must request recovery on
  the replacement watcher. Cancelled debounce timers and queued work will not
  produce another event by themselves. Keep tracked upload queue sends
  cancellation-aware so save can join the old generation.
- Explicit save mutations must use the uploader's session History/version
  recorder and the bytes actually sent to Redis. Record each completed mutation
  before continuing; retries should compare observed remote state to avoid
  duplicate rows. Preserve ownership of partially created directories/symlinks
  when their final chmod fails.
- Sync save completion must join every previous worker generation and delayed
  sender before scanning or acknowledging a tree. Queue emptiness alone does
  not establish completion. Verify remote bytes and keep the stored hash format
  compatible with the chunk metadata used by subsequent background edits.
  Cancel inbound replacements before they change the local tree, and compare
  the local tree before and after draining active work.

- Chunk metadata lookup can return `redis.Nil` for a missing remote file.
  Chunked sync uploads must recreate all local chunks when the remote file is
  absent, including chunks unchanged since the previous sync.
- Search/BM25 promotion in the Cloud UI should stay restrained and
  operational: prefer compact status text in existing workspace and monitor
  surfaces over extra badge rows or standalone promo cards.
- New UI pages must start from existing Redis UI controls and local shared
  components such as `Button`, `Table`, `Menu`, `TableHeading.SearchInput`, and
  `ui/src/foundation/tables/workspace-table.styles.ts`. Do not invent bespoke
  cards, inputs, rows, or CSS unless the needed component does not already exist
  in the project.
- Workspace pages retain the existing tree browser, content viewer, History,
  checkpoints, and settings. Do not reintroduce attachment editors, composed
  manifests, volume resources, or per-volume mount paths.
- Workspace/detail tab bars must stay inside their card. Use short tab labels
  and horizontal tab scrolling instead of allowing tab controls to overflow.
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
  is the powerful ranked retrieval command. Plain `query` sends hybrid mode
  with rerank auto, currently falls back to keyword-ranked results until hybrid
  vector/rerank is complete, supports `lex:`, `vec:`, `hyde:`, and `intent:`
  documents, and uses `--keyword` / `--semantic` for narrower modes. Do not
  reintroduce public `search` or `vsearch` commands unless the product direction
  changes again.
- On the Workspace details page, the merged file/lifecycle timeline is titled
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
- Semantic index creation/backfill can take longer than normal control-plane
  calls. Keep query HTTP client timeouts separate from quick metadata/status
  calls so explicit embedding work does not fail at 30s.
- Semantic query must not backfill embeddings as a side effect. Imports should
  start embedding creation in the control plane when the global provider is
  available, and existing workspaces should use an explicit query index create
  path for embedding backfill.
- Workspace file/query CLI calls use resolved workspace routes under
  `/v1/workspaces/<id>/...`; when adding a scoped database route, add the
  matching resolved route and a regression test for workspace IDs.
- `afs ws mount <workspace>` and root `afs mount` connect one workspace tree
  directly to one local directory. The unmount shortcuts target that same tree
  or local path. Prompts list independent workspaces.
- `afs ws` owns create/import/list/show/config/fork/delete/save and mounts.
  Public `vol`, attachment, and bookmark commands have been removed.
- The root `afs help` screen should stay concise. Do not reintroduce a
  `Common Flows` section there.
- `afs status` shows each tree mount as a workspace, including old registry
  entries with composition tags; keep each actual local path addressable.
- Do not restart the user's running control plane automatically. Rebuild
  binaries when needed, but let the user restart `afs-control-plane`.
- Self-managed `afs auth login --access-token` must preserve the CLI bearer
  token. Do not normalize self-managed access-token logins into unauthenticated
  local-control-plane access, or workspace-scoped mount tokens can list and
  mount workspaces outside their scope.
- `afs tokens create --workspace <name>` uses the v1 workspace CLI-token route.
  The token authorizes only that tree and must preserve read-only restrictions.
  Never promote old composition tokens into unrestricted tree access.
- SDK `fs.mount` accepts one `workspace`, exposes root-relative file API paths,
  and materializes directly into one local directory. Shell commands use normal
  relative paths in that directory; there is no virtual workspace-name prefix.
- Server checkpoints capture published Redis state and do not flush every
  client's pending local files. Local save and checkpoint creation are separate
  operations.
- Watcher overflow recovery must use a notification channel separate from the
  saturated event queue. Consume the request before scanning so losses during
  recovery schedule another pass. Refresh native directory watches and use a
  warm scan; cold hydration can replace an unsynced tree containing only hidden
  paths such as `.venv`.
- After a reconciliation upload, record the remote modification time from
  `Stat`. Substituting the local timestamp can create a false remote change
  and turn a later local edit into a conflict during recovery.
- Full reconciliation must keep local directories writable while applying
  descendant changes, including existing read-only directories with no mkdir
  action. Restore modes deepest-first after workers join, even on failure or
  cancellation, and preserve application chmods or directory replacements.

Overflow scans must defer files with queued uploads until their results update
the baseline. An existing remote inode can still contain a partial chunked
upload. Deferred scans in a running daemon must stay on the warm merge path
and retain recovery retries. Keep the uploaded snapshot's local timestamp
separate from its Redis timestamp, preserve hashes during metadata refresh,
and verify content and mode before creating a conflict copy.

When a rename stages an edited destination, capture the final provisional
baseline version before enqueueing the rename. A missing remote path followed
by a failed or obsolete content upload must recover the local destination;
it must not treat that provisional baseline as proof of a remote deletion.

- A production UI served on localhost uses its serving origin for API calls.
  The localhost `:8091` fallback belongs only to Vite development mode.
- Workspace route loaders use `loaderDeps.databaseId`; `search` is not a loader
  argument. Wait for authentication readiness before warming detail routes.
- Live Topology shows independent workspace nodes without a shared workspace
  bounding box. Keep host grouping for agents; connection endpoints attach to
  individual workspace nodes.
- Import-lock heartbeats are advisory. Root cleanup, materialization, head
  markers, and final generation publication must atomically validate both the
  import token and replacement generation so a paused worker cannot overwrite
  a newer tree after losing its lease.
- A checkpoint can clear the root dirty marker only if its head, generation,
  and captured mutation journal still match. Publish dirty markers and journal
  entries atomically with mutations, including chmod and namespace operations;
  disabling Pub/Sub must not disable the durable journal.
- File mutation and read-refresh paths must not write a previously read whole
  workspace metadata record back to Redis. Update the latest metadata under
  concurrency checks so a concurrent checkpoint or restore head survives.
