package client

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	rootInodeID   = "1"
	schemaVersion = "2"
)

var inodeMetaFields = []string{
	"type", "mode", "uid", "gid", "size",
	"ctime_ms", "mtime_ms", "atime_ms", "target",
	"parent", "name",
}

var inodeWarmFields = []string{
	"path",
	"type", "mode", "uid", "gid", "size",
	"ctime_ms", "mtime_ms", "atime_ms", "target",
	"parent", "name",
}

type namedInode struct {
	Name  string
	Path  string
	Inode *inodeData
}

type warmedPathEntry struct {
	path  string
	inode *inodeData
}

var errNeedDirWatch = errors.New("need destination dir watch")

func (c *nativeClient) loadRootInode(ctx context.Context) (*inodeData, error) {
	if c.cache != nil {
		if cached, ok := c.cache.Get("/"); ok {
			return cloneInodeMeta(cached.(*inodeData)), nil
		}
	}

	root, err := c.loadInodeByID(ctx, rootInodeID)
	if err != nil {
		return nil, err
	}
	if root != nil {
		c.cachePath("/", root)
	}
	return root, nil
}

func (c *nativeClient) ensureRoot(ctx context.Context) error {
	root, err := c.loadRootInode(ctx)
	if err != nil {
		return err
	}
	if root != nil {
		return nil
	}

	now := nowMs()
	root = &inodeData{
		ID:      rootInodeID,
		Type:    "dir",
		Mode:    0o755,
		UID:     0,
		GID:     0,
		Size:    0,
		CtimeMs: now,
		MtimeMs: now,
		AtimeMs: now,
	}

	pipe := c.rdb.TxPipeline()
	pipe.HSet(ctx, c.keys.inode(rootInodeID), c.inodeFieldsAtPath(root, "/", false))
	pipe.HSet(ctx, c.keys.info(), map[string]interface{}{
		"schema_version":   schemaVersion,
		"files":            0,
		"directories":      1,
		"symlinks":         0,
		"total_data_bytes": 0,
	})
	pipe.SetNX(ctx, c.keys.nextInode(), rootInodeID, 0)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	c.cachePath("/", root)
	return nil
}

func (c *nativeClient) ensureParents(ctx context.Context, p string) error {
	if err := c.ensureRoot(ctx); err != nil {
		return err
	}
	parentPath := parentOf(p)
	if parentPath == "/" {
		return nil
	}

	parts := splitComponents(parentPath)
	curPath := "/"
	curInode, err := c.loadRootInode(ctx)
	if err != nil {
		return err
	}
	if curInode == nil {
		return redis.Nil
	}

	for _, part := range parts {
		nextPath := joinPath(curPath, part)
		childID, err := c.lookupChildID(ctx, curInode.ID, part)
		switch {
		case err == nil:
			child, err := c.loadInodeByID(ctx, childID)
			if err != nil {
				return err
			}
			if child == nil || child.Type != "dir" {
				return errors.New("parent path conflict")
			}
			c.cachePath(nextPath, child)
			curPath = nextPath
			curInode = child
		case errors.Is(err, redis.Nil):
			now := nowMs()
			child := &inodeData{
				Type:    "dir",
				Mode:    0o755,
				UID:     0,
				GID:     0,
				Size:    0,
				CtimeMs: now,
				MtimeMs: now,
				AtimeMs: now,
			}
			if err := c.createInodeUnderParent(ctx, nextPath, curInode, part, child); err != nil {
				return err
			}
			curPath = nextPath
			curInode = child
		default:
			return err
		}
	}
	return nil
}

