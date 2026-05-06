package afsfs

import (
	"errors"
	"fmt"
	"syscall"
	"testing"

	"github.com/redis/agent-filesystem/mount/internal/client"
)

func TestMapError(t *testing.T) {
	cases := []struct {
		msg  string
		want syscall.Errno
	}{
		{"ERR no such file or directory", syscall.ENOENT},
		{"ERR not a directory", syscall.ENOTDIR},
		{"ERR not a file", syscall.EISDIR},
		{"ERR destination already exists", syscall.EEXIST},
		{"ERR directory not empty", syscall.ENOTEMPTY},
		{"ERR operation not supported", syscall.ENOTSUP},
		{"ERR too many levels of symbolic links", syscall.ELOOP},
		{"ERR path depth exceeds limit", syscall.EINVAL},
		{"ERR invalid lock range", syscall.EINVAL},
		{"ERR mode must be an octal value between 0000 and 07777", syscall.EINVAL},
		{"ERR uid out of range", syscall.EINVAL},
		{"ERR cannot move a directory into its own subtree", syscall.EINVAL},
	}

	for _, tc := range cases {
		got := mapError(errors.New(tc.msg))
		if got != tc.want {
			t.Fatalf("mapError(%q) = %d, want %d", tc.msg, got, tc.want)
		}
	}
}

// TestMapErrorSentinels confirms mapError matches client sentinels via
// errors.Is, including when the caller wraps the sentinel with extra
// context. The substring fallback would silently break under wrapping;
// this test ensures the principled path doesn't.
func TestMapErrorSentinels(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want syscall.Errno
	}{
		{"NotFound", client.ErrNotFound, syscall.ENOENT},
		{"NotFile", client.ErrNotFile, syscall.EISDIR},
		{"NotDir", client.ErrNotDir, syscall.ENOTDIR},
		{"AlreadyExists", client.ErrAlreadyExists, syscall.EEXIST},
		{"DirNotEmpty", client.ErrDirNotEmpty, syscall.ENOTEMPTY},
		{"Unsupported", client.ErrUnsupported, syscall.ENOTSUP},
		{"LockWouldBlock", client.ErrLockWouldBlock, syscall.EAGAIN},
		{"WrappedNotFile", fmt.Errorf("inode 42: %w", client.ErrNotFile), syscall.EISDIR},
		{"WrappedAlreadyExists", fmt.Errorf("create %q: %w", "/foo", client.ErrAlreadyExists), syscall.EEXIST},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapError(tc.err); got != tc.want {
				t.Fatalf("mapError(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}
