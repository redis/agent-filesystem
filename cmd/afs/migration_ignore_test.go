package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type fakeImportClient struct {
	dirs     map[string]bool
	files    map[string]string
	links    map[string]string
	metadata map[string]bool
}

func newFakeImportClient() *fakeImportClient {
	return &fakeImportClient{
		dirs:     make(map[string]bool),
		files:    make(map[string]string),
		links:    make(map[string]string),
		metadata: make(map[string]bool),
	}
}

func (f *fakeImportClient) Mkdir(_ context.Context, path string) error {
	f.dirs[path] = true
	return nil
}

func (f *fakeImportClient) Echo(_ context.Context, path string, data []byte) error {
	f.files[path] = string(data)
	return nil
}

func (f *fakeImportClient) Ln(_ context.Context, target, linkpath string) error {
	f.links[linkpath] = target
	return nil
}

func (f *fakeImportClient) Chmod(_ context.Context, path string, _ uint32) error {
	f.metadata[path] = true
	return nil
}

func (f *fakeImportClient) Chown(_ context.Context, path string, _, _ uint32) error {
	f.metadata[path] = true
	return nil
}

func (f *fakeImportClient) Utimens(_ context.Context, path string, _, _ int64) error {
	f.metadata[path] = true
	return nil
}

func TestImportDirectoryRespectsAFSIgnore(t *testing.T) {
	sourceDir := t.TempDir()

	writeTestFile(t, filepath.Join(sourceDir, ".afsignore"), "cache/\nworktrees/\n*.log\n!logs/keep.log\n")
	writeTestFile(t, filepath.Join(sourceDir, "keep.txt"), "keep")
	writeTestFile(t, filepath.Join(sourceDir, "cache", "state.json"), "{}")
	writeTestFile(t, filepath.Join(sourceDir, "logs", "debug.log"), "ignore me")
	writeTestFile(t, filepath.Join(sourceDir, "logs", "keep.log"), "keep me")
	writeTestFile(t, filepath.Join(sourceDir, "worktrees", "session", "config.json"), "{}")

	ignorer, err := loadMigrationIgnore(sourceDir)
	if err != nil {
		t.Fatalf("loadMigrationIgnore returned error: %v", err)
	}
	if ignorer == nil {
		t.Fatal("expected ignorer to be loaded")
	}

	client := newFakeImportClient()
	files, dirs, symlinks, ignoredBytes, ignored, err := importDirectory(context.Background(), client, sourceDir, ignorer, nil)
	if err != nil {
		t.Fatalf("importDirectory returned error: %v", err)
	}

	if files != 3 {
		t.Fatalf("expected 3 imported files, got %d", files)
	}
	if dirs != 1 {
		t.Fatalf("expected 1 imported directory, got %d", dirs)
	}
	if symlinks != 0 {
		t.Fatalf("expected 0 imported symlinks, got %d", symlinks)
	}
	if ignored != 3 {
		t.Fatalf("expected 3 ignored paths, got %d", ignored)
	}
	if ignoredBytes <= 0 {
		t.Fatalf("expected imported byte count to be positive")
	}
	if client.files["/keep.txt"] != "keep" {
		t.Fatalf("expected keep.txt to be imported")
	}
	if client.files["/logs/keep.log"] != "keep me" {
		t.Fatalf("expected logs/keep.log to be re-included")
	}
	if client.files["/.afsignore"] == "" {
		t.Fatalf("expected .afsignore to be imported")
	}
	if _, ok := client.files["/logs/debug.log"]; ok {
		t.Fatalf("expected logs/debug.log to be ignored")
	}
	if _, ok := client.files["/cache/state.json"]; ok {
		t.Fatalf("expected cache/state.json to be ignored")
	}
	if _, ok := client.files["/worktrees/session/config.json"]; ok {
		t.Fatalf("expected worktrees/session/config.json to be ignored")
	}
	if !client.metadata["/keep.txt"] || !client.metadata["/logs/keep.log"] {
		t.Fatalf("expected metadata to be applied to imported files")
	}
}

func TestImportDirectoryImportsIncludedPathsAndSkipsIgnoredOnes(t *testing.T) {
	sourceDir := t.TempDir()

	writeTestFile(t, filepath.Join(sourceDir, ".afsignore"), "cache/\n*.tmp\n!important.tmp\n")
	writeTestFile(t, filepath.Join(sourceDir, "important.tmp"), "keep")
	writeTestFile(t, filepath.Join(sourceDir, "throwaway.tmp"), "skip")
	writeTestFile(t, filepath.Join(sourceDir, "cache", "blob.txt"), "skip")
	writeTestFile(t, filepath.Join(sourceDir, "notes.md"), "keep")

	ignorer, err := loadMigrationIgnore(sourceDir)
	if err != nil {
		t.Fatalf("loadMigrationIgnore returned error: %v", err)
	}

	client := newFakeImportClient()
	_, _, _, _, _, err = importDirectory(context.Background(), client, sourceDir, ignorer, nil)
	if err != nil {
		t.Fatalf("importDirectory returned error: %v", err)
	}

	if client.files["/important.tmp"] != "keep" {
		t.Fatalf("expected important.tmp to be imported")
	}
	if client.files["/notes.md"] != "keep" {
		t.Fatalf("expected notes.md to be imported")
	}
	if client.files["/.afsignore"] == "" {
		t.Fatalf("expected .afsignore to be imported")
	}
	if _, ok := client.files["/throwaway.tmp"]; ok {
		t.Fatalf("expected throwaway.tmp to be ignored")
	}
	if _, ok := client.files["/cache/blob.txt"]; ok {
		t.Fatalf("expected cache/blob.txt to be ignored")
	}
	if _, ok := client.dirs["/cache"]; ok {
		t.Fatalf("expected /cache directory to be ignored")
	}
}

func TestLoadMigrationIgnoreFallsBackToLegacyOriginalIgnore(t *testing.T) {
	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, ".rfsignore"), "cache/\n")

	ignorer, err := loadMigrationIgnore(sourceDir)
	if err != nil {
		t.Fatalf("loadMigrationIgnore returned error: %v", err)
	}
	if ignorer == nil {
		t.Fatal("expected ignorer to be loaded")
	}
	if !ignorer.legacy {
		t.Fatal("expected legacy ignore file to be marked as legacy")
	}
	if filepath.Base(ignorer.path) != ".rfsignore" {
		t.Fatalf("ignore path = %q, want .rfsignore", ignorer.path)
	}
}

func TestLoadMigrationIgnorePrefersAFSIgnore(t *testing.T) {
	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, ".afsignore"), "cache/\n")
	writeTestFile(t, filepath.Join(sourceDir, ".rfsignore"), "logs/\n")

	ignorer, err := loadMigrationIgnore(sourceDir)
	if err != nil {
		t.Fatalf("loadMigrationIgnore returned error: %v", err)
	}
	if ignorer == nil {
		t.Fatal("expected ignorer to be loaded")
	}
	if ignorer.legacy {
		t.Fatal("expected .afsignore to take precedence over legacy file")
	}
	if filepath.Base(ignorer.path) != ".afsignore" {
		t.Fatalf("ignore path = %q, want .afsignore", ignorer.path)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
