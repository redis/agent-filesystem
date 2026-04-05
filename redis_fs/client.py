"""Redis-FS client implementation."""

from typing import Optional, List, Dict, Any, Union
from redis import Redis
from redis.exceptions import ResponseError

from redis_fs.exceptions import (
    NotAFileError,
    NotADirectoryError,
    PathNotFoundError,
    SymlinkLoopError,
)


class RedisFS:
    """Pythonic interface to Redis-FS filesystem module.
    
    Args:
        redis: Redis client instance.
        key: Redis key name for this filesystem volume.
    
    Example:
        >>> import redis
        >>> from redis_fs import RedisFS
        >>> r = redis.Redis()
        >>> fs = RedisFS(r, "myproject")
        >>> fs.write("/notes/todo.md", "# TODO\\n- Item 1")
        >>> fs.read("/notes/todo.md")
        '# TODO\\n- Item 1'
    """

    def __init__(self, redis: Redis, key: str):
        self._redis = redis
        self._key = key

    def _execute(self, cmd: str, *args) -> Any:
        """Execute a FS.* command."""
        try:
            return self._redis.execute_command(f"FS.{cmd}", self._key, *args)
        except ResponseError as e:
            err_msg = str(e).lower()
            if "no such filesystem" in err_msg or "not found" in err_msg:
                return None
            if "not a file" in err_msg:
                raise NotAFileError(str(e))
            if "not a directory" in err_msg:
                raise NotADirectoryError(str(e))
            if "symbolic links" in err_msg:
                raise SymlinkLoopError(str(e))
            raise

    def _handle_error(self, result: Any) -> None:
        """Check for error responses and raise appropriate exceptions."""
        if isinstance(result, bytes):
            msg = result.decode("utf-8", errors="replace")
            if "not a file" in msg.lower():
                raise NotAFileError(msg)
            if "not a directory" in msg.lower():
                raise NotADirectoryError(msg)
            if "symbolic links" in msg.lower():
                raise SymlinkLoopError(msg)

    # === Reading ===

    def read(self, path: str) -> Optional[str]:
        """Read entire file content.
        
        Returns None if file doesn't exist.
        """
        result = self._execute("CAT", path)
        if result is None:
            return None
        return result.decode("utf-8") if isinstance(result, bytes) else result

    def lines(self, path: str, start: int = 1, end: int = -1) -> Optional[str]:
        """Read specific line range (1-indexed, end=-1 means to EOF)."""
        result = self._execute("LINES", path, start, end)
        if result is None:
            return None
        return result.decode("utf-8") if isinstance(result, bytes) else result

    def head(self, path: str, n: int = 10) -> Optional[str]:
        """Read first N lines."""
        result = self._execute("HEAD", path, n)
        if result is None:
            return None
        return result.decode("utf-8") if isinstance(result, bytes) else result

    def tail(self, path: str, n: int = 10) -> Optional[str]:
        """Read last N lines."""
        result = self._execute("TAIL", path, n)
        if result is None:
            return None
        return result.decode("utf-8") if isinstance(result, bytes) else result

    # === Writing ===

    def write(self, path: str, content: str) -> int:
        """Write content to file (creates parents, overwrites existing).
        
        Returns the file size in bytes.
        """
        return self._execute("ECHO", path, content)

    def append(self, path: str, content: str) -> int:
        """Append content to file.
        
        Returns the new file size in bytes.
        """
        return self._execute("APPEND", path, content)

    def insert(self, path: str, after_line: int, content: str) -> bool:
        """Insert content after line N (0=beginning, -1=end).
        
        Returns True on success.
        """
        result = self._execute("INSERT", path, after_line, content)
        return result == b"OK" or result == "OK"

    # === Editing ===

    def replace(
        self,
        path: str,
        old: str,
        new: str,
        all: bool = False,
        line_start: Optional[int] = None,
        line_end: Optional[int] = None,
    ) -> int:
        """Replace text in file.
        
        Args:
            path: File path.
            old: Text to find.
            new: Replacement text.
            all: Replace all occurrences (default: first only).
            line_start: Constrain to lines >= this (1-indexed).
            line_end: Constrain to lines <= this.
        
        Returns:
            Number of replacements made.
        """
        args = [path, old, new]
        if line_start is not None and line_end is not None:
            args.extend(["LINE", line_start, line_end])
        if all:
            args.append("ALL")
        return self._execute("REPLACE", *args)

    def delete_lines(self, path: str, start: int, end: int) -> int:
        """Delete line range (inclusive, 1-indexed).

        Returns number of lines deleted.
        """
        return self._execute("DELETELINES", path, start, end)

    # === Navigation ===

    def ls(self, path: str = "/", long: bool = False) -> List[str]:
        """List directory contents.

        Args:
            path: Directory path.
            long: Include detailed info (mode, size, etc.).

        Returns:
            List of entries (or detailed info if long=True).
        """
        args = [path]
        if long:
            args.append("LONG")
        result = self._execute("LS", *args)
        if result is None:
            return []
        if isinstance(result, list):
            return [e.decode("utf-8") if isinstance(e, bytes) else e for e in result]
        return []

    def tree(self, path: str = "/", depth: Optional[int] = None) -> str:
        """Get directory tree representation."""
        args = [path]
        if depth is not None:
            args.extend(["DEPTH", depth])
        result = self._execute("TREE", *args)
        return result.decode("utf-8") if isinstance(result, bytes) else result or ""

    def find(
        self, path: str, pattern: str, type: Optional[str] = None
    ) -> List[str]:
        """Find files/directories matching glob pattern.

        Args:
            path: Starting path.
            pattern: Glob pattern (e.g., "*.md").
            type: Filter by type ("file", "dir", "link").

        Returns:
            List of matching paths.
        """
        args = [path, pattern]
        if type:
            args.extend(["TYPE", type])
        result = self._execute("FIND", *args)
        if result is None:
            return []
        if isinstance(result, list):
            return [e.decode("utf-8") if isinstance(e, bytes) else e for e in result]
        return []

    def stat(self, path: str) -> Optional[Dict[str, Any]]:
        """Get file/directory metadata."""
        result = self._execute("STAT", path)
        if result is None:
            return None
        if isinstance(result, list):
            # Convert alternating key-value list to dict
            it = iter(result)
            return {
                (k.decode("utf-8") if isinstance(k, bytes) else k): v
                for k, v in zip(it, it)
            }
        return None

    def exists(self, path: str) -> bool:
        """Check if path exists."""
        result = self._execute("TEST", path)
        return result == 1

    # === Search ===

    def grep(
        self, path: str, pattern: str, nocase: bool = False
    ) -> List[Dict[str, Any]]:
        """Search file contents with glob pattern.

        Returns list of matches with file path and matching lines.
        """
        args = [path, pattern]
        if nocase:
            args.append("NOCASE")
        result = self._execute("GREP", *args)
        if result is None:
            return []
        # Parse grep results - they come as [path, match, path, match, ...]
        if isinstance(result, list):
            matches = []
            for item in result:
                if isinstance(item, bytes):
                    matches.append(item.decode("utf-8"))
                else:
                    matches.append(item)
            return matches
        return []

    # === Organization ===

    def mkdir(self, path: str, parents: bool = False) -> bool:
        """Create directory.

        Args:
            path: Directory path.
            parents: Create parent directories as needed.
        """
        args = [path]
        if parents:
            args.append("PARENTS")
        result = self._execute("MKDIR", *args)
        return result == b"OK" or result == "OK"

    def rm(self, path: str, recursive: bool = False) -> bool:
        """Remove file or directory."""
        args = [path]
        if recursive:
            args.append("RECURSIVE")
        result = self._execute("RM", *args)
        return result == 1

    def cp(self, src: str, dst: str, recursive: bool = False) -> bool:
        """Copy file or directory."""
        args = [src, dst]
        if recursive:
            args.append("RECURSIVE")
        result = self._execute("CP", *args)
        return result == b"OK" or result == "OK"

    def mv(self, src: str, dst: str) -> bool:
        """Move/rename file or directory."""
        result = self._execute("MV", src, dst)
        return result == b"OK" or result == "OK"

    def ln(self, target: str, link: str) -> bool:
        """Create symbolic link."""
        result = self._execute("LN", target, link)
        return result == b"OK" or result == "OK"

    def readlink(self, path: str) -> Optional[str]:
        """Read symlink target."""
        result = self._execute("READLINK", path)
        if result is None:
            return None
        return result.decode("utf-8") if isinstance(result, bytes) else result

    # === Stats ===

    def wc(self, path: str) -> Optional[Dict[str, int]]:
        """Get line/word/character counts.

        Returns dict with 'lines', 'words', 'chars' keys.
        """
        result = self._execute("WC", path)
        if result is None:
            return None
        if isinstance(result, list):
            it = iter(result)
            return {
                (k.decode("utf-8") if isinstance(k, bytes) else k): v
                for k, v in zip(it, it)
            }
        return None

    def info(self) -> Dict[str, Any]:
        """Get filesystem statistics."""
        result = self._redis.execute_command("FS.INFO", self._key)
        if isinstance(result, list):
            it = iter(result)
            return {
                (k.decode("utf-8") if isinstance(k, bytes) else k): v
                for k, v in zip(it, it)
            }
        return {}

