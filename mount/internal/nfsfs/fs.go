package nfsfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/redis/agent-filesystem/mount/internal/client"
	"github.com/redis/go-redis/v9"
)

var _ billy.Filesystem = (*FS)(nil)
var _ billy.Change = (*FS)(nil)

type FS struct {
	client   client.Client
	readOnly bool
	debug    bool
	// shadow holds AppleDouble "._*" sidecar files in memory. They are not
	// persisted to Redis — see appledouble.go for the rationale.
	shadow *shadowStore
}

func New(c client.Client, readOnly bool) *FS {
	return &FS{
		client:   c,
		readOnly: readOnly,
		debug:    os.Getenv("AFS_NFS_DEBUG") == "1",
		shadow:   newShadowStore(),
	}
}

func (f *FS) withTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// openShadow is the OpenFile fast path for AppleDouble "._*" sidecar
// files. It honors O_CREATE / O_EXCL / O_TRUNC semantics against the
// in-memory shadow store instead of reaching Redis.
func (f *FS) openShadow(p string, flag int, perm os.FileMode) (billy.File, error) {
	existing := f.shadow.get(p)
	if existing == nil {
		if flag&os.O_CREATE == 0 {
			return nil, os.ErrNotExist
		}
		if f.readOnly {
			return nil, os.ErrPermission
		}
		file, _ := f.shadow.getOrCreate(p, perm.Perm())
		return &shadowHandle{path: p, store: f.shadow, file: file}, nil
	}
	if flag&os.O_EXCL != 0 && flag&os.O_CREATE != 0 {
		return nil, os.ErrExist
	}
	if flag&os.O_TRUNC != 0 {
		if f.readOnly {
			return nil, os.ErrPermission
		}
		existing.mu.Lock()
		existing.content = existing.content[:0]
		existing.mtime = time.Now()
		existing.mu.Unlock()
	}
	return &shadowHandle{path: p, store: f.shadow, file: existing}, nil
}

func (f *FS) normalize(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	clean := path.Clean(p)
	if clean == "." {
		return "/"
	}
	return clean
}

func (f *FS) Open(filename string) (billy.File, error) {
	return f.OpenFile(filename, os.O_RDONLY, 0)
}

func (f *FS) debugf(format string, args ...interface{}) {
	if !f.debug {
		return
	}
	log.Printf("nfsfs: "+format, args...)
}

func (f *FS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	p := f.normalize(filename)
	f.debugf("OpenFile path=%q flag=%#x perm=%#o", p, flag, perm.Perm())
	if isAppleDoublePath(p) {
		return f.openShadow(p, flag, perm)
	}
	ctx, cancel := f.withTimeout()
	defer cancel()

	st, err := f.client.Stat(ctx, p)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			err = os.ErrNotExist
		}
		return nil, err
	}
	missing := st == nil
	existedBeforeOpen := !missing
	if missing {
		if flag&os.O_CREATE == 0 {
			return nil, os.ErrNotExist
		}
		if f.readOnly {
			return nil, os.ErrPermission
		}
		created, _, err := f.client.CreateFile(ctx, p, uint32(perm.Perm()), flag&os.O_EXCL != 0)
		if err != nil {
			if errors.Is(err, client.ErrAlreadyExists) {
				return nil, os.ErrExist
			}
			return nil, err
		}
		st = created
		missing = false
	}

	if st.Type == "dir" {
		return nil, fmt.Errorf("%s is a directory", p)
	}
	if flag&os.O_EXCL != 0 && flag&os.O_CREATE != 0 && existedBeforeOpen {
		return nil, os.ErrExist
	}

	if flag&os.O_TRUNC != 0 {
		if f.readOnly {
			return nil, os.ErrPermission
		}
		f.debugf("OpenFile truncating inode=%d path=%q", st.Inode, p)
		if err := f.client.TruncateInodeAtPath(ctx, st.Inode, p, 0); err != nil {
			return nil, err
		}
		st.Size = 0
	}

	fh := &fileHandle{
		inode:    st.Inode,
		size:     st.Size,
		fs:       f,
		path:     p,
		writable: flag&(os.O_WRONLY|os.O_RDWR) != 0 || flag&(os.O_CREATE|os.O_APPEND|os.O_TRUNC) != 0,
		append:   flag&os.O_APPEND != 0,
	}
	if fh.append {
		fh.pos = st.Size
	}
	return fh, nil
}

