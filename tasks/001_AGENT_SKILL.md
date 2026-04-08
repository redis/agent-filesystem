# Task 001: Agent Skill (npx skills compatible)

## Overview

Create an installable agent skill that teaches AI agents how to use Agent Filesystem for persistent storage of text-based content like memories, markdown documents, state, and task lists.

## Key Points for the Skill

### What Agent Filesystem IS For
- **Memories**: Store and retrieve agent memories, conversation history, learned facts
- **Markdown documents**: Notes, documentation, READMEs, plans
- **State**: Configuration, preferences, session state as JSON/YAML
- **Task lists**: TODO files, work tracking, project state
- **Logs**: Append-only logs, audit trails, activity history
- **Text-based data**: Any UTF-8 content that benefits from filesystem semantics

### What Agent Filesystem is NOT For
- **Binaries**: Cannot execute programs (not a real filesystem)
- **Scripts**: Cannot run shell scripts, Python, etc.
- **Large media files**: Not optimized for images, videos, audio
- **System files**: No `/etc`, `/bin`, `/usr` semantics

### Integration Methods
The skill should guide agents to use Agent Filesystem via:
1. **`afs mcp`** (preferred): workspace-aware MCP tools for listing workspaces, editing files, grepping, and checkpointing
2. **Mounted workspace + `afs` CLI**: work in a real local directory and use `afs grep` for direct workspace search
3. **Direct redis-cli** (fallback): `redis-cli FS.CAT myfs /path/to/file.txt`

## Subtasks

### 1. Restructure skill directory layout
- [ ] Create `skills/agent-filesystem/` directory
- [ ] Move/create `SKILL.md` in that location
- [ ] Ensure compatible with `npx skills add` discovery

### 2. Update SKILL.md frontmatter
Required YAML frontmatter per agent skills spec:
```yaml
---
name: agent-filesystem
description: Persistent filesystem storage in Redis for agent memories, documents, state, and tasks. Use via `afs mcp`, the `afs` CLI, mounted workspaces, or redis-cli.
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
- Integration options (`afs mcp` > mounted workspace/`afs` > redis-cli)
- Examples for common agent workflows

### 4. Add install-skill target to Makefile
Uses Vercel's npx skills CLI to install to ALL detected agents (Claude Code, Cursor, Codex, Windsurf, etc.):
```makefile
install-skill:
	npx skills add . --skill agent-filesystem -g -y
```

### 5. Add install-skill-local target to Makefile
Manual symlink for Claude Code only (no Node.js required):
```makefile
install-skill-local:
	@mkdir -p ~/.claude/skills/agent-filesystem
	@ln -sf $(PWD)/skills/agent-filesystem/SKILL.md ~/.claude/skills/agent-filesystem/SKILL.md
	@echo "Installed agent-filesystem skill to ~/.claude/skills/agent-filesystem/"
```

### 6. Test skill installation
- [ ] Test `npx skills add . --skill agent-filesystem --list`
- [ ] Test `make install-skill-local`
- [ ] Verify skill appears in agent's skill list

## Success Criteria
- [ ] `npx skills add <repo>` discovers and installs the skill
- [ ] `make install-skill` and `make install-skill-local` work
- [ ] Skill content clearly explains use cases and anti-patterns
- [ ] Skill references `afs mcp`, mounted workspaces, `afs grep`, and direct `redis-cli` as integration options
