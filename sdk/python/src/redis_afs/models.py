"""Public data models and shared constants for the redis_afs SDK."""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum
from typing import Any, Mapping

from .errors import AFSError

DEFAULT_BASE_URL = "https://afs.cloud"


class MountMode(str, Enum):
    """Workspace mount mode.

    Mirrors the control plane's workspace-* MCP profiles
    (MCPProfileWorkspaceRO/RW/RWCheckpoint); the profile is always
    ``workspace-{mode}``.
    """

    RO = "ro"
    RW = "rw"
    RW_CHECKPOINT = "rw-checkpoint"

    @property
    def profile(self) -> str:
        return f"workspace-{self.value}"

    @classmethod
    def coerce(cls, value: "MountMode | str") -> "MountMode":
        try:
            return cls(value)
        except ValueError:
            raise AFSError(f"mode must be one of {[m.value for m in cls]}, got {value!r}")


@dataclass(frozen=True)
class BashResult:
    stdout: str
    stderr: str
    exit_code: int
    command: str
    mapped_command: str


def as_workspace_name(workspace: str | Mapping[str, Any] | None) -> str:
    if isinstance(workspace, str):
        return workspace
    if workspace is None:
        raise AFSError("workspace name is required")
    name = str(workspace.get("name", "")).strip()
    if not name:
        raise AFSError("workspace name is required")
    return name
