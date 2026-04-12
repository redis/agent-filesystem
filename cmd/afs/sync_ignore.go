package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

// syncIgnore decides whether a workspace-relative path should be skipped by
// the sync daemon in either direction. It composes a baseline rule set
// (mac/Windows turds and our own daemon-temp files) with an optional
// .afsignore matcher loaded from the local root.
type syncIgnore struct {
	matcher ignore.IgnoreParser
	root    string
}

// loadSyncIgnore reads .afsignore (or the legacy .rfsignore) from the
// workspace root, if present. The matcher only fires on root-relative paths;
// per-directory .afsignore files are a v2 feature.
func loadSyncIgnore(root string) (*syncIgnore, error) {
	si := &syncIgnore{root: filepath.Clean(root)}
	candidates := []string{
		filepath.Join(root, afsIgnoreFilename),
		filepath.Join(root, legacyOriginalIgnoreFilename),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		matcher, err := ignore.CompileIgnoreFile(path)
		if err != nil {
			return nil, err
		}
		si.matcher = matcher
		break
	}
	return si, nil
}

// shouldIgnore returns true if rel (a workspace-relative POSIX path, no
// leading slash) should be skipped. isDir is needed for gitignore semantics
// where trailing slashes change matching.
func (s *syncIgnore) shouldIgnore(rel string, isDir bool) bool {
	if s == nil {
		return false
	}
	if rel == "" || rel == "." {
		return false
	}
	rel = filepath.ToSlash(rel)
	if isBaselineIgnored(rel) {
		return true
	}
	if s.matcher == nil {
		return false
	}
	candidate := rel
	if isDir && !strings.HasSuffix(candidate, "/") {
		candidate += "/"
	}
	return s.matcher.MatchesPath(candidate)
}

// shouldIgnoreEntry is the fs.WalkDir-friendly form used during full
// reconciliation. It computes the rel path from the absolute one and consults
// shouldIgnore.
func (s *syncIgnore) shouldIgnoreEntry(absPath string, entry fs.DirEntry) bool {
	if s == nil {
		return false
	}
	rel, err := filepath.Rel(s.root, absPath)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return s.shouldIgnore(rel, entry.IsDir())
}

// isBaselineIgnored matches the always-on rules: macOS resource forks, our
// own daemon temp files, and the .afsignore file itself.
func isBaselineIgnored(rel string) bool {
	if rel == "" {
		return false
	}
	base := pathBase(rel)
	if base == ".DS_Store" {
		return true
	}
	if strings.HasPrefix(base, "._") {
		return true
	}
	if strings.HasPrefix(base, ".afs-sync.tmp.") {
		return true
	}
	if strings.Contains(base, ".afssync.tmp.") {
		return true
	}
	if base == afsIgnoreFilename || base == legacyOriginalIgnoreFilename {
		return true
	}
	// State + lock files for the daemon, in case the user mounts ~/.afs.
	if strings.HasSuffix(base, ".afssync.lock") {
		return true
	}
	return false
}

// pathBase returns the final segment of a slash-separated path. We avoid
// filepath.Base because rel paths from the watcher are always slash-form on
// every platform.
func pathBase(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[i+1:]
	}
	return rel
}
