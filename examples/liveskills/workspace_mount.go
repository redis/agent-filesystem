package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SkillsWorkspace struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Scope string `json:"scope"`
	Root  string `json:"root"`
}

type skillsWorkspaceMetadata struct {
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	Root      string `json:"root"`
	Mode      string `json:"mode"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt"`
}

type skillVolumeAttachmentMetadata struct {
	Version      int    `json:"version"`
	WorkspaceID  string `json:"workspaceId"`
	Scope        string `json:"scope"`
	Slug         string `json:"slug"`
	VolumeID     string `json:"volumeId"`
	CheckpointID string `json:"checkpointId"`
	Path         string `json:"path"`
	Mode         string `json:"mode"`
	AttachedAt   string `json:"attachedAt"`
	UpdatedAt    string `json:"updatedAt"`
}

func (a *LocalAFSAdapter) SkillsWorkspaceID(scope, workspaceName string) string {
	scope = safeWorkspacePart(defaultString(scope, scopeProject))
	name := safeWorkspacePart(defaultString(workspaceName, scope))
	return "liveskills_" + scope + "_" + name
}

func (a *LocalAFSAdapter) EnsureSkillsWorkspace(scope, workspaceName string) (SkillsWorkspace, error) {
	scope = defaultString(scope, scopeProject)
	workspaceName = defaultString(workspaceName, scope)
	workspace := SkillsWorkspace{
		ID:    a.SkillsWorkspaceID(scope, workspaceName),
		Name:  workspaceName,
		Scope: scope,
		Root:  a.skillsWorkspaceRoot(scope, workspaceName),
	}
	if a.usesCLI() {
		if err := a.Runner.Run("ws", "create", workspace.ID); err != nil && !isAlreadyExistsError(err) {
			return SkillsWorkspace{}, err
		}
	}
	if err := os.MkdirAll(workspace.Root, 0o755); err != nil {
		return SkillsWorkspace{}, err
	}
	if err := a.writeSkillsWorkspaceMetadata(workspace, "local-afs-workspace"); err != nil {
		return SkillsWorkspace{}, err
	}
	return workspace, nil
}

func (a *LocalAFSAdapter) MountSkillsWorkspace(workspace SkillsWorkspace, mountPoint string) (SkillsWorkspace, error) {
	if mountPoint == "" {
		mountPoint = workspace.Root
	}
	mountPoint, err := cleanAbs(mountPoint)
	if err != nil {
		return SkillsWorkspace{}, err
	}
	if a.usesCLI() {
		if err := a.Runner.Run("ws", "mount", workspace.ID, mountPoint); err != nil &&
			!isExistingWorkspaceMount(err, workspace.ID, mountPoint) &&
			!isEmptySkillsWorkspaceMount(err, workspace.ID) {
			return SkillsWorkspace{}, err
		}
	}
	workspace.Root = mountPoint
	if err := os.MkdirAll(workspace.Root, 0o755); err != nil {
		return SkillsWorkspace{}, err
	}
	if err := a.writeSkillsWorkspaceMetadata(workspace, "local-afs-workspace-mount"); err != nil {
		return SkillsWorkspace{}, err
	}
	return workspace, nil
}

func (a *LocalAFSAdapter) AttachSkillVolume(workspace SkillsWorkspace, volumeID, checkpointID, slug string) (string, error) {
	if slug = slugify(slug); slug == "" {
		return "", fail("Skill slug cannot be empty")
	}
	canonicalPath := canonicalSkillPath(workspace.Root, slug)
	if a.usesCLI() {
		if checkpointID != "" {
			err := a.Runner.Run("cp", "restore", volumeID, checkpointID)
			if err != nil {
				if !needsStagedCheckpointImport(err) {
					return "", err
				}
				if importErr := a.importStagedCheckpoint(volumeID, checkpointID); importErr != nil {
					return "", importErr
				}
				if err := a.Runner.Run("cp", "restore", volumeID, checkpointID); err != nil {
					return "", err
				}
			}
		}
		session := "liveskills-ws-" + hashText(workspace.ID + ":" + slug)[:12]
		if err := a.Runner.Run("vol", "mount", "--yes", "--session", session, volumeID, canonicalPath); err != nil {
			switch {
			case isExistingAFSMount(err, volumeID, canonicalPath):
				// Already mounted at the requested canonical path.
			case isMountedVolumeAtDifferentPath(err, volumeID):
				existingPath, _ := existingMountedVolumePath(err, volumeID)
				if linkErr := ensureCanonicalMountAlias(canonicalPath, existingPath); linkErr != nil {
					return "", linkErr
				}
			default:
				return "", err
			}
		}
	} else {
		if checkpointID == "" {
			if err := os.MkdirAll(canonicalPath, 0o755); err != nil {
				return "", err
			}
		} else {
			if err := os.RemoveAll(canonicalPath); err != nil {
				return "", err
			}
			if err := copyTree(a.versionPath(volumeID, checkpointID), canonicalPath, map[string]bool{".afs-version.json": true}); err != nil {
				return "", err
			}
		}
	}
	if err := a.writeSkillVolumeAttachment(workspace, slug, volumeID, checkpointID, canonicalPath); err != nil {
		return "", err
	}
	return canonicalPath, nil
}

