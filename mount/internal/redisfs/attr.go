package redisfs

import (
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/redis-fs/mount/internal/client"
)

// statToAttr converts a StatResult to a fuse.Attr.
func statToAttr(st *client.StatResult, uid, gid uint32) fuse.Attr {
	var mode uint32
	switch st.Type {
	case "file":
		mode = syscall.S_IFREG | st.Mode
	case "dir":
		mode = syscall.S_IFDIR | st.Mode
	case "symlink":
		mode = syscall.S_IFLNK | st.Mode
	}

	var nlink uint32 = 1
	if st.Type == "dir" {
		nlink = 2
	}

	attr := fuse.Attr{
		Mode:  mode,
		Nlink: nlink,
		Size:  uint64(st.Size),
		Owner: fuse.Owner{Uid: uid, Gid: gid},
		Atime: uint64(st.Atime / 1000),
		Atimensec: uint32((st.Atime % 1000) * 1_000_000),
		Mtime: uint64(st.Mtime / 1000),
		Mtimensec: uint32((st.Mtime % 1000) * 1_000_000),
		Ctime: uint64(st.Ctime / 1000),
		Ctimensec: uint32((st.Ctime % 1000) * 1_000_000),
	}

	if st.Type == "dir" {
		attr.Size = 4096
	}

	// Blocks: 512-byte blocks
	attr.Blocks = (attr.Size + 511) / 512

	return attr
}
