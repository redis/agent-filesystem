"""Remote path normalization and workspace path resolution."""

from __future__ import annotations

import posixpath
import re
from pathlib import Path
from typing import Sequence

from .errors import AFSError


def normalize_remote_path(path: str) -> str:
    raw = path.strip()
    if not raw:
        return "/"
    parts = [part for part in raw.split("/") if part]
    if ".." in parts:
        raise AFSError(f"path {path} must not contain '..'")
    normalized = posixpath.normpath(raw if raw.startswith("/") else f"/{raw}")
    return "/" if normalized == "." else normalized


class MountTable:
    """Pure path logic for a set of mounted workspaces.

    Owns longest-prefix workspace resolution, the single-workspace fallback,
    and the absolute-path rewrite used to map remote workspace roots onto a
    local mirror. Holds no workspace clients and performs no I/O.
    """

    def __init__(self, workspace_names: Sequence[str]) -> None:
        self._names = list(workspace_names)

    @property
    def names(self) -> list[str]:
        return list(self._names)

    def resolve(self, raw_path: str) -> tuple[str, str]:
        """Resolve a raw path to ``(workspace_name, remote_path)``."""
        normalized = normalize_remote_path(raw_path)
        for name in sorted(self._names, key=len, reverse=True):
            prefix = f"/{name}"
            if normalized == prefix:
                return name, "/"
            if normalized.startswith(f"{prefix}/"):
                return name, normalized[len(prefix) :] or "/"
        if len(self._names) == 1:
            return self._names[0], normalized
        choices = ", ".join(f"/{name}" for name in self._names)
        raise AFSError(f"path {raw_path} must start with one of: {choices}")

    def map_absolute(self, command: str, local_root: str) -> str:
        """Rewrite absolute ``/workspace`` prefixes to their local mirror paths."""
        out = command
        for name in sorted(self._names, key=len, reverse=True):
            remote_prefix = f"/{name}"
            local_prefix = str(Path(local_root, name)).replace("\\", "/")
            out = re.sub(rf"{re.escape(remote_prefix)}(?=/|\s|$)", local_prefix, out)
        return out
