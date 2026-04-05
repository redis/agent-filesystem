"""
RedisClaw CLI - OpenClaw-style interactive console interface.

Supports:
- Interactive task-solving mode (agent loop)
- Session management (resume, list, clear)
- Direct tool commands (/bash, /read, /write, etc.)
"""

import argparse
import os
import sys

from rich.console import Console
from rich.markdown import Markdown
from rich.panel import Panel
from rich.table import Table
from prompt_toolkit import PromptSession
from prompt_toolkit.history import FileHistory

from .agent import Agent, AgentConfig, AgentEvent
from .memory import MemoryManager, MEMORY_FILES
from .tools import ToolExecutor


console = Console()


def print_welcome(session_id: str | None = None):
    """Print welcome message."""
    session_info = f"\n[dim]Session: {session_id[:8]}...[/dim]" if session_id else ""
    console.print(Panel.fit(
        "[bold cyan]🦀 RedisClaw[/bold cyan]\n"
        "An OpenClaw-style task-solving coding agent\n"
        "with Redis-FS backed persistent sandbox" + session_info + "\n\n"
        "[dim]Commands:[/dim]\n"
        "  /task <desc>   - Give the agent a task to solve\n"
        "  /bash <cmd>    - Run a shell command directly\n"
        "  /read <path>   - Read a file\n"
        "  /write <path>  - Write a file (stdin)\n"
        "  /ls [path]     - List files\n\n"
        "[dim]Memory (OpenClaw-style):[/dim]\n"
        "  /memory        - List memory files\n"
        "  /memory <name> - View a memory file (memory, soul, user, identity)\n"
        "  /memory edit <name> - Edit a memory file (stdin)\n\n"
        "[dim]Sessions:[/dim]\n"
        "  /session     - Show session info\n"
        "  /sessions    - List all sessions\n"
        "  /new         - Start new session\n"
        "  /resume <id> - Resume a session\n"
        "  /clear       - Clear current session\n"
        "  /help        - Show this help\n"
        "  /exit        - Exit\n\n"
        "[dim]Or just type a task directly to start the agent loop.[/dim]",
        title="Welcome",
        border_style="cyan",
    ))


def format_event(event: AgentEvent) -> str | None:
    """Format an agent event for display."""
    if event.type == "lifecycle":
        if event.phase == "start":
            return "[cyan]▶ Starting agent loop...[/cyan]"
        elif event.phase == "end":
            data = event.data or {}
            return f"[green]✓ Complete ({data.get('iterations', '?')} iterations, {data.get('duration', 0):.1f}s)[/green]"
        elif event.phase == "error":
            return f"[red]✗ Error: {event.data.get('error', 'Unknown')}[/red]"

    elif event.type == "tool":
        if event.phase == "start":
            import json
            name = event.data.get("name", "?")
            inp = event.data.get("input", {})
            # Compact display for common tools
            if name == "Bash":
                return f"[yellow]🔧 Bash:[/yellow] {inp.get('command', '')[:80]}"
            elif name in ("Read", "Write", "Edit"):
                return f"[yellow]🔧 {name}:[/yellow] {inp.get('path', '')}"
            else:
                return f"[yellow]🔧 {name}:[/yellow] {json.dumps(inp)[:60]}"
        elif event.phase == "end":
            result = event.data.get("result", "")
            if len(result) > 200:
                result = result[:200] + "..."
            return f"[dim]   └─ {result}[/dim]"

    return None


def run_interactive(config: AgentConfig, session_id: str | None = None):
    """Run interactive CLI mode."""
    # Check for API key
    api_key = os.environ.get("ANTHROPIC_API_KEY")

    if api_key:
        agent = Agent(config, session_id=session_id)
        tools = agent.tools
        print_welcome(agent.get_session_id())
    else:
        console.print("[yellow]⚠️  ANTHROPIC_API_KEY not set - agent mode disabled[/yellow]")
        console.print("[dim]You can still use /bash, /read, /ls commands directly[/dim]\n")
        agent = None
        tools = ToolExecutor(config.sandbox_url, config.redis_url, config.fs_key)
        print_welcome()

    # Setup prompt
    history_file = os.path.expanduser("~/.redisclaw_history")
    prompt_session = PromptSession(history=FileHistory(history_file))

    try:
        while True:
            try:
                prompt_text = f"[{agent.get_session_id()[:8]}] > " if agent else "redisclaw> "
                user_input = prompt_session.prompt(prompt_text).strip()
            except KeyboardInterrupt:
                console.print("\n[dim]Interrupted. Type /exit to quit.[/dim]")
                continue
            except EOFError:
                break

            if not user_input:
                continue

            # Handle commands
            if user_input.startswith("/"):
                result = handle_command(user_input, tools, agent, config)
                if result == "exit":
                    break
                elif result == "new_agent":
                    # Recreate agent with new session
                    if agent:
                        agent.close()
                    agent = Agent(config)
                    tools = agent.tools
                    console.print(f"[green]New session: {agent.get_session_id()[:8]}...[/green]")
                elif isinstance(result, str) and result.startswith("resume:"):
                    # Resume a session
                    if agent:
                        agent.close()
                    resume_id = result.split(":", 1)[1]
                    agent = Agent(config, session_id=resume_id)
                    tools = agent.tools
                    console.print(f"[green]Resumed session: {agent.get_session_id()[:8]}...[/green]")
                continue

            # Agent task-solving mode
            if agent is None:
                console.print("[yellow]Agent not available (no API key)[/yellow]")
                console.print("[dim]Use /bash, /read, /ls or set ANTHROPIC_API_KEY[/dim]")
                continue

            console.print()

            # Run the agent loop
            try:
                for event in agent.run(user_input):
                    formatted = format_event(event)
                    if formatted:
                        console.print(formatted)

                    # Also print assistant text as markdown
                    if event.type == "assistant" and event.phase == "delta":
                        text = event.data.get("text", "")
                        if text.strip():
                            console.print(Markdown(text))
            except Exception as e:
                console.print(f"[red]Agent error: {e}[/red]")

            console.print()

    finally:
        if agent:
            agent.close()
        else:
            tools.close()