func (a *LocalAFSAdapter) DetachSkillVolumeIfUnused(workspace SkillsWorkspace, volumeID, slug string, linkedPaths []string) error {
	if slug = slugify(slug); slug == "" {
		return fail("Skill slug cannot be empty")
	}
	canonicalPath := canonicalSkillPath(workspace.Root, slug)
	for _, linkedPath := range linkedPaths {
		if SkillSymlinkPointsTo(linkedPath, canonicalPath) {
			return nil
		}
	}
	if a.usesCLI() {
		if err := a.Runner.Run("vol", "unmount", canonicalPath); err != nil && !isNotMountedError(err) {
			return err
		}
	} else if err := os.RemoveAll(canonicalPath); err != nil {
		return err
	}
	if err := os.Remove(a.skillVolumeAttachmentPath(workspace, slug)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (a *LocalAFSAdapter) writeSkillsWorkspaceMetadata(workspace SkillsWorkspace, mode string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	metadata := skillsWorkspaceMetadata{
		Version:   1,
		ID:        workspace.ID,
		Name:      workspace.Name,
		Scope:     workspace.Scope,
		Root:      workspace.Root,
		Mode:      mode,
		UpdatedAt: now,
	}
	metadataPath := filepath.Join(workspace.Root, ".liveskills", "workspace.json")
	if existing, err := os.Stat(metadataPath); err == nil && !existing.IsDir() {
		metadata.CreatedAt = now
	} else {
		metadata.CreatedAt = now
	}
	if err := writeJSON(metadataPath, metadata); err != nil {
		return err
	}
	return writeJSON(filepath.Join(a.Home, "workspaces", workspace.ID+".json"), metadata)
}

func (a *LocalAFSAdapter) writeSkillVolumeAttachment(workspace SkillsWorkspace, slug, volumeID, checkpointID, canonicalPath string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	metadata := skillVolumeAttachmentMetadata{
		Version:      1,
		WorkspaceID:  workspace.ID,
		Scope:        workspace.Scope,
		Slug:         slug,
		VolumeID:     volumeID,
		CheckpointID: checkpointID,
		Path:         canonicalPath,
		Mode:         "local-afs-volume-attachment",
		AttachedAt:   now,
		UpdatedAt:    now,
	}
	return writeJSON(a.skillVolumeAttachmentPath(workspace, slug), metadata)
}

func (a *LocalAFSAdapter) skillVolumeAttachmentPath(workspace SkillsWorkspace, slug string) string {
	return filepath.Join(workspace.Root, ".liveskills", "volumes", slug+".json")
}

func (a *LocalAFSAdapter) skillsWorkspaceRoot(scope, workspaceName string) string {
	scope = safeWorkspacePart(defaultString(scope, scopeProject))
	name := safeWorkspacePart(defaultString(workspaceName, scope))
	return filepath.Join(a.Root, "workspaces", scope, name)
}

func canonicalSkillPath(workspaceRoot, slug string) string {
	return filepath.Join(workspaceRoot, "skills", slug)
}

func safeWorkspacePart(value string) string {
	safe := slugify(value)
	if safe == "" {
		return "default"
	}
	return safe
}

func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already exists") || strings.Contains(message, "exists")
}

func isExistingWorkspaceMount(err error, workspaceID, mountPoint string) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "overlaps existing mount") &&
		strings.Contains(message, strings.ToLower(workspaceID)) &&
		strings.Contains(message, strings.ToLower(filepath.Clean(mountPoint)))
}

func isEmptySkillsWorkspaceMount(err error, workspaceID string) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, strings.ToLower(workspaceID)) &&
		strings.Contains(message, "has no attached volumes")
}

func isNotMountedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not mounted") || strings.Contains(message, "does not exist") || strings.Contains(message, "not found")
}
