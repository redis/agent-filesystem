---
name: redis-fs
description: Persistent filesystem storage in Redis for agent memories, documents, state, and tasks. NOT for binaries or executables. Use via MCP tools, Python library, or redis-cli.
---

# Redis-FS: Distributed Filesystem for Agents

Redis-FS provides persistent, distributed filesystem storage backed by Redis. One Redis key = one filesystem volume.

## When to Use This Skill

**USE FOR:**
- 📝 **Memories**: Conversation history, learned facts, agent context
- 📄 **Markdown documents**: Notes, documentation, plans, READMEs
- 🔧 **State/Config**: JSON/YAML configuration, session state, preferences
- ✅ **Task lists**: TODOs, work tracking, project state files
- 📊 **Logs**: Append-only logs, audit trails, activity history
- 💾 **Text data**: Any UTF-8 content benefiting from filesystem semantics

**DO NOT USE FOR:**
- ❌ **Binaries/Executables**: Cannot run programs (not a real filesystem)
- ❌ **Scripts**: Cannot execute shell scripts, Python, etc.
- ❌ **Large media**: Not optimized for images, videos, audio
- ❌ **System files**: No `/bin`, `/usr`, process execution

## Integration Methods (in order of preference)

### 1. MCP Server (if available)
If the `redis-fs` MCP server is configured, use native tools:
```
fs_read, fs_write, fs_lines, fs_replace, fs_insert, fs_grep, fs_find, fs_ls
```

### 2. Python Library
```python
from redis_fs import RedisFS
fs = RedisFS(redis_client, "myvolume")
content = fs.read("/memories/context.md")
fs.write("/tasks/todo.md", "# Tasks\n- Item 1")
```

### 3. CLI (`redis-fs` command)
```bash
redis-fs cat myvolume /memories/context.md
redis-fs echo myvolume /tasks/todo.md "# Tasks"
```

### 4. Direct redis-cli
```bash
redis-cli FS.CAT myvolume /memories/context.md
redis-cli FS.ECHO myvolume /tasks/todo.md "# Tasks"
```

## Command Reference

### Reading Files
| Command | Description | Example |
|---------|-------------|---------|
| `FS.CAT key path` | Read entire file | `FS.CAT vol /file.md` |
| `FS.LINES key path start end` | Read line range (1-indexed, -1=EOF) | `FS.LINES vol /file.md 10 20` |
| `FS.HEAD key path [n]` | First N lines (default 10) | `FS.HEAD vol /file.md 5` |
| `FS.TAIL key path [n]` | Last N lines (default 10) | `FS.TAIL vol /file.md 5` |

### Writing Files
| Command | Description | Example |
|---------|-------------|---------|
| `FS.ECHO key path content` | Write file (creates parents) | `FS.ECHO vol /file.md "content"` |
| `FS.ECHO key path content APPEND` | Append to file | `FS.ECHO vol /log.txt "line" APPEND` |
| `FS.INSERT key path line content` | Insert after line N (0=start, -1=end) | `FS.INSERT vol /file.md 5 "new line"` |

### Editing Files (Agent-Friendly)
| Command | Description | Example |
|---------|-------------|---------|
| `FS.REPLACE key path old new [ALL] [LINE s e]` | Replace text | `FS.REPLACE vol /f.md "old" "new" ALL` |
| `FS.DELETELINES key path start end` | Delete line range | `FS.DELETELINES vol /file.md 10 15` |

### Navigation
| Command | Description | Example |
|---------|-------------|---------|
| `FS.LS key path [LONG]` | List directory | `FS.LS vol /notes LONG` |
| `FS.TREE key path [DEPTH n]` | Directory tree | `FS.TREE vol / DEPTH 2` |
| `FS.FIND key path pattern [TYPE f\|d\|l]` | Find by glob pattern | `FS.FIND vol / "*.md" TYPE file` |
| `FS.STAT key path` | File metadata | `FS.STAT vol /file.md` |
| `FS.TEST key path` | Check existence (1/0) | `FS.TEST vol /file.md` |

### Search
| Command | Description | Example |
|---------|-------------|---------|
| `FS.GREP key path pattern [NOCASE]` | Search content (glob) | `FS.GREP vol /notes "*TODO*" NOCASE` |

### Organization
| Command | Description | Example |
|---------|-------------|---------|
| `FS.MKDIR key path [PARENTS]` | Create directory | `FS.MKDIR vol /a/b/c PARENTS` |
| `FS.RM key path [RECURSIVE]` | Remove file/directory | `FS.RM vol /old RECURSIVE` |
| `FS.CP key src dst [RECURSIVE]` | Copy | `FS.CP vol /a /b RECURSIVE` |
| `FS.MV key src dst` | Move/rename | `FS.MV vol /draft.md /final.md` |
| `FS.LN key target link` | Create symlink | `FS.LN vol /current /latest` |

### Stats
| Command | Description | Example |
|---------|-------------|---------|
| `FS.WC key path` | Line/word/char counts | `FS.WC vol /file.md` |
| `FS.INFO key` | Filesystem stats | `FS.INFO vol` |

## Common Agent Workflows

### Store and retrieve memories
```bash
# Save memory
FS.ECHO agent-memory /context/user-preferences.json '{"theme": "dark"}'

# Read memory
FS.CAT agent-memory /context/user-preferences.json
```

### Manage task lists
```bash
# Create task file
FS.ECHO project /tasks/sprint-1.md "# Sprint 1\n- [ ] Task 1\n- [ ] Task 2"

# Update a task (mark complete)
FS.REPLACE project /tasks/sprint-1.md "- [ ] Task 1" "- [x] Task 1"

# Append new task
FS.INSERT project /tasks/sprint-1.md -1 "- [ ] New task"
```

### Edit documents with context
```bash
# View lines 50-60 before editing
FS.LINES project /docs/README.md 50 60

# Replace text within those lines
FS.REPLACE project /docs/README.md "old API" "new API" LINE 50 60

# Insert after line 55
FS.INSERT project /docs/README.md 55 "## New Section"
```

### Search and navigate
```bash
# Find all markdown files
FS.FIND project / "*.md" TYPE file

# Search for TODOs
FS.GREP project / "*TODO*" NOCASE

# Get directory overview
FS.TREE project / DEPTH 2
```

## Key Points
- All paths are **absolute** (start with `/`)
- No working directory — every command needs full path
- `FS.GREP` uses **glob patterns** (`*pattern*`), not regex
- Parent directories are **auto-created** by `FS.ECHO`
- Symlinks resolve automatically (max 40 levels)
- Delete entire filesystem: `DEL volumename`

