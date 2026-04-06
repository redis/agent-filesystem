package afsfs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/rowantrollope/agent-filesystem/mount/internal/client"
)

// Lookup implements fs.NodeLookuper.
func (n *FSNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	child := n.newChild(name)

	// Check attr cache.
	if cached, ok := n.attrCache.Get(child.fsPath); ok {
		out.Attr = cached.(fuse.Attr)
		out.SetEntryTimeout(n.opts.AttrTimeout)
		out.SetAttrTimeout(n.opts.AttrTimeout)
		node := n.NewInode(ctx, child, fs.StableAttr{Mode: out.Attr.Mode & syscall.S_IFMT, Ino: out.Attr.Ino})
		return node, 0
	}

	st, err := n.client.Stat(ctx, child.fsPath)
	if err != nil {
		return nil, mapError(err)
	}
	if st == nil {
		return nil, syscall.ENOENT
	}

	attr := statToAttr(st)
	n.attrCache.Set(child.fsPath, attr)

	out.Attr = attr
	out.SetEntryTimeout(n.opts.AttrTimeout)
	out.SetAttrTimeout(n.opts.AttrTimeout)

	node := n.NewInode(ctx, child, fs.StableAttr{Mode: attr.Mode & syscall.S_IFMT, Ino: st.Inode})
	return node, 0
}

// Readdir implements fs.NodeReaddirer.
func (n *FSNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	// Check dir cache.
	if cached, ok := n.dirCache.Get(n.fsPath); ok {
		return fs.NewListDirStream(cached.([]fuse.DirEntry)), 0
	}

	entries, err := n.client.LsLong(ctx, n.fsPath)
	if err != nil {
		return nil, mapError(err)
	}

	result := make([]fuse.DirEntry, 0, len(entries))
	for _, e := range entries {
		var mode uint32
		switch e.Type {
		case "file":
			mode = syscall.S_IFREG
		case "dir":
			mode = syscall.S_IFDIR
		case "symlink":
			mode = syscall.S_IFLNK
		}
		result = append(result, fuse.DirEntry{
			Name: e.Name,
			Mode: mode,
			Ino:  e.Inode,
		})

		// Pre-populate attr cache from the long listing.
		childPath := n.fsPath + "/" + e.Name
		if n.fsPath == "/" {
			childPath = "/" + e.Name
		}
		n.attrCache.Set(childPath, lsEntryToAttr(&e))
	}

	n.dirCache.Set(n.fsPath, result)
	return fs.NewListDirStream(result), 0
}

// lsEntryToAttr converts an LsEntry to fuse.Attr (partial — only has mtime, mode, size).
func lsEntryToAttr(e *client.LsEntry) fuse.Attr {
	var mode uint32
	switch e.Type {
	case "file":
		mode = syscall.S_IFREG | e.Mode
	case "dir":
		mode = syscall.S_IFDIR | e.Mode
	case "symlink":
		mode = syscall.S_IFLNK | e.Mode
	}

	var nlink uint32 = 1
	if e.Type == "dir" {
		nlink = 2
	}

	size := uint64(e.Size)
	if e.Type == "dir" {
		size = 4096
	}

	return fuse.Attr{
		Ino:       e.Inode,
		Mode:      mode,
		Nlink:     nlink,
		Size:      size,
		Owner:     fuse.Owner{Uid: e.UID, Gid: e.GID},
		Mtime:     uint64(e.Mtime / 1000),
		Mtimensec: uint32((e.Mtime % 1000) * 1_000_000),
		Blocks:    (size + 511) / 512,
	}
}

// Mkdir implements fs.NodeMkdirer.
func (n *FSNode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if n.opts.ReadOnly {
		return nil, syscall.EROFS
	}

	child := n.newChild(name)

	if err := n.client.Mkdir(ctx, child.fsPath); err != nil {
		return nil, mapError(err)
	}

	if mode != 0 {
		_ = n.client.Chmod(ctx, child.fsPath, mode&07777)
	}

	n.root().invalidatePath(child.fsPath)

	// Fetch the attr for the new dir.
	st, err := n.client.Stat(ctx, child.fsPath)
	if err != nil {
		return nil, mapError(err)
	}
	attr := statToAttr(st)
	out.Attr = attr
	out.SetEntryTimeout(n.opts.AttrTimeout)
	out.SetAttrTimeout(n.opts.AttrTimeout)

	node := n.NewInode(ctx, child, fs.StableAttr{Mode: syscall.S_IFDIR, Ino: st.Inode})
	return node, 0
}

// Rmdir implements fs.NodeRmdirer.
func (n *FSNode) Rmdir(ctx context.Context, name string) syscall.Errno {
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

// Rename implements fs.NodeRenamer.
func (n *FSNode) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	if n.opts.ReadOnly {
		return syscall.EROFS
	}

	oldPath := n.newChild(name).fsPath
	var newParentNode *FSNode
	switch p := newParent.(type) {
	case *FSNode:
		newParentNode = p
	case *FSRoot:
		newParentNode = &p.FSNode
	default:
		return syscall.EIO
	}
	newPath := newParentNode.newChild(newName).fsPath

	if err := n.client.Rename(ctx, oldPath, newPath, flags); err != nil {
		return mapError(err)
	}

	n.root().invalidatePathPrefix(oldPath)
	n.root().invalidatePathPrefix(newPath)
	return 0
}

// Ensure interfaces are satisfied.
var _ fs.NodeLookuper = (*FSNode)(nil)
var _ fs.NodeReaddirer = (*FSNode)(nil)
var _ fs.NodeMkdirer = (*FSNode)(nil)
var _ fs.NodeRmdirer = (*FSNode)(nil)
var _ fs.NodeRenamer = (*FSNode)(nil)