func (c *nativeClient) resolvePath(ctx context.Context, p string, followFinal bool) (string, *inodeData, error) {
	if err := c.ensureRoot(ctx); err != nil {
		return "", nil, err
	}

	p = normalizePath(p)
	if p == "/" {
		root, err := c.loadRootInode(ctx)
		if err != nil {
			return "", nil, err
		}
		if root == nil {
			return "", nil, redis.Nil
		}
		return "/", root, nil
	}

	components := splitComponents(p)
	curPath := "/"
	curInode, err := c.loadRootInode(ctx)
	if err != nil {
		return "", nil, err
	}
	if curInode == nil {
		return "", nil, redis.Nil
	}

	depth := 0
	for i := 0; i < len(components); i++ {
		nextPath := joinPath(curPath, components[i])

		var inode *inodeData
		if c.cache != nil {
			if cached, ok := c.cache.Get(nextPath); ok {
				inode = cloneInodeMeta(cached.(*inodeData))
			}
		}
		if inode == nil {
			childID, err := c.lookupChildID(ctx, curInode.ID, components[i])
			if err != nil {
				if errors.Is(err, redis.Nil) {
					return "", nil, redis.Nil
				}
				return "", nil, err
			}
			inode, err = c.loadInodeByID(ctx, childID)
			if err != nil {
				return "", nil, err
			}
			if inode == nil {
				return "", nil, redis.Nil
			}
			c.cachePath(nextPath, inode)
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
				rebuilt = path.Join(curPath, target)
			}
			if remaining != "" {
				rebuilt = path.Join(rebuilt, remaining)
			}

			p = normalizePath(rebuilt)
			components = splitComponents(p)
			curPath = "/"
			curInode, err = c.loadRootInode(ctx)
			if err != nil {
				return "", nil, err
			}
			if curInode == nil {
				return "", nil, redis.Nil
			}
			i = -1
			continue
		}

		if isFinal {
			return nextPath, inode, nil
		}
		if inode.Type != "dir" {
			return "", nil, errors.New("not a directory")
		}

		curPath = nextPath
		curInode = inode
	}

	return curPath, curInode, nil
}

func (c *nativeClient) loadInode(ctx context.Context, p string) (*inodeData, error) {
	p = normalizePath(p)
	if c.cache != nil {
		if cached, ok := c.cache.Get(p); ok {
			return cloneInodeMeta(cached.(*inodeData)), nil
		}
	}

	resolved, inode, err := c.resolvePath(ctx, p, false)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	c.cachePath(resolved, inode)
	if resolved != p {
		c.cachePath(p, inode)
	}
	return cloneInodeMeta(inode), nil
}

func (c *nativeClient) loadInodeByID(ctx context.Context, id string) (*inodeData, error) {
	vals, err := c.rdb.HMGet(ctx, c.keys.inode(id), inodeMetaFields...).Result()
	if err != nil {
		return nil, err
	}
	return inodeFromValues(id, vals), nil
}

func (c *nativeClient) loadInodesByID(ctx context.Context, ids []string) (map[string]*inodeData, error) {
	if len(ids) == 0 {
		return map[string]*inodeData{}, nil
	}

	pipe := c.rdb.Pipeline()
	cmds := make([]*redis.SliceCmd, len(ids))
	for i, id := range ids {
		cmds[i] = pipe.HMGet(ctx, c.keys.inode(id), inodeMetaFields...)
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	out := make(map[string]*inodeData, len(ids))
	for i, id := range ids {
		vals, err := cmds[i].Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, err
		}
		out[id] = inodeFromValues(id, vals)
	}
	return out, nil
}

