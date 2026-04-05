"""Agent Filesystem exceptions."""


class AgentFilesystemError(Exception):
    """Base exception for Agent Filesystem errors."""
    pass


class NotAFileError(AgentFilesystemError):
    """Raised when a file operation is attempted on a non-file."""
    pass


class NotADirectoryError(AgentFilesystemError):
    """Raised when a directory operation is attempted on a non-directory."""
    pass


class PathNotFoundError(AgentFilesystemError):
    """Raised when a path does not exist."""
    pass


class SymlinkLoopError(AgentFilesystemError):
    """Raised when too many levels of symbolic links are encountered."""
    pass
