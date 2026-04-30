// Package afsfs implements a FUSE filesystem backed by the native Redis
// workspace client.
package afsfs

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/redis/agent-filesystem/mount/internal/cache"
	"github.com/redis/agent-filesystem/mount/internal/client"
)

// Options configures the FUSE mount.
type Options struct {
	AttrTimeout time.Duration
	ReadOnly    bool
	AllowOther  bool
	Debug       bool
	UID         uint32
	GID         uint32
	// DisableCrossClientInvalidation skips starting the pub/sub subscriber
	// and tells the client to stop publishing cache invalidations. Mounts
	// then only see each other's writes after their TTL expires.
	DisableCrossClientInvalidation bool
}

// FSRoot is the root of the FUSE filesystem.
type FSRoot struct {
	FSNode
	// server is populated by Mount() after fs.Mount returns. It's needed
	// by the invalidation subscriber to issue kernel notifications. Reads
	// use atomic.Pointer because the subscriber goroutine may run before
	// the handshake between Mount and the first request completes.
	server atomic.Pointer[fuse.Server]
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
//
// ctx, when non-nil, controls the lifetime of the cross-client invalidation
// subscriber: cancel it to stop the subscriber goroutine (e.g. during
// shutdown). Passing a nil ctx uses context.Background() and relies on the
// process exiting to tear the subscriber down.
func Mount(ctx context.Context, mountpoint string, c client.Client, opts *Options) (*fuse.Server, error) {
	if opts.AttrTimeout == 0 {
		opts.AttrTimeout = time.Second
	}
	if ctx == nil {
		ctx = context.Background()
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
	root.server.Store(server)

	if opts.DisableCrossClientInvalidation {
		c.DisableInvalidationPublishing()
	} else {
		if err := c.SubscribeInvalidations(ctx, root.handleInvalidation); err != nil {
			// Non-fatal: log and fall back to TTL-based consistency.
			log.Printf("afs: failed to start invalidation subscriber: %v", err)
		}
	}

	return server, nil
}

// handleInvalidation is the subscriber callback. It drops matching entries
// from the FUSE-layer attrCache/dirCache and pushes kernel-level notifies
// through go-fuse so the in-kernel dentry and page caches forget the stale
// data. Messages originating from this client have already been filtered out
// upstream, so every call here is a peer's mutation.
func (r *FSRoot) handleInvalidation(ev client.InvalidateEvent) {
	for _, p := range ev.Paths {
		if p == "" {
			continue
		}
		switch ev.Op {
		case client.InvalidateOpInode:
			r.invalidatePath(p)
			r.notifyEntryChange(p)
		case client.InvalidateOpDir:
			r.dirCache.Invalidate(p)
			// Don't touch the kernel here: afsfs Readdir will re-populate
			// from the fresh LsLong on next READDIR, and go-fuse has no
			// "invalidate the listing only" operation.
		case client.InvalidateOpPrefix, client.InvalidateOpRootReplace:
			r.invalidatePathPrefix(p)
			r.notifyPrefixChange(p)
		case client.InvalidateOpContent:
			r.invalidatePath(p)
			r.notifyContentChange(p)
		}
	}
}

// notifyEntryChange tells the kernel to forget the cached dentry for path's
// basename under its parent directory. Used for create, delete, metadata
// changes — anything that changes what a Lookup would return.
func (r *FSRoot) notifyEntryChange(path string) {
	if path == "/" {
		return
	}
	parent, name := splitParent(path)
	parentInode := r.findInode(parent)
	if parentInode == nil {
		// Kernel never saw this parent; nothing to invalidate.
		return
	}
	_ = parentInode.NotifyEntry(name)
}

// notifyPrefixChange walks the kernel-known inodes under path and issues
// NotifyEntry on each. Paths the kernel doesn't know about are silently
// skipped (GetChild returns nil).
func (r *FSRoot) notifyPrefixChange(prefix string) {
	// First, invalidate the entry for the prefix itself.
	r.notifyEntryChange(prefix)

	// Then walk the subtree. We bound this to nodes currently known to the
	// kernel: unknown descendants return nil from GetChild and we stop.
	root := r.findInode(prefix)
	if root == nil {
		return
	}
	r.walkNotifyEntries(root)
}

func (r *FSRoot) walkNotifyEntries(n *fs.Inode) {
	for name, child := range n.Children() {
		_ = n.NotifyEntry(name)
		if child != nil {
			r.walkNotifyEntries(child)
		}
	}
}

// notifyContentChange tells the kernel to drop cached bytes for the file at
// path. Full-file invalidation for v1.
func (r *FSRoot) notifyContentChange(path string) {
	node := r.findInode(path)
	if node == nil {
		return
	}
	_ = node.NotifyContent(0, -1)
	// Also drop the dentry so the next Getattr re-reads size/mtime.
	r.notifyEntryChange(path)
}

// findInode walks the live go-fuse Inode tree from the root down, returning
// the Inode for path or nil if the kernel doesn't have it cached. This is a
// read-only walk: it never creates new Inodes.
func (r *FSRoot) findInode(path string) *fs.Inode {
	node := &r.Inode
	if path == "" || path == "/" {
		return node
	}
	trimmed := strings.TrimPrefix(path, "/")
	for _, comp := range strings.Split(trimmed, "/") {
		if comp == "" {
			continue
		}
		child := node.GetChild(comp)
		if child == nil {
			return nil
		}
		node = child
	}
	return node
}

// splitParent returns (parent dir, basename) for an absolute path. For "/foo"
// it returns ("/", "foo"). For "/" it returns ("/", "").
func splitParent(path string) (string, string) {
	if path == "/" || path == "" {
		return "/", ""
	}
	parent := filepath.Dir(path)
	name := filepath.Base(path)
	if parent == "." {
		parent = "/"
	}
	return parent, name
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
