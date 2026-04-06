// Package afsfs implements a FUSE filesystem backed by Redis FS.* commands.
package afsfs

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/rowantrollope/agent-filesystem/mount/internal/cache"
	"github.com/rowantrollope/agent-filesystem/mount/internal/client"
)

// Options configures the FUSE mount.
type Options struct {
	AttrTimeout time.Duration
	ReadOnly    bool
	AllowOther  bool
	Debug       bool
	UID         uint32
	GID         uint32
}

// FSRoot is the root of the FUSE filesystem.
type FSRoot struct {
	FSNode
}

// FSNode represents a node (file, directory, or symlink) in the filesystem.
type FSNode struct {
	fs.Inode

	client    client.Client
	attrCache *cache.Cache
	dirCache  *cache.Cache
	opts      *Options
	fsPath    string // absolute path in AFS mount storage (e.g. "/", "/foo/bar")
}

// root returns the FSRoot from any node.
func (n *FSNode) root() *FSRoot {
	return n.Root().Operations().(*FSRoot)
}

// invalidatePath invalidates caches for a path and its parent directory.
func (r *FSRoot) invalidatePath(path string) {
	r.attrCache.Invalidate(path)
	parent := filepath.Dir(path)
	r.dirCache.Invalidate(parent)
	r.attrCache.Invalidate(parent)
}

// invalidatePathPrefix invalidates caches for a subtree and its parent directory.
func (r *FSRoot) invalidatePathPrefix(path string) {
	r.attrCache.InvalidatePrefix(path)
	r.dirCache.InvalidatePrefix(path)
	r.invalidatePath(path)
}

// newChild creates a child FSNode for the given basename.
func (n *FSNode) newChild(name string) *FSNode {
	childPath := n.fsPath + "/" + name
	if n.fsPath == "/" {
		childPath = "/" + name
	}
	return &FSNode{
		client:    n.client,
		attrCache: n.attrCache,
		dirCache:  n.dirCache,
		opts:      n.opts,
		fsPath:    childPath,
	}
}

// Mount mounts the AFS filesystem at the given mountpoint.
func Mount(mountpoint string, c client.Client, opts *Options) (*fuse.Server, error) {
	if opts.AttrTimeout == 0 {
		opts.AttrTimeout = time.Second
	}

	attrCache := cache.New(opts.AttrTimeout)
	dirCache := cache.New(opts.AttrTimeout)

	root := &FSRoot{
		FSNode: FSNode{
			client:    c,
			attrCache: attrCache,
			dirCache:  dirCache,
			opts:      opts,
			fsPath:    "/",
		},
	}

	fuseOpts := &fs.Options{
		MountOptions: fuse.MountOptions{
			AllowOther: opts.AllowOther,
			FsName:     "agent-filesystem",
			Name:       "agent-filesystem",
			Debug:      opts.Debug,
		},
		EntryTimeout: &opts.AttrTimeout,
		AttrTimeout:  &opts.AttrTimeout,

		UID: opts.UID,
		GID: opts.GID,
	}

	if opts.ReadOnly {
		fuseOpts.MountOptions.Options = append(fuseOpts.MountOptions.Options, "ro")
	}

	server, err := fs.Mount(mountpoint, root, fuseOpts)
	if err != nil {
		return nil, err
	}
	return server, nil
}

// Statfs implements fs.NodeStatfser.
func (n *FSNode) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	info, err := n.client.Info(ctx)
	if err != nil {
		log.Printf("Statfs error: %v", err)
		return syscall.EIO
	}

	const blockSize = 4096
	totalBlocks := uint64(info.TotalDataBytes+blockSize-1) / blockSize
	if totalBlocks < 1024 {
		totalBlocks = 1024
	}

	out.Bsize = blockSize
	out.Frsize = blockSize
	out.Blocks = totalBlocks * 10 // report 10x used as total
	out.Bfree = totalBlocks * 9
	out.Bavail = totalBlocks * 9
	out.Files = uint64(info.TotalInodes)
	out.Ffree = 1000000
	out.NameLen = 255
	return 0
}

// Getattr implements fs.NodeGetattrer.
func (n *FSNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	// Check cache first.
	if cached, ok := n.attrCache.Get(n.fsPath); ok {
		out.Attr = cached.(fuse.Attr)
		out.SetTimeout(n.opts.AttrTimeout)
		return 0
	}

	st, err := n.client.Stat(ctx, n.fsPath)
	if err != nil {
		return mapError(err)
	}
	if st == nil {
		return syscall.ENOENT
	}

	attr := statToAttr(st)
	n.attrCache.Set(n.fsPath, attr)
	out.Attr = attr
	out.SetTimeout(n.opts.AttrTimeout)
	return 0
}

// Setattr implements fs.NodeSetattrer.
func (n *FSNode) Setattr(ctx context.Context, fh fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	if n.opts.ReadOnly {
		return syscall.EROFS
	}

	// Handle truncate.
	if sz, ok := in.GetSize(); ok {
		if err := n.client.Truncate(ctx, n.fsPath, int64(sz)); err != nil {
			return mapError(err)
		}
	}

	// Handle mode change.
	if mode, ok := in.GetMode(); ok {
		if err := n.client.Chmod(ctx, n.fsPath, mode&07777); err != nil {
			return mapError(err)
		}
	}

	// Handle uid/gid change.
	uid, uidOk := in.GetUID()
	gid, gidOk := in.GetGID()
	if uidOk || gidOk {
		st, err := n.client.Stat(ctx, n.fsPath)
		if err != nil {
			return mapError(err)
		}
		if st == nil {
			return syscall.ENOENT
		}
		newUID := st.UID
		newGID := st.GID
		if uidOk {
			newUID = uid
		}
		if gidOk {
			newGID = gid
		}
		if err := n.client.Chown(ctx, n.fsPath, newUID, newGID); err != nil {
			return mapError(err)
		}
	}

	// Handle atime/mtime.
	atime, atimeOk := in.GetATime()
	mtime, mtimeOk := in.GetMTime()
	if atimeOk || mtimeOk {
		atimeMs := int64(-1)
		mtimeMs := int64(-1)
		if atimeOk {
			atimeMs = atime.UnixNano() / 1_000_000
		}
		if mtimeOk {
			mtimeMs = mtime.UnixNano() / 1_000_000
		}
		if err := n.client.Utimens(ctx, n.fsPath, atimeMs, mtimeMs); err != nil {
			return mapError(err)
		}
	}

	n.attrCache.Invalidate(n.fsPath)

	return n.Getattr(ctx, fh, out)
}

// GetOwnership returns the uid/gid to use. Defaults come from opts.
func GetOwnership() (uint32, uint32) {
	return uint32(os.Getuid()), uint32(os.Getgid())
}

// parentPath returns the parent dir of a path.
func parentPath(p string) string {
	if p == "/" {
		return "/"
	}
	parent := filepath.Dir(p)
	if parent == "." {
		return "/"
	}
	return parent
}

// baseName returns the last component of a path.
func baseName(p string) string {
	if p == "/" {
		return ""
	}
	parts := strings.Split(p, "/")
	return parts[len(parts)-1]
}

// Ensure interfaces are satisfied.
var _ fs.NodeStatfser = (*FSNode)(nil)
var _ fs.NodeGetattrer = (*FSNode)(nil)
var _ fs.NodeSetattrer = (*FSNode)(nil)
