package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStreamChunkHashes(t *testing.T) {
	dir := t.TempDir()

	// Create a file with known content spanning multiple chunks.
	chunkSize := 64
	data := make([]byte, chunkSize*3+17) // 3 full chunks + partial
	for i := range data {
		data[i] = byte(i % 256)
	}
	path := filepath.Join(dir, "test.bin")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	hashes, size, err := streamChunkHashes(path, chunkSize)
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(data)) {
		t.Fatalf("size = %d, want %d", size, len(data))
	}
	if len(hashes) != 4 {
		t.Fatalf("got %d hashes, want 4", len(hashes))
	}

	// Verify hashes are deterministic.
	hashes2, _, _ := streamChunkHashes(path, chunkSize)
	for i := range hashes {
		if hashes[i] != hashes2[i] {
			t.Fatalf("hash %d not deterministic", i)
		}
	}

	// Verify different content produces different hashes.
	data[0] = ^data[0]
	_ = os.WriteFile(path, data, 0o644)
	hashes3, _, _ := streamChunkHashes(path, chunkSize)
	if hashes3[0] == hashes[0] {
		t.Fatal("expected first chunk hash to change after content change")
	}
	// Last chunk should be unchanged (content change was in first chunk only).
	if hashes3[3] != hashes[3] {
		t.Fatal("expected last chunk hash to be unchanged")
	}
}

func TestStreamChunkHashesEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.bin")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	hashes, size, err := streamChunkHashes(path, 64)
	if err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		t.Fatalf("size = %d, want 0", size)
	}
	if len(hashes) != 0 {
		t.Fatalf("got %d hashes, want 0", len(hashes))
	}
}

func TestDiffChunkManifests(t *testing.T) {
	cases := []struct {
		name      string
		old       []string
		new       []string
		wantIdx   []int
		wantTrunc bool
	}{
		{"identical", []string{"a", "b"}, []string{"a", "b"}, nil, false},
		{"one changed", []string{"a", "b"}, []string{"a", "X"}, []int{1}, false},
		{"grown", []string{"a"}, []string{"a", "b"}, []int{1}, false},
		{"shrunk", []string{"a", "b", "c"}, []string{"a"}, nil, true},
		{"all new", nil, []string{"a", "b"}, []int{0, 1}, false},
		{"all removed", []string{"a", "b"}, nil, nil, true},
		{"changed and grown", []string{"a"}, []string{"X", "Y"}, []int{0, 1}, false},
		{"changed and shrunk", []string{"a", "b", "c"}, []string{"X"}, []int{0}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, trunc := diffChunkManifests(tc.old, tc.new)
			if trunc != tc.wantTrunc {
				t.Fatalf("truncated = %v, want %v", trunc, tc.wantTrunc)
			}
			if len(got) != len(tc.wantIdx) {
				t.Fatalf("changed = %v, want %v", got, tc.wantIdx)
			}
			for i := range got {
				if got[i] != tc.wantIdx[i] {
					t.Fatalf("changed[%d] = %d, want %d", i, got[i], tc.wantIdx[i])
				}
			}
		})
	}
}

func TestReadChunkFromDisk(t *testing.T) {
	dir := t.TempDir()
	chunkSize := 32
	data := make([]byte, chunkSize*2+10)
	for i := range data {
		data[i] = byte(i % 256)
	}
	path := filepath.Join(dir, "test.bin")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Read first full chunk.
	chunk, err := readChunkFromDisk(path, 0, chunkSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk) != chunkSize {
		t.Fatalf("chunk 0 len = %d, want %d", len(chunk), chunkSize)
	}

	// Read last partial chunk.
	chunk, err = readChunkFromDisk(path, 2, chunkSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk) != 10 {
		t.Fatalf("chunk 2 len = %d, want 10", len(chunk))
	}
}

func TestCompositeHash(t *testing.T) {
	h1 := compositeHash([]string{"aaa", "bbb"})
	h2 := compositeHash([]string{"aaa", "bbb"})
	h3 := compositeHash([]string{"aaa", "ccc"})
	if h1 != h2 {
		t.Fatal("same inputs should produce same hash")
	}
	if h1 == h3 {
		t.Fatal("different inputs should produce different hash")
	}
	// Empty produces a valid hash.
	h4 := compositeHash(nil)
	if h4 == "" {
		t.Fatal("empty input should produce a hash")
	}
}

func TestStreamAndReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	chunkSize := 64
	data := make([]byte, chunkSize*3+17)
	for i := range data {
		data[i] = byte((i*7 + 3) % 256)
	}
	path := filepath.Join(dir, "test.bin")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	hashes, _, err := streamChunkHashes(path, chunkSize)
	if err != nil {
		t.Fatal(err)
	}

	// Read all chunks back and verify hash matches.
	for i, expected := range hashes {
		chunk, err := readChunkFromDisk(path, i, chunkSize)
		if err != nil {
			t.Fatalf("read chunk %d: %v", i, err)
		}
		got := sha256Hex(chunk)
		if got != expected {
			t.Fatalf("chunk %d hash mismatch: got %s, want %s", i, got, expected)
		}
	}
}
