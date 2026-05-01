package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const mountRegistryVersion = 1

type mountRegistry struct {
	Version     int                `json:"version"`
	UpdatedAt   time.Time          `json:"updated_at"`
	Mounts []mountRecord `json:"mounts"`
}

type mountRecord struct {
	ID                   string    `json:"id"`
	Workspace            string    `json:"workspace"`
	WorkspaceID          string    `json:"workspace_id,omitempty"`
	LocalPath            string    `json:"local_path"`
	Mode                 string    `json:"mode"`
	ProductMode          string    `json:"product_mode,omitempty"`
	ControlPlaneURL      string    `json:"control_plane_url,omitempty"`
	ControlPlaneDatabase string    `json:"control_plane_database,omitempty"`
	SessionID            string    `json:"session_id,omitempty"`
	RedisAddr            string    `json:"redis_addr"`
	RedisDB              int       `json:"redis_db"`
	RedisKey             string    `json:"redis_key"`
	PID                  int       `json:"pid"`
	ReadOnly             bool      `json:"read_only,omitempty"`
	SyncLog              string    `json:"sync_log,omitempty"`
	StartedAt            time.Time `json:"started_at"`
}

func mountRegistryPath() string {
	return filepath.Join(stateDir(), "mounts.json")
}

func loadMountRegistry() (mountRegistry, error) {
	reg := mountRegistry{Version: mountRegistryVersion}
	raw, err := os.ReadFile(mountRegistryPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return reg, nil
		}
		return reg, err
	}
	if err := json.Unmarshal(raw, &reg); err != nil {
		return reg, fmt.Errorf("parse mount registry: %w", err)
	}
	if reg.Version == 0 {
		reg.Version = mountRegistryVersion
	}
	if reg.Mounts == nil {
		reg.Mounts = []mountRecord{}
	}
	return reg, nil
}

func saveMountRegistry(reg mountRegistry) error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	reg.Version = mountRegistryVersion
	reg.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(stateDir(), ".mounts-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, mountRegistryPath())
}

func normalizeMountPath(raw string) (string, error) {
	path, err := expandPath(raw)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(path) == "" {
		return "", errors.New("directory is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func mountByPath(reg mountRegistry, localPath string) (mountRecord, bool) {
	target := filepath.Clean(localPath)
	for _, rec := range reg.Mounts {
		if filepath.Clean(rec.LocalPath) == target {
			return rec, true
		}
	}
	return mountRecord{}, false
}

func removeMountByPath(reg *mountRegistry, localPath string) (mountRecord, bool) {
	if reg == nil {
		return mountRecord{}, false
	}
	target := filepath.Clean(localPath)
	for i, rec := range reg.Mounts {
		if filepath.Clean(rec.LocalPath) == target {
			reg.Mounts = append(reg.Mounts[:i], reg.Mounts[i+1:]...)
			return rec, true
		}
	}
	return mountRecord{}, false
}

func removeMountByWorkspaceRef(reg *mountRegistry, ref string) (mountRecord, bool, error) {
	if reg == nil {
		return mountRecord{}, false, nil
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return mountRecord{}, false, nil
	}
	matches := make([]int, 0, 1)
	for i, rec := range reg.Mounts {
		if strings.TrimSpace(rec.Workspace) == ref || strings.TrimSpace(rec.WorkspaceID) == ref {
			matches = append(matches, i)
		}
	}
	switch len(matches) {
	case 0:
		return mountRecord{}, false, nil
	case 1:
		i := matches[0]
		rec := reg.Mounts[i]
		reg.Mounts = append(reg.Mounts[:i], reg.Mounts[i+1:]...)
		return rec, true, nil
	default:
		paths := make([]string, 0, len(matches))
		for _, i := range matches {
			paths = append(paths, reg.Mounts[i].LocalPath)
		}
		return mountRecord{}, false, fmt.Errorf("workspace %s matches multiple mounts: %s", ref, strings.Join(paths, ", "))
	}
}

func upsertMount(reg *mountRegistry, rec mountRecord) {
	if reg == nil {
		return
	}
	rec.LocalPath = filepath.Clean(rec.LocalPath)
	for i, existing := range reg.Mounts {
		if filepath.Clean(existing.LocalPath) == rec.LocalPath {
			reg.Mounts[i] = rec
			return
		}
	}
	reg.Mounts = append(reg.Mounts, rec)
}

func mountPathConflict(reg mountRegistry, localPath string) (mountRecord, bool) {
	target := filepath.Clean(localPath)
	for _, rec := range reg.Mounts {
		existing := filepath.Clean(rec.LocalPath)
		if existing == target || pathContains(existing, target) || pathContains(target, existing) {
			return rec, true
		}
	}
	return mountRecord{}, false
}

func mountWorkspaceConflict(reg mountRegistry, workspaceID, workspaceName string) (mountRecord, bool) {
	workspaceID = strings.TrimSpace(workspaceID)
	workspaceName = strings.TrimSpace(workspaceName)
	for _, rec := range reg.Mounts {
		if workspaceID != "" && strings.TrimSpace(rec.WorkspaceID) == workspaceID {
			return rec, true
		}
		if workspaceName != "" && strings.TrimSpace(rec.Workspace) == workspaceName {
			return rec, true
		}
	}
	return mountRecord{}, false
}

func pathContains(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func mountStatus(rec mountRecord) string {
	if rec.PID > 0 && processAlive(rec.PID) {
		return "running"
	}
	return "stopped"
}
