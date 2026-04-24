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
- Keep docs, active backlog notes, and long-form plans under the single `docs/`
  root. Raw benchmark output belongs in `/tmp` or another artifact directory,
  with durable conclusions summarized in `docs/performance.md`.

## Git & Shell

- Quote route filenames that contain `$` when running shell or git commands in
  `zsh`, for example `git add "ui/src/routes/workspaces.$workspaceId.tsx"`.

## UI Semantics

- In the workspace browser, treat `head` as the single canonical label for the
  current saved checkpoint. Do not also surface that same checkpoint by name,
  and only show `working-copy` when the live draft actually differs from head.

## Plugin Layout

- Keep the baseline Codex AFS plugin at `plugins/agent-filesystem/`, with plugin
  and MCP server name `agent-filesystem`; make it secret-free by reading
  control-plane tokens through `bearer_token_env_var`.
- For the Codex AFS plugin, cloud vs localhost is an endpoint choice in
  `.mcp.json` (`url`), while the token stays outside the plugin in
  `AFS_CONTROL_PLANE_TOKEN`.
