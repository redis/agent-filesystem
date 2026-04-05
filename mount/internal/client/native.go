package client

import (
	"context"
	"errors"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/redis-fs/mount/internal/cache"
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

func (c *nativeClient) Stat(ctx context.Context, p string) (*StatResult, error) {
	resolved, inode, err := c.resolvePath(ctx, p, false)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	_ = resolved
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
	content, err := c.loadContent(ctx, resolved)
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
			return c.createFile(ctx, p, "", 0o644)
		}
		return err
	}
	if inode.Type != "file" {
		return errors.New("not a file")
	}
	now := nowMs()
	inode.MtimeMs = now
	inode.AtimeMs = now
	return c.saveInodeMeta(ctx, resolved, inode)
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
	return c.createDir(ctx, p, 0o755)
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
		n, err := c.rdb.SCard(ctx, c.keys.children(resolved)).Result()
		if err != nil {
			return err
		}
		if n > 0 {
			return errors.New("directory not empty")
		}
	}

	c.invalidateInode(resolved)
	pipe := c.rdb.Pipeline()
	pipe.Del(ctx, c.keys.inode(resolved))
	if inode.Type == "dir" {
		pipe.Del(ctx, c.keys.children(resolved))
	}
	parent := parentOf(resolved)
	if parent != resolved {
		pipe.SRem(ctx, c.keys.children(parent), baseName(resolved))
	}
	switch inode.Type {
	case "file":
		pipe.HIncrBy(ctx, c.keys.info(), "files", -1)
		if inode.Size > 0 {
			pipe.HIncrBy(ctx, c.keys.info(), "total_data_bytes", -inode.Size)
		}
	case "dir":
		pipe.HIncrBy(ctx, c.keys.info(), "directories", -1)
	case "symlink":
		pipe.HIncrBy(ctx, c.keys.info(), "symlinks", -1)
	}
	_, err = pipe.Exec(ctx)
	return err
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

	names, err := c.rdb.SMembers(ctx, c.keys.children(resolved)).Result()
	if err != nil {
		return nil, err
	}
	sort.Strings(names)

	// Pipeline HMGet calls — fetch only the 4 fields needed for listing,
	// NOT content (which can be tens of MB for large files).
	pipe := c.rdb.Pipeline()
	cmds := make([]*redis.SliceCmd, len(names))
	childPaths := make([]string, len(names))
	for i, name := range names {
		childPaths[i] = joinPath(resolved, name)
		cmds[i] = pipe.HMGet(ctx, c.keys.inode(childPaths[i]), "type", "mode", "size", "mtime_ms")
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}

	out := make([]LsEntry, 0, len(names))
	for i, name := range names {
		vals, err := cmds[i].Result()
		if err != nil || len(vals) < 4 || vals[0] == nil {
			continue
		}
		out = append(out, LsEntry{
			Name:  name,
			Type:  toStr(vals[0]),
			Mode:  uint32(toInt(vals[1])),
			Size:  toInt(vals[2]),
			Mtime: toInt(vals[3]),
		})
	}
	return out, nil
}

func (c *nativeClient) Mv(ctx context.Context, src, dst string) error {
	src = normalizePath(src)
	dst = normalizePath(dst)
	if src == "/" {
		return errors.New("cannot move root")
	}
	if src == dst {
		return nil
	}

	if _, _, err := c.resolvePath(ctx, dst, false); err == nil {
		return errors.New("already exists")
	} else if !errors.Is(err, redis.Nil) {
		return err
	}
	resolvedSrc, srcInode, err := c.resolvePath(ctx, src, false)
	if err != nil {
		return err
	}
	if srcInode.Type == "dir" && (dst == resolvedSrc || strings.HasPrefix(dst, resolvedSrc+"/")) {
		return errors.New("cannot move a directory into its own subtree")
	}
	if err := c.ensureParents(ctx, dst); err != nil {
		return err
	}

	moved, err := c.collectSubtreePaths(ctx, resolvedSrc)
	if err != nil {
		return err
	}

	// Phase 1: pipeline-read all inodes and children sets.
	readPipe := c.rdb.Pipeline()
	inodeReadCmds := make([]*redis.MapStringStringCmd, len(moved))
	childrenReadCmds := make([]*redis.StringSliceCmd, len(moved))
	for i, oldPath := range moved {
		inodeReadCmds[i] = readPipe.HGetAll(ctx, c.keys.inode(oldPath))
		childrenReadCmds[i] = readPipe.SMembers(ctx, c.keys.children(oldPath))
	}
	if _, err := readPipe.Exec(ctx); err != nil && err != redis.Nil {
		return err
	}

	// Phase 2: pipeline-write all new inodes + children, delete old keys, update parents.
	oldParent := parentOf(resolvedSrc)
	newParent := parentOf(dst)
	writePipe := c.rdb.Pipeline()
	for i, oldPath := range moved {
		newPath := dst + strings.TrimPrefix(oldPath, resolvedSrc)
		values, err := inodeReadCmds[i].Result()
		if err != nil || len(values) == 0 {
			continue
		}
		writePipe.HSet(ctx, c.keys.inode(newPath), values)
		if values["type"] == "dir" {
			children, err := childrenReadCmds[i].Result()
			if err == nil && len(children) > 0 {
				args := make([]interface{}, len(children))
				for j, child := range children {
					args[j] = child
				}
				writePipe.SAdd(ctx, c.keys.children(newPath), args...)
			}
		}
		writePipe.Del(ctx, c.keys.inode(oldPath), c.keys.children(oldPath))
	}
	writePipe.SRem(ctx, c.keys.children(oldParent), baseName(resolvedSrc))
	writePipe.SAdd(ctx, c.keys.children(newParent), baseName(dst))
	if _, err := writePipe.Exec(ctx); err != nil && err != redis.Nil {
		return err
	}
	if c.cache != nil {
		c.cache.InvalidatePrefix(resolvedSrc)
		c.invalidateInode(dst)
	}
	return nil
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
	if err := c.saveInode(ctx, linkpath, inode); err != nil {
		return err
	}
	if err := c.rdb.SAdd(ctx, c.keys.children(parentOf(linkpath)), baseName(linkpath)).Err(); err != nil {
		return err
	}
	return c.adjustInfoForCreate(ctx, inode)
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
	return c.saveInodeMeta(ctx, resolved, inode)
}

