"""Async resource clients: workspaces, checkpoints, filesystem mount, and the AsyncAFS facade."""

from __future__ import annotations

import asyncio
from typing import Any, Mapping, Sequence

from ..errors import AFSError
from ..models import (
    MountMode,
    as_workspace_name as _workspace_name,
)
from ._http import AsyncMCPHttpClient
from ._mount import AsyncMountedFS, _AsyncMountedWorkspace


def _pick_workspace(
    workspace: str | Mapping[str, Any] | None,
    repo: str | Mapping[str, Any] | None,
) -> str:
    """Resolve workspace-or-repo for workspace ops, coercing names (str|Mapping)."""
    return _workspace_name(workspace if workspace is not None else repo)


def _require_checkpoint_workspace(workspace: str | None, repo: str | None, *, action: str) -> str:
    """Resolve the checkpoint workspace by plain truthiness (no name coercion)."""
    workspace_name = workspace or repo
    if not workspace_name:
        raise AFSError(f"checkpoint.{action} requires a workspace")
    return workspace_name


class AsyncWorkspaceClient:
    def __init__(self, mcp: "AsyncMCPHttpClient") -> None:
        self._mcp = mcp

    async def create(
        self, *, name: str, description: str | None = None, template_slug: str | None = None
    ) -> dict[str, Any]:
        return await self._mcp.call_tool(
            "workspace_create",
            {
                "name": name,
                "description": description,
                "template_slug": template_slug,
            },
        )

    async def list(self) -> list[dict[str, Any]]:
        response = await self._mcp.call_tool("workspace_list")
        if isinstance(response, list):
            return response
        return list(response.get("items", []))

    async def get(
        self,
        workspace: str | Mapping[str, Any] | None = None,
        *,
        repo: str | Mapping[str, Any] | None = None,
    ) -> dict[str, Any]:
        return await self._mcp.call_tool(
            "workspace_get",
            {"workspace": _pick_workspace(workspace, repo)},
        )

    async def fork(self, *, source: str, name: str) -> dict[str, Any]:
        return await self._mcp.call_tool("workspace_fork", {"source": source, "name": name})

    async def delete(
        self,
        workspace: str | Mapping[str, Any] | None = None,
        *,
        repo: str | Mapping[str, Any] | None = None,
    ) -> dict[str, Any]:
        return await self._mcp.call_tool(
            "workspace_delete",
            {"workspace": _pick_workspace(workspace, repo)},
        )


AsyncRepoClient = AsyncWorkspaceClient


class AsyncCheckpointClient:
    def __init__(self, mcp: "AsyncMCPHttpClient") -> None:
        self._mcp = mcp

    async def list(self, workspace: str | Mapping[str, Any]) -> list[dict[str, Any]]:
        response = await self._mcp.call_tool("checkpoint_list", {"workspace": _workspace_name(workspace)})
        return list(response.get("checkpoints", []))

    async def create(
        self,
        *,
        workspace: str | None = None,
        repo: str | None = None,
        checkpoint: str | None = None,
    ) -> dict[str, Any]:
        workspace_name = _require_checkpoint_workspace(workspace, repo, action="create")
        return await self._mcp.call_tool("checkpoint_create", {"workspace": workspace_name, "checkpoint": checkpoint})

    async def restore(
        self, *, workspace: str | None = None, repo: str | None = None, checkpoint: str
    ) -> dict[str, Any]:
        workspace_name = _require_checkpoint_workspace(workspace, repo, action="restore")
        return await self._mcp.call_tool("checkpoint_restore", {"workspace": workspace_name, "checkpoint": checkpoint})


class AsyncFSClient:
    def __init__(self, control_plane: "AsyncMCPHttpClient") -> None:
        self._control_plane = control_plane

    async def _mount_one(
        self,
        ref: Mapping[str, Any],
        *,
        profile: str,
        token_name: str | None,
    ) -> _AsyncMountedWorkspace:
        name = _workspace_name(ref)
        issued = await self._control_plane.call_tool(
            "mcp_token_issue",
            {
                "workspace": name,
                "name": token_name or f"redis-afs {name}",
                "profile": profile,
            },
        )
        token = str(issued.get("token", ""))
        if not token:
            raise AFSError(f"mcp_token_issue did not return a token for {name}", payload=issued)
        return _AsyncMountedWorkspace(
            name=name,
            token=token,
            client=AsyncMCPHttpClient(
                api_key=token,
                base_url=issued.get("url") or self._control_plane.endpoint,
                timeout=self._control_plane.timeout,
            ),
        )

    async def mount(
        self,
        *,
        workspaces: Sequence[Mapping[str, Any]] | None = None,
        repos: Sequence[Mapping[str, Any]] | None = None,
        mode: MountMode | str = MountMode.RW,
        token_name: str | None = None,
        concurrency: int = 16,
    ) -> "AsyncMountedFS":
        workspace_refs = list(workspaces if workspaces is not None else repos or [])
        if not workspace_refs:
            raise AFSError("fs.mount requires at least one workspace")
        profile = MountMode.coerce(mode).profile
        # Issue every workspace token concurrently; gather preserves input order.
        results = await asyncio.gather(
            *(self._mount_one(ref, profile=profile, token_name=token_name) for ref in workspace_refs),
            return_exceptions=True,
        )
        mounted = [r for r in results if isinstance(r, _AsyncMountedWorkspace)]
        failure = next((r for r in results if isinstance(r, BaseException)), None)
        if failure is not None:
            # Partial failure: close the children we did build before re-raising.
            await asyncio.gather(*(m.client.aclose() for m in mounted), return_exceptions=True)
            raise failure
        return AsyncMountedFS(mounted, mode=mode, concurrency=concurrency)


class AsyncAFS:
    def __init__(
        self,
        *,
        api_key: str | None = None,
        base_url: str | None = None,
        timeout: float = 30.0,
        headers: Mapping[str, str] | None = None,
    ) -> None:
        self._control_plane = AsyncMCPHttpClient(
            api_key=api_key,
            base_url=base_url,
            timeout=timeout,
            headers=headers,
        )
        self.workspace = AsyncWorkspaceClient(self._control_plane)
        self.workspaces = self.workspace
        self.repo = self.workspace
        self.repos = self.workspace
        self.checkpoint = AsyncCheckpointClient(self._control_plane)
        self.checkpoints = self.checkpoint
        self.fs = AsyncFSClient(self._control_plane)

    async def call_tool(self, name: str, arguments: Mapping[str, Any] | None = None) -> Any:
        return await self._control_plane.call_tool(name, arguments or {})

    async def aclose(self) -> None:
        await self._control_plane.aclose()

    async def __aenter__(self) -> "AsyncAFS":
        return self

    async def __aexit__(self, exc_type: object, exc: object, tb: object) -> None:
        await self.aclose()
