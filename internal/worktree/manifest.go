package worktree

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rowantrollope/agent-filesystem/internal/controlplane"
)

func BuildManifestFromDirectory(root, workspace, savepoint string, opts BuildManifestOptions) (controlplane.Manifest, map[string][]byte, ManifestStats, error) {
	entries := make(map[string]controlplane.ManifestEntry)
	blobs := make(map[string][]byte)
	stats := ManifestStats{}
	progress := ImportStats{}

	rootInfo, err := os.Lstat(root)
	if err != nil {
		return controlplane.Manifest{}, nil, stats, err
	}
	if !rootInfo.IsDir() {
		return controlplane.Manifest{}, nil, stats, fmt.Errorf("%s is not a directory", root)
	}

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if path != root && opts.Ignore != nil {
			skip, err := opts.Ignore(root, path, d)
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
		manifestPath, err := manifestPath(root, path)
		if err != nil {
			return err
		}

		entry := controlplane.ManifestEntry{
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
			if len(data) <= controlplane.InlineThreshold {
				entry.Inline = base64.StdEncoding.EncodeToString(data)
			} else {
				entry.BlobID = sha256Hex(data)
				if _, ok := blobs[entry.BlobID]; !ok {
					blobs[entry.BlobID] = data
				}
			}
		}

		entries[manifestPath] = entry
		if opts.OnProgress != nil {
			opts.OnProgress(progress)
		}
		return nil
	})
	if err != nil {
		return controlplane.Manifest{}, nil, stats, err
	}

	return controlplane.Manifest{
		Version:   controlplane.FormatVersion,
		Workspace: workspace,
		Savepoint: savepoint,
		Entries:   entries,
	}, blobs, stats, nil
}

func MaterializeManifestToDirectory(targetDir string, m controlplane.Manifest, loadBlob BlobLoader, opts MaterializeOptions) (ImportStats, error) {
	rootEntry := controlplane.ManifestEntry{Type: "dir", Mode: 0o755}
	if entry, ok := m.Entries["/"]; ok {
		rootEntry = entry
	}

	dirs, others := materializePaths(m)
	progress := ImportStats{}

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

	for _, manifestPath := range dirs {
		fullPath := MaterializedPath(targetDir, manifestPath)
		if err := os.MkdirAll(fullPath, fileModeOrDefault(m.Entries[manifestPath].Mode, 0o755)); err != nil {
			return progress, err
		}
		progress.Dirs++
		if opts.OnProgress != nil {
			opts.OnProgress(progress)
		}
	}

	for _, manifestPath := range others {
		entry := m.Entries[manifestPath]
		fullPath := MaterializedPath(targetDir, manifestPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return progress, err
		}
		switch entry.Type {
		case "file":
			data, err := controlplane.ManifestEntryData(entry, fetchBlob)
			if err != nil {
				return progress, err
			}
			mode := fileModeOrDefault(entry.Mode, 0o644)
			if err := os.WriteFile(fullPath, data, mode); err != nil {
				return progress, err
			}
			if opts.PreserveMetadata {
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
		if opts.OnProgress != nil {
			opts.OnProgress(progress)
		}
	}

	if opts.PreserveMetadata {
		if err := applyMaterializedManifestMetadata(targetDir, rootEntry, m, dirs); err != nil {
			return progress, err
		}
	} else if err := os.Chmod(targetDir, fileModeOrDefault(rootEntry.Mode, 0o755)); err != nil {
		return progress, err
	}

	return progress, nil
}

func materializePaths(m controlplane.Manifest) ([]string, []string) {
	paths := manifestPaths(m)
	dirs := make([]string, 0, len(paths))
	others := make([]string, 0, len(paths))
	for _, manifestPath := range paths {
		if manifestPath == "/" {
			continue
		}
		if m.Entries[manifestPath].Type == "dir" {
			dirs = append(dirs, manifestPath)
			continue
		}
		others = append(others, manifestPath)
	}

	sort.Slice(dirs, func(i, j int) bool {
		if len(dirs[i]) == len(dirs[j]) {
			return dirs[i] < dirs[j]
		}
		return len(dirs[i]) < len(dirs[j])
	})
	return dirs, others
}

func applyMaterializedManifestMetadata(targetDir string, rootEntry controlplane.ManifestEntry, m controlplane.Manifest, dirs []string) error {
	sort.Slice(dirs, func(i, j int) bool {
		if len(dirs[i]) == len(dirs[j]) {
			return dirs[i] > dirs[j]
		}
		return len(dirs[i]) > len(dirs[j])
	})
	for _, manifestPath := range dirs {
		entry := m.Entries[manifestPath]
		fullPath := MaterializedPath(targetDir, manifestPath)
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

func manifestPaths(m controlplane.Manifest) []string {
	paths := make([]string, 0, len(m.Entries))
	for manifestPath := range m.Entries {
		paths = append(paths, manifestPath)
	}
	sort.Strings(paths)
	return paths
}

func manifestPath(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "/", nil
	}
	return "/" + filepath.ToSlash(rel), nil
}

func fileModeOrDefault(mode uint32, fallback os.FileMode) os.FileMode {
	if mode == 0 {
		return fallback
	}
	return os.FileMode(mode)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
