from __future__ import annotations

import json
import os
import posixpath
import re
import shutil
import subprocess
import tempfile
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Mapping, MutableMapping, Sequence

DEFAULT_BASE_URL = "https://afs.cloud"


class AFSError(RuntimeError):
    def __init__(
        self,
        message: str,
        *,
        status: int | None = None,
        code: int | None = None,
        payload: Any | None = None,
    ) -> None:
        super().__init__(message)
        self.status = status
        self.code = code
        self.payload = payload


@dataclass(frozen=True)
class BashResult:
    stdout: str
    stderr: str
    exit_code: int
    command: str
    mapped_command: str


class AFS:
    def __init__(
        self,
        *,
        api_key: str | None = None,
        base_url: str | None = None,
        timeout: float = 30.0,
        headers: Mapping[str, str] | None = None,
    ) -> None:
        self._control_plane = MCPHttpClient(
            api_key=api_key,
            base_url=base_url,
            timeout=timeout,
            headers=headers,
        )
        self.workspace = WorkspaceClient(self._control_plane)
        self.workspaces = self.workspace
        self.repo = self.workspace
        self.repos = self.workspace
        self.checkpoint = CheckpointClient(self._control_plane)
        self.checkpoints = self.checkpoint
        self.fs = FSClient(self._control_plane)

    def call_tool(self, name: str, arguments: Mapping[str, Any] | None = None) -> Any:
        return self._control_plane.call_tool(name, arguments or {})


class WorkspaceClient:
    def __init__(self, mcp: "MCPHttpClient") -> None:
        self._mcp = mcp

    def create(self, *, name: str, description: str | None = None, template_slug: str | None = None) -> dict[str, Any]:
        return self._mcp.call_tool(
            "workspace_create",
            {
                "name": name,
                "description": description,
                "template_slug": template_slug,
            },
        )

    def list(self) -> list[dict[str, Any]]:
        response = self._mcp.call_tool("workspace_list")
        if isinstance(response, list):
            return response
        return list(response.get("items", []))

    def get(
        self,
        workspace: str | Mapping[str, Any] | None = None,
        *,
        repo: str | Mapping[str, Any] | None = None,
    ) -> dict[str, Any]:
        return self._mcp.call_tool(
            "workspace_get",
            {"workspace": _workspace_name(workspace if workspace is not None else repo)},
        )

    def fork(self, *, source: str, name: str) -> dict[str, Any]:
        return self._mcp.call_tool("workspace_fork", {"source": source, "name": name})

    def delete(
        self,
        workspace: str | Mapping[str, Any] | None = None,
        *,
        repo: str | Mapping[str, Any] | None = None,
    ) -> dict[str, Any]:
        return self._mcp.call_tool(
            "workspace_delete",
            {"workspace": _workspace_name(workspace if workspace is not None else repo)},
        )


RepoClient = WorkspaceClient


class CheckpointClient:
    def __init__(self, mcp: "MCPHttpClient") -> None:
        self._mcp = mcp

    def list(self, workspace: str | Mapping[str, Any]) -> list[dict[str, Any]]:
        response = self._mcp.call_tool("checkpoint_list", {"workspace": _workspace_name(workspace)})
        return list(response.get("checkpoints", []))

    def create(
        self,
        *,
        workspace: str | None = None,
        repo: str | None = None,
        checkpoint: str | None = None,
    ) -> dict[str, Any]:
        workspace_name = workspace or repo
        if not workspace_name:
            raise AFSError("checkpoint.create requires a workspace")
        return self._mcp.call_tool("checkpoint_create", {"workspace": workspace_name, "checkpoint": checkpoint})

    def restore(self, *, workspace: str | None = None, repo: str | None = None, checkpoint: str) -> dict[str, Any]:
        workspace_name = workspace or repo
        if not workspace_name:
            raise AFSError("checkpoint.restore requires a workspace")
        return self._mcp.call_tool("checkpoint_restore", {"workspace": workspace_name, "checkpoint": checkpoint})