func (c *nativeClient) Chown(ctx context.Context, p string, uid, gid uint32) error {
	resolved, inode, err := c.resolvePath(ctx, p, false)
	if err != nil {
		return err
	}
	inode.UID = uid
	inode.GID = gid
	return c.saveInodeMeta(ctx, resolved, inode)
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
	// Load content only when we actually need to truncate it.
	var content []byte
	if size > 0 {
		raw, err := c.loadContent(ctx, resolved)
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
	return c.adjustTotalData(ctx, delta)
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
	return c.saveInodeMeta(ctx, resolved, inode)
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

// ---------------------------------------------------------------------------
// Text-processing commands (Phase 2)
// ---------------------------------------------------------------------------

func (c *nativeClient) Head(ctx context.Context, p string, n int) (string, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return "", err
	}
	if inode.Type != "file" {
		return "", errors.New("not a file")
	}
	content, err := c.loadContent(ctx, resolved)
	if err != nil {
		return "", err
	}
	inode.AtimeMs = nowMs()
	_ = c.saveInodeMeta(ctx, resolved, inode)

	lines := splitLines(content)
	if n <= 0 {
		n = 10
	}
	if n > len(lines) {
		n = len(lines)
	}
	return joinLines(lines[:n]), nil
}

func (c *nativeClient) Tail(ctx context.Context, p string, n int) (string, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return "", err
	}
	if inode.Type != "file" {
		return "", errors.New("not a file")
	}
	content, err := c.loadContent(ctx, resolved)
	if err != nil {
		return "", err
	}
	inode.AtimeMs = nowMs()
	_ = c.saveInodeMeta(ctx, resolved, inode)

	lines := splitLines(content)
	if n <= 0 {
		n = 10
	}
	if n > len(lines) {
		n = len(lines)
	}
	return joinLines(lines[len(lines)-n:]), nil
}

func (c *nativeClient) Lines(ctx context.Context, p string, start, end int) (string, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return "", err
	}
	if inode.Type != "file" {
		return "", errors.New("not a file")
	}
	content, err := c.loadContent(ctx, resolved)
	if err != nil {
		return "", err
	}
	inode.AtimeMs = nowMs()
	_ = c.saveInodeMeta(ctx, resolved, inode)

	lines := splitLines(content)
	// 1-indexed; end=-1 means EOF
	if start < 1 {
		start = 1
	}
	if end < 0 || end > len(lines) {
		end = len(lines)
	}
	if start > len(lines) {
		return "", nil
	}
	return joinLines(lines[start-1 : end]), nil
}

func (c *nativeClient) Wc(ctx context.Context, p string) (*WcResult, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return nil, err
	}
	if inode.Type != "file" {
		return nil, errors.New("not a file")
	}
	content, err := c.loadContent(ctx, resolved)
	if err != nil {
		return nil, err
	}
	inode.AtimeMs = nowMs()
	_ = c.saveInodeMeta(ctx, resolved, inode)
	chars := int64(len(content))

	// Count lines: number of \n characters; if content is non-empty and
	// doesn't end with \n, count the last line too.
	lineCount := int64(strings.Count(content, "\n"))
	if chars > 0 && !strings.HasSuffix(content, "\n") {
		lineCount++
	}

	// Count words: whitespace-delimited tokens
	wordCount := int64(len(strings.Fields(content)))

	return &WcResult{
		Lines: lineCount,
		Words: wordCount,
		Chars: chars,
	}, nil
}

