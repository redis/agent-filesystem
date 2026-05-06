package client

import (
	"context"
	"errors"
	"strings"

	"github.com/redis/agent-filesystem/internal/rediscontent"
	"github.com/redis/go-redis/v9"
)

func (c *nativeClient) Cp(ctx context.Context, src, dst string, recursive bool) error {
	src = normalizePath(src)
	dst = normalizePath(dst)

	resolved, inode, err := c.resolvePath(ctx, src, true)
	if err != nil {
		return err
	}
	if inode.Type == "dir" && !recursive {
		return errors.New("source is a directory — use recursive")
	}

	if _, _, err := c.resolvePath(ctx, dst, false); err == nil {
		return errors.New("already exists")
	} else if !errors.Is(err, redis.Nil) {
		return err
	}

	switch inode.Type {
	case "file":
		return c.copyFile(ctx, resolved, dst, inode)
	case "symlink":
		return c.copySymlink(ctx, dst, inode)
	case "dir":
		return c.copyDirRecursive(ctx, resolved, dst, inode)
	default:
		return errors.New("unsupported inode type")
	}
}

func (c *nativeClient) copyFile(ctx context.Context, srcPath, dstPath string, src *inodeData) error {
	content, err := c.loadContentExternal(ctx, src.ID, src.ContentRef)
	if err != nil {
		return err
	}
	now := nowMs()
	return c.createInodeAtPath(ctx, dstPath, &inodeData{
		Type:    "file",
		Mode:    src.Mode,
		UID:     src.UID,
		GID:     src.GID,
		Size:    src.Size,
		CtimeMs: now,
		MtimeMs: now,
		AtimeMs: now,
		Content: content,
	}, true)
}

func (c *nativeClient) copySymlink(ctx context.Context, dstPath string, src *inodeData) error {
	now := nowMs()
	return c.createInodeAtPath(ctx, dstPath, &inodeData{
		Type:    "symlink",
		Mode:    src.Mode,
		UID:     src.UID,
		GID:     src.GID,
		Size:    src.Size,
		CtimeMs: now,
		MtimeMs: now,
		AtimeMs: now,
		Target:  src.Target,
	}, true)
}

