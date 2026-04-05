"""Redis-FS MCP Server implementation."""

import os
import asyncio
from typing import Any

from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import Tool, TextContent
import redis

from redis_fs import RedisFS

# Default port for HTTP transport
DEFAULT_HTTP_PORT = 8089


def get_redis() -> redis.Redis:
    """Get Redis client from environment."""
    url = os.environ.get("REDIS_URL")
    if url:
        return redis.from_url(url)
    
    host = os.environ.get("REDIS_HOST", "localhost")
    port = int(os.environ.get("REDIS_PORT", "6379"))
    db = int(os.environ.get("REDIS_DB", "0"))
    return redis.Redis(host=host, port=port, db=db)


def create_server() -> Server:
    """Create and configure the MCP server."""
    server = Server("redis-fs")
    r = get_redis()

    def get_fs(key: str) -> RedisFS:
        return RedisFS(r, key)

    @server.list_tools()
    async def list_tools() -> list[Tool]:
        """List available tools."""
        return [
            Tool(
                name="fs_read",
                description="Read entire file content from Redis-FS",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "key": {"type": "string", "description": "Filesystem volume key"},
                        "path": {"type": "string", "description": "Absolute file path"},
                    },
                    "required": ["key", "path"],
                },
            ),
            Tool(
                name="fs_write",
                description="Write content to file in Redis-FS (creates parents)",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "key": {"type": "string", "description": "Filesystem volume key"},
                        "path": {"type": "string", "description": "Absolute file path"},
                        "content": {"type": "string", "description": "Content to write"},
                    },
                    "required": ["key", "path", "content"],
                },
            ),
            Tool(
                name="fs_append",
                description="Append content to file in Redis-FS",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "key": {"type": "string", "description": "Filesystem volume key"},
                        "path": {"type": "string", "description": "Absolute file path"},
                        "content": {"type": "string", "description": "Content to append"},
                    },
                    "required": ["key", "path", "content"],
                },
            ),
            Tool(
                name="fs_lines",
                description="Read specific line range from file (1-indexed, end=-1 for EOF)",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "key": {"type": "string", "description": "Filesystem volume key"},
                        "path": {"type": "string", "description": "Absolute file path"},
                        "start": {"type": "integer", "description": "Start line (1-indexed)"},
                        "end": {"type": "integer", "description": "End line (-1 for EOF)", "default": -1},
                    },
                    "required": ["key", "path", "start"],
                },
            ),
            Tool(
                name="fs_replace",
                description="Replace text in file",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "key": {"type": "string", "description": "Filesystem volume key"},
                        "path": {"type": "string", "description": "Absolute file path"},
                        "old": {"type": "string", "description": "Text to find"},
                        "new": {"type": "string", "description": "Replacement text"},
                        "all": {"type": "boolean", "description": "Replace all occurrences", "default": False},
                        "line_start": {"type": "integer", "description": "Constrain to lines >= this"},
                        "line_end": {"type": "integer", "description": "Constrain to lines <= this"},
                    },
                    "required": ["key", "path", "old", "new"],
                },
            ),
            Tool(
                name="fs_insert",
                description="Insert content after line N (0=beginning, -1=end)",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "key": {"type": "string", "description": "Filesystem volume key"},
                        "path": {"type": "string", "description": "Absolute file path"},
                        "line": {"type": "integer", "description": "Line number to insert after"},
                        "content": {"type": "string", "description": "Content to insert"},
                    },
                    "required": ["key", "path", "line", "content"],
                },
            ),
            Tool(
                name="fs_delete_lines",
                description="Delete line range from file",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "key": {"type": "string", "description": "Filesystem volume key"},
                        "path": {"type": "string", "description": "Absolute file path"},
                        "start": {"type": "integer", "description": "Start line (1-indexed)"},
                        "end": {"type": "integer", "description": "End line (inclusive)"},
                    },
                    "required": ["key", "path", "start", "end"],
                },
            ),
            Tool(
                name="fs_ls",
                description="List directory contents",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "key": {"type": "string", "description": "Filesystem volume key"},
                        "path": {"type": "string", "description": "Directory path", "default": "/"},
                    },
                    "required": ["key"],
                },
            ),
            Tool(
                name="fs_find",
                description="Find files matching glob pattern",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "key": {"type": "string", "description": "Filesystem volume key"},
                        "path": {"type": "string", "description": "Starting path"},
                        "pattern": {"type": "string", "description": "Glob pattern (e.g., *.md)"},
                        "type": {"type": "string", "enum": ["file", "dir", "link"], "description": "Filter by type"},
                    },
                    "required": ["key", "path", "pattern"],
                },
            ),
            Tool(
                name="fs_grep",
                description="Search file contents with glob pattern",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "key": {"type": "string", "description": "Filesystem volume key"},
                        "path": {"type": "string", "description": "Search path"},
                        "pattern": {"type": "string", "description": "Glob pattern (e.g., *TODO*)"},
                        "nocase": {"type": "boolean", "description": "Case-insensitive", "default": False},
                    },
                    "required": ["key", "path", "pattern"],
                },
            ),
            Tool(
                name="fs_mkdir",
                description="Create directory",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "key": {"type": "string", "description": "Filesystem volume key"},
                        "path": {"type": "string", "description": "Directory path"},
                        "parents": {"type": "boolean", "description": "Create parent dirs", "default": False},
                    },
                    "required": ["key", "path"],
                },
            ),
            Tool(
                name="fs_rm",
                description="Remove file or directory",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "key": {"type": "string", "description": "Filesystem volume key"},
                        "path": {"type": "string", "description": "Path to remove"},
                        "recursive": {"type": "boolean", "description": "Remove recursively", "default": False},
                    },
                    "required": ["key", "path"],
                },
            ),
            Tool(
                name="fs_info",
                description="Get filesystem statistics",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "key": {"type": "string", "description": "Filesystem volume key"},
                    },
                    "required": ["key"],
                },
            ),
        ]

    @server.call_tool()
    async def call_tool(name: str, arguments: dict[str, Any]) -> list[TextContent]:
        """Handle tool calls."""
        key = arguments.get("key", "")
        fs = get_fs(key)

        try:
            if name == "fs_read":
                result = fs.read(arguments["path"])
                return [TextContent(type="text", text=result or "")]

            elif name == "fs_write":
                fs.write(arguments["path"], arguments["content"])
                return [TextContent(type="text", text="OK")]

            elif name == "fs_append":
                fs.append(arguments["path"], arguments["content"])
                return [TextContent(type="text", text="OK")]

            elif name == "fs_lines":
                end = arguments.get("end", -1)
                result = fs.lines(arguments["path"], arguments["start"], end)
                return [TextContent(type="text", text=result or "")]

            elif name == "fs_replace":
                count = fs.replace(
                    arguments["path"],
                    arguments["old"],
                    arguments["new"],
                    all=arguments.get("all", False),
                    line_start=arguments.get("line_start"),
                    line_end=arguments.get("line_end"),
                )
                return [TextContent(type="text", text=f"{count} replacement(s)")]

            elif name == "fs_insert":
                fs.insert(arguments["path"], arguments["line"], arguments["content"])
                return [TextContent(type="text", text="OK")]

            elif name == "fs_delete_lines":
                count = fs.delete_lines(arguments["path"], arguments["start"], arguments["end"])
                return [TextContent(type="text", text=f"{count} line(s) deleted")]

            elif name == "fs_ls":
                path = arguments.get("path", "/")
                entries = fs.ls(path)
                return [TextContent(type="text", text="\n".join(entries))]

            elif name == "fs_find":
                results = fs.find(
                    arguments["path"],
                    arguments["pattern"],
                    type=arguments.get("type"),
                )
                return [TextContent(type="text", text="\n".join(results))]

            elif name == "fs_grep":
                results = fs.grep(
                    arguments["path"],
                    arguments["pattern"],
                    nocase=arguments.get("nocase", False),
                )
                return [TextContent(type="text", text="\n".join(str(r) for r in results))]

            elif name == "fs_mkdir":
                fs.mkdir(arguments["path"], parents=arguments.get("parents", False))
                return [TextContent(type="text", text="OK")]

            elif name == "fs_rm":
                fs.rm(arguments["path"], recursive=arguments.get("recursive", False))
                return [TextContent(type="text", text="OK")]

            elif name == "fs_info":
                info = fs.info()
                text = "\n".join(f"{k}: {v}" for k, v in info.items())
                return [TextContent(type="text", text=text)]

            else:
                return [TextContent(type="text", text=f"Unknown tool: {name}")]

        except Exception as e:
            return [TextContent(type="text", text=f"Error: {e}")]

    return server


