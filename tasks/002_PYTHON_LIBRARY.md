# Task 002: Python Library (agent-filesystem-py)

## Overview

Create an ergonomic Python wrapper for Agent Filesystem that provides:
1. A clean, Pythonic API for agents and applications
2. A `agent-filesystem` CLI executable that wraps redis-cli FS.* commands

## Design Goals

1. **Feels like a filesystem**: Methods like `read()`, `write()`, `ls()`, `mkdir()`
2. **Agent-friendly editing**: `replace()`, `insert()`, `delete_lines()` mirror agent editing patterns
3. **Minimal dependencies**: Only requires `redis-py`
4. **Type-safe**: Full type hints, `py.typed` marker for mypy
5. **CLI included**: `agent-filesystem` command for shell usage

## API Design

```python
from agent_filesystem import AgentFilesystem
import redis

# Initialize
r = redis.Redis()
fs = AgentFilesystem(r, "myproject")  # key name in Redis

# Basic I/O
content = fs.read("/notes/todo.md")
fs.write("/notes/todo.md", "# TODO\n- Item 1\n- Item 2")
fs.append("/logs/activity.log", "User logged in\n")

# Line-based operations (agent-friendly)
lines = fs.lines("/src/main.py", start=50, end=60)  # View range
fs.replace("/src/main.py", "old_text", "new_text", line_start=50, line_end=60)
fs.insert("/src/main.py", after_line=10, content="# New comment")
fs.delete_lines("/src/main.py", start=20, end=25)

# Navigation
entries = fs.ls("/notes")
tree = fs.tree("/")
files = fs.find("/", "*.md", type="file")
exists = fs.exists("/notes/todo.md")
info = fs.stat("/notes/todo.md")

# Search
matches = fs.grep("/notes", "TODO")

# Organization
fs.mkdir("/notes/archive", parents=True)
fs.cp("/notes/old.md", "/notes/archive/old.md")
fs.mv("/notes/temp.md", "/notes/final.md")
fs.rm("/notes/draft.md")
fs.ln("/notes/latest.md", "/notes/current.md")  # symlink

# Utils
fs.head("/logs/app.log", n=20)
fs.tail("/logs/app.log", n=50)
stats = fs.wc("/notes/todo.md")  # {"lines": 10, "words": 50, "chars": 300}
```

## Subtasks

### 1. Create agent_filesystem package structure
```
agent_filesystem/
├── __init__.py      # Exports AgentFilesystem
├── client.py        # AgentFilesystem class implementation
├── cli.py           # Click-based CLI (agent-filesystem command)
├── exceptions.py    # Custom exceptions (NotAFile, NotADirectory, etc.)
└── py.typed         # Marker for type checkers
```

### 2. Implement AgentFilesystem client class

Core methods to implement:

| Method | Redis Command | Notes |
|--------|--------------|-------|
| `read(path)` | `FS.CAT` | Returns str or None |
| `write(path, content)` | `FS.ECHO` | Creates parent dirs |
| `append(path, content)` | `FS.APPEND` | |
| `lines(path, start, end)` | `FS.LINES` | 1-indexed |
| `replace(path, old, new, ...)` | `FS.REPLACE` | Returns count |
| `insert(path, after_line, content)` | `FS.INSERT` | |
| `delete_lines(path, start, end)` | `FS.DELETELINES` | Returns count |
| `head(path, n=10)` | `FS.HEAD` | |
| `tail(path, n=10)` | `FS.TAIL` | |
| `wc(path)` | `FS.WC` | Returns dict |
| `ls(path)` | `FS.LS` | Returns list |
| `tree(path)` | `FS.TREE` | Returns str |
| `find(path, pattern, type=None)` | `FS.FIND` | |
| `grep(path, pattern, nocase=False)` | `FS.GREP` | |
| `stat(path)` | `FS.STAT` | Returns dict |
| `exists(path)` | `FS.TEST` | Returns bool |
| `mkdir(path, parents=False)` | `FS.MKDIR` | |
| `rm(path, recursive=False)` | `FS.RM` | |
| `cp(src, dst, recursive=False)` | `FS.CP` | |
| `mv(src, dst)` | `FS.MV` | |
| `ln(target, link)` | `FS.LN` | Symlink |
| `readlink(path)` | `FS.READLINK` | |
| `info()` | `FS.INFO` | Filesystem stats |

