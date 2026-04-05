"""Redis-FS: Filesystem storage in Redis for agents."""

from redis_fs.client import RedisFS
from redis_fs.exceptions import (
    RedisFSError,
    NotAFileError,
    NotADirectoryError,
    PathNotFoundError,
    SymlinkLoopError,
)

__version__ = "0.1.0"
__all__ = [
    "RedisFS",
    "RedisFSError",
    "NotAFileError",
    "NotADirectoryError",
    "PathNotFoundError",
    "SymlinkLoopError",
]

