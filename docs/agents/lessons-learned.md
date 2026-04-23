# Lessons Learned

Add short, concrete notes here when you hit a repo-specific sharp edge that future agents should not have to rediscover.

## Build & Runtime

- If UI assets matter, do not rely on plain `go build ./cmd/afs-control-plane`. Use a path that rebuilds embedded UI assets such as `make afs-control-plane`, `make web-build`, or `make embed-ui`.
- `cmd/afs/auth_commands.go` treats cloud-vs-self-managed login as a hostname allowlist problem. When the public cloud domain changes, update that list alongside the default cloud URL or `afs login --url <new-cloud-host>` will silently route users into the self-managed flow.

## Git & Shell

- Quote route filenames that contain `$` when running shell or git commands in `zsh`, for example `git add "ui/src/routes/workspaces.$workspaceId.tsx"`.

## UI Semantics

- In the workspace browser, treat `head` as the single canonical label for the current saved checkpoint. Do not also surface that same checkpoint by name, and only show `working-copy` when the live draft actually differs from head.
