"""
Memory Manager for RedisClaw - OpenClaw-style markdown memory system.

Memory files stored in Redis-FS:
- /memory/MEMORY.md     - Long-term curated memory (always in context)
- /memory/SOUL.md       - AI personality, rules, tone
- /memory/USER.md       - User preferences, patterns, info
- /memory/IDENTITY.md   - AI identity/persona
- /memory/AGENTS.md     - Behavioral guidelines
- /memory/HEARTBEAT.md  - Periodic check tasks
- /memory/YYYY-MM-DD.md - Daily conversation logs
"""

import json
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any

import redis

# Memory file paths
MEMORY_DIR = "/memory"
MEMORY_FILES = {
    "memory": f"{MEMORY_DIR}/MEMORY.md",
    "soul": f"{MEMORY_DIR}/SOUL.md",
    "user": f"{MEMORY_DIR}/USER.md",
    "identity": f"{MEMORY_DIR}/IDENTITY.md",
    "agents": f"{MEMORY_DIR}/AGENTS.md",
    "heartbeat": f"{MEMORY_DIR}/HEARTBEAT.md",
}

# Default content for memory files
DEFAULT_MEMORY = """# Memory

This file contains important long-term memories and context.
The agent will always have access to this file.

## Key Facts

<!-- Add important facts about the user or project here -->

## Learned Preferences

<!-- Patterns and preferences the agent has learned -->

## Important Context

<!-- Critical context that should persist across sessions -->
"""

DEFAULT_SOUL = """# Soul

This file defines the AI assistant's personality and behavior.

## Personality

- Helpful and proactive
- Concise but thorough
- Asks clarifying questions when needed

## Rules

- Always explain what you're doing before taking action
- If a command fails, analyze the error and suggest fixes
- Keep track of progress using the TodoWrite tool

## Tone

- Professional but friendly
- Direct and clear
- Patient with complex problems
"""

DEFAULT_USER = """# User

Information about the user.

## Preferences

<!-- Work style, communication preferences -->

## Timezone

<!-- User's timezone for scheduling -->

## Context

<!-- Current projects, goals, etc. -->
"""

DEFAULT_IDENTITY = """# Identity

I am RedisClaw, a coding assistant with persistent memory.

## Role

I help with coding tasks, file management, and shell operations
in a sandboxed environment backed by Redis-FS.

## Capabilities

- Execute shell commands (Bash)
- Read, write, and edit files
- Search files with Glob and Grep
- Track tasks with TodoWrite
- Remember context across sessions
"""

DEFAULT_AGENTS = """# Agent Guidelines

## Task Execution

1. Understand the task before acting
2. Break complex tasks into steps
3. Use TodoWrite to track progress
4. Verify results after each step

## Safety

- Never delete files without confirmation
- Be cautious with destructive commands
- Always show command output to user

## Communication

- Explain actions before taking them
- Report errors clearly
- Suggest next steps when done
"""

DEFAULT_HEARTBEAT = """# Heartbeat

Periodic checks to run (not yet implemented in RedisClaw).

## Checks

- [ ] Review pending tasks
- [ ] Check for file changes
- [ ] Summarize recent activity
"""

DEFAULTS = {
    "memory": DEFAULT_MEMORY,
    "soul": DEFAULT_SOUL,
    "user": DEFAULT_USER,
    "identity": DEFAULT_IDENTITY,
    "agents": DEFAULT_AGENTS,
    "heartbeat": DEFAULT_HEARTBEAT,
}


