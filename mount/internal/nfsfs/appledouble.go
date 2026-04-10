package nfsfs

import (
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"
)

// macOS creates AppleDouble sidecar files (prefix "._") on non-HFS/APFS
// filesystems to carry resource forks and extended attributes. In practice
// this means that for every real file touched via the Finder, Spotlight,
// or any API that uses copyfile(3), macOS also creates, reads, writes, and
// eventually deletes a paired "._<basename>" file in the same directory.
//
// On a Redis-backed filesystem these sidecars are extremely expensive: each
// one adds multiple NFS RPCs (CREATE, TRUNCATE, several WRITEs of 4 KB each,
// plus repeated LOOKUP/GETATTR bursts). For a single shell `echo > file`,
// measurement showed ~18 NFS RPCs, half of them against the "._" sidecar.
//
// AppleDouble sidecars are purely a macOS fallback for filesystems that do
// not expose native extended attributes. They are not observable to
// cross-platform tools and any agent accessing the workspace through the
// AFS CLI will never see them. It is therefore safe — and a major
// performance win — to keep these files entirely in NFS-server memory for
// the life of the process and never persist them to Redis.
//
// This file implements that in-memory shadow. All operations on paths whose
// basename begins with "._" are routed here instead of to the Redis-backed
// client.

// appleDoublePrefix is the basename prefix macOS uses for its AppleDouble
// sidecar files.
const appleDoublePrefix = "._"

// isAppleDoublePath reports whether p should be handled by the sidecar
// shadow store instead of the real client.
func isAppleDoublePath(p string) bool {
	return strings.HasPrefix(path.Base(p), appleDoublePrefix)
}

// shadowFile is a minimal in-memory file that imitates enough of a regular
// file for the NFS server to satisfy macOS's AppleDouble traffic.
type shadowFile struct {
	mu      sync.Mutex
	mode    os.FileMode
	content []byte
	mtime   time.Time
	ctime   time.Time
	atime   time.Time
	// A stable synthetic inode number so NFS handles remain consistent
	// across operations on the same path within a session.
	inode uint64
}

// shadowStore holds every AppleDouble sidecar the NFS server has seen so
// far. All operations are keyed by normalized absolute path.
type shadowStore struct {
	mu        sync.RWMutex
	files     map[string]*shadowFile
	nextInode uint64
}

func newShadowStore() *shadowStore {
	return &shadowStore{
		files: make(map[string]*shadowFile),
		// Start far above any realistic real-inode value to make collisions
		// with inode numbers from the Redis-backed client impossible.
		nextInode: 1 << 40,
	}
}

// get returns the shadow file at p, or nil if it doesn't exist.
func (s *shadowStore) get(p string) *shadowFile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.files[p]
}

// getOrCreate returns the shadow file at p, creating it with the supplied
// default mode if it does not yet exist. The second return value is true
// when a new entry was created.
func (s *shadowStore) getOrCreate(p string, mode os.FileMode) (*shadowFile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.files[p]; ok {
		return f, false
	}
	now := time.Now()
	s.nextInode++
	f := &shadowFile{
		mode:  mode | 0o600,
		mtime: now,
		ctime: now,
		atime: now,
		inode: s.nextInode,
	}
	s.files[p] = f
	return f, true
}

// remove deletes a shadow file. Returns true if something was removed.
func (s *shadowStore) remove(p string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.files[p]
	if ok {
		delete(s.files, p)
	}
	return ok
}

// rename moves the shadow file at oldPath to newPath. Returns true on
// success, false if no shadow file was registered at oldPath.
func (s *shadowStore) rename(oldPath, newPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[oldPath]
	if !ok {
		return false
	}
	delete(s.files, oldPath)
	s.files[newPath] = f
	return true
}

// listChildren returns all shadow files whose parent directory equals dir.
// Used to merge shadow entries into ReadDir results.
func (s *shadowStore) listChildren(dir string) []os.FileInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []os.FileInfo
	for p, f := range s.files {
		if path.Dir(p) != dir {
			continue
		}
		out = append(out, shadowFileInfo{name: path.Base(p), file: f})
	}
	return out
}

