package worktree

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	WorkRoot string
}

type LocalState struct {
	Version        int       `json:"version"`
	Workspace      string    `json:"workspace"`
	HeadSavepoint  string    `json:"head_savepoint"`
	Dirty          bool      `json:"dirty"`
	MaterializedAt time.Time `json:"materialized_at"`
	LastScanAt     time.Time `json:"last_scan_at"`
	ArchivedAt     time.Time `json:"archived_at,omitempty"`
}

type ImportStats struct {
	Files    int
	Dirs     int
	Symlinks int
	Ignored  int
	Bytes    int64
}

type ManifestStats struct {
	FileCount  int
	DirCount   int
	TotalBytes int64
}

type IgnoreFunc func(root, path string, d os.DirEntry) (bool, error)

type BuildManifestOptions struct {
	Ignore     IgnoreFunc
	OnProgress func(ImportStats)
}

type BlobLoader func(blobID string) ([]byte, error)

type MaterializeOptions struct {
	OnProgress       func(ImportStats)
	PreserveMetadata bool
}

var ErrWorkspaceConflict = errors.New("afs workspace head conflict")
var ErrWorkspaceNotMaterialized = errors.New("afs workspace is not materialized")

func WorkspaceDir(cfg Config, workspace string) string {
	return filepath.Join(cfg.WorkRoot, workspace)
}

func WorkspaceArchiveDir(cfg Config, workspace string) string {
	return filepath.Join(WorkspaceDir(cfg, workspace), "archive")
}

func WorkspaceStatePath(cfg Config, workspace string) string {
	return filepath.Join(WorkspaceDir(cfg, workspace), "state.json")
}

func WorkspaceTreePath(cfg Config, workspace string) string {
	return filepath.Join(WorkspaceDir(cfg, workspace), "tree")
}

func LoadLocalState(cfg Config, workspace string) (LocalState, error) {
	var st LocalState
	raw, err := os.ReadFile(WorkspaceStatePath(cfg, workspace))
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, err
	}
	return st, nil
}

func SaveLocalState(cfg Config, st LocalState) error {
	if err := os.MkdirAll(WorkspaceDir(cfg, st.Workspace), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(WorkspaceStatePath(cfg, st.Workspace), raw, 0o600)
}

func RemoveLocalWorkspace(cfg Config, workspace string) error {
	return os.RemoveAll(WorkspaceDir(cfg, workspace))
}

func ArchiveLocalTree(cfg Config, workspace string) (string, error) {
	treePath := WorkspaceTreePath(cfg, workspace)
	if _, err := os.Stat(treePath); err != nil {
		return "", err
	}

	archiveDir := WorkspaceArchiveDir(cfg, workspace)
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return "", err
	}

	target := filepath.Join(archiveDir, fmt.Sprintf("%s-%d", workspace, time.Now().UTC().UnixNano()))
	if err := os.Rename(treePath, target); err != nil {
		return "", err
	}
	return target, nil
}

func MaterializedPath(treePath, manifestPath string) string {
	if manifestPath == "/" {
		return treePath
	}
	trimmed := manifestPath
	for len(trimmed) > 0 && trimmed[0] == '/' {
		trimmed = trimmed[1:]
	}
	return filepath.Join(treePath, filepath.FromSlash(trimmed))
}
