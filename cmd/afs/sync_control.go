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

func cmdFS(args []string) error {
	if len(args) < 2 || isHelpArg(args[1]) {
		fmt.Fprint(os.Stderr, fsUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	parsed, err := parseFSDispatchArgs(args[1:])
	if err != nil {
		return err
	}
	if parsed.subcommand == "" || isHelpArg(parsed.subcommand) {
		fmt.Fprint(os.Stderr, fsUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	switch parsed.subcommand {
	case "ls":
		return cmdFSList(parsed.workspace, parsed.args)
	case "cat":
		return cmdFSCat(parsed.workspace, parsed.args)
	case "find":
		return cmdFSFind(parsed.workspace, parsed.args)
	case "create-exclusive":
		if strings.TrimSpace(parsed.workspace) != "" {
			return errors.New("--workspace is not supported with fs create-exclusive; use the attached sync workspace")
		}
		return cmdFileCreateExclusive(parsed.args)
	case "grep":
		return cmdFSGrep(parsed.workspace, parsed.args)
	default:
		return fmt.Errorf("unknown filesystem subcommand %q\n\n%s", parsed.subcommand, fsUsageText(filepath.Base(os.Args[0])))
	}
}

type fsDispatchArgs struct {
	workspace  string
	subcommand string
	args       []string
}

func parseFSDispatchArgs(args []string) (fsDispatchArgs, error) {
	var parsed fsDispatchArgs
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--workspace" || arg == "-w":
			if i+1 >= len(args) {
				return parsed, fmt.Errorf("missing value for %q", arg)
			}
			i++
			parsed.workspace = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--workspace="):
			parsed.workspace = strings.TrimSpace(strings.TrimPrefix(arg, "--workspace="))
		case strings.HasPrefix(arg, "-"):
			return parsed, fmt.Errorf("unknown filesystem flag %q\n\n%s", arg, fsUsageText(filepath.Base(os.Args[0])))
		default:
			parsed.subcommand = arg
			parsed.args = args[i+1:]
			return parsed, nil
		}
	}
	return parsed, nil
}

func cmdFileCreateExclusive(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, fileCreateExclusiveUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	fs := flag.NewFlagSet("fs create-exclusive", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var content optionalString
	var contentFile string
	var timeout time.Duration
	fs.Var(&content, "content", "file content")
	fs.StringVar(&contentFile, "content-file", "", "read content from file")
	fs.DurationVar(&timeout, "timeout", defaultSyncControlTimeout, "how long to wait for the file operation result")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s", fileCreateExclusiveUsageText(filepath.Base(os.Args[0])))
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("%s", fileCreateExclusiveUsageText(filepath.Base(os.Args[0])))
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
		return fmt.Errorf("AFS is not attached in sync mode: %w\nRun '%s ws attach <workspace> <directory>' first", err, filepath.Base(os.Args[0]))
	}
	if strings.TrimSpace(st.Mode) != modeSync || st.SyncPID <= 0 || !processAlive(st.SyncPID) {
		return fmt.Errorf("AFS is not attached in sync mode\nRun '%s ws attach <workspace> <directory>' first", filepath.Base(os.Args[0]))
	}
	if !runtimeStateMatchesConfig(cfg, st) {
		return fmt.Errorf("running AFS sync process does not match the current config\nDetach and attach the workspace again")
	}

	localRoot := strings.TrimSpace(st.LocalPath)
	if localRoot == "" {
		localRoot = strings.TrimSpace(cfg.LocalPath)
	}
	if localRoot == "" {
		return errors.New("AFS local sync path is not configured")
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
				return fmt.Errorf("parse file operation result: %w", err)
			}
			if !result.Success {
				if strings.TrimSpace(result.Error) == "" {
					return errors.New("fs create-exclusive failed")
				}
				return errors.New(result.Error)
			}
			printSection(markerSuccess+" "+clr(ansiBold, "fs create-exclusive"), []outputRow{
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
			return fmt.Errorf("timed out waiting for fs create-exclusive result for %s", normalizedPath)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func fsUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs <subcommand>

Read, search, and safely write workspace files.

Options:
  -w, --workspace <workspace>   Select the workspace for remote inspection

Subcommands:
  ls                 List workspace files
  cat                Print a workspace file
  find               Find workspace paths by name
  grep               Search workspace files
  create-exclusive   Create a workspace file only if it does not already exist

Examples:
  %s fs -w demo ls
  %s fs -w demo cat README.md
  %s fs -w demo find . -name '*.md' -print
  %s fs -w demo grep Redis
`, bin, bin, bin, bin, bin)
}

func fileCreateExclusiveUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs create-exclusive [--content <text> | --content-file <path>] [--timeout <duration>] <path>

Create <path> only if it does not already exist in the workspace. The create is
atomic across connected AFS clients. Requires AFS to be running in sync mode on
this machine. The path must be absolute inside the workspace, for example:

  %s fs create-exclusive /tasks/001.claim
  %s fs create-exclusive --content "agent-a\n" /tasks/001.claim
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
