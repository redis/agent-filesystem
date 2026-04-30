# Agent Filesystem Guide for Agents

This document is for AI coding agents using Agent Filesystem (AFS). Read it
before you create workspaces, edit files, configure MCP, or run the AFS CLI.

## Core Model

AFS is workspace-first. A workspace is a complete file tree for source code,
prompts, notes, generated files, logs, and agent scratch state.

Redis is the canonical store for workspace metadata, manifests, blobs,
checkpoints, live roots, and activity. Local folders, mounts, the web UI, CLI,
SDKs, and MCP tools are all surfaces over that same workspace model.

Related command references:

- [CLI command reference](cli-reference.md)
- [TypeScript command reference](typescript-reference.md)
- [Python command reference](python-reference.md)
- [MCP tool reference](mcp-reference.md)

Remember these rules:

- File edits change the live workspace state.
- File edits do not automatically create checkpoints.
- Checkpoints are explicit restore points.
- Forks create a second workspace from another line of work.
- The canonical starter workspace name is `getting-started`.
- Use `Self-managed` in user-facing copy for the control-plane-backed mode.

## Pick The Right Access Path

Use MCP when your client has AFS MCP tools. This is the most direct agent path.

Use the CLI when you are operating from a shell, setting up a local runtime, or
debugging configuration.

Use sync mode when humans, editors, language servers, tests, or shell tools need
a normal directory on disk.

Use live mount mode when you specifically need a live filesystem view. On macOS
AFS uses NFS; on Linux it uses FUSE. Sync mode is usually the friendlier
default.

## Agent Operating Loop

1. Identify the workspace.
2. Inspect before editing.
3. Create a checkpoint before broad or risky changes.
4. Make focused edits.
5. Validate with the relevant build, test, or CLI command.
6. Create a checkpoint after a useful result if the state should be preserved.
7. Report the workspace, changed paths, validation, and checkpoint name.

Do not assume the active workspace. If there is no selected workspace, choose or
create one before running workspace-scoped commands.

## MCP Setup

For local stdio MCP, configure your client with an absolute path to the `afs`
binary:

```json
{
  "mcpServers": {
    "agent-filesystem": {
      "command": "/absolute/path/to/afs",
      "args": ["mcp", "--workspace", "getting-started", "--profile", "workspace-rw-checkpoint"]
    }
  }
}
```

Profiles:

| Profile | Scope |
| --- | --- |
| `workspace-ro` | Workspace-bound read-only file tools. |
| `workspace-rw` | Workspace-bound read/write file tools. This is the default. |
| `workspace-rw-checkpoint` | Read/write file tools plus checkpoint operations. |
| `admin-ro` | Broad read-only workspace administration. |
| `admin-rw` | Broad read/write workspace administration. |

Common workspace-scoped MCP tools:

| Tool | Use it for |
| --- | --- |
| `workspace_current` | Confirm which workspace the MCP server is bound to. |
| `file_list` | List directory contents. |
| `file_glob` | Find files or directories by basename glob. Use this for filename discovery. |
| `file_grep` | Search file contents. Use this for content search, not directory discovery. |
| `file_read` | Read a whole file. |
| `file_lines` | Read a specific line range. |
| `file_write` | Create or fully overwrite a file. Use carefully. |
| `file_create_exclusive` | Create a file only if it does not already exist. |
| `file_replace` | Replace exact text after inspecting the file. |
| `file_insert` | Insert content at a known line. |
| `file_delete_lines` | Delete a known line range. |
| `file_patch` | Apply structured patches. Prefer this for precise edits. |
| `checkpoint_list` | List saved restore points. |
| `checkpoint_create` | Save the current live state. |
| `checkpoint_restore` | Restore a saved checkpoint. This overwrites live state. |

## CLI Quick Start

Authenticate and create a workspace:

```bash
afs login
afs workspace create getting-started
afs workspace use getting-started
afs up
```

Import an existing directory:

```bash
afs workspace import my-project ~/src/my-project
afs workspace use my-project
afs up my-project ~/src/my-project
```

Create checkpoints around important changes:

```bash
afs checkpoint create my-project before-agent
# make edits
afs checkpoint create my-project after-agent
afs checkpoint list my-project
```

Fork for parallel work:

```bash
afs workspace fork my-project my-project-experiment
afs workspace use my-project-experiment
```

Run the MCP server:

```bash
afs mcp --workspace my-project --profile workspace-rw-checkpoint
```

## CLI Command Reference

