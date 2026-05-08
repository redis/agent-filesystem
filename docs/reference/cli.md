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
| `afs setup` | Configure the default local mode. |
| `afs status` | Show AFS status and mounted workspaces. |
| `afs ws` | Create, list, mount, unmount, fork, delete, or import workspaces. |
| `afs fs` | Read, search, and safely write workspace files. |
| `afs cp` | Create, list, and restore checkpoints. |
| `afs database` | Advanced control-plane database operations. |
| `afs log` | Read workspace file-change logs and summaries. |
| `afs config` | Read, persist, and reset local configuration. |
| `afs tokens` | Create scoped CLI access tokens. |
| `afs mcp` | Start the workspace-first MCP server over stdio. |
| `afs skill` | Show or install the packaged AFS skill. |

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
afs auth login --control-plane-url <url> --access-token <token>
```

Flags:

| Flag | Meaning |
| --- | --- |
| `--cloud` | Force cloud mode with browser OAuth. |
| `--self-hosted` | Force Self-managed mode. |
| `--url`, `--control-plane-url <url>` | Override the control-plane URL. |
| `--token <token>` | Use a one-time onboarding token instead of browser auth. |
| `--access-token <token>` | Save a durable CLI access token directly. |
| `--workspace <name|id>` | Preferred workspace for cloud login. |

Plain browser login and one-time onboarding token exchange save an
account-scoped CLI access token by default. `--access-token` is for an already
minted CLI token, including a single-workspace mount token created with
`afs tokens create`.

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

## CLI Access Tokens

### `afs tokens`

```bash
afs tokens [command]
```

Subcommands:

| Command | Meaning |
| --- | --- |
| `afs tokens create` | Create a workspace-scoped CLI access token. |

### `afs tokens create`

```bash
afs tokens create --workspace <workspace> [--permission ro|rw] [--expires 30d]
afs tokens create <workspace> [--permission ro|rw]
```

Creates a CLI token scoped to one workspace for mount use. The default
permission is read-write. `--permission ro` creates a read-only mount token;
when that token is used with `afs auth login --access-token`, the mount session
is forced read-only.

Flags:

| Flag | Meaning |
| --- | --- |
| `--workspace <name|id>` | Workspace the token may mount. |
| `--use mount` | Token use. `mount` is currently the only supported value. |
| `--permission ro\|rw` | Mount permission. Defaults to `rw`. |
| `--expires <duration>` | Expiry such as `12h`, `30d`, `4w`, RFC3339, or `never`. |
| `--name <label>` | Optional token label. |

Examples:

```bash
afs tokens create --workspace repo --permission rw
afs tokens create repo --permission ro --expires 7d --name "ci read-only mount"
afs auth login --url https://afs.example.com --access-token afs_cli_...
afs ws mount repo ~/repo
```

Workspace-scoped mount tokens can list only the workspace they are scoped to
and can open client mount sessions for that workspace. Use a normal
account-scoped login token for account, workspace-management, checkpoint, or
MCP-token administration.

## First Run And Lifecycle

### `afs setup`

```bash
afs setup
```

Guided configuration for the default local mode. Use `afs auth login` for
Cloud or Self-managed connection setup, then use
`afs ws mount <workspace> <directory>` to mount a workspace when you are ready
to work, or `afs ws set-default <workspace>` to save a default for commands
where the workspace argument is optional.

### `afs ws mount`

```bash
afs ws mount [--dry-run] [--yes] [--verbose] [--readonly] [--session <name>] [<workspace> <directory>]
```

Mounts a durable workspace to a local directory using sync mode. The
directory is created if needed. AFS no longer saves a "current local path" in
`afs.config.json`; active mounts are runtime state keyed by local directory.
Commands that omit a workspace can use the saved default, the workspace
mounted at the current directory, or the only mounted workspace when that is
unambiguous.

Mount safety rules:

- Empty local directory + populated workspace: downloads workspace files.
- Populated local directory + empty workspace: shows a safe import plan and,
  after confirmation or `--yes`, uploads local files.
- Populated local directory + populated workspace with no prior sync baseline:
  disjoint local-only and remote-only files are shown as a reconciliation plan
  that you can confirm; overlapping paths with mismatched content are treated
  as conflicts and block the mount.
- Existing sync baseline: AFS reconciles from that baseline.

Examples:

```bash
afs ws mount getting-started ~/getting-started
afs ws mount notes ~/work/notes
afs ws mount --readonly notes ~/work/notes
afs ws mount notes --session "auth refactor"
afs ws mount --dry-run notes ~/work/notes
```

`--readonly` makes this mount refuse local writes. `agent.name` identifies the
agent/machine. `--session` names this specific mount session, so the UI can
show both the stable agent and the current task.

### `afs ws unmount`

```bash
afs ws unmount [--delete] [<workspace|directory>]
```

Stops AFS from managing a mounted workspace. The target can be a workspace
name, workspace ID, or mounted local directory. With no target, AFS lists
mounted workspaces and prompts for a numbered selection.
Local files are preserved by default. Use `--delete` only when you intentionally
want to remove the mounted local directory after the daemon stops.

```bash
afs ws unmount notes
afs ws unmount ~/work/notes
afs ws unmount --delete ~/scratch/throwaway
```

### `afs status`

```bash
afs status [--verbose]
```

Shows active mounts in aligned plain columns. Use `--verbose` to include
control-plane, database, session, mount id, and process details.

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
| `sync.fileSizeCapMB` | Maximum file size synced by the mount daemon. |

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
| `afs ws list [--json]` | List workspaces. |
| `afs ws default` | Show the saved default and the effective workspace AFS will use for omitted workspace arguments. |
| `afs ws set-default <workspace>` | Save a default workspace for commands that allow the workspace argument to be omitted. |
| `afs ws unset-default` | Clear the saved default workspace. |
| `afs ws info [workspace]` | Show workspace metadata without mounting it locally. |
| `afs ws mount [--readonly] <workspace> [directory]` | Mount a workspace at a local directory. |
| `afs ws unmount [--delete] [<workspace|directory>]` | Unmount a workspace from AFS. |
| `afs ws fork [source-workspace] <new-workspace>` | Create a new workspace from the source workspace's current checkpoint. |
| `afs ws delete [--no-confirmation] [workspace]...` | Delete one or more workspaces and local materialized state. If omitted, prompts for a workspace. Prompts before deleting unless `--no-confirmation` is set. |
| `afs ws import [--force] [--mount-at-source] [--database <database>] <workspace> <directory>` | Import a local directory into a workspace. `--mount-at-source` mounts the source folder after import. |

Examples:

```bash
afs ws create demo
afs ws list
afs ws set-default demo
afs ws default
afs ws info demo
afs ws import --mount-at-source demo ~/src/demo
afs ws mount demo ~/src/demo
afs ws unmount demo
afs ws fork demo demo-copy
afs ws delete --no-confirmation demo-copy
```

Import options:

| Option | Meaning |
| --- | --- |
| `--force` | Replace an existing workspace. |
| `--mount-at-source` | Mount the imported source directory immediately after import. |
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

If `workspace` is omitted, AFS uses the default, the CWD (if a workspace is
mounted there), the only mounted workspace, or prompts for one.
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

`afs fs` inspects live workspace files without mounting the workspace to a
local directory. Pass the workspace before the subcommand:

```bash
afs fs [workspace] <subcommand>
```

Subcommands:

| Command | Meaning |
| --- | --- |
| `afs fs [workspace] ls [path]` | List files in a workspace directory. |
| `afs fs [workspace] cat <path>` | Print a workspace file. |
| `afs fs [workspace] find [path] [-name <pattern>] [-type f|d|l] [-print]` | Find workspace paths by basename pattern. |
| `afs fs [workspace] grep [flags] <pattern>` | Search workspace file contents. |
| `afs fs [workspace] query [flags] <query>` | Rank workspace files for a concept or question. |
| `afs fs create-exclusive <path>` | Create a file through a mounted sync workspace only if it does not exist. |

Examples:

```bash
afs fs repo ls
afs fs repo ls /src
afs fs repo cat README.md
afs fs repo find . -name '*.md' -print
afs fs repo find /src -type f -name '*.go'
afs fs repo grep "hello"
afs fs repo query "how do checkpoints work?"
```

### `afs fs grep`

```bash
afs fs [workspace] grep [flags] <pattern>
afs fs [workspace] grep [flags] -e <pattern>
```

Searches the live Redis-backed AFS namespace for a workspace. Literal
substring matching is the default. Use `-E` or `-G` for regex mode, `-F` for
fixed strings, or `--glob` for AFS glob matching semantics.

Flags:

| Flag | Meaning |
| --- | --- |
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
afs fs repo grep "hello"
afs fs repo grep -E "error|warning"
afs fs repo grep -w --path /logs token
afs fs repo grep -l -i "disk full"
afs fs repo grep --glob --path /src "*TODO*"
```

