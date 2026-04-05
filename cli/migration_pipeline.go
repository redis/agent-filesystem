package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

const migrationMaxRetries = 3

const (
	migrationDirBatchMaxEntries  = 256
	migrationDirBatchMaxBytes    = 1 << 20
	migrationFileBatchMaxEntries = 64
	migrationFileBatchMaxBytes   = 4 << 20
)

type migrationCapacity struct {
	UsedMemory        int64
	MaxMemory         int64
	Available         int64
	EstimatedRequired int64
	Verifiable        bool
}

type migrationKeyBuilder struct {
	fsKey string
}

type migrationEntryKind string

const (
	migrationDirEntry     migrationEntryKind = "dir"
	migrationFileEntry    migrationEntryKind = "file"
	migrationSymlinkEntry migrationEntryKind = "symlink"
)

type migrationEntry struct {
	absPath   string
	redisPath string
	kind      migrationEntryKind
	size      int64
	mode      uint32
	uid       uint32
	gid       uint32
	ctimeMs   int64
	mtimeMs   int64
	atimeMs   int64
	target    string
}

func newMigrationKeyBuilder(fsKey string) migrationKeyBuilder {
	return migrationKeyBuilder{fsKey: fsKey}
}

func (k migrationKeyBuilder) inode(p string) string {
	return "rfs:{" + k.fsKey + "}:inode:" + p
}

func (k migrationKeyBuilder) children(p string) string {
	return "rfs:{" + k.fsKey + "}:children:" + p
}

func (k migrationKeyBuilder) info() string {
	return "rfs:{" + k.fsKey + "}:info"
}

func namespaceExists(ctx context.Context, rdb *redis.Client, fsKey string) (bool, error) {
	n, err := rdb.Exists(ctx, newMigrationKeyBuilder(fsKey).inode("/")).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func checkRedisCapacity(ctx context.Context, rdb *redis.Client, total importStats) (migrationCapacity, error) {
	info, err := rdb.Info(ctx, "memory").Result()
	if err != nil {
		return migrationCapacity{}, err
	}

	capacity := parseRedisMemoryInfo(info)
	capacity.EstimatedRequired = estimateMigrationRequiredBytes(total)
	if capacity.MaxMemory > 0 {
		capacity.Verifiable = true
		capacity.Available = capacity.MaxMemory - capacity.UsedMemory
	}
	return capacity, nil
}

func parseRedisMemoryInfo(info string) migrationCapacity {
	var capacity migrationCapacity

	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			continue
		}

		switch key {
		case "used_memory":
			capacity.UsedMemory = parsed
		case "maxmemory":
			capacity.MaxMemory = parsed
		}
	}

	return capacity
}

func estimateMigrationRequiredBytes(total importStats) int64 {
	const (
		fileOverhead    = int64(1024)
		dirOverhead     = int64(512)
		symlinkOverhead = int64(512)
		safetyNumerator = int64(5)
		safetyDenom     = int64(4)
	)

	required := total.Bytes +
		int64(total.Files)*fileOverhead +
		int64(total.Dirs)*dirOverhead +
		int64(total.Symlinks)*symlinkOverhead

	return (required * safetyNumerator) / safetyDenom
}

func createMigrationNamespace(ctx context.Context, rdb *redis.Client, fsKey string) error {
	keys := newMigrationKeyBuilder(fsKey)
	now := time.Now().UnixMilli()
	pipe := rdb.Pipeline()
	pipe.HSet(ctx, keys.inode("/"), map[string]interface{}{
		"type":     "dir",
		"mode":     0o755,
		"uid":      0,
		"gid":      0,
		"size":     0,
		"ctime_ms": now,
		"mtime_ms": now,
		"atime_ms": now,
	})
	pipe.HSet(ctx, keys.info(), map[string]interface{}{
		"schema_version":   "1",
		"files":            0,
		"directories":      1,
		"symlinks":         0,
		"total_data_bytes": 0,
	})
	_, err := pipe.Exec(ctx)
	return err
}