async def run_stdio_server():
    """Run the MCP server with stdio transport."""
    server = create_server()
    async with stdio_server() as (read_stream, write_stream):
        await server.run(read_stream, write_stream, server.create_initialization_options())


async def run_http_server(host: str = "0.0.0.0", port: int = DEFAULT_HTTP_PORT):
    """Run the MCP server with HTTP/SSE transport."""
    from mcp.server.sse import SseServerTransport
    from starlette.applications import Starlette
    from starlette.routing import Route, Mount
    from starlette.responses import JSONResponse
    import uvicorn

    server = create_server()
    sse = SseServerTransport("/messages/")

    async def handle_sse(request):
        async with sse.connect_sse(
            request.scope, request.receive, request._send
        ) as streams:
            await server.run(
                streams[0], streams[1], server.create_initialization_options()
            )

    async def handle_messages(request):
        await sse.handle_post_message(request.scope, request.receive, request._send)

    async def health(request):
        return JSONResponse({"status": "ok", "server": "redis-fs-mcp"})

    app = Starlette(
        routes=[
            Route("/health", health),
            Route("/sse", handle_sse),
            Mount("/messages", routes=[Route("/", handle_messages, methods=["POST"])]),
        ]
    )

    print(f"Starting Redis-FS MCP server on http://{host}:{port}")
    print(f"  SSE endpoint: http://{host}:{port}/sse")
    print(f"  Health check: http://{host}:{port}/health")

    config = uvicorn.Config(app, host=host, port=port, log_level="info")
    server_instance = uvicorn.Server(config)
    await server_instance.serve()


def main():
    """Entry point."""
    import argparse

    parser = argparse.ArgumentParser(description="Redis-FS MCP Server")
    parser.add_argument(
        "--transport", "-t",
        choices=["stdio", "http"],
        default="stdio",
        help="Transport type (default: stdio)"
    )
    parser.add_argument(
        "--host", "-H",
        default="0.0.0.0",
        help="Host for HTTP transport (default: 0.0.0.0)"
    )
    parser.add_argument(
        "--port", "-p",
        type=int,
        default=DEFAULT_HTTP_PORT,
        help=f"Port for HTTP transport (default: {DEFAULT_HTTP_PORT})"
    )

    args = parser.parse_args()

    if args.transport == "http":
        asyncio.run(run_http_server(args.host, args.port))
    else:
        asyncio.run(run_stdio_server())


if __name__ == "__main__":
    main()

