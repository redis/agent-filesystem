package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func splitAddr(addr string) (string, int, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid address %q (expected host:port)", addr)
	}
	p, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, err
	}
	return parts[0], p, nil
}

func expandPath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, p[2:])
	}
	return filepath.Abs(p)
}

func resolveBinary(p string) (string, error) {
	if strings.Contains(p, "/") {
		return expandPath(p)
	}
	lp, err := exec.LookPath(p)
	if err != nil {
		return "", fmt.Errorf("binary %q not found in PATH", p)
	}
	return lp, nil
}

func exeDir() string {
	exe, err := executablePath()
	if err != nil {
		cwd, _ := os.Getwd()
		return cwd
	}
	return filepath.Dir(exe)
}

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return resolveExecutablePath(exe), nil
}

func resolveExecutablePath(exe string) string {
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

func defaultRedisBin() string {
	candidate := filepath.Join(os.Getenv("HOME"), "git", "redis", "src", "redis-server")
	if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
		return candidate
	}
	if lp, err := exec.LookPath("redis-server"); err == nil {
		return lp
	}
	return "redis-server"
}

func fatal(err error) {
	showCursor()
	fmt.Fprint(os.Stderr, formatCLIError(err))
	os.Exit(1)
}

func formatCLIError(err error) string {
	if err == nil {
		return "\nError\n\nUnknown error.\n\n"
	}
	message := strings.TrimSpace(strings.ReplaceAll(err.Error(), "\r\n", "\n"))
	message = strings.TrimPrefix(message, "error:")
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Unknown error"
	}

	var out strings.Builder
	out.WriteString("\n")
	out.WriteString(clr(ansiBold+ansiRed, "Error"))
	out.WriteString("\n\n")

	blocks := splitErrorBlocks(message)
	for i, block := range blocks {
		if i > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(block)
	}
	out.WriteString("\n\n")
	return out.String()
}

func splitErrorBlocks(message string) []string {
	rawBlocks := strings.Split(message, "\n\n")
	blocks := make([]string, 0, len(rawBlocks))
	for _, raw := range rawBlocks {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if isUsageBlock(raw) {
			blocks = append(blocks, raw)
			continue
		}
		lines := nonEmptyErrorLines(raw)
		if len(lines) == 0 {
			continue
		}
		if len(lines) == 1 {
			blocks = append(blocks, formatErrorSentence(lines[0]))
			continue
		}

		var current strings.Builder
		for i, line := range lines {
			switch {
			case i == 0:
				current.WriteString(formatErrorSentence(line))
			case isErrorDetailLine(line):
				current.WriteString("\n")
				current.WriteString(line)
			default:
				blocks = append(blocks, current.String())
				current.Reset()
				current.WriteString(formatErrorSentence(line))
			}
		}
		if current.Len() > 0 {
			blocks = append(blocks, current.String())
		}
	}
	if len(blocks) == 0 {
		return []string{"Unknown error."}
	}
	return blocks
}

func nonEmptyErrorLines(raw string) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func isUsageBlock(block string) bool {
	trimmed := strings.TrimSpace(block)
	return strings.HasPrefix(trimmed, "Redis Agent Filesystem") ||
		strings.HasPrefix(trimmed, "Usage:") ||
		strings.Contains(trimmed, "\nUsage:")
}

func isErrorDetailLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "Run ") || strings.HasPrefix(trimmed, "Use ") {
		return false
	}
	before, _, ok := strings.Cut(trimmed, ":")
	if !ok || before == "" || strings.Contains(before, " ") {
		return false
	}
	return true
}

func formatErrorSentence(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return line
	}
	line = uppercaseFirstASCII(line)
	if hasTerminalPunctuation(line) {
		return line
	}
	return line + "."
}

func uppercaseFirstASCII(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] >= 'a' && s[i] <= 'z' {
			return s[:i] + string(s[i]-'a'+'A') + s[i+1:]
		}
		if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= '0' && s[i] <= '9') || s[i] == '"' || s[i] == '\'' || s[i] == '`' {
			return s
		}
	}
	return s
}

func hasTerminalPunctuation(s string) bool {
	s = strings.TrimSpace(stripAnsi(s))
	if s == "" {
		return false
	}
	switch s[len(s)-1] {
	case '.', '!', '?', ':':
		return true
	default:
		return false
	}
}
