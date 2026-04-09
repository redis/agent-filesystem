package client

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/redis/agent-filesystem/mount/internal/cache"
	"github.com/redis/go-redis/v9"
)

const maxSymlinkDepth = 40

type nativeClient struct {
	rdb   *redis.Client
	key   string
	keys  keyBuilder
	cache *cache.Cache
}

type inodeData struct {
	ID      string
	Parent  string
	Name    string
	Type    string
	Mode    uint32
	UID     uint32
	GID     uint32
	Size    int64
	CtimeMs int64
	MtimeMs int64
	AtimeMs int64
	Target  string
	Content string
}

func newNativeClient(rdb *redis.Client, key string) Client {
	return &nativeClient{
		rdb:  rdb,
		key:  key,
		keys: newKeyBuilder(key),
	}
}

func newNativeClientWithCache(rdb *redis.Client, key string, ttl time.Duration) Client {
	return &nativeClient{
		rdb:   rdb,
		key:   key,
		keys:  newKeyBuilder(key),
		cache: cache.New(ttl),
	}
}

func (c *nativeClient) invalidateInode(p string) {
	if c.cache != nil {
		c.cache.Invalidate(p)
	}
}

func (c *nativeClient) invalidatePrefix(prefix string) {
	if c.cache != nil {
		c.cache.InvalidatePrefix(prefix)
	}
}

func (c *nativeClient) Stat(ctx context.Context, p string) (*StatResult, error) {
	resolved, inode, err := c.resolvePath(ctx, p, false)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	c.cachePath(resolved, inode)
	return inode.toStat(), nil
}

func (c *nativeClient) Cat(ctx context.Context, p string) ([]byte, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return nil, err
	}
	if inode.Type != "file" {
		return nil, errors.New("not a file")
	}
	content, err := c.loadContentByID(ctx, inode.ID)
	if err != nil {
		return nil, err
	}
	inode.AtimeMs = nowMs()
	_ = c.saveInodeMeta(ctx, resolved, inode)
	return []byte(content), nil
}

func (c *nativeClient) Echo(ctx context.Context, p string, data []byte) error {
	return c.writeFile(ctx, p, data, false)
}

func (c *nativeClient) EchoCreate(ctx context.Context, p string, data []byte, mode uint32) error {
	return c.writeFileWithMode(ctx, p, data, mode)
}

func (c *nativeClient) EchoAppend(ctx context.Context, p string, data []byte) error {
	return c.writeFile(ctx, p, data, true)
}

func (c *nativeClient) Touch(ctx context.Context, p string) error {
	p = normalizePath(p)
	if p == "/" {
		return errors.New("cannot write to root")
	}
	if err := c.ensureParents(ctx, p); err != nil {
		return err
	}

	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			_, _, err := c.CreateFile(ctx, p, 0o644, false)
			return err
		}
		return err
	}
	if inode.Type != "file" {
		return errors.New("not a file")
	}
	now := nowMs()
	inode.MtimeMs = now
	inode.AtimeMs = now
	if err := c.saveInodeMeta(ctx, resolved, inode); err != nil {
		return err
	}
	return c.markRootDirty(ctx)
}

func (c *nativeClient) CreateFile(ctx context.Context, p string, mode uint32, exclusive bool) (*StatResult, bool, error) {
	p = normalizePath(p)
	if p == "/" {
		return nil, false, errors.New("cannot write to root")
	}
	if err := c.ensureParents(ctx, p); err != nil {
		return nil, false, err
	}

	inode, created, err := c.createFileIfMissing(ctx, p, "", mode, exclusive)
	if err != nil {
		return nil, false, err
	}
	c.cachePath(p, inode)
	if created {
		if err := c.markRootDirty(ctx); err != nil {
			return nil, false, err
		}
	}
	return inode.toStat(), created, nil
}

func (c *nativeClient) Mkdir(ctx context.Context, p string) error {
	p = normalizePath(p)
	if p == "/" {
		return c.ensureRoot(ctx)
	}
	if err := c.ensureParents(ctx, p); err != nil {
		return err
	}
	existing, err := c.loadInode(ctx, p)
	if err != nil {
		return err
	}
	if existing != nil {
		if existing.Type == "dir" {
			return nil
		}
		return errors.New("already exists")
	}
	if err := c.createDir(ctx, p, 0o755); err != nil {
		return err
	}
	return c.markRootDirty(ctx)
}

func (c *nativeClient) Rm(ctx context.Context, p string) error {
	p = normalizePath(p)
	if p == "/" {
		return errors.New("cannot remove root")
	}
	resolved, inode, err := c.resolvePath(ctx, p, false)
	if err != nil {
		return err
	}
	if inode.Type == "dir" {
		children, err := c.loadDirEntries(ctx, inode.ID)
		if err != nil {
			return err
		}
		if len(children) > 0 {
			return errors.New("directory not empty")
		}
	}

	c.invalidateInode(resolved)
	pipe := c.rdb.Pipeline()
	pipe.Del(ctx, c.keys.inode(inode.ID))
	if inode.Type == "dir" {
		pipe.Del(ctx, c.keys.dirents(inode.ID))
	}
	parentPath := parentOf(resolved)
	if inode.Parent != "" {
		pipe.HDel(ctx, c.keys.dirents(inode.Parent), inode.Name)
		c.queueTouchTimes(pipe, inode.Parent, nowMs())
	}
	c.queueDeleteInfo(pipe, inode)
	_, err = pipe.Exec(ctx)
	c.invalidateInode(parentPath)
	if err != nil {
		return err
	}
	return c.markRootDirty(ctx)
}

