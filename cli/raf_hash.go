package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type canonicalManifest struct {
	Version   int                      `json:"version"`
	Workspace string                   `json:"workspace"`
	Savepoint string                   `json:"savepoint"`
	Entries   []canonicalManifestEntry `json:"entries"`
}

type canonicalManifestEntry struct {
	Path string `json:"path"`
	manifestEntry
}

func buildManifestFromDirectory(root, workspace, savepoint string) (manifest, map[string][]byte, manifestStats, error) {
	return buildManifestFromDirectoryWithOptions(root, workspace, savepoint, nil, nil)
}

func buildManifestFromDirectoryWithProgress(root, workspace, savepoint string, onProgress func(importStats)) (manifest, map[string][]byte, manifestStats, error) {
	return buildManifestFromDirectoryWithOptions(root, workspace, savepoint, nil, onProgress)
}

func buildManifestFromDirectoryWithOptions(root, workspace, savepoint string, ignorer *migrationIgnore, onProgress func(importStats)) (manifest, map[string][]byte, manifestStats, error) {
	entries := make(map[string]manifestEntry)
	blobs := make(map[string][]byte)
	stats := manifestStats{}
	progress := importStats{}

	rootInfo, err := os.Lstat(root)
	if err != nil {
		return manifest{}, nil, stats, err
	}
	if !rootInfo.IsDir() {
		return manifest{}, nil, stats, fmt.Errorf("%s is not a directory", root)
	}

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if path != root {
			skip, err := ignorer.matches(root, path, d)
			if err != nil {
				return err
			}
			if skip {
				progress.Ignored++
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		manifestPath, err := rafManifestPath(root, path)
		if err != nil {
			return err
		}

		entry := manifestEntry{
			Mode:    uint32(info.Mode().Perm()),
			MtimeMs: info.ModTime().UTC().UnixMilli(),
			Size:    info.Size(),
		}

		switch {
		case d.Type()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entry.Type = "symlink"
			entry.Target = target
			entry.Size = int64(len(target))
			progress.Symlinks++
		case d.IsDir():
			entry.Type = "dir"
			entry.Size = 0
			if manifestPath != "/" {
				stats.DirCount++
				progress.Dirs++
			}
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entry.Type = "file"
			entry.Size = int64(len(data))
			stats.FileCount++
			stats.TotalBytes += int64(len(data))
			progress.Files++
			progress.Bytes += int64(len(data))
			if len(data) <= rafInlineThreshold {
				entry.Inline = base64.StdEncoding.EncodeToString(data)
			} else {
				entry.BlobID = sha256Hex(data)
				if _, ok := blobs[entry.BlobID]; !ok {
					blobs[entry.BlobID] = data
				}
			}
		}

		entries[manifestPath] = entry
		if onProgress != nil {
			onProgress(progress)
		}
		return nil
	})
	if err != nil {
		return manifest{}, nil, stats, err
	}

	return manifest{
		Version:   rafFormatVersion,
		Workspace: workspace,
		Savepoint: savepoint,
		Entries:   entries,
	}, blobs, stats, nil
}

func canonicalManifestBytes(m manifest) ([]byte, error) {
	paths := manifestPaths(m)
	entries := make([]canonicalManifestEntry, 0, len(paths))
	for _, path := range paths {
		entries = append(entries, canonicalManifestEntry{
			Path:          path,
			manifestEntry: m.Entries[path],
		})
	}
	return json.Marshal(canonicalManifest{
		Version:   m.Version,
		Workspace: m.Workspace,
		Savepoint: m.Savepoint,
		Entries:   entries,
	})
}

func hashManifest(m manifest) (string, error) {
	b, err := canonicalManifestBytes(m)
	if err != nil {
		return "", err
	}
	return sha256Hex(b), nil
}

func manifestPaths(m manifest) []string {
	paths := make([]string, 0, len(m.Entries))
	for path := range m.Entries {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func manifestEquivalent(a, b manifest) bool {
	if len(a.Entries) != len(b.Entries) {
		return false
	}
	for path, left := range a.Entries {
		right, ok := b.Entries[path]
		if !ok {
			return false
		}
		if !manifestEntryEquivalent(left, right) {
			return false
		}
	}
	return true
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
	refs := map[string]int64{}
	for _, entry := range m.Entries {
		if entry.BlobID == "" {
			continue
		}
		refs[entry.BlobID] = entry.Size
	}
	return refs
}

func manifestImportStats(m manifest) importStats {
	var stats importStats
	for path, entry := range m.Entries {
		if path == "/" {
			continue
		}
		switch entry.Type {
		case "file":
			stats.Files++
			stats.Bytes += entry.Size
		case "dir":
			stats.Dirs++
		case "symlink":
			stats.Symlinks++
		}
	}
	return stats
}

func manifestEntryData(entry manifestEntry, fetchBlob func(blobID string) ([]byte, error)) ([]byte, error) {
	switch {
	case entry.Inline != "":
		return base64.StdEncoding.DecodeString(entry.Inline)
	case entry.BlobID != "":
		return fetchBlob(entry.BlobID)
	case entry.Type == "file" && entry.Size == 0:
		return []byte{}, nil
	default:
		return nil, fmt.Errorf("manifest entry does not contain file data")
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func rafManifestPath(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "/", nil
	}
	return "/" + filepath.ToSlash(rel), nil
}
