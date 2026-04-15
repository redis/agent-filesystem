# Agent Filesystem (AFS) — Agent Guide

<!-- Keep in sync with ui/src/routes/agent-guide.tsx -->

AFS gives you persistent, checkpointed workspaces backed by Redis. You can create workspaces, read/write files, search content, and save/restore checkpoints. Redis is the source of truth; you work through the AFS CLI, MCP tools, or a synced local directory.

## MCP Server Setup

Add this to your MCP configuration (e.g. `claude_desktop_config.json` or `.claude/settings.json`):

```json
{
  "mcpServers": {
    "agent-filesystem": {
      "command": "/absolute/path/to/afs",
      "args": ["mcp"]
    }
  }
}
```

**Important:** The `command` path must be absolute. Relative paths will not resolve.

Running `afs mcp` starts a stdio-based MCP server (JSON-RPC over stdin/stdout) that exposes workspace tools. Configuration is read from `afs.config.json` next to the binary.

## Available Tools

When connected via MCP, you have access to:

| Category | Operations |
|----------|-----------|
| Workspace management | Create, list, select, fork, delete workspaces |
| File operations | Read, write, edit, delete, copy, move files |
| Navigation | List directories, tree view, find by pattern, stat |
| Search | Grep across workspace files directly in Redis |
| Checkpoints | Create snapshots, list history, restore to any point |

The MCP server is workspace-first: file operations are automatically tracked.

## Quick Start

### 1. Create a workspace

```bash
./afs workspace create my-project
```

### 2. Start syncing (optional — only if you want a local directory)

```bash
./afs workspace use my-project
./afs up
# Workspace is now at ~/afs/my-project/
```

### 3. Work with files

If synced, use normal file tools in the local directory. If using MCP, use the exposed file tools directly.

```bash
# Via CLI
echo "# My Project" > ~/afs/my-project/README.md

# Via redis-cli (direct)
redis-cli FS.ECHO my-project /README.md "# My Project"
```

### 4. Create checkpoints

Always checkpoint before risky changes:

```bash
./afs checkpoint create before-refactor
```

To restore:

```bash
./afs checkpoint restore before-refactor
```

### 5. Import an existing directory

```bash
./afs workspace import my-project /path/to/existing/directory
```

Add a `.afsignore` file (same syntax as `.gitignore`) to exclude `node_modules/`, `.venv/`, build artifacts, etc.

### 6. Fork for parallel work

```bash
./afs workspace fork my-project my-project-experiment
```

### 7. Search workspace contents

Search directly in Redis without needing a local mount:

```bash
./afs grep "TODO" --workspace my-project
./afs grep --workspace my-project --path /src -E "function|class"
```

## Configuration

AFS reads `afs.config.json` from the same directory as the `afs` binary:

```json
{
  "redis": {
    "addr": "localhost:6379",
    "username": "",
    "password": "",
    "db": 0,
    "tls": false
  },
  "mode": "sync",
  "currentWorkspace": "my-project",
  "localPath": "~/afs"
}
```

| Field | Description |
|-------|-------------|
| `redis.addr` | Redis host:port |
| `redis.password` | Auth password (empty = no auth) |
| `redis.tls` | Enable TLS |
| `mode` | `sync` (recommended), `mount`, or `none` |
| `currentWorkspace` | Default workspace name |
| `localPath` | Local directory for sync/mount |

## CLI Command Reference

### Setup & Status

| Command | Description |
|---------|-------------|
| `afs setup` | Interactive first-time configuration |
| `afs up` | Start syncing/mounting workspaces |
| `afs down` | Stop services and unmount |
| `afs status` | Show current status |

### Workspaces

| Command | Description |
|---------|-------------|
| `afs workspace create <name>` | Create a new workspace |
| `afs workspace list` | List all workspaces |
| `afs workspace use <name>` | Set the current workspace |
| `afs workspace import <name> <dir>` | Import directory as workspace |
| `afs workspace clone <name> <dir>` | Export workspace to local dir |
| `afs workspace fork <name> <new>` | Fork for parallel work |
| `afs workspace delete <name>` | Remove a workspace |

### Checkpoints

| Command | Description |
|---------|-------------|
| `afs checkpoint create <name>` | Save current state |
| `afs checkpoint list` | List all checkpoints |
| `afs checkpoint restore <name>` | Restore to a checkpoint |

### Search

| Command | Description |
|---------|-------------|
| `afs grep <pattern>` | Search workspace files in Redis |

### MCP

| Command | Description |
|---------|-------------|
| `afs mcp` | Start the MCP server (stdio JSON-RPC) |

## Redis Data Format (Direct Access)

AFS stores workspace data using native Redis types. All keys are scoped under `afs:{workspace}:` using hash tags for cluster compatibility. Here is the key layout for a workspace called `my-project`:

### Workspace metadata

| Key | Type | Description |
|-----|------|-------------|
| `afs:{my-project}:workspace:meta` | Hash (JSON) | Workspace name, head checkpoint, timestamps, source |
| `afs:{my-project}:workspace:savepoints` | Sorted Set | Checkpoint IDs ordered by creation time |
| `afs:{my-project}:workspace:audit` | Stream | Activity log (create, restore, session events) |
| `afs:{my-project}:workspace:sessions` | Sorted Set | Active agent session IDs |

### Checkpoints

| Key | Type | Description |
|-----|------|-------------|
| `afs:{my-project}:savepoint:{name}:meta` | Hash (JSON) | Checkpoint metadata: author, parent, file/dir counts, size |
| `afs:{my-project}:savepoint:{name}:manifest` | Hash (JSON) | Full file tree: path → type, mode, size, inline content or blob ID |
| `afs:{my-project}:blob:{sha256}` | String | File content for files larger than 4 KB (deduplicated by hash) |

### Live workspace root (inode tree)

The live workspace is a materialized inode tree that agents and mounts operate on:

| Key | Type | Description |
|-----|------|-------------|
| `afs:{my-project}:info` | Hash | Schema version, file/dir/symlink counts, total bytes |
| `afs:{my-project}:inode:{id}` | Hash | Inode fields: `type`, `mode`, `size`, `mtime_ms`, `content`, `name`, `path` |
| `afs:{my-project}:dirents:{id}` | Hash | Directory entries: child name → child inode ID |
| `afs:{my-project}:content:{id}` | String | External file content (for files exceeding inline threshold) |
| `afs:{my-project}:root_head_savepoint` | String | Checkpoint ID the live root was materialized from |
| `afs:{my-project}:root_dirty` | String | `"1"` if the live root has unsaved changes |

Inode `1` is always the root directory (`/`). Each inode hash contains:

| Field | Description |
|-------|-------------|
| `type` | `file`, `dir`, or `symlink` |
| `mode` | Unix permission bits (e.g. `0644`) |
| `size` | File size in bytes |
| `content` | File content (inline for small files) |
| `mtime_ms` | Last modified timestamp (milliseconds) |
| `path` | Absolute path (e.g. `/src/main.py`) |
| `name` | Filename (e.g. `main.py`) |
| `parent` | Parent inode ID |

## Best Practices

- **Checkpoint before risky operations** — gives you instant rollback
- **Use descriptive workspace names** — easier to find in UI and CLI
- **Use sync mode** for most workflows — best tool compatibility
- **Add `.afsignore`** when importing — exclude `node_modules/`, `.venv/`, build dirs
- **Use MCP for agent-only workflows** — no local mount needed
- **Fork workspaces for parallel experiments** — keeps main workspace clean
- All file paths are **absolute** (start with `/`)
