package helpers

import (
	"crypto/sha256"
	"encoding/binary"
	"io/fs"
	"reflect"
	"sync"

	"github.com/willscott/go-nfs"

	"github.com/go-git/go-billy/v5"
	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"
)

// NewCachingHandler wraps a handler to provide a basic to/from-file handle cache.
func NewCachingHandler(h nfs.Handler, limit int) nfs.Handler {
	return NewCachingHandlerWithVerifierLimit(h, limit, limit)
}

// NewCachingHandlerWithVerifierLimit provides a basic to/from-file handle cache that can be tuned with a smaller cache of active directory listings.
func NewCachingHandlerWithVerifierLimit(h nfs.Handler, limit int, verifierLimit int) nfs.Handler {
	if limit < 2 || verifierLimit < 2 {
		nfs.Log.Warnf("Caching handler created with insufficient cache to support directory listing", "size", limit, "verifiers", verifierLimit)
	}
	cache, _ := lru.New[uuid.UUID, entry](limit)
	reverseCache := make(map[string][]uuid.UUID)
	verifiers, _ := lru.New[uint64, verifier](verifierLimit)
	return &CachingHandler{
		Handler:         h,
		activeHandles:   cache,
		reverseHandles:  reverseCache,
		activeVerifiers: verifiers,
		cacheLimit:      limit,
	}
}

// CachingHandler implements to/from handle via an LRU cache.
type CachingHandler struct {
	nfs.Handler
	mu              sync.Mutex
	activeHandles   *lru.Cache[uuid.UUID, entry]
	reverseHandles  map[string][]uuid.UUID
	activeVerifiers *lru.Cache[uint64, verifier]
	cacheLimit      int
}

type entry struct {
	f billy.Filesystem
	p []string
}

// ToHandle takes a file and represents it with an opaque handle to reference it.
// In stateless nfs (when it's serving a unix fs) this can be the device + inode
// but we can generalize with a stateful local cache of handed out IDs.
func (c *CachingHandler) ToHandle(f billy.Filesystem, path []string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	joinedPath := f.Join(path...)

	if handle := c.searchReverseCacheLocked(f, joinedPath); handle != nil {
		return handle
	}

	id := uuid.New()

	newPath := make([]string, len(path))

	copy(newPath, path)
	evictedKey, evictedPath, ok := c.activeHandles.GetOldest()
	if evicted := c.activeHandles.Add(id, entry{f, newPath}); evicted && ok {
		rk := evictedPath.f.Join(evictedPath.p...)
		c.evictReverseCacheLocked(rk, evictedKey)
	}

	if _, ok := c.reverseHandles[joinedPath]; !ok {
		c.reverseHandles[joinedPath] = []uuid.UUID{}
	}
	c.reverseHandles[joinedPath] = append(c.reverseHandles[joinedPath], id)
	b, _ := id.MarshalBinary()

	return b
}

// FromHandle converts from an opaque handle to the file it represents
func (c *CachingHandler) FromHandle(fh []byte) (billy.Filesystem, []string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id, err := uuid.FromBytes(fh)
	if err != nil {
		return nil, []string{}, err
	}

	if f, ok := c.activeHandles.Get(id); ok {
		for _, k := range c.activeHandles.Keys() {
			candidate, _ := c.activeHandles.Peek(k)
			if hasPrefix(f.p, candidate.p) {
				_, _ = c.activeHandles.Get(k)
			}
		}
		if ok {
			newP := make([]string, len(f.p))
			copy(newP, f.p)
			return f.f, newP, nil
		}
	}
	return nil, []string{}, &nfs.NFSStatusError{NFSStatus: nfs.NFSStatusStale}
}

func (c *CachingHandler) searchReverseCacheLocked(f billy.Filesystem, path string) []byte {
	uuids, exists := c.reverseHandles[path]

	if !exists {
		return nil
	}

	for _, id := range uuids {
		if candidate, ok := c.activeHandles.Get(id); ok {
			if reflect.DeepEqual(candidate.f, f) {
				return id[:]
			}
		}
	}

	return nil
}

func (c *CachingHandler) evictReverseCacheLocked(path string, handle uuid.UUID) {
	uuids, exists := c.reverseHandles[path]

	if !exists {
		return
	}
	for i, u := range uuids {
		if u == handle {
			uuids = append(uuids[:i], uuids[i+1:]...)
			c.reverseHandles[path] = uuids
			return
		}
	}
}

func (c *CachingHandler) InvalidateHandle(fs billy.Filesystem, handle []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	//Remove from cache
	id, _ := uuid.FromBytes(handle)
	entry, ok := c.activeHandles.Get(id)
	if ok {
		rk := entry.f.Join(entry.p...)
		c.evictReverseCacheLocked(rk, id)
	}
	c.activeHandles.Remove(id)
	return nil
}

// RenameHandle preserves stable file handles across rename by remapping any
// cached paths under oldPath onto the newPath subtree.
func (c *CachingHandler) RenameHandle(fs billy.Filesystem, oldPath, newPath []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, id := range c.activeHandles.Keys() {
		current, ok := c.activeHandles.Peek(id)
		if !ok || !reflect.DeepEqual(current.f, fs) {
			continue
		}
		if !hasPrefix(current.p, oldPath) {
			continue
		}

		oldJoined := current.f.Join(current.p...)
		c.evictReverseCacheLocked(oldJoined, id)

		updated := make([]string, 0, len(newPath)+len(current.p)-len(oldPath))
		updated = append(updated, newPath...)
		if len(current.p) > len(oldPath) {
			updated = append(updated, current.p[len(oldPath):]...)
		}
		c.activeHandles.Add(id, entry{f: current.f, p: updated})

		newJoined := current.f.Join(updated...)
		c.reverseHandles[newJoined] = append(c.reverseHandles[newJoined], id)
	}
	return nil
}

// HandleLimit exports how many file handles can be safely stored by this cache.
func (c *CachingHandler) HandleLimit() int {
	return c.cacheLimit
}

func hasPrefix(path, prefix []string) bool {
	if len(prefix) > len(path) {
		return false
	}
	for i, e := range prefix {
		if path[i] != e {
			return false
		}
	}
	return true
}

type verifier struct {
	path     string
	contents []fs.FileInfo
}

func hashPathAndContents(path string, contents []fs.FileInfo) uint64 {
	//calculate a cookie-verifier.
	vHash := sha256.New()

	// Add the path to avoid collisions of directories with the same content
	vHash.Write(binary.BigEndian.AppendUint64([]byte{}, uint64(len(path))))
	vHash.Write([]byte(path))

	for _, c := range contents {
		vHash.Write([]byte(c.Name())) // Never fails according to the docs
	}

	verify := vHash.Sum(nil)[0:8]
	return binary.BigEndian.Uint64(verify)
}

func (c *CachingHandler) VerifierFor(path string, contents []fs.FileInfo) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := hashPathAndContents(path, contents)
	c.activeVerifiers.Add(id, verifier{path, contents})
	return id
}

func (c *CachingHandler) DataForVerifier(path string, id uint64) []fs.FileInfo {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cache, ok := c.activeVerifiers.Get(id); ok {
		if cache.path == path {
			return cache.contents
		}
	}
	return nil
}

func (c *CachingHandler) InvalidateVerifier(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, id := range c.activeVerifiers.Keys() {
		cache, ok := c.activeVerifiers.Peek(id)
		if !ok || cache.path != path {
			continue
		}
		c.activeVerifiers.Remove(id)
	}
}
