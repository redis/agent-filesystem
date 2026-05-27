# LiveSkills Agent Guide

## What This Repo Is

LiveSkills is a Go CLI for installing agent skills through an AFS-backed registry.

The normal user workflow should stay dead simple:

```bash
liveskills add <source>
liveskills list
liveskills add -g <source>
liveskills list -g
```

Do not make users think about publish/register in the happy path. `add` should register/version the skill in AFS if needed, then install it.

## Core Product Rules

- Match the `skills` CLI mental model where possible: project installs are default, `-g` / `--global` means user-level global install.
- `list` shows project skills by default. If none exist, print the hint to try `-g`.
- `list -g` shows global skills.
- Keep `publish`, `show`, and `download` available only as advanced/debug/export workflows.
- A skill install is one skill folder, not ownership of the whole skills parent directory.
- Project installs go under `.agents/skills/<skill-slug>` by default.
- Global Codex installs go under `~/.codex/skills/<skill-slug>`.
- Claude Code installs go under `.claude/skills/<skill-slug>` for project scope.
- LiveSkills must preserve neighboring regular downloaded skills under `.agents/skills`.

## AFS Boundary

AFS supports direct volume mounts:

```bash
afs vol mount <volume> <directory>
```

For LiveSkills, use direct volume mounts, not `afs ws mount`.

The desired production shape is:

```text
.agents/skills/<skill>     <- direct AFS volume mount
~/.codex/skills/<skill>    <- direct AFS volume mount
```

`afs.go` currently contains `LocalAFSAdapter`, a local development stand-in. It materializes checkpoint files and writes mount metadata, but the production integration should replace that boundary with real control-plane/AFS calls while preserving CLI behavior.

## Code Map

- `main.go`: process entrypoint.
- `cli.go`: command parsing, help text, command output.
- `app.go`: application behavior for add/list/update/remove/publish/show/download.
- `afs.go`: local AFS adapter boundary; replace here for real AFS.
- `source.go`: skill source discovery, Git clone support, `SKILL.md` metadata parsing, file hashing.
- `manifest.go`: agent install paths, global paths, project manifest helpers.
- `store.go`: local registry at `$LIVESKILLS_HOME/registry.json`.
- `types.go`: persisted JSON types and API payload structs.
- `liveskills_test.go`: CLI behavior tests.

## Development Commands

Run from repo root:

```bash
go test ./...
go run . --help
go run . add ./examples/react-best-practices
go run . list
```

Reinstall the local binary:

```bash
go build -o ~/.local/bin/liveskills .
```

Use `LIVESKILLS_HOME` to isolate local state during tests or manual smoke runs:

```bash
LIVESKILLS_HOME=/tmp/liveskills-demo liveskills list
```

## Testing Expectations

Before calling work done, run:

```bash
gofmt -w *.go
go test ./...
```

For CLI behavior changes, also do a manual smoke:

```bash
tmp=$(mktemp -d)
project="$tmp/project"
home="$tmp/home"
mkdir -p "$project" "$home"
cd "$project"
HOME="$home" LIVESKILLS_HOME="$tmp/store" liveskills list
HOME="$home" LIVESKILLS_HOME="$tmp/store" liveskills add /Users/rowantrollope/git/agent-filesystem/examples/liveskills/examples/react-best-practices
HOME="$home" LIVESKILLS_HOME="$tmp/store" liveskills list
HOME="$home" LIVESKILLS_HOME="$tmp/store" liveskills add -g /Users/rowantrollope/git/agent-filesystem/examples/liveskills/examples/react-best-practices
HOME="$home" LIVESKILLS_HOME="$tmp/store" liveskills list -g
```

## Design Gotchas

- Do not reintroduce a required two-step `publish` then `add` user flow.
- Do not mount or replace `.agents/skills` as a parent. Only install/mount the specific skill folder.
- Do not make `list` show all registry skills by default; it should show installed project skills.
- Do not break `-g` / `--global`; it should mirror the `skills` CLI scope model.
- Preserve deterministic hashes and IDs when touching source discovery or versioning.
- Reject symlinks in skill sources.
- Codex global inventory must scan durable packaged skill roots under
  `$CODEX_HOME/skills` too:
  `$CODEX_HOME/skills/.system`, `$CODEX_HOME/skills/.curated`,
  and `$CODEX_HOME/skills/.experimental`, not just direct
  `$CODEX_HOME/skills/<skill>` folders. Do not list volatile
  `$CODEX_HOME/plugins/cache/.../skills` entries as installed global skills.
- Keep JSON files indented with two spaces and a trailing newline.
- Keep the project dependency-light. This module currently uses only the Go standard library.
