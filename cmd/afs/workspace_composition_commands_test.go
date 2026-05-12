package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/controlplane"
)

func TestWorkspaceCommandsManageAgentWorkspaceManifests(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNone
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	if err := cmdVolume([]string{"vol", "create", "repo"}); err != nil {
		t.Fatalf("cmdVolume(create) returned error: %v", err)
	}
	if err := cmdWorkspace([]string{"ws", "create", "coding-agent", "--description", "Reads repo files"}); err != nil {
		t.Fatalf("cmdWorkspace(create) returned error: %v", err)
	}
	if err := cmdWorkspace([]string{"ws", "attach", "coding-agent", "repo", "--at", "/repo", "--readonly"}); err != nil {
		t.Fatalf("cmdWorkspace(attach) returned error: %v", err)
	}

	workspaceList, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "list"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(list) returned error: %v", err)
	}
	if !strings.Contains(workspaceList, "coding-agent") {
		t.Fatalf("cmdWorkspace(list) output = %q, want Agent Workspace manifest", workspaceList)
	}
	if strings.Contains(workspaceList, "repo") {
		t.Fatalf("cmdWorkspace(list) output = %q, did not expect raw volume row", workspaceList)
	}

	volumeList, err := captureStdout(t, func() error {
		return cmdVolume([]string{"vol", "list"})
	})
	if err != nil {
		t.Fatalf("cmdVolume(list) returned error: %v", err)
	}
	if !strings.Contains(volumeList, "repo") {
		t.Fatalf("cmdVolume(list) output = %q, want volume row", volumeList)
	}
	if strings.Contains(volumeList, "coding-agent") {
		t.Fatalf("cmdVolume(list) output = %q, did not expect Agent Workspace manifest row", volumeList)
	}

	showJSON, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "show", "coding-agent", "--json"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(show --json) returned error: %v", err)
	}
	var detail controlplane.WorkspaceCompositionDetail
	if err := json.Unmarshal([]byte(showJSON), &detail); err != nil {
		t.Fatalf("Unmarshal(workspace detail) returned error: %v\n%s", err, showJSON)
	}
	if detail.Name != "coding-agent" || len(detail.Mounts) != 1 {
		t.Fatalf("workspace detail = %+v, want one mounted volume", detail)
	}
	mount := detail.Mounts[0]
	if mount.VolumeID == "" || mount.VolumeName != "repo" || mount.MountPath != "/repo" || !mount.Readonly {
		t.Fatalf("mount = %+v, want repo at /repo readonly", mount)
	}
}

func TestWorkspaceAddImportsFolderAndAttachesDefaultPath(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNone
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	sourceDir := filepath.Join(t.TempDir(), "coding-skills")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(sourceDir) returned error: %v", err)
	}
	writeTestFile(t, filepath.Join(sourceDir, "README.md"), "skills\n")

	if err := cmdWorkspace([]string{"ws", "create", "coding-agent"}); err != nil {
		t.Fatalf("cmdWorkspace(create) returned error: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "add", "coding-agent", sourceDir, "--readonly"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(add) returned error: %v", err)
	}
	for _, want := range []string{
		"folder added to workspace",
		"workspace  coding-agent",
		"volume     coding-skills",
		"path       /coding-skills",
		"mode       read-only",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("cmdWorkspace(add) output = %q, want %q", out, want)
		}
	}

	showJSON, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "show", "coding-agent", "--json"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(show --json) returned error: %v", err)
	}
	var detail controlplane.WorkspaceCompositionDetail
	if err := json.Unmarshal([]byte(showJSON), &detail); err != nil {
		t.Fatalf("Unmarshal(workspace detail) returned error: %v\n%s", err, showJSON)
	}
	if len(detail.Mounts) != 1 {
		t.Fatalf("mount count = %d, want 1", len(detail.Mounts))
	}
	mount := detail.Mounts[0]
	if mount.VolumeName != "coding-skills" || mount.MountPath != "/coding-skills" || !mount.Readonly {
		t.Fatalf("mount = %+v, want coding-skills at /coding-skills readonly", mount)
	}
}