func (c *nativeClient) Insert(ctx context.Context, p string, afterLine int, content string) error {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return err
	}
	if inode.Type != "file" {
		return errors.New("not a file")
	}
	existing, err := c.loadContent(ctx, resolved)
	if err != nil {
		return err
	}

	lines := splitLines(existing)

	var newLines []string
	switch {
	case afterLine == 0:
		// Prepend
		newLines = append([]string{content}, lines...)
	case afterLine < 0 || afterLine >= len(lines):
		// Append
		newLines = append(lines, content)
	default:
		// Insert after line N (1-indexed)
		newLines = make([]string, 0, len(lines)+1)
		newLines = append(newLines, lines[:afterLine]...)
		newLines = append(newLines, content)
		newLines = append(newLines, lines[afterLine:]...)
	}

	newContent := joinLines(newLines)
	delta := int64(len(newContent)) - inode.Size
	inode.Content = newContent
	inode.Size = int64(len(newContent))
	now := nowMs()
	inode.MtimeMs = now
	inode.AtimeMs = now
	if err := c.saveInode(ctx, resolved, inode); err != nil {
		return err
	}
	return c.adjustTotalData(ctx, delta)
}

func (c *nativeClient) Replace(ctx context.Context, p string, old, new string, all bool) (int64, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return 0, err
	}
	if inode.Type != "file" {
		return 0, errors.New("not a file")
	}
	existing, err := c.loadContent(ctx, resolved)
	if err != nil {
		return 0, err
	}

	var count int64
	var newContent string
	if all {
		count = int64(strings.Count(existing, old))
		newContent = strings.ReplaceAll(existing, old, new)
	} else {
		if strings.Contains(existing, old) {
			count = 1
			newContent = strings.Replace(existing, old, new, 1)
		} else {
			return 0, nil
		}
	}

	if count == 0 {
		return 0, nil
	}

	delta := int64(len(newContent)) - inode.Size
	inode.Content = newContent
	inode.Size = int64(len(newContent))
	now := nowMs()
	inode.MtimeMs = now
	inode.AtimeMs = now
	if err := c.saveInode(ctx, resolved, inode); err != nil {
		return 0, err
	}
	if err := c.adjustTotalData(ctx, delta); err != nil {
		return 0, err
	}
	return count, nil
}

func (c *nativeClient) DeleteLines(ctx context.Context, p string, start, end int) (int64, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return 0, err
	}
	if inode.Type != "file" {
		return 0, errors.New("not a file")
	}
	existing, err := c.loadContent(ctx, resolved)
	if err != nil {
		return 0, err
	}

	lines := splitLines(existing)
	// 1-indexed
	if start < 1 {
		start = 1
	}
	if end < 0 || end > len(lines) {
		end = len(lines)
	}
	if start > len(lines) {
		return 0, nil
	}

	deleted := int64(end - start + 1)
	newLines := make([]string, 0, len(lines)-int(deleted))
	newLines = append(newLines, lines[:start-1]...)
	newLines = append(newLines, lines[end:]...)

	newContent := joinLines(newLines)
	delta := int64(len(newContent)) - inode.Size
	inode.Content = newContent
	inode.Size = int64(len(newContent))
	now := nowMs()
	inode.MtimeMs = now
	inode.AtimeMs = now
	if err := c.saveInode(ctx, resolved, inode); err != nil {
		return 0, err
	}
	if err := c.adjustTotalData(ctx, delta); err != nil {
		return 0, err
	}
	return deleted, nil
}

// ---------------------------------------------------------------------------
// Recursive/walk commands (Phase 3)
// ---------------------------------------------------------------------------

func (c *nativeClient) Cp(ctx context.Context, src, dst string, recursive bool) error {
	src = normalizePath(src)
	dst = normalizePath(dst)

	resolved, inode, err := c.resolvePath(ctx, src, true)
	if err != nil {
		return err
	}

	if inode.Type == "dir" && !recursive {
		return errors.New("source is a directory — use recursive")
	}

	if _, existing, err := c.resolvePath(ctx, dst, false); err == nil && existing != nil {
		return errors.New("already exists")
	} else if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}

	if err := c.ensureParents(ctx, dst); err != nil {
		return err
	}

	if inode.Type == "file" {
		return c.copyFile(ctx, resolved, dst, inode)
	}
	if inode.Type == "symlink" {
		return c.copySymlink(ctx, dst, inode)
	}

	// Directory: recursive DFS copy
	return c.copyDirRecursive(ctx, resolved, dst)
}

