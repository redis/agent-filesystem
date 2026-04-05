package redisfs

import (
	"errors"
	"syscall"
	"testing"
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
		{"ERR too many levels of symbolic links", syscall.ELOOP},
		{"ERR path depth exceeds limit", syscall.EINVAL},
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
