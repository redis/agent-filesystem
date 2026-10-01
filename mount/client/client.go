package client

import (
	"context"
	"time"

	internal "github.com/redis/agent-filesystem/mount/internal/client"
	"github.com/redis/go-redis/v9"
)

type Client = internal.Client
type StatResult = internal.StatResult
type LsEntry = internal.LsEntry
type InfoResult = internal.InfoResult
type WcResult = internal.WcResult
type TreeEntry = internal.TreeEntry
type GrepMatch = internal.GrepMatch
type InvalidateEvent = internal.InvalidateEvent
type AttrUpdate = internal.AttrUpdate
type ChangeStreamEntry = internal.ChangeStreamEntry
type VersionedSnapshot = internal.VersionedSnapshot
type MutationObserver = internal.MutationObserver
type FileSnapshot = internal.FileSnapshot
type MutationResult = internal.MutationResult

func ReadFileSnapshot(ctx context.Context, fs Client, path string) (FileSnapshot, error) {
	return internal.ReadFileSnapshot(ctx, fs, path)
}

func SameFileRevision(a, b *StatResult) bool { return internal.SameFileRevision(a, b) }

func WithMutationResult(ctx context.Context, result *MutationResult) context.Context {
	return internal.WithMutationResult(ctx, result)
}

func WithFileMode(ctx context.Context, mode uint32) context.Context {
	return internal.WithFileMode(ctx, mode)
}

var ErrStreamTrimmed = internal.ErrStreamTrimmed

const (
	InvalidateOpInode       = internal.InvalidateOpInode
	InvalidateOpDir         = internal.InvalidateOpDir
	InvalidateOpPrefix      = internal.InvalidateOpPrefix
	InvalidateOpContent     = internal.InvalidateOpContent
	InvalidateOpRootReplace = internal.InvalidateOpRootReplace
)

func New(rdb *redis.Client, key string) Client {
	return internal.New(rdb, key)
}

func NewWithCache(rdb *redis.Client, key string, ttl time.Duration) Client {
	return internal.NewWithCache(rdb, key, ttl)
}

func PublishInvalidation(ctx context.Context, rdb *redis.Client, key string, ev InvalidateEvent) error {
	return internal.PublishInvalidation(ctx, rdb, key, ev)
}

func NewWithObserver(rdb *redis.Client, key string, observer MutationObserver) Client {
	return internal.NewWithObserver(rdb, key, observer)
}

func NewWithCacheAndObserver(rdb *redis.Client, key string, ttl time.Duration, observer MutationObserver) Client {
	return internal.NewWithCacheAndObserver(rdb, key, ttl, observer)
}

var ErrWriteConflict = internal.ErrWriteConflict
var ErrWorkspaceChanged = internal.ErrWorkspaceChanged
var ErrGenerationRequired = internal.ErrGenerationRequired

func WithExpectedStat(ctx context.Context, stat *StatResult) context.Context {
	return internal.WithExpectedStat(ctx, stat)
}
func WithWorkspaceGeneration(ctx context.Context, generation string) context.Context {
	return internal.WithWorkspaceGeneration(ctx, generation)
}
