"""Redis-FS CLI - thin wrapper over FS.* commands."""

import sys
import click
import redis as redis_lib

from redis_fs import RedisFS


def get_redis(ctx) -> redis_lib.Redis:
    """Get Redis client from context."""
    return ctx.obj["redis"]


def get_fs(ctx, key: str) -> RedisFS:
    """Get RedisFS instance."""
    return RedisFS(get_redis(ctx), key)


@click.group()
@click.option("--host", "-h", default="localhost", help="Redis host")
@click.option("--port", "-p", default=6379, type=int, help="Redis port")
@click.option("--db", "-n", default=0, type=int, help="Redis database number")
@click.option("--url", "-u", default=None, help="Redis URL (overrides host/port/db)")
@click.pass_context
def cli(ctx, host, port, db, url):
    """Redis-FS: Filesystem operations in Redis.
    
    Each command operates on a filesystem volume (KEY) stored in Redis.
    """
    ctx.ensure_object(dict)
    if url:
        ctx.obj["redis"] = redis_lib.from_url(url)
    else:
        ctx.obj["redis"] = redis_lib.Redis(host=host, port=port, db=db)


# === Reading ===

@cli.command()
@click.argument("key")
@click.argument("path")
@click.pass_context
def cat(ctx, key, path):
    """Read entire file content."""
    fs = get_fs(ctx, key)
    content = fs.read(path)
    if content is not None:
        click.echo(content, nl=False)


@cli.command()
@click.argument("key")
@click.argument("path")
@click.argument("start", type=int)
@click.argument("end", type=int, default=-1)
@click.pass_context
def lines(ctx, key, path, start, end):
    """Read specific line range (1-indexed, -1=EOF)."""
    fs = get_fs(ctx, key)
    content = fs.lines(path, start, end)
    if content is not None:
        click.echo(content, nl=False)


@cli.command()
@click.argument("key")
@click.argument("path")
@click.option("-n", "--lines", "n", default=10, type=int, help="Number of lines")
@click.pass_context
def head(ctx, key, path, n):
    """Read first N lines (default 10)."""
    fs = get_fs(ctx, key)
    content = fs.head(path, n)
    if content is not None:
        click.echo(content, nl=False)


@cli.command()
@click.argument("key")
@click.argument("path")
@click.option("-n", "--lines", "n", default=10, type=int, help="Number of lines")
@click.pass_context
def tail(ctx, key, path, n):
    """Read last N lines (default 10)."""
    fs = get_fs(ctx, key)
    content = fs.tail(path, n)
    if content is not None:
        click.echo(content, nl=False)


# === Writing ===

@cli.command()
@click.argument("key")
@click.argument("path")
@click.argument("content")
@click.option("--append", "-a", is_flag=True, help="Append instead of overwrite")
@click.pass_context
def echo(ctx, key, path, content, append):
    """Write content to file."""
    fs = get_fs(ctx, key)
    # Handle escaped newlines
    content = content.replace("\\n", "\n")
    if append:
        fs.append(path, content)
    else:
        fs.write(path, content)


@cli.command()
@click.argument("key")
@click.argument("path")
@click.argument("line", type=int)
@click.argument("content")
@click.pass_context
def insert(ctx, key, path, line, content):
    """Insert content after line N (0=beginning, -1=end)."""
    fs = get_fs(ctx, key)
    content = content.replace("\\n", "\n")
    fs.insert(path, line, content)


# === Editing ===

@cli.command()
@click.argument("key")
@click.argument("path")
@click.argument("old")
@click.argument("new")
@click.option("--all", "-a", "replace_all", is_flag=True, help="Replace all occurrences")
@click.option("--line", "-l", "line_range", nargs=2, type=int, help="Constrain to line range")
@click.pass_context
def replace(ctx, key, path, old, new, replace_all, line_range):
    """Replace text in file."""
    fs = get_fs(ctx, key)
    line_start, line_end = line_range if line_range else (None, None)
    count = fs.replace(path, old, new, all=replace_all, line_start=line_start, line_end=line_end)
    click.echo(f"{count} replacement(s)")


@cli.command("delete-lines")
@click.argument("key")
@click.argument("path")
@click.argument("start", type=int)
@click.argument("end", type=int)
@click.pass_context
def delete_lines(ctx, key, path, start, end):
    """Delete line range (inclusive, 1-indexed)."""
    fs = get_fs(ctx, key)
    count = fs.delete_lines(path, start, end)
    click.echo(f"{count} line(s) deleted")


