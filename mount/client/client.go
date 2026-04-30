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
