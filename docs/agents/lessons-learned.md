# Lessons Learned

Add short, concrete notes here when you hit a repo-specific sharp edge that
future agents should not have to rediscover.

## Build & Runtime

- If UI assets matter, do not rely on plain `go build ./cmd/afs-control-plane`.
  Use a path that rebuilds embedded UI assets such as `make afs-control-plane`,
  `make web-build`, or `make embed-ui`.
- `cmd/afs/auth_commands.go` treats cloud-vs-self-managed login as a hostname
  allowlist problem. When the public cloud domain changes, update that list
  alongside the default cloud URL or `afs login --url <new-cloud-host>` will
  silently route users into the self-managed flow.
- Plain `afs login` should ask Cloud vs Self-managed before opening any browser
  login. Keep `--cloud`, `--self-hosted`, and token handoff noninteractive for
  scripted install paths.
- Keep docs, active backlog notes, and long-form plans under the single `docs/`
  root. Raw benchmark output belongs in `/tmp` or another artifact directory,
  with durable conclusions summarized in `docs/performance.md`.
- Benchmark helpers that open the Redis filesystem client directly must resolve
  the workspace storage ID after import. New imports use opaque workspace IDs,
  so `client.New(rdb, workspaceName)` can silently point at an empty namespace.

## Git & Shell

- Quote route filenames that contain `$` when running shell or git commands in
  `zsh`, for example `git add "ui/src/routes/workspaces.$workspaceId.tsx"`.
- If `afs login` reports `Unknown command: login`, check whether the user is
  running a stale local `./afs` binary from an old checkout or worktree. The
  current PATH install should print `login` in `afs --help`; rebuild or use the
  repo-root binary before debugging auth flow code.

## UI Semantics

- In the workspace browser, treat `head` as the single canonical label for the
  current saved checkpoint. Do not also surface that same checkpoint by name,
  and only show `working-copy` when the live draft actually differs from head.
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
