"""Agent Filesystem: Filesystem storage in Redis for agents."""

from agent_filesystem.client import AgentFilesystem
from agent_filesystem.exceptions import (
    AgentFilesystemError,
    NotAFileError,
    NotADirectoryError,
    PathNotFoundError,
    SymlinkLoopError,
)

__version__ = "0.1.0"
__all__ = [
    "AgentFilesystem",
    "AgentFilesystemError",
    "NotAFileError",
    "NotADirectoryError",
    "PathNotFoundError",
    "SymlinkLoopError",
]
