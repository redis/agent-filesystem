package client

import (
	"context"
	"errors"
	"strings"
)

func (c *nativeClient) Head(ctx context.Context, p string, n int) (string, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return "", err
	}
	if inode.Type != "file" {
		return "", errors.New("not a file")
	}
	content, err := c.loadContentByID(ctx, inode.ID)
	if err != nil {
		return "", err
	}
	inode.AtimeMs = nowMs()
	_ = c.saveInodeMeta(ctx, resolved, inode)

	lines := splitLines(content)
	if n <= 0 {
		n = 10
	}
	if n > len(lines) {
		n = len(lines)
	}
	return joinLines(lines[:n]), nil
}

func (c *nativeClient) Tail(ctx context.Context, p string, n int) (string, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return "", err
	}
	if inode.Type != "file" {
		return "", errors.New("not a file")
	}
	content, err := c.loadContentByID(ctx, inode.ID)
	if err != nil {
		return "", err
	}
	inode.AtimeMs = nowMs()
	_ = c.saveInodeMeta(ctx, resolved, inode)

	lines := splitLines(content)
	if n <= 0 {
		n = 10
	}
	if n > len(lines) {
		n = len(lines)
	}
	return joinLines(lines[len(lines)-n:]), nil
}

func (c *nativeClient) Lines(ctx context.Context, p string, start, end int) (string, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return "", err
	}
	if inode.Type != "file" {
		return "", errors.New("not a file")
	}
	content, err := c.loadContentByID(ctx, inode.ID)
	if err != nil {
		return "", err
	}
	inode.AtimeMs = nowMs()
	_ = c.saveInodeMeta(ctx, resolved, inode)

	lines := splitLines(content)
	if start < 1 {
		start = 1
	}
	if end < 0 || end > len(lines) {
		end = len(lines)
	}
	if start > len(lines) {
		return "", nil
	}
	return joinLines(lines[start-1 : end]), nil
}

func (c *nativeClient) Wc(ctx context.Context, p string) (*WcResult, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return nil, err
	}
	if inode.Type != "file" {
		return nil, errors.New("not a file")
	}
	content, err := c.loadContentByID(ctx, inode.ID)
	if err != nil {
		return nil, err
	}
	inode.AtimeMs = nowMs()
	_ = c.saveInodeMeta(ctx, resolved, inode)

	chars := int64(len(content))
	lineCount := int64(strings.Count(content, "\n"))
	if chars > 0 && !strings.HasSuffix(content, "\n") {
		lineCount++
	}

	return &WcResult{
		Lines: lineCount,
		Words: int64(len(strings.Fields(content))),
		Chars: chars,
	}, nil
}

func (c *nativeClient) Insert(ctx context.Context, p string, afterLine int, content string) error {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return err
	}
	if inode.Type != "file" {
		return errors.New("not a file")
	}
	existing, err := c.loadContentByID(ctx, inode.ID)
	if err != nil {
		return err
	}

	lines := splitLines(existing)
	var newLines []string
	switch {
	case afterLine == 0:
		newLines = append([]string{content}, lines...)
	case afterLine < 0 || afterLine >= len(lines):
		newLines = append(lines, content)
	default:
		newLines = make([]string, 0, len(lines)+1)
		newLines = append(newLines, lines[:afterLine]...)
		newLines = append(newLines, content)
		newLines = append(newLines, lines[afterLine:]...)
	}

	newContent := joinLines(newLines)
	delta := int64(len(newContent)) - inode.Size
	inode.Content = newContent
	inode.Size = int64(len(newContent))
	now := nowMs()
	inode.MtimeMs = now
	inode.AtimeMs = now
	if err := c.saveInode(ctx, resolved, inode); err != nil {
		return err
	}
	return c.adjustTotalData(ctx, delta)
}

func (c *nativeClient) Replace(ctx context.Context, p string, old, new string, all bool) (int64, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return 0, err
	}
	if inode.Type != "file" {
		return 0, errors.New("not a file")
	}
	existing, err := c.loadContentByID(ctx, inode.ID)
	if err != nil {
		return 0, err
	}

	var count int64
	var newContent string
	if all {
		count = int64(strings.Count(existing, old))
		newContent = strings.ReplaceAll(existing, old, new)
	} else {
		if strings.Contains(existing, old) {
			count = 1
			newContent = strings.Replace(existing, old, new, 1)
		} else {
			return 0, nil
		}
	}
	if count == 0 {
		return 0, nil
	}

	delta := int64(len(newContent)) - inode.Size
	inode.Content = newContent
	inode.Size = int64(len(newContent))
	now := nowMs()
	inode.MtimeMs = now
	inode.AtimeMs = now
	if err := c.saveInode(ctx, resolved, inode); err != nil {
		return 0, err
	}
	if err := c.adjustTotalData(ctx, delta); err != nil {
		return 0, err
	}
	return count, nil
}

func (c *nativeClient) DeleteLines(ctx context.Context, p string, start, end int) (int64, error) {
	resolved, inode, err := c.resolvePath(ctx, p, true)
	if err != nil {
		return 0, err
	}
	if inode.Type != "file" {
		return 0, errors.New("not a file")
	}
	existing, err := c.loadContentByID(ctx, inode.ID)
	if err != nil {
		return 0, err
	}

	lines := splitLines(existing)
	if start < 1 {
		start = 1
	}
	if end < 0 || end > len(lines) {
		end = len(lines)
	}
	if start > len(lines) {
		return 0, nil
	}

	deleted := int64(end - start + 1)
	newLines := make([]string, 0, len(lines)-int(deleted))
	newLines = append(newLines, lines[:start-1]...)
	newLines = append(newLines, lines[end:]...)

	newContent := joinLines(newLines)
	delta := int64(len(newContent)) - inode.Size
	inode.Content = newContent
	inode.Size = int64(len(newContent))
	now := nowMs()
	inode.MtimeMs = now
	inode.AtimeMs = now
	if err := c.saveInode(ctx, resolved, inode); err != nil {
		return 0, err
	}
	if err := c.adjustTotalData(ctx, delta); err != nil {
		return 0, err
	}
	return deleted, nil
}

func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
