"""Tests for the MCP server.

Tests that the MCP server module can be imported and the server created.
Full functional testing of the tools is done via test_redis_fs.py since
the MCP server is a thin wrapper around the Python library.

Requires Redis server with fs.so loaded on port 6399:
    redis-server --port 6399 --loadmodule ./fs.so
"""

import os
import pytest

# Set test Redis port before importing
os.environ["REDIS_PORT"] = "6399"


def test_mcp_server_import():
    """Test that MCP server module can be imported."""
    from mcp_server import create_server
    assert create_server is not None


def test_mcp_server_creation():
    """Test that MCP server can be created."""
    from mcp_server import create_server
    server = create_server()
    assert server is not None
    assert server.name == "redis-fs"