func (c *nativeClient) copyFile(ctx context.Context, srcPath, dstPath string, src *inodeData) error {
	content, err := c.loadContent(ctx, srcPath)
	if err != nil {
		return err
	}
	now := nowMs()
	dst := &inodeData{
		Type:    "file",
		Mode:    src.Mode,
		UID:     src.UID,
		GID:     src.GID,
		Size:    src.Size,
		CtimeMs: now,
		MtimeMs: now,
		AtimeMs: now,
		Content: content,
	}
	if err := c.saveInode(ctx, dstPath, dst); err != nil {
		return err
	}
	if err := c.rdb.SAdd(ctx, c.keys.children(parentOf(dstPath)), baseName(dstPath)).Err(); err != nil {
		return err
	}
	return c.adjustInfoForCreate(ctx, dst)
}

func (c *nativeClient) copySymlink(ctx context.Context, dstPath string, src *inodeData) error {
	now := nowMs()
	dst := &inodeData{
		Type:    "symlink",
		Mode:    src.Mode,
		UID:     src.UID,
		GID:     src.GID,
		Size:    src.Size,
		CtimeMs: now,
		MtimeMs: now,
		AtimeMs: now,
		Target:  src.Target,
	}
	if err := c.saveInode(ctx, dstPath, dst); err != nil {
		return err
	}
	if err := c.rdb.SAdd(ctx, c.keys.children(parentOf(dstPath)), baseName(dstPath)).Err(); err != nil {
		return err
	}
	return c.adjustInfoForCreate(ctx, dst)
}

