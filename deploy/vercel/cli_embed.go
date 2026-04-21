package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// cliBundle holds the cross-compiled afs binaries that the control plane
// ships to users via /v1/cli. The prod.sh deploy stages the matching
// binaries at cmd/server/cli/<os>-<arch>/afs before Vercel builds this
// package, so the files referenced by //go:embed exist at build time.
// On non-Vercel local builds the directory may contain only a .keep
// placeholder, in which case the resolver falls back to building from
// source.
//
//go:embed all:cli
var cliBundle embed.FS

// extractCLIBundle copies any embedded CLI binaries into a writable
// directory and points the control plane at them via
// AFS_CLI_ARTIFACT_DIR. Returns the directory path (or empty if nothing
// was embedded so the caller can fall through to the normal resolver).
func extractCLIBundle() (string, error) {
	entries, err := fs.ReadDir(cliBundle, "cli")
	if err != nil {
		// Package has no embedded bundle (non-Vercel build).
		return "", nil
	}
	if len(entries) == 0 {
		return "", nil
	}

	targetRoot := filepath.Join(os.TempDir(), "afs-cli")
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return "", fmt.Errorf("create cli artifact dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.Contains(name, "-") {
			continue
		}
		srcDir := path("cli", name)
		dstDir := filepath.Join(targetRoot, name)
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			return "", fmt.Errorf("mkdir %s: %w", dstDir, err)
		}
		files, err := fs.ReadDir(cliBundle, srcDir)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", srcDir, err)
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			data, err := fs.ReadFile(cliBundle, path(srcDir, f.Name()))
			if err != nil {
				return "", fmt.Errorf("read %s: %w", f.Name(), err)
			}
			dstPath := filepath.Join(dstDir, f.Name())
			if err := os.WriteFile(dstPath, data, 0o755); err != nil {
				return "", fmt.Errorf("write %s: %w", dstPath, err)
			}
		}
	}

	return targetRoot, nil
}

// path joins embedded FS segments with forward slashes because embed.FS
// always uses Unix-style separators regardless of the host platform.
func path(parts ...string) string {
	return strings.Join(parts, "/")
}
