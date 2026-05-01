# AFS CLI Command Reference

This reference covers the current `afs` command surface. It is written for
humans and agents who need exact command shapes, flags, and the right command
family for each task.

## Global Shape

```bash
afs [options] [command]
```

Global options:

| Option | Meaning |
| --- | --- |
| `--config <path>` | Override the `afs.config.json` path. |
| `-h`, `--help` | Display help for a command. |
| `-V`, `--version` | Print the CLI version. |

Primary commands:

| Command | Use it for |
| --- | --- |
| `afs auth` | Log in, log out, and inspect authentication. |
| `afs setup` | Configure basic connection defaults. |
| `afs status` | Show AFS status and attached workspaces. |
| `afs ws` | Create, list, attach, detach, fork, delete, or import workspaces. |
| `afs fs` | Read, search, and safely write workspace files. |
| `afs cp` | Create, list, and restore checkpoints. |
| `afs database` | Advanced control-plane database operations. |
| `afs log` | Read workspace file-change logs and summaries. |
| `afs config` | Read, persist, and reset local configuration. |
| `afs mcp` | Start the workspace-first MCP server over stdio. |

## Authentication

### `afs auth`

```bash
afs auth [command]
```

Subcommands:

| Command | Meaning |
| --- | --- |
| `afs auth login` | Connect this CLI to AFS Cloud or a control plane. |
| `afs auth logout` | Clear cached login and return to local-only mode. |
| `afs auth status` | Show authentication status. |

### `afs auth login`

```bash
afs auth login [--cloud] [--url <cloud-url>]
afs auth login --self-hosted [--url <url>]
afs auth login --control-plane-url <url> --token <token>
```

Flags:

| Flag | Meaning |
| --- | --- |
| `--cloud` | Force cloud mode with browser OAuth. |
| `--self-hosted` | Force Self-managed mode. |
| `--url`, `--control-plane-url <url>` | Override the control-plane URL. |
| `--token <token>` | Use a one-time onboarding token instead of browser auth. |
| `--workspace <name|id>` | Preferred workspace for cloud login. |

Examples:

```bash
afs auth login
afs auth login --self-hosted
afs auth login --self-hosted --url http://my-host:8091
afs auth login --cloud
```

### `afs auth logout`

```bash
afs auth logout
```

Clears any cached cloud login from this machine and switches back to
local-only mode. Safe to run when not signed in.

### `afs auth status`

```bash
afs auth status
```

Shows whether this machine is signed in, which control plane it targets, and
the selected cloud database when available.

## First Run And Lifecycle

### `afs setup`

```bash
afs setup
```

Guided configuration for connection basics. It does not select a persistent
"current" workspace; use `afs ws attach <workspace> <directory>` to attach a
workspace when you are ready to work.

### `afs ws attach`

```bash
afs ws attach [--dry-run] [--verbose] [<workspace> <directory>]
```

Attaches a durable workspace to a local directory using sync mode. The
directory is created if needed. AFS no longer saves "current workspace" or
"current local path" in `afs.config.json`; active attachments are runtime
state keyed by local directory.

Attach safety rules:

- Empty local directory + populated workspace: downloads workspace files.
- Populated local directory + empty workspace: uploads local files.
- Populated local directory + populated workspace with no prior sync baseline:
  attach is blocked so files are not overwritten silently.
- Existing sync baseline: AFS reconciles from that baseline.

Examples:

```bash
afs ws attach getting-started ~/getting-started
afs ws attach notes ~/work/notes
afs ws attach --dry-run notes ~/work/notes
```

### `afs ws detach`

```bash
afs ws detach [--delete] [<workspace|directory>]
```

Stops AFS from managing an attached workspace. The target can be a workspace
name, workspace ID, or attached local directory. With no target, AFS lists
attached workspaces and prompts for a numbered selection.
Local files are preserved by default. Use `--delete` only when you intentionally
want to remove the attached local directory after the daemon stops.

```bash
afs ws detach notes
afs ws detach ~/work/notes
afs ws detach --delete ~/scratch/throwaway
```

### `afs status`

```bash
afs status [--verbose]
```

Shows active attachments in aligned plain columns. Use `--verbose` to include
control-plane, database, session, attachment id, and process details.

## Configuration

### `afs config`

```bash
afs config <subcommand>
```

Use `afs config reset` to reset local config and runtime state while keeping
the CLI installed. If AFS is running, this command stops it first.

Subcommands:

| Command | Meaning |
| --- | --- |
| `afs config get <key> [--json]` | Read a config value. |
| `afs config show [--json]` | Show the full saved config. |
| `afs config set <key> <value>` | Persist a config value. |
| `afs config set [flags]` | Persist values through legacy flag shortcuts. |
| `afs config list [--json]` | List known config values. |
| `afs config unset <key>` | Reset a config value to default or empty state. |
| `afs config reset` | Reset local config and runtime state. |