class FSClient:
    def __init__(self, control_plane: "MCPHttpClient") -> None:
        self._control_plane = control_plane

    def mount(
        self,
        *,
        workspaces: Sequence[Mapping[str, Any]] | None = None,
        repos: Sequence[Mapping[str, Any]] | None = None,
        mode: str = "rw",
        token_name: str | None = None,
    ) -> "MountedFS":
        workspace_refs = list(workspaces if workspaces is not None else repos or [])
        if not workspace_refs:
            raise AFSError("fs.mount requires at least one workspace")
        profile = _profile_for_mode(mode)
        mounted: list[_MountedWorkspace] = []
        for workspace in workspace_refs:
            name = _workspace_name(workspace)
            issued = self._control_plane.call_tool(
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
            mounted.append(
                _MountedWorkspace(
                    name=name,
                    token=token,
                    client=MCPHttpClient(
                        api_key=token,
                        base_url=issued.get("url") or self._control_plane.endpoint,
                        timeout=self._control_plane.timeout,
                    ),
                )
            )
        return MountedFS(mounted, mode=mode)


@dataclass(frozen=True)
class _MountedWorkspace:
    name: str
    token: str
    client: "MCPHttpClient"


class MountedFS:
    def __init__(self, workspaces: Sequence[_MountedWorkspace], *, mode: str = "rw") -> None:
        self._workspaces = list(workspaces)
        self._workspaces_by_name = {workspace.name: workspace for workspace in self._workspaces}
        if len(self._workspaces_by_name) != len(self._workspaces):
            raise AFSError("workspaces must be mounted at most once")
        self.mode = mode
        self._local_root: tempfile.TemporaryDirectory[str] | None = None

    @property
    def repo_names(self) -> list[str]:
        return self.workspace_names

    @property
    def workspace_names(self) -> list[str]:
        return [workspace.name for workspace in self._workspaces]

    @property
    def local_root(self) -> str | None:
        return self._local_root.name if self._local_root else None

    def read_file(self, path: str) -> str:
        workspace, remote_path = self._resolve_path(path)
        response = workspace.client.call_tool("file_read", {"path": remote_path})
        if response.get("binary"):
            raise AFSError(f"file {remote_path} is binary and cannot be returned as text")
        if response.get("kind") == "dir":
            raise AFSError(f"path {remote_path} is a directory")
        return str(response.get("content", ""))

    def write_file(self, path: str, content: str | bytes) -> None:
        workspace, remote_path = self._resolve_path(path)
        text = content.decode("utf-8") if isinstance(content, bytes) else content
        workspace.client.call_tool("file_write", {"path": remote_path, "content": text})
        if self.local_root:
            local_path = self._local_path_for(workspace.name, remote_path)
            local_path.parent.mkdir(parents=True, exist_ok=True)
            local_path.write_text(text, encoding="utf-8")

    def list_files(self, path: str = "/", depth: int = 1) -> list[dict[str, Any]]:
        workspace, remote_path = self._resolve_path(path)
        response = workspace.client.call_tool("file_list", {"path": remote_path, "depth": depth})
        return list(response.get("entries", []))

    def glob(
        self,
        pattern: str,
        *,
        path: str = "/",
        kind: str | None = None,
        limit: int | None = None,
    ) -> dict[str, Any]:
        workspace, remote_path = self._resolve_path(path)
        return workspace.client.call_tool(
            "file_glob",
            {"path": remote_path, "pattern": pattern, "kind": kind, "limit": limit},
        )

    def grep(self, pattern: str, **options: Any) -> dict[str, Any]:
        workspace, remote_path = self._resolve_path(str(options.pop("path", "/")))
        return workspace.client.call_tool("file_grep", {"path": remote_path, "pattern": pattern, **options})

    def checkpoint(self, name: str | None = None) -> list[dict[str, Any]]:
        return [workspace.client.call_tool("checkpoint_create", {"checkpoint": name}) for workspace in self._workspaces]

    def bash(self) -> "BashRunner":
        return BashRunner(self)

    def sync_from_remote(self) -> str:
        root = self._ensure_local_root()
        for workspace in self._workspaces:
            workspace_root = Path(root, workspace.name)
            shutil.rmtree(workspace_root, ignore_errors=True)
            workspace_root.mkdir(parents=True, exist_ok=True)
            self._copy_remote_directory(workspace, "/", workspace_root)
        return root

    def sync_to_remote(self) -> None:
        if not self.local_root:
            return
        for workspace in self._workspaces:
            workspace_root = Path(self.local_root, workspace.name)
            if workspace_root.exists():
                self._copy_local_directory(workspace, workspace_root, "/")

    def close(self) -> None:
        if self._local_root:
            self._local_root.cleanup()
            self._local_root = None

    def __enter__(self) -> "MountedFS":
        return self

    def __exit__(self, exc_type: object, exc: object, tb: object) -> None:
        self.close()

    def map_absolute_workspace_paths(self, command: str) -> str:
        if not self.local_root:
            return command
        out = command
        for name in sorted(self.workspace_names, key=len, reverse=True):
            remote_prefix = f"/{name}"
            local_prefix = str(Path(self.local_root, name)).replace("\\", "/")
            out = re.sub(rf"{re.escape(remote_prefix)}(?=/|\s|$)", local_prefix, out)
        return out

    def map_absolute_repo_paths(self, command: str) -> str:
        return self.map_absolute_workspace_paths(command)

    def _resolve_path(self, raw_path: str) -> tuple[_MountedWorkspace, str]:
        normalized = _normalize_remote_path(raw_path)
        for name in sorted(self.workspace_names, key=len, reverse=True):
            prefix = f"/{name}"
            if normalized == prefix:
                return self._workspaces_by_name[name], "/"
            if normalized.startswith(f"{prefix}/"):
                return self._workspaces_by_name[name], normalized[len(prefix) :] or "/"
        if len(self._workspaces) == 1:
            return self._workspaces[0], normalized
        choices = ", ".join(f"/{name}" for name in self.workspace_names)
        raise AFSError(f"path {raw_path} must start with one of: {choices}")

    def _ensure_local_root(self) -> str:
        if not self._local_root:
            self._local_root = tempfile.TemporaryDirectory(prefix="afs-fs-")
        return self._local_root.name

    def _local_path_for(self, workspace_name: str, remote_path: str) -> Path:
        if not self.local_root:
            raise AFSError("mount has not been materialized locally yet")
        relative = _normalize_remote_path(remote_path).lstrip("/")
        return Path(self.local_root, workspace_name, relative)

    def _copy_remote_directory(self, workspace: _MountedWorkspace, remote_path: str, local_path: Path) -> None:
        response = workspace.client.call_tool("file_list", {"path": remote_path, "depth": 1})
        for entry in response.get("entries", []):
            target = local_path / entry["name"]
            kind = entry.get("kind")
            if kind == "dir":
                target.mkdir(parents=True, exist_ok=True)
                self._copy_remote_directory(workspace, entry["path"], target)
            elif kind == "symlink" and entry.get("target"):
                try:
                    target.symlink_to(entry["target"])
                except FileExistsError:
                    pass
            elif kind == "file":
                file_response = workspace.client.call_tool("file_read", {"path": entry["path"]})
                if not file_response.get("binary"):
                    target.parent.mkdir(parents=True, exist_ok=True)
                    target.write_text(str(file_response.get("content", "")), encoding="utf-8")

    def _copy_local_directory(self, workspace: _MountedWorkspace, local_directory: Path, remote_directory: str) -> None:
        for child in local_directory.iterdir():
            remote_path = _normalize_remote_path(posixpath.join(remote_directory, child.name))
            if child.is_dir():
                self._copy_local_directory(workspace, child, remote_path)
            elif child.is_file():
                workspace.client.call_tool(
                    "file_write",
                    {"path": remote_path, "content": child.read_text(encoding="utf-8")},
                )


class BashRunner:
    def __init__(self, mounted_fs: MountedFS) -> None:
        self._fs = mounted_fs

    def exec(
        self,
        command: str,
        *,
        cwd: str | None = None,
        env: Mapping[str, str | None] | None = None,
        timeout: float | None = None,
        check: bool = False,
    ) -> BashResult:
        root = self._fs.sync_from_remote()
        mapped_command = self._fs.map_absolute_workspace_paths(command)
        run_env: MutableMapping[str, str] = dict(os.environ)
        if env:
            for key, value in env.items():
                if value is None:
                    run_env.pop(key, None)
                else:
                    run_env[key] = value
        completed = subprocess.run(
            mapped_command,
            cwd=str(Path(root, cwd)) if cwd else root,
            env=run_env,
            shell=True,
            executable="/bin/bash",
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
        self._fs.sync_to_remote()
        result = BashResult(
            stdout=completed.stdout,
            stderr=completed.stderr,
            exit_code=completed.returncode,
            command=command,
            mapped_command=mapped_command,
        )
        if check and result.exit_code != 0:
            raise AFSError(f"command exited with status {result.exit_code}", payload=result)
        return result


class MCPHttpClient:
    def __init__(
        self,
        *,
        api_key: str | None = None,
        base_url: str | None = None,
        timeout: float = 30.0,
        headers: Mapping[str, str] | None = None,
    ) -> None:
        self.api_key = api_key or os.environ.get("AFS_API_KEY") or ""
        if not self.api_key:
            raise AFSError("AFS api_key is required")
        base = base_url or os.environ.get("AFS_API_BASE_URL") or DEFAULT_BASE_URL
        self.endpoint = _normalize_mcp_endpoint(base)
        self.timeout = timeout
        self.headers = dict(headers or {})
        self._next_id = 1

    def call_tool(self, name: str, arguments: Mapping[str, Any] | None = None) -> Any:
        result = self.request(
            "tools/call",
            {
                "name": name,
                "arguments": _strip_none(dict(arguments or {})),
            },
        )
        if result.get("isError"):
            content = "\n".join(item.get("text", "") for item in result.get("content", []))
            raise AFSError(content or f"MCP tool {name} failed", payload=result)
        return result.get("structuredContent", result)

    def request(self, method: str, params: Mapping[str, Any] | None = None) -> Any:
        body = json.dumps(
            {
                "jsonrpc": "2.0",
                "id": self._next_id,
                "method": method,
                "params": dict(params or {}),
            }
        ).encode("utf-8")
        self._next_id += 1
        headers = {
            "content-type": "application/json",
            "authorization": f"Bearer {self.api_key}",
            **self.headers,
        }
        request = urllib.request.Request(self.endpoint, data=body, headers=headers, method="POST")
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                payload = json.loads(response.read().decode("utf-8") or "{}")
        except urllib.error.HTTPError as exc:
            text = exc.read().decode("utf-8", errors="replace")
            raise AFSError(f"MCP request failed with HTTP {exc.code}: {text}", status=exc.code, payload=text) from exc
        if payload.get("error"):
            error = payload["error"]
            raise AFSError(str(error.get("message", "MCP request failed")), code=error.get("code"), payload=payload)
        return payload.get("result")


def _workspace_name(workspace: str | Mapping[str, Any] | None) -> str:
    if isinstance(workspace, str):
        return workspace
    if workspace is None:
        raise AFSError("workspace name is required")
    name = str(workspace.get("name", "")).strip()
    if not name:
        raise AFSError("workspace name is required")
    return name


def _profile_for_mode(mode: str) -> str:
    if mode == "ro":
        return "workspace-ro"
    if mode == "rw":
        return "workspace-rw"
    if mode == "rw-checkpoint":
        return "workspace-rw-checkpoint"
    raise AFSError('mode must be "ro", "rw", or "rw-checkpoint"')


def _normalize_mcp_endpoint(base_url: str) -> str:
    trimmed = base_url.strip().rstrip("/")
    if not trimmed:
        raise AFSError("base_url is required")
    return trimmed if trimmed.endswith("/mcp") else f"{trimmed}/mcp"


def _normalize_remote_path(path: str) -> str:
    raw = path.strip()
    if not raw:
        return "/"
    parts = [part for part in raw.split("/") if part]
    if ".." in parts:
        raise AFSError(f"path {path} must not contain '..'")
    normalized = posixpath.normpath(raw if raw.startswith("/") else f"/{raw}")
    return "/" if normalized == "." else normalized


def _strip_none(values: dict[str, Any]) -> dict[str, Any]:
    return {key: value for key, value in values.items() if value is not None}