### `afs fs query`

```bash
afs fs [workspace] query [flags] <query>
afs fs [workspace] query --keyword <query>
afs fs [workspace] query --semantic <query>
afs fs [workspace] query index status
afs fs [workspace] query index rebuild --wait
afs query model status
afs query model download
afs fs [workspace] query index clean --yes
```

Ranks workspace files for a concept or natural-language question. Plain
`query` is the recommended hybrid surface and currently falls back to
keyword-ranked results until hybrid vector/rerank is complete. Keyword ranking
uses Redis Search BM25 over a chunk-level HASH projection when available, with
a direct content-ranking fallback. `query --semantic` runs vector-only
retrieval through the global embedding provider. Semantic embeddings are on by
default; AFS runs real embedding generation through OpenAI when
`OPENAI_API_KEY` is set in the control-plane environment, with local GGUF
available as an explicit provider override. It manages a global local GGUF
model cache for the local provider path. Redis vector KNN is used when
available, with a direct vector-ranking fallback. Use `grep` when you know the
exact text.

Existing workspaces are backfilled automatically on first query when Redis
Search is available. `query index status --json` can also catch up pending work
while it inspects files, ready chunks, pending work, skipped files, and
unindexed files. Use `query index rebuild --wait` to force an immediate
backfill. Use `query index clean --yes` to clear generated query chunks and
restart indexing from scratch without changing workspace files.