# === Navigation ===

@cli.command()
@click.argument("key")
@click.argument("path", default="/")
@click.option("-l", "--long", is_flag=True, help="Long format with details")
@click.pass_context
def ls(ctx, key, path, long):
    """List directory contents."""
    fs = get_fs(ctx, key)
    entries = fs.ls(path, long=long)
    for entry in entries:
        click.echo(entry)


@cli.command()
@click.argument("key")
@click.argument("path", default="/")
@click.option("-d", "--depth", type=int, help="Maximum depth")
@click.pass_context
def tree(ctx, key, path, depth):
    """Show directory tree."""
    fs = get_fs(ctx, key)
    result = fs.tree(path, depth=depth)
    click.echo(result)


@cli.command()
@click.argument("key")
@click.argument("path")
@click.argument("pattern")
@click.option("-t", "--type", "type_", type=click.Choice(["file", "dir", "link"]))
@click.pass_context
def find(ctx, key, path, pattern, type_):
    """Find files matching glob pattern."""
    fs = get_fs(ctx, key)
    results = fs.find(path, pattern, type=type_)
    for result in results:
        click.echo(result)


@cli.command()
@click.argument("key")
@click.argument("path")
@click.pass_context
def stat(ctx, key, path):
    """Get file/directory metadata."""
    fs = get_fs(ctx, key)
    info = fs.stat(path)
    if info:
        for k, v in info.items():
            click.echo(f"{k}: {v}")
    else:
        click.echo("Not found", err=True)
        sys.exit(1)


# === Search ===

@cli.command()
@click.argument("key")
@click.argument("path")
@click.argument("pattern")
@click.option("-i", "--nocase", is_flag=True, help="Case-insensitive search")
@click.pass_context
def grep(ctx, key, path, pattern, nocase):
    """Search file contents with glob pattern."""
    fs = get_fs(ctx, key)
    matches = fs.grep(path, pattern, nocase=nocase)
    for match in matches:
        click.echo(match)


# === Organization ===

@cli.command()
@click.argument("key")
@click.argument("path")
@click.option("-p", "--parents", is_flag=True, help="Create parent directories")
@click.pass_context
def mkdir(ctx, key, path, parents):
    """Create directory."""
    fs = get_fs(ctx, key)
    fs.mkdir(path, parents=parents)


@cli.command()
@click.argument("key")
@click.argument("path")
@click.option("-r", "--recursive", is_flag=True, help="Remove directories recursively")
@click.pass_context
def rm(ctx, key, path, recursive):
    """Remove file or directory."""
    fs = get_fs(ctx, key)
    fs.rm(path, recursive=recursive)


@cli.command()
@click.argument("key")
@click.argument("src")
@click.argument("dst")
@click.option("-r", "--recursive", is_flag=True, help="Copy directories recursively")
@click.pass_context
def cp(ctx, key, src, dst, recursive):
    """Copy file or directory."""
    fs = get_fs(ctx, key)
    fs.cp(src, dst, recursive=recursive)


@cli.command()
@click.argument("key")
@click.argument("src")
@click.argument("dst")
@click.pass_context
def mv(ctx, key, src, dst):
    """Move/rename file or directory."""
    fs = get_fs(ctx, key)
    fs.mv(src, dst)


@cli.command()
@click.argument("key")
@click.argument("target")
@click.argument("link")
@click.pass_context
def ln(ctx, key, target, link):
    """Create symbolic link."""
    fs = get_fs(ctx, key)
    fs.ln(target, link)


# === Stats ===

@cli.command()
@click.argument("key")
@click.argument("path")
@click.pass_context
def wc(ctx, key, path):
    """Get line/word/character counts."""
    fs = get_fs(ctx, key)
    stats = fs.wc(path)
    if stats:
        click.echo(f"{stats.get('lines', 0)} {stats.get('words', 0)} {stats.get('chars', 0)}")
    else:
        click.echo("Not found", err=True)
        sys.exit(1)


@cli.command()
@click.argument("key")
@click.pass_context
def info(ctx, key):
    """Get filesystem statistics."""
    fs = get_fs(ctx, key)
    stats = fs.info()
    for k, v in stats.items():
        click.echo(f"{k}: {v}")


def main():
    """Entry point."""
    cli()


if __name__ == "__main__":
    main()