func incrementImportCounters(ctx context.Context, rdb *redis.Client, fsKey string, files, dirs, symlinks int, totalBytes int64) error {
	if files == 0 && dirs == 0 && symlinks == 0 && totalBytes == 0 {
		return nil
	}

	keys := newMigrationKeyBuilder(fsKey)
	pipe := rdb.Pipeline()
	if dirs > 0 {
		pipe.HIncrBy(ctx, keys.info(), "directories", int64(dirs))
	}
	if files > 0 {
		pipe.HIncrBy(ctx, keys.info(), "files", int64(files))
	}
	if symlinks > 0 {
		pipe.HIncrBy(ctx, keys.info(), "symlinks", int64(symlinks))
	}
	if totalBytes > 0 {
		pipe.HIncrBy(ctx, keys.info(), "total_data_bytes", totalBytes)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func importDirectoriesBatched(ctx context.Context, rdb *redis.Client, fsKey, source string, ignorer *migrationIgnore, onProgress func(importStats)) (int, time.Duration, error) {
	keys := newMigrationKeyBuilder(fsKey)
	started := time.Now()
	var importedDirs int
	batch := make([]migrationEntry, 0, migrationDirBatchMaxEntries)
	var batchBytes int64

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := writeMigrationBatchWithRetry(ctx, rdb, keys, batch); err != nil {
			return err
		}
		importedDirs += len(batch)
		if onProgress != nil {
			onProgress(importStats{Dirs: importedDirs})
		}
		batch = batch[:0]
		batchBytes = 0
		return nil
	}

	err := filepath.WalkDir(source, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}

		skip, err := ignorer.matches(source, path, d)
		if err != nil {
			return err
		}
		if skip {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}

		entry, err := migrationEntryFromPath(source, path, migrationDirEntry)
		if err != nil {
			return err
		}

		weight := migrationEntryWeight(entry)
		if len(batch) >= migrationDirBatchMaxEntries || (batchBytes > 0 && batchBytes+weight > migrationDirBatchMaxBytes) {
			if err := flush(); err != nil {
				return err
			}
		}
		batch = append(batch, entry)
		batchBytes += weight
		return nil
	})
	if err != nil {
		return importedDirs, time.Since(started), err
	}
	if err := flush(); err != nil {
		return importedDirs, time.Since(started), err
	}

	if err := incrementImportCounters(ctx, rdb, fsKey, 0, importedDirs, 0, 0); err != nil {
		return importedDirs, time.Since(started), err
	}
	return importedDirs, time.Since(started), nil
}

func importFilesBatched(ctx context.Context, rdb *redis.Client, fsKey, source string, ignorer *migrationIgnore, onProgress func(importStats)) (int, int, int64, time.Duration, error) {
	keys := newMigrationKeyBuilder(fsKey)
	started := time.Now()
	var importedFiles, importedLinks int
	var importedBytes int64
	batch := make([]migrationEntry, 0, migrationFileBatchMaxEntries)
	var batchBytes int64

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := writeMigrationBatchWithRetry(ctx, rdb, keys, batch); err != nil {
			return err
		}
		for _, entry := range batch {
			switch entry.kind {
			case migrationFileEntry:
				importedFiles++
				importedBytes += entry.size
			case migrationSymlinkEntry:
				importedLinks++
			}
		}
		if onProgress != nil {
			onProgress(importStats{
				Files:    importedFiles,
				Symlinks: importedLinks,
				Bytes:    importedBytes,
			})
		}
		batch = batch[:0]
		batchBytes = 0
		return nil
	}

	err := filepath.WalkDir(source, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}

		skip, err := ignorer.matches(source, path, d)
		if err != nil {
			return err
		}
		if skip {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}

		kind := migrationFileEntry
		if d.Type()&os.ModeSymlink != 0 {
			kind = migrationSymlinkEntry
		}

		entry, err := migrationEntryFromPath(source, path, kind)
		if err != nil {
			return err
		}

		weight := migrationEntryWeight(entry)
		if len(batch) >= migrationFileBatchMaxEntries || (batchBytes > 0 && batchBytes+weight > migrationFileBatchMaxBytes) {
			if err := flush(); err != nil {
				return err
			}
		}
		batch = append(batch, entry)
		batchBytes += weight
		return nil
	})
	if err != nil {
		return importedFiles, importedLinks, importedBytes, time.Since(started), err
	}
	if err := flush(); err != nil {
		return importedFiles, importedLinks, importedBytes, time.Since(started), err
	}

	if err := incrementImportCounters(ctx, rdb, fsKey, importedFiles, 0, importedLinks, importedBytes); err != nil {
		return importedFiles, importedLinks, importedBytes, time.Since(started), err
	}
	return importedFiles, importedLinks, importedBytes, time.Since(started), nil
}

func writeMigrationBatch(ctx context.Context, rdb *redis.Client, keys migrationKeyBuilder, entries []migrationEntry) error {
	pipe := rdb.Pipeline()

	for _, entry := range entries {
		fields := map[string]interface{}{
			"type":     string(entry.kind),
			"mode":     entry.mode,
			"uid":      entry.uid,
			"gid":      entry.gid,
			"size":     entry.size,
			"ctime_ms": entry.ctimeMs,
			"mtime_ms": entry.mtimeMs,
			"atime_ms": entry.atimeMs,
		}

		switch entry.kind {
		case migrationFileEntry:
			data, err := os.ReadFile(entry.absPath)
			if err != nil {
				return err
			}
			fields["content"] = string(data)
		case migrationSymlinkEntry:
			fields["target"] = entry.target
		}

		pipe.HSet(ctx, keys.inode(entry.redisPath), fields)
		pipe.SAdd(ctx, keys.children(parentOfRedisPath(entry.redisPath)), baseNameRedisPath(entry.redisPath))
	}

	_, err := pipe.Exec(ctx)
	return err
}

