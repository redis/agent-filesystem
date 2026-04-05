// Package cache provides TTL-based caching for filesystem attributes and directory listings.
package cache

import (
	"sync"
	"time"
)

// AttrEntry is a cached attribute entry.
type AttrEntry struct {
	Data   interface{}
	Expiry time.Time
}

// Cache provides thread-safe TTL-based caching.
type Cache struct {
	mu  sync.RWMutex
	m   map[string]AttrEntry
	ttl time.Duration
}

// New creates a cache with the given TTL.
func New(ttl time.Duration) *Cache {
	return &Cache{
		m:   make(map[string]AttrEntry),
		ttl: ttl,
	}
}

// Get returns the cached value and true if found and not expired.
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	entry, ok := c.m[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.Expiry) {
		return nil, false
	}
	return entry.Data, true
}

// Set stores a value in the cache.
func (c *Cache) Set(key string, data interface{}) {
	c.mu.Lock()
	c.m[key] = AttrEntry{Data: data, Expiry: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

// Invalidate removes a key from the cache.
func (c *Cache) Invalidate(key string) {
	c.mu.Lock()
	delete(c.m, key)
	c.mu.Unlock()
}

// InvalidatePrefix removes all keys with the given prefix.
func (c *Cache) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	for k := range c.m {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.m, k)
		}
	}
	c.mu.Unlock()
}

// InvalidateAll clears the entire cache.
func (c *Cache) InvalidateAll() {
	c.mu.Lock()
	c.m = make(map[string]AttrEntry)
	c.mu.Unlock()
}
