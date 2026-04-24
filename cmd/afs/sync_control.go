package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

const (
	syncControlVersion           = 1
	syncControlDirName           = ".afs-sync"
	syncControlRequestsDirName   = ".afs-sync/requests"
	syncControlResultsDirName    = ".afs-sync/results"
	syncControlOpCreateExclusive = "create-exclusive"
	defaultSyncControlTimeout    = 10 * time.Second
)

type syncControlRequest struct {
	Version   int    `json:"version"`
	Operation string `json:"operation"`
	Path      string `json:"path"`
	Content   string `json:"content"`
}

type syncControlResult struct {
	Version   int    `json:"version"`
	Operation string `json:"operation"`
	Path      string `json:"path"`
	Success   bool   `json:"success"`
	Bytes     int    `json:"bytes,omitempty"`
	Error     string `json:"error,omitempty"`
}

func cmdSync(args []string) error {
	if len(args) < 2 || isHelpArg(args[1]) {
		fmt.Fprint(os.Stderr, syncUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	switch args[1] {
	case "create-exclusive":
		return cmdSyncCreateExclusive(args[2:])
	default:
		return fmt.Errorf("unknown sync subcommand %q\n\n%s", args[1], syncUsageText(filepath.Base(os.Args[0])))
	}
}

func cmdSyncCreateExclusive(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, syncCreateExclusiveUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	fs := flag.NewFlagSet("sync create-exclusive", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var content optionalString
	var contentFile string
	var timeout time.Duration
	fs.Var(&content, "content", "file content")
	fs.StringVar(&contentFile, "content-file", "", "read content from file")
	fs.DurationVar(&timeout, "timeout", defaultSyncControlTimeout, "how long to wait for the daemon result")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s", syncCreateExclusiveUsageText(filepath.Base(os.Args[0])))
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("%s", syncCreateExclusiveUsageText(filepath.Base(os.Args[0])))
	}
	if content.set && strings.TrimSpace(contentFile) != "" {
		return errors.New("--content and --content-file are mutually exclusive")
	}

	cfg, err := loadAFSConfig()
	if err != nil {
		return err
	}
	st, err := loadState()
	if err != nil {
		return fmt.Errorf("sync state unavailable: %w\nRun '%s up --mode sync' first", err, filepath.Base(os.Args[0]))
	}
	if strings.TrimSpace(st.Mode) != modeSync || st.SyncPID <= 0 || !processAlive(st.SyncPID) {
		return fmt.Errorf("sync mode is not running\nRun '%s up --mode sync' first", filepath.Base(os.Args[0]))
	}
	if !runtimeStateMatchesConfig(cfg, st) {
		return fmt.Errorf("sync daemon does not match the current config\nRun '%s up --mode sync' again", filepath.Base(os.Args[0]))
	}

	localRoot := strings.TrimSpace(st.LocalPath)
	if localRoot == "" {
		localRoot = strings.TrimSpace(cfg.LocalPath)
	}
	if localRoot == "" {
		return errors.New("sync local path is not configured")
	}

	contentValue := content.value
	if strings.TrimSpace(contentFile) != "" {
		data, err := os.ReadFile(contentFile)
		if err != nil {
			return err
		}
		contentValue = string(data)
	}

	normalizedPath, err := normalizeSyncControlTarget(fs.Arg(0))
	if err != nil {
		return err
	}

	requestID, err := randomSuffix()
	if err != nil {
		return err
	}
	request := syncControlRequest{
		Version:   syncControlVersion,
		Operation: syncControlOpCreateExclusive,
		Path:      normalizedPath,
		Content:   contentValue,
	}

	if err := writeSyncControlJSON(syncControlRequestPath(localRoot, requestID), request, 0o600); err != nil {
		return err
	}

	resultPath := syncControlResultPath(localRoot, requestID)
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(resultPath)
		if err == nil {
			_ = os.Remove(resultPath)
			var result syncControlResult
			if err := json.Unmarshal(data, &result); err != nil {
				return fmt.Errorf("parse sync result: %w", err)
			}
			if !result.Success {
				if strings.TrimSpace(result.Error) == "" {
					return errors.New("sync create-exclusive failed")
				}
				return errors.New(result.Error)
			}
			printBox(markerSuccess+" "+clr(ansiBold, "sync create-exclusive"), []boxRow{
				{Label: "workspace", Value: currentWorkspaceLabel(st.CurrentWorkspace)},
				{Label: "path", Value: result.Path},
				{Label: "bytes", Value: fmt.Sprintf("%d", result.Bytes)},
			})
			return nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for sync daemon result for %s", normalizedPath)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func syncUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s sync <subcommand>

Explicit operations for a running sync daemon.

Subcommands:
  create-exclusive   Atomically create a file in the synced workspace exactly once
`, bin)
}

func syncCreateExclusiveUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s sync create-exclusive [--content <text> | --content-file <path>] [--timeout <duration>] <path>

Ask the running sync daemon to atomically create <path> exactly once across
all clients. The path must be absolute inside the workspace, for example:

  %s sync create-exclusive /tasks/001.claim
  %s sync create-exclusive --content "agent-a\n" /tasks/001.claim
`, bin, bin, bin)
}

func syncControlRequestPath(root, requestID string) string {
	return filepath.Join(root, syncControlDirName, "requests", requestID+".json")
}

func syncControlResultPath(root, requestID string) string {
	return filepath.Join(root, syncControlDirName, "results", requestID+".json")
}

func isSyncControlPath(rel string) bool {
	rel = strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/")
	if rel == "" {
		return false
	}
	return rel == syncControlDirName || strings.HasPrefix(rel, syncControlDirName+"/")
}

// Sync mode can observe path changes but not the original open flags, so
// exclusive-create requests travel through a daemon-owned request/result side
// channel under the local sync root.
func syncControlRequestID(rel string) (string, bool) {
	rel = strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/")
	prefix := syncControlRequestsDirName + "/"
	if !strings.HasPrefix(rel, prefix) || !strings.HasSuffix(rel, ".json") {
		return "", false
	}
	rest := strings.TrimSuffix(strings.TrimPrefix(rel, prefix), ".json")
	if rest == "" || strings.Contains(rest, "/") {
		return "", false
	}
	return rest, true
}

func normalizeSyncControlTarget(raw string) (string, error) {
	normalized := normalizeAFSGrepPath(raw)
	if normalized == "/" {
		return "", errors.New("target path must not be /")
	}
	if isSyncControlPath(strings.TrimPrefix(normalized, "/")) {
		return "", fmt.Errorf("path %q is reserved for sync control", normalized)
	}
	return normalized, nil
}

func writeSyncControlJSON(path string, value any, mode uint32) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeAtomicFile(path, data, mode)
}

func writeAtomicFile(absPath string, data []byte, mode uint32) error {
	if mode == 0 {
		mode = 0o644
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	dir := filepath.Dir(absPath)
	base := filepath.Base(absPath)
	suffix, err := randomSuffix()
	if err != nil {
		return err
	}
	tmpName := filepath.Join(dir, "."+base+".afssync.tmp."+suffix)
	f, err := os.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_TRUNC, os.FileMode(mode&0o7777))
	if err != nil {
		return err
	}
	cleanup := func() {
		_ = os.Remove(tmpName)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, absPath); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(absPath, os.FileMode(mode&0o7777)); err != nil && !errors.Is(err, os.ErrNotExist) {
	}
	return nil
}

func ensureSyncRemoteParentDirs(ctx context.Context, fsClient client.Client, normalizedPath string) error {
	trimmed := strings.Trim(normalizedPath, "/")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) <= 1 {
		return nil
	}
	current := ""
	for _, part := range parts[:len(parts)-1] {
		current += "/" + part
		if stat, err := fsClient.Stat(ctx, current); err == nil && stat != nil {
			continue
		} else if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		if err := fsClient.Mkdir(ctx, current); err != nil {
			return err
		}
	}
	return nil
}