@dataclass
class MemoryManager:
    """Manages markdown memory files stored in Redis-FS."""

    redis_client: redis.Redis
    redis_key: str = "sandbox"
    _cache: dict = field(default_factory=dict)

    def _fs_cmd(self, *args) -> Any:
        """Execute a Redis-FS command."""
        return self.redis_client.execute_command(*args)

    def _ensure_memory_dir(self) -> None:
        """Create /memory directory if it doesn't exist."""
        try:
            self._fs_cmd("FS.MKDIR", self.redis_key, MEMORY_DIR)
        except redis.ResponseError:
            pass  # Directory already exists

    def _file_exists(self, path: str) -> bool:
        """Check if a file exists in Redis-FS."""
        try:
            self._fs_cmd("FS.STAT", self.redis_key, path)
            return True
        except redis.ResponseError:
            return False

    def read_file(self, path: str) -> str | None:
        """Read a file from Redis-FS."""
        try:
            content = self._fs_cmd("FS.CAT", self.redis_key, path)
            return content.decode() if isinstance(content, bytes) else content
        except redis.ResponseError:
            return None

    def write_file(self, path: str, content: str) -> bool:
        """Write content to a file in Redis-FS."""
        try:
            self._ensure_memory_dir()
            self._fs_cmd("FS.ECHO", self.redis_key, path, content)
            return True
        except redis.ResponseError as e:
            print(f"Error writing {path}: {e}")
            return False

    def get_memory(self, name: str) -> str:
        """Get a memory file by name, creating with defaults if needed."""
        if name not in MEMORY_FILES:
            raise ValueError(f"Unknown memory file: {name}")

        path = MEMORY_FILES[name]
        content = self.read_file(path)

        if content is None:
            # Create with default content
            default = DEFAULTS.get(name, "")
            self.write_file(path, default)
            return default

        return content

    def set_memory(self, name: str, content: str) -> bool:
        """Set a memory file by name."""
        if name not in MEMORY_FILES:
            raise ValueError(f"Unknown memory file: {name}")

        path = MEMORY_FILES[name]
        return self.write_file(path, content)

    def append_memory(self, name: str, content: str) -> bool:
        """Append content to a memory file."""
        current = self.get_memory(name)
        new_content = current.rstrip() + "\n\n" + content
        return self.set_memory(name, new_content)

    def get_daily_log(self, date: datetime | None = None) -> str:
        """Get or create a daily conversation log."""
        if date is None:
            date = datetime.now(timezone.utc)

        filename = date.strftime("%Y-%m-%d") + ".md"
        path = f"{MEMORY_DIR}/{filename}"

        content = self.read_file(path)
        if content is None:
            header = f"# Daily Log - {date.strftime('%Y-%m-%d')}\n\n"
            self.write_file(path, header)
            return header

        return content

    def append_daily_log(self, entry: str, date: datetime | None = None) -> bool:
        """Append an entry to today's daily log."""
        if date is None:
            date = datetime.now(timezone.utc)

        filename = date.strftime("%Y-%m-%d") + ".md"
        path = f"{MEMORY_DIR}/{filename}"

        current = self.get_daily_log(date)
        timestamp = datetime.now(timezone.utc).strftime("%H:%M:%S UTC")
        new_entry = f"\n## {timestamp}\n\n{entry}\n"

        return self.write_file(path, current + new_entry)

    def get_context_prompt(self) -> str:
        """Build system prompt context from memory files."""
        sections = []

        # Always include MEMORY.md (core memories)
        memory = self.get_memory("memory")
        if memory and memory.strip():
            sections.append(f"<memory>\n{memory}\n</memory>")

        # Include SOUL.md (personality)
        soul = self.get_memory("soul")
        if soul and soul.strip():
            sections.append(f"<soul>\n{soul}\n</soul>")

        # Include USER.md (user info)
        user = self.get_memory("user")
        if user and user.strip():
            sections.append(f"<user_profile>\n{user}\n</user_profile>")

        # Include IDENTITY.md (AI identity)
        identity = self.get_memory("identity")
        if identity and identity.strip():
            sections.append(f"<identity>\n{identity}\n</identity>")

        if not sections:
            return ""

        return "\n\n".join(sections)

    def list_memory_files(self) -> list[str]:
        """List all memory files."""
        try:
            result = self._fs_cmd("FS.LS", self.redis_key, MEMORY_DIR, "-a")
            if isinstance(result, list):
                return [r.decode() if isinstance(r, bytes) else r for r in result]
            return []
        except redis.ResponseError:
            return []

    def initialize_defaults(self) -> None:
        """Initialize all default memory files if they don't exist."""
        self._ensure_memory_dir()
        for name in MEMORY_FILES:
            self.get_memory(name)  # Creates with defaults if missing

