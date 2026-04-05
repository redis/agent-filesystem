# Task 001: Agent Skill (npx skills compatible)

## Overview

Create an installable agent skill that teaches AI agents how to use Redis-FS for persistent storage of text-based content like memories, markdown documents, state, and task lists.

## Key Points for the Skill

### What Redis-FS IS For
- **Memories**: Store and retrieve agent memories, conversation history, learned facts
- **Markdown documents**: Notes, documentation, READMEs, plans
- **State**: Configuration, preferences, session state as JSON/YAML
- **Task lists**: TODO files, work tracking, project state
- **Logs**: Append-only logs, audit trails, activity history
- **Text-based data**: Any UTF-8 content that benefits from filesystem semantics

### What Redis-FS is NOT For
- **Binaries**: Cannot execute programs (not a real filesystem)
- **Scripts**: Cannot run shell scripts, Python, etc.
- **Large media files**: Not optimized for images, videos, audio
- **System files**: No `/etc`, `/bin`, `/usr` semantics

### Integration Methods
The skill should guide agents to use Redis-FS via:
1. **MCP Server** (preferred if available): Native tool calls like `fs_read`, `fs_write`, `fs_replace`
2. **Python library** (for Python agents): `from redis_fs import RedisFS`
3. **Direct redis-cli** (fallback): `redis-cli FS.CAT myfs /path/to/file.txt`

## Subtasks

### 1. Restructure skill directory layout
- [ ] Create `skills/redis-fs/` directory
- [ ] Move/create `SKILL.md` in that location
- [ ] Ensure compatible with `npx skills add` discovery

### 2. Update SKILL.md frontmatter
Required YAML frontmatter per agent skills spec:
```yaml
---
name: redis-fs
description: Persistent filesystem storage in Redis for agent memories, documents, state, and tasks. Use via MCP tools, Python library, or redis-cli.
---
```

### 3. Update SKILL.md content
Include sections for:
- When to use this skill (memories, markdown, state, tasks)
- When NOT to use this skill (binaries, scripts, executables)
- Available commands grouped by purpose:
  - **Reading**: `FS.CAT`, `FS.LINES`, `FS.HEAD`, `FS.TAIL`
  - **Writing**: `FS.ECHO`, `FS.APPEND`, `FS.INSERT`
  - **Editing**: `FS.REPLACE`, `FS.DELETELINES`
  - **Navigation**: `FS.LS`, `FS.TREE`, `FS.FIND`, `FS.STAT`
  - **Search**: `FS.GREP`
  - **Organization**: `FS.MKDIR`, `FS.CP`, `FS.MV`, `FS.RM`, `FS.LN`
  - **Stats**: `FS.WC`, `FS.INFO`
- Integration options (MCP > Python > CLI)
- Examples for common agent workflows

### 4. Add install-skill target to Makefile
Uses Vercel's npx skills CLI to install to ALL detected agents (Claude Code, Cursor, Codex, Windsurf, etc.):
```makefile
install-skill:
	npx skills add . --skill redis-fs -g -y
```

### 5. Add install-skill-local target to Makefile
Manual symlink for Claude Code only (no Node.js required):
```makefile
install-skill-local:
	@mkdir -p ~/.claude/skills/redis-fs
	@ln -sf $(PWD)/skills/redis-fs/SKILL.md ~/.claude/skills/redis-fs/SKILL.md
	@echo "Installed redis-fs skill to ~/.claude/skills/redis-fs/"
```

### 6. Test skill installation
- [ ] Test `npx skills add . --skill redis-fs --list`
- [ ] Test `make install-skill-local`
- [ ] Verify skill appears in agent's skill list

## Success Criteria
- [ ] `npx skills add <repo>` discovers and installs the skill
- [ ] `make install-skill` and `make install-skill-local` work
- [ ] Skill content clearly explains use cases and anti-patterns
- [ ] Skill references MCP server and Python library as integration options

