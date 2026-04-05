"""Redis-FS exceptions."""


class RedisFSError(Exception):
    """Base exception for Redis-FS errors."""
    pass


class NotAFileError(RedisFSError):
    """Raised when a file operation is attempted on a non-file."""
    pass


class NotADirectoryError(RedisFSError):
    """Raised when a directory operation is attempted on a non-directory."""
    pass


class PathNotFoundError(RedisFSError):
    """Raised when a path does not exist."""
    pass


class SymlinkLoopError(RedisFSError):
    """Raised when too many levels of symbolic links are encountered."""
    pass