func (c *nativeClient) WarmPathCache(ctx context.Context) error {
	if c.cache == nil {
		return nil
	}

	const scanCount = 256
	dirChildren := make(map[string][]namedInode)
	var cursor uint64
	for {
		keys, next, err := c.rdb.Scan(ctx, cursor, c.keys.inodePrefix()+"*", scanCount).Result()
		if err != nil {
			return err
		}
		entries, err := c.loadWarmBatch(ctx, keys)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.inode == nil || entry.path == "" {
				continue
			}
			c.cachePath(entry.path, entry.inode)
			if entry.inode.Type == "dir" {
				if _, ok := dirChildren[entry.path]; !ok {
					dirChildren[entry.path] = nil
				}
			}
			if entry.path == "/" {
				continue
			}
			parent := parentOf(entry.path)
			dirChildren[parent] = append(dirChildren[parent], namedInode{
				Name:  entry.inode.Name,
				Path:  entry.path,
				Inode: cloneInodeMeta(entry.inode),
			})
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	for dirPath, children := range dirChildren {
		sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
		c.cache.Set(dirCacheKey(dirPath), cloneNamedInodes(children))
	}
	return nil
}

func (c *nativeClient) loadWarmBatch(ctx context.Context, keys []string) ([]warmedPathEntry, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	pipe := c.rdb.Pipeline()
	cmds := make([]*redis.SliceCmd, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.HMGet(ctx, key, inodeWarmFields...)
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	entries := make([]warmedPathEntry, 0, len(keys))
	for i, key := range keys {
		vals, err := cmds[i].Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, err
		}
		if len(vals) < len(inodeWarmFields) || vals[0] == nil {
			continue
		}
		id := strings.TrimPrefix(key, c.keys.inodePrefix())
		inode := inodeFromValues(id, vals[1:])
		if inode == nil {
			continue
		}
		path := toStr(vals[0])
		if path == "" {
			continue
		}
		entries = append(entries, warmedPathEntry{path: path, inode: inode})
	}
	return entries, nil
}

func (c *nativeClient) loadContentByID(ctx context.Context, id string) (string, error) {
	v, err := c.rdb.HGet(ctx, c.keys.inode(id), "content").Result()
	if err != nil && err != redis.Nil {
		return "", err
	}
	return v, nil
}

func (c *nativeClient) saveInode(ctx context.Context, p string, inode *inodeData) error {
	if inode == nil || inode.ID == "" {
		return errors.New("missing inode id")
	}
	if err := c.saveInodeAtPath(ctx, p, inode, true); err != nil {
		return err
	}
	c.cachePath(p, inode)
	return nil
}

func (c *nativeClient) saveInodeMeta(ctx context.Context, p string, inode *inodeData) error {
	if inode == nil || inode.ID == "" {
		return errors.New("missing inode id")
	}
	if err := c.saveInodeAtPath(ctx, p, inode, false); err != nil {
		return err
	}
	c.cachePath(p, inode)
	return nil
}

func (c *nativeClient) saveInodeAtPath(ctx context.Context, p string, inode *inodeData, includeContent bool) error {
	if inode == nil || inode.ID == "" {
		return errors.New("missing inode id")
	}
	return c.rdb.HSet(ctx, c.keys.inode(inode.ID), c.inodeFieldsAtPath(inode, p, includeContent)).Err()
}

func (c *nativeClient) saveInodeDirect(ctx context.Context, inode *inodeData, includeContent bool) error {
	if inode == nil || inode.ID == "" {
		return errors.New("missing inode id")
	}
	return c.rdb.HSet(ctx, c.keys.inode(inode.ID), c.inodeFields(inode, includeContent)).Err()
}

func (c *nativeClient) createInodeAtPath(ctx context.Context, p string, inode *inodeData, ensureParents bool) error {
	p = normalizePath(p)
	if p == "/" {
		return errors.New("already exists")
	}
	if ensureParents {
		if err := c.ensureParents(ctx, p); err != nil {
			return err
		}
	}

	parentPath := parentOf(p)
	_, parentInode, err := c.resolvePath(ctx, parentPath, true)
	if err != nil {
		return err
	}
	return c.createInodeUnderParent(ctx, p, parentInode, baseName(p), inode)
}

func (c *nativeClient) createInodeUnderParent(ctx context.Context, childPath string, parent *inodeData, name string, inode *inodeData) error {
	if parent == nil || parent.Type != "dir" {
		return errors.New("parent path conflict")
	}
	if _, err := c.lookupChildID(ctx, parent.ID, name); err == nil {
		return errors.New("already exists")
	} else if !errors.Is(err, redis.Nil) {
		return err
	}

	id, err := c.allocInodeID(ctx)
	if err != nil {
		return err
	}

	inode.ID = id
	inode.Parent = parent.ID
	inode.Name = name
	now := nowMs()

	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, c.keys.inode(id), c.inodeFieldsAtPath(inode, childPath, inode.Type == "file"))
	pipe.HSet(ctx, c.keys.dirents(parent.ID), name, id)
	c.queueTouchTimes(pipe, parent.ID, now)
	c.queueCreateInfo(pipe, inode)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	c.invalidateInode(parentOf(childPath))
	c.cachePath(childPath, inode)
	return nil
}