### 3. Add Python tests
```
tests/
├── test_agent_filesystem.py    # Integration tests (requires Redis + fs.so)
```

Test categories:
- [ ] Basic read/write operations
- [ ] Line-based editing (replace, insert, delete_lines)
- [ ] Navigation (ls, find, tree)
- [ ] Search (grep)
- [ ] Organization (mkdir, cp, mv, rm)
- [ ] Error handling (not found, wrong type)

### 4. Implement CLI (agent-filesystem command)
- [ ] Implement all commands using Click
- [ ] Support `--host`, `--port`, `--db`, `--url` options
- [ ] Mirror Unix command syntax (cat, ls, grep, etc.)
- [ ] Proper exit codes and error messages

### 5. Create pyproject.toml
```toml
[project]
name = "agent-filesystem"
version = "0.1.0"
description = "Python client and CLI for Agent Filesystem filesystem module"
requires-python = ">=3.8"
dependencies = ["redis>=4.0.0", "click>=8.0.0"]

[project.optional-dependencies]
dev = ["pytest", "pytest-redis"]

[project.scripts]
agent-filesystem = "agent_filesystem.cli:cli"
```

## CLI Design (`agent-filesystem` command)

Thin wrapper that mirrors Unix commands but talks to Agent Filesystem:

```bash
# Reading
agent-filesystem cat <key> <path>
agent-filesystem head <key> <path> [-n 10]
agent-filesystem tail <key> <path> [-n 10]
agent-filesystem lines <key> <path> <start> [end]

# Writing
agent-filesystem echo <key> <path> <content> [--append]
agent-filesystem insert <key> <path> <line> <content>

# Editing
agent-filesystem replace <key> <path> <old> <new> [--all] [--line START END]
agent-filesystem delete-lines <key> <path> <start> <end>

# Navigation
agent-filesystem ls <key> <path> [-l]
agent-filesystem tree <key> <path>
agent-filesystem find <key> <path> <pattern> [--type f|d|l]
agent-filesystem stat <key> <path>

# Search
agent-filesystem grep <key> <path> <pattern> [--nocase]

# Organization
agent-filesystem mkdir <key> <path> [-p]
agent-filesystem rm <key> <path> [-r]
agent-filesystem cp <key> <src> <dst> [-r]
agent-filesystem mv <key> <src> <dst>
agent-filesystem ln <key> <target> <link>

# Stats
agent-filesystem wc <key> <path>
agent-filesystem info <key>

# Connection options (all commands)
agent-filesystem --host localhost --port 6379 --db 0 cat myfs /file.txt
agent-filesystem --url redis://localhost:6379/0 cat myfs /file.txt
```

### Implementation

```python
# agent_filesystem/cli.py
import click
from agent_filesystem import AgentFilesystem
import redis

@click.group()
@click.option('--host', default='localhost')
@click.option('--port', default=6379)
@click.option('--db', default=0)
@click.option('--url', default=None)
@click.pass_context
def cli(ctx, host, port, db, url):
    if url:
        ctx.obj = redis.from_url(url)
    else:
        ctx.obj = redis.Redis(host=host, port=port, db=db)

@cli.command()
@click.argument('key')
@click.argument('path')
@click.pass_context
def cat(ctx, key, path):
    """Read file content."""
    fs = AgentFilesystem(ctx.obj, key)
    content = fs.read(path)
    if content:
        click.echo(content)

# ... more commands
```

### 5. Add CLI to pyproject.toml

```toml
[project.scripts]
agent-filesystem = "agent_filesystem.cli:cli"
```

## Success Criteria
- [ ] All FS.* commands wrapped with Pythonic API
- [ ] Type hints on all public methods
- [ ] Tests pass against real Redis with fs.so loaded
- [ ] Can be installed with `pip install -e .`
- [ ] `agent-filesystem` CLI works for all commands
- [ ] CLI mirrors familiar Unix command syntax
