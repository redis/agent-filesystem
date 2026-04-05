# Add agent integration: Python library, MCP server, and skill

This PR adds comprehensive agent integration for Redis-FS, enabling AI agents to use Redis-backed filesystem storage for memories, documents, state, and tasks.

## Features

### Python Library (`redis_fs/`)
- `RedisFS` client class wrapping all FS.* commands with Pythonic API
- `redis-fs` CLI with Unix-like commands (cat, ls, grep, replace, etc.)
- Full type hints and pip-installable via `pyproject.toml`

### MCP Server (`mcp_server/`)
- HTTP/SSE transport for integration with Claude, Auggie, and other MCP clients
- 13 tools: `fs_read`, `fs_write`, `fs_replace`, `fs_insert`, `fs_grep`, etc.
- Health check endpoint at `/health`

### Agent Skill (`skills/redis-fs/`)
- Compatible with `npx skills add` for 37+ agents
- Documents use cases (memories, markdown, state) and anti-patterns (binaries, scripts)
- Complete command reference with examples

### Docker Support
- `docker-compose.yml` orchestrates Redis + MCP server
- `make mcp-up` builds and starts everything

## Usage

```bash
make mcp-up              # Start Redis + MCP server
make install-mcp-auggie  # Add to Auggie CLI
make install-skill-local # Install skill to Claude Code
```

