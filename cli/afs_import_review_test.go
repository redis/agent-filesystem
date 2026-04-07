package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDirectoryReportTracksLargestFilesAndDirs(t *testing.T) {
	sourceDir := t.TempDir()

	writeTestFile(t, filepath.Join(sourceDir, "root.txt"), "12345")
	writeTestFile(t, filepath.Join(sourceDir, "cache", "blob.bin"), "1234567890")
	writeTestFile(t, filepath.Join(sourceDir, "vendor", "pkg.tar"), "1234567")
	writeTestFile(t, filepath.Join(sourceDir, "vendor", "README.md"), "12")
	writeTestFile(t, filepath.Join(sourceDir, "docs", "guide.md"), "1234")

	report, err := scanDirectoryReport(sourceDir, nil, 3)
	if err != nil {
		t.Fatalf("scanDirectoryReport() returned error: %v", err)
	}

	if report.Stats.Files != 5 {
		t.Fatalf("Files = %d, want 5", report.Stats.Files)
	}
	if report.Stats.Dirs != 3 {
		t.Fatalf("Dirs = %d, want 3", report.Stats.Dirs)
	}
	if report.Stats.Bytes != 28 {
		t.Fatalf("Bytes = %d, want 28", report.Stats.Bytes)
	}

	if len(report.LargestFiles) != 3 {
		t.Fatalf("len(LargestFiles) = %d, want 3", len(report.LargestFiles))
	}
	if report.LargestFiles[0].Path != "cache/blob.bin" || report.LargestFiles[0].Bytes != 10 {
		t.Fatalf("LargestFiles[0] = %+v, want cache/blob.bin (10 bytes)", report.LargestFiles[0])
	}
	if report.LargestFiles[1].Path != "vendor/pkg.tar" || report.LargestFiles[1].Bytes != 7 {
		t.Fatalf("LargestFiles[1] = %+v, want vendor/pkg.tar (7 bytes)", report.LargestFiles[1])
	}
	if report.LargestFiles[2].Path != "root.txt" || report.LargestFiles[2].Bytes != 5 {
		t.Fatalf("LargestFiles[2] = %+v, want root.txt (5 bytes)", report.LargestFiles[2])
	}

	if len(report.LargestDirs) != 3 {
		t.Fatalf("len(LargestDirs) = %d, want 3", len(report.LargestDirs))
	}
	if report.LargestDirs[0].Path != "cache" || report.LargestDirs[0].Bytes != 10 || report.LargestDirs[0].Files != 1 {
		t.Fatalf("LargestDirs[0] = %+v, want cache (10 bytes, 1 file)", report.LargestDirs[0])
	}
	if report.LargestDirs[1].Path != "vendor" || report.LargestDirs[1].Bytes != 9 || report.LargestDirs[1].Files != 2 {
		t.Fatalf("LargestDirs[1] = %+v, want vendor (9 bytes, 2 files)", report.LargestDirs[1])
	}
	if report.LargestDirs[2].Path != "docs" || report.LargestDirs[2].Bytes != 4 || report.LargestDirs[2].Files != 1 {
		t.Fatalf("LargestDirs[2] = %+v, want docs (4 bytes, 1 file)", report.LargestDirs[2])
	}
}

func TestApplyAFSImportExclusionsOverridesExistingIgnoreBehavior(t *testing.T) {
	sourceDir := t.TempDir()

	writeTestFile(t, filepath.Join(sourceDir, ".afsignore"), "*.log\n!logs/keep.log\n")
	writeTestFile(t, filepath.Join(sourceDir, "logs", "keep.log"), "keep")
	writeTestFile(t, filepath.Join(sourceDir, "build", "artifact.bin"), "artifact")

	baseIgnorer, err := loadMigrationIgnore(sourceDir)
	if err != nil {
		t.Fatalf("loadMigrationIgnore() returned error: %v", err)
	}

	exclusions := newAFSImportExclusions()
	if !exclusions.add(importScanEntry{Kind: "file", Path: "logs/keep.log"}) {
		t.Fatal("expected keep.log to be added as a temporary exclusion")
	}
	if !exclusions.add(importScanEntry{Kind: "dir", Path: "build"}) {
		t.Fatal("expected build/ to be added as a temporary exclusion")
	}

	ignorer := applyAFSImportExclusions(baseIgnorer, exclusions)

	keepEntry := dirEntryForTest(t, filepath.Join(sourceDir, "logs"), "keep.log")
	keepPath := filepath.Join(sourceDir, "logs", "keep.log")
	matches, err := ignorer.matches(sourceDir, keepPath, keepEntry)
	if err != nil {
		t.Fatalf("ignorer.matches(keep.log) returned error: %v", err)
	}
	if !matches {
		t.Fatal("expected temporary file exclusion to override the allowlist in .afsignore")
	}

	buildDirEntry := dirEntryForTest(t, sourceDir, "build")
	buildPath := filepath.Join(sourceDir, "build")
	matches, err = ignorer.matches(sourceDir, buildPath, buildDirEntry)
	if err != nil {
		t.Fatalf("ignorer.matches(build/) returned error: %v", err)
	}
	if !matches {
		t.Fatal("expected build/ directory to be excluded")
	}

	artifactEntry := dirEntryForTest(t, filepath.Join(sourceDir, "build"), "artifact.bin")
	artifactPath := filepath.Join(sourceDir, "build", "artifact.bin")
	matches, err = ignorer.matches(sourceDir, artifactPath, artifactEntry)
	if err != nil {
		t.Fatalf("ignorer.matches(build/artifact.bin) returned error: %v", err)
	}
	if !matches {
		t.Fatal("expected children of a temporary directory exclusion to be excluded")
	}
}

func dirEntryForTest(t *testing.T, dir, base string) os.DirEntry {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s) returned error: %v", dir, err)
	}
	for _, entry := range entries {
		if entry.Name() == base {
			return entry
		}
	}
	t.Fatalf("entry %s not found under %s", base, dir)
	return nil
}
