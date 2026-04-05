# Task 003: MCP Server (redis-fs-mcp)

## Overview

Create an MCP (Model Context Protocol) server that exposes Redis-FS operations as tools for Claude and other MCP-compatible agents. This provides native tool integration without requiring the agent to use redis-cli.

## Design Goals

1. **Uses Python library**: Built on top of `redis_fs` package (Task 002)
2. **Standard MCP protocol**: Compatible with Claude Desktop, Claude Code, and other MCP clients
3. **Complete coverage**: Exposes all useful Redis-FS operations as tools
4. **Clear documentation**: Each tool has a description that helps the agent understand when to use it

## Tool Definitions

### Reading Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `fs_read` | Read entire file content | `key`, `path` |
| `fs_lines` | Read specific line range | `key`, `path`, `start`, `end` |
| `fs_head` | Read first N lines | `key`, `path`, `n=10` |
| `fs_tail` | Read last N lines | `key`, `path`, `n=10` |

### Writing Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `fs_write` | Write content to file (overwrites) | `key`, `path`, `content` |
| `fs_append` | Append content to file | `key`, `path`, `content` |
| `fs_insert` | Insert content after line N | `key`, `path`, `line`, `content` |

### Editing Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `fs_replace` | Replace text in file | `key`, `path`, `old`, `new`, `all=False`, `line_start`, `line_end` |
| `fs_delete_lines` | Delete line range | `key`, `path`, `start`, `end` |

### Navigation Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `fs_ls` | List directory contents | `key`, `path`, `long=False` |
| `fs_tree` | Show directory tree | `key`, `path` |
| `fs_find` | Find files by pattern | `key`, `path`, `pattern`, `type=None` |
| `fs_stat` | Get file/directory info | `key`, `path` |
| `fs_exists` | Check if path exists | `key`, `path` |

### Search Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `fs_grep` | Search file contents | `key`, `path`, `pattern`, `nocase=False` |

### Organization Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `fs_mkdir` | Create directory | `key`, `path`, `parents=False` |
| `fs_rm` | Remove file or directory | `key`, `path`, `recursive=False` |
| `fs_cp` | Copy file or directory | `key`, `src`, `dst`, `recursive=False` |
| `fs_mv` | Move/rename file or directory | `key`, `src`, `dst` |
| `fs_ln` | Create symbolic link | `key`, `target`, `link` |

### Stats Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `fs_wc` | Word/line/char counts | `key`, `path` |
| `fs_info` | Filesystem statistics | `key` |

## Subtasks

### 1. Create mcp_server package structure
```
mcp_server/
├── __init__.py
├── server.py         # MCP server implementation
└── tools.py          # Tool definitions and handlers
```

### 2. Implement MCP tool definitions
- [ ] Define all tools with JSON Schema for parameters
- [ ] Write clear descriptions for agent understanding
- [ ] Group tools logically in the tool list

### 3. Implement MCP server
```python
# server.py
from mcp.server import Server
from redis_fs import RedisFS
import redis

app = Server("redis-fs")

@app.tool()
async def fs_read(key: str, path: str) -> str:
    """Read entire file content from Redis-FS."""
    fs = RedisFS(redis.Redis(), key)
    content = fs.read(path)
    return content if content else ""

# ... more tools
```

### 4. Add MCP server tests
- [ ] Test each tool invocation
- [ ] Test error handling (file not found, etc.)
- [ ] Test parameter validation

### 5. Add MCP config example
Create `examples/claude_desktop_config.json`:
```json
{
  "mcpServers": {
    "redis-fs": {
      "command": "python",
      "args": ["-m", "mcp_server"],
      "env": {
        "REDIS_URL": "redis://localhost:6379"
      }
    }
  }
}
```

### 6. Add to pyproject.toml
```toml
[project.optional-dependencies]
mcp = ["mcp>=1.0.0"]

[project.scripts]
redis-fs-mcp = "mcp_server:main"
```

## Usage

### Claude Desktop
Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:
```json
{
  "mcpServers": {
    "redis-fs": {
      "command": "redis-fs-mcp",
      "env": {"REDIS_URL": "redis://localhost:6379"}
    }
  }
}
```

### Claude Code / Other MCP Clients
```bash
# Start the MCP server
python -m mcp_server

# Or if installed
redis-fs-mcp
```

## Success Criteria
- [ ] All tools properly defined with schemas
- [ ] Server starts and responds to MCP protocol
- [ ] Tools delegate to Python library correctly
- [ ] Example config works with Claude Desktop
- [ ] Tests pass for all tool invocations

