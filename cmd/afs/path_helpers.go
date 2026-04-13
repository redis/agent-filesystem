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
	if colorTerm {
		fmt.Fprintf(os.Stderr, "\n  %s%serror:%s %v\n\n", ansiBold, ansiRed, ansiReset, err)
	} else {
		fmt.Fprintf(os.Stderr, "\n  error: %v\n\n", err)
	}
	os.Exit(1)
}
