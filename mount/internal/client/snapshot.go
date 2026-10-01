package client

import (
	"context"
	"errors"
	"path"

	"github.com/redis/go-redis/v9"
)

// FileSnapshot pairs bytes with the revision which produced them. A nil Stat
// represents absence, and can be used as a conditional-create precondition.
type FileSnapshot struct {
	Stat    *StatResult
	Content []byte
	Parent  *StatResult
	Path    string
}

// WriteContext carries the observation all the way into Redis publication.
// The existing parent-chain WATCH also fences ancestor renames/replacements.
func (s FileSnapshot) WriteContext(ctx context.Context) context.Context {
	ctx = WithExpectedStat(ctx, s.Stat)
	if s.Parent != nil {
		ctx = WithExpectedParent(ctx, path.Dir(s.Path), s.Parent.Inode)
	}
	return ctx
}

// ReadFileSnapshot deliberately bypasses the attribute cache. Stat/Cat/Stat
// must describe the same publication even for String content: an atomic GET
// alone doesn't pair the bytes with the revision checked by a later upload.
func ReadFileSnapshot(ctx context.Context, fs Client, p string) (FileSnapshot, error) {
	p = normalizePath(p)
	for attempt := 0; attempt < 8; attempt++ {
		if err := ctx.Err(); err != nil {
			return FileSnapshot{}, err
		}
		fs.InvalidateCache()
		parent, err := fs.Stat(ctx, path.Dir(p))
		if err != nil && !errors.Is(err, redis.Nil) && !errors.Is(err, ErrNotFound) {
			return FileSnapshot{}, err
		}
		before, err := fs.Stat(ctx, p)
		if err != nil && !errors.Is(err, redis.Nil) && !errors.Is(err, ErrNotFound) {
			return FileSnapshot{}, err
		}
		var data []byte
		var readErr error
		if before != nil {
			if before.Type != "file" {
				return FileSnapshot{}, ErrNotFile
			}
			data, readErr = fs.Cat(ctx, p)
		}
		fs.InvalidateCache()
		after, err := fs.Stat(ctx, p)
		if err != nil && !errors.Is(err, redis.Nil) && !errors.Is(err, ErrNotFound) {
			return FileSnapshot{}, err
		}
		currentParent, err := fs.Stat(ctx, path.Dir(p))
		if err != nil && !errors.Is(err, redis.Nil) && !errors.Is(err, ErrNotFound) {
			return FileSnapshot{}, err
		}
		if !SameFileRevision(before, after) || !sameParent(parent, currentParent) {
			continue
		}
		if readErr != nil {
			return FileSnapshot{}, readErr
		}
		return FileSnapshot{Stat: before, Content: data, Parent: parent, Path: p}, nil
	}
	return FileSnapshot{}, ErrWriteConflict
}

func SameFileRevision(a, b *StatResult) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	// Metadata comparisons also support legacy inodes without revisions.
	return a.Inode == b.Inode && a.Revision == b.Revision && a.Type == b.Type &&
		a.Size == b.Size && a.Mode == b.Mode && a.Mtime == b.Mtime
}

func sameParent(a, b *StatResult) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Inode == b.Inode && a.Type == b.Type
}

type mutationResultKey struct{}
type fileModeKey struct{}

// MutationResult describes our committed publication, even if a peer writes
// immediately afterward. It is per operation; do not share it across goroutines.
type MutationResult struct {
	Stat        *StatResult
	OperationID string
}

func WithMutationResult(ctx context.Context, result *MutationResult) context.Context {
	return context.WithValue(ctx, mutationResultKey{}, result)
}

// WithFileMode includes mode in the content publication, removing the trailing
// unconditional chmod that otherwise races with a peer's newer publication.
func WithFileMode(ctx context.Context, mode uint32) context.Context {
	return context.WithValue(ctx, fileModeKey{}, mode)
}

func captureMutationResult(ctx context.Context, inode *inodeData, operationID string) {
	if result, _ := ctx.Value(mutationResultKey{}).(*MutationResult); result != nil {
		result.Stat = inode.toStat()
		result.OperationID = operationID
	}
}