func (c *nativeClient) createFileIfMissing(ctx context.Context, p string, content string, mode uint32, exclusive bool) (*inodeData, bool, error) {
	p = normalizePath(p)
	if p == "/" {
		return nil, false, errors.New("cannot write to root")
	}
	parentPath := parentOf(p)
	_, parentInode, err := c.resolvePath(ctx, parentPath, true)
	if err != nil {
		return nil, false, err
	}
	if parentInode.Type != "dir" {
		return nil, false, errors.New("parent path conflict")
	}

	var (
		result  *inodeData
		created bool
	)
	direntsKey := c.keys.dirents(parentInode.ID)
	name := baseName(p)
	err = c.retryWatch(ctx, []string{direntsKey}, func(tx *redis.Tx) error {
		childID, err := tx.HGet(ctx, direntsKey, name).Result()
		switch {
		case err == nil:
			existing, err := c.loadInodeByID(ctx, childID)
			if err != nil {
				return err
			}
			if existing == nil {
				return redis.TxFailedErr
			}
			if existing.Type != "file" {
				return errors.New("not a file")
			}
			if exclusive {
				return errors.New("already exists")
			}
			result = existing
			created = false
			return nil
		case !errors.Is(err, redis.Nil):
			return err
		}

		id, err := c.allocInodeID(ctx)
		if err != nil {
			return err
		}
		now := nowMs()
		inode := &inodeData{
			ID:      id,
			Parent:  parentInode.ID,
			Name:    name,
			Type:    "file",
			Mode:    mode,
			UID:     0,
			GID:     0,
			Size:    int64(len(content)),
			CtimeMs: now,
			MtimeMs: now,
			AtimeMs: now,
			Content: content,
		}
		if _, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.HSet(ctx, c.keys.inode(inode.ID), c.inodeFieldsAtPath(inode, p, true))
			pipe.HSet(ctx, direntsKey, name, inode.ID)
			c.queueTouchTimes(pipe, parentInode.ID, now)
			c.queueCreateInfo(pipe, inode)
			return nil
		}); err != nil {
			return err
		}
		result = inode
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	c.invalidateInode(parentPath)
	c.cachePath(p, result)
	return result, created, nil
}