func (c *nativeClient) Ls(ctx context.Context, p string) ([]string, error) {
	entries, err := c.LsLong(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name)
	}
	return out, nil
}

func (c *nativeClient) LsLong(ctx context.Context, p string) ([]LsEntry, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return nil, err
	}
	if inode.Type != "dir" {
		return nil, errors.New("not a directory")
	}

	children, err := c.listDirChildren(ctx, resolved, inode)
	if err != nil {
		return nil, err
	}

	out := make([]LsEntry, 0, len(children))
	for _, child := range children {
		out = append(out, LsEntry{
			Inode: inodeUint64(child.Inode.ID),
			Name:  child.Name,
			Type:  child.Inode.Type,
			Mode:  child.Inode.Mode,
			UID:   child.Inode.UID,
			GID:   child.Inode.GID,
			Size:  child.Inode.Size,
			Mtime: child.Inode.MtimeMs,
		})
	}
	return out, nil
}

func (c *nativeClient) Rename(ctx context.Context, src, dst string, flags uint32) error {
	src = normalizePath(src)
	dst = normalizePath(dst)
	if src == "/" {
		return errors.New("cannot move root")
	}
	if src == dst {
		return nil
	}
	if flags&RenameExchange != 0 {
		return errors.New("operation not supported")
	}
	if flags&^RenameNoreplace != 0 {
		return errors.New("operation not supported")
	}

	resolvedSrc, srcInode, err := c.resolvePath(ctx, src, false)
	if err != nil {
		return err
	}
	if srcInode.Type == "dir" && (dst == resolvedSrc || strings.HasPrefix(dst, resolvedSrc+"/")) {
		return errors.New("cannot move a directory into its own subtree")
	}
	resolvedParent, newParent, err := c.resolvePath(ctx, parentOf(dst), true)
	if err != nil {
		return err
	}
	_ = resolvedParent
	if newParent.Type != "dir" {
		return errors.New("parent path conflict")
	}

	if err := c.renamePath(ctx, resolvedSrc, srcInode, dst, newParent, flags); err != nil {
		return err
	}
	return c.markRootDirty(ctx)
}

func (c *nativeClient) Mv(ctx context.Context, src, dst string) error {
	return c.Rename(ctx, src, dst, 0)
}

func (c *nativeClient) Ln(ctx context.Context, target, linkpath string) error {
	linkpath = normalizePath(linkpath)
	if linkpath == "/" {
		return errors.New("already exists")
	}
	if err := c.ensureParents(ctx, linkpath); err != nil {
		return err
	}
	existing, err := c.loadInode(ctx, linkpath)
	if err != nil {
		return err
	}
	if existing != nil {
		return errors.New("already exists")
	}
	now := nowMs()
	inode := &inodeData{
		Type:    "symlink",
		Mode:    0o777,
		UID:     0,
		GID:     0,
		Size:    int64(len(target)),
		CtimeMs: now,
		MtimeMs: now,
		AtimeMs: now,
		Target:  target,
	}
	if err := c.createInodeAtPath(ctx, linkpath, inode, false); err != nil {
		return err
	}
	return c.markRootDirty(ctx)
}

func (c *nativeClient) Readlink(ctx context.Context, p string) (string, error) {
	_, inode, err := c.resolvePath(ctx, p, false)
	if err != nil {
		return "", err
	}
	if inode.Type != "symlink" {
		return "", errors.New("not a symlink")
	}
	return inode.Target, nil
}

func (c *nativeClient) Chmod(ctx context.Context, p string, mode uint32) error {
	resolved, inode, err := c.resolvePath(ctx, p, false)
	if err != nil {
		return err
	}
	inode.Mode = mode
	if err := c.saveInodeMeta(ctx, resolved, inode); err != nil {
		return err
	}
	return c.markRootDirty(ctx)
}

func (c *nativeClient) Chown(ctx context.Context, p string, uid, gid uint32) error {
	resolved, inode, err := c.resolvePath(ctx, p, false)
	if err != nil {
		return err
	}
	inode.UID = uid
	inode.GID = gid
	if err := c.saveInodeMeta(ctx, resolved, inode); err != nil {
		return err
	}
	return c.markRootDirty(ctx)
}