// shadowFileInfo implements os.FileInfo for a shadowFile.
type shadowFileInfo struct {
	name string
	file *shadowFile
}

func (fi shadowFileInfo) Name() string { return fi.name }
func (fi shadowFileInfo) Size() int64 {
	fi.file.mu.Lock()
	defer fi.file.mu.Unlock()
	return int64(len(fi.file.content))
}
func (fi shadowFileInfo) Mode() os.FileMode {
	fi.file.mu.Lock()
	defer fi.file.mu.Unlock()
	return fi.file.mode
}
func (fi shadowFileInfo) ModTime() time.Time {
	fi.file.mu.Lock()
	defer fi.file.mu.Unlock()
	return fi.file.mtime
}
func (fi shadowFileInfo) IsDir() bool { return false }
func (fi shadowFileInfo) Sys() interface{} {
	return &syscall.Stat_t{
		Ino:   fi.file.inode,
		Nlink: 1,
	}
}

// shadowHandle implements billy.File for a shadow file.
type shadowHandle struct {
	path   string
	store  *shadowStore
	file   *shadowFile
	pos    int64
	closed bool
}

func (h *shadowHandle) Name() string { return h.path }

func (h *shadowHandle) Read(p []byte) (int, error) {
	if h.closed {
		return 0, os.ErrClosed
	}
	h.file.mu.Lock()
	defer h.file.mu.Unlock()
	if h.pos >= int64(len(h.file.content)) {
		return 0, io.EOF
	}
	n := copy(p, h.file.content[h.pos:])
	h.pos += int64(n)
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (h *shadowHandle) ReadAt(p []byte, off int64) (int, error) {
	if h.closed {
		return 0, os.ErrClosed
	}
	h.file.mu.Lock()
	defer h.file.mu.Unlock()
	if off >= int64(len(h.file.content)) {
		return 0, io.EOF
	}
	n := copy(p, h.file.content[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (h *shadowHandle) Write(p []byte) (int, error) {
	if h.closed {
		return 0, os.ErrClosed
	}
	h.file.mu.Lock()
	defer h.file.mu.Unlock()
	end := h.pos + int64(len(p))
	if end > int64(len(h.file.content)) {
		grown := make([]byte, end)
		copy(grown, h.file.content)
		h.file.content = grown
	}
	copy(h.file.content[h.pos:end], p)
	h.pos = end
	now := time.Now()
	h.file.mtime = now
	h.file.atime = now
	return len(p), nil
}

func (h *shadowHandle) Seek(offset int64, whence int) (int64, error) {
	if h.closed {
		return 0, os.ErrClosed
	}
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = h.pos + offset
	case io.SeekEnd:
		h.file.mu.Lock()
		next = int64(len(h.file.content)) + offset
		h.file.mu.Unlock()
	default:
		return 0, errors.New("invalid whence")
	}
	if next < 0 {
		return 0, errors.New("negative position")
	}
	h.pos = next
	return next, nil
}

func (h *shadowHandle) Close() error {
	h.closed = true
	return nil
}

func (h *shadowHandle) Truncate(size int64) error {
	if h.closed {
		return os.ErrClosed
	}
	if size < 0 {
		return errors.New("negative size")
	}
	h.file.mu.Lock()
	defer h.file.mu.Unlock()
	if int64(len(h.file.content)) > size {
		h.file.content = h.file.content[:size]
	} else if int64(len(h.file.content)) < size {
		grown := make([]byte, size)
		copy(grown, h.file.content)
		h.file.content = grown
	}
	if h.pos > size {
		h.pos = size
	}
	now := time.Now()
	h.file.mtime = now
	h.file.atime = now
	return nil
}

// Lock / Unlock — NFS advisory locking is disabled for this export, mirror
// fileHandle behavior.
func (h *shadowHandle) Lock() error {
	return errors.New("nfs advisory locking is disabled; mount clients should use nolock/nolocks")
}

func (h *shadowHandle) Unlock() error {
	return errors.New("nfs advisory locking is disabled; mount clients should use nolock/nolocks")
}
