package main

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/internal/worktree"
)

func buildManifestFromDirectory(root, workspace, savepoint string) (manifest, map[string][]byte, manifestStats, error) {
	return buildManifestFromDirectoryWithOptions(root, workspace, savepoint, nil, nil)
}

func buildManifestFromDirectoryWithProgress(root, workspace, savepoint string, onProgress func(importStats)) (manifest, map[string][]byte, manifestStats, error) {
	return buildManifestFromDirectoryWithOptions(root, workspace, savepoint, nil, onProgress)
}

func buildManifestFromDirectoryWithOptions(root, workspace, savepoint string, ignorer *migrationIgnore, onProgress func(importStats)) (manifest, map[string][]byte, manifestStats, error) {
	opts := worktree.BuildManifestOptions{}
	if ignorer != nil {
		opts.Ignore = ignorer.matches
	}
	if onProgress != nil {
		opts.OnProgress = onProgress
	}
	return worktree.BuildManifestFromDirectory(root, workspace, savepoint, opts)
}

// buildManifestStreaming is the fast path used by `afs ws import`. It
// hands non-inline blobs to the supplied sink so the caller can pipeline them
// to Redis and drop the in-memory buffers incrementally.
func buildManifestStreaming(root, workspace, savepoint string, ignorer *migrationIgnore, sink worktree.BlobSink, onProgress func(importStats)) (manifest, manifestStats, error) {
	opts := worktree.BuildManifestOptions{Sink: sink}
	if ignorer != nil {
		opts.Ignore = ignorer.matches
	}
	if onProgress != nil {
		opts.OnProgress = onProgress
	}
	m, _, stats, err := worktree.BuildManifestFromDirectory(root, workspace, savepoint, opts)
	return m, stats, err
}

func hashManifest(m manifest) (string, error) {
	return controlplane.HashManifest(m)
}

func manifestEquivalent(a, b manifest) bool {
	return controlplane.ManifestEquivalent(a, b)
}

func manifestEntryEquivalent(a, b manifestEntry) bool {
	if a.Type != b.Type || a.Mode != b.Mode || a.Size != b.Size || a.BlobID != b.BlobID || a.Inline != b.Inline || a.Target != b.Target {
		return false
	}
	if a.Type == "symlink" || a.Type == "dir" {
		return true
	}
	return a.MtimeMs == b.MtimeMs
}

func manifestBlobRefs(m manifest) map[string]int64 {
	return controlplane.ManifestBlobRefs(m)
}

func manifestEntryData(entry manifestEntry, fetchBlob func(blobID string) ([]byte, error)) ([]byte, error) {
	return controlplane.ManifestEntryData(entry, fetchBlob)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
