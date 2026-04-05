package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type manifestBlobLoader func(blobID string) ([]byte, error)

type manifestMaterializeOptions struct {
	onProgress       func(importStats)
	preserveMetadata bool
}

func materializeManifestToDirectory(targetDir string, m manifest, loadBlob manifestBlobLoader, opts manifestMaterializeOptions) (importStats, error) {
	rootEntry := manifestEntry{Type: "dir", Mode: 0o755}
	if entry, ok := m.Entries["/"]; ok {
		rootEntry = entry
	}

	dirs, others := manifestMaterializePaths(m)
	progress := importStats{}

	if err := os.RemoveAll(targetDir); err != nil {
		return progress, err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return progress, err
	}

	fetchBlob := loadBlob
	if fetchBlob == nil {
		fetchBlob = func(blobID string) ([]byte, error) {
			return nil, fmt.Errorf("manifest blob %q cannot be loaded", blobID)
		}
	}

	for _, path := range dirs {
		fullPath := rafMaterializedPath(targetDir, path)
		if err := os.MkdirAll(fullPath, fileModeOrDefault(m.Entries[path].Mode, 0o755)); err != nil {
			return progress, err
		}
		progress.Dirs++
		if opts.onProgress != nil {
			opts.onProgress(progress)
		}
	}

	for _, path := range others {
		entry := m.Entries[path]
		fullPath := rafMaterializedPath(targetDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return progress, err
		}
		switch entry.Type {
		case "file":
			data, err := manifestEntryData(entry, fetchBlob)
			if err != nil {
				return progress, err
			}
			mode := fileModeOrDefault(entry.Mode, 0o644)
			if err := os.WriteFile(fullPath, data, mode); err != nil {
				return progress, err
			}
			if opts.preserveMetadata {
				if err := os.Chmod(fullPath, mode); err != nil {
					return progress, err
				}
				mtime := time.UnixMilli(entry.MtimeMs)
				if err := os.Chtimes(fullPath, mtime, mtime); err != nil {
					return progress, err
				}
			}
			progress.Files++
			progress.Bytes += int64(len(data))
		case "symlink":
			if err := os.Symlink(entry.Target, fullPath); err != nil {
				return progress, err
			}
			progress.Symlinks++
		default:
			return progress, fmt.Errorf("unsupported manifest entry type %q", entry.Type)
		}
		if opts.onProgress != nil {
			opts.onProgress(progress)
		}
	}

	if opts.preserveMetadata {
		if err := applyMaterializedManifestMetadata(targetDir, rootEntry, m, dirs); err != nil {
			return progress, err
		}
	} else if err := os.Chmod(targetDir, fileModeOrDefault(rootEntry.Mode, 0o755)); err != nil {
		return progress, err
	}

	return progress, nil
}

func manifestMaterializePaths(m manifest) ([]string, []string) {
	paths := manifestPaths(m)
	dirs := make([]string, 0, len(paths))
	others := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "/" {
			continue
		}
		if m.Entries[path].Type == "dir" {
			dirs = append(dirs, path)
			continue
		}
		others = append(others, path)
	}

	sort.Slice(dirs, func(i, j int) bool {
		if len(dirs[i]) == len(dirs[j]) {
			return dirs[i] < dirs[j]
		}
		return len(dirs[i]) < len(dirs[j])
	})
	return dirs, others
}

func applyMaterializedManifestMetadata(targetDir string, rootEntry manifestEntry, m manifest, dirs []string) error {
	sort.Slice(dirs, func(i, j int) bool {
		if len(dirs[i]) == len(dirs[j]) {
			return dirs[i] > dirs[j]
		}
		return len(dirs[i]) > len(dirs[j])
	})
	for _, path := range dirs {
		entry := m.Entries[path]
		fullPath := rafMaterializedPath(targetDir, path)
		if err := os.Chmod(fullPath, fileModeOrDefault(entry.Mode, 0o755)); err != nil {
			return err
		}
		mtime := time.UnixMilli(entry.MtimeMs)
		if err := os.Chtimes(fullPath, mtime, mtime); err != nil {
			return err
		}
	}

	if err := os.Chmod(targetDir, fileModeOrDefault(rootEntry.Mode, 0o755)); err != nil {
		return err
	}
	rootTime := time.UnixMilli(rootEntry.MtimeMs)
	return os.Chtimes(targetDir, rootTime, rootTime)
}