func (c *nativeClient) copyDirRecursive(ctx context.Context, srcDir, dstDir string) error {
	// Create destination directory
	if err := c.createDir(ctx, dstDir, 0o755); err != nil {
		return err
	}

	// Load source dir's mode
	srcInode, err := c.loadInode(ctx, srcDir)
	if err != nil {
		return err
	}
	if srcInode != nil {
		dstInode, err := c.loadInode(ctx, dstDir)
		if err != nil {
			return err
		}
		if dstInode != nil {
			dstInode.Mode = srcInode.Mode
			dstInode.UID = srcInode.UID
			dstInode.GID = srcInode.GID
			_ = c.saveInode(ctx, dstDir, dstInode)
		}
	}

	children, err := c.rdb.SMembers(ctx, c.keys.children(srcDir)).Result()
	if err != nil {
		return err
	}

	for _, name := range children {
		srcChild := joinPath(srcDir, name)
		dstChild := joinPath(dstDir, name)

		childInode, err := c.loadInode(ctx, srcChild)
		if err != nil {
			return err
		}
		if childInode == nil {
			continue
		}

		switch childInode.Type {
		case "file":
			if err := c.copyFile(ctx, srcChild, dstChild, childInode); err != nil {
				return err
			}
		case "dir":
			if err := c.copyDirRecursive(ctx, srcChild, dstChild); err != nil {
				return err
			}
		case "symlink":
			if err := c.ensureParents(ctx, dstChild); err != nil {
				return err
			}
			if err := c.copySymlink(ctx, dstChild, childInode); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *nativeClient) Tree(ctx context.Context, p string, maxDepth int) ([]TreeEntry, error) {
	if maxDepth <= 0 {
		maxDepth = 64
	}
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return nil, err
	}
	if inode.Type != "dir" {
		return nil, errors.New("not a directory")
	}

	var entries []TreeEntry
	entries = append(entries, TreeEntry{Path: resolved, Type: "dir", Depth: 0})
	if err := c.treeWalk(ctx, resolved, 1, maxDepth, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (c *nativeClient) treeWalk(ctx context.Context, dir string, depth, maxDepth int, entries *[]TreeEntry) error {
	if depth > maxDepth {
		return nil
	}
	children, err := c.rdb.SMembers(ctx, c.keys.children(dir)).Result()
	if err != nil {
		return err
	}
	sort.Strings(children)

	for _, name := range children {
		childPath := joinPath(dir, name)
		inode, err := c.loadInode(ctx, childPath)
		if err != nil {
			return err
		}
		if inode == nil {
			continue
		}
		*entries = append(*entries, TreeEntry{Path: childPath, Type: inode.Type, Depth: depth})
		if inode.Type == "dir" {
			if err := c.treeWalk(ctx, childPath, depth+1, maxDepth, entries); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *nativeClient) Find(ctx context.Context, p, pattern string, typeFilter string) ([]string, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return nil, err
	}
	if inode.Type != "dir" {
		return nil, errors.New("not a directory")
	}

	var matches []string
	if err := c.findWalk(ctx, resolved, pattern, typeFilter, &matches); err != nil {
		return nil, err
	}
	return matches, nil
}

func (c *nativeClient) findWalk(ctx context.Context, dir, pattern, typeFilter string, matches *[]string) error {
	// Check the directory itself
	if typeFilter == "" || typeFilter == "dir" {
		if globMatch(pattern, baseName(dir)) || dir == "/" {
			if dir == "/" && globMatch(pattern, "/") {
				*matches = append(*matches, dir)
			} else if dir != "/" && globMatch(pattern, baseName(dir)) {
				*matches = append(*matches, dir)
			}
		}
	}

	children, err := c.rdb.SMembers(ctx, c.keys.children(dir)).Result()
	if err != nil {
		return err
	}
	sort.Strings(children)

	for _, name := range children {
		childPath := joinPath(dir, name)
		inode, err := c.loadInode(ctx, childPath)
		if err != nil {
			return err
		}
		if inode == nil {
			continue
		}

		if inode.Type == "dir" {
			if err := c.findWalk(ctx, childPath, pattern, typeFilter, matches); err != nil {
				return err
			}
		} else {
			if typeFilter == "" || typeFilter == inode.Type {
				if globMatch(pattern, name) {
					*matches = append(*matches, childPath)
				}
			}
		}
	}
	return nil
}

func (c *nativeClient) Grep(ctx context.Context, p, pattern string, nocase bool) ([]GrepMatch, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return nil, err
	}

	var matches []GrepMatch

	switch inode.Type {
	case "file":
		content, err := c.loadContent(ctx, resolved)
		if err != nil {
			return nil, err
		}
		fileMatches := c.grepFile(resolved, content, pattern, nocase)
		matches = append(matches, fileMatches...)
	case "dir":
		if err := c.grepWalk(ctx, resolved, pattern, nocase, &matches); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("not a file or directory")
	}

	return matches, nil
}

func (c *nativeClient) grepWalk(ctx context.Context, dir, pattern string, nocase bool, matches *[]GrepMatch) error {
	children, err := c.rdb.SMembers(ctx, c.keys.children(dir)).Result()
	if err != nil {
		return err
	}
	sort.Strings(children)

	for _, name := range children {
		childPath := joinPath(dir, name)
		inode, err := c.loadInode(ctx, childPath)
		if err != nil {
			return err
		}
		if inode == nil {
			continue
		}

		switch inode.Type {
		case "file":
			content, err := c.loadContent(ctx, childPath)
			if err != nil {
				return err
			}
			fileMatches := c.grepFile(childPath, content, pattern, nocase)
			*matches = append(*matches, fileMatches...)
		case "dir":
			if err := c.grepWalk(ctx, childPath, pattern, nocase, matches); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *nativeClient) grepFile(filePath string, content string, pattern string, nocase bool) []GrepMatch {

	// Binary detection: NUL in first 8KB
	checkLen := len(content)
	if checkLen > 8192 {
		checkLen = 8192
	}
	if strings.ContainsRune(content[:checkLen], '\x00') {
		// Binary file — check if pattern matches anywhere
		pat := pattern
		text := content
		if nocase {
			pat = strings.ToLower(pat)
			text = strings.ToLower(text)
		}
		if globMatch(pat, text) {
			return []GrepMatch{{Path: filePath, LineNum: 0, Line: "Binary file matches"}}
		}
		return nil
	}

	lines := strings.Split(content, "\n")
	var matches []GrepMatch
	for i, line := range lines {
		pat := pattern
		text := line
		if nocase {
			pat = strings.ToLower(pat)
			text = strings.ToLower(text)
		}
		if globMatch(pat, text) {
			matches = append(matches, GrepMatch{
				Path:    filePath,
				LineNum: int64(i + 1),
				Line:    line,
			})
		}
	}
	return matches
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

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
	c.invalidateInode(resolved)
	fields := map[string]interface{}{
		"type": inode.Type, "mode": inode.Mode,
		"uid": inode.UID, "gid": inode.GID,
		"size": inode.Size,
		"ctime_ms": inode.CtimeMs, "mtime_ms": inode.MtimeMs, "atime_ms": inode.AtimeMs,
		"content": inode.Content,
	}
	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, c.keys.inode(resolved), fields)
	_, err = pipe.Exec(ctx)
	return err
}

func (c *nativeClient) writeFile(ctx context.Context, p string, data []byte, appendMode bool) error {
	p = normalizePath(p)
	if p == "/" {
		return errors.New("cannot write to root")
	}
	// Try resolvePath first: if file exists, parents are guaranteed to exist.
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// New file — need parents.
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
		existing, err := c.loadContent(ctx, resolved)
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
	c.invalidateInode(resolved)
	delta := inode.Size - before
	fields := map[string]interface{}{
		"type": inode.Type, "mode": inode.Mode,
		"uid": inode.UID, "gid": inode.GID,
		"size": inode.Size,
		"ctime_ms": inode.CtimeMs, "mtime_ms": inode.MtimeMs, "atime_ms": inode.AtimeMs,
		"content": inode.Content,
	}
	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, c.keys.inode(resolved), fields)
	if delta != 0 {
		pipe.HIncrBy(ctx, c.keys.info(), "total_data_bytes", delta)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (c *nativeClient) createFile(ctx context.Context, p string, content string, mode uint32) error {
	if err := c.ensureParents(ctx, p); err != nil {
		return err
	}
	now := nowMs()
	fields := map[string]interface{}{
		"type": "file", "mode": mode,
		"uid": uint32(0), "gid": uint32(0),
		"size": int64(len(content)),
		"ctime_ms": now, "mtime_ms": now, "atime_ms": now,
		"content": content,
	}
	c.invalidateInode(p)
	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, c.keys.inode(p), fields)
	pipe.SAdd(ctx, c.keys.children(parentOf(p)), baseName(p))
	pipe.HIncrBy(ctx, c.keys.info(), "files", 1)
	if len(content) > 0 {
		pipe.HIncrBy(ctx, c.keys.info(), "total_data_bytes", int64(len(content)))
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (c *nativeClient) createDir(ctx context.Context, p string, mode uint32) error {
	if err := c.ensureParents(ctx, p); err != nil {
		return err
	}
	return c.createDirNoParents(ctx, p, mode)
}

func (c *nativeClient) createDirNoParents(ctx context.Context, p string, mode uint32) error {
	now := nowMs()
	fields := map[string]interface{}{
		"type": "dir", "mode": mode,
		"uid": uint32(0), "gid": uint32(0),
		"size": int64(0),
		"ctime_ms": now, "mtime_ms": now, "atime_ms": now,
	}
	c.invalidateInode(p)
	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, c.keys.inode(p), fields)
	pipe.SAdd(ctx, c.keys.children(parentOf(p)), baseName(p))
	pipe.HIncrBy(ctx, c.keys.info(), "directories", 1)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *nativeClient) ensureRoot(ctx context.Context) error {
	root, err := c.loadInode(ctx, "/")
	if err != nil {
		return err
	}
	if root != nil {
		return nil
	}
	now := nowMs()
	root = &inodeData{
		Type:    "dir",
		Mode:    0o755,
		UID:     0,
		GID:     0,
		Size:    0,
		CtimeMs: now,
		MtimeMs: now,
		AtimeMs: now,
	}
	if err := c.saveInode(ctx, "/", root); err != nil {
		return err
	}
	return c.rdb.HSet(ctx, c.keys.info(), map[string]interface{}{
		"schema_version":   "1",
		"files":            0,
		"directories":      1,
		"symlinks":         0,
		"total_data_bytes": 0,
	}).Err()
}

func (c *nativeClient) ensureParents(ctx context.Context, p string) error {
	if err := c.ensureRoot(ctx); err != nil {
		return err
	}
	parent := parentOf(p)
	if parent == "/" {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(parent, "/"), "/")
	cur := "/"
	for _, part := range parts {
		cur = joinPath(cur, part)
		inode, err := c.loadInode(ctx, cur)
		if err != nil {
			return err
		}
		if inode != nil {
			if inode.Type != "dir" {
				return errors.New("parent path conflict")
			}
			continue
		}
		if err := c.createDirNoParents(ctx, cur, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (c *nativeClient) resolvePath(ctx context.Context, p string, followFinal bool) (string, *inodeData, error) {
	p = normalizePath(p)
	components := splitComponents(p)
	cur := "/"
	depth := 0
	for i := 0; ; {
		next := cur
		if i < len(components) {
			next = joinPath(cur, components[i])
		}
		inode, err := c.loadInode(ctx, next)
		if err != nil {
			return "", nil, err
		}
		if inode == nil {
			return "", nil, redis.Nil
		}
		isFinal := i == len(components)-1
		if inode.Type == "symlink" && (followFinal || !isFinal) {
			depth++
			if depth > maxSymlinkDepth {
				return "", nil, errors.New("too many levels of symbolic links")
			}
			remaining := ""
			if i+1 < len(components) {
				remaining = strings.Join(components[i+1:], "/")
			}
			target := inode.Target
			if target == "" {
				return "", nil, errors.New("invalid symlink target")
			}
			var rebuilt string
			if strings.HasPrefix(target, "/") {
				rebuilt = target
			} else {
				rebuilt = path.Join(cur, target)
			}
			if remaining != "" {
				rebuilt = path.Join(rebuilt, remaining)
			}
			p = normalizePath(rebuilt)
			components = splitComponents(p)
			cur = "/"
			i = 0
			continue
		}
		if i >= len(components)-1 {
			return next, inode, nil
		}
		if inode.Type != "dir" {
			return "", nil, errors.New("not a directory")
		}
		cur = next
		i++
	}
}

// loadInode fetches inode metadata (no content) via HMGet.
// Use loadContent separately when file content is needed.
func (c *nativeClient) loadInode(ctx context.Context, p string) (*inodeData, error) {
	if c.cache != nil {
		if v, ok := c.cache.Get(p); ok {
			return v.(*inodeData), nil
		}
	}
	vals, err := c.rdb.HMGet(ctx, c.keys.inode(p),
		"type", "mode", "uid", "gid", "size",
		"ctime_ms", "mtime_ms", "atime_ms", "target").Result()
	if err != nil {
		return nil, err
	}
	if vals[0] == nil {
		return nil, nil
	}
	inode := &inodeData{
		Type:    toStr(vals[0]),
		Mode:    uint32(toInt(vals[1])),
		UID:     uint32(toInt(vals[2])),
		GID:     uint32(toInt(vals[3])),
		Size:    toInt(vals[4]),
		CtimeMs: toInt(vals[5]),
		MtimeMs: toInt(vals[6]),
		AtimeMs: toInt(vals[7]),
		Target:  toStr(vals[8]),
	}
	if c.cache != nil {
		c.cache.Set(p, inode)
	}
	return inode, nil
}

// loadContent fetches only the content field for a file inode.
func (c *nativeClient) loadContent(ctx context.Context, p string) (string, error) {
	v, err := c.rdb.HGet(ctx, c.keys.inode(p), "content").Result()
	if err != nil && err != redis.Nil {
		return "", err
	}
	return v, nil
}

// saveInode writes all fields including content. Use for content-modifying operations.
func (c *nativeClient) saveInode(ctx context.Context, p string, inode *inodeData) error {
	c.invalidateInode(p)
	fields := map[string]interface{}{
		"type":     inode.Type,
		"mode":     inode.Mode,
		"uid":      inode.UID,
		"gid":      inode.GID,
		"size":     inode.Size,
		"ctime_ms": inode.CtimeMs,
		"mtime_ms": inode.MtimeMs,
		"atime_ms": inode.AtimeMs,
	}
	if inode.Type == "symlink" {
		fields["target"] = inode.Target
	}
	if inode.Type == "file" {
		fields["content"] = inode.Content
	}
	return c.rdb.HSet(ctx, c.keys.inode(p), fields).Err()
}

// saveInodeMeta writes only metadata fields (no content). Use for chmod, chown, touch, atime updates.
func (c *nativeClient) saveInodeMeta(ctx context.Context, p string, inode *inodeData) error {
	c.invalidateInode(p)
	fields := map[string]interface{}{
		"type": inode.Type, "mode": inode.Mode,
		"uid": inode.UID, "gid": inode.GID,
		"size": inode.Size,
		"ctime_ms": inode.CtimeMs, "mtime_ms": inode.MtimeMs, "atime_ms": inode.AtimeMs,
	}
	if inode.Type == "symlink" {
		fields["target"] = inode.Target
	}
	return c.rdb.HSet(ctx, c.keys.inode(p), fields).Err()
}

func toStr(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toInt(v interface{}) int64 {
	if v == nil {
		return 0
	}
	s, ok := v.(string)
	if !ok {
		return 0
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func (c *nativeClient) adjustInfoForCreate(ctx context.Context, inode *inodeData) error {
	if err := c.ensureRoot(ctx); err != nil {
		return err
	}
	switch inode.Type {
	case "file":
		if err := c.rdb.HIncrBy(ctx, c.keys.info(), "files", 1).Err(); err != nil {
			return err
		}
		return c.adjustTotalData(ctx, inode.Size)
	case "dir":
		return c.rdb.HIncrBy(ctx, c.keys.info(), "directories", 1).Err()
	case "symlink":
		return c.rdb.HIncrBy(ctx, c.keys.info(), "symlinks", 1).Err()
	default:
		return nil
	}
}

func (c *nativeClient) adjustInfoForDelete(ctx context.Context, inode *inodeData) error {
	if err := c.ensureRoot(ctx); err != nil {
		return err
	}
	switch inode.Type {
	case "file":
		if err := c.rdb.HIncrBy(ctx, c.keys.info(), "files", -1).Err(); err != nil {
			return err
		}
		return c.adjustTotalData(ctx, -inode.Size)
	case "dir":
		return c.rdb.HIncrBy(ctx, c.keys.info(), "directories", -1).Err()
	case "symlink":
		return c.rdb.HIncrBy(ctx, c.keys.info(), "symlinks", -1).Err()
	default:
		return nil
	}
}

func (c *nativeClient) adjustTotalData(ctx context.Context, delta int64) error {
	if delta == 0 {
		return nil
	}
	return c.rdb.HIncrBy(ctx, c.keys.info(), "total_data_bytes", delta).Err()
}

func (c *nativeClient) collectSubtreePaths(ctx context.Context, root string) ([]string, error) {
	rootInode, err := c.loadInode(ctx, root)
	if err != nil {
		return nil, err
	}
	if rootInode == nil {
		return nil, nil
	}
	out := []string{root}
	if rootInode.Type != "dir" {
		return out, nil
	}

	// Level-by-level BFS: pipeline SMembers for all dirs in each level,
	// then pipeline HGetAll for all children to find next set of dirs.
	// Reduces O(2N) serial RTTs to O(2×depth) RTTs.
	currentDirs := []string{root}
	for len(currentDirs) > 0 {
		// Pipeline SMembers for every dir in this level.
		pipe := c.rdb.Pipeline()
		smbCmds := make([]*redis.StringSliceCmd, len(currentDirs))
		for i, d := range currentDirs {
			smbCmds[i] = pipe.SMembers(ctx, c.keys.children(d))
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return nil, err
		}

		// Collect all child paths for this level.
		var childPaths []string
		for i, cmd := range smbCmds {
			names, err := cmd.Result()
			if err != nil {
				continue
			}
			sort.Strings(names)
			for _, name := range names {
				childPaths = append(childPaths, joinPath(currentDirs[i], name))
			}
		}
		if len(childPaths) == 0 {
			break
		}

		// Pipeline HGetAll for all children to find which are dirs.
		// Use loadInode so the cache is consulted and populated.
		var nextDirs []string
		pipe2 := c.rdb.Pipeline()
		typeCmds := make([]*redis.SliceCmd, len(childPaths))
		// Only fetch from Redis for paths not already in cache.
		uncachedIdx := make([]int, 0, len(childPaths))
		for i, p := range childPaths {
			if c.cache != nil {
				if v, ok := c.cache.Get(p); ok {
					inode := v.(*inodeData)
					out = append(out, p)
					if inode.Type == "dir" {
						nextDirs = append(nextDirs, p)
					}
					typeCmds[i] = nil
					continue
				}
			}
			typeCmds[i] = pipe2.HMGet(ctx, c.keys.inode(p),
				"type", "mode", "uid", "gid", "size",
				"ctime_ms", "mtime_ms", "atime_ms", "target")
			uncachedIdx = append(uncachedIdx, i)
		}
		if len(uncachedIdx) > 0 {
			if _, err := pipe2.Exec(ctx); err != nil && err != redis.Nil {
				return nil, err
			}
			for _, i := range uncachedIdx {
				vals, err := typeCmds[i].Result()
				if err != nil || len(vals) < 9 || vals[0] == nil {
					continue
				}
				inode := &inodeData{
					Type:    toStr(vals[0]),
					Mode:    uint32(toInt(vals[1])),
					UID:     uint32(toInt(vals[2])),
					GID:     uint32(toInt(vals[3])),
					Size:    toInt(vals[4]),
					CtimeMs: toInt(vals[5]),
					MtimeMs: toInt(vals[6]),
					AtimeMs: toInt(vals[7]),
					Target:  toStr(vals[8]),
				}
				if c.cache != nil {
					c.cache.Set(childPaths[i], inode)
				}
				out = append(out, childPaths[i])
				if inode.Type == "dir" {
					nextDirs = append(nextDirs, childPaths[i])
				}
			}
		}
		currentDirs = nextDirs
	}

	sort.Slice(out, func(i, j int) bool { return len(out[i]) < len(out[j]) })
	return out, nil
}

func (i *inodeData) toStat() *StatResult {
	return &StatResult{
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

func nowMs() int64 {
	return time.Now().UnixMilli()
}

func splitComponents(p string) []string {
	if p == "/" {
		return nil
	}
	return strings.Split(strings.TrimPrefix(p, "/"), "/")
}

func joinPath(parent, child string) string {
	if parent == "/" {
		return "/" + child
	}
	return parent + "/" + child
}

func parentOf(p string) string {
	if p == "/" {
		return "/"
	}
	parent := path.Dir(p)
	if parent == "." {
		return "/"
	}
	return parent
}

func baseName(p string) string {
	if p == "/" {
		return ""
	}
	return path.Base(p)
}

func parseInt64OrZero(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// splitLines splits content into lines. If content ends with \n,
// the trailing empty string is excluded (matching C module behavior).
func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// joinLines joins lines back together with \n separator and trailing \n.
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// countWords counts whitespace-delimited words in a string.
func countWords(s string) int64 {
	count := int64(0)
	inWord := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			inWord = false
		} else {
			if !inWord {
				count++
			}
			inWord = true
		}
	}
	return count
}
