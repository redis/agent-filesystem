package main

import (
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
)

type importScanEntry struct {
	Kind  string
	Path  string
	Bytes int64
	Files int
}

type importScanReport struct {
	Stats        importStats
	LargestFiles []importScanEntry
	LargestDirs  []importScanEntry
}

func scanDirectoryReport(source string, ignorer *migrationIgnore, limit int) (importScanReport, error) {
	var report importScanReport
	dirBytes := make(map[string]int64)
	dirFiles := make(map[string]int)

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
			report.Stats.Ignored++
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := os.Lstat(path)
		if err != nil {
			return err
		}

		switch {
		case d.Type()&os.ModeSymlink != 0:
			report.Stats.Symlinks++
		case d.IsDir():
			report.Stats.Dirs++
		default:
			rel, err := relativeImportPath(source, path)
			if err != nil {
				return err
			}

			size := info.Size()
			report.Stats.Files++
			report.Stats.Bytes += size

			if limit > 0 {
				report.LargestFiles = insertTopImportEntry(report.LargestFiles, importScanEntry{
					Kind:  "file",
					Path:  rel,
					Bytes: size,
					Files: 1,
				}, limit)
				for dir := pathpkg.Dir(rel); dir != "." && dir != "/"; dir = pathpkg.Dir(dir) {
					dirBytes[dir] += size
					dirFiles[dir]++
				}
			}
		}
		return nil
	})
	if err != nil {
		return report, err
	}

	if limit > 0 {
		for dir, size := range dirBytes {
			if size <= 0 {
				continue
			}
			report.LargestDirs = insertTopImportEntry(report.LargestDirs, importScanEntry{
				Kind:  "dir",
				Path:  dir,
				Bytes: size,
				Files: dirFiles[dir],
			}, limit)
		}
	}

	return report, nil
}

func insertTopImportEntry(entries []importScanEntry, candidate importScanEntry, limit int) []importScanEntry {
	if limit <= 0 {
		return entries
	}
	entries = append(entries, candidate)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Bytes == entries[j].Bytes {
			if entries[i].Files == entries[j].Files {
				return entries[i].Path < entries[j].Path
			}
			return entries[i].Files > entries[j].Files
		}
		return entries[i].Bytes > entries[j].Bytes
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries
}
