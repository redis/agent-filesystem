package client

import (
	"context"
	"errors"
	"strconv"

	"github.com/redis/go-redis/v9"
)

func (c *nativeClient) StatInode(ctx context.Context, inode uint64) (*StatResult, error) {
	data, err := c.loadInodeByID(ctx, strconv.FormatUint(inode, 10))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return data.toStat(), nil
}

func (c *nativeClient) ReadInodeAt(ctx context.Context, inode uint64, off int64, size int) ([]byte, error) {
	if off < 0 {
		return nil, errors.New("invalid offset")
	}
	data, err := c.loadInodeByID(ctx, strconv.FormatUint(inode, 10))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, errors.New("no such file or directory")
	}
	if data.Type != "file" {
		return nil, errors.New("not a file")
	}
	content, err := c.loadContentExternal(ctx, data.ID, data.ContentRef)
	if err != nil {
		return nil, err
	}
	if off >= int64(len(content)) {
		return []byte{}, nil
	}
	end := off + int64(size)
	if end > int64(len(content)) {
		end = int64(len(content))
	}
	return []byte(content[off:end]), nil
}

// inodeWithContentFields is inodeMetaFields + "content". Used for legacy
// inline inodes where content is still in the HASH.
var inodeWithContentFields = append(append([]string{}, inodeMetaFields...), "content")

// loadInodeWithContentByID fetches the full inode metadata and file content.
// For external content (content_ref="ext") it pipelines the metadata HMGET
// with a GET on the content key. For legacy inline inodes it reads both from
// the HASH in one HMGET.
func (c *nativeClient) loadInodeWithContentByID(ctx context.Context, id string) (*inodeData, error) {
	// First fetch metadata (including content_ref) to determine storage mode.
	// For the common external case, pipeline metadata + content GET together.
	pipe := c.rdb.Pipeline()
	metaCmd := pipe.HMGet(ctx, c.keys.inode(id), inodeMetaFields...)
	contentCmd := pipe.Get(ctx, c.keys.content(id))
	// Also try legacy inline content in case this is an old inode.
	inlineCmd := pipe.HGet(ctx, c.keys.inode(id), "content")
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	vals, err := metaCmd.Result()
	if err != nil {
		return nil, err
	}
	inode := inodeFromValues(id, vals)
	if inode == nil {
		return nil, nil
	}

	if inode.ContentRef == "ext" {
		if v, err := contentCmd.Result(); err == nil {
			inode.Content = v
		}
	} else {
		if v, err := inlineCmd.Result(); err == nil {
			inode.Content = v
		}
	}
	return inode, nil
}

// WriteInodeAt is the legacy entry point; callers that do not know the path
// still invalidate the entire path cache as a precaution. NFS callers should
// use WriteInodeAtPath instead to update the cache entry in place.
func (c *nativeClient) WriteInodeAt(ctx context.Context, inode uint64, payload []byte, off int64) error {
	return c.WriteInodeAtPath(ctx, inode, "", payload, off)
}