func (c *nativeClient) Truncate(ctx context.Context, p string, size int64) error {
	if size < 0 {
		return errors.New("invalid size")
	}
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return err
	}
	if inode.Type != "file" {
		return errors.New("not a file")
	}

	var content []byte
	if size > 0 {
		raw, err := c.loadContentByID(ctx, inode.ID)
		if err != nil {
			return err
		}
		content = []byte(raw)
	}
	if int64(len(content)) > size {
		content = content[:size]
	} else if int64(len(content)) < size {
		newBuf := make([]byte, size)
		copy(newBuf, content)
		content = newBuf
	}

	delta := int64(len(content)) - inode.Size
	inode.Content = string(content)
	inode.Size = int64(len(content))
	now := nowMs()
	inode.MtimeMs = now
	inode.AtimeMs = now
	if err := c.saveInode(ctx, resolved, inode); err != nil {
		return err
	}
	if err := c.adjustTotalData(ctx, delta); err != nil {
		return err
	}
	return c.markRootDirty(ctx)
}

func (c *nativeClient) Utimens(ctx context.Context, p string, atimeMs, mtimeMs int64) error {
	resolved, inode, err := c.resolvePath(ctx, p, false)
	if err != nil {
		return err
	}
	if atimeMs >= 0 {
		inode.AtimeMs = atimeMs
	}
	if mtimeMs >= 0 {
		inode.MtimeMs = mtimeMs
	}
	if err := c.saveInodeMeta(ctx, resolved, inode); err != nil {
		return err
	}
	return c.markRootDirty(ctx)
}

func (c *nativeClient) Info(ctx context.Context) (*InfoResult, error) {
	values, err := c.rdb.HGetAll(ctx, c.keys.info()).Result()
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return &InfoResult{}, nil
	}
	files := parseInt64OrZero(values["files"])
	dirs := parseInt64OrZero(values["directories"])
	symlinks := parseInt64OrZero(values["symlinks"])
	totalData := parseInt64OrZero(values["total_data_bytes"])
	return &InfoResult{
		Files:          files,
		Directories:    dirs,
		Symlinks:       symlinks,
		TotalDataBytes: totalData,
		TotalInodes:    files + dirs + symlinks,
	}, nil
}

func (c *nativeClient) writeFileWithMode(ctx context.Context, p string, data []byte, mode uint32) error {
	p = normalizePath(p)
	if p == "/" {
		return errors.New("cannot write to root")
	}
	if err := c.ensureParents(ctx, p); err != nil {
		return err
	}
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return c.createFile(ctx, p, string(data), mode)
		}
		return err
	}
	if inode.Type != "file" {
		return errors.New("not a file")
	}
	inode.Content = string(data)
	inode.Size = int64(len(inode.Content))
	inode.Mode = mode
	now := nowMs()
	inode.MtimeMs = now
	inode.AtimeMs = now
	if err := c.saveInode(ctx, resolved, inode); err != nil {
		return err
	}
	return c.markRootDirty(ctx)
}

func (c *nativeClient) writeFile(ctx context.Context, p string, data []byte, appendMode bool) error {
	p = normalizePath(p)
	if p == "/" {
		return errors.New("cannot write to root")
	}
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			if err := c.ensureParents(ctx, p); err != nil {
				return err
			}
			return c.createFile(ctx, p, string(data), 0o644)
		}
		return err
	}
	if inode.Type != "file" {
		return errors.New("not a file")
	}

	before := inode.Size
	if appendMode {
		existing, err := c.loadContentByID(ctx, inode.ID)
		if err != nil {
			return err
		}
		inode.Content = existing + string(data)
	} else {
		inode.Content = string(data)
	}
	inode.Size = int64(len(inode.Content))
	now := nowMs()
	inode.MtimeMs = now
	inode.AtimeMs = now
	if err := c.saveInode(ctx, resolved, inode); err != nil {
		return err
	}
	if err := c.adjustTotalData(ctx, inode.Size-before); err != nil {
		return err
	}
	return c.markRootDirty(ctx)
}

func (c *nativeClient) createFile(ctx context.Context, p string, content string, mode uint32) error {
	_, _, err := c.createFileIfMissing(ctx, p, content, mode, false)
	if err != nil {
		return err
	}
	return c.markRootDirty(ctx)
}

func (c *nativeClient) createDir(ctx context.Context, p string, mode uint32) error {
	if err := c.ensureParents(ctx, p); err != nil {
		return err
	}
	return c.createDirNoParents(ctx, p, mode)
}

func (c *nativeClient) createDirNoParents(ctx context.Context, p string, mode uint32) error {
	now := nowMs()
	return c.createInodeAtPath(ctx, p, &inodeData{
		Type:    "dir",
		Mode:    mode,
		UID:     0,
		GID:     0,
		Size:    0,
		CtimeMs: now,
		MtimeMs: now,
		AtimeMs: now,
	}, false)
}

func (i *inodeData) toStat() *StatResult {
	return &StatResult{
		Inode: inodeUint64(i.ID),
		Type:  i.Type,
		Mode:  i.Mode,
		UID:   i.UID,
		GID:   i.GID,
		Size:  i.Size,
		Ctime: i.CtimeMs,
		Mtime: i.MtimeMs,
		Atime: i.AtimeMs,
	}
}

func sortNames(values map[string]string) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
