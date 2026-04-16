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
	if size <= 0 {
		return []byte{}, nil
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
	if off >= data.Size {
		return []byte{}, nil
	}
	if data.ContentRef != "ext" {
		content, err := c.loadContentExternal(ctx, data.ID, data.ContentRef)
		if err != nil {
			return nil, err
		}
		end := off + int64(size)
		if end > int64(len(content)) {
			end = int64(len(content))
		}
		return []byte(content[off:end]), nil
	}

	end := off + int64(size) - 1
	if end >= data.Size {
		end = data.Size - 1
	}
	chunk, err := c.rdb.GetRange(ctx, c.keys.content(data.ID), off, end).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	return []byte(chunk), nil
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
// External-content files now stay on the byte-range path: we use GETRANGE /
// SETRANGE against afs:{fs}:content:{inode} and only touch inode metadata in
// the HASH. Small files still refresh search fields from the updated content;
// large files flip to the "large" search state without reloading the whole
// object into Go.
func (c *nativeClient) WriteInodeAtPath(ctx context.Context, inode uint64, path string, payload []byte, off int64) error {
	if off < 0 {
		return errors.New("invalid offset")
	}

	id := strconv.FormatUint(inode, 10)
	data, err := c.loadInodeByID(ctx, id)
	if err != nil {
		return err
	}
	if data == nil {
		return errors.New("no such file or directory")
	}
	if data.Type != "file" {
		return errors.New("not a file")
	}
	if err := c.ensureExternalContentForRangeIO(ctx, data); err != nil {
		return err
	}

	oldSize := data.Size
	newSize := oldSize
	if len(payload) > 0 {
		end := off + int64(len(payload))
		if end > newSize {
			newSize = end
		}
	}
	delta := newSize - oldSize
	now := nowMs()
	data.Size = newSize
	data.MtimeMs = now
	data.AtimeMs = now
	data.ContentRef = "ext"

	fields := c.inodeFields(data, false)
	if path != "" {
		fields = c.inodeFieldsAtPath(data, path, false)
	}

	if len(payload) == 0 {
		return c.finishRangeWrite(ctx, data, path, mergeFieldMaps(fields), delta)
	}

	var searchFields map[string]interface{}
	if newSize <= fileSearchMaxIndexedBytes {
		before := ""
		if oldSize > 0 {
			pipe := c.rdb.Pipeline()
			beforeCmd := pipe.GetRange(ctx, c.keys.content(data.ID), 0, oldSize-1)
			pipe.SetRange(ctx, c.keys.content(data.ID), off, string(payload))
			if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
				return err
			}
			before = beforeCmd.Val()
		} else if err := c.rdb.SetRange(ctx, c.keys.content(data.ID), off, string(payload)).Err(); err != nil {
			return err
		}
		searchFields = fileSearchIndexFields(applyRangeWrite(before, payload, off))
	} else {
		searchFields = map[string]interface{}{
			"search_state":  fileSearchStateLarge,
			"grep_grams_ci": "",
		}
		pipe := c.rdb.Pipeline()
		pipe.SetRange(ctx, c.keys.content(data.ID), off, string(payload))
		pipe.HSet(ctx, c.keys.inode(data.ID), mergeFieldMaps(fields, searchFields))
		if delta != 0 {
			pipe.HIncrBy(ctx, c.keys.info(), "total_data_bytes", delta)
		}
		pipe.Set(ctx, c.keys.rootDirty(), "1", 0)
		if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		return c.finishRangeWriteCache(ctx, data, path)
	}

	return c.finishRangeWrite(ctx, data, path, mergeFieldMaps(fields, searchFields), delta)
}

// TruncateInode is the legacy entry point. Prefer TruncateInodeAtPath from
// the NFS layer so the path cache survives the truncate.
func (c *nativeClient) TruncateInode(ctx context.Context, inode uint64, size int64) error {
	return c.TruncateInodeAtPath(ctx, inode, "", size)
}

