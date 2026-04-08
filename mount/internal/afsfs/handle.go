package afsfs

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/redis/agent-filesystem/mount/internal/client"
)

var nextHandleID uint64

// FileHandle manages buffered I/O for an open file.
type FileHandle struct {
	path     string
	inode    uint64
	handleID string
	client   client.Client
	node     *FSNode

	mu sync.Mutex
}

func newFileHandle(path string, inode uint64, c client.Client, node *FSNode) *FileHandle {
	return &FileHandle{
		path:     path,
		inode:    inode,
		handleID: fmt.Sprintf("%d-%d", os.Getpid(), atomic.AddUint64(&nextHandleID, 1)),
		client:   c,
		node:     node,
	}
}

func (fh *FileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	data, err := fh.client.ReadInodeAt(ctx, fh.inode, off, len(dest))
	if err != nil {
		return nil, mapError(err)
	}
	return fuse.ReadResultData(data), 0
}

func (fh *FileHandle) Write(ctx context.Context, data []byte, off int64) (uint32, syscall.Errno) {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	if err := fh.client.WriteInodeAt(ctx, fh.inode, data, off); err != nil {
		return 0, mapError(err)
	}
	fh.node.root().invalidatePath(fh.path)

	return uint32(len(data)), 0
}

func (fh *FileHandle) Flush(ctx context.Context) syscall.Errno {
	return 0
}

func (fh *FileHandle) Release(ctx context.Context) syscall.Errno {
	flushErr := fh.Flush(ctx)
	unlockErr := fh.unlockAll(ctx)
	if flushErr != 0 {
		return flushErr
	}
	return unlockErr
}

func (fh *FileHandle) Getlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) syscall.Errno {
	conflict, err := fh.client.Getlk(ctx, fh.inode, fh.handleID, fuseToClientLock(lk))
	if err != nil {
		return mapError(err)
	}
	if conflict == nil {
		out.Typ = syscall.F_UNLCK
		out.Start = 0
		out.End = 0
		out.Pid = 0
		return 0
	}
	*out = clientToFuseLock(conflict)
	return 0
}

func (fh *FileHandle) Setlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) syscall.Errno {
	err := fh.client.Setlk(ctx, fh.inode, fh.handleID, fuseToClientLock(lk), false)
	if err == nil {
		return 0
	}
	if err == context.Canceled || err == context.DeadlineExceeded {
		return syscall.EINTR
	}
	if err.Error() == "lock would block" {
		return syscall.EAGAIN
	}
	return mapError(err)
}

func (fh *FileHandle) Setlkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) syscall.Errno {
	err := fh.client.Setlk(ctx, fh.inode, fh.handleID, fuseToClientLock(lk), true)
	if err == nil {
		return 0
	}
	if err == context.Canceled || err == context.DeadlineExceeded {
		return syscall.EINTR
	}
	if err.Error() == "lock would block" {
		return syscall.EAGAIN
	}
	return mapError(err)
}

func (fh *FileHandle) unlockAll(ctx context.Context) syscall.Errno {
	if fh.inode == 0 {
		return 0
	}
	if err := fh.client.UnlockAll(ctx, fh.inode, fh.handleID); err != nil {
		if err == context.Canceled || err == context.DeadlineExceeded {
			return syscall.EINTR
		}
		return mapError(err)
	}
	return 0
}

func fuseToClientLock(lk *fuse.FileLock) *client.FileLock {
	if lk == nil {
		return nil
	}
	return &client.FileLock{
		Start: lk.Start,
		End:   lk.End,
		Type:  lk.Typ,
		PID:   lk.Pid,
	}
}

func clientToFuseLock(lk *client.FileLock) fuse.FileLock {
	if lk == nil {
		return fuse.FileLock{Typ: syscall.F_UNLCK}
	}
	return fuse.FileLock{
		Start: lk.Start,
		End:   lk.End,
		Typ:   lk.Type,
		Pid:   lk.PID,
	}
}

var _ fs.FileGetlker = (*FileHandle)(nil)
var _ fs.FileSetlker = (*FileHandle)(nil)
var _ fs.FileSetlkwer = (*FileHandle)(nil)
