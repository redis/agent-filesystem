"""
RedisClaw Agent - OpenClaw-style task-solving agent loop.

The agent loop is the core runtime that:
1. Receives a task/message
2. Plans and executes using tools
3. Iterates until the task is complete
4. Maintains session state for context

Like OpenClaw/Pi agent, we use a minimal system prompt and
let the model figure out the best approach.
"""

import json
import time
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Callable, Generator

import anthropic
import redis

from .memory import MemoryManager
from .tools import TOOLS, ToolExecutor


def utc_now_iso() -> str:
    """Get current UTC time as ISO format string."""
    return datetime.now(timezone.utc).isoformat()


# Base system prompt - memory context is prepended dynamically
BASE_SYSTEM_PROMPT = """You are RedisClaw, a coding agent with persistent memory.

Environment:
- Workspace: /workspace (persistent, Redis-backed filesystem)
- Memory: /memory (persistent markdown files for context)
- Tools: Bash, Read, Write, Edit, Glob, Grep, TodoWrite
- Available: python3, node, npm, git, common CLI tools

Approach:
1. Understand the task
2. Break it down if complex (use TodoWrite to track)
3. Execute step by step
4. Verify results
5. Iterate until complete

Memory System:
- /memory/MEMORY.md contains important facts to remember
- You can read/write memory files to persist learnings
- Use Write tool to update memory when you learn important things

Be concise. Show your work. Fix errors when they occur."""


@dataclass
class Message:
    """A conversation message."""
    role: str
    content: Any  # Can be str or list of content blocks
    timestamp: str = field(default_factory=utc_now_iso)


@dataclass
class Session:
    """Session state for conversation persistence."""
    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    messages: list[Message] = field(default_factory=list)
    created_at: str = field(default_factory=utc_now_iso)
    updated_at: str = field(default_factory=utc_now_iso)
    metadata: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        """Serialize session to dict."""
        return {
            "id": self.id,
            "messages": [
                {"role": m.role, "content": m.content, "timestamp": m.timestamp}
                for m in self.messages
            ],
            "created_at": self.created_at,
            "updated_at": self.updated_at,
            "metadata": self.metadata,
        }

    @classmethod
    def from_dict(cls, data: dict) -> "Session":
        """Deserialize session from dict."""
        return cls(
            id=data["id"],
            messages=[
                Message(role=m["role"], content=m["content"], timestamp=m.get("timestamp", ""))
                for m in data["messages"]
            ],
            created_at=data["created_at"],
            updated_at=data["updated_at"],
            metadata=data.get("metadata", {}),
        )


class SessionManager:
    """Manages session persistence in Redis."""

    def __init__(self, redis_client: redis.Redis, prefix: str = "redisclaw:session:"):
        self.redis = redis_client
        self.prefix = prefix

    def save(self, session: Session) -> None:
        """Save session to Redis."""
        session.updated_at = utc_now_iso()
        key = f"{self.prefix}{session.id}"
        self.redis.set(key, json.dumps(session.to_dict()))
        # Set 7-day TTL for sessions
        self.redis.expire(key, 7 * 24 * 3600)

    def load(self, session_id: str) -> Session | None:
        """Load session from Redis."""
        key = f"{self.prefix}{session_id}"
        data = self.redis.get(key)
        if data:
            return Session.from_dict(json.loads(data))
        return None

    def delete(self, session_id: str) -> None:
        """Delete a session."""
        self.redis.delete(f"{self.prefix}{session_id}")

    def list_sessions(self) -> list[str]:
        """List all session IDs."""
        keys = self.redis.keys(f"{self.prefix}*")
        return [k.decode().replace(self.prefix, "") for k in keys]


@dataclass
class AgentConfig:
    """Agent configuration."""
    sandbox_url: str = "http://localhost:8090"
    redis_url: str = "redis://localhost:6380"
    fs_key: str = "sandbox"
    model: str = "claude-sonnet-4-20250514"
    max_tokens: int = 4096
    max_iterations: int = 50  # Prevent infinite loops
    timeout: int = 600  # 10 minute timeout like OpenClaw


@dataclass
class AgentEvent:
    """Event emitted during agent execution."""
    type: str  # "lifecycle", "assistant", "tool"
    phase: str  # "start", "delta", "end", "error"
    data: Any = None
    timestamp: str = field(default_factory=utc_now_iso)