Flags:

| Flag | Meaning |
| --- | --- |
| `--path <path>` | Scope search to a workspace path. |
| `-n`, `--limit <num>` | Maximum results. |
| `--all` | Return all results. |
| `--min-score <num>` | Minimum score. |
| `--json` | Write JSON output. |
| `--files` | Write QMD-style `#id,score,afs://workspace/path` lines, deduplicated to one candidate per file. |
| `--paths` | Show only matching workspace paths. |
| `--csv` | Write CSV output. |
| `--md` | Write Markdown output. |
| `--xml` | Write XML output. |
| `--full` | Include full content. |
| `--line-numbers` | Include line numbers. |
| `--explain` | Include retrieval explanation when available. |
| `--candidate-limit <num>` | Candidate result limit. |
| `--no-rerank` | Disable reranking when available. |
| `--keyword` | Keyword-ranked retrieval only. |
| `--semantic` | Vector-only semantic retrieval through the global embedding provider. Requires a build with semantic retrieval enabled. |
| `--intent <text>` | Extra search intent. |
| `--chunk-strategy <auto|regex>` | Chunk strategy. |

Query index commands:

```bash
afs fs repo query index status
afs fs repo query index create --embeddings --wait
afs fs repo query index rebuild --wait
```

Global local GGUF model commands:

```bash
afs query model status
afs query model download
afs query model download --model hf:ggml-org/embeddinggemma-300M-GGUF/embeddinggemma-300M-Q8_0.gguf
```

`afs query model download` asks the control plane's local embedding helper to
resolve, download, and load the pure GGUF artifact. The cache is global to the
control plane, not a workspace and not the invoking client's cache. On AFS
Cloud, only an admin identity can trigger this server-side warm-up. The default
model is
`hf:ggml-org/embeddinggemma-300M-GGUF/embeddinggemma-300M-Q8_0.gguf`. Set
`AFS_EMBED_MODEL_DIR` in the control-plane environment to override the cache
directory. GGUF inference uses a managed Node helper with `node-llama-cpp`, the
same runtime shape QMD uses. Set `AFS_EMBED_HELPER_CMD` to override the Node.js
command, or `AFS_NODE_LLAMA_CPP_MODULE` to point at a specific
`node-llama-cpp` module.