func TestWorkspaceAttachPromptsForVolumeWhenOmitted(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNone
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	if err := cmdVolume([]string{"vol", "create", "common-skills"}); err != nil {
		t.Fatalf("cmdVolume(create) returned error: %v", err)
	}
	if err := cmdWorkspace([]string{"ws", "create", "coding-agent"}); err != nil {
		t.Fatalf("cmdWorkspace(create) returned error: %v", err)
	}

	input, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("CreateTemp(stdin) returned error: %v", err)
	}
	if _, err := input.WriteString("1\n"); err != nil {
		t.Fatalf("WriteString(stdin) returned error: %v", err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatalf("Seek(stdin) returned error: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = input.Close()
	})

	out, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "attach", "coding-agent"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(attach prompt) returned error: %v", err)
	}
	for _, want := range []string{
		"Select volume",
		"Volume:",
		"volume attached",
		"volume     common-skills",
		"path       /common-skills",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("cmdWorkspace(attach prompt) output = %q, want %q", out, want)
		}
	}
}

func TestWorkspaceDetachAcceptsVolumeName(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNone
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	if err := cmdVolume([]string{"vol", "create", "knowledge-base"}); err != nil {
		t.Fatalf("cmdVolume(create) returned error: %v", err)
	}
	if err := cmdWorkspace([]string{"ws", "create", "coding-agent"}); err != nil {
		t.Fatalf("cmdWorkspace(create) returned error: %v", err)
	}
	if err := cmdWorkspace([]string{"ws", "attach", "coding-agent", "knowledge-base"}); err != nil {
		t.Fatalf("cmdWorkspace(attach) returned error: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "detach", "coding-agent", "knowledge-base"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(detach) returned error: %v", err)
	}
	if !strings.Contains(out, "volume detached") {
		t.Fatalf("cmdWorkspace(detach) output = %q, want detach summary", out)
	}

	showJSON, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "show", "coding-agent", "--json"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(show --json) returned error: %v", err)
	}
	var detail controlplane.WorkspaceCompositionDetail
	if err := json.Unmarshal([]byte(showJSON), &detail); err != nil {
		t.Fatalf("Unmarshal(workspace detail) returned error: %v\n%s", err, showJSON)
	}
	if len(detail.Mounts) != 0 {
		t.Fatalf("mount count = %d, want 0", len(detail.Mounts))
	}
}

func TestWorkspaceLegacyAttachAliasesAreNotSupported(t *testing.T) {
	t.Helper()

	for _, subcommand := range []string{"remove", "mount-volume", "unmount-volume"} {
		err := cmdWorkspace([]string{"ws", subcommand, "coding-agent", "repo"})
		if err == nil {
			t.Fatalf("cmdWorkspace(%s) returned nil error, want unknown subcommand", subcommand)
		}
		want := `unknown workspace subcommand "` + subcommand + `"`
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("cmdWorkspace(%s) error = %q, want %q", subcommand, err, want)
		}
	}
}