class Agent:
    """
    RedisClaw agent with OpenClaw-style agent loop.

    The agent loop:
    1. Takes user input
    2. Calls the model
    3. If model returns tool calls, executes them
    4. Repeats until model gives a final response
    5. Persists session state
    """

    def __init__(
        self,
        config: AgentConfig | None = None,
        session_id: str | None = None,
    ):
        self.config = config or AgentConfig()
        self.client = anthropic.Anthropic()
        self.tools = ToolExecutor(
            self.config.sandbox_url,
            self.config.redis_url,
            self.config.fs_key,
        )

        # Session management
        self._redis = redis.from_url(self.config.redis_url)
        self.session_manager = SessionManager(self._redis)

        # Memory management (OpenClaw-style markdown files)
        self.memory = MemoryManager(
            redis_client=self._redis,
            redis_key=self.config.fs_key,
        )
        # Initialize default memory files if they don't exist
        try:
            self.memory.initialize_defaults()
        except Exception:
            pass  # Don't fail if memory initialization fails

        # Load or create session
        if session_id:
            self.session = self.session_manager.load(session_id) or Session(id=session_id)
        else:
            self.session = Session()

    def _build_system_prompt(self) -> str:
        """Build the full system prompt including memory context."""
        memory_context = ""
        try:
            memory_context = self.memory.get_context_prompt()
        except Exception:
            pass  # Don't fail if memory read fails

        if memory_context:
            return f"{memory_context}\n\n{BASE_SYSTEM_PROMPT}"
        return BASE_SYSTEM_PROMPT

    def run(
        self,
        task: str,
        on_event: Callable[[AgentEvent], None] | None = None,
    ) -> Generator[AgentEvent, None, str]:
        """
        Run the agent loop to solve a task.

        This is the main entry point. It:
        1. Adds the task as a user message
        2. Iterates through model calls and tool execution
        3. Yields events during execution
        4. Returns the final response

        Args:
            task: The task/message from the user
            on_event: Optional callback for events

        Yields:
            AgentEvent objects for streaming UI updates

        Returns:
            The final assistant response
        """
        # Emit lifecycle start
        start_event = AgentEvent(type="lifecycle", phase="start", data={"task": task})
        yield start_event
        if on_event:
            on_event(start_event)

        # Add user message
        self.session.messages.append(Message(role="user", content=task))

        start_time = time.time()
        iterations = 0
        final_response = ""

        try:
            while iterations < self.config.max_iterations:
                # Check timeout
                if time.time() - start_time > self.config.timeout:
                    error_event = AgentEvent(
                        type="lifecycle", phase="error",
                        data={"error": "Agent timeout exceeded"}
                    )
                    yield error_event
                    if on_event:
                        on_event(error_event)
                    break

                iterations += 1

                # Build API messages
                api_messages = self._build_api_messages()

                # Call model
                response = self.client.messages.create(
                    model=self.config.model,
                    max_tokens=self.config.max_tokens,
                    system=self._build_system_prompt(),
                    tools=TOOLS,
                    messages=api_messages,
                )

                # Process response
                assistant_content = []
                tool_uses = []
                text_parts = []

                for block in response.content:
                    if block.type == "text":
                        text_parts.append(block.text)
                        assistant_content.append({"type": "text", "text": block.text})

                        # Emit assistant delta
                        delta_event = AgentEvent(
                            type="assistant", phase="delta",
                            data={"text": block.text}
                        )
                        yield delta_event
                        if on_event:
                            on_event(delta_event)

                    elif block.type == "tool_use":
                        tool_uses.append(block)
                        assistant_content.append({
                            "type": "tool_use",
                            "id": block.id,
                            "name": block.name,
                            "input": block.input,
                        })

                # Store assistant message
                self.session.messages.append(Message(
                    role="assistant",
                    content=assistant_content
                ))

                # If no tool use, we're done
                if not tool_uses:
                    final_response = "\n".join(text_parts)
                    break

                # Execute tools
                tool_results = []
                for tool_use in tool_uses:
                    # Emit tool start
                    tool_start = AgentEvent(
                        type="tool", phase="start",
                        data={"name": tool_use.name, "input": tool_use.input}
                    )
                    yield tool_start
                    if on_event:
                        on_event(tool_start)

                    # Execute
                    result = self.tools.execute(tool_use.name, tool_use.input)

                    # Truncate very long outputs
                    if len(result) > 10000:
                        result = result[:10000] + "\n... (truncated)"

                    tool_results.append({
                        "type": "tool_result",
                        "tool_use_id": tool_use.id,
                        "content": result,
                    })

                    # Emit tool end
                    tool_end = AgentEvent(
                        type="tool", phase="end",
                        data={"name": tool_use.name, "result": result[:1000]}
                    )
                    yield tool_end
                    if on_event:
                        on_event(tool_end)

                # Add tool results as user message
                self.session.messages.append(Message(
                    role="user",
                    content=tool_results
                ))

                # Check if model wanted to end after tools
                if response.stop_reason == "end_turn":
                    final_response = "\n".join(text_parts) if text_parts else ""
                    break

            # Save session
            self.session_manager.save(self.session)

            # Emit lifecycle end
            end_event = AgentEvent(
                type="lifecycle", phase="end",
                data={
                    "response": final_response,
                    "iterations": iterations,
                    "duration": time.time() - start_time,
                }
            )
            yield end_event
            if on_event:
                on_event(end_event)

            return final_response

        except Exception as e:
            error_event = AgentEvent(
                type="lifecycle", phase="error",
                data={"error": str(e)}
            )
            yield error_event
            if on_event:
                on_event(error_event)
            raise

    def chat(self, user_message: str) -> Generator[str, None, None]:
        """
        Simple chat interface that yields text updates.

        This is a convenience wrapper around run() for simpler use cases.
        """
        for event in self.run(user_message):
            if event.type == "assistant" and event.phase == "delta":
                yield event.data.get("text", "")
            elif event.type == "tool" and event.phase == "start":
                yield f"\n🔧 {event.data['name']}: {json.dumps(event.data['input'])}\n"
            elif event.type == "tool" and event.phase == "end":
                result = event.data.get("result", "")
                yield f"📤 {result[:500]}{'...' if len(result) > 500 else ''}\n"
            elif event.type == "lifecycle" and event.phase == "error":
                yield f"\n❌ Error: {event.data.get('error', 'Unknown error')}\n"

    def _build_api_messages(self) -> list[dict]:
        """Build messages list for API call."""
        api_messages = []

        for msg in self.session.messages:
            if isinstance(msg.content, str):
                api_messages.append({"role": msg.role, "content": msg.content})
            else:
                # Already structured content (tool use/results)
                api_messages.append({"role": msg.role, "content": msg.content})

        return api_messages

    def reset(self) -> None:
        """Clear conversation and start new session."""
        self.session = Session()

    def get_session_id(self) -> str:
        """Get the current session ID."""
        return self.session.id

    def close(self) -> None:
        """Clean up resources."""
        self.tools.close()
        self._redis.close()

