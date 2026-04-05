"""
RedisClaw Tools - OpenClaw-style minimal tools for the agent.

Like Pi agent, we use a minimal set of powerful tools:
- Bash: Run shell commands in the sandbox
- Read: Read file contents
- Write: Write/create files
- Edit: Make targeted edits to files
- Glob: Find files by pattern
- Grep: Search file contents
- TodoWrite: Track tasks and progress
"""

from typing import Any

import httpx
import redis


# OpenClaw-style minimal tool definitions
TOOLS = [
    {
        "name": "Bash",
        "description": """Run a shell command in the sandbox environment.
- Working directory is /workspace
- Has access to python3, node, npm, git, and common CLI tools
- Commands run with a 300s timeout by default
- Use for: running code, installing packages, git operations, builds, tests""",
        "input_schema": {
            "type": "object",
            "properties": {
                "command": {
                    "type": "string",
                    "description": "The shell command to execute"
                },
                "timeout": {
                    "type": "integer",
                    "description": "Timeout in seconds (default: 300)"
                }
            },
            "required": ["command"]
        }
    },
    {
        "name": "Read",
        "description": """Read the contents of a file.
- Paths are relative to /workspace
- Returns the full file content
- Returns error if file doesn't exist""",
        "input_schema": {
            "type": "object",
            "properties": {
                "path": {
                    "type": "string",
                    "description": "Path to the file (relative to /workspace)"
                }
            },
            "required": ["path"]
        }
    },
    {
        "name": "Write",
        "description": """Write content to a file. Creates the file if it doesn't exist.
- Creates parent directories automatically
- Overwrites existing content
- Paths are relative to /workspace""",
        "input_schema": {
            "type": "object",
            "properties": {
                "path": {
                    "type": "string",
                    "description": "Path to the file (relative to /workspace)"
                },
                "content": {
                    "type": "string",
                    "description": "Content to write to the file"
                }
            },
            "required": ["path", "content"]
        }
    },
    {
        "name": "Edit",
        "description": """Make targeted edits to a file using search and replace.
- Finds and replaces exact text matches
- Use for small, precise changes
- Fails if old_str not found exactly once""",
        "input_schema": {
            "type": "object",
            "properties": {
                "path": {
                    "type": "string",
                    "description": "Path to the file"
                },
                "old_str": {
                    "type": "string",
                    "description": "The exact text to find and replace"
                },
                "new_str": {
                    "type": "string",
                    "description": "The text to replace it with"
                }
            },
            "required": ["path", "old_str", "new_str"]
        }
    },
    {
        "name": "Glob",
        "description": """Find files matching a glob pattern.
- Searches from /workspace
- Supports *, **, ?
- Returns list of matching paths""",
        "input_schema": {
            "type": "object",
            "properties": {
                "pattern": {
                    "type": "string",
                    "description": "Glob pattern (e.g., '**/*.py', 'src/*.js')"
                }
            },
            "required": ["pattern"]
        }
    },
    {
        "name": "Grep",
        "description": """Search for text/patterns in files.
- Searches file contents
- Supports regex patterns
- Returns matching lines with context""",
        "input_schema": {
            "type": "object",
            "properties": {
                "pattern": {
                    "type": "string",
                    "description": "Search pattern (regex supported)"
                },
                "path": {
                    "type": "string",
                    "description": "File or directory to search (default: /workspace)"
                },
                "include": {
                    "type": "string",
                    "description": "File pattern to include (e.g., '*.py')"
                }
            },
            "required": ["pattern"]
        }
    },
    {
        "name": "TodoWrite",
        "description": """Write/update the task list to track progress.
- Use to plan multi-step tasks
- Update as you complete steps
- Helps maintain context across turns""",
        "input_schema": {
            "type": "object",
            "properties": {
                "tasks": {
                    "type": "array",
                    "items": {
                        "type": "object",
                        "properties": {
                            "task": {"type": "string", "description": "Task description"},
                            "status": {
                                "type": "string",
                                "enum": ["pending", "in_progress", "done", "blocked"],
                                "description": "Task status"
                            }
                        },
                        "required": ["task", "status"]
                    },
                    "description": "List of tasks with their status"
                }
            },
            "required": ["tasks"]
        }
    },
]


