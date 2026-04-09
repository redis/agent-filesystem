package client

import (
	"context"
	"errors"
	"strconv"
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
	content, err := c.loadContentByID(ctx, data.ID)
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

func (c *nativeClient) WriteInodeAt(ctx context.Context, inode uint64, payload []byte, off int64) error {
	if off < 0 {
		return errors.New("invalid offset")
	}
	data, err := c.loadInodeByID(ctx, strconv.FormatUint(inode, 10))
	if err != nil {
		return err
	}
	if data == nil {
		return errors.New("no such file or directory")
	}
	if data.Type != "file" {
		return errors.New("not a file")
	}
	content, err := c.loadContentByID(ctx, data.ID)
	if err != nil {
		return err
	}

	buf := []byte(content)
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
	if err := c.saveInodeDirect(ctx, data, true); err != nil {
		return err
	}
	// Inode-addressed writes bypass path lookups, so any cached path attrs can
	// still reflect the pre-write size. Drop them before a follow-up metadata
	// call rewrites the stale size back into Redis.
	c.invalidatePrefix("/")
	if err := c.adjustTotalData(ctx, delta); err != nil {
		return err
	}
	return c.markRootDirty(ctx)
}

func (c *nativeClient) TruncateInode(ctx context.Context, inode uint64, size int64) error {
	if size < 0 {
		return errors.New("invalid size")
	}
	data, err := c.loadInodeByID(ctx, strconv.FormatUint(inode, 10))
	if err != nil {
		return err
	}
	if data == nil {
		return errors.New("no such file or directory")
	}
	if data.Type != "file" {
		return errors.New("not a file")
	}
	content, err := c.loadContentByID(ctx, data.ID)
	if err != nil {
		return err
	}
	buf := []byte(content)
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
	if err := c.saveInodeDirect(ctx, data, true); err != nil {
		return err
	}
	// Keep path-based metadata operations from persisting a stale cached size
	// after an inode-addressed truncate.
	c.invalidatePrefix("/")
	if err := c.adjustTotalData(ctx, delta); err != nil {
		return err
	}
	return c.markRootDirty(ctx)
}
