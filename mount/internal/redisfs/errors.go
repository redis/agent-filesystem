package redisfs

import (
	"strings"
	"syscall"
)

// mapError maps a Redis FS error to a syscall errno.
func mapError(err error) syscall.Errno {
	if err == nil {
		return 0
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
	case strings.Contains(msg, "too many levels of symbolic links"):
		return syscall.ELOOP
	case strings.Contains(msg, "path depth exceeds limit"),
		strings.Contains(msg, "mode must be"),
		strings.Contains(msg, "uid out of range"),
		strings.Contains(msg, "gid out of range"),
		strings.Contains(msg, "cannot move a directory into its own subtree"),
		strings.Contains(msg, "syntax error"):
		return syscall.EINVAL
	case strings.Contains(msg, "WRONGTYPE"):
		return syscall.EINVAL
	default:
		return syscall.EIO
	}
}
