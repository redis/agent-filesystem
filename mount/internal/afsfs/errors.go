package afsfs

import (
	"errors"
	"strings"
	"syscall"

	"github.com/redis/agent-filesystem/mount/internal/client"
)

// mapError maps an AFS client error to a syscall errno. Sentinel checks
// (errors.Is) take precedence so a wrapped error
// (fmt.Errorf("...: %w", client.ErrNotFile)) still maps correctly. The
// substring fallback exists because some errors arrive from outside the
// client package — Redis "ERR ..." replies, redis-go errors, etc. — that
// can't carry one of our sentinels but still mention the same condition.
func mapError(err error) syscall.Errno {
	if err == nil {
		return 0
	}

	switch {
	case errors.Is(err, client.ErrNotFound):
		return syscall.ENOENT
	case errors.Is(err, client.ErrNotFile),
		errors.Is(err, client.ErrCannotWriteRoot):
		return syscall.EISDIR
	case errors.Is(err, client.ErrNotDir),
		errors.Is(err, client.ErrParentConflict):
		return syscall.ENOTDIR
	case errors.Is(err, client.ErrAlreadyExists):
		return syscall.EEXIST
	case errors.Is(err, client.ErrDirNotEmpty):
		return syscall.ENOTEMPTY
	case errors.Is(err, client.ErrUnsupported):
		return syscall.ENOTSUP
	case errors.Is(err, client.ErrLockWouldBlock):
		return syscall.EAGAIN
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "no such filesystem key"),
		strings.Contains(msg, "no such file or directory"),
		strings.Contains(msg, "no such directory"):
		return syscall.ENOENT
	case strings.Contains(msg, "not a file"),
		strings.Contains(msg, "cannot write to root"):
		return syscall.EISDIR
	case strings.Contains(msg, "not a directory"),
		strings.Contains(msg, "parent path conflict"):
		return syscall.ENOTDIR
	case strings.Contains(msg, "already exists"):
		return syscall.EEXIST
	case strings.Contains(msg, "directory not empty"):
		return syscall.ENOTEMPTY
	case strings.Contains(msg, "operation not supported"):
		return syscall.ENOTSUP
	case strings.Contains(msg, "too many levels of symbolic links"):
		return syscall.ELOOP
	case strings.Contains(msg, "path depth exceeds limit"),
		strings.Contains(msg, "mode must be"),
		strings.Contains(msg, "uid out of range"),
		strings.Contains(msg, "gid out of range"),
		strings.Contains(msg, "invalid lock"),
		strings.Contains(msg, "cannot move a directory into its own subtree"),
		strings.Contains(msg, "syntax error"):
		return syscall.EINVAL
	case strings.Contains(msg, "WRONGTYPE"):
		return syscall.EINVAL
	default:
		return syscall.EIO
	}
}
