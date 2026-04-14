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

## Redis FS.* Commands (Direct Access)

If you have direct Redis access, you can use the FS module commands:

### Reading

| Command | Example |
|---------|---------|
| `FS.CAT key path` | `FS.CAT vol /file.md` |
| `FS.LINES key path start end` | `FS.LINES vol /file.md 10 20` |
| `FS.HEAD key path [n]` | `FS.HEAD vol /file.md 5` |
| `FS.TAIL key path [n]` | `FS.TAIL vol /file.md 5` |

### Writing

| Command | Example |
|---------|---------|
| `FS.ECHO key path content` | `FS.ECHO vol /file.md "content"` |
| `FS.ECHO key path content APPEND` | `FS.ECHO vol /log.txt "line" APPEND` |
| `FS.INSERT key path line content` | `FS.INSERT vol /file.md 5 "new"` |

### Editing

| Command | Example |
|---------|---------|
| `FS.REPLACE key path old new [ALL]` | `FS.REPLACE vol /f.md "old" "new" ALL` |
| `FS.DELETELINES key path start end` | `FS.DELETELINES vol /file.md 10 15` |

### Navigation

| Command | Example |
|---------|---------|
| `FS.LS key path [LONG]` | `FS.LS vol /notes LONG` |
| `FS.TREE key path [DEPTH n]` | `FS.TREE vol / DEPTH 2` |
| `FS.FIND key path pattern [TYPE f\|d]` | `FS.FIND vol / "*.md" TYPE file` |
| `FS.STAT key path` | `FS.STAT vol /file.md` |
| `FS.TEST key path` | `FS.TEST vol /file.md` |

### Search

| Command | Example |
|---------|---------|
| `FS.GREP key path pattern [NOCASE]` | `FS.GREP vol / "*TODO*" NOCASE` |

### Organization

| Command | Example |
|---------|---------|
| `FS.MKDIR key path [PARENTS]` | `FS.MKDIR vol /a/b/c PARENTS` |
| `FS.RM key path [RECURSIVE]` | `FS.RM vol /old RECURSIVE` |
| `FS.CP key src dst [RECURSIVE]` | `FS.CP vol /a /b RECURSIVE` |
| `FS.MV key src dst` | `FS.MV vol /old.md /new.md` |

## Best Practices

- **Checkpoint before risky operations** — gives you instant rollback
- **Use descriptive workspace names** — easier to find in UI and CLI
- **Use sync mode** for most workflows — best tool compatibility
- **Add `.afsignore`** when importing — exclude `node_modules/`, `.venv/`, build dirs
- **Use MCP for agent-only workflows** — no local mount needed
- **Fork workspaces for parallel experiments** — keeps main workspace clean
- All file paths are **absolute** (start with `/`)
- `FS.GREP` uses **glob patterns** (`*pattern*`), not regex
- Parent directories are **auto-created** by `FS.ECHO`