| Command | Use it for |
| --- | --- |
| `afs login` | Authenticate the CLI to AFS Cloud or a control plane. |
| `afs setup` | Configure first-run settings. |
| `afs status` | Check login, selected workspace, local path, and runtime state. |
| `afs up [workspace] [path]` | Start sync or mount for a workspace. |
| `afs down` | Stop the local runtime. |
| `afs workspace list` | List workspaces. |
| `afs workspace current` | Print the active workspace. |
| `afs workspace create <name>` | Create an empty workspace. |
| `afs workspace import <name> <dir>` | Import a local directory into AFS. |
| `afs workspace use <name>` | Select the default workspace. |
| `afs workspace fork <source> <target>` | Create a second line of work. |
| `afs workspace clone <name> <dir>` | Export a workspace to a normal local directory. |
| `afs checkpoint create [workspace] [name]` | Save live state as a restore point. |
| `afs checkpoint list [workspace]` | List checkpoints. |
| `afs checkpoint restore [workspace] <name>` | Restore a checkpoint. This overwrites live state. |
| `afs grep <pattern>` | Search workspace files. |
| `afs mcp` | Start the workspace-first MCP server over stdio. |

## Config Commands

Use the key-based config commands:

```bash
afs config get redis.url
afs config set config.source self-managed
afs config set controlPlane.url http://127.0.0.1:8091
afs config set mount.path ~/afs/my-project
afs config list
afs config unset mount.path
```

Useful keys:

| Key | Meaning |
| --- | --- |
| `config.source` | `cloud`, `self-managed`, or `local`. |
| `controlPlane.url` | Self-managed control plane URL. |
| `controlPlane.databaseID` | Database choice when the control plane has more than one database. |
| `redis.url` | Standalone Redis connection URL. |
| `mount.path` | Saved local sync or mount path. |
| `mount.backend` | Mount backend such as `nfs` on macOS. |
| `agent.name` | Human-friendly agent name used in attribution. |

## Editing Rules For Agents

- Read the file before changing it.
- Prefer precise patches over full rewrites.
- Use `file_glob` for filename discovery and `file_grep` for content search.
- Preserve user changes you did not make.
- Create a checkpoint before destructive or broad edits.
- Do not restore a checkpoint unless the user asked or you are certain the live
  state can be overwritten.
- Keep generated artifacts, dependencies, logs, and machine-local files out of
  imported workspaces with `.afsignore`.

Example `.afsignore`:

```gitignore
node_modules/
.venv/
dist/
build/
*.log
.DS_Store
```

## Search Guidance

Use `afs grep` or `file_grep` for workspace content search. Literal searches use
the Redis Search indexed path when it is available, then verify candidate file
contents through AFS.

Regex and advanced matching can fall back to traversal. For regex-heavy scans on
a synced or mounted workspace, local `rg` is often the right tool.

Examples:

```bash
afs grep "TODO" --workspace my-project
afs grep -l -i --workspace my-project "disk full"
afs grep -E "error|warning" --workspace my-project
```

## Local Runtime Notes

Sync mode:

```bash
afs workspace use my-project
afs up --mode sync
cd ~/afs/my-project
```

Mount mode:

```bash
afs config set mount.backend nfs
afs up my-project ~/afs/my-project --mode mount
afs down
```

If you run `afs up` without a selected workspace, select one first with
`afs workspace use <name>` or pass the workspace explicitly.

## Deployment Modes

| Mode | Use it when |
| --- | --- |
| Cloud-hosted | You want browser auth, hosted UI, and managed workspace access. |
| Self-managed | You run your own control plane and UI with your own Redis database. |
| Standalone | You want the CLI to talk directly to Redis without the hosted UI. |

Local Self-managed development:

```bash
make web-dev
# control plane: http://127.0.0.1:8091
# Vite UI: printed by the dev server
```

Point the CLI at a Self-managed control plane:

```bash
afs config set config.source self-managed
afs config set controlPlane.url http://127.0.0.1:8091
afs up --control-plane-url http://127.0.0.1:8091 getting-started
```

## Handoff Template

When finishing an AFS task, report:

- Workspace name.
- Files changed.
- Commands or MCP tools used.
- Validation run and result.
- Checkpoint created, if any.
- Any restore, fork, or destructive action performed.

Example:

```text
Workspace: my-project
Changed: /src/app.ts, /README.md
Validation: npm test passed
Checkpoint: after-agent-readme-update
Notes: no checkpoint restore performed
```
