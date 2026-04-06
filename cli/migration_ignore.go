package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

const (
	afsIgnoreFilename            = ".afsignore"
	legacyOriginalIgnoreFilename = ".rfsignore"
)

type migrationIgnore struct {
	path    string
	legacy  bool
	matcher ignore.IgnoreParser
}

func loadMigrationIgnore(sourceDir string) (*migrationIgnore, error) {
	candidates := []struct {
		filename string
		legacy   bool
	}{
		{filename: afsIgnoreFilename},
		{filename: legacyOriginalIgnoreFilename, legacy: true},
	}
	for _, candidate := range candidates {
		ignorePath := filepath.Join(sourceDir, candidate.filename)
		if _, err := os.Stat(ignorePath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("stat %s: %w", ignorePath, err)
		}

		matcher, err := ignore.CompileIgnoreFile(ignorePath)
		if err != nil {
			return nil, fmt.Errorf("compile %s: %w", ignorePath, err)
		}

		return &migrationIgnore{
			path:    ignorePath,
			legacy:  candidate.legacy,
			matcher: matcher,
		}, nil
	}
	return nil, nil
}

func (m *migrationIgnore) matches(sourceDir, path string, d fs.DirEntry) (bool, error) {
	if m == nil {
		return false, nil
	}

	rel, err := filepath.Rel(sourceDir, path)
	if err != nil {
		return false, err
	}
	if rel == "." {
		return false, nil
	}

	candidate := filepath.ToSlash(rel)
	if d.IsDir() && !strings.HasSuffix(candidate, "/") {
		candidate += "/"
	}
	return m.matcher.MatchesPath(candidate), nil
}
