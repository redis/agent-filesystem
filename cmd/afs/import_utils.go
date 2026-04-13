package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func importDirectory(ctx context.Context, fsClient importClient, source string, ignorer *migrationIgnore, onProgress func(importStats)) (int, int, int, int64, int, error) {
	var files, dirs, symlinks, ignored int
	var importedBytes int64
	err := filepath.WalkDir(source, func(path string, d os.DirEntry, walkErr error) error {
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
			ignored++
			if onProgress != nil {
				onProgress(importStats{
					Files:    files,
					Dirs:     dirs,
					Symlinks: symlinks,
					Ignored:  ignored,
					Bytes:    importedBytes,
				})
			}
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		rel, relErr := filepath.Rel(source, path)
		if relErr != nil {
			return relErr
		}
		redisPath := "/" + filepath.ToSlash(rel)

		info, err := os.Lstat(path)
		if err != nil {
			return err
		}

		switch {
		case d.Type()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := fsClient.Ln(ctx, target, redisPath); err != nil {
				return fmt.Errorf("ln %s: %w", redisPath, err)
			}
			symlinks++
		case d.IsDir():
			if err := fsClient.Mkdir(ctx, redisPath); err != nil {
				return fmt.Errorf("mkdir %s: %w", redisPath, err)
			}
			dirs++
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if err := fsClient.Echo(ctx, redisPath, data); err != nil {
				return fmt.Errorf("echo %s: %w", redisPath, err)
			}
			files++
			importedBytes += int64(len(data))
		}

		if err := applyMetadata(ctx, fsClient, redisPath, info); err != nil {
			return err
		}
		if onProgress != nil {
			onProgress(importStats{
				Files:    files,
				Dirs:     dirs,
				Symlinks: symlinks,
				Ignored:  ignored,
				Bytes:    importedBytes,
			})
		}
		return nil
	})
	return files, dirs, symlinks, importedBytes, ignored, err
}

func scanDirectory(source string, ignorer *migrationIgnore) (importStats, error) {
	var stats importStats
	err := filepath.WalkDir(source, func(path string, d os.DirEntry, walkErr error) error {
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
			stats.Ignored++
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		switch {
		case d.Type()&os.ModeSymlink != 0:
			stats.Symlinks++
		case d.IsDir():
			stats.Dirs++
		default:
			info, err := d.Info()
			if err != nil {
				return err
			}
			stats.Files++
			stats.Bytes += info.Size()
		}
		return nil
	})
	return stats, err
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	value := float64(size) / unit
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	unitIndex := 0
	for value >= unit && unitIndex < len(units)-1 {
		value /= unit
		unitIndex++
	}
	return fmt.Sprintf("%.1f %s", value, units[unitIndex])
}

func applyMetadata(ctx context.Context, fsClient importClient, path string, info os.FileInfo) error {
	if err := fsClient.Chmod(ctx, path, uint32(info.Mode().Perm())); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		if err := fsClient.Chown(ctx, path, st.Uid, st.Gid); err != nil {
			return fmt.Errorf("chown %s: %w", path, err)
		}
		aSec, aNsec := statAtime(st)
		mSec, mNsec := statMtime(st)
		atimeMs := aSec*1000 + aNsec/1_000_000
		mtimeMs := mSec*1000 + mNsec/1_000_000
		if err := fsClient.Utimens(ctx, path, atimeMs, mtimeMs); err != nil {
			return fmt.Errorf("utimens %s: %w", path, err)
		}
	}
	return nil
}
