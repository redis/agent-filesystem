package client

import (
	"context"
	"errors"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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

	// originID is this client's opaque publisher ID, included on every
	// outgoing invalidation message. The subscriber skips any message
	// whose origin matches, so self-publishes are ignored (local cache
	// state was already updated synchronously at the mutation site).
	originID string

	// publishDisabled, when set via DisableInvalidationPublishing(), makes
	// publishInvalidate a no-op. Used by the --disable-cross-client-
	// invalidation flag. Local cache state still tracks mutations.
	publishDisabled atomic.Bool

	// dirtyMu protects the markRootDirty throttle window.
	//
	// Rationale: every mutating client op (Chmod/Chown/Utimens/Write/
	// Truncate/CreateFile/Mkdir/Rm/Rename) ends with a
	// `SET rootDirty "1"` to tell the control plane that the workspace
	// has unflushed changes. This is idempotent — the value is always
	// "1" — so doing it on every single op wastes a full Redis round
	// trip per mutation. Under bursty writes (e.g. a shell loop that
	// creates 50 files) that is the single largest contributor to the
	// wall-clock cost of each op.
	//
	// Throttle: if we already issued a SET within the recent window
	// (see markRootDirtyThrottle), skip the round trip. The worst case
	// is that a reader observing the marker lags by up to that window,
	// which is acceptable because the marker is a hint, not a lock.
	dirtyMu       sync.Mutex
	dirtyLastSent time.Time
}

// markRootDirtyThrottle is the minimum interval between real Redis
// writes of the rootDirty marker. Picking ~100ms collapses a burst of
// sequential metadata ops into a single round trip while still letting
// a genuinely idle-then-active workspace signal dirtiness within a
// short human-perceivable delay.
const markRootDirtyThrottle = 100 * time.Millisecond

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
		rdb:      rdb,
		key:      key,
		keys:     newKeyBuilder(key),
		originID: newOriginID(),
	}
}

func newNativeClientWithCache(rdb *redis.Client, key string, ttl time.Duration) Client {
	return &nativeClient{
		rdb:      rdb,
		key:      key,
		keys:     newKeyBuilder(key),
		cache:    cache.New(ttl),
		originID: newOriginID(),
	}
}

// OriginID returns this client's publisher ID. See client.go for semantics.
func (c *nativeClient) OriginID() string {
	return c.originID
}

// DisableInvalidationPublishing makes subsequent publishInvalidate calls a
// no-op. Local cache updates are unaffected. This is the implementation of
// the --disable-cross-client-invalidation escape hatch.
func (c *nativeClient) DisableInvalidationPublishing() {
	c.publishDisabled.Store(true)
}

// invalidateInode drops the path's own cache entry AND (implicitly, via the
// dirCacheKey entry) any cached directory listing of which it is a child, and
// broadcasts the same invalidation to peer clients over pub/sub.
func (c *nativeClient) invalidateInode(ctx context.Context, p string) {
	if c.cache != nil {
		c.cache.Invalidate(p)
		c.cache.Invalidate(dirCacheKey(p))
	}
	c.publishInvalidate(ctx, InvalidateOpInode, p)
}

// invalidateDirListing drops only the cached READDIR listing for a directory
// while preserving the directory's own path-cache entry. Use this after
// creating or deleting a child: the parent inode's identity has not changed,
// only its listing is now stale. Dropping just the listing lets subsequent
// path lookups through the parent continue to hit the cache instead of
// paying a parent re-resolve RTT. The same narrow invalidation is broadcast
// to peer clients.
func (c *nativeClient) invalidateDirListing(ctx context.Context, p string) {
	if c.cache != nil {
		c.cache.Invalidate(dirCacheKey(p))
	}
	c.publishInvalidate(ctx, InvalidateOpDir, p)
}

// invalidatePrefix drops every cached path and dir-listing entry under the
// given prefix, and broadcasts a matching prefix invalidation to peer
// clients. Use for subtree-wide events like a directory rename.
func (c *nativeClient) invalidatePrefix(ctx context.Context, prefix string) {
	if c.cache != nil {
		c.cache.InvalidatePrefix(prefix)
		c.cache.InvalidatePrefix(dirCachePrefix(prefix))
	}
	c.publishInvalidate(ctx, InvalidateOpPrefix, prefix)
}

