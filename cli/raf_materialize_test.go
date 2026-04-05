package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestMaterializeManifestToPathWritesManifestEntries(t *testing.T) {
	t.Helper()

	store, closeStore := newRAFStoreForTest(t)
	defer closeStore()

	sourceDir := t.TempDir()
	writeTestFile(t, filepath.Join(sourceDir, "README.md"), "hello raf\n")
	writeTestFile(t, filepath.Join(sourceDir, "nested", "app.txt"), "nested data\n")
	writeTestFile(t, filepath.Join(sourceDir, "large.txt"), strings.Repeat("x", rafInlineThreshold+32))
	if err := os.Symlink("README.md", filepath.Join(sourceDir, "link.txt")); err != nil {
		t.Fatalf("Symlink() returned error: %v", err)
	}

	manifest := seedWorkspaceSessionFromDirectory(t, store, "repo", "main", "initial", sourceDir)
	targetDir := filepath.Join(t.TempDir(), "mounted")
	if err := materializeManifestToPath(context.Background(), store, "repo", manifest, targetDir); err != nil {
		t.Fatalf("materializeManifestToPath() returned error: %v", err)
	}

	readme, err := os.ReadFile(filepath.Join(targetDir, "README.md"))
	if err != nil {
		t.Fatalf("ReadFile(README.md) returned error: %v", err)
	}
	if string(readme) != "hello raf\n" {
		t.Fatalf("README.md = %q, want %q", string(readme), "hello raf\n")
	}

	nested, err := os.ReadFile(filepath.Join(targetDir, "nested", "app.txt"))
	if err != nil {
		t.Fatalf("ReadFile(nested/app.txt) returned error: %v", err)
	}
	if string(nested) != "nested data\n" {
		t.Fatalf("nested/app.txt = %q, want %q", string(nested), "nested data\n")
	}

	large, err := os.ReadFile(filepath.Join(targetDir, "large.txt"))
	if err != nil {
		t.Fatalf("ReadFile(large.txt) returned error: %v", err)
	}
	if string(large) != strings.Repeat("x", rafInlineThreshold+32) {
		t.Fatalf("large.txt length = %d, want %d", len(large), rafInlineThreshold+32)
	}

	linkTarget, err := os.Readlink(filepath.Join(targetDir, "link.txt"))
	if err != nil {
		t.Fatalf("Readlink(link.txt) returned error: %v", err)
	}
	if linkTarget != "README.md" {
		t.Fatalf("Readlink(link.txt) = %q, want %q", linkTarget, "README.md")
	}
}

func TestMaterializeSessionPreservesModesAndTimes(t *testing.T) {
	t.Helper()

	store, closeStore := newRAFStoreForTest(t)
	defer closeStore()

	sourceDir := t.TempDir()
	scriptPath := filepath.Join(sourceDir, "scripts", "tool.sh")
	privateDir := filepath.Join(sourceDir, "private")
	writeTestFile(t, scriptPath, "#!/bin/sh\necho hi\n")
	writeTestFile(t, filepath.Join(privateDir, "note.txt"), "secret\n")
	if err := os.Chmod(scriptPath, 0o751); err != nil {
		t.Fatalf("Chmod(script) returned error: %v", err)
	}
	if err := os.Chmod(privateDir, 0o710); err != nil {
		t.Fatalf("Chmod(private dir) returned error: %v", err)
	}

	fileTime := time.Unix(1_700_000_000, 0).UTC()
	dirTime := fileTime.Add(2 * time.Hour)
	rootTime := fileTime.Add(4 * time.Hour)
	if err := os.Chtimes(scriptPath, fileTime, fileTime); err != nil {
		t.Fatalf("Chtimes(script) returned error: %v", err)
	}
	if err := os.Chtimes(privateDir, dirTime, dirTime); err != nil {
		t.Fatalf("Chtimes(private dir) returned error: %v", err)
	}
	if err := os.Chtimes(sourceDir, rootTime, rootTime); err != nil {
		t.Fatalf("Chtimes(source dir) returned error: %v", err)
	}

	cfg := defaultConfig()
	cfg.WorkRoot = t.TempDir()

	seedWorkspaceSessionFromDirectory(t, store, "repo", "main", "initial", sourceDir)
	if err := materializeSession(context.Background(), store, cfg, "repo", "main"); err != nil {
		t.Fatalf("materializeSession() returned error: %v", err)
	}

	treePath := rafSessionTreePath(cfg, "repo", "main")
	fileInfo, err := os.Stat(filepath.Join(treePath, "scripts", "tool.sh"))
	if err != nil {
		t.Fatalf("Stat(tool.sh) returned error: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o751 {
		t.Fatalf("tool.sh mode = %#o, want %#o", got, 0o751)
	}
	if got := fileInfo.ModTime().UnixMilli(); got != fileTime.UnixMilli() {
		t.Fatalf("tool.sh mtime = %d, want %d", got, fileTime.UnixMilli())
	}

	dirInfo, err := os.Stat(filepath.Join(treePath, "private"))
	if err != nil {
		t.Fatalf("Stat(private) returned error: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o710 {
		t.Fatalf("private dir mode = %#o, want %#o", got, 0o710)
	}
	if got := dirInfo.ModTime().UnixMilli(); got != dirTime.UnixMilli() {
		t.Fatalf("private dir mtime = %d, want %d", got, dirTime.UnixMilli())
	}
}

func newRAFStoreForTest(t *testing.T) (*rafStore, func()) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return newRAFStore(rdb), func() {
		_ = rdb.Close()
	}
}

func seedWorkspaceSessionFromDirectory(t *testing.T, store *rafStore, workspace, session, savepoint, sourceDir string) manifest {
	t.Helper()

	manifest, blobs, stats, err := buildManifestFromDirectory(sourceDir, workspace, savepoint)
	if err != nil {
		t.Fatalf("buildManifestFromDirectory() returned error: %v", err)
	}
	hash, err := hashManifest(manifest)
	if err != nil {
		t.Fatalf("hashManifest() returned error: %v", err)
	}

	now := time.Now().UTC()
	ctx := context.Background()
	if err := store.saveBlobs(ctx, workspace, blobs); err != nil {
		t.Fatalf("saveBlobs() returned error: %v", err)
	}
	if err := store.addBlobRefs(ctx, workspace, manifest, now); err != nil {
		t.Fatalf("addBlobRefs() returned error: %v", err)
	}
	if err := store.putWorkspaceMeta(ctx, workspaceMeta{
		Version:          rafFormatVersion,
		Name:             workspace,
		CreatedAt:        now,
		DefaultSession:   session,
		DefaultSavepoint: savepoint,
	}); err != nil {
		t.Fatalf("putWorkspaceMeta() returned error: %v", err)
	}
	if err := store.putSavepoint(ctx, savepointMeta{
		Version:      rafFormatVersion,
		ID:           savepoint,
		Name:         savepoint,
		Workspace:    workspace,
		Session:      session,
		ManifestHash: hash,
		CreatedAt:    now,
		FileCount:    stats.FileCount,
		DirCount:     stats.DirCount,
		TotalBytes:   stats.TotalBytes,
	}, manifest); err != nil {
		t.Fatalf("putSavepoint() returned error: %v", err)
	}
	if err := store.putSessionMeta(ctx, sessionMeta{
		Version:       rafFormatVersion,
		Workspace:     workspace,
		Name:          session,
		HeadSavepoint: savepoint,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("putSessionMeta() returned error: %v", err)
	}
	return manifest
}