func TestWorkspaceMountUsesDefaultLocalFolderAfterManifestLookup(t *testing.T) {
	t.Helper()

	homeDir := withTempHome(t)
	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.Mode = modeSync
	cfg.MountBackend = mountBackendNone
	saveTempConfig(t, cfg)

	if err := cmdVolume([]string{"vol", "create", "repo"}); err != nil {
		t.Fatalf("cmdVolume(create) returned error: %v", err)
	}
	if err := cmdWorkspace([]string{"ws", "create", "coding-agent"}); err != nil {
		t.Fatalf("cmdWorkspace(create) returned error: %v", err)
	}
	if err := cmdWorkspace([]string{"ws", "attach", "coding-agent", "repo", "--at", "/repo"}); err != nil {
		t.Fatalf("cmdWorkspace(attach) returned error: %v", err)
	}

	input, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("CreateTemp(stdin) returned error: %v", err)
	}
	if _, err := input.WriteString("\n"); err != nil {
		t.Fatalf("WriteString(stdin) returned error: %v", err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatalf("Seek(stdin) returned error: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = input.Close()
	})

	origStartSync := startSyncMountForWorkspaceMount
	t.Cleanup(func() {
		startSyncMountForWorkspaceMount = origStartSync
	})
	var mounted []config
	startSyncMountForWorkspaceMount = func(ctx context.Context, cfg config, selection workspaceSelection, opts mountOptions) error {
		if !opts.quiet {
			t.Fatalf("workspace composition mount should suppress child volume output")
		}
		mounted = append(mounted, cfg)
		reg, err := loadMountRegistry()
		if err != nil {
			return err
		}
		upsertMount(&reg, mountRecord{
			ID:          "mnt_" + selection.ID,
			Workspace:   selection.Name,
			WorkspaceID: selection.ID,
			LocalPath:   cfg.LocalPath,
			Mode:        modeSync,
			RedisAddr:   cfg.RedisAddr,
			RedisDB:     cfg.RedisDB,
		})
		if err := saveMountRegistry(reg); err != nil {
			return err
		}
		return saveSyncState(&SyncState{
			Version:   syncStateVersion,
			Workspace: selection.Name,
			LocalPath: cfg.LocalPath,
			Entries: map[string]SyncEntry{
				"README.md":       {Type: "file"},
				"docs":            {Type: "dir"},
				"docs/guide.md":   {Type: "file"},
				"deleted-old.txt": {Type: "file", Deleted: true},
			},
		})
	}

	out, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "mount", "coding-agent"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(mount) returned error: %v", err)
	}
	if !strings.Contains(out, "Local folder [~/coding-agent]:") {
		t.Fatalf("mount output = %q, want default folder prompt", out)
	}
	for _, want := range []string{
		"Workspace Mounted",
		"workspace  coding-agent",
		"path       ~/coding-agent",
		"mode       sync",
		"Volumes:",
		"~/coding-agent/repo  read-write  2 files",
		"unmount  afs.test ws unmount coding-agent",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("mount output = %q, want substring %q", out, want)
		}
	}
	if strings.Contains(out, "\n/coding-agent/repo") {
		t.Fatalf("mount output should show local home-relative volume path, not root-relative logical path:\n%s", out)
	}
	if strings.Contains(out, "Volume mounted") {
		t.Fatalf("mount output should not include child volume mount sections:\n%s", out)
	}
	if len(mounted) != 1 {
		t.Fatalf("mounted configs = %+v, want one mounted volume", mounted)
	}
	wantPath := filepath.Join(homeDir, "coding-agent", "repo")
	if mounted[0].LocalPath != wantPath {
		t.Fatalf("mounted LocalPath = %q, want %q", mounted[0].LocalPath, wantPath)
	}
	reg, err := loadMountRegistry()
	if err != nil {
		t.Fatalf("loadMountRegistry() returned error: %v", err)
	}
	rec, ok := mountByPath(reg, wantPath)
	if !ok {
		t.Fatalf("mount registry missing %s: %+v", wantPath, reg.Mounts)
	}
	if rec.AgentWorkspace != "coding-agent" || rec.AgentWorkspaceRoot != filepath.Join(homeDir, "coding-agent") || rec.AgentWorkspacePath != "/repo" {
		t.Fatalf("agent workspace mount tags = %+v, want coding-agent root/path metadata", rec)
	}
}

func TestWorkspaceMountMissingManifestDoesNotPromptForLocalFolder(t *testing.T) {
	t.Helper()

	withTempHome(t)
	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.Mode = modeSync
	cfg.MountBackend = mountBackendNone
	saveTempConfig(t, cfg)

	input, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("CreateTemp(stdin) returned error: %v", err)
	}
	if _, err := input.WriteString("\n"); err != nil {
		t.Fatalf("WriteString(stdin) returned error: %v", err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatalf("Seek(stdin) returned error: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = input.Close()
	})

	out, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"ws", "mount", "missing-agent"})
	})
	if err == nil {
		t.Fatal("cmdWorkspace(mount missing) returned nil error, want missing manifest error")
	}
	if !strings.Contains(err.Error(), `Agent Workspace "missing-agent" does not exist`) {
		t.Fatalf("error = %q, want missing Agent Workspace message", err)
	}
	if strings.Contains(out, "Local folder") {
		t.Fatalf("output = %q, did not want local folder prompt before manifest lookup", out)
	}
}

