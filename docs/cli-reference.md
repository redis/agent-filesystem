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
| `afs login` | Connect this CLI to AFS Cloud or a control plane. |
| `afs logout` | Clear cached login and return to local-only mode. |
| `afs setup` | Interactive workspace, local path, and connection setup. |
| `afs up` | Start sync or mount for the selected workspace. |
| `afs down` | Stop the local runtime and unmount. |
| `afs status` | Show connection, workspace, and sync or mount status. |
| `afs file` | Workspace file operations. |
| `afs workspace` | Create, list, use, clone, fork, delete, or import workspaces. |
| `afs database` | List or select control-plane databases. |
| `afs checkpoint` | Create, list, and restore checkpoints. |
| `afs session` | Read file-change session logs and summaries. |
| `afs grep` | Search workspace contents. |
| `afs config` | Read and persist local configuration. |
| `afs reset` | Reset local config and runtime state while keeping the CLI installed. |
| `afs mcp` | Start the workspace-first MCP server over stdio. |

## Authentication

### `afs login`

```bash
afs login [--cloud] [--url <cloud-url>]
afs login --self-hosted [--url <url>]
afs login --control-plane-url <url> --token <token>
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
afs login
afs login --self-hosted
afs login --self-hosted --url http://my-host:8091
afs login --cloud
```

### `afs logout`

```bash
afs logout
```

Clears any cached cloud login from this machine and switches back to
local-only mode. Safe to run when not signed in.

## First Run And Lifecycle

### `afs setup`

```bash
afs setup
```

Opens the interactive setup flow for workspace, local path, and connection
settings.

### `afs up`

```bash
afs up [flags]
afs up <workspace> [<mountpoint>]
```

Starts AFS using saved config. If `<workspace>` is provided, AFS saves it and
starts. The mountpoint defaults to `~/afs/<workspace>` when omitted. Both are
persisted for future `afs up` runs.

Flags:

| Flag | Meaning |
| --- | --- |
| `--mode <sync|mount>` | Persist a mode override before starting. |
| `--control-plane-url <url>` | One-shot Self-managed control plane URL override. |
| `--control-plane-database <database-id>` | One-shot database override for Self-managed mode. |
| `--redis-url <redis://...|rediss://...>` | One-shot Redis override for local mode. |
| `--mount-backend <auto|none|fuse|nfs>` | One-shot mount backend override. |
| `--mountpoint <path>` | One-shot local surface path override. |
| `--readonly[=true|false]` | One-shot readonly override. |
| `--interactive`, `-i` | Run the sync daemon in the foreground with live logs. |

Examples:

```bash
afs up
afs up --mode sync
afs up --control-plane-url http://127.0.0.1:8091 getting-started
afs up --interactive
afs up claude-code ~/.claude
```

Notes:

- Select a current workspace with `afs workspace use <workspace>` or pass a
  workspace positionally.
- Use `afs config set ...` for persistent Redis, control-plane, and mount
  settings.

### `afs down`

```bash
afs down
```

Stops AFS, unmounts the local surface, and clears active runtime state.

### `afs status`

```bash
afs status
```

Shows connection, selected workspace, local path, sync state, and mount state.

### `afs reset`

```bash
afs reset
```

Resets local config and runtime state while keeping the CLI installed. If AFS
is running, this command stops it first.

## Configuration

### `afs config`

```bash
afs config <subcommand>
```

Subcommands:

| Command | Meaning |
| --- | --- |
| `afs config get <key> [--json]` | Read a config value. |
| `afs config show [--json]` | Show the full saved config. |
| `afs config set <key> <value>` | Persist a config value. |
| `afs config set [flags]` | Persist values through legacy flag shortcuts. |
| `afs config list [--json]` | List known config values. |
| `afs config unset <key>` | Reset a config value to default or empty state. |

Common keys:

| Key | Meaning |
| --- | --- |
| `config.source` | `cloud`, `self-managed`, or `local`. |
| `controlPlane.url` | Self-managed control plane URL. |
| `controlPlane.database` | Control-plane database override. |
| `redis.url` | Standalone Redis URL. |
| `mount.backend` | `auto`, `none`, `fuse`, or `nfs`. |
| `mount.path` | Local sync or mount path. |
| `mount.readonly` | Readonly mode. |
| `workspace.current` | Current workspace value. Prefer `afs workspace use`. |
| `agent.name` | Human-friendly agent name for attribution. |

Examples:

```bash
afs config get redis.url
afs config show --json
afs config set config.source self-managed
afs config set controlPlane.url http://127.0.0.1:8091
afs config set mount.path ~/afs/demo
afs config set agent.name "Claude Code"
afs config unset controlPlane.database
afs config list
```

Legacy flag shortcuts:

```bash
afs config set --redis-url rediss://user:pass@redis.example:6379/4
afs config set --config-source self-hosted --control-plane-url http://127.0.0.1:8091
afs config set --mount-backend nfs --mountpoint ~/afs/demo --readonly=true
```

Use `Self-managed` in user-facing copy. The older `self-hosted` flag value is
accepted for compatibility.

## Workspaces

### `afs workspace`

```bash
afs workspace <subcommand>
```

