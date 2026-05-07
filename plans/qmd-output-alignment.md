# QMD Output Alignment

Status: active
Owner: Codex
Created: 2026-05-07
Updated: 2026-05-07

## Goal

Line up AFS CLI retrieval output with the useful QMD defaults while preserving
AFS workspace-first naming.

## Scope

- Add QMD-style query output formats: files, csv, xml, and empty-format behavior.
- Keep old path-only query listing available as `--paths`.
- Add QMD-style `fs get` and `fs multi-get` retrieval commands.
- Add machine-readable workspace/file listing where cheap and agent-useful.
- Do not restart the control plane. Tell the user when restart is required.

## Checklist

- [x] Query output formats and tests.
- [x] `fs get` line-slice command and tests.
- [x] `fs multi-get` glob/comma command and tests.
- [x] `fs ls --json/--files` and tests.
- [x] `ws list --json` and tests.
- [x] Help/docs updates.
- [x] Verification and rebuild.

## Verification

- `go test ./cmd/afs -run 'TestFSRemoteCommands|TestParseFSDispatchArgs|TestParseWorkspaceQueryArgs|TestWriteWorkspaceQueryResponse|TestWorkspaceQueryUsage|TestCmdQueryContract|TestWorkspaceCommandsImportCloneForkListAndDelete|TestWorkspaceList' -count=1`
- `go test ./internal/querysearch ./internal/queryindex ./internal/controlplane -run 'Query|WorkspaceQuery|QueryIndexStatus|ResolvedWorkspaceQueryRoutes|InspectDoesNotDoubleCountPendingFilesAsStale|ExtractSnippet' -count=1`
- `go test ./cmd/afs -count=1`
- `git diff --check`
- `make commands`
- `make afs`
- `./afs query --help`
- `./afs fs get --help`
- `./afs fs multi-get --help`
- `./afs ws list --help`

## Result

AFS query now has QMD-style default snippets and output formats (`--files`,
`--csv`, `--md`, `--xml`) while preserving path-only output as `--paths`.
AFS also has QMD-style targeted retrieval via `afs fs get` and batch retrieval
via `afs fs multi-get`, plus machine-readable `afs fs ls --json/--files` and
`afs ws list --json`.

The rebuilt `afs-control-plane` binary includes the compact RedisSearch snippet
backend. The user must restart the running control plane to pick that up.
