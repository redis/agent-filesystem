package worktree

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

type fakeSink struct {
	mu    sync.Mutex
	blobs map[string][]byte
}

func newFakeSink() *fakeSink {
	return &fakeSink{blobs: make(map[string][]byte)}
}

func (s *fakeSink) Submit(id string, data []byte, size int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make([]byte, len(data))
	copy(copied, data)
	s.blobs[id] = copied
	return nil
}

type errorSink struct{ err error }

func (e errorSink) Submit(id string, data []byte, size int64) error { return e.err }

func buildFixtureTree(t *testing.T) (string, []byte, []byte) {
	t.Helper()
	dir := t.TempDir()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hello\n"), 0o644))
	must(os.Mkdir(filepath.Join(dir, "src"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main\n"), 0o644))

	// Large file that exceeds the inline threshold.
	large := bytes.Repeat([]byte("abc"), controlplane.InlineThreshold)
	must(os.WriteFile(filepath.Join(dir, "src", "blob.bin"), large, 0o644))

	// Symlink.
	must(os.Symlink("src/main.go", filepath.Join(dir, "latest")))

	return dir, large, []byte("# hello\n")
}

func TestBuildManifestFromDirectoryWithoutSink(t *testing.T) {
	dir, large, readme := buildFixtureTree(t)

	m, blobs, stats, err := BuildManifestFromDirectory(dir, "demo", "initial", BuildManifestOptions{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if stats.FileCount != 3 {
		t.Fatalf("FileCount = %d, want 3", stats.FileCount)
	}
	// dir count excludes root "/"
	if stats.DirCount != 1 {
		t.Fatalf("DirCount = %d, want 1", stats.DirCount)
	}
	if stats.TotalBytes != int64(len("package main\n")+len(readme)+len(large)) {
		t.Fatalf("TotalBytes = %d", stats.TotalBytes)
	}

	entry, ok := m.Entries["/README.md"]
	if !ok {
		t.Fatalf("README.md missing from manifest")
	}
	if entry.Inline == "" {
		t.Fatalf("README.md should be inlined")
	}
	decoded, err := base64.StdEncoding.DecodeString(entry.Inline)
	if err != nil || !bytes.Equal(decoded, readme) {
		t.Fatalf("README inline decode mismatch: %v", err)
	}

	blobEntry, ok := m.Entries["/src/blob.bin"]
	if !ok {
		t.Fatalf("blob.bin missing from manifest")
	}
	if blobEntry.BlobID == "" {
		t.Fatalf("blob.bin should have a BlobID")
	}
	got, ok := blobs[blobEntry.BlobID]
	if !ok {
		t.Fatalf("blob map missing %s", blobEntry.BlobID)
	}
	if !bytes.Equal(got, large) {
		t.Fatalf("blob map bytes mismatch")
	}

	if sym, ok := m.Entries["/latest"]; !ok || sym.Type != "symlink" || sym.Target != "src/main.go" {
		t.Fatalf("latest symlink not captured: %+v", sym)
	}
}

func TestBuildManifestWithSinkDropsBlobMap(t *testing.T) {
	dir, large, _ := buildFixtureTree(t)
	sink := newFakeSink()

	m, blobs, _, err := BuildManifestFromDirectory(dir, "demo", "initial", BuildManifestOptions{Sink: sink})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if blobs != nil {
		t.Fatalf("blob map should be nil when sink is provided, got %d entries", len(blobs))
	}

	blobEntry := m.Entries["/src/blob.bin"]
	got, ok := sink.blobs[blobEntry.BlobID]
	if !ok {
		t.Fatalf("sink missing blob %q", blobEntry.BlobID)
	}
	if !bytes.Equal(got, large) {
		t.Fatalf("sink blob bytes mismatch")
	}
}

func TestBuildManifestPropagatesIgnore(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(rel string, data []byte) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("keep.txt", []byte("ok"))
	mustWrite("node_modules/a/index.js", []byte("nope"))
	mustWrite("node_modules/b/x.js", []byte("nope"))

	ignore := func(root, path string, d os.DirEntry) (bool, error) {
		rel, _ := filepath.Rel(root, path)
		return rel == "node_modules" || rel == "node_modules/a" || rel == "node_modules/b", nil
	}
	m, _, stats, err := BuildManifestFromDirectory(dir, "demo", "initial", BuildManifestOptions{Ignore: ignore})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if stats.FileCount != 1 {
		t.Fatalf("FileCount = %d, want 1", stats.FileCount)
	}
	if _, ok := m.Entries["/node_modules"]; ok {
		t.Fatalf("node_modules should be ignored")
	}
}

func TestBuildManifestReportsWorkerError(t *testing.T) {
	dir, _, _ := buildFixtureTree(t)

	wantErr := errors.New("boom")
	_, _, _, err := BuildManifestFromDirectory(dir, "demo", "initial", BuildManifestOptions{
		Sink:    errorSink{err: wantErr},
		Workers: 1,
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, wantErr) && err.Error() == "" {
		t.Fatalf("err = %v, want wrapping %v", err, wantErr)
	}
}

func TestBuildManifestParallelWorkersAllAgree(t *testing.T) {
	dir := t.TempDir()
	// Generate a larger tree to exercise parallelism.
	for i := 0; i < 200; i++ {
		sub := filepath.Join(dir, fmt.Sprintf("d%d", i%16))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		data := bytes.Repeat([]byte{byte(i % 255)}, 1024+i)
		if err := os.WriteFile(filepath.Join(sub, fmt.Sprintf("f%d.bin", i)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	serial, _, _, err := BuildManifestFromDirectory(dir, "demo", "initial", BuildManifestOptions{Workers: 1})
	if err != nil {
		t.Fatalf("serial: %v", err)
	}
	parallel, _, _, err := BuildManifestFromDirectory(dir, "demo", "initial", BuildManifestOptions{Workers: 8})
	if err != nil {
		t.Fatalf("parallel: %v", err)
	}
	if len(serial.Entries) != len(parallel.Entries) {
		t.Fatalf("entry counts differ: serial=%d parallel=%d", len(serial.Entries), len(parallel.Entries))
	}
	for path, entry := range serial.Entries {
		other, ok := parallel.Entries[path]
		if !ok {
			t.Fatalf("parallel missing %s", path)
		}
		if entry.Type != other.Type || entry.BlobID != other.BlobID || entry.Inline != other.Inline || entry.Size != other.Size {
			t.Fatalf("entry %s differs: serial=%+v parallel=%+v", path, entry, other)
		}
	}
}