// publishInvalidate broadcasts an invalidation event on this FS key's pub/sub
// channel. Failure to publish is logged but never fails the enclosing
// mutation: local state is already correct, and missing a broadcast merely
// degrades peers to TTL-based staleness (today's behavior).
func (c *nativeClient) publishInvalidate(ctx context.Context, op string, paths ...string) {
	if c.publishDisabled.Load() {
		return
	}
	if len(paths) == 0 {
		return
	}
	// Strip empty strings defensively so we don't confuse peers.
	cleaned := paths[:0]
	for _, p := range paths {
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	if len(cleaned) == 0 {
		return
	}
	payload, err := encodeInvalidate(InvalidateEvent{
		Origin: c.originID,
		Op:     op,
		Paths:  cleaned,
	})
	if err != nil {
		log.Printf("afs: invalidate encode failed op=%s paths=%v: %v", op, cleaned, err)
		return
	}
	if err := c.rdb.Publish(ctx, c.keys.invalidateChannel(), payload).Err(); err != nil {
		// Best-effort broadcast. Log once per failure; callers never see it.
		log.Printf("afs: invalidate publish failed op=%s paths=%v: %v", op, cleaned, err)
	}
}

// SubscribeInvalidations implements the Client interface. It runs a goroutine
// that listens on this FS key's pub/sub channel until ctx is cancelled,
// decoding each message, dropping matching entries from the local client
// cache, and then invoking handler with the event so callers can drive
// higher-level cache layers (afsfs attrCache/dirCache, FUSE kernel notifies).
//
// Messages whose origin matches this client's ID are filtered out before
// handler is called: the publisher has already invalidated its own local
// state at the mutation site, and a second flush would be a pointless waste.
//
// Redis outages cause the subscribe loop to log a warning and reconnect with
// exponential backoff capped at 5 seconds. During an outage, this client
// falls back to TTL-based staleness for cross-client updates.
func (c *nativeClient) SubscribeInvalidations(ctx context.Context, handler func(InvalidateEvent)) error {
	if handler == nil {
		handler = func(InvalidateEvent) {}
	}
	channel := c.keys.invalidateChannel()
	go c.runInvalidationSubscriber(ctx, channel, handler)
	return nil
}

func (c *nativeClient) runInvalidationSubscriber(ctx context.Context, channel string, handler func(InvalidateEvent)) {
	backoff := 100 * time.Millisecond
	const maxBackoff = 5 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}
		sub := c.rdb.Subscribe(ctx, channel)
		// Wait for the subscription to be confirmed before taking messages.
		if _, err := sub.Receive(ctx); err != nil {
			_ = sub.Close()
			if ctx.Err() != nil {
				return
			}
			log.Printf("afs: invalidate subscribe to %s failed: %v (retry in %s)", channel, err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		backoff = 100 * time.Millisecond // reset after a successful subscribe
		ch := sub.Channel()
		c.consumeInvalidationChannel(ctx, ch, handler)
		_ = sub.Close()
		if ctx.Err() != nil {
			return
		}
		// Connection dropped (channel closed without ctx cancellation);
		// loop around and resubscribe.
		log.Printf("afs: invalidate subscription to %s dropped, reconnecting", channel)
	}
}

func (c *nativeClient) consumeInvalidationChannel(ctx context.Context, ch <-chan *redis.Message, handler func(InvalidateEvent)) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			ev, err := decodeInvalidate([]byte(msg.Payload))
			if err != nil {
				log.Printf("afs: invalidate decode failed: %v (payload=%q)", err, msg.Payload)
				continue
			}
			if ev.Origin == c.originID {
				// Our own mutation — we invalidated locally at the
				// publish site already.
				continue
			}
			// Drop matching entries from the client-layer cache first so
			// the higher-level handler sees a consistent state when it
			// walks caches above us.
			c.applyRemoteInvalidation(ev)
			handler(*ev)
		}
	}
}

