package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type openFileHandle struct {
	PID     int
	Command string
	Path    string
}

type openHandleCheckError struct {
	Root    string
	Handles []openFileHandle
	Err     error
}

func (e openHandleCheckError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("cannot verify open file handles under %q: %v", e.Root, e.Err)
	}
	if len(e.Handles) == 0 {
		return fmt.Sprintf("cannot verify open file handles under %q", e.Root)
	}
	limit := len(e.Handles)
	if limit > 5 {
		limit = 5
	}
	parts := make([]string, 0, limit)
	for _, handle := range e.Handles[:limit] {
		label := strings.TrimSpace(handle.Command)
		if label == "" {
			label = "process"
		}
		if handle.PID > 0 {
			label = fmt.Sprintf("%s pid %d", label, handle.PID)
		}
		if path := strings.TrimSpace(handle.Path); path != "" {
			label = fmt.Sprintf("%s (%s)", label, path)
		}
		parts = append(parts, label)
	}
	if extra := len(e.Handles) - limit; extra > 0 {
		parts = append(parts, fmt.Sprintf("%d more", extra))
	}
	return fmt.Sprintf("cannot replace sync folder %q while files are open: %s; close those processes and retry", e.Root, strings.Join(parts, ", "))
}

func (e openHandleCheckError) Unwrap() error {
	return e.Err
}

var checkOpenHandlesUnderPath = realCheckOpenHandlesUnderPath

func ensureNoOpenHandlesUnderPath(root string, ignoredPIDs ...int) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	handles, err := checkOpenHandlesUnderPath(root)
	if err != nil {
		return openHandleCheckError{Root: root, Err: err}
	}
	handles = filterIgnoredOpenHandles(handles, ignoredPIDs...)
	if len(handles) > 0 {
		return openHandleCheckError{Root: root, Handles: handles}
	}
	return nil
}

func filterIgnoredOpenHandles(handles []openFileHandle, ignoredPIDs ...int) []openFileHandle {
	if len(handles) == 0 || len(ignoredPIDs) == 0 {
		return handles
	}
	ignored := make(map[int]struct{}, len(ignoredPIDs))
	for _, pid := range ignoredPIDs {
		if pid > 0 {
			ignored[pid] = struct{}{}
		}
	}
	if len(ignored) == 0 {
		return handles
	}
	filtered := handles[:0]
	for _, handle := range handles {
		if _, ok := ignored[handle.PID]; ok {
			continue
		}
		filtered = append(filtered, handle)
	}
	return filtered
}

func realCheckOpenHandlesUnderPath(root string) ([]openFileHandle, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	absRoot = filepath.Clean(absRoot)
	if _, err := os.Stat(absRoot); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if runtime.GOOS == "linux" {
		return openHandlesFromProc(absRoot)
	}
	return openHandlesFromLsof(absRoot)
}

func openHandlesFromProc(root string) ([]openFileHandle, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var handles []openFileHandle
	seen := make(map[string]struct{})
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		procRoot := filepath.Join("/proc", entry.Name())
		command := procCommand(procRoot)
		for _, candidate := range procHandlePaths(procRoot) {
			recordOpenHandle(root, pid, command, candidate, seen, &handles)
		}
	}
	return handles, nil
}

func procCommand(procRoot string) string {
	raw, err := os.ReadFile(filepath.Join(procRoot, "comm"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func procHandlePaths(procRoot string) []string {
	var paths []string
	for _, name := range []string{"cwd", "root"} {
		if target, err := os.Readlink(filepath.Join(procRoot, name)); err == nil {
			paths = append(paths, target)
		}
	}
	fdDir := filepath.Join(procRoot, "fd")
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return paths
	}
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
		if err != nil {
			continue
		}
		paths = append(paths, target)
	}
	return paths
}

func openHandlesFromLsof(root string) ([]openFileHandle, error) {
	output, err := exec.Command("lsof", "-nP", "-Fpcn", "+D", root).CombinedOutput()
	if len(output) == 0 {
		if err != nil && errors.Is(err, exec.ErrNotFound) {
			return nil, err
		}
		return nil, nil
	}
	handles := parseLsofFieldOutput(root, string(output))
	if len(handles) > 0 {
		return handles, nil
	}
	if err != nil && errors.Is(err, exec.ErrNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil, nil
}

func parseLsofFieldOutput(root, output string) []openFileHandle {
	var handles []openFileHandle
	seen := make(map[string]struct{})
	pid := 0
	command := ""
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(strings.TrimSpace(line[1:]))
		case 'c':
			command = strings.TrimSpace(line[1:])
		case 'n':
			recordOpenHandle(root, pid, command, line[1:], seen, &handles)
		}
	}
	return handles
}

func recordOpenHandle(root string, pid int, command, candidate string, seen map[string]struct{}, handles *[]openFileHandle) {
	path := cleanOpenHandlePath(candidate)
	if path == "" || !pathUnderRoot(root, path) {
		return
	}
	key := fmt.Sprintf("%d\x00%s", pid, path)
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*handles = append(*handles, openFileHandle{PID: pid, Command: command, Path: path})
}

func cleanOpenHandlePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimSuffix(path, " (deleted)")
	if idx := strings.Index(path, " type="); idx >= 0 {
		path = path[:idx]
	}
	if path == "" || !filepath.IsAbs(path) {
		return ""
	}
	return filepath.Clean(path)
}

func pathUnderRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