func (c *nativeClient) renamePath(ctx context.Context, resolvedSrc string, srcInode *inodeData, dst string, newParent *inodeData, flags uint32) error {
	oldParentID := srcInode.Parent
	oldName := srcInode.Name
	newName := baseName(dst)
	if oldParentID == "" {
		return errors.New("no such file or directory")
	}

	var watchDstDirID string
	for attempts := 0; attempts < 8; attempts++ {
		keys := uniqueStrings(c.keys.dirents(oldParentID), c.keys.dirents(newParent.ID))
		if watchDstDirID != "" {
			keys = uniqueStrings(append(keys, c.keys.dirents(watchDstDirID))...)
		}

		var nextDstDirID string
		err := c.retryWatch(ctx, keys, func(tx *redis.Tx) error {
			currentSrcID, err := tx.HGet(ctx, c.keys.dirents(oldParentID), oldName).Result()
			if err != nil {
				if errors.Is(err, redis.Nil) {
					return errors.New("no such file or directory")
				}
				return err
			}
			if currentSrcID != srcInode.ID {
				return redis.TxFailedErr
			}

			currentSrc, err := c.loadInodeByID(ctx, currentSrcID)
			if err != nil {
				return err
			}
			if currentSrc == nil {
				return errors.New("no such file or directory")
			}

			var replaced *inodeData
			replacedID, err := tx.HGet(ctx, c.keys.dirents(newParent.ID), newName).Result()
			switch {
			case err == nil:
				replaced, err = c.loadInodeByID(ctx, replacedID)
				if err != nil {
					return err
				}
				if replaced == nil {
					return redis.TxFailedErr
				}
				if flags&RenameNoreplace != 0 {
					return errors.New("already exists")
				}
				if currentSrc.Type == "dir" && replaced.Type != "dir" {
					return errors.New("not a directory")
				}
				if currentSrc.Type != "dir" && replaced.Type == "dir" {
					return errors.New("not a file")
				}
				if replaced.Type == "dir" {
					if watchDstDirID != replaced.ID {
						nextDstDirID = replaced.ID
						return errNeedDirWatch
					}
					count, err := tx.HLen(ctx, c.keys.dirents(replaced.ID)).Result()
					if err != nil {
						return err
					}
					if count > 0 {
						return errors.New("directory not empty")
					}
				}
			case !errors.Is(err, redis.Nil):
				return err
			}

			nextSrc := *currentSrc
			nextSrc.Parent = newParent.ID
			nextSrc.Name = newName
			nextSrc.CtimeMs = nowMs()

			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HDel(ctx, c.keys.dirents(oldParentID), oldName)
				pipe.HSet(ctx, c.keys.dirents(newParent.ID), newName, nextSrc.ID)
				pipe.HSet(ctx, c.keys.inode(nextSrc.ID), map[string]interface{}{
					"parent":         nextSrc.Parent,
					"name":           nextSrc.Name,
					"ctime_ms":       nextSrc.CtimeMs,
					"path":           dst,
					"path_ancestors": indexedPathAncestors(dst),
				})
				c.queueTouchTimes(pipe, oldParentID, nextSrc.CtimeMs)
				if newParent.ID != oldParentID {
					c.queueTouchTimes(pipe, newParent.ID, nextSrc.CtimeMs)
				}
				if replaced != nil {
					pipe.Del(ctx, c.keys.inode(replaced.ID))
					if replaced.Type == "dir" {
						pipe.Del(ctx, c.keys.dirents(replaced.ID))
					}
					c.queueDeleteInfo(pipe, replaced)
				}
				return nil
			})
			if err != nil {
				return err
			}

			srcInode.Parent = nextSrc.Parent
			srcInode.Name = nextSrc.Name
			srcInode.CtimeMs = nextSrc.CtimeMs
			return nil
		})
		switch {
		case err == nil:
			c.invalidateInode(parentOf(resolvedSrc))
			c.invalidateInode(parentOf(dst))
			c.invalidatePrefix(resolvedSrc)
			c.invalidatePrefix(dst)
			if err := c.refreshIndexedSubtree(ctx, dst, srcInode); err != nil {
				return err
			}
			return nil
		case errors.Is(err, errNeedDirWatch):
			watchDstDirID = nextDstDirID
			continue
		default:
			return err
		}
	}

	return fmt.Errorf("destination changed too often")
}

func (c *nativeClient) listDirChildren(ctx context.Context, dirPath string, dir *inodeData) ([]namedInode, error) {
	if dir == nil || dir.Type != "dir" {
		return nil, errors.New("not a directory")
	}
	if c.cache != nil {
		if cached, ok := c.cache.Get(dirCacheKey(dirPath)); ok {
			return cloneNamedInodes(cached.([]namedInode)), nil
		}
	}

	entries, err := c.loadDirEntries(ctx, dir.ID)
	if err != nil {
		return nil, err
	}

	names := sortNames(entries)
	ids := make([]string, 0, len(names))
	for _, name := range names {
		ids = append(ids, entries[name])
	}
	inodesByID, err := c.loadInodesByID(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]namedInode, 0, len(names))
	for _, name := range names {
		child := inodesByID[entries[name]]
		if child == nil {
			continue
		}
		childPath := joinPath(dirPath, name)
		c.cachePath(childPath, child)
		out = append(out, namedInode{
			Name:  name,
			Path:  childPath,
			Inode: child,
		})
	}
	if c.cache != nil {
		c.cache.Set(dirCacheKey(dirPath), cloneNamedInodes(out))
	}
	return out, nil
}

