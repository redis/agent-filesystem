package controlplane_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/internal/worktree"
	"github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

// importPipelineSink composes a BlobWriter with an in-memory cache so the
// subsequent SyncWorkspaceRootWithOptions call can resolve blobs without
// reading them back from Redis. Mirrors the structure used by cmdImport.
type importPipelineSink struct {
	ctx    context.Context
	writer *controlplane.BlobWriter
	mu     sync.Mutex
	cache  map[string][]byte
}

func (s *importPipelineSink) Submit(id string, data []byte, size int64) error {
	s.mu.Lock()
	if _, ok := s.cache[id]; ok {
		s.mu.Unlock()
		return nil
	}
	s.cache[id] = data
	s.mu.Unlock()
	return s.writer.Submit(s.ctx, id, data, size)
}

func (s *importPipelineSink) Provider(id string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.cache[id]
	return data, ok
}

// TestImportPipelineEndToEnd exercises the full streaming import pipeline
// (build → blob writer → sync with provider → skip namespace reset) against
// miniredis. It verifies that blob bodies round-trip through Redis exactly
// once (write during import) and that the materialized workspace FS exposes
// the expected contents.
func TestImportPipelineEndToEnd(t *testing.T) {
	// Build a fixture tree.
	src := t.TempDir()
	mkdir := func(rel string) {
		if err := os.MkdirAll(filepath.Join(src, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mkfile := func(rel string, data []byte) {
		if err := os.WriteFile(filepath.Join(src, rel), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mkdir("docs")
	mkdir("src/deep")
	mkfile("README.md", []byte("# demo\n"))
	mkfile("docs/guide.md", []byte("hello world\n"))
	mkfile("src/deep/main.go", []byte("package main\n"))

	// Three large files share the same content → one blob should be written.
	large := bytes.Repeat([]byte("x"), controlplane.InlineThreshold+256)
	mkfile("large1.bin", large)
	mkfile("large2.bin", large)
	mkfile("large3.bin", large)

	// A distinct large file to confirm different hashes coexist.
	otherLarge := bytes.Repeat([]byte("y"), controlplane.InlineThreshold+300)
	mkfile("other.bin", otherLarge)

	// Start miniredis + store.
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := controlplane.NewStore(rdb)
	ctx := context.Background()

	// Acquire the import lock (required for a real import).
	lock, err := controlplane.AcquireImportLock(ctx, store, "demo")
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	defer lock.Release(ctx)

	// Build manifest with streaming sink.
	writer := controlplane.NewBlobWriter(rdb, "demo", time.Now())
	sink := &importPipelineSink{ctx: ctx, writer: writer, cache: make(map[string][]byte)}

	m, _, stats, err := worktree.BuildManifestFromDirectory(src, "demo", "initial", worktree.BuildManifestOptions{Sink: sink})
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if err := writer.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	blobCount, _ := writer.Totals()
	if blobCount != 2 {
		t.Fatalf("unique blobs written = %d, want 2", blobCount)
	}
	if stats.FileCount != 7 {
		t.Fatalf("file count = %d, want 7", stats.FileCount)
	}

	// Write savepoint + workspace meta (minimal subset).
	now := time.Now().UTC()
	hash, err := controlplane.HashManifest(m)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	meta := controlplane.WorkspaceMeta{
		Version:          controlplane.FormatVersion,
		Name:             "demo",
		CreatedAt:        now,
		UpdatedAt:        now,
		HeadSavepoint:    "initial",
		DefaultSavepoint: "initial",
	}
	if err := store.PutWorkspaceMeta(ctx, meta); err != nil {
		t.Fatalf("put workspace meta: %v", err)
	}
	if err := store.PutSavepoint(ctx, controlplane.SavepointMeta{
		Version:      controlplane.FormatVersion,
		ID:           "initial",
		Name:         "initial",
		Workspace:    "demo",
		ManifestHash: hash,
		CreatedAt:    now,
		FileCount:    stats.FileCount,
		DirCount:     stats.DirCount,
		TotalBytes:   stats.TotalBytes,
	}, m); err != nil {
		t.Fatalf("put savepoint: %v", err)
	}

	// Sync workspace root using the in-memory cache (no extra Redis reads).
	if err := controlplane.SyncWorkspaceRootWithOptions(ctx, store, "demo", m, controlplane.SyncOptions{
		BlobProvider:       sink.Provider,
		SkipNamespaceReset: true,
	}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Verify: both large blob keys exist, each written exactly once.
	largeID := m.Entries["/large1.bin"].BlobID
	if largeID == "" {
		t.Fatalf("expected /large1.bin to have a BlobID")
	}
	if m.Entries["/large2.bin"].BlobID != largeID || m.Entries["/large3.bin"].BlobID != largeID {
		t.Fatalf("duplicate large files should share blob IDs")
	}
	otherID := m.Entries["/other.bin"].BlobID
	if otherID == "" || otherID == largeID {
		t.Fatalf("other.bin should have a distinct BlobID")
	}

	for _, id := range []string{largeID, otherID} {
		if exists, _ := rdb.Exists(ctx, controlplane.BlobKey("demo", id)).Result(); exists != 1 {
			t.Fatalf("blob %s should exist in redis", id)
		}
		refRaw, err := rdb.Get(ctx, controlplane.BlobRefKey("demo", id)).Bytes()
		if err != nil {
			t.Fatalf("get ref %s: %v", id, err)
		}
		if len(refRaw) == 0 {
			t.Fatalf("ref %s empty", id)
		}
	}

	// Verify: the materialized workspace FS exposes the large content using
	// the mount client (which reads the inode hash just like the real mount).
	fsClient := client.New(rdb, controlplane.WorkspaceFSKey("demo"))
	for _, path := range []string{"/large1.bin", "/large2.bin", "/large3.bin"} {
		data, err := fsClient.Cat(ctx, path)
		if err != nil {
			t.Fatalf("cat %s: %v", path, err)
		}
		if !bytes.Equal(data, large) {
			t.Fatalf("%s content mismatch", path)
		}
	}
	otherData, err := fsClient.Cat(ctx, "/other.bin")
	if err != nil {
		t.Fatalf("cat other.bin: %v", err)
	}
	if !bytes.Equal(otherData, otherLarge) {
		t.Fatalf("other.bin content mismatch")
	}
}

// TestImportPipelineRefusesConcurrent verifies that while the lock is held,
// CheckImportLock-based gates refuse with ErrImportInProgress.
func TestImportPipelineRefusesConcurrent(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := controlplane.NewStore(rdb)
	ctx := context.Background()

	lock, err := controlplane.AcquireImportLock(ctx, store, "demo")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer lock.Release(ctx)

	if err := controlplane.CheckImportLock(ctx, store, "demo"); !errors.Is(err, controlplane.ErrImportInProgress) {
		t.Fatalf("CheckImportLock should refuse while held: %v", err)
	}
}