func writeMigrationBatchWithRetry(ctx context.Context, rdb *redis.Client, keys migrationKeyBuilder, entries []migrationEntry) error {
	backoff := time.Second
	for attempt := 1; attempt <= migrationMaxRetries; attempt++ {
		err := writeMigrationBatch(ctx, rdb, keys, entries)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil || isRedisOOM(err) || attempt == migrationMaxRetries {
			return err
		}
		time.Sleep(backoff)
		backoff *= 2
	}
	return nil
}

func migrationEntryFromPath(source, path string, kind migrationEntryKind) (migrationEntry, error) {
	rel, err := filepath.Rel(source, path)
	if err != nil {
		return migrationEntry{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return migrationEntry{}, err
	}

	entry := migrationEntry{
		absPath:   path,
		redisPath: "/" + filepath.ToSlash(rel),
		kind:      kind,
		mode:      uint32(info.Mode().Perm()),
		ctimeMs:   time.Now().UnixMilli(),
		mtimeMs:   info.ModTime().UnixMilli(),
		atimeMs:   info.ModTime().UnixMilli(),
	}

	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		entry.uid = st.Uid
		entry.gid = st.Gid
		aSec, aNsec := statAtime(st)
		mSec, mNsec := statMtime(st)
		entry.atimeMs = aSec*1000 + aNsec/1_000_000
		entry.mtimeMs = mSec*1000 + mNsec/1_000_000
	}

	switch kind {
	case migrationFileEntry:
		entry.size = info.Size()
	case migrationSymlinkEntry:
		target, err := os.Readlink(path)
		if err != nil {
			return migrationEntry{}, err
		}
		entry.target = target
		entry.size = int64(len(target))
	default:
		entry.size = 0
	}

	return entry, nil
}

func migrationEntryWeight(entry migrationEntry) int64 {
	switch entry.kind {
	case migrationFileEntry:
		if entry.size <= 0 {
			return 1
		}
		return entry.size
	case migrationSymlinkEntry:
		return int64(len(entry.target) + 256)
	default:
		return int64(len(entry.redisPath) + 256)
	}
}

func parentOfRedisPath(p string) string {
	if p == "/" {
		return "/"
	}
	parent := filepath.ToSlash(filepath.Dir(p))
	if parent == "." {
		return "/"
	}
	if !strings.HasPrefix(parent, "/") {
		parent = "/" + parent
	}
	return parent
}

func baseNameRedisPath(p string) string {
	if p == "/" {
		return ""
	}
	return filepath.Base(p)
}

func isRedisOOM(err error) bool {
	return err != nil && strings.Contains(err.Error(), "OOM")
}

func formatMigrationThroughput(bytes int64, elapsed time.Duration) string {
	if bytes <= 0 || elapsed <= 0 {
		return "0 B/s"
	}
	bytesPerSecond := int64(float64(bytes) / elapsed.Seconds())
	if bytesPerSecond <= 0 {
		bytesPerSecond = 1
	}
	return formatBytes(bytesPerSecond) + "/s"
}

func formatMigrationETA(completed, total int64, elapsed time.Duration) string {
	if completed <= 0 || total <= 0 || completed >= total || elapsed <= 0 {
		return "n/a"
	}
	remaining := total - completed
	eta := time.Duration(float64(elapsed) / float64(completed) * float64(remaining))
	return formatStepDuration(eta)
}

func formatStepDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	if d < time.Second {
		return d.Round(10 * time.Millisecond).String()
	}
	if d < time.Minute {
		return d.Round(100 * time.Millisecond).String()
	}
	return d.Round(time.Second).String()
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func formatDirImportLabel(progress, total importStats, elapsed time.Duration) string {
	label := fmt.Sprintf("Creating directories · %d/%d dirs", progress.Dirs, total.Dirs)
	if progress.Dirs > 0 {
		rate := float64(progress.Dirs) / elapsed.Seconds()
		label += fmt.Sprintf(" · %s elapsed · %.1f dirs/s · ETA %s", formatStepDuration(elapsed), rate, formatMigrationETA(int64(progress.Dirs), int64(total.Dirs), elapsed))
	}
	return label
}

func formatFileImportLabel(progress, total importStats, elapsed time.Duration) string {
	label := fmt.Sprintf("Importing files · %d/%d files", progress.Files, total.Files)
	if total.Symlinks > 0 {
		label += fmt.Sprintf(", %d/%d symlinks", progress.Symlinks, total.Symlinks)
	}
	if total.Bytes > 0 {
		pct := int((progress.Bytes * 100) / total.Bytes)
		label += fmt.Sprintf(" · %s / %s (%d%%)", formatBytes(progress.Bytes), formatBytes(total.Bytes), pct)
	}
	if elapsed > 0 {
		label += fmt.Sprintf(" · %s elapsed · %s", formatStepDuration(elapsed), formatMigrationThroughput(progress.Bytes, elapsed))
	}
	if total.Bytes > 0 && progress.Bytes > 0 {
		label += fmt.Sprintf(" · ETA %s", formatMigrationETA(progress.Bytes, total.Bytes, elapsed))
	}
	return label
}