func (c *nativeClient) loadDirEntries(ctx context.Context, dirID string) (map[string]string, error) {
	values, err := c.rdb.HGetAll(ctx, c.keys.dirents(dirID)).Result()
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return map[string]string{}, nil
	}
	return values, nil
}

func (c *nativeClient) lookupChildID(ctx context.Context, dirID, name string) (string, error) {
	return c.rdb.HGet(ctx, c.keys.dirents(dirID), name).Result()
}

func (c *nativeClient) allocInodeID(ctx context.Context) (string, error) {
	if err := c.ensureRoot(ctx); err != nil {
		return "", err
	}
	id, err := c.rdb.Incr(ctx, c.keys.nextInode()).Result()
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(id, 10), nil
}

func (c *nativeClient) retryWatch(ctx context.Context, keys []string, fn func(*redis.Tx) error) error {
	for attempts := 0; attempts < 16; attempts++ {
		err := c.rdb.Watch(ctx, fn, keys...)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return err
	}
	return redis.TxFailedErr
}

func (c *nativeClient) inodeFields(inode *inodeData, includeContent bool) map[string]interface{} {
	fields := map[string]interface{}{
		"type":     inode.Type,
		"mode":     inode.Mode,
		"uid":      inode.UID,
		"gid":      inode.GID,
		"size":     inode.Size,
		"ctime_ms": inode.CtimeMs,
		"mtime_ms": inode.MtimeMs,
		"atime_ms": inode.AtimeMs,
		"parent":   inode.Parent,
		"name":     inode.Name,
	}
	if inode.Type == "symlink" {
		fields["target"] = inode.Target
	}
	if includeContent && inode.Type == "file" {
		fields["content"] = inode.Content
	}
	return fields
}

func (c *nativeClient) inodeFieldsAtPath(inode *inodeData, p string, includeContent bool) map[string]interface{} {
	fields := c.inodeFields(inode, includeContent)
	fields["path"] = p
	fields["path_ancestors"] = indexedPathAncestors(p)
	return fields
}