class ToolExecutor:
    """
    Executes tools against the sandbox and Redis-FS.

    The sandbox provides isolated command execution.
    Redis-FS provides persistent file storage.
    Files written to Redis-FS are immediately visible in the sandbox at /workspace.
    """

    def __init__(self, sandbox_url: str, redis_url: str, fs_key: str):
        self.sandbox_url = sandbox_url.rstrip("/")
        self.redis = redis.from_url(redis_url)
        self.fs_key = fs_key
        self.http = httpx.Client(timeout=300)
        self.todos: list[dict] = []  # In-memory todo list

    def execute(self, name: str, args: dict[str, Any]) -> str:
        """Execute a tool and return the result."""
        try:
            handler = {
                "Bash": self._bash,
                "Read": self._read,
                "Write": self._write,
                "Edit": self._edit,
                "Glob": self._glob,
                "Grep": self._grep,
                "TodoWrite": self._todo_write,
                # Legacy tool names for backwards compatibility
                "run_command": self._bash,
                "read_file": self._read,
                "write_file": self._write,
                "list_files": self._list_files,
                "delete_file": self._delete_file,
            }.get(name)

            if not handler:
                return f"Unknown tool: {name}"
            return handler(args)
        except Exception as e:
            return f"Error: {e}"

    def _normalize_path(self, path: str) -> str:
        """Normalize path to absolute form for Redis-FS."""
        if not path.startswith("/"):
            path = "/" + path
        return path

    def _bash(self, args: dict) -> str:
        """Run a command in the sandbox."""
        command = args["command"]
        timeout = args.get("timeout", 300)

        resp = self.http.post(
            f"{self.sandbox_url}/processes",
            json={
                "command": command,
                "cwd": "",
                "timeout_secs": timeout,
                "wait": True,
            }
        )
        result = resp.json()

        output_parts = []
        if result.get("stdout"):
            output_parts.append(result["stdout"])
        if result.get("stderr"):
            output_parts.append(f"[stderr]\n{result['stderr']}")
        if result.get("exit_code", 0) != 0:
            output_parts.append(f"[exit code: {result['exit_code']}]")

        return "\n".join(output_parts) if output_parts else "(no output)"

    def _read(self, args: dict) -> str:
        """Read a file via Redis-FS."""
        path = self._normalize_path(args["path"])
        result = self.redis.execute_command("FS.CAT", self.fs_key, path)
        if result is None:
            return f"Error: File not found: {path}"
        return result.decode() if isinstance(result, bytes) else str(result)

    def _write(self, args: dict) -> str:
        """Write a file via Redis-FS."""
        path = self._normalize_path(args["path"])
        content = args["content"]
        self.redis.execute_command("FS.ECHO", self.fs_key, path, content)
        return f"Wrote {len(content)} bytes to {path}"

    def _edit(self, args: dict) -> str:
        """Edit a file by search and replace."""
        path = self._normalize_path(args["path"])
        old_str = args["old_str"]
        new_str = args["new_str"]

        # Read current content
        content = self.redis.execute_command("FS.CAT", self.fs_key, path)
        if content is None:
            return f"Error: File not found: {path}"
        content = content.decode() if isinstance(content, bytes) else str(content)

        # Count occurrences
        count = content.count(old_str)
        if count == 0:
            return f"Error: Could not find text to replace in {path}"
        if count > 1:
            return f"Error: Found {count} occurrences of the text. Edit requires exactly 1 match."

        # Replace and write back
        new_content = content.replace(old_str, new_str, 1)
        self.redis.execute_command("FS.ECHO", self.fs_key, path, new_content)
        return f"Edited {path}: replaced {len(old_str)} chars with {len(new_str)} chars"

    def _glob(self, args: dict) -> str:
        """Find files matching a glob pattern."""
        pattern = args["pattern"]

        # List all files recursively via FS.FIND or walk the tree
        # For now, use sandbox's find command for full glob support
        resp = self.http.post(
            f"{self.sandbox_url}/processes",
            json={
                "command": f"find /workspace -type f -name '{pattern.split('/')[-1]}' 2>/dev/null | head -100",
                "cwd": "",
                "timeout_secs": 30,
                "wait": True,
            }
        )
        result = resp.json()
        stdout = result.get("stdout", "")

        if not stdout.strip():
            return "No files found matching pattern"

        # Convert to relative paths
        lines = [
            line.replace("/workspace/", "")
            for line in stdout.strip().split("\n")
            if line.strip()
        ]
        return "\n".join(lines)

    def _grep(self, args: dict) -> str:
        """Search for pattern in files using Redis-FS GREP."""
        pattern = args["pattern"]
        path = args.get("path", "/")
        include = args.get("include", "*")

        # Use FS.GREP if available, otherwise fall back to sandbox grep
        try:
            result = self.redis.execute_command("FS.GREP", self.fs_key, path, pattern)
            if result:
                matches = []
                for item in result:
                    if isinstance(item, bytes):
                        matches.append(item.decode())
                    elif isinstance(item, list):
                        # Format: [filename, [matches...]]
                        filename = item[0].decode() if isinstance(item[0], bytes) else item[0]
                        for match in item[1:]:
                            m = match.decode() if isinstance(match, bytes) else match
                            matches.append(f"{filename}: {m}")
                return "\n".join(matches) if matches else "No matches found"
        except Exception:
            pass

        # Fallback to sandbox grep
        resp = self.http.post(
            f"{self.sandbox_url}/processes",
            json={
                "command": f"grep -rn '{pattern}' /workspace --include='{include}' 2>/dev/null | head -50",
                "cwd": "",
                "timeout_secs": 30,
                "wait": True,
            }
        )
        result = resp.json()
        stdout = result.get("stdout", "")
        return stdout.strip() if stdout.strip() else "No matches found"

    def _todo_write(self, args: dict) -> str:
        """Write/update the todo list."""
        self.todos = args["tasks"]

        # Format the todo list
        lines = ["## Task List"]
        for i, task in enumerate(self.todos, 1):
            status_icon = {
                "pending": "[ ]",
                "in_progress": "[/]",
                "done": "[x]",
                "blocked": "[-]",
            }.get(task["status"], "[ ]")
            lines.append(f"{i}. {status_icon} {task['task']}")

        return "\n".join(lines)

    # Legacy tool implementations for backwards compatibility
    def _list_files(self, args: dict) -> str:
        """List files via Redis-FS."""
        path = self._normalize_path(args.get("path", "/"))
        result = self.redis.execute_command("FS.LS", self.fs_key, path)
        if not result:
            return "(empty directory)"
        files = [f.decode() if isinstance(f, bytes) else str(f) for f in result]
        return "\n".join(files)

    def _delete_file(self, args: dict) -> str:
        """Delete a file via Redis-FS."""
        path = self._normalize_path(args["path"])
        self.redis.execute_command("FS.RM", self.fs_key, path)
        return f"Deleted {path}"

    def close(self):
        """Clean up resources."""
        self.http.close()
        self.redis.close()