// TruncateInodeAtPath updates the external content key in-place when the file
// grows, and rewrites only the kept prefix when it shrinks.
func (c *nativeClient) TruncateInodeAtPath(ctx context.Context, inode uint64, path string, size int64) error {
	if size < 0 {
		return errors.New("invalid size")
	}

	id := strconv.FormatUint(inode, 10)
	data, err := c.loadInodeByID(ctx, id)
	if err != nil {
		return err
	}
	if data == nil {
		return errors.New("no such file or directory")
	}
	if data.Type != "file" {
		return errors.New("not a file")
	}
	if err := c.ensureExternalContentForRangeIO(ctx, data); err != nil {
		return err
	}

	delta := size - data.Size
	oldSize := data.Size
	data.Size = size
	now := nowMs()
	data.MtimeMs = now
	data.AtimeMs = now
	data.ContentRef = "ext"

	fields := c.inodeFields(data, false)
	if path != "" {
		fields = c.inodeFieldsAtPath(data, path, false)
	}

	if delta == 0 {
		return c.finishRangeWrite(ctx, data, path, mergeFieldMaps(fields), 0)
	}

	var searchFields map[string]interface{}
	switch {
	case size < oldSize:
		truncated := ""
		if size > 0 {
			var err error
			truncated, err = c.rdb.GetRange(ctx, c.keys.content(data.ID), 0, size-1).Result()
			if err != nil && !errors.Is(err, redis.Nil) {
				return err
			}
		}
		if err := c.rdb.Set(ctx, c.keys.content(data.ID), truncated, 0).Err(); err != nil {
			return err
		}
		if size <= fileSearchMaxIndexedBytes {
			searchFields = fileSearchIndexFields(truncated)
		} else {
			searchFields = map[string]interface{}{
				"search_state":  fileSearchStateLarge,
				"grep_grams_ci": "",
			}
		}
	case size <= fileSearchMaxIndexedBytes:
		before := ""
		if oldSize > 0 {
			pipe := c.rdb.Pipeline()
			beforeCmd := pipe.GetRange(ctx, c.keys.content(data.ID), 0, oldSize-1)
			pipe.SetRange(ctx, c.keys.content(data.ID), size-1, "\x00")
			if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
				return err
			}
			before = beforeCmd.Val()
		} else if err := c.rdb.SetRange(ctx, c.keys.content(data.ID), size-1, "\x00").Err(); err != nil {
			return err
		}
		searchFields = fileSearchIndexFields(resizeRangeContent(before, size))
	default:
		pipe := c.rdb.Pipeline()
		pipe.SetRange(ctx, c.keys.content(data.ID), size-1, "\x00")
		pipe.HSet(ctx, c.keys.inode(data.ID), mergeFieldMaps(fields, map[string]interface{}{
			"search_state":  fileSearchStateLarge,
			"grep_grams_ci": "",
		}))
		pipe.HIncrBy(ctx, c.keys.info(), "total_data_bytes", delta)
		pipe.Set(ctx, c.keys.rootDirty(), "1", 0)
		if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		return c.finishRangeWriteCache(ctx, data, path)
	}

	return c.finishRangeWrite(ctx, data, path, mergeFieldMaps(fields, searchFields), delta)
}

func (c *nativeClient) ensureExternalContentForRangeIO(ctx context.Context, inode *inodeData) error {
	if inode == nil || inode.Type != "file" || inode.ContentRef == "ext" {
		return nil
	}
	content, err := c.rdb.HGet(ctx, c.keys.inode(inode.ID), "content").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	pipe := c.rdb.Pipeline()
	pipe.Set(ctx, c.keys.content(inode.ID), content, 0)
	pipe.HSet(ctx, c.keys.inode(inode.ID), mergeFieldMaps(map[string]interface{}{
		"content_ref": "ext",
	}, fileSearchIndexFields(content)))
	pipe.HDel(ctx, c.keys.inode(inode.ID), "content")
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	inode.ContentRef = "ext"
	return nil
}

func applyRangeWrite(existing string, payload []byte, off int64) string {
	end := off + int64(len(payload))
	buf := []byte(existing)
	if end > int64(len(buf)) {
		grown := make([]byte, end)
		copy(grown, buf)
		buf = grown
	}
	copy(buf[off:end], payload)
	return string(buf)
}

func resizeRangeContent(existing string, size int64) string {
	buf := []byte(existing)
	switch {
	case int64(len(buf)) > size:
		return string(buf[:size])
	case int64(len(buf)) < size:
		grown := make([]byte, size)
		copy(grown, buf)
		return string(grown)
	default:
		return existing
	}
}

func (c *nativeClient) finishRangeWrite(ctx context.Context, data *inodeData, path string, fields map[string]interface{}, delta int64) error {
	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, c.keys.inode(data.ID), fields)
	if delta != 0 {
		pipe.HIncrBy(ctx, c.keys.info(), "total_data_bytes", delta)
	}
	pipe.Set(ctx, c.keys.rootDirty(), "1", 0)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	return c.finishRangeWriteCache(ctx, data, path)
}

func (c *nativeClient) finishRangeWriteCache(ctx context.Context, data *inodeData, path string) error {
	if path != "" {
		c.cachePath(path, data)
		c.publishInvalidate(ctx, InvalidateOpContent, path)
		return nil
	}
	c.invalidatePrefix(ctx, "/")
	return nil
}
