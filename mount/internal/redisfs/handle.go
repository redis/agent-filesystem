package redisfs

import (
	"context"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/rowantrollope/agent-filesystem/mount/internal/client"
)

// FileHandle manages buffered I/O for an open file.
type FileHandle struct {
	path   string
	client client.Client
	node   *FSNode

	mu      sync.Mutex
	content []byte // fetched lazily on first Read
	loaded  bool   // whether content has been fetched
	dirty   bool   // whether buffer has been modified
}

func newFileHandle(path string, c client.Client, node *FSNode) *FileHandle {
	return &FileHandle{
		path:   path,
		client: c,
		node:   node,
	}
}

func (fh *FileHandle) load(ctx context.Context) error {
	if fh.loaded {
		return nil
	}
	data, err := fh.client.Cat(ctx, fh.path)
	if err != nil {
		// File might be empty or new
		if mapError(err) == syscall.ENOENT {
			fh.content = nil
			fh.loaded = true
			return nil
		}
		return err
	}
	fh.content = data
	fh.loaded = true
	return nil
}

// Read reads data from the file handle.
func (fh *FileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	if err := fh.load(ctx); err != nil {
		return nil, mapError(err)
	}

	size := int64(len(fh.content))
	if off >= size {
		return fuse.ReadResultData(nil), 0
	}

	end := off + int64(len(dest))
	if end > size {
		end = size
	}

	return fuse.ReadResultData(fh.content[off:end]), 0
}

// Write writes data to the file handle buffer.
func (fh *FileHandle) Write(ctx context.Context, data []byte, off int64) (uint32, syscall.Errno) {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	if err := fh.load(ctx); err != nil {
		return 0, mapError(err)
	}

	end := off + int64(len(data))
	if end > int64(len(fh.content)) {
		// Extend the buffer.
		newBuf := make([]byte, end)
		copy(newBuf, fh.content)
		fh.content = newBuf
	}
	copy(fh.content[off:], data)
	fh.dirty = true

	return uint32(len(data)), 0
}

// Flush writes the buffer to Redis if dirty.
func (fh *FileHandle) Flush(ctx context.Context) syscall.Errno {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	if !fh.dirty {
		return 0
	}

	data := fh.content
	if data == nil {
		data = []byte{}
	}

	if err := fh.client.Echo(ctx, fh.path, data); err != nil {
		return mapError(err)
	}
	fh.dirty = false

	// Invalidate caches for this file and parent dir.
	fh.node.root().invalidatePath(fh.path)

	return 0
}

// SetTruncated marks the handle as truncated (empty, dirty).
func (fh *FileHandle) SetTruncated() {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.content = []byte{}
	fh.loaded = true
	fh.dirty = true
}
