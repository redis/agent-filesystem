package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	afsModeAuto  = "auto"
	afsModeCLI   = "cli"
	afsModeLocal = "local"
)

type afsCommandRunner interface {
	Run(args ...string) error
}

type shellAFSRunner struct{}

func (shellAFSRunner) Run(args ...string) error {
	cmd := exec.Command("afs", args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		message = err.Error()
	}
	if strings.Contains(strings.ToLower(message), "forbidden") {
		return fail("afs %s failed: %s\nLiveSkills needs AFS permission to create, import, and mount volumes. Run `afs auth login` or `afs setup`, or set LIVESKILLS_AFS_MODE=local to use the local development adapter.", strings.Join(args, " "), message)
	}
	return fail("afs %s failed: %s", strings.Join(args, " "), message)
}

type LocalAFSAdapter struct {
	Home   string
	Root   string
	Mode   string
	Runner afsCommandRunner
}

func NewLocalAFSAdapter(home string, env map[string]string) *LocalAFSAdapter {
	mode := afsMode(env)
	return &LocalAFSAdapter{
		Home:   home,
		Root:   filepath.Join(home, "afs"),
		Mode:   mode,
		Runner: shellAFSRunner{},
	}
}

func (a *LocalAFSAdapter) SkillVolumeID(owner, slug string) string {
	id := "skill_" + owner + "_" + slug
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, id)
}

func (a *LocalAFSAdapter) EnsureSkillVolume(owner, slug string) (string, error) {
	volumeID := a.SkillVolumeID(owner, slug)
	return volumeID, os.MkdirAll(a.volumePath(volumeID), 0o755)
}

func (a *LocalAFSAdapter) PublishVersion(volumeID, version string, files []SkillFile) (checkpointID string, contentHash string, err error) {
	parts := make([]string, len(files))
	for index, file := range files {
		parts[index] = file.Path + ":" + file.Hash
	}
	sort.Strings(parts)
	contentHash = hashText(strings.Join(parts, "\n"))
	checkpointID = shortID("chk", volumeID+":"+version+":"+contentHash)
	versionRoot := a.versionPath(volumeID, checkpointID)
	if err := os.RemoveAll(versionRoot); err != nil {
		return "", "", err
	}
	for _, file := range files {
		target := filepath.Join(versionRoot, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", "", err
		}
		if err := os.WriteFile(target, file.Content, 0o644); err != nil {
			return "", "", err
		}
	}
	metadata := map[string]string{
		"volumeId":     volumeID,
		"version":      version,
		"checkpointId": checkpointID,
		"contentHash":  contentHash,
	}
	if err := writeJSON(filepath.Join(versionRoot, ".afs-version.json"), metadata); err != nil {
		return "", "", err
	}
	if a.usesCLI() {
		if err := a.importVolumeSnapshot(volumeID, versionRoot); err != nil {
			return "", "", err
		}
		if err := a.Runner.Run("cp", "create", volumeID, checkpointID, "--description", "LiveSkills "+version); err != nil {
			return "", "", err
		}
	}
	return checkpointID, contentHash, nil
}

func (a *LocalAFSAdapter) ExportVersion(volumeID, checkpointID, outputDirectory string) error {
	if err := os.RemoveAll(outputDirectory); err != nil {
		return err
	}
	return copyTree(a.versionPath(volumeID, checkpointID), outputDirectory, map[string]bool{".afs-version.json": true})
}

func (a *LocalAFSAdapter) MaterializeVersion(volumeID, checkpointID, workspaceRoot, relativePath string) (string, error) {
	target := filepath.Join(workspaceRoot, filepath.FromSlash(relativePath))
	if a.usesCLI() {
		return target, os.MkdirAll(target, 0o755)
	}
	if err := os.RemoveAll(target); err != nil {
		return "", err
	}
	if err := copyTree(a.versionPath(volumeID, checkpointID), target, map[string]bool{".afs-version.json": true}); err != nil {
		return "", err
	}
	return target, nil
}

func (a *LocalAFSAdapter) MountWorkspace(workspaceRoot, workspaceName string) error {
	metadata := map[string]string{
		"workspace": workspaceName,
		"mode":      "local-afs",
		"mountedAt": time.Now().UTC().Format(time.RFC3339),
		"note":      "Local development mount metadata. Swap LocalAFSAdapter for the real AFS mount adapter in production.",
	}
	return writeJSON(filepath.Join(workspaceRoot, ".liveskills", "mount.json"), metadata)
}