func TestRootMountPromptsForAgentWorkspacesNotVolumes(t *testing.T) {
	t.Helper()

	homeDir := withTempHome(t)
	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.Mode = modeSync
	cfg.MountBackend = mountBackendNone
	saveTempConfig(t, cfg)

	if err := cmdVolume([]string{"vol", "create", "repo"}); err != nil {
		t.Fatalf("cmdVolume(create repo) returned error: %v", err)
	}
	if err := cmdVolume([]string{"vol", "create", "loose-volume"}); err != nil {
		t.Fatalf("cmdVolume(create loose-volume) returned error: %v", err)
	}
	if err := cmdWorkspace([]string{"ws", "create", "coding-agent"}); err != nil {
		t.Fatalf("cmdWorkspace(create) returned error: %v", err)
	}
	if err := cmdWorkspace([]string{"ws", "attach", "coding-agent", "repo", "--at", "/repo"}); err != nil {
		t.Fatalf("cmdWorkspace(attach) returned error: %v", err)
	}

	input, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("CreateTemp(stdin) returned error: %v", err)
	}
	if _, err := input.WriteString("1\n\n"); err != nil {
		t.Fatalf("WriteString(stdin) returned error: %v", err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatalf("Seek(stdin) returned error: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = input.Close()
	})

	origStartSync := startSyncMountForWorkspaceMount
	t.Cleanup(func() {
		startSyncMountForWorkspaceMount = origStartSync
	})
	startSyncMountForWorkspaceMount = func(ctx context.Context, cfg config, selection workspaceSelection, opts mountOptions) error {
		reg, err := loadMountRegistry()
		if err != nil {
			return err
		}
		upsertMount(&reg, mountRecord{
			ID:          "mnt_" + selection.ID,
			Workspace:   selection.Name,
			WorkspaceID: selection.ID,
			LocalPath:   cfg.LocalPath,
			Mode:        modeSync,
			RedisAddr:   cfg.RedisAddr,
			RedisDB:     cfg.RedisDB,
		})
		if err := saveMountRegistry(reg); err != nil {
			return err
		}
		return nil
	}

	out, err := captureStdout(t, func() error {
		return cmdRootMountArgs(nil)
	})
	if err != nil {
		t.Fatalf("cmdRootMountArgs(nil) returned error: %v", err)
	}
	for _, want := range []string{"Mount Agent Workspace", "Workspace to mount:", "coding-agent", "Local folder [~/coding-agent]:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "loose-volume") {
		t.Fatalf("root mount prompt listed raw volume:\n%s", out)
	}

	wantPath := filepath.Join(homeDir, "coding-agent", "repo")
	reg, err := loadMountRegistry()
	if err != nil {
		t.Fatalf("loadMountRegistry() returned error: %v", err)
	}
	rec, ok := mountByPath(reg, wantPath)
	if !ok {
		t.Fatalf("mount registry missing %s: %+v", wantPath, reg.Mounts)
	}
	if rec.AgentWorkspace != "coding-agent" {
		t.Fatalf("AgentWorkspace = %q, want coding-agent", rec.AgentWorkspace)
	}
}

