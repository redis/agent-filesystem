package client

import "errors"

// Sentinel errors returned by the filesystem client. Callers (notably
// the FUSE/NFS error mappers) compare against these via errors.Is so a
// caller wrapping the error with extra context (fmt.Errorf("...: %w", err))
// stays mappable — the previous strings.Contains(err.Error(), "...")
// approach silently broke as soon as a wrapper rephrased the message.
var (
	ErrNotFound        = errors.New("no such file or directory")
	ErrNotFile         = errors.New("not a file")
	ErrNotDir          = errors.New("not a directory")
	ErrNotSymlink      = errors.New("not a symlink")
	ErrAlreadyExists   = errors.New("already exists")
	ErrDirNotEmpty     = errors.New("directory not empty")
	ErrUnsupported     = errors.New("operation not supported")
	ErrCannotWriteRoot = errors.New("cannot write to root")
	ErrCannotMoveRoot  = errors.New("cannot move root")
	ErrCannotRemoveRoot = errors.New("cannot remove root")
	ErrParentConflict  = errors.New("parent path conflict")
	ErrLockWouldBlock  = errors.New("lock would block")
)