func (f *FS) Create(filename string) (billy.File, error) {
	return f.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

func (f *FS) Stat(filename string) (os.FileInfo, error) {
	p := f.normalize(filename)
	if isAppleDoublePath(p) {
		if sf := f.shadow.get(p); sf != nil {
			return shadowFileInfo{name: path.Base(p), file: sf}, nil
		}
		return nil, os.ErrNotExist
	}
	ctx, cancel := f.withTimeout()
	defer cancel()
	st, err := f.client.Stat(ctx, p)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	if st == nil {
		return nil, os.ErrNotExist
	}
	return newFileInfo(path.Base(p), st), nil
}

func (f *FS) Lstat(filename string) (os.FileInfo, error) {
	return f.Stat(filename)
}

func (f *FS) Rename(oldpath, newpath string) error {
	if f.readOnly {
		return os.ErrPermission
	}
	oldN := f.normalize(oldpath)
	newN := f.normalize(newpath)
	oldShadow := isAppleDoublePath(oldN)
	newShadow := isAppleDoublePath(newN)
	if oldShadow && newShadow {
		if !f.shadow.rename(oldN, newN) {
			return os.ErrNotExist
		}
		return nil
	}
	if oldShadow || newShadow {
		// Crossing between shadow and real world is a macOS-side quirk we
		// refuse; it never occurs in normal usage.
		return os.ErrPermission
	}
	ctx, cancel := f.withTimeout()
	defer cancel()
	return f.client.Rename(ctx, oldN, newN, 0)
}

func (f *FS) Remove(filename string) error {
	if f.readOnly {
		return os.ErrPermission
	}
	p := f.normalize(filename)
	if isAppleDoublePath(p) {
		if !f.shadow.remove(p) {
			return os.ErrNotExist
		}
		return nil
	}
	ctx, cancel := f.withTimeout()
	defer cancel()
	return f.client.Rm(ctx, p)
}

func (f *FS) Join(elem ...string) string {
	if len(elem) == 0 {
		return "/"
	}
	return f.normalize(path.Join(elem...))
}

func (f *FS) TempFile(dir, prefix string) (billy.File, error) {
	base := f.normalize(dir)
	name := fmt.Sprintf("%s-%d.tmp", prefix, time.Now().UnixNano())
	if base == "/" {
		return f.Create("/" + name)
	}
	return f.Create(base + "/" + name)
}

func (f *FS) ReadDir(p string) ([]os.FileInfo, error) {
	dir := f.normalize(p)
	ctx, cancel := f.withTimeout()
	defer cancel()
	entries, err := f.client.LsLong(ctx, dir)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	out := make([]os.FileInfo, 0, len(entries))
	for _, entry := range entries {
		st := &client.StatResult{
			Inode: entry.Inode,
			Type:  entry.Type,
			Mode:  entry.Mode,
			UID:   entry.UID,
			GID:   entry.GID,
			Size:  entry.Size,
			Mtime: entry.Mtime,
		}
		out = append(out, newFileInfo(entry.Name, st))
	}
	// Do NOT merge AppleDouble shadow entries into READDIR. macOS accesses
	// sidecars via direct LOOKUP/OPEN (the shadow handles those just fine)
	// and hiding them from READDIR matches the semantics of a native Apple
	// filesystem, where "._*" files are invisible to directory enumeration.
	// As a bonus, it avoids a TOCTOU race with tools like rsync that enumerate
	// a directory then stat each entry: the shadow store mutates during
	// live Claude Code sessions, and a shadow file could be evicted between
	// the ReadDir snapshot and the follow-up Stat.
	return out, nil
}

func (f *FS) MkdirAll(filename string, perm os.FileMode) error {
	if f.readOnly {
		return os.ErrPermission
	}
	p := f.normalize(filename)
	ctx, cancel := f.withTimeout()
	defer cancel()
	if err := f.client.Mkdir(ctx, p); err != nil {
		return err
	}
	// createDirNoParents defaults to 0o755; skip the Chmod round-trip when it matches.
	if perm.Perm() != 0o755 {
		return f.client.Chmod(ctx, p, uint32(perm.Perm()))
	}
	return nil
}

func (f *FS) Readlink(link string) (string, error) {
	ctx, cancel := f.withTimeout()
	defer cancel()
	return f.client.Readlink(ctx, f.normalize(link))
}

func (f *FS) Symlink(target, link string) error {
	if f.readOnly {
		return os.ErrPermission
	}
	ctx, cancel := f.withTimeout()
	defer cancel()
	return f.client.Ln(ctx, target, f.normalize(link))
}

func (f *FS) Chroot(string) (billy.Filesystem, error) {
	return nil, errors.New("chroot is not supported")
}

func (f *FS) Root() string { return "/" }

func (f *FS) Chmod(name string, mode os.FileMode) error {
	if f.readOnly {
		return os.ErrPermission
	}
	p := f.normalize(name)
	if isAppleDoublePath(p) {
		if sf := f.shadow.get(p); sf != nil {
			sf.mu.Lock()
			sf.mode = mode
			sf.ctime = time.Now()
			sf.mu.Unlock()
			return nil
		}
		return os.ErrNotExist
	}
	ctx, cancel := f.withTimeout()
	defer cancel()
	return f.client.Chmod(ctx, p, uint32(mode.Perm()))
}

func (f *FS) Lchown(name string, uid, gid int) error {
	// Agent Filesystem stores ownership per path; symlink-vs-target semantics are not distinguished.
	return f.Chown(name, uid, gid)
}

func (f *FS) Chown(name string, uid, gid int) error {
	if f.readOnly {
		return os.ErrPermission
	}
	p := f.normalize(name)
	if isAppleDoublePath(p) {
		// Shadow files do not track uid/gid; treat as a no-op success.
		if f.shadow.get(p) != nil {
			return nil
		}
		return os.ErrNotExist
	}
	ctx, cancel := f.withTimeout()
	defer cancel()
	return f.client.Chown(ctx, p, uint32(uid), uint32(gid))
}

func (f *FS) Chtimes(name string, atime time.Time, mtime time.Time) error {
	if f.readOnly {
		return os.ErrPermission
	}
	p := f.normalize(name)
	if isAppleDoublePath(p) {
		if sf := f.shadow.get(p); sf != nil {
			sf.mu.Lock()
			sf.atime = atime
			sf.mtime = mtime
			sf.mu.Unlock()
			return nil
		}
		return os.ErrNotExist
	}
	ctx, cancel := f.withTimeout()
	defer cancel()
	return f.client.Utimens(ctx, p, atime.UnixMilli(), mtime.UnixMilli())
}

// SetAttrs is the batched counterpart of Chmod / Chown / Chtimes used by the
// go-nfs SETATTR and CREATE fast paths. It satisfies third_party/go-nfs's
// optional BatchSetAttrer interface via structural typing — the go-nfs
// package uses a type assertion on billy.Change to detect this method and
// dispatch a single client round trip instead of a sequential Chmod+Lchown+
// Chtimes triple. See third_party/go-nfs/file.go SetFileAttributes.Apply.
//
// Nil pointers mean "do not change this field." An all-nil call returns nil
// immediately after the shadow / readOnly checks.
func (f *FS) SetAttrs(
	name string,
	mode *os.FileMode,
	uid *int,
	gid *int,
	atime *time.Time,
	mtime *time.Time,
) error {
	if f.readOnly {
		return os.ErrPermission
	}
	p := f.normalize(name)
	if isAppleDoublePath(p) {
		sf := f.shadow.get(p)
		if sf == nil {
			return os.ErrNotExist
		}
		sf.mu.Lock()
		defer sf.mu.Unlock()
		if mode != nil {
			sf.mode = *mode
			sf.ctime = time.Now()
		}
		// uid/gid intentionally ignored for shadow files (see Chown above).
		if atime != nil {
			sf.atime = *atime
		}
		if mtime != nil {
			sf.mtime = *mtime
		}
		return nil
	}

	var upd client.AttrUpdate
	if mode != nil {
		m := uint32(mode.Perm())
		upd.Mode = &m
	}
	if uid != nil {
		u := uint32(*uid)
		upd.UID = &u
	}
	if gid != nil {
		g := uint32(*gid)
		upd.GID = &g
	}
	if atime != nil {
		ms := atime.UnixMilli()
		upd.AtimeMs = &ms
	}
	if mtime != nil {
		ms := mtime.UnixMilli()
		upd.MtimeMs = &ms
	}
	if upd.IsEmpty() {
		return nil
	}
	ctx, cancel := f.withTimeout()
	defer cancel()
	return f.client.SetAttrs(ctx, p, upd)
}

type fileInfo struct {
	name string
	st   *client.StatResult
}

func newFileInfo(name string, st *client.StatResult) os.FileInfo {
	return fileInfo{name: name, st: st}
}

func (fi fileInfo) Name() string { return fi.name }
func (fi fileInfo) Size() int64  { return fi.st.Size }
func (fi fileInfo) Mode() os.FileMode {
	mode := os.FileMode(fi.st.Mode & 0o777)
	switch fi.st.Type {
	case "dir":
		mode |= os.ModeDir
	case "symlink":
		mode |= os.ModeSymlink
	}
	return mode
}
func (fi fileInfo) ModTime() time.Time { return time.UnixMilli(fi.st.Mtime) }
func (fi fileInfo) IsDir() bool        { return fi.st.Type == "dir" }
func (fi fileInfo) Sys() interface{} {
	stat := &syscall.Stat_t{
		Ino: fi.st.Inode,
		Uid: fi.st.UID,
		Gid: fi.st.GID,
	}
	if fi.st.Type == "dir" {
		stat.Nlink = 2
	} else {
		stat.Nlink = 1
	}
	return stat
}

type fileHandle struct {
	mu       sync.Mutex
	fs       *FS
	path     string
	inode    uint64
	size     int64
	pos      int64
	writable bool
	append   bool
	closed   bool
}

func (fh *fileHandle) Name() string { return fh.path }

func (fh *fileHandle) ensureOpen() error {
	if fh.closed {
		return os.ErrClosed
	}
	return nil
}

func (fh *fileHandle) Read(p []byte) (int, error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	if err := fh.ensureOpen(); err != nil {
		return 0, err
	}
	ctx, cancel := fh.fs.withTimeout()
	defer cancel()
	data, err := fh.fs.client.ReadInodeAt(ctx, fh.inode, fh.pos, len(p))
	if err != nil {
		return 0, err
	}
	fh.fs.debugf("Read inode=%d path=%q off=%d size=%d -> %d bytes", fh.inode, fh.path, fh.pos, len(p), len(data))
	if len(data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, data)
	fh.pos += int64(n)
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (fh *fileHandle) ReadAt(p []byte, off int64) (int, error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	if err := fh.ensureOpen(); err != nil {
		return 0, err
	}
	ctx, cancel := fh.fs.withTimeout()
	defer cancel()
	data, err := fh.fs.client.ReadInodeAt(ctx, fh.inode, off, len(p))
	if err != nil {
		return 0, err
	}
	fh.fs.debugf("ReadAt inode=%d path=%q off=%d size=%d -> %d bytes", fh.inode, fh.path, off, len(p), len(data))
	if len(data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, data)
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (fh *fileHandle) Write(p []byte) (int, error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	if err := fh.ensureOpen(); err != nil {
		return 0, err
	}
	if !fh.writable || fh.fs.readOnly {
		return 0, os.ErrPermission
	}
	if fh.append {
		ctx, cancel := fh.fs.withTimeout()
		defer cancel()
		st, err := fh.fs.client.StatInode(ctx, fh.inode)
		if err != nil {
			return 0, err
		}
		if st == nil {
			return 0, os.ErrNotExist
		}
		fh.size = st.Size
		fh.pos = st.Size
	}
	ctx, cancel := fh.fs.withTimeout()
	defer cancel()
	fh.fs.debugf("Write inode=%d path=%q off=%d len=%d", fh.inode, fh.path, fh.pos, len(p))
	// Pass the known path so the client can update the attribute cache in
	// place and avoid wiping the warm path cache on every write.
	if err := fh.fs.client.WriteInodeAtPath(ctx, fh.inode, fh.path, p, fh.pos); err != nil {
		return 0, err
	}
	end := fh.pos + int64(len(p))
	if end > fh.size {
		fh.size = end
	}
	fh.pos = end
	fh.fs.debugf("Write complete inode=%d path=%q new_size=%d new_pos=%d", fh.inode, fh.path, fh.size, fh.pos)
	return len(p), nil
}

func (fh *fileHandle) Seek(offset int64, whence int) (int64, error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	if err := fh.ensureOpen(); err != nil {
		return 0, err
	}
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = fh.pos + offset
	case io.SeekEnd:
		ctx, cancel := fh.fs.withTimeout()
		defer cancel()
		st, err := fh.fs.client.StatInode(ctx, fh.inode)
		if err != nil {
			return 0, err
		}
		if st == nil {
			return 0, os.ErrNotExist
		}
		fh.size = st.Size
		next = st.Size + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if next < 0 {
		return 0, errors.New("negative position")
	}
	fh.pos = next
	return fh.pos, nil
}

func (fh *fileHandle) Close() error {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	if fh.closed {
		return nil
	}
	fh.closed = true
	fh.fs.debugf("Close inode=%d path=%q", fh.inode, fh.path)
	return nil
}

func (fh *fileHandle) Lock() error {
	return errors.New("nfs advisory locking is disabled; mount clients should use nolock/nolocks")
}

func (fh *fileHandle) Unlock() error {
	return errors.New("nfs advisory locking is disabled; mount clients should use nolock/nolocks")
}

func (fh *fileHandle) Truncate(size int64) error {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	if err := fh.ensureOpen(); err != nil {
		return err
	}
	if !fh.writable || fh.fs.readOnly {
		return os.ErrPermission
	}
	if size < 0 {
		return errors.New("negative size")
	}
	ctx, cancel := fh.fs.withTimeout()
	defer cancel()
	fh.fs.debugf("Truncate inode=%d path=%q size=%d", fh.inode, fh.path, size)
	if err := fh.fs.client.TruncateInodeAtPath(ctx, fh.inode, fh.path, size); err != nil {
		return err
	}
	fh.size = size
	if fh.pos > size {
		fh.pos = size
	}
	return nil
}