Subcommands:

| Command | Meaning |
| --- | --- |
| `afs workspace create [--database <database>] <workspace>` | Create an empty workspace with an initial checkpoint named `initial`. |
| `afs workspace list` | List workspaces stored in Redis. |
| `afs workspace current` | Show the workspace used when a workspace argument is omitted. |
| `afs workspace use <workspace-name-or-id>` | Set the current workspace. |
| `afs workspace clone [workspace] <directory>` | Export a workspace to a local directory. Destination must be empty. |
| `afs workspace fork [source-workspace] <new-workspace>` | Create a new workspace from the source workspace's current checkpoint. |
| `afs workspace delete <workspace>...` | Delete one or more workspaces and local materialized state. |
| `afs workspace import [--force] [--database <database>] [--mount-at-source] <workspace> <directory>` | Import a local directory into a workspace. |

Examples:

```bash
afs workspace create demo
afs workspace use demo
afs workspace current
afs workspace list
afs workspace import demo ~/src/demo
afs workspace clone demo ~/exports/demo
afs workspace fork demo demo-copy
afs workspace delete demo-copy
```

Import options:

| Option | Meaning |
| --- | --- |
| `--force` | Replace an existing workspace. |
| `--database <database-id|database-name>` | Override the control-plane database for the import. |
| `--mount-at-source` | Archive the source directory to `<directory>.pre-afs` and mount the imported workspace at the original path. |

## Checkpoints

### `afs checkpoint`

```bash
afs checkpoint <subcommand>
```

Subcommands:

| Command | Meaning |
| --- | --- |
| `afs checkpoint list [workspace]` | List checkpoints newest first. |
| `afs checkpoint create [workspace] [checkpoint] [--description <text>]` | Save the active workspace state. |
| `afs checkpoint show [workspace] <checkpoint> [--json]` | Show checkpoint metadata and parent-change summary. |
| `afs checkpoint diff [workspace] <base> <target> [--json]` | Compare two checkpoints or workspace states. |
| `afs checkpoint diff [workspace] <checkpoint> --active [--json]` | Compare a checkpoint to the active workspace. |
| `afs checkpoint restore [workspace] <checkpoint>` | Restore active state to a checkpoint. |

Examples:

```bash
afs checkpoint list demo
afs checkpoint create demo before-refactor --description "Before the agent rewrite"
afs checkpoint show demo before-refactor
afs checkpoint diff demo before-refactor --active
afs checkpoint diff demo initial before-refactor --json
afs checkpoint restore demo initial
```

Checkpoint rules:

- File edits change active state.
- Checkpoints are explicit.
- If a checkpoint name is omitted, AFS generates a timestamped name.
- Restoring a checkpoint overwrites active workspace state. If the active workspace
  has unsaved changes, AFS first creates a `safety` checkpoint and prints its
  ID in the restore output.
- In sync mode, restore replaces the local sync folder from the restored active
  state. AFS checks for open handles first and asks you to close them before it
  starts replacing files.

## Search

### `afs grep`

```bash
afs grep [flags] <pattern>
afs grep [flags] -e <pattern>
```

Searches the live Redis-backed AFS namespace for a workspace. Literal
substring matching is the default. Use `-E` or `-G` for regex mode, `-F` for
fixed strings, or `--glob` for AFS glob matching semantics.

Flags:

| Flag | Meaning |
| --- | --- |
| `--workspace <name>` | Search a specific workspace. Defaults to current workspace. |
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
afs grep "hello"
afs grep -E "error|warning"
afs grep -w --path /logs token
afs grep -l -i --workspace repo "disk full"
afs grep --glob --path /src "*TODO*"
```

## Databases

Database commands are for Self-managed control-plane mode.

| Command | Meaning |
| --- | --- |
| `afs database list` | List databases configured in the control plane. |
| `afs database use <database-id|database-name|auto>` | Choose which control-plane database new workspaces and imports use. |

Use `auto` to clear the local database override and fall back to the
control-plane default.

## Sessions

Session commands inspect file-change history for a running or recent local
runtime.

| Command | Meaning |
| --- | --- |
| `afs session log [session-id] [flags]` | Show file-change history. |
| `afs session summary [session-id] [flags]` | Show per-session totals. |

`session log` flags:

| Flag | Meaning |
| --- | --- |
| `--workspace`, `-w <name>` | Override the current workspace. |
| `--limit <n>` | Number of recent entries to show. Default `50`. |
| `--follow`, `-f` | Stream new entries every two seconds. |
| `--all` | Include entries from other sessions. |

Examples:

```bash
afs session log
afs session log --follow
afs session log <session-id>
afs session summary <session-id>
```

## File Operations

### `afs file create-exclusive`

```bash
afs file create-exclusive [--content <text> | --content-file <path>] [--timeout <duration>] <path>
```

Creates `<path>` only if it does not already exist in the workspace. The create
is atomic across connected AFS clients. Requires AFS to be running in sync mode
on this machine. The path must be absolute inside the workspace.

Examples:

```bash
afs file create-exclusive /tasks/001.claim
afs file create-exclusive --content "agent-a\n" /tasks/001.claim
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
