package nfsfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/redis/go-redis/v9"
	"github.com/rowantrollope/agent-filesystem/mount/internal/client"
)

var _ billy.Filesystem = (*FS)(nil)
var _ billy.Change = (*FS)(nil)

type FS struct {
	client   client.Client
	readOnly bool
}

func New(c client.Client, readOnly bool) *FS {
	return &FS{client: c, readOnly: readOnly}
}

func (f *FS) withTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
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

func (f *FS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	p := f.normalize(filename)
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
	if missing {
		if flag&os.O_CREATE == 0 {
			return nil, os.ErrNotExist
		}
		if f.readOnly {
			return nil, os.ErrPermission
		}
		// Defer creation to Close — avoids double-write (create empty then overwrite).
		return &fileHandle{
			fs:            f,
			path:          p,
			data:          nil,
			writable:      true,
			dirty:         true,
			pendingCreate: true,
			createMode:    perm,
		}, nil
	}

	if st.Type == "dir" {
		return nil, fmt.Errorf("%s is a directory", p)
	}
	if flag&os.O_EXCL != 0 && flag&os.O_CREATE != 0 && !missing {
		return nil, os.ErrExist
	}

	var data []byte
	if flag&os.O_TRUNC != 0 {
		if f.readOnly {
			return nil, os.ErrPermission
		}
		data = nil
	} else {
		data, err = f.client.Cat(ctx, p)
		if err != nil {
			return nil, err
		}
	}

	fh := &fileHandle{
		fs:       f,
		path:     p,
		data:     append([]byte(nil), data...),
		writable: flag&(os.O_WRONLY|os.O_RDWR) != 0 || flag&(os.O_CREATE|os.O_APPEND|os.O_TRUNC) != 0,
		append:   flag&os.O_APPEND != 0,
	}
	if fh.append {
		fh.pos = int64(len(fh.data))
	}
	if flag&os.O_TRUNC != 0 {
		fh.dirty = true
	}
	return fh, nil
}

func (f *FS) Create(filename string) (billy.File, error) {
	return f.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

func (f *FS) Stat(filename string) (os.FileInfo, error) {
	p := f.normalize(filename)
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
	ctx, cancel := f.withTimeout()
	defer cancel()
	return f.client.Mv(ctx, f.normalize(oldpath), f.normalize(newpath))
}

func (f *FS) Remove(filename string) error {
	if f.readOnly {
		return os.ErrPermission
	}
	ctx, cancel := f.withTimeout()
	defer cancel()
	return f.client.Rm(ctx, f.normalize(filename))
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
			Type:  entry.Type,
			Mode:  entry.Mode,
			Size:  entry.Size,
			Mtime: entry.Mtime,
		}
		out = append(out, newFileInfo(entry.Name, st))
	}
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
	ctx, cancel := f.withTimeout()
	defer cancel()
	return f.client.Chmod(ctx, f.normalize(name), uint32(mode.Perm()))
}

func (f *FS) Lchown(name string, uid, gid int) error {
	// Agent Filesystem stores ownership per path; symlink-vs-target semantics are not distinguished.
	return f.Chown(name, uid, gid)
}

func (f *FS) Chown(name string, uid, gid int) error {
	if f.readOnly {
		return os.ErrPermission
	}
	ctx, cancel := f.withTimeout()
	defer cancel()
	return f.client.Chown(ctx, f.normalize(name), uint32(uid), uint32(gid))
}

func (f *FS) Chtimes(name string, atime time.Time, mtime time.Time) error {
	if f.readOnly {
		return os.ErrPermission
	}
	ctx, cancel := f.withTimeout()
	defer cancel()
	return f.client.Utimens(ctx, f.normalize(name), atime.UnixMilli(), mtime.UnixMilli())
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
func (fi fileInfo) Sys() interface{}   { return nil }

type fileHandle struct {
	mu            sync.Mutex
	fs            *FS
	path          string
	data          []byte
	pos           int64
	writable      bool
	append        bool
	dirty         bool
	closed        bool
	pendingCreate bool
	createMode    os.FileMode
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
	if fh.pos >= int64(len(fh.data)) {
		return 0, io.EOF
	}
	n := copy(p, fh.data[fh.pos:])
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
	if off >= int64(len(fh.data)) {
		return 0, io.EOF
	}
	n := copy(p, fh.data[off:])
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
		fh.pos = int64(len(fh.data))
	}
	end := fh.pos + int64(len(p))
	if end > int64(len(fh.data)) {
		newCap := int64(cap(fh.data)) * 2
		if newCap < end {
			newCap = end
		}
		grown := make([]byte, end, newCap)
		copy(grown, fh.data)
		fh.data = grown
	}
	copy(fh.data[fh.pos:end], p)
	fh.pos = end
	fh.dirty = true
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
		next = int64(len(fh.data)) + offset
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
	if !fh.dirty {
		return nil
	}
	ctx, cancel := fh.fs.withTimeout()
	defer cancel()
	if fh.pendingCreate {
		mode := uint32(0o644)
		if fh.createMode != 0 {
			mode = uint32(fh.createMode.Perm())
		}
		return fh.fs.client.EchoCreate(ctx, fh.path, fh.data, mode)
	}
	return fh.fs.client.Echo(ctx, fh.path, fh.data)
}

func (fh *fileHandle) Lock() error   { return nil }
func (fh *fileHandle) Unlock() error { return nil }

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
	if size <= int64(len(fh.data)) {
		fh.data = fh.data[:size]
	} else {
		grown := make([]byte, size)
		copy(grown, fh.data)
		fh.data = grown
	}
	if fh.pos > size {
		fh.pos = size
	}
	fh.dirty = true
	return nil
}
