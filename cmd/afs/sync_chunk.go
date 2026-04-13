package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
)

const (
	// defaultChunkSize is the number of bytes per chunk for delta sync.
	// 256 KB balances granularity against overhead.
	defaultChunkSize = 256 * 1024

	// defaultChunkThreshold is the minimum file size to enable chunked sync.
	// Files below this use the existing full-file path (no overhead).
	defaultChunkThreshold = 1024 * 1024 // 1 MB

	// chunkPipelineBatch is how many chunks are uploaded/downloaded per
	// Redis pipeline call. Keeps memory bounded at chunkSize * batch.
	chunkPipelineBatch = 16
)

// streamChunkHashes reads a file chunk-by-chunk without loading it all into
// memory. Returns per-chunk SHA256 hex hashes and the total file size.
// Memory usage: O(chunkSize), not O(fileSize).
func streamChunkHashes(path string, chunkSize int) ([]string, int64, error) {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	buf := make([]byte, chunkSize)
	var hashes []string
	var totalSize int64

	for {
		n, err := io.ReadFull(f, buf)
		if n > 0 {
			totalSize += int64(n)
			h := sha256.Sum256(buf[:n])
			hashes = append(hashes, hex.EncodeToString(h[:]))
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return nil, 0, err
		}
	}
	return hashes, totalSize, nil
}

// diffChunkManifests compares old and new chunk hash lists index-by-index.
// Returns indices of chunks that differ (changed or added). If the new list
// is shorter than the old, truncated is true (caller should truncate the
// remote content after writing dirty chunks).
func diffChunkManifests(oldHashes, newHashes []string) (changed []int, truncated bool) {
	maxLen := len(newHashes)
	if len(oldHashes) > maxLen {
		maxLen = len(oldHashes)
	}
	for i := 0; i < len(newHashes); i++ {
		if i >= len(oldHashes) || oldHashes[i] != newHashes[i] {
			changed = append(changed, i)
		}
	}
	truncated = len(newHashes) < len(oldHashes)
	return changed, truncated
}

// readChunkFromDisk reads exactly one chunk from a file at the given index.
// Returns the data, which may be shorter than chunkSize for the last chunk.
func readChunkFromDisk(path string, chunkIndex int, chunkSize int) ([]byte, error) {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	offset := int64(chunkIndex) * int64(chunkSize)
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	buf := make([]byte, chunkSize)
	n, err := io.ReadFull(f, buf)
	if n > 0 {
		return buf[:n], nil
	}
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return buf[:n], nil
	}
	return nil, err
}

// compositeHash computes a single hash representing the entire file from its
// chunk hashes. Used as LocalHash/RemoteHash in SyncEntry for backward compat
// with the existing three-way conflict detection model.
func compositeHash(chunkHashes []string) string {
	if len(chunkHashes) == 0 {
		return sha256Hex(nil)
	}
	h := sha256.New()
	h.Write([]byte(strings.Join(chunkHashes, ",")))
	return hex.EncodeToString(h.Sum(nil))
}
