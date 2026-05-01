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

const attachmentRegistryVersion = 1

type attachmentRegistry struct {
	Version     int                `json:"version"`
	UpdatedAt   time.Time          `json:"updated_at"`
	Attachments []attachmentRecord `json:"attachments"`
}

type attachmentRecord struct {
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

func attachmentRegistryPath() string {
	return filepath.Join(stateDir(), "attachments.json")
}

func loadAttachmentRegistry() (attachmentRegistry, error) {
	reg := attachmentRegistry{Version: attachmentRegistryVersion}
	raw, err := os.ReadFile(attachmentRegistryPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return reg, nil
		}
		return reg, err
	}
	if err := json.Unmarshal(raw, &reg); err != nil {
		return reg, fmt.Errorf("parse attachment registry: %w", err)
	}
	if reg.Version == 0 {
		reg.Version = attachmentRegistryVersion
	}
	if reg.Attachments == nil {
		reg.Attachments = []attachmentRecord{}
	}
	return reg, nil
}

func saveAttachmentRegistry(reg attachmentRegistry) error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	reg.Version = attachmentRegistryVersion
	reg.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(stateDir(), ".attachments-*.json")
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
	return os.Rename(tmpName, attachmentRegistryPath())
}

func normalizeAttachmentPath(raw string) (string, error) {
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

func attachmentByPath(reg attachmentRegistry, localPath string) (attachmentRecord, bool) {
	target := filepath.Clean(localPath)
	for _, rec := range reg.Attachments {
		if filepath.Clean(rec.LocalPath) == target {
			return rec, true
		}
	}
	return attachmentRecord{}, false
}

func removeAttachmentByPath(reg *attachmentRegistry, localPath string) (attachmentRecord, bool) {
	if reg == nil {
		return attachmentRecord{}, false
	}
	target := filepath.Clean(localPath)
	for i, rec := range reg.Attachments {
		if filepath.Clean(rec.LocalPath) == target {
			reg.Attachments = append(reg.Attachments[:i], reg.Attachments[i+1:]...)
			return rec, true
		}
	}
	return attachmentRecord{}, false
}

func removeAttachmentByWorkspaceRef(reg *attachmentRegistry, ref string) (attachmentRecord, bool, error) {
	if reg == nil {
		return attachmentRecord{}, false, nil
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return attachmentRecord{}, false, nil
	}
	matches := make([]int, 0, 1)
	for i, rec := range reg.Attachments {
		if strings.TrimSpace(rec.Workspace) == ref || strings.TrimSpace(rec.WorkspaceID) == ref {
			matches = append(matches, i)
		}
	}
	switch len(matches) {
	case 0:
		return attachmentRecord{}, false, nil
	case 1:
		i := matches[0]
		rec := reg.Attachments[i]
		reg.Attachments = append(reg.Attachments[:i], reg.Attachments[i+1:]...)
		return rec, true, nil
	default:
		paths := make([]string, 0, len(matches))
		for _, i := range matches {
			paths = append(paths, reg.Attachments[i].LocalPath)
		}
		return attachmentRecord{}, false, fmt.Errorf("workspace %s matches multiple attachments: %s", ref, strings.Join(paths, ", "))
	}
}

func upsertAttachment(reg *attachmentRegistry, rec attachmentRecord) {
	if reg == nil {
		return
	}
	rec.LocalPath = filepath.Clean(rec.LocalPath)
	for i, existing := range reg.Attachments {
		if filepath.Clean(existing.LocalPath) == rec.LocalPath {
			reg.Attachments[i] = rec
			return
		}
	}
	reg.Attachments = append(reg.Attachments, rec)
}

func attachmentPathConflict(reg attachmentRegistry, localPath string) (attachmentRecord, bool) {
	target := filepath.Clean(localPath)
	for _, rec := range reg.Attachments {
		existing := filepath.Clean(rec.LocalPath)
		if existing == target || pathContains(existing, target) || pathContains(target, existing) {
			return rec, true
		}
	}
	return attachmentRecord{}, false
}

func attachmentWorkspaceConflict(reg attachmentRegistry, workspaceID, workspaceName string) (attachmentRecord, bool) {
	workspaceID = strings.TrimSpace(workspaceID)
	workspaceName = strings.TrimSpace(workspaceName)
	for _, rec := range reg.Attachments {
		if workspaceID != "" && strings.TrimSpace(rec.WorkspaceID) == workspaceID {
			return rec, true
		}
		if workspaceName != "" && strings.TrimSpace(rec.Workspace) == workspaceName {
			return rec, true
		}
	}
	return attachmentRecord{}, false
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

func attachmentStatus(rec attachmentRecord) string {
	if rec.PID > 0 && processAlive(rec.PID) {
		return "running"
	}
	return "stopped"
}
