package helpers

import (
	"io/fs"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
)

type stubInfo string

func (s stubInfo) Name() string       { return string(s) }
func (s stubInfo) Size() int64        { return 0 }
func (s stubInfo) Mode() fs.FileMode  { return 0 }
func (s stubInfo) ModTime() time.Time { return time.Time{} }
func (s stubInfo) IsDir() bool        { return false }
func (s stubInfo) Sys() any           { return nil }

func TestCachingHandlerVerifierCacheMatchesPathAndCanInvalidate(t *testing.T) {
	handler := NewCachingHandlerWithVerifierLimit(NewNullAuthHandler(memfs.New()), 32, 32)
	cache, ok := handler.(*CachingHandler)
	if !ok {
		t.Fatalf("handler type = %T, want *CachingHandler", handler)
	}

	entries := []fs.FileInfo{stubInfo("a.txt")}
	verifier := cache.VerifierFor("/dir", entries)

	if got := cache.DataForVerifier("/other", verifier); got != nil {
		t.Fatalf("path-mismatched verifier lookup = %v, want nil", got)
	}
	if got := cache.DataForVerifier("/dir", verifier); len(got) != 1 || got[0].Name() != "a.txt" {
		t.Fatalf("path-matched verifier lookup = %v, want [a.txt]", got)
	}

	cache.InvalidateVerifier("/dir")
	if got := cache.DataForVerifier("/dir", verifier); got != nil {
		t.Fatalf("verifier lookup after invalidation = %v, want nil", got)
	}
}
