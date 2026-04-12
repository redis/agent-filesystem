package client

import (
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
	InvalidateOpInode   = internal.InvalidateOpInode
	InvalidateOpDir     = internal.InvalidateOpDir
	InvalidateOpPrefix  = internal.InvalidateOpPrefix
	InvalidateOpContent = internal.InvalidateOpContent
)

func New(rdb *redis.Client, key string) Client {
	return internal.New(rdb, key)
}
