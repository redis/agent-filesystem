package redisfs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Create implements fs.NodeCreater.
func (n *FSNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (inode *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if n.opts.ReadOnly {
		return nil, nil, 0, syscall.EROFS
	}

	child := n.newChild(name)

	if err := n.client.Touch(ctx, child.fsPath); err != nil {
		return nil, nil, 0, mapError(err)
	}

	if mode != 0 {
		_ = n.client.Chmod(ctx, child.fsPath, mode&07777)
	}

	n.root().invalidatePath(child.fsPath)

	st, err := n.client.Stat(ctx, child.fsPath)
	if err != nil {
		return nil, nil, 0, mapError(err)
	}
	attr := statToAttr(st, n.opts.UID, n.opts.GID)
	out.Attr = attr
	out.SetEntryTimeout(n.opts.AttrTimeout)
	out.SetAttrTimeout(n.opts.AttrTimeout)

	node := n.NewInode(ctx, child, fs.StableAttr{Mode: syscall.S_IFREG})

	handle := newFileHandle(child.fsPath, n.client, child)
	if flags&syscall.O_TRUNC != 0 {
		handle.SetTruncated()
	}

	return node, handle, 0, 0
}

// Open implements fs.NodeOpener.
func (n *FSNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	if n.opts.ReadOnly && (flags&(syscall.O_WRONLY|syscall.O_RDWR)) != 0 {
		return nil, 0, syscall.EROFS
	}

	handle := newFileHandle(n.fsPath, n.client, n)

	if flags&syscall.O_TRUNC != 0 {
		handle.SetTruncated()
	}

	return handle, 0, 0
}

// Read implements fs.NodeReader.
func (n *FSNode) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	if h, ok := fh.(*FileHandle); ok {
		return h.Read(ctx, dest, off)
	}

	// Fallback: direct read without handle.
	data, err := n.client.Cat(ctx, n.fsPath)
	if err != nil {
		return nil, mapError(err)
	}

	size := int64(len(data))
	if off >= size {
		return fuse.ReadResultData(nil), 0
	}
	end := off + int64(len(dest))
	if end > size {
		end = size
	}
	return fuse.ReadResultData(data[off:end]), 0
}

// Write implements fs.NodeWriter.
func (n *FSNode) Write(ctx context.Context, fh fs.FileHandle, data []byte, off int64) (uint32, syscall.Errno) {
	if n.opts.ReadOnly {
		return 0, syscall.EROFS
	}

	if h, ok := fh.(*FileHandle); ok {
		return h.Write(ctx, data, off)
	}

	return 0, syscall.EIO
}

// Fsync implements fs.NodeFsyncer.
func (n *FSNode) Fsync(ctx context.Context, fh fs.FileHandle, flags uint32) syscall.Errno {
	if h, ok := fh.(*FileHandle); ok {
		return h.Flush(ctx)
	}
	return 0
}

// Flush implements fs.NodeFlusher.
func (n *FSNode) Flush(ctx context.Context, fh fs.FileHandle) syscall.Errno {
	if h, ok := fh.(*FileHandle); ok {
		return h.Flush(ctx)
	}
	return 0
}

// Release implements fs.NodeReleaser.
func (n *FSNode) Release(ctx context.Context, fh fs.FileHandle) syscall.Errno {
	if h, ok := fh.(*FileHandle); ok {
		return h.Flush(ctx)
	}
	return 0
}

// Unlink implements fs.NodeUnlinker.
func (n *FSNode) Unlink(ctx context.Context, name string) syscall.Errno {
	if n.opts.ReadOnly {
		return syscall.EROFS
	}

	child := n.newChild(name)
	if err := n.client.Rm(ctx, child.fsPath); err != nil {
		return mapError(err)
	}

	n.root().invalidatePath(child.fsPath)
	return 0
}

// Link implements fs.NodeLinker — returns ENOTSUP (no hard links in Redis-FS).
func (n *FSNode) Link(ctx context.Context, target fs.InodeEmbedder, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	return nil, syscall.ENOTSUP
}

// Getxattr implements fs.NodeGetxattrer — returns ENODATA (no xattr support).
func (n *FSNode) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	return 0, syscall.ENODATA
}

// Setxattr implements fs.NodeSetxattrer — returns ENOTSUP.
func (n *FSNode) Setxattr(ctx context.Context, attr string, data []byte, flags uint32) syscall.Errno {
	return syscall.ENOTSUP
}

// Listxattr implements fs.NodeListxattrer — returns empty.
func (n *FSNode) Listxattr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	return 0, 0
}

// Ensure interfaces are satisfied.
var _ fs.NodeCreater = (*FSNode)(nil)
var _ fs.NodeOpener = (*FSNode)(nil)
var _ fs.NodeReader = (*FSNode)(nil)
var _ fs.NodeWriter = (*FSNode)(nil)
var _ fs.NodeFsyncer = (*FSNode)(nil)
var _ fs.NodeFlusher = (*FSNode)(nil)
var _ fs.NodeReleaser = (*FSNode)(nil)
var _ fs.NodeUnlinker = (*FSNode)(nil)
var _ fs.NodeLinker = (*FSNode)(nil)
var _ fs.NodeGetxattrer = (*FSNode)(nil)
var _ fs.NodeSetxattrer = (*FSNode)(nil)
var _ fs.NodeListxattrer = (*FSNode)(nil)
