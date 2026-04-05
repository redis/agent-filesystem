package redisfs

import (
	"testing"
	"time"

	"github.com/redis-fs/mount/internal/cache"
)

func TestInvalidatePathPrefix(t *testing.T) {
	root := &FSRoot{
		FSNode: FSNode{
			attrCache: cache.New(time.Minute),
			dirCache:  cache.New(time.Minute),
		},
	}

	root.attrCache.Set("/a", 1)
	root.attrCache.Set("/a/b", 2)
	root.attrCache.Set("/x", 3)
	root.dirCache.Set("/a", 1)
	root.dirCache.Set("/a/b", 2)
	root.dirCache.Set("/", 3)

	root.invalidatePathPrefix("/a")

	if _, ok := root.attrCache.Get("/a"); ok {
		t.Fatalf("expected /a attr cache invalidated")
	}
	if _, ok := root.attrCache.Get("/a/b"); ok {
		t.Fatalf("expected /a/b attr cache invalidated")
	}
	if _, ok := root.dirCache.Get("/a"); ok {
		t.Fatalf("expected /a dir cache invalidated")
	}
	if _, ok := root.dirCache.Get("/a/b"); ok {
		t.Fatalf("expected /a/b dir cache invalidated")
	}
	if _, ok := root.attrCache.Get("/x"); !ok {
		t.Fatalf("expected unrelated attr cache entry to remain")
	}
	if _, ok := root.dirCache.Get("/"); ok {
		t.Fatalf("expected parent dir cache (/) invalidated")
	}
}
