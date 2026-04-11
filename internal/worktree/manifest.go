package worktree

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

type walkItem struct {
	path         string
	manifestPath string
	info         os.FileInfo
}

type manifestBuildResult struct {
	manifestPath string
	entry        controlplane.ManifestEntry
	// blobID and blobData are populated for non-inline files when no sink is
	// attached to the build. They let the collector assemble the legacy
	// map[string][]byte return value.
	blobID   string
	blobData []byte
	byteSize int64
}

// BuildManifestFromDirectory walks root, hashing files concurrently, and
// returns the resulting manifest. When opts.Sink is set, non-inline blob
// bytes are handed to the sink and dropped from the builder's memory; the
// returned blob map is nil in that case.
func BuildManifestFromDirectory(root, workspace, savepoint string, opts BuildManifestOptions) (controlplane.Manifest, map[string][]byte, ManifestStats, error) {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return controlplane.Manifest{}, nil, ManifestStats{}, err
	}
	if !rootInfo.IsDir() {
		return controlplane.Manifest{}, nil, ManifestStats{}, fmt.Errorf("%s is not a directory", root)
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = resolveWorkerCount()
	}
	if workers < 1 {
		workers = 1
	}

	// Buffered channels so the walker isn't blocked on a slow worker and
	// workers aren't blocked on a slow collector.
	walkCh := make(chan walkItem, workers*4)
	resultCh := make(chan manifestBuildResult, workers*4)

	// Shared state
	var (
		wg             sync.WaitGroup
		errOnce        sync.Once
		firstErr       error
		stopCh         = make(chan struct{})
		stopped        = false
		stopMu         sync.Mutex
		progressMu     sync.Mutex
		progress       ImportStats
		symlinkEntries []manifestBuildResult
		dirEntries     []manifestBuildResult
		hasSink        = opts.Sink != nil
	)

	stop := func() {
		stopMu.Lock()
		defer stopMu.Unlock()
		if !stopped {
			stopped = true
			close(stopCh)
		}
	}
	reportErr := func(err error) {
		if err == nil {
			return
		}
		errOnce.Do(func() {
			firstErr = err
			stop()
		})
	}
	isStopped := func() bool {
		select {
		case <-stopCh:
			return true
		default:
			return false
		}
	}

	emitProgress := func(delta ImportStats) {
		progressMu.Lock()
		progress.Files += delta.Files
		progress.Dirs += delta.Dirs
		progress.Symlinks += delta.Symlinks
		progress.Ignored += delta.Ignored
		progress.Bytes += delta.Bytes
		snapshot := progress
		progressMu.Unlock()
		if opts.OnProgress != nil {
			opts.OnProgress(snapshot)
		}
	}

	// Walker goroutine: emits file walkItems to workers. Dir and symlink
	// manifest entries are produced inline (cheap, no file IO) and sent on
	// the result channel alongside hashed file entries.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(walkCh)

		walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if isStopped() {
				return filepath.SkipAll
			}

			if path != root && opts.Ignore != nil {
				skip, err := opts.Ignore(root, path, d)
				if err != nil {
					return err
				}
				if skip {
					emitProgress(ImportStats{Ignored: 1})
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}

			info, err := d.Info()
			if err != nil {
				return err
			}
			mp, err := manifestPath(root, path)
			if err != nil {
				return err
			}

			switch {
			case d.Type()&os.ModeSymlink != 0:
				target, err := os.Readlink(path)
				if err != nil {
					return err
				}
				entry := controlplane.ManifestEntry{
					Type:    "symlink",
					Mode:    uint32(info.Mode().Perm()),
					MtimeMs: info.ModTime().UTC().UnixMilli(),
					Size:    int64(len(target)),
					Target:  target,
				}
				symlinkEntries = append(symlinkEntries, manifestBuildResult{manifestPath: mp, entry: entry})
				emitProgress(ImportStats{Symlinks: 1})
			case d.IsDir():
				entry := controlplane.ManifestEntry{
					Type:    "dir",
					Mode:    uint32(info.Mode().Perm()),
					MtimeMs: info.ModTime().UTC().UnixMilli(),
				}
				dirEntries = append(dirEntries, manifestBuildResult{manifestPath: mp, entry: entry})
				if mp != "/" {
					emitProgress(ImportStats{Dirs: 1})
				}
			default:
				select {
				case walkCh <- walkItem{path: path, manifestPath: mp, info: info}:
				case <-stopCh:
					return filepath.SkipAll
				}
			}
			return nil
		})
		if walkErr != nil && walkErr != filepath.SkipAll {
			reportErr(walkErr)
		}
	}()

	// Worker pool: read + hash + emit manifest entries for each file.
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range walkCh {
				if isStopped() {
					continue
				}
				data, err := os.ReadFile(item.path)
				if err != nil {
					reportErr(fmt.Errorf("read %s: %w", item.path, err))
					continue
				}
				entry := controlplane.ManifestEntry{
					Type:    "file",
					Mode:    uint32(item.info.Mode().Perm()),
					MtimeMs: item.info.ModTime().UTC().UnixMilli(),
					Size:    int64(len(data)),
				}
				byteSize := int64(len(data))
				var blobID string
				var retained []byte
				if len(data) <= controlplane.InlineThreshold {
					entry.Inline = base64.StdEncoding.EncodeToString(data)
				} else {
					blobID = sha256Hex(data)
					entry.BlobID = blobID
					if hasSink {
						if err := opts.Sink.Submit(blobID, data, byteSize); err != nil {
							reportErr(fmt.Errorf("sink %s: %w", item.path, err))
							continue
						}
					} else {
						// No sink: retain the bytes so the collector can
						// populate the legacy return map. Share a single
						// reference; the collector dedupes on blobID.
						retained = data
					}
				}
				emitProgress(ImportStats{Files: 1, Bytes: byteSize})
				select {
				case resultCh <- manifestBuildResult{
					manifestPath: item.manifestPath,
					entry:        entry,
					blobID:       blobID,
					blobData:     retained,
					byteSize:     byteSize,
				}:
				case <-stopCh:
					return
				}
			}
		}()
	}

	// Closer goroutine: waits for walker + workers, then closes the result
	// channel so the collector exits.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collector (runs on the caller's goroutine).
	entries := make(map[string]controlplane.ManifestEntry, 256)
	var blobs map[string][]byte
	if !hasSink {
		blobs = make(map[string][]byte)
	}
	var stats ManifestStats

	for res := range resultCh {
		entries[res.manifestPath] = res.entry
		switch res.entry.Type {
		case "file":
			stats.FileCount++
			stats.TotalBytes += res.byteSize
			if !hasSink && res.blobID != "" {
				if _, ok := blobs[res.blobID]; !ok {
					blobs[res.blobID] = res.blobData
				}
			}
		}
	}

	// Merge dir/symlink entries emitted by the walker.
	for _, de := range dirEntries {
		entries[de.manifestPath] = de.entry
		if de.manifestPath != "/" {
			stats.DirCount++
		}
	}
	for _, se := range symlinkEntries {
		entries[se.manifestPath] = se.entry
	}

	if firstErr != nil {
		return controlplane.Manifest{}, nil, stats, firstErr
	}

	return controlplane.Manifest{
		Version:   controlplane.FormatVersion,
		Workspace: workspace,
		Savepoint: savepoint,
		Entries:   entries,
	}, blobs, stats, nil
}

func resolveWorkerCount() int {
	return DefaultImportWorkers()
}

// DefaultImportWorkers returns the number of hash workers that
// BuildManifestFromDirectory will launch when opts.Workers is zero. Honors
// the AFS_IMPORT_WORKERS environment variable, otherwise falls back to
// runtime.NumCPU().
func DefaultImportWorkers() int {
	if raw := os.Getenv("AFS_IMPORT_WORKERS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return runtime.NumCPU()
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