func (c *nativeClient) copyDirRecursive(ctx context.Context, srcDir, dstDir string, src *inodeData) error {
	now := nowMs()
	if err := c.createInodeAtPath(ctx, dstDir, &inodeData{
		Type:    "dir",
		Mode:    src.Mode,
		UID:     src.UID,
		GID:     src.GID,
		Size:    0,
		CtimeMs: now,
		MtimeMs: now,
		AtimeMs: now,
	}, true); err != nil {
		return err
	}

	children, err := c.listDirChildren(ctx, srcDir, src)
	if err != nil {
		return err
	}
	for _, child := range children {
		dstChild := joinPath(dstDir, child.Name)
		switch child.Inode.Type {
		case "file":
			if err := c.copyFile(ctx, child.Path, dstChild, child.Inode); err != nil {
				return err
			}
		case "dir":
			if err := c.copyDirRecursive(ctx, child.Path, dstChild, child.Inode); err != nil {
				return err
			}
		case "symlink":
			if err := c.copySymlink(ctx, dstChild, child.Inode); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *nativeClient) Tree(ctx context.Context, p string, maxDepth int) ([]TreeEntry, error) {
	if maxDepth <= 0 {
		maxDepth = 64
	}
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return nil, err
	}
	if inode.Type != "dir" {
		return nil, errors.New("not a directory")
	}

	entries := []TreeEntry{{Path: resolved, Type: "dir", Depth: 0}}
	if err := c.treeWalk(ctx, resolved, inode, 1, maxDepth, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (c *nativeClient) treeWalk(ctx context.Context, dirPath string, dir *inodeData, depth, maxDepth int, entries *[]TreeEntry) error {
	if depth > maxDepth {
		return nil
	}
	children, err := c.listDirChildren(ctx, dirPath, dir)
	if err != nil {
		return err
	}
	for _, child := range children {
		*entries = append(*entries, TreeEntry{
			Path:  child.Path,
			Type:  child.Inode.Type,
			Depth: depth,
		})
		if child.Inode.Type == "dir" {
			if err := c.treeWalk(ctx, child.Path, child.Inode, depth+1, maxDepth, entries); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *nativeClient) Find(ctx context.Context, p, pattern string, typeFilter string) ([]string, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return nil, err
	}
	if inode.Type != "dir" {
		return nil, errors.New("not a directory")
	}

	var matches []string
	if err := c.findWalk(ctx, resolved, inode, pattern, typeFilter, &matches); err != nil {
		return nil, err
	}
	return matches, nil
}

func (c *nativeClient) findWalk(ctx context.Context, dirPath string, dir *inodeData, pattern string, typeFilter string, matches *[]string) error {
	if typeFilter == "" || typeFilter == "dir" {
		if globMatch(pattern, baseName(dirPath)) || dirPath == "/" {
			if dirPath == "/" && globMatch(pattern, "/") {
				*matches = append(*matches, dirPath)
			} else if dirPath != "/" && globMatch(pattern, baseName(dirPath)) {
				*matches = append(*matches, dirPath)
			}
		}
	}

	children, err := c.listDirChildren(ctx, dirPath, dir)
	if err != nil {
		return err
	}
	for _, child := range children {
		if child.Inode.Type == "dir" {
			if err := c.findWalk(ctx, child.Path, child.Inode, pattern, typeFilter, matches); err != nil {
				return err
			}
			continue
		}
		if typeFilter == "" || typeFilter == child.Inode.Type {
			if globMatch(pattern, child.Name) {
				*matches = append(*matches, child.Path)
			}
		}
	}
	return nil
}

func (c *nativeClient) Grep(ctx context.Context, p, pattern string, nocase bool) ([]GrepMatch, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return nil, err
	}

	var matches []GrepMatch
	switch inode.Type {
	case "file":
		fileMatches, err := c.grepFileWithPrefilter(ctx, resolved, inode, pattern, nocase)
		if err != nil {
			return nil, err
		}
		matches = append(matches, fileMatches...)
	case "dir":
		if err := c.grepWalk(ctx, resolved, inode, pattern, nocase, &matches); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("not a file or directory")
	}
	return matches, nil
}

func (c *nativeClient) grepWalk(ctx context.Context, dirPath string, dir *inodeData, pattern string, nocase bool, matches *[]GrepMatch) error {
	children, err := c.listDirChildren(ctx, dirPath, dir)
	if err != nil {
		return err
	}
	for _, child := range children {
		switch child.Inode.Type {
		case "file":
			fileMatches, err := c.grepFileWithPrefilter(ctx, child.Path, child.Inode, pattern, nocase)
			if err != nil {
				return err
			}
			*matches = append(*matches, fileMatches...)
		case "dir":
			if err := c.grepWalk(ctx, child.Path, child.Inode, pattern, nocase, matches); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *nativeClient) grepFileWithPrefilter(ctx context.Context, filePath string, inode *inodeData, pattern string, nocase bool) ([]GrepMatch, error) {
	if inode != nil && inode.ContentRef == rediscontent.RefArray {
		if probe := grepLiteralProbe(pattern); probe != "" {
			ok, err := rediscontent.MayContainLiteral(ctx, c.rdb, c.keys.content(inode.ID), inode.Size, probe, nocase)
			if err == nil && !ok {
				return nil, nil
			}
		}
	}
	content, err := c.loadContentExternal(ctx, inode.ID, inode.ContentRef)
	if err != nil {
		return nil, err
	}
	return c.grepFile(filePath, content, pattern, nocase), nil
}

func (c *nativeClient) grepFile(filePath string, content string, pattern string, nocase bool) []GrepMatch {
	checkLen := len(content)
	if checkLen > 8192 {
		checkLen = 8192
	}
	if strings.ContainsRune(content[:checkLen], '\x00') {
		pat := pattern
		text := content
		if nocase {
			pat = strings.ToLower(pat)
			text = strings.ToLower(text)
		}
		if globMatch(pat, text) {
			return []GrepMatch{{Path: filePath, LineNum: 0, Line: "Binary file matches"}}
		}
		return nil
	}

	lines := strings.Split(content, "\n")
	matches := make([]GrepMatch, 0)
	for i, line := range lines {
		pat := pattern
		text := line
		if nocase {
			pat = strings.ToLower(pat)
			text = strings.ToLower(text)
		}
		if globMatch(pat, text) {
			matches = append(matches, GrepMatch{
				Path:    filePath,
				LineNum: int64(i + 1),
				Line:    line,
			})
		}
	}
	return matches
}

func grepLiteralProbe(pattern string) string {
	best := ""
	var current strings.Builder
	escaped := false
	inClass := false

	flush := func() {
		if current.Len() > len(best) {
			best = current.String()
		}
		current.Reset()
	}

	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if escaped {
			current.WriteByte(ch)
			escaped = false
			continue
		}
		if inClass {
			if ch == ']' {
				inClass = false
			}
			flush()
			continue
		}
		switch ch {
		case '\\':
			escaped = true
		case '*', '?':
			flush()
		case '[':
			inClass = true
			flush()
		default:
			current.WriteByte(ch)
		}
	}
	if escaped {
		current.WriteByte('\\')
	}
	flush()
	return best
}