// applyRemoteInvalidation mirrors the local-invalidate behavior of
// invalidateInode/invalidateDirListing/invalidatePrefix against an event
// received from a peer. It does NOT re-broadcast: publishing from a
// subscriber callback would create an infinite loop.
func (c *nativeClient) applyRemoteInvalidation(ev *InvalidateEvent) {
	if c.cache == nil || ev == nil {
		return
	}
	for _, p := range ev.Paths {
		if p == "" {
			continue
		}
		switch ev.Op {
		case InvalidateOpInode:
			c.cache.Invalidate(p)
			c.cache.Invalidate(dirCacheKey(p))
			// Peer also bumped the parent's dir listing mtime. Drop it
			// so our next Ls re-fetches from Redis.
			c.cache.Invalidate(dirCacheKey(parentOf(p)))
		case InvalidateOpDir:
			c.cache.Invalidate(dirCacheKey(p))
		case InvalidateOpPrefix:
			c.cache.InvalidatePrefix(p)
			c.cache.InvalidatePrefix(dirCachePrefix(p))
		case InvalidateOpContent:
			// Peer changed file bytes. The path entry carries size and
			// mtime, both of which are now stale.
			c.cache.Invalidate(p)
			c.cache.Invalidate(dirCacheKey(parentOf(p)))
		}
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
	c.publishInvalidate(ctx, InvalidateOpInode, resolved)
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

	c.invalidateInode(ctx, resolved)
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
	c.invalidateInode(ctx, parentPath)
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
	c.publishInvalidate(ctx, InvalidateOpInode, resolved)
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
	c.publishInvalidate(ctx, InvalidateOpInode, resolved)
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
	c.publishInvalidate(ctx, InvalidateOpContent, resolved)
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
	c.publishInvalidate(ctx, InvalidateOpInode, resolved)
	return c.markRootDirty(ctx)
}

// SetAttrs applies the non-nil fields in upd to the inode at p in one
// partial HSet. This is the batched fast path for NFS SETATTR / CREATE,
// collapsing the Chmod + Chown + Utimens triple (3 RTTs) into a single
// HSet (1 RTT) and skipping the round trip entirely when upd is empty.
//
// Reads are through the warm path cache, so resolvePath is typically
// 0 RTTs. The saved HSet payload contains only the fields that changed —
// we deliberately do NOT use saveInodeMeta here because that helper writes
// the full 13-field metadata map via inodeFieldsAtPath, which would rebuild
// the path_ancestors CSV and ship a dozen unchanged fields over the wire.
func (c *nativeClient) SetAttrs(ctx context.Context, p string, upd AttrUpdate) error {
	if upd.IsEmpty() {
		return nil
	}
	resolved, inode, err := c.resolvePath(ctx, p, false)
	if err != nil {
		return err
	}

	// Mutate in-memory inode so the subsequent cachePath reflects the new
	// state; also build a sparse HSet map of only the fields that changed.
	fields := make(map[string]interface{}, 5)
	if upd.Mode != nil {
		inode.Mode = *upd.Mode
		fields["mode"] = inode.Mode
	}
	if upd.UID != nil {
		inode.UID = *upd.UID
		fields["uid"] = inode.UID
	}
	if upd.GID != nil {
		inode.GID = *upd.GID
		fields["gid"] = inode.GID
	}
	if upd.AtimeMs != nil {
		inode.AtimeMs = *upd.AtimeMs
		fields["atime_ms"] = inode.AtimeMs
	}
	if upd.MtimeMs != nil {
		inode.MtimeMs = *upd.MtimeMs
		fields["mtime_ms"] = inode.MtimeMs
	}

	if err := c.rdb.HSet(ctx, c.keys.inode(inode.ID), fields).Err(); err != nil {
		return err
	}
	c.cachePath(resolved, inode)
	c.publishInvalidate(ctx, InvalidateOpInode, resolved)
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
	c.publishInvalidate(ctx, InvalidateOpContent, resolved)
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
	c.publishInvalidate(ctx, InvalidateOpContent, resolved)
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
