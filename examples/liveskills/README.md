# LiveSkills

LiveSkills is a Go CLI example for installing agent skills into Codex, Claude
Code, and other local agent skill folders through an AFS-backed registry model.

The normal workflow is intentionally small:

```bash
liveskills add <source-or-ref>
liveskills list
liveskills add -g <source-or-ref>
liveskills list -g
```

`add` is the happy path. It can take a local skill source and register it before
installing, or it can install an existing registry reference such as
`local/react-best-practices`. `publish`, `show`, and `download` remain available
for explicit registry, debug, and export workflows.

## Quick Start

Run these commands from this directory:

```bash
make
LIVESKILLS_AFS_MODE=local ./bin/liveskills help
LIVESKILLS_AFS_MODE=local ./bin/liveskills add ./examples/react-best-practices --yes
LIVESKILLS_AFS_MODE=local ./bin/liveskills list
make test
make surface-test
```

Use `LIVESKILLS_AFS_MODE=local` for isolated development. Without it,
LiveSkills uses the `afs` CLI when it is available on `PATH`; if `afs` is not
available, it falls back to the local adapter.

Registry data and local AFS staging data live under `~/.liveskills` by default.
Set `LIVESKILLS_HOME` to isolate a demo or test run:

```bash
LIVESKILLS_HOME=/tmp/liveskills-demo LIVESKILLS_AFS_MODE=local ./bin/liveskills list
```

## Build And Install

Build the local binary:

```bash
make
```

Install it onto your command line:

```bash
make install
```

By default this installs `liveskills` to `~/.local/bin/liveskills`. Override the
target with `BINDIR=/path/to/bin` or `PREFIX=/usr/local`:

```bash
make install PREFIX=/usr/local
```

Other useful targets:

```bash
make test          # go test ./...
make surface-test  # isolated end-to-end CLI harness
make fmt           # gofmt -w *.go
make clean         # remove bin/
```

## Current Install Model

Project installs are the default. Global installs use `-g` / `--global`.

For each scope, LiveSkills keeps a canonical skills workspace and installs one
skill folder at a time from that canonical copy. Agent-facing skill folders are
relative symlinks by default; `--copy` writes a standalone copy instead.

```text
<project>/.liveskills/mount/skills/<skill>  # project canonical copy
<project>/.agents/skills/<skill>            # Codex/project universal target
<project>/.claude/skills/<skill>            # Claude Code project target

~/.liveskills/mount/skills/<skill>          # global canonical copy
~/.codex/skills/<skill>                     # Codex global target
~/.claude/skills/<skill>                    # Claude Code global target
```

LiveSkills owns only the specific installed skill folder. Neighboring manual
skills under `.agents/skills`, `.claude/skills`, `~/.codex/skills`, or other
agent folders must remain untouched.

## AFS Boundary

`afs.go` and `workspace_mount.go` contain the AFS adapter boundary. In local
mode, the adapter materializes checkpoint files and writes mount metadata under
`$LIVESKILLS_HOME`. In CLI mode, it calls `afs` to create/import checkpoints and
mount skill volumes into the canonical skills workspace.

The current shape is:

- one skills workspace per scope
- registered skill content under `skills/<skill-slug>` in that workspace
- direct per-skill volume attachment at the canonical skill path
- symlinked agent folders by default
- copy fallback with `--copy`

## Commands

| Command | Description |
| --- | --- |
| `liveskills help` | Show the compact command screen. |
| `liveskills add <source-or-ref>` | Register/version a local source if needed, then install it into the current project. |
| `liveskills add -g <source-or-ref>` | Install into global agent folders. |
| `liveskills add <source-or-ref> --agent <agent>` | Install for one or more selected agents. Repeat `--agent` for multiple targets. |
| `liveskills add <source> --skill <skill>` | Select one or more skills from a multi-skill source. Repeat `--skill` for multiple selections. |
| `liveskills add <source> --all` | Install every skill found in a multi-skill source. |
| `liveskills add <source> --list` | List skills available in a source without installing. |
| `liveskills add <source-or-ref> --copy` | Copy instead of symlinking from the LiveSkills canonical workspace. |
| `liveskills add <source-or-ref> --yes` | Skip the interactive install confirmation. |
| `liveskills list` / `liveskills ls` | Show current-project installed skills. |
| `liveskills list -g` | Show global installed skills, plus current-project LiveSkills mounts. |
| `liveskills find [query]` | Open the interactive skill finder in a terminal; with a query or non-TTY output, print copyable install refs. |
| `liveskills find --interactive [query]` | Force the terminal skill finder, optionally seeded with a query. |
| `liveskills publish <source> [--skill <name>]` | Advanced: register a source without installing it. |
| `liveskills show <owner>/<skill>` | Show registry details and versions for a skill. |
| `liveskills download <owner>/<skill> --output <dir>` | Export a registry snapshot to a local directory. |
| `liveskills update <owner>/<skill>` | Move an existing install to the selected/latest version. |
| `liveskills remove <owner>/<skill>` / `liveskills rm <owner>/<skill>` | Remove a managed install. |
| `liveskills scan [-g|-p] [--agent <agent>]` | Scan local skill folders without using the registry. |
| `liveskills auth login` | Store registry auth settings in the local registry config. |

Most commands support `--json` for machine-readable output.

## Add Behavior

`<source-or-ref>` may be:

- a local skill folder containing `SKILL.md`
- a local folder containing multiple skill folders
- a remote Git/GitHub source
- a registry reference in `<owner>/<skill>` form
- a source with an inline `@skill` selector

When adding a local source, LiveSkills publishes it with owner `local` by
default, then installs the selected version. If a source contains multiple
skills, pass repeated `--skill <name>` values or `--all`; otherwise the command
fails rather than guessing.

The install output reports the selected skill, version, scope, workspace,
canonical path, agent targets, and the matching `list` command. Non-JSON `add`
also prints a local security assessment and a final reminder that installed
skills run with full agent permissions.

## Lists And Scans

`liveskills list` is an installed inventory, not a registry browser. It shows
LiveSkills-managed rows separately from local unmanaged skill folders discovered
on disk. If no project skills are found, it hints to try `liveskills list -g`.

Use `liveskills find` for the interactive registry picker, or
`liveskills find [query]` for copyable search output. Use `liveskills scan` for
a read-only scan of project and global skill folders across supported agents.
When the picker selection is confirmed with Enter, LiveSkills shows the skill
summary, asks which agents to install to, asks for project or global scope,
asks for symlink or copy when multiple agent folders are targeted, shows an
installation summary and local security assessment, and asks before installing.

## Full Surface Test

Run the isolated CLI harness when changing command behavior:

```bash
make surface-test
```

The harness builds a temporary `liveskills` binary, runs `go test ./...`, then
uses isolated `HOME`, `LIVESKILLS_HOME`, and `LIVESKILLS_AFS_MODE=local` values
to exercise auth, publish, find, show, download, add, list, update, remove,
scan, project/global installs, agent-specific paths, validation errors, and
deterministic stress coverage.

Use `--keep-temp` to inspect the generated project, home, store, and source
fixtures after a run:

```bash
python3 scripts/liveskills_surface_test.py --keep-temp --verbose
```
