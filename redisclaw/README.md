# RedisClaw 🦀

An OpenClaw-style task-solving coding agent with Redis-FS backed persistent sandbox.

## Features

- **Task-Solving Agent Loop**: Iterates until the task is complete (like OpenClaw/Pi)
- **Minimal Tool Set**: Bash, Read, Write, Edit, Glob, Grep, TodoWrite
- **Session Management**: Persist and resume conversations
- **Redis-FS Storage**: Persistent filesystem backed by Redis
- **Sandboxed Execution**: Code runs in isolated Docker container

## Quick Start

```bash
# Start the sandbox
cd ../sandbox && docker-compose up -d

# Install RedisClaw
pip install -e .

# Run a task (requires ANTHROPIC_API_KEY)
export ANTHROPIC_API_KEY=your_key
redisclaw --task "Write a Python script that prints hello world and run it"

# Interactive mode
redisclaw

# Direct commands (no API key needed)
redisclaw --bash "python3 --version"
redisclaw --read /some/file.txt
redisclaw --ls /workspace
```

## Architecture

```
┌────────────────────────────────────────────┐
│              RedisClaw CLI                  │
│  /task, /bash, /session, /new, /resume     │
└─────────────────────┬──────────────────────┘
                      │
┌─────────────────────┴──────────────────────┐
│            Agent Loop                       │
│  1. Receive task                           │
│  2. Call Claude with tools                 │
│  3. Execute tool calls                     │
│  4. Loop until complete                    │
│  5. Save session                           │
└─────────────────────┬──────────────────────┘
                      │
        ┌─────────────┴─────────────┐
        │                           │
        ▼                           ▼
┌───────────────┐          ┌───────────────┐
│    Sandbox    │          │   Redis-FS    │
│  (Docker)     │          │  (Storage)    │
│               │◄────────►│               │
│ /workspace    │   FUSE   │ Key: sandbox  │
│ python, node  │  mount   │               │
└───────────────┘          └───────────────┘
```

## CLI Commands

**Interactive mode commands:**

| Command | Description |
|---------|-------------|
| `/task <desc>` | Give the agent a task to solve |
| `/bash <cmd>` | Run a shell command directly |
| `/read <path>` | Read a file |
| `/write <path>` | Write a file (stdin) |
| `/ls [path]` | List files |
| `/glob <pattern>` | Find files by pattern |
| `/grep <pattern> [path]` | Search file contents |
| `/session` | Show current session info |
| `/sessions` | List all sessions |
| `/new` | Start a new session |
| `/resume <id>` | Resume a session |
| `/clear` | Clear current session |
| `/help` | Show help |
| `/exit` | Exit |

Or just type a task directly to start the agent loop.

## Tools

The agent uses a minimal, powerful tool set (like Pi agent):

| Tool | Description |
|------|-------------|
| `Bash` | Run shell commands in the sandbox |
| `Read` | Read file contents |
| `Write` | Write/create files |
| `Edit` | Make targeted search/replace edits |
| `Glob` | Find files by pattern |
| `Grep` | Search file contents |
| `TodoWrite` | Track multi-step task progress |

## Memory System (OpenClaw-style)

RedisClaw uses markdown files for persistent memory, inspired by OpenClaw:

| File | Description |
|------|-------------|
| `/memory/MEMORY.md` | Long-term curated memory (always in context) |
| `/memory/SOUL.md` | AI personality, rules, tone |
| `/memory/USER.md` | User preferences, patterns, info |
| `/memory/IDENTITY.md` | AI identity/persona |
| `/memory/AGENTS.md` | Behavioral guidelines |
| `/memory/HEARTBEAT.md` | Periodic check tasks (future) |
| `/memory/YYYY-MM-DD.md` | Daily conversation logs |

Memory files are automatically loaded into the system prompt, giving
the agent persistent context across sessions.

**CLI Commands:**

```bash
/memory          # List memory files
/memory soul     # View SOUL.md
/memory user     # View USER.md
/memory edit soul    # Edit SOUL.md (enter content, Ctrl+D to save)
/memory append memory "Important fact to remember"
```

The agent can also read/write memory files using the standard tools:

```
Agent> Read the file /memory/MEMORY.md
Agent> Write to /memory/MEMORY.md and add "User prefers TypeScript"
```

## Session Management

Sessions persist to Redis with a 7-day TTL:

```bash
# Start a new session
redisclaw

# Resume a specific session
redisclaw --session <session_id>

# Or in interactive mode
/sessions        # List all sessions
/resume abc123   # Resume by ID prefix
/new             # Start fresh
```

## Testing

```bash
# Install dev dependencies
pip install -e ".[dev]"

# Run E2E tests (sandbox must be running)
pytest tests/test_e2e.py -v
```

## Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `--sandbox` | `http://localhost:8090` | Sandbox server URL |
| `--redis` | `redis://localhost:6380` | Redis URL |
| `--key` | `sandbox` | Redis FS key |
| `--model` | `claude-sonnet-4-20250514` | Claude model |
| `--session` | (none) | Resume a session by ID |
| `--task` | (none) | Run a task and exit |
| `--bash` | (none) | Run a command and exit |

