// Package client provides filesystem client backends over Redis.
package client

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	RenameNoreplace uint32 = 0x1
	RenameExchange  uint32 = 0x2
)

// Client provides the filesystem operation surface used by the mount layer.
type Client interface {
	Stat(ctx context.Context, path string) (*StatResult, error)
	StatInode(ctx context.Context, inode uint64) (*StatResult, error)
	Cat(ctx context.Context, path string) ([]byte, error)
	Echo(ctx context.Context, path string, data []byte) error
	EchoCreate(ctx context.Context, path string, data []byte, mode uint32) error
	CreateFile(ctx context.Context, path string, mode uint32, exclusive bool) (*StatResult, bool, error)
	EchoAppend(ctx context.Context, path string, data []byte) error
	Touch(ctx context.Context, path string) error
	ReadInodeAt(ctx context.Context, inode uint64, off int64, size int) ([]byte, error)
	WriteInodeAt(ctx context.Context, inode uint64, data []byte, off int64) error
	TruncateInode(ctx context.Context, inode uint64, size int64) error
	Getlk(ctx context.Context, inode uint64, handleID string, lk *FileLock) (*FileLock, error)
	Setlk(ctx context.Context, inode uint64, handleID string, lk *FileLock, wait bool) error
	UnlockAll(ctx context.Context, inode uint64, handleID string) error
	Mkdir(ctx context.Context, path string) error
	Rm(ctx context.Context, path string) error
	Ls(ctx context.Context, path string) ([]string, error)
	LsLong(ctx context.Context, path string) ([]LsEntry, error)
	Rename(ctx context.Context, src, dst string, flags uint32) error
	Mv(ctx context.Context, src, dst string) error
	Ln(ctx context.Context, target, linkpath string) error
	Readlink(ctx context.Context, path string) (string, error)
	Chmod(ctx context.Context, path string, mode uint32) error
	Chown(ctx context.Context, path string, uid, gid uint32) error
	Truncate(ctx context.Context, path string, size int64) error
	Utimens(ctx context.Context, path string, atimeMs, mtimeMs int64) error
	Info(ctx context.Context) (*InfoResult, error)

	Head(ctx context.Context, path string, n int) (string, error)
	Tail(ctx context.Context, path string, n int) (string, error)
	Lines(ctx context.Context, path string, start, end int) (string, error)
	Wc(ctx context.Context, path string) (*WcResult, error)
	Insert(ctx context.Context, path string, afterLine int, content string) error
	Replace(ctx context.Context, path string, old, new string, all bool) (int64, error)
	DeleteLines(ctx context.Context, path string, start, end int) (int64, error)

	Cp(ctx context.Context, src, dst string, recursive bool) error
	Tree(ctx context.Context, path string, maxDepth int) ([]TreeEntry, error)
	Find(ctx context.Context, path, pattern string, typeFilter string) ([]string, error)
	Grep(ctx context.Context, path, pattern string, nocase bool) ([]GrepMatch, error)
}

// PathCacheWarmer is implemented by clients that can prewarm exact-path cache
// entries from backend metadata.
type PathCacheWarmer interface {
	WarmPathCache(ctx context.Context) error
}

// New creates a filesystem client for the given Redis key.
// It uses the native HASH/SET backend that works with any Redis instance.
func New(rdb *redis.Client, key string) Client {
	return newNativeClient(rdb, key)
}

// NewWithCache creates a filesystem client with an inode cache.
// Repeated path lookups within the TTL window skip Redis round-trips.
// All write operations automatically invalidate affected cache entries.
func NewWithCache(rdb *redis.Client, key string, ttl time.Duration) Client {
	return newNativeClientWithCache(rdb, key, ttl)
}
