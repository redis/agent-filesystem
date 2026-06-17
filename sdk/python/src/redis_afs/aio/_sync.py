"""Semaphore-bounded remote<->local tree copy for a single mounted workspace."""

from __future__ import annotations

import asyncio
import posixpath
from pathlib import Path
from typing import Any, Awaitable, Callable

from .._paths import normalize_remote_path as _normalize_remote_path


class _TreeSync:
    """Copies one workspace's tree between the remote and a local directory.

    The semaphore guards only the network ``call_tool`` calls; local disk I/O
    runs synchronously. Constructed per-workspace with a shared semaphore so the
    concurrency bound spans every workspace in a mount.
    """

    def __init__(self, workspace_client: Any, semaphore: asyncio.Semaphore) -> None:
        self._client = workspace_client
        self._semaphore = semaphore

    async def pull(self, remote_root: str, local_dir: Path) -> None:
        """Copy the remote tree at ``remote_root`` into ``local_dir`` (remote -> local)."""
        await self._copy_remote_directory(remote_root, local_dir)

    async def push(self, local_dir: Path, remote_root: str) -> None:
        """Copy ``local_dir`` into the remote tree at ``remote_root`` (local -> remote)."""
        await self._copy_local_directory(local_dir, remote_root)

    async def _guarded(self, coro_factory: Callable[[], Awaitable[Any]]) -> Any:
        # Takes a factory (callable returning a fresh coroutine), not a coroutine object,
        # so the semaphore is acquired before the coroutine starts; do not "simplify" to a
        # passed-in coroutine or it becomes a double-await/already-awaited bug.
        async with self._semaphore:
            return await coro_factory()

    async def _copy_remote_directory(self, remote_path: str, local_path: Path) -> None:
        response = await self._guarded(
            lambda: self._client.call_tool("file_list", {"path": remote_path, "depth": 1})
        )
        tasks: list[Awaitable[None]] = []
        for entry in response.get("entries", []):
            target = local_path / entry["name"]
            kind = entry.get("kind")
            if kind == "dir":
                # Create the directory before recursing so child writes land.
                target.mkdir(parents=True, exist_ok=True)
                tasks.append(self._copy_remote_directory(entry["path"], target))
            elif kind == "symlink" and entry.get("target"):
                try:
                    target.symlink_to(entry["target"])
                except FileExistsError:
                    pass
            elif kind == "file":
                tasks.append(self._copy_remote_file(entry["path"], target))
        if tasks:
            await asyncio.gather(*tasks)

    async def _copy_remote_file(self, remote_path: str, target: Path) -> None:
        file_response = await self._guarded(
            lambda: self._client.call_tool("file_read", {"path": remote_path})
        )
        if not file_response.get("binary"):
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(str(file_response.get("content", "")), encoding="utf-8")

    async def _copy_local_directory(self, local_directory: Path, remote_directory: str) -> None:
        tasks: list[Awaitable[None]] = []
        for child in local_directory.iterdir():
            remote_path = _normalize_remote_path(posixpath.join(remote_directory, child.name))
            if child.is_symlink():
                continue
            elif child.is_dir():
                tasks.append(self._copy_local_directory(child, remote_path))
            elif child.is_file():
                tasks.append(self._copy_local_file(child, remote_path))
        if tasks:
            await asyncio.gather(*tasks)

    async def _copy_local_file(self, child: Path, remote_path: str) -> None:
        content = child.read_text(encoding="utf-8")
        await self._guarded(
            lambda: self._client.call_tool("file_write", {"path": remote_path, "content": content})
        )
