# Lessons Learned

Add short, concrete notes here when you hit a repo-specific sharp edge that
future agents should not have to rediscover.

## Build & Runtime

- If UI assets matter, do not rely on plain `go build ./cmd/afs-control-plane`.
  Use a path that rebuilds embedded UI assets such as `make afs-control-plane`,
  `make web-build`, or `make embed-ui`.
- Tenant-scoped client routes must run through the same auth middleware as
  admin routes before they resolve workspace names. Otherwise bearer tokens do
  not attach an auth subject and duplicate workspace-name errors can expose
  cross-tenant workspace/database identifiers.
- Auth commands are nested under `afs auth`. Keep authentication
  login/logout/status under that command family; top-level `afs status`
  remains the workspace mount status. Use `afs auth login`,
  `afs auth logout`, and `afs auth status` in help text, docs, and install
  scripts.
- `cmd/afs/auth_commands.go` treats cloud-vs-self-managed login as a hostname
  allowlist problem. When the public cloud domain changes, update that list
  alongside the default cloud URL or `afs auth login --url <new-cloud-host>`
  will silently route users into the self-managed flow.
- Plain `afs auth login` should ask Cloud vs Self-managed before opening any
  browser login. Keep `--cloud`, `--self-hosted`, and token handoff
  noninteractive for scripted install paths.
- Keep docs, active backlog notes, and long-form plans under the single `docs/`
  root. Raw benchmark output belongs in `/tmp` or another artifact directory,
  with durable conclusions summarized in `docs/performance.md`.
- Benchmark helpers that open the Redis filesystem client directly must resolve
  the workspace storage ID after import. New imports use opaque workspace IDs,
  so `client.New(rdb, workspaceName)` can silently point at an empty namespace.
- Build versions must use the AFS product tag namespace. Use `vX.Y.Z` or
  `afs-vX.Y.Z` tags for CLI/control-plane releases and keep SDK tags such as
  `redis-afs-python-vX.Y.Z` out of `git describe` paths.
- Sync-mode file writes only reach the user-facing changelog when the daemon
  has a tracked workspace session id. If the web UI shows no active agents and
  file changes only appear in the low-level `:changes` stream, inspect session
  creation before debugging the uploader.
- Local mount state and control-plane agent sessions are different views.
  `~/.afs/mounts.json` is what `afs status`/`afs ws unmount` can manage
  locally; `/v1/agents` shows every fresh session heartbeat, including daemons
  that are no longer in the local mount registry.
- `afs status` should lead with daemon liveness (`AFS Running`/`AFS Not
  Running`). Stopped mount-registry rows are cleanup records, not mounted
  workspaces. Cross-check the local process table for `_sync-daemon` so
  registry drift cannot hide unmanaged live daemons.
- Browser/UI `draft_state` must come from the live workspace root dirty marker
  when it exists, not only `WorkspaceMeta.DirtyHint`. A stale clean
  `DirtyHint` can hide `working-copy` even though the Redis root contains
  unsaved files.
- Remounting a workspace to an empty path with prior sync state is
  ambiguous. A missing local root should be treated as a fresh mount and
  download from the workspace; an existing empty root needs an explicit
  destructive confirmation before propagating local absence as remote deletes.

## Git & Shell

- Quote route filenames that contain `$` when running shell or git commands in
  `zsh`, for example `git add "ui/src/routes/workspaces.$workspaceId.tsx"`.
- If `afs auth login` reports `Unknown command: auth`, check whether the user
  is running a stale local `./afs` binary from an old checkout or worktree. The
  current PATH install should print `auth` in `afs --help`; rebuild or use the
  repo-root binary before debugging auth flow code.

## UI Semantics

- Signed-out public routes must not use route loaders that prefetch protected
  workspace, database, or agent APIs. Keep the public landing/docs path
  renderable before auth, then enter the authenticated app shell after sign-in.
- In the workspace browser, use "Active workspace" for the server-side live
  filesystem state. Keep `head` and `working-copy` as internal/API view names:
  when active state is dirty, active resolves to `working-copy` and checkpoints
  should remain visible as saved states below a divider.
- Checkpoint restore is an explicit active-workspace replacement, not a reason
  to block normal clients. For sync clients, preflight the local tree for open
  handles, create the safety checkpoint server-side when dirty, publish a
  root-replace invalidation, and rematerialize the local folder from Redis as
  authoritative.
- TanStack route files should only export their `Route`. If login/signup or
  other sibling routes need shared UI, move the shared component and search
  validators into `ui/src/features/` instead of importing from another route
  file, or the dev server will warn that those exports cannot be code-split.

## Plugin Layout

- Keep the baseline Codex AFS plugin at `plugins/agent-filesystem/`, with plugin
  and MCP server name `agent-filesystem`; make it secret-free by reading
  control-plane tokens through `bearer_token_env_var`.
- For the Codex AFS plugin, cloud vs localhost is an endpoint choice in
  `.mcp.json` (`url`), while the token stays outside the plugin in
  `AFS_CONTROL_PLANE_TOKEN`.

## Workspace Templates

- Template source files live under `templates/<template-id>/`. After changing
  manifest, seed, skill, or command files, run `npm run templates:generate`
  from `ui/` or `make templates-generate` before validating UI behavior.
