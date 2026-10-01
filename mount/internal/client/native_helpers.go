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

	"github.com/redis/agent-filesystem/internal/rediscontent"
	"github.com/redis/go-redis/v9"
)

const (
	rootInodeID   = "1"
	schemaVersion = "2"
)

var inodeMetaFields = []string{
	"type", "mode", "uid", "gid", "size",
	"ctime_ms", "mtime_ms", "atime_ms", "target",
	"parent", "name", "content_ref", "revision",
}

var inodeWarmFields = []string{
	"path",
	"type", "mode", "uid", "gid", "size",
	"ctime_ms", "mtime_ms", "atime_ms", "target",
	"parent", "name", "content_ref", "revision",
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
	if err := c.checkGeneration(ctx); err != nil {
		return err
	}
	root, err := c.loadRootInode(ctx)
	if err != nil {
		return err
	}
	if root != nil {
		return nil
	}
	if _, managed := ctx.Value(workspaceGenerationKey{}).(string); managed {
		return ErrWorkspaceChanged
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
	if parents, _ := ctx.Value(expectedParentsKey{}).(expectedParents); len(parents) > 0 {
		// Native handles refer to existing directories. A stale pathname must
		// never cause implicit parent creation before the identity check.
		_, parent, err := c.resolvePath(ctx, parentOf(normalizePath(p)), true)
		if errors.Is(err, redis.Nil) {
			return ErrWriteConflict
		}
		if err != nil {
			return err
		}
		return checkExpectedParent(ctx, p, parent.ID)
	}
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

		// Cache-first walk: if the parent chain is already warm (typical after
		// the NFS path cache warming or after a previous op in the same dir),
		// skip the Redis round trips entirely. Only fall back to Redis on miss
		// or when the cached entry is not a directory.
		if c.cache != nil {
			if cached, ok := c.cache.Get(nextPath); ok {
				if child, ok := cached.(*inodeData); ok && child != nil && child.Type == "dir" {
					curPath = nextPath
					curInode = cloneInodeMeta(child)
					continue
				}
			}
		}

		childID, err := c.lookupChildID(ctx, curInode.ID, part)
		switch {
		case err == nil:
			child, err := c.loadInodeByID(ctx, childID)
			if err != nil {
				return err
			}
			if child == nil || child.Type != "dir" {
				return ErrParentConflict
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
	if err := c.checkGeneration(ctx); err != nil {
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
			return "", nil, ErrNotDir
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

// loadContentByID reads file content for the given inode ID, handling both
// legacy inline storage (content field in the inode HASH) and the new external
// storage (dedicated STRING key). When the inode's content_ref is already
// known, pass it via loadContentExternal to skip the extra round trip.
func (c *nativeClient) loadContentByID(ctx context.Context, id string) (string, error) {
	values, err := c.rdb.HMGet(ctx, c.keys.inode(id), "content_ref", "size").Result()
	if err != nil && err != redis.Nil {
		return "", err
	}
	ref := toStr(values[0])
	if ref == rediscontent.RefExternal {
		v, err := c.rdb.Get(ctx, c.keys.content(id)).Result()
		if err != nil && err != redis.Nil {
			return "", err
		}
		return v, nil
	}
	if !isExternalContentRef(ref) {
		return c.loadContentExternal(ctx, id, ref)
	}
	size := int64(0)
	if len(values) > 1 {
		size = toInt(values[1])
	}
	data, err := rediscontent.Load(ctx, c.rdb, c.keys.content(id), ref, size)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// loadContentExternal reads content using the already-known content_ref value.
// When contentRef points at an external backend, reads from the dedicated
// content key. Otherwise falls back to the legacy inline HASH field and lazily
// migrates the content to the preferred external backend on first access.
func (c *nativeClient) loadContentExternal(ctx context.Context, id, contentRef string) (string, error) {
	if contentRef == rediscontent.RefExternal {
		v, err := c.rdb.Get(ctx, c.keys.content(id)).Result()
		if err != nil && err != redis.Nil {
			return "", err
		}
		return v, nil
	}
	if isExternalContentRef(contentRef) {
		size, err := c.loadInodeSize(ctx, id)
		if err != nil {
			return "", err
		}
		data, err := rediscontent.Load(ctx, c.rdb, c.keys.content(id), contentRef, size)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	// Legacy inline mode: content stored in the inode HASH field.
	v, err := c.rdb.HGet(ctx, c.keys.inode(id), "content").Result()
	if err != nil && err != redis.Nil {
		return "", err
	}
	// A read must not migrate live content outside the publication guard.
	// The next staged write converts the representation atomically.
	return v, nil
}

func mergeFieldMaps(base map[string]interface{}, extras ...map[string]interface{}) map[string]interface{} {
	for _, extra := range extras {
		for key, value := range extra {
			base[key] = value
		}
	}
	return base
}

func (c *nativeClient) saveInode(ctx context.Context, p string, inode *inodeData) error {
	if inode == nil || inode.ID == "" {
		return errors.New("missing inode id")
	}
	if err := c.selectContentRef(ctx, inode); err != nil {
		return err
	}
	if inode.Type != "file" {
		return c.saveInodeAtPath(ctx, p, inode, true)
	}
	if err := checkWriteCondition(ctx, inode, false); err != nil {
		return err
	}
	stage, err := c.stageFullFile(ctx, inode)
	if err != nil {
		return err
	}
	defer c.discardStage(stage)
	fields := fileSearchIndexFields(inode.Content)
	return c.publishStagedFile(ctx, p, inode, stage, false, fields)
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
	return c.updatePublishedInode(ctx, p, inode, c.inodeFieldsAtPath(inode, p, includeContent))
}

func (c *nativeClient) createInodeAtPath(ctx context.Context, p string, inode *inodeData, ensureParents bool) error {
	p = normalizePath(p)
	if p == "/" {
		return ErrAlreadyExists
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
		return ErrParentConflict
	}
	if err := checkExpectedParent(ctx, childPath, parent.ID); err != nil {
		return err
	}
	if _, err := c.lookupChildID(ctx, parent.ID, name); err == nil {
		return ErrAlreadyExists
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

	// New files use the preferred external content backend.
	if inode.Type == "file" {
		if err := c.selectContentRef(ctx, inode); err != nil {
			return err
		}
	}

	err = c.retryWatch(ctx, []string{c.keys.dirents(parent.ID), c.keys.inode(parent.ID)}, func(tx *redis.Tx) error {
		exists, err := tx.Exists(ctx, c.keys.inode(parent.ID)).Result()
		if err != nil {
			return err
		}
		if exists == 0 {
			return ErrWriteConflict
		}
		existsName, err := tx.HExists(ctx, c.keys.dirents(parent.ID), name).Result()
		if err != nil {
			return err
		}
		if existsName {
			return ErrAlreadyExists
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			if inode.Type == "file" && isExternalContentRef(inode.ContentRef) {
				// Content goes to the external backend; metadata-only to the HASH.
				rediscontent.QueueWriteFull(ctx, pipe, c.keys.content(id), inode.ContentRef, []byte(inode.Content))
				pipe.HSet(ctx, c.keys.inode(id), c.inodeFieldsAtPath(inode, childPath, false))
			} else {
				pipe.HSet(ctx, c.keys.inode(id), c.inodeFieldsAtPath(inode, childPath, inode.Type == "file"))
			}
			pipe.HSet(ctx, c.keys.dirents(parent.ID), name, id)
			c.queueTouchTimes(pipe, parent.ID, now)
			c.queueCreateInfo(pipe, inode)
			pipe.Set(ctx, c.keys.rootDirty(), "1", 0)
			c.queueInvalidation(ctx, pipe, InvalidateOpInode, childPath)
			return nil
		})
		return err
	})
	if err != nil {
		return err
	}

	// Only the parent's dir listing is now stale; its path entry is still
	// valid (same inode id, just a bumped mtime). Update the parent's cached
	// mtime in place instead of wiping the entry, so the next lookup for a
	// sibling does not pay a parent re-resolve RTT.
	parentPath := parentOf(childPath)
	parent.MtimeMs = now
	parent.CtimeMs = now
	c.cachePath(parentPath, parent)
	c.invalidateDirListing(ctx, parentPath)
	c.cachePath(childPath, inode)
	// Peers also need to know a new inode appeared at childPath so their
	// cached stat (typically a negative-lookup entry) gets dropped.
	c.publishInvalidate(ctx, InvalidateOpInode, childPath)
	return nil
}

// createFileIfMissing retains the atomic name claim and inode representation.
// Content is staged before the name claim so interrupted creation cannot leave
// a visible empty file or dangling directory entry.
func (c *nativeClient) createFileIfMissing(ctx context.Context, p string, content string, mode uint32, exclusive bool) (*inodeData, bool, error) {
	p = normalizePath(p)
	if p == "/" {
		return nil, false, ErrCannotWriteRoot
	}
	parentPath := parentOf(p)
	_, parent, err := c.resolvePath(ctx, parentPath, true)
	if err != nil {
		return nil, false, err
	}
	if parent.Type != "dir" {
		return nil, false, ErrParentConflict
	}
	if err := checkExpectedParent(ctx, p, parent.ID); err != nil {
		return nil, false, err
	}
	id, err := c.allocInodeID(ctx)
	if err != nil {
		return nil, false, err
	}
	now := nowMs()
	inode := &inodeData{ID: id, Parent: parent.ID, Name: baseName(p), Type: "file", Mode: mode, Size: int64(len(content)), CtimeMs: now, MtimeMs: now, AtimeMs: now, Content: content}
	if err := c.selectContentRef(ctx, inode); err != nil {
		return nil, false, err
	}
	if err := checkWriteCondition(ctx, inode, true); err != nil {
		return nil, false, err
	}
	stage, err := c.stageFullFile(ctx, inode)
	if err != nil {
		return nil, false, err
	}
	defer c.discardStage(stage)
	err = c.publishStagedFile(ctx, p, inode, stage, true, fileSearchIndexFields(inode.Content))
	if err == nil {
		parent.MtimeMs = now
		parent.CtimeMs = now
		c.cachePath(parentPath, parent)
		c.invalidateDirListing(ctx, parentPath)
		c.publishInvalidate(ctx, InvalidateOpInode, p)
		return inode, true, nil
	}
	if !errors.Is(err, ErrWriteConflict) {
		return nil, false, err
	}
	if _, conditional := ctx.Value(expectedStatKey{}).(expectedStat); conditional {
		return nil, false, ErrWriteConflict
	}
	if parents, _ := ctx.Value(expectedParentsKey{}).(expectedParents); len(parents) > 0 {
		return nil, false, ErrWriteConflict
	}
	if exclusive {
		return nil, false, ErrAlreadyExists
	}
	existingID, err := c.rdb.HGet(ctx, c.keys.dirents(parent.ID), baseName(p)).Result()
	if err != nil {
		return nil, false, err
	}
	existing, err := c.loadInodeByID(ctx, existingID)
	if err != nil {
		return nil, false, err
	}
	if existing == nil {
		return nil, false, ErrNotFound
	}
	if existing.Type != "file" {
		return nil, false, ErrNotFile
	}
	c.cachePath(p, existing)
	return existing, false, nil
}

func (c *nativeClient) renamePath(ctx context.Context, resolvedSrc string, srcInode *inodeData, dst string, newParent *inodeData, flags uint32) error {
	if err := checkExpectedParent(ctx, resolvedSrc, srcInode.Parent); err != nil {
		return err
	}
	if err := checkExpectedParent(ctx, dst, newParent.ID); err != nil {
		return err
	}
	if err := checkWriteCondition(ctx, srcInode, false); err != nil {
		return err
	}
	oldParentID := srcInode.Parent
	oldName := srcInode.Name
	newName := baseName(dst)
	if oldParentID == "" {
		return ErrNotFound
	}

	var watchDstDirID string
	for attempts := 0; attempts < 8; attempts++ {
		keys := uniqueStrings(c.keys.dirents(oldParentID), c.keys.dirents(newParent.ID), c.keys.inode(srcInode.ID))
		parentKeys := uniqueStrings(c.keys.inode(oldParentID), c.keys.inode(newParent.ID))
		keys = uniqueStrings(append(keys, parentKeys...)...)
		if watchDstDirID != "" {
			keys = uniqueStrings(append(keys, c.keys.dirents(watchDstDirID))...)
		}

		var nextDstDirID string
		err := c.retryWatch(ctx, keys, func(tx *redis.Tx) error {
			live, err := tx.Exists(ctx, parentKeys...).Result()
			if err != nil {
				return err
			}
			if live != int64(len(parentKeys)) {
				return ErrWriteConflict
			}
			currentSrcID, err := tx.HGet(ctx, c.keys.dirents(oldParentID), oldName).Result()
			if err != nil {
				if errors.Is(err, redis.Nil) {
					return ErrNotFound
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
				return ErrNotFound
			}
			if currentSrc.Revision != srcInode.Revision {
				return ErrWriteConflict
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
					return ErrAlreadyExists
				}
				if currentSrc.Type == "dir" && replaced.Type != "dir" {
					return ErrNotDir
				}
				if currentSrc.Type != "dir" && replaced.Type == "dir" {
					return ErrNotFile
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
						return ErrDirNotEmpty
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
					if replaced.Type == "file" {
						pipe.Del(ctx, c.keys.content(replaced.ID))

					}
					if replaced.Type == "dir" {
						pipe.Del(ctx, c.keys.dirents(replaced.ID))
					}
					c.queueDeleteInfo(pipe, replaced)
				}
				pipe.Set(ctx, c.keys.rootDirty(), "1", 0)
				c.queueInvalidation(ctx, pipe, InvalidateOpPrefix, resolvedSrc, dst)
				if replaced != nil {
					c.queueQueryDeleted(ctx, pipe, replaced.ID)
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
			c.invalidateInode(ctx, parentOf(resolvedSrc))
			c.invalidateInode(ctx, parentOf(dst))
			c.invalidatePrefix(ctx, resolvedSrc)
			c.invalidatePrefix(ctx, dst)
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
		return nil, ErrNotDir
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
	generation, managed := ctx.Value(workspaceGenerationKey{}).(string)
	lease := nativeLease(ctx)
	if lease != "" {
		keys = uniqueStrings(append(keys, lease)...)
	}
	if managed {
		keys = uniqueStrings(append(keys, c.keys.generation())...)
	}
	guarded := func(tx *redis.Tx) error {
		if lease != "" {
			if exists, err := tx.Exists(ctx, lease).Result(); err != nil {
				return err
			} else if exists == 0 {
				return ErrNativeSessionLost
			}
		}
		if managed {
			actual, err := tx.Get(ctx, c.keys.generation()).Result()
			if errors.Is(err, redis.Nil) || (err == nil && actual != generation) {
				return ErrWorkspaceChanged
			}
			if err != nil {
				return err
			}
		}
		if err := c.watchExpectedParents(ctx, tx); err != nil {
			return err
		}
		return fn(tx)
	}
	for attempts := 0; attempts < 16; attempts++ {
		err := c.rdb.Watch(ctx, guarded, keys...)
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
	if inode.ContentRef != "" {
		fields["content_ref"] = inode.ContentRef
	}
	if includeContent && inode.Type == "file" && !isExternalContentRef(inode.ContentRef) {
		// Legacy inline mode: content stored in the inode HASH.
		fields["content"] = inode.Content
	}
	// When content_ref points at an external backend, the caller is responsible
	// for writing content to the separate content key via c.keys.content(id).
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
	return c.retryWatch(ctx, []string{c.keys.inode(inode.ID)}, func(tx *redis.Tx) error {
		_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error { return c.queueRefreshIndexedSubtree(ctx, pipe, rootPath, inode) })
		return err
	})
}

func (c *nativeClient) queueRefreshIndexedSubtree(ctx context.Context, pipe redis.Pipeliner, currentPath string, inode *inodeData) error {
	if inode.Type == "file" {
		c.queueQueryDirty(ctx, pipe, inode.ID)
	}
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

func (c *nativeClient) markRootDirty(ctx context.Context) error {
	// Throttle: within the debounce window, skip the Redis round trip.
	// See the comment on nativeClient.dirtyMu for rationale.
	c.dirtyMu.Lock()
	if time.Since(c.dirtyLastSent) < markRootDirtyThrottle {
		c.dirtyMu.Unlock()
		return nil
	}
	c.dirtyLastSent = time.Now()
	c.dirtyMu.Unlock()
	if generation, managed := ctx.Value(workspaceGenerationKey{}).(string); managed {
		code, err := markRootDirtyGuarded.Run(ctx, c.rdb, []string{c.keys.rootDirty(), c.keys.generation(), c.leaseGuardKey(ctx)}, generation).Int()
		if err != nil {
			return err
		}
		if code == -2 {
			return ErrWorkspaceChanged
		}
		if code == -5 {
			return ErrNativeSessionLost
		}
		return nil
	}
	return c.rdb.Set(ctx, c.keys.rootDirty(), "1", 0).Err()
}

var markRootDirtyGuarded = redis.NewScript(`
if redis.call('GET',KEYS[2])~=ARGV[1] then return -2 end
if KEYS[3]~=KEYS[2] and redis.call('EXISTS',KEYS[3])==0 then return -5 end
redis.call('SET',KEYS[1],'1')
return 1
`)

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
	clone.committedSnapshot = nil
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
	inode := &inodeData{
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
	// content_ref was added after the initial schema; tolerate its absence.
	if len(vals) > 11 {
		inode.ContentRef = toStr(vals[11])
	}
	if len(vals) > 12 {
		inode.Revision = toStr(vals[12])
	}
	return inode
}

func (c *nativeClient) preferredContentRef(ctx context.Context) (string, error) {
	return rediscontent.PreferredRef(ctx, c.rdb)
}

func (c *nativeClient) selectContentRef(ctx context.Context, inode *inodeData) error {
	if inode == nil || inode.Type != "file" {
		return nil
	}
	preferredRef, err := c.preferredContentRef(ctx)
	if err != nil {
		return err
	}
	switch inode.ContentRef {
	case "":
		inode.ContentRef = preferredRef
	case rediscontent.RefExternal:
		if preferredRef == rediscontent.RefArray {
			inode.ContentRef = preferredRef
		}
	}
	return nil
}

func (c *nativeClient) loadInodeSize(ctx context.Context, id string) (int64, error) {
	value, err := c.rdb.HGet(ctx, c.keys.inode(id), "size").Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	n, parseErr := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if parseErr != nil {
		return 0, nil
	}
	return n, nil
}

func isExternalContentRef(ref string) bool {
	switch strings.TrimSpace(ref) {
	case rediscontent.RefExternal, rediscontent.RefArray:
		return true
	default:
		return false
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
