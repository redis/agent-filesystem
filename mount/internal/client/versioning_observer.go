package client

import (
	"context"
	"errors"
	"strings"

	"github.com/redis/go-redis/v9"
)

type VersionedSnapshot struct {
	Path      string
	Exists    bool
	Kind      string
	Mode      uint32
	Content   []byte
	Target    string
	SizeBytes int64
}

type MutationObserver interface {
	RecordMutation(ctx context.Context, workspace string, before, after VersionedSnapshot) error
}

func (c *nativeClient) versionedSnapshotFromResolved(ctx context.Context, resolvedPath string, inode *inodeData) (VersionedSnapshot, error) {
	snapshot := VersionedSnapshot{Path: normalizePath(resolvedPath)}
	if c.observer == nil {
		return snapshot, nil
	}
	if inode == nil {
		return snapshot, nil
	}
	switch inode.Type {
	case "file":
		content, err := c.loadContentExternal(ctx, inode.ID, inode.ContentRef)
		if err != nil {
			return VersionedSnapshot{}, err
		}
		snapshot.Exists = true
		snapshot.Kind = "file"
		snapshot.Mode = inode.Mode
		snapshot.SizeBytes = inode.Size
		snapshot.Content = []byte(content)
		return snapshot, nil
	case "symlink":
		snapshot.Exists = true
		snapshot.Kind = "symlink"
		snapshot.Mode = inode.Mode
		snapshot.Target = inode.Target
		snapshot.SizeBytes = int64(len(inode.Target))
		return snapshot, nil
	default:
		return snapshot, nil
	}
}

func (c *nativeClient) versionedSnapshotForCurrentInode(ctx context.Context, inodeID, pathHint string) (VersionedSnapshot, error) {
	if c.observer == nil {
		return VersionedSnapshot{Path: normalizePath(pathHint)}, nil
	}
	inodeID = strings.TrimSpace(inodeID)
	if inodeID == "" {
		return VersionedSnapshot{Path: normalizePath(pathHint)}, nil
	}
	inode, err := c.loadInodeByID(ctx, inodeID)
	if err != nil {
		return VersionedSnapshot{}, err
	}
	if inode == nil {
		return VersionedSnapshot{Path: normalizePath(pathHint)}, nil
	}
	resolvedPath := normalizePath(pathHint)
	if resolvedPath == "/" || strings.TrimSpace(pathHint) == "" {
		value, err := c.rdb.HGet(ctx, c.keys.inode(inodeID), "path").Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return VersionedSnapshot{}, err
		}
		if strings.TrimSpace(value) != "" {
			resolvedPath = normalizePath(value)
		}
	}
	return c.versionedSnapshotFromResolved(ctx, resolvedPath, inode)
}

func (c *nativeClient) recordVersionMutation(ctx context.Context, before, after VersionedSnapshot) error {
	if c.observer == nil {
		return nil
	}
	return c.observer.RecordMutation(ctx, c.key, before, after)
}