Common keys:

| Key | Meaning |
| --- | --- |
| `config.source` | `cloud`, `self-managed`, or `local`. |
| `controlPlane.url` | Self-managed control plane URL. |
| `controlPlane.database` | Control-plane database override. |
| `mode` | Local runtime mode: `sync` or `mount`. |
| `redis.url` | Standalone Redis URL. |
| `agent.name` | Human-friendly agent name for attribution. |
| `sync.fileSizeCapMB` | Maximum file size synced by the attach daemon. |

Examples:

```bash
afs config get redis.url
afs config show --json
afs config set config.source self-managed
afs config set mode mount
afs config set controlPlane.url http://127.0.0.1:8091
afs config set agent.name "Claude Code"
afs config set sync.fileSizeCapMB 4096
afs config unset controlPlane.database
afs config reset
afs config list
```

Flag examples:

```bash
afs config set --redis-url rediss://user:pass@redis.example:6379/4
afs config set --config-source self-hosted --control-plane-url http://127.0.0.1:8091
```

Use `Self-managed` in user-facing copy. The older `self-hosted` flag value is
accepted for compatibility.

## Workspaces

### `afs ws`

```bash
afs ws <subcommand>
```

Subcommands:

| Command | Meaning |
| --- | --- |
| `afs ws create [--database <database>] <workspace>` | Create an empty workspace with an initial checkpoint named `initial`. |
| `afs ws list` | List workspaces. |
| `afs ws info <workspace>` | Show workspace metadata without attaching it locally. |
| `afs ws attach <workspace> [directory]` | Attach a workspace to a local directory. |
| `afs ws detach [--delete] [<workspace|directory>]` | Detach a workspace from AFS. |
| `afs ws fork [source-workspace] <new-workspace>` | Create a new workspace from the source workspace's current checkpoint. |
| `afs ws delete [--no-confirmation] <workspace>...` | Delete one or more workspaces and local materialized state. Prompts before deleting unless `--no-confirmation` is set. |
| `afs ws import [--force] [--attach-at-source] [--database <database>] <workspace> <directory>` | Import a local directory into a workspace. `--attach-at-source` attaches the source folder after import. |

Examples:

```bash
afs ws create demo
afs ws list
afs ws info demo
afs ws import --attach-at-source demo ~/src/demo
afs ws attach demo ~/src/demo
afs ws detach demo
afs ws fork demo demo-copy
afs ws delete --no-confirmation demo-copy
```

Import options:

| Option | Meaning |
| --- | --- |
| `--force` | Replace an existing workspace. |
| `--attach-at-source` | Attach the imported source directory immediately after import. |
| `--database <database-id|database-name>` | Override the control-plane database for the import. |

## Checkpoints

### `afs cp`

```bash
afs cp <subcommand>
```

Subcommands:

| Command | Meaning |
| --- | --- |
| `afs cp list [workspace]` | List checkpoints newest first. |
| `afs cp create [workspace] [checkpoint] [--description <text>]` | Save workspace state. |
| `afs cp show [workspace] <checkpoint> [--json]` | Show checkpoint metadata and parent-change summary. |
| `afs cp diff [workspace] <base> <target> [--json]` | Compare two checkpoints or workspace states. |
| `afs cp diff [workspace] <checkpoint> --active [--json]` | Compare a checkpoint to workspace state. |
| `afs cp restore [workspace] <checkpoint>` | Restore workspace state to a checkpoint. |

If `workspace` is omitted, AFS lists workspaces and prompts for a selection.
For `afs cp create <name>`, the single positional argument is treated as the
checkpoint name.

Examples:

```bash
afs cp list demo
afs cp create demo before-refactor --description "Before the agent rewrite"
afs cp show demo before-refactor
afs cp diff demo before-refactor --active
afs cp diff demo initial before-refactor --json
afs cp restore demo initial
```

Checkpoint rules:

- File edits change workspace state.
- Checkpoints are explicit.
- If a checkpoint name is omitted, AFS generates a timestamped name.
- Restoring a checkpoint overwrites workspace state. If the workspace
  has unsaved changes, AFS first creates a `safety` checkpoint and prints its
  ID in the restore output.
- In sync mode, restore replaces the local sync folder from the restored active
  state. AFS checks for open handles first and asks you to close them before it
  starts replacing files.

## Filesystem

`afs fs` inspects live workspace files without attaching the workspace to a
local directory. Pass the workspace before the subcommand:

```bash
afs fs -w <workspace> <subcommand>
```

Subcommands:

| Command | Meaning |
| --- | --- |
| `afs fs -w <workspace> ls [path]` | List files in a workspace directory. |
| `afs fs -w <workspace> cat <path>` | Print a workspace file. |
| `afs fs -w <workspace> find [path] [-name <pattern>] [-type f|d|l] [-print]` | Find workspace paths by basename pattern. |
| `afs fs -w <workspace> grep [flags] <pattern>` | Search workspace file contents. |
| `afs fs create-exclusive <path>` | Create a file through an attached sync workspace only if it does not exist. |

Examples:

```bash
afs fs -w repo ls
afs fs -w repo ls /src
afs fs -w repo cat README.md
afs fs -w repo find . -name '*.md' -print
afs fs -w repo find /src -type f -name '*.go'
afs fs -w repo grep "hello"
```

### `afs fs grep`

```bash
afs fs -w <workspace> grep [flags] <pattern>
afs fs grep [flags] <pattern>
afs fs grep [flags] -e <pattern>
```

Searches the live Redis-backed AFS namespace for a workspace. Literal
substring matching is the default. Use `-E` or `-G` for regex mode, `-F` for
fixed strings, or `--glob` for AFS glob matching semantics.

Flags:

| Flag | Meaning |
| --- | --- |
| `--workspace <name>` | Search a specific workspace. |
| `--path <path>` | Limit search to a file or directory. |
| `-i`, `--ignore-case` | Case-insensitive matching. |
| `-F` | Treat patterns as fixed strings. |
| `-E` | Use regex mode with RE2 syntax. |
| `-G` | Use regex mode with RE2 syntax; accepted for grep familiarity. |
| `-e <pattern>` | Add a pattern. Repeatable. |
| `-w` | Match whole words. |
| `-x` | Match whole lines. |
| `-v` | Invert the match. |
| `-l` | Print matching file paths only. |
| `-c` | Print per-file match counts. |
| `-m <num>` | Stop after `NUM` selected lines per file. |
| `-n` | Accepted for grep familiarity. Line numbers are shown by default. |
| `--glob` | Treat patterns as AFS globs instead of literals. |

Examples:

```bash
afs fs -w repo grep "hello"
afs fs -w repo grep -E "error|warning"
afs fs -w repo grep -w --path /logs token
afs fs grep -l -i --workspace repo "disk full"
afs fs -w repo grep --glob --path /src "*TODO*"
```

## Databases

Database commands are for Self-managed control-plane mode.

| Command | Meaning |
| --- | --- |
| `afs database list` | List databases configured in the control plane. |
| `afs database use <database-id|database-name|auto>` | Choose which control-plane database new workspaces and imports use. |

Use `auto` to clear the local database override and fall back to the
control-plane default.

## Logs

Log commands inspect file-change history for a running or recent local
attachment.

| Command | Meaning |
| --- | --- |
| `afs log [session-id] [flags]` | Show file-change history. |
| `afs log summary [session-id] [flags]` | Show per-session totals. |

`log` flags:

| Flag | Meaning |
| --- | --- |
| `--workspace`, `-w <name>` | Read log entries for a specific workspace. |
| `--limit <n>` | Number of recent entries to show. Default `50`. |
| `--follow`, `-f` | Stream new entries every two seconds. |
| `--all` | Include entries from other sessions. |

Examples:

```bash
afs log
afs log --follow
afs log <session-id>
afs log summary <session-id>
```

## File Operations

### `afs fs create-exclusive`

```bash
afs fs create-exclusive [--content <text> | --content-file <path>] [--timeout <duration>] <path>
```

Creates `<path>` only if it does not already exist in the workspace. The create
is atomic across connected AFS clients. Requires AFS to be running in sync mode
on this machine. The path must be absolute inside the workspace.

Examples:

```bash
afs fs create-exclusive /tasks/001.claim
afs fs create-exclusive --content "agent-a\n" /tasks/001.claim
```

## MCP Server

```bash
afs mcp [--workspace <name>] [--profile <profile>]
```

Starts the Agent Filesystem MCP server over stdio. This command is meant to be
launched by an MCP client.

Profiles:

| Profile | Scope |
| --- | --- |
| `workspace-ro` | Workspace-bound read-only file tools. |
| `workspace-rw` | Workspace-bound read/write file tools. Default. |
| `workspace-rw-checkpoint` | Workspace-bound file tools plus checkpoint operations. |
| `admin-ro` | Broad read-only MCP surface. |
| `admin-rw` | Broad read/write MCP surface. |

Example MCP config:

```json
{
  "mcpServers": {
    "afs": {
      "command": "/absolute/path/to/afs",
      "args": ["mcp", "--workspace", "my-workspace", "--profile", "workspace-rw"]
    }
  }
}
```