OpenAI embedding setup:

```bash
export OPENAI_API_KEY=...
export AFS_EMBED_MODEL=openai:text-embedding-3-small
# Start or restart afs-control-plane from this environment.
afs fs repo query --semantic "how do checkpoints work?"
```

Local GGUF setup:

```bash
export AFS_EMBED_PROVIDER=local
afs query model download
export AFS_EMBED_MODEL=hf:ggml-org/embeddinggemma-300M-GGUF/embeddinggemma-300M-Q8_0.gguf
# Start or restart afs-control-plane from this environment.
```

With `AFS_EMBED_PROVIDER=local`, the control plane embeds with the cached GGUF
through the persistent Node helper. The default EmbeddingGemma model uses 768
dimensions; set `AFS_EMBED_DIMENSIONS` for other local GGUF embedding models.

Optional environment overrides:

```bash
export AFS_EMBED_PROVIDER=openai # or local
export AFS_EMBED_MODEL=openai:text-embedding-3-small
export AFS_EMBED_DIMENSIONS=1536
export AFS_EMBED_MODEL_DIR=~/.cache/afs/models
export AFS_EMBED_HELPER_CMD=node
export AFS_NODE_LLAMA_CPP_MODULE=node-llama-cpp
export OPENAI_BASE_URL=https://api.openai.com
```

`OPENAI_API_KEY`, `AFS_EMBED_MODEL`, `AFS_EMBED_PROVIDER`, and
`AFS_EMBED_DIMENSIONS` are read by the control-plane process, not by each
individual `afs query` invocation. `afs query model status/download` call the
control plane to inspect or populate the global local GGUF cache.

Typed query documents are supported on the default `query` mode:

```bash
afs fs repo query $'lex: checkpoint\nvec: how do I save a snapshot?'
afs fs repo query $'intent: mount setup\nlex: "mount backend"\nhyde: setup stores the selected backend in config'
afs fs repo query --files "checkpoint recovery"
afs fs repo query --paths "checkpoint recovery"
```

Top-level shortcuts are also available:

```bash
afs grep "DirtyHint"
afs query "how do checkpoints work?"
```

The shortcuts use the saved default workspace. Use
`afs fs <workspace> grep` or `afs fs <workspace> query` to choose a workspace
explicitly.

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
mount.

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

### `afs fs ls`

```bash
afs fs [workspace] ls [path] [--json|--files]
```

Lists files under a workspace path. `--json` returns the same workspace/path
envelope plus tree items. `--files` prints only workspace paths.

### `afs fs get`

```bash
afs fs [workspace] get <path>[:line] [--from <line>] [-l <lines>] [--line-numbers]
```

Prints a workspace text file, optionally sliced to a line range. This is the
QMD-style targeted read surface for agents that need a small part of a file.

Examples:

```bash
afs fs repo get README.md:120 -l 30
afs fs repo get docs/guide.md --from 40 -l 10 --line-numbers
```

### `afs fs multi-get`

```bash
afs fs [workspace] multi-get <pattern> [-l <lines>] [--max-bytes <bytes>] [--json|--csv|--md|--xml|--files]
```

Fetches multiple workspace text files by glob or comma-separated path list.
Default output uses QMD-style file separators. Use structured formats for agent
consumption.

Examples:

```bash
afs fs repo multi-get 'docs/*.md' --md
afs fs repo multi-get README.md,docs/guide.md --json
```

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

## Agent Skill

```bash
afs skill <show|install> [options]
afs --skill
```

`afs skill show` prints the packaged AFS skill. `afs --skill` is an alias for
`afs skill show`.

`afs skill install` installs the packaged skill into `./.agents/skills/afs`.
Use `afs skill install --global` to install into `~/.agents/skills/afs`.

Options:

| Option | Description |
| --- | --- |
| `--global` | Install into `~/.agents/skills/afs`. |
| `--yes` | Also create the `.claude/skills/afs` symlink. |
| `-f`, `--force` | Replace an existing install or symlink. |
