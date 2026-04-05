package redisfs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Symlink implements fs.NodeSymlinker.
func (n *FSNode) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if n.opts.ReadOnly {
		return nil, syscall.EROFS
	}

	child := n.newChild(name)

	if err := n.client.Ln(ctx, target, child.fsPath); err != nil {
		return nil, mapError(err)
	}

	n.root().invalidatePath(child.fsPath)

	st, err := n.client.Stat(ctx, child.fsPath)
	if err != nil {
		return nil, mapError(err)
	}
	attr := statToAttr(st, n.opts.UID, n.opts.GID)
	out.Attr = attr
	out.SetEntryTimeout(n.opts.AttrTimeout)
	out.SetAttrTimeout(n.opts.AttrTimeout)

	node := n.NewInode(ctx, child, fs.StableAttr{Mode: syscall.S_IFLNK})
	return node, 0
}

// Readlink implements fs.NodeReadlinker.
func (n *FSNode) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	target, err := n.client.Readlink(ctx, n.fsPath)
	if err != nil {
		return nil, mapError(err)
	}
	return []byte(target), 0
}

// Ensure interfaces are satisfied.
var _ fs.NodeSymlinker = (*FSNode)(nil)
var _ fs.NodeReadlinker = (*FSNode)(nil)