func (c *nativeClient) refreshIndexedSubtree(ctx context.Context, rootPath string, inode *inodeData) error {
	if inode == nil || inode.ID == "" {
		return nil
	}
	pipe := c.rdb.Pipeline()
	if err := c.queueRefreshIndexedSubtree(ctx, pipe, rootPath, inode); err != nil {
		return err
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (c *nativeClient) queueRefreshIndexedSubtree(ctx context.Context, pipe redis.Pipeliner, currentPath string, inode *inodeData) error {
	pipe.HSet(ctx, c.keys.inode(inode.ID), map[string]interface{}{
		"path":           currentPath,
		"path_ancestors": indexedPathAncestors(currentPath),
	})
	if inode.Type != "dir" {
		return nil
	}

	entries, err := c.loadDirEntries(ctx, inode.ID)
	if err != nil {
		return err
	}
	for name, childID := range entries {
		child, err := c.loadInodeByID(ctx, childID)
		if err != nil {
			return err
		}
		if child == nil {
			continue
		}
		if err := c.queueRefreshIndexedSubtree(ctx, pipe, joinPath(currentPath, name), child); err != nil {
			return err
		}
	}
	return nil
}

func (c *nativeClient) queueCreateInfo(pipe redis.Pipeliner, inode *inodeData) {
	switch inode.Type {
	case "file":
		pipe.HIncrBy(context.Background(), c.keys.info(), "files", 1)
		if inode.Size > 0 {
			pipe.HIncrBy(context.Background(), c.keys.info(), "total_data_bytes", inode.Size)
		}
	case "dir":
		pipe.HIncrBy(context.Background(), c.keys.info(), "directories", 1)
	case "symlink":
		pipe.HIncrBy(context.Background(), c.keys.info(), "symlinks", 1)
	}
}

func (c *nativeClient) queueDeleteInfo(pipe redis.Pipeliner, inode *inodeData) {
	switch inode.Type {
	case "file":
		pipe.HIncrBy(context.Background(), c.keys.info(), "files", -1)
		if inode.Size > 0 {
			pipe.HIncrBy(context.Background(), c.keys.info(), "total_data_bytes", -inode.Size)
		}
	case "dir":
		pipe.HIncrBy(context.Background(), c.keys.info(), "directories", -1)
	case "symlink":
		pipe.HIncrBy(context.Background(), c.keys.info(), "symlinks", -1)
	}
}

func (c *nativeClient) queueTouchTimes(pipe redis.Pipeliner, inodeID string, ts int64) {
	if inodeID == "" {
		return
	}
	pipe.HSet(context.Background(), c.keys.inode(inodeID), map[string]interface{}{
		"ctime_ms": ts,
		"mtime_ms": ts,
	})
}

func (c *nativeClient) adjustTotalData(ctx context.Context, delta int64) error {
	if delta == 0 {
		return nil
	}
	return c.rdb.HIncrBy(ctx, c.keys.info(), "total_data_bytes", delta).Err()
}

func (c *nativeClient) markRootDirty(ctx context.Context) error {
	return c.rdb.Set(ctx, c.keys.rootDirty(), "1", 0).Err()
}

func (c *nativeClient) cachePath(p string, inode *inodeData) {
	if c.cache == nil || inode == nil {
		return
	}
	c.cache.Set(p, cloneInodeMeta(inode))
}

func dirCacheKey(path string) string {
	return "\x00dir:" + normalizePath(path)
}

func dirCachePrefix(path string) string {
	return "\x00dir:" + normalizePath(path)
}

func cloneInodeMeta(inode *inodeData) *inodeData {
	if inode == nil {
		return nil
	}
	clone := *inode
	clone.Content = ""
	return &clone
}

func cloneNamedInodes(items []namedInode) []namedInode {
	if len(items) == 0 {
		return nil
	}
	out := make([]namedInode, 0, len(items))
	for _, item := range items {
		out = append(out, namedInode{
			Name:  item.Name,
			Path:  item.Path,
			Inode: cloneInodeMeta(item.Inode),
		})
	}
	return out
}

func inodeFromValues(id string, vals []interface{}) *inodeData {
	if len(vals) < 11 || vals[0] == nil {
		return nil
	}
	return &inodeData{
		ID:      id,
		Type:    toStr(vals[0]),
		Mode:    uint32(toInt(vals[1])),
		UID:     uint32(toInt(vals[2])),
		GID:     uint32(toInt(vals[3])),
		Size:    toInt(vals[4]),
		CtimeMs: toInt(vals[5]),
		MtimeMs: toInt(vals[6]),
		AtimeMs: toInt(vals[7]),
		Target:  toStr(vals[8]),
		Parent:  toStr(vals[9]),
		Name:    toStr(vals[10]),
	}
}

func inodeUint64(id string) uint64 {
	value, _ := strconv.ParseUint(id, 10, 64)
	return value
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
	switch value := v.(type) {
	case string:
		n, _ := strconv.ParseInt(value, 10, 64)
		return n
	case int64:
		return value
	case int:
		return int64(value)
	}
	return 0
}

func nowMs() int64 {
	return time.Now().UnixMilli()
}

func indexedPathAncestors(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || trimmed == "/" {
		return "/"
	}
	trimmed = strings.TrimSuffix(trimmed, "/")
	parts := strings.Split(strings.TrimPrefix(trimmed, "/"), "/")
	ancestors := make([]string, 0, len(parts)+1)
	current := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		current += "/" + part
		ancestors = append(ancestors, current)
	}
	if len(ancestors) == 0 {
		return "/"
	}
	return strings.Join(ancestors, ",")
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

func uniqueStrings(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