func TestRootUnmountPromptsForMountedAgentWorkspacesNotVolumes(t *testing.T) {
	t.Helper()

	homeDir := withTempHome(t)
	workspaceRoot := filepath.Join(homeDir, "coding-agent")
	workspaceVolumePath := filepath.Join(workspaceRoot, "repo")
	directVolumePath := filepath.Join(homeDir, "loose-volume")
	if err := saveMountRegistry(mountRegistry{Mounts: []mountRecord{
		{
			ID:                 "mnt_repo",
			Workspace:          "repo",
			WorkspaceID:        "ws_repo",
			AgentWorkspace:     "coding-agent",
			AgentWorkspaceID:   "agt_coding",
			AgentWorkspaceRoot: workspaceRoot,
			AgentWorkspacePath: "/repo",
			LocalPath:          workspaceVolumePath,
			Mode:               modeSync,
		},
		{
			ID:          "mnt_loose",
			Workspace:   "loose-volume",
			WorkspaceID: "ws_loose",
			LocalPath:   directVolumePath,
			Mode:        modeSync,
		},
	}}); err != nil {
		t.Fatalf("saveMountRegistry() returned error: %v", err)
	}

	input, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("CreateTemp(stdin) returned error: %v", err)
	}
	if _, err := input.WriteString("1\n"); err != nil {
		t.Fatalf("WriteString(stdin) returned error: %v", err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatalf("Seek(stdin) returned error: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = input.Close()
	})

	out, err := captureStdout(t, func() error {
		return cmdRootUnmountArgs(nil)
	})
	if err != nil {
		t.Fatalf("cmdRootUnmountArgs(nil) returned error: %v", err)
	}
	for _, want := range []string{"Unmount Agent Workspace", "Workspace to unmount:", "coding-agent"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "loose-volume") {
		t.Fatalf("root unmount prompt listed raw volume:\n%s", out)
	}
	loaded, err := loadMountRegistry()
	if err != nil {
		t.Fatalf("loadMountRegistry() returned error: %v", err)
	}
	if _, ok := mountByPath(loaded, workspaceVolumePath); ok {
		t.Fatalf("workspace volume mount still registered at %s", workspaceVolumePath)
	}
	if _, ok := mountByPath(loaded, directVolumePath); !ok {
		t.Fatalf("direct volume mount was removed from %s", directVolumePath)
	}
}

func TestRootUnmountByAgentWorkspaceRemovesAllGroupedVolumeMounts(t *testing.T) {
	t.Helper()

	homeDir := withTempHome(t)
	workspaceRoot := filepath.Join(homeDir, "coding-agent")
	repoPath := filepath.Join(workspaceRoot, "repo")
	memoryPath := filepath.Join(workspaceRoot, "memory")
	directVolumePath := filepath.Join(homeDir, "loose-volume")
	if err := saveMountRegistry(mountRegistry{Mounts: []mountRecord{
		{
			ID:                 "mnt_repo",
			Workspace:          "repo",
			WorkspaceID:        "ws_repo",
			AgentWorkspace:     "coding-agent",
			AgentWorkspaceID:   "agt_coding",
			AgentWorkspaceRoot: workspaceRoot,
			AgentWorkspacePath: "/repo",
			LocalPath:          repoPath,
			Mode:               modeSync,
		},
		{
			ID:                 "mnt_memory",
			Workspace:          "memory",
			WorkspaceID:        "ws_memory",
			AgentWorkspace:     "coding-agent",
			AgentWorkspaceID:   "agt_coding",
			AgentWorkspaceRoot: workspaceRoot,
			AgentWorkspacePath: "/memory",
			LocalPath:          memoryPath,
			Mode:               modeSync,
		},
		{
			ID:          "mnt_loose",
			Workspace:   "loose-volume",
			WorkspaceID: "ws_loose",
			LocalPath:   directVolumePath,
			Mode:        modeSync,
		},
	}}); err != nil {
		t.Fatalf("saveMountRegistry() returned error: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return cmdRootUnmountArgs([]string{"coding-agent"})
	})
	if err != nil {
		t.Fatalf("cmdRootUnmountArgs(coding-agent) returned error: %v", err)
	}
	for _, want := range []string{"Agent Workspace unmounted", "workspace  coding-agent", "volumes    2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Volume unmounted") {
		t.Fatalf("output should not print per-volume unmounts:\n%s", out)
	}
	loaded, err := loadMountRegistry()
	if err != nil {
		t.Fatalf("loadMountRegistry() returned error: %v", err)
	}
	for _, removed := range []string{repoPath, memoryPath} {
		if _, ok := mountByPath(loaded, removed); ok {
			t.Fatalf("workspace volume mount still registered at %s", removed)
		}
	}
	if _, ok := mountByPath(loaded, directVolumePath); !ok {
		t.Fatalf("direct volume mount was removed from %s", directVolumePath)
	}
}