func (a *LocalAFSAdapter) MountSkillVolume(volumeID, checkpointID, mountPoint, scope, agent string) error {
	if a.usesCLI() {
		session := "liveskills-" + hashText(mountPoint)[:12]
		if checkpointID != "" {
			err := a.Runner.Run("cp", "restore", volumeID, checkpointID)
			if err != nil {
				if !needsStagedCheckpointImport(err) {
					return err
				}
				if importErr := a.importStagedCheckpoint(volumeID, checkpointID); importErr != nil {
					return importErr
				}
				if err := a.Runner.Run("cp", "restore", volumeID, checkpointID); err != nil {
					return err
				}
			}
		}
		if err := a.Runner.Run("vol", "mount", "--yes", "--session", session, volumeID, mountPoint); err != nil {
			if isExistingAFSMount(err, volumeID, mountPoint) {
				return nil
			}
			if existingPath, ok := existingMountedVolumePath(err, volumeID); ok {
				return ensureCanonicalMountAlias(mountPoint, existingPath)
			}
			return err
		}
		return nil
	}
	metadata := map[string]string{
		"volumeId":     volumeID,
		"checkpointId": checkpointID,
		"mountPoint":   mountPoint,
		"scope":        scope,
		"agent":        agent,
		"mode":         "local-afs-volume",
		"mountedAt":    time.Now().UTC().Format(time.RFC3339),
		"note":         "Local development metadata for a direct AFS volume mount. CLI mode calls: afs vol mount <volume> <directory>.",
	}
	return writeJSON(filepath.Join(a.Home, "mounts", hashText(mountPoint)+".json"), metadata)
}

func (a *LocalAFSAdapter) importStagedCheckpoint(volumeID, checkpointID string) error {
	source := a.versionPath(volumeID, checkpointID)
	if _, err := os.Stat(source); err != nil {
		return err
	}
	if err := a.importVolumeSnapshot(volumeID, source); err != nil {
		return err
	}
	return a.Runner.Run("cp", "create", volumeID, checkpointID, "--description", "LiveSkills "+checkpointID)
}

func (a *LocalAFSAdapter) importVolumeSnapshot(volumeID, source string) error {
	err := a.Runner.Run("vol", "import", volumeID, source)
	if err == nil {
		return nil
	}
	if !isExistingVolumeImportError(err) {
		return err
	}
	return a.Runner.Run("vol", "import", "--force", volumeID, source)
}

func needsStagedCheckpointImport(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "does not exist") || strings.Contains(message, "not found")
}

func isExistingVolumeImportError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already exists") ||
		strings.Contains(message, "rerun with --force") ||
		strings.Contains(message, "re-run with --force")
}

func isExistingAFSMount(err error, volumeID, mountPoint string) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "overlaps existing mount") &&
		strings.Contains(message, strings.ToLower(volumeID)) &&
		strings.Contains(message, strings.ToLower(filepath.Clean(mountPoint)))
}

func isMountedVolumeAtDifferentPath(err error, volumeID string) bool {
	_, ok := existingMountedVolumePath(err, volumeID)
	return ok
}

func existingMountedVolumePath(err error, volumeID string) (string, bool) {
	if err == nil {
		return "", false
	}
	message := err.Error()
	lower := strings.ToLower(message)
	if !strings.Contains(lower, strings.ToLower(volumeID)) || !strings.Contains(lower, "already mounted at ") {
		return "", false
	}
	index := strings.LastIndex(lower, "already mounted at ")
	if index < 0 {
		return "", false
	}
	path := strings.TrimSpace(message[index+len("already mounted at "):])
	path = strings.TrimRight(path, ". \t\r\n")
	if path == "" {
		return "", false
	}
	return filepath.Clean(path), true
}

func afsMode(env map[string]string) string {
	mode := strings.ToLower(envValue(env, "LIVESKILLS_AFS_MODE"))
	if mode == "" {
		mode = afsModeAuto
	}
	if mode == "afs" {
		mode = afsModeCLI
	}
	if mode == afsModeAuto {
		if _, err := exec.LookPath("afs"); err == nil {
			return afsModeCLI
		}
		return afsModeLocal
	}
	if mode == afsModeCLI || mode == afsModeLocal {
		return mode
	}
	return afsModeLocal
}

func (a *LocalAFSAdapter) usesCLI() bool {
	return a.Mode == afsModeCLI
}

func (a *LocalAFSAdapter) volumePath(volumeID string) string {
	return filepath.Join(a.Root, "volumes", volumeID)
}

func (a *LocalAFSAdapter) versionPath(volumeID, checkpointID string) string {
	return filepath.Join(a.volumePath(volumeID), "checkpoints", checkpointID)
}

func copyTree(source, destination string, skip map[string]bool) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		if skip[entry.Name()] {
			continue
		}
		sourcePath := filepath.Join(source, entry.Name())
		destinationPath := filepath.Join(destination, entry.Name())
		if entry.IsDir() {
			if err := copyTree(sourcePath, destinationPath, skip); err != nil {
				return err
			}
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(destinationPath, data, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}