def handle_command(cmd: str, tools: ToolExecutor, agent: Agent | None, config: AgentConfig) -> str | None:
    """Handle a slash command. Returns action string or None."""
    parts = cmd.split(maxsplit=1)
    command = parts[0].lower()
    arg = parts[1] if len(parts) > 1 else ""

    if command in ("/exit", "/quit", "/q"):
        console.print("[cyan]Goodbye![/cyan]")
        return "exit"

    elif command == "/help":
        print_welcome(agent.get_session_id() if agent else None)

    elif command == "/new":
        return "new_agent"

    elif command == "/clear":
        if agent:
            agent.reset()
        console.print("[green]Session cleared[/green]")

    elif command == "/session":
        if agent:
            s = agent.session
            console.print(f"[cyan]Session ID:[/cyan] {s.id}")
            console.print(f"[cyan]Messages:[/cyan] {len(s.messages)}")
            console.print(f"[cyan]Created:[/cyan] {s.created_at}")
            console.print(f"[cyan]Updated:[/cyan] {s.updated_at}")
        else:
            console.print("[dim]No active session (agent not available)[/dim]")

    elif command == "/sessions":
        if agent:
            sessions = agent.session_manager.list_sessions()
            if sessions:
                table = Table(title="Sessions")
                table.add_column("ID", style="cyan")
                table.add_column("Status", style="green")
                for sid in sessions[:10]:  # Limit to 10
                    status = "active" if sid == agent.get_session_id() else ""
                    table.add_row(sid[:16] + "...", status)
                console.print(table)
            else:
                console.print("[dim]No sessions found[/dim]")
        else:
            console.print("[dim]Agent not available[/dim]")

    elif command == "/resume":
        if not arg:
            console.print("[red]Usage: /resume <session_id>[/red]")
        else:
            return f"resume:{arg}"

    elif command == "/task":
        if not arg:
            console.print("[red]Usage: /task <description>[/red]")
        elif agent:
            # Same as direct input
            console.print()
            for event in agent.run(arg):
                formatted = format_event(event)
                if formatted:
                    console.print(formatted)
                if event.type == "assistant" and event.phase == "delta":
                    text = event.data.get("text", "")
                    if text.strip():
                        console.print(Markdown(text))
            console.print()
        else:
            console.print("[yellow]Agent not available (no API key)[/yellow]")

    elif command in ("/bash", "/run"):
        if not arg:
            console.print("[red]Usage: /bash <command>[/red]")
        else:
            result = tools.execute("Bash", {"command": arg})
            console.print(result)

    elif command == "/read":
        if not arg:
            console.print("[red]Usage: /read <path>[/red]")
        else:
            result = tools.execute("Read", {"path": arg})
            console.print(result)

    elif command == "/write":
        if not arg:
            console.print("[red]Usage: /write <path>[/red]")
        else:
            console.print("Enter content (Ctrl+D when done):")
            content = sys.stdin.read()
            result = tools.execute("Write", {"path": arg, "content": content})
            console.print(result)

    elif command == "/ls":
        # Use Bash to list files (more reliable)
        path = arg or "/workspace"
        result = tools.execute("Bash", {"command": f"ls -la {path}"})
        console.print(result)

    elif command == "/glob":
        if not arg:
            console.print("[red]Usage: /glob <pattern>[/red]")
        else:
            result = tools.execute("Glob", {"pattern": arg})
            console.print(result)

    elif command == "/grep":
        if not arg:
            console.print("[red]Usage: /grep <pattern> [path][/red]")
        else:
            grep_parts = arg.split(maxsplit=1)
            pattern = grep_parts[0]
            path = grep_parts[1] if len(grep_parts) > 1 else "/workspace"
            result = tools.execute("Grep", {"pattern": pattern, "path": path})
            console.print(result)

    elif command == "/memory":
        # Memory management commands (OpenClaw-style)
        if agent is None:
            console.print("[yellow]Memory requires an active agent session[/yellow]")
        elif not arg:
            # List memory files
            memory = agent.memory
            files = memory.list_memory_files()
            if files:
                table = Table(title="Memory Files")
                table.add_column("File", style="cyan")
                table.add_column("Path", style="dim")
                for name, path in MEMORY_FILES.items():
                    table.add_row(name, path)
                for f in files:
                    if f not in [p.split("/")[-1] for p in MEMORY_FILES.values()]:
                        table.add_row(f, f"/memory/{f}")
                console.print(table)
            else:
                console.print("[dim]No memory files yet[/dim]")
        elif arg.startswith("edit "):
            # Edit a memory file
            name = arg[5:].strip()
            if name not in MEMORY_FILES:
                console.print(f"[red]Unknown memory file: {name}[/red]")
                console.print(f"[dim]Available: {', '.join(MEMORY_FILES.keys())}[/dim]")
            else:
                console.print(f"Enter content for {name} (Ctrl+D when done):")
                content = sys.stdin.read()
                if agent.memory.set_memory(name, content):
                    console.print(f"[green]Updated {name}[/green]")
                else:
                    console.print(f"[red]Failed to update {name}[/red]")
        elif arg.startswith("append "):
            # Append to memory
            append_parts = arg[7:].split(maxsplit=1)
            if len(append_parts) < 2:
                console.print("[red]Usage: /memory append <name> <content>[/red]")
            else:
                name, content = append_parts[0], append_parts[1]
                if name not in MEMORY_FILES:
                    console.print(f"[red]Unknown memory file: {name}[/red]")
                else:
                    if agent.memory.append_memory(name, content):
                        console.print(f"[green]Appended to {name}[/green]")
                    else:
                        console.print(f"[red]Failed to append[/red]")
        else:
            # View a memory file
            name = arg.strip()
            if name not in MEMORY_FILES:
                console.print(f"[red]Unknown memory file: {name}[/red]")
                console.print(f"[dim]Available: {', '.join(MEMORY_FILES.keys())}[/dim]")
            else:
                content = agent.memory.get_memory(name)
                console.print(Panel(Markdown(content), title=f"{name}.md", border_style="cyan"))

    else:
        console.print(f"[red]Unknown command: {command}[/red]")
        console.print("[dim]Type /help for available commands[/dim]")

    return None


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="RedisClaw - OpenClaw-style task-solving coding agent",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  redisclaw                          # Interactive mode
  redisclaw --session abc123         # Resume session
  redisclaw --bash "python --version" # Run command and exit
  redisclaw --task "Write a hello world script" # Run task and exit
        """,
    )
    parser.add_argument("--sandbox", default="http://localhost:8090", help="Sandbox URL")
    parser.add_argument("--redis", default="redis://localhost:6380", help="Redis URL")
    parser.add_argument("--key", default="sandbox", help="Redis FS key")
    parser.add_argument("--model", default="claude-sonnet-4-20250514", help="Model to use")
    parser.add_argument("--session", metavar="ID", help="Resume a session by ID")
    parser.add_argument("--bash", metavar="CMD", help="Run a shell command and exit")
    parser.add_argument("--read", metavar="PATH", help="Read a file and exit")
    parser.add_argument("--ls", metavar="PATH", nargs="?", const="/workspace", help="List files and exit")
    parser.add_argument("--task", metavar="DESC", help="Run a task and exit")

    # Legacy argument names
    parser.add_argument("--run", metavar="CMD", help=argparse.SUPPRESS)  # Hidden, use --bash

    args = parser.parse_args()

    config = AgentConfig(
        sandbox_url=args.sandbox,
        redis_url=args.redis,
        fs_key=args.key,
        model=args.model,
    )

    # Single command modes
    if args.bash or args.run or args.read or args.ls:
        tools = ToolExecutor(config.sandbox_url, config.redis_url, config.fs_key)
        try:
            if args.bash or args.run:
                cmd = args.bash or args.run
                print(tools.execute("Bash", {"command": cmd}))
            elif args.read:
                print(tools.execute("Read", {"path": args.read}))
            elif args.ls:
                print(tools.execute("Bash", {"command": f"ls -la {args.ls}"}))
        finally:
            tools.close()
        return

    # Single task mode
    if args.task:
        if not os.environ.get("ANTHROPIC_API_KEY"):
            console.print("[red]ANTHROPIC_API_KEY required for --task mode[/red]")
            sys.exit(1)

        agent = Agent(config)
        try:
            for event in agent.run(args.task):
                formatted = format_event(event)
                if formatted:
                    console.print(formatted)
                if event.type == "assistant" and event.phase == "delta":
                    text = event.data.get("text", "")
                    if text.strip():
                        console.print(Markdown(text))
        finally:
            agent.close()
        return

    # Interactive mode
    run_interactive(config, session_id=args.session)


if __name__ == "__main__":
    main()