// WriteInodeAtPath writes `payload` at `off` into the file with the given
// inode. When `path` is non-empty, the updated metadata is cached under that
// path and the entire path cache is preserved (no prefix invalidation).
//
// The read-modify-write cycle is compressed to two Redis round trips:
//  1. one HMGET to load metadata + content together, and
//  2. one pipeline containing HSET (new metadata + content) + HINCRBY
//     (total_data_bytes) + SET (root dirty marker).
//
// Previously this function did 5 sequential Redis round trips and wiped the
// entire attribute cache on every write, which was the dominant cost for
// Claude Code's jsonl append pattern.
func (c *nativeClient) WriteInodeAtPath(ctx context.Context, inode uint64, path string, payload []byte, off int64) error {
	if off < 0 {
		return errors.New("invalid offset")
	}

	id := strconv.FormatUint(inode, 10)
	data, err := c.loadInodeWithContentByID(ctx, id)
	if err != nil {
		return err
	}
	if data == nil {
		return errors.New("no such file or directory")
	}
	if data.Type != "file" {
		return errors.New("not a file")
	}

	buf := []byte(data.Content)
	end := off + int64(len(payload))
	if end > int64(len(buf)) {
		grown := make([]byte, end)
		copy(grown, buf)
		buf = grown
	}
	copy(buf[off:end], payload)

	delta := int64(len(buf)) - data.Size
	data.Content = string(buf)
	data.Size = int64(len(buf))
	now := nowMs()
	data.MtimeMs = now
	data.AtimeMs = now
	if data.ContentRef == "" {
		data.ContentRef = "ext"
	}

	// For external content, write content to STRING key and metadata to
	// HASH. includeContent=false because content_ref="ext" skips the
	// content field in inodeFields.
	var fields map[string]interface{}
	if path != "" {
		fields = c.inodeFieldsAtPath(data, path, false)
	} else {
		fields = c.inodeFields(data, false)
	}

	pipe := c.rdb.Pipeline()
	pipe.Set(ctx, c.keys.content(data.ID), data.Content, 0)
	pipe.HSet(ctx, c.keys.inode(data.ID), fields)
	if delta != 0 {
		pipe.HIncrBy(ctx, c.keys.info(), "total_data_bytes", delta)
	}
	pipe.Set(ctx, c.keys.rootDirty(), "1", 0)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return err
	}

	if path != "" {
		// Known path: update the cache entry in place so subsequent stat/read
		// traffic stays warm. macOS NFS tends to burst GETATTR/LOOKUP
		// immediately after a WRITE, so this matters a lot.
		c.cachePath(path, data)
		// Peers still need their copies (path entry + kernel page cache)
		// dropped — broadcast a content-op for the path.
		c.publishInvalidate(ctx, InvalidateOpContent, path)
	} else {
		// Unknown path (legacy caller): fall back to the old defensive
		// invalidation to avoid serving stale sizes from cached entries.
		c.invalidatePrefix(ctx, "/")
	}
	return nil
}

// TruncateInode is the legacy entry point. Prefer TruncateInodeAtPath from
// the NFS layer so the path cache survives the truncate.
func (c *nativeClient) TruncateInode(ctx context.Context, inode uint64, size int64) error {
	return c.TruncateInodeAtPath(ctx, inode, "", size)
}

// TruncateInodeAtPath truncates a file to `size` bytes using the same
// two-round-trip / in-place cache update pattern as WriteInodeAtPath.
func (c *nativeClient) TruncateInodeAtPath(ctx context.Context, inode uint64, path string, size int64) error {
	if size < 0 {
		return errors.New("invalid size")
	}

	id := strconv.FormatUint(inode, 10)
	data, err := c.loadInodeWithContentByID(ctx, id)
	if err != nil {
		return err
	}
	if data == nil {
		return errors.New("no such file or directory")
	}
	if data.Type != "file" {
		return errors.New("not a file")
	}

	buf := []byte(data.Content)
	if int64(len(buf)) > size {
		buf = buf[:size]
	} else if int64(len(buf)) < size {
		grown := make([]byte, size)
		copy(grown, buf)
		buf = grown
	}

	delta := int64(len(buf)) - data.Size
	data.Content = string(buf)
	data.Size = int64(len(buf))
	now := nowMs()
	data.MtimeMs = now
	data.AtimeMs = now
	if data.ContentRef == "" {
		data.ContentRef = "ext"
	}

	var fields map[string]interface{}
	if path != "" {
		fields = c.inodeFieldsAtPath(data, path, false)
	} else {
		fields = c.inodeFields(data, false)
	}

	pipe := c.rdb.Pipeline()
	pipe.Set(ctx, c.keys.content(data.ID), data.Content, 0)
	pipe.HSet(ctx, c.keys.inode(data.ID), fields)
	if delta != 0 {
		pipe.HIncrBy(ctx, c.keys.info(), "total_data_bytes", delta)
	}
	pipe.Set(ctx, c.keys.rootDirty(), "1", 0)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return err
	}

	if path != "" {
		c.cachePath(path, data)
		c.publishInvalidate(ctx, InvalidateOpContent, path)
	} else {
		c.invalidatePrefix(ctx, "/")
	}
	return nil
}
