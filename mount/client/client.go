package client

import (
	"github.com/redis/go-redis/v9"
	internal "github.com/rowantrollope/agent-filesystem/mount/internal/client"
)

type Client = internal.Client
type StatResult = internal.StatResult
type LsEntry = internal.LsEntry
type InfoResult = internal.InfoResult
type WcResult = internal.WcResult
type TreeEntry = internal.TreeEntry
type GrepMatch = internal.GrepMatch

func New(rdb *redis.Client, key string) Client {
	return internal.New(rdb, key)
}
