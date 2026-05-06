package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestNormalizeMountBackendSupportsNone(t *testing.T) {
	t.Helper()

	got, err := normalizeMountBackend("none")
	if err != nil {
		t.Fatalf("normalizeMountBackend() returned error: %v", err)
	}
	if got != mountBackendNone {
		t.Fatalf("normalizeMountBackend() = %q, want %q", got, mountBackendNone)
	}
}

func TestResolveConfigPathsFilesystemOnlySkipsMountResolution(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.MountBackend = mountBackendNone
	cfg.LocalPath = "~/mypath"
	cfg.WorkRoot = t.TempDir()

	if err := resolveConfigPaths(&cfg); err != nil {
		t.Fatalf("resolveConfigPaths() returned error: %v", err)
	}
	if cfg.MountBackend != mountBackendNone {
		t.Fatalf("MountBackend = %q, want %q", cfg.MountBackend, mountBackendNone)
	}
	// LocalPath is kept even when backend=none (used for sync mode).
	if cfg.LocalPath == "" {
		t.Fatalf("LocalPath should be preserved when mountBackend=none; got empty")
	}
}

func TestRunSetupWizardFirstRunPromptsOnlyForMode(t *testing.T) {
	t.Helper()

	reader := bufio.NewReader(bytes.NewBufferString("2\n"))
	var output bytes.Buffer

	cfg, err := runSetupWizard(reader, &output, defaultConfig(), true)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if cfg.Mode != modeMount {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, modeMount)
	}

	got := output.String()
	for _, want := range []string{"Mode", "How should AFS expose the workspace locally?"} {
		if !strings.Contains(got, want) {
			t.Fatalf("setup output = %q, want substring %q", got, want)
		}
	}
	for _, forbidden := range []string{
		"Configuration Source",
		"Redis Connection",
		"Workspace",
		"Choose workspace",
		"Local Path",
		"Local path",
		"Choose local mount point",
		"Filesystem Mount",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("setup output unexpectedly contains %q:\n%s", forbidden, got)
		}
	}
}

func TestRunSetupWizardEditModePreservesWorkspaceAndLocalPath(t *testing.T) {
	t.Helper()

	existing := defaultConfig()
	existing.ProductMode = productModeCloud
	existing.URL = "https://afs.example.com"
	existing.DatabaseID = "afs-cloud"
	existing.CurrentWorkspace = "demo"
	existing.CurrentWorkspaceID = "ws_demo"
	existing.LocalPath = "/tmp/demo"
	existing.Mode = modeSync

	reader := bufio.NewReader(bytes.NewBufferString("2\n"))
	var output bytes.Buffer

	cfg, err := runSetupWizard(reader, &output, existing, false)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if cfg.Mode != modeMount {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, modeMount)
	}
	if cfg.CurrentWorkspace != "demo" {
		t.Fatalf("CurrentWorkspace = %q, want preserved demo", cfg.CurrentWorkspace)
	}
	if cfg.CurrentWorkspaceID != "ws_demo" {
		t.Fatalf("CurrentWorkspaceID = %q, want preserved ws_demo", cfg.CurrentWorkspaceID)
	}
	if cfg.LocalPath != "/tmp/demo" {
		t.Fatalf("LocalPath = %q, want preserved /tmp/demo", cfg.LocalPath)
	}

	got := output.String()
	for _, forbidden := range []string{
		"What would you like to change?",
		"Change current workspace",
		"Change Local Path",
		"Change Filesystem Mount",
		"Choose workspace",
		"Local path",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("setup output unexpectedly contains %q:\n%s", forbidden, got)
		}
	}
}

func TestCmdSetupDoesNotStartServices(t *testing.T) {
	t.Helper()

	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv(HOME) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})

	configFile := filepath.Join(t.TempDir(), "afs.config.json")
	origConfigPath := cfgPathOverride
	cfgPathOverride = configFile
	t.Cleanup(func() {
		cfgPathOverride = origConfigPath
	})

	stdinPath := filepath.Join(t.TempDir(), "setup-input.txt")
	if err := os.WriteFile(stdinPath, []byte("\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(stdin) returned error: %v", err)
	}
	stdinFile, err := os.Open(stdinPath)
	if err != nil {
		t.Fatalf("Open(stdin) returned error: %v", err)
	}
	defer stdinFile.Close()

	stdoutFile, err := os.CreateTemp(t.TempDir(), "setup-stdout-*.txt")
	if err != nil {
		t.Fatalf("CreateTemp(stdout) returned error: %v", err)
	}
	defer stdoutFile.Close()

	origStdin := os.Stdin
	origStdout := os.Stdout
	os.Stdin = stdinFile
	os.Stdout = stdoutFile
	t.Cleanup(func() {
		os.Stdin = origStdin
		os.Stdout = origStdout
	})

	if err := cmdSetup(); err != nil {
		t.Fatalf("cmdSetup() returned error: %v", err)
	}

	if _, statErr := os.Stat(configFile); statErr != nil {
		t.Fatalf("config file stat error = %v, want saved config", statErr)
	}

	if _, statErr := os.Stat(statePath()); statErr == nil {
		t.Fatal("cmdSetup() should not have written state (it should not start services)")
	} else if !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("state stat error = %v, want ErrNotExist", statErr)
	}

	if _, err := stdoutFile.Seek(0, 0); err != nil {
		t.Fatalf("Seek(stdout) returned error: %v", err)
	}
	outputBytes, err := os.ReadFile(stdoutFile.Name())
	if err != nil {
		t.Fatalf("ReadFile(stdout) returned error: %v", err)
	}
	output := string(outputBytes)
	if !strings.Contains(output, "Run ") || !strings.Contains(output, " ws mount") {
		t.Fatalf("cmdSetup() output should mention mounting a workspace afterward; got:\n%s", output)
	}
	if strings.Contains(output, "Choose workspace") || strings.Contains(output, "Local path") {
		t.Fatalf("cmdSetup() should not ask for workspace or local path; got:\n%s", output)
	}
}

func TestStartServicesFilesystemOnlyUsesRedisWithoutMountpoint(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv(HOME) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNone
	cfg.WorkRoot = filepath.Join(homeDir, ".afs", "workspaces")

	if err := resolveConfigPaths(&cfg); err != nil {
		t.Fatalf("resolveConfigPaths() returned error: %v", err)
	}
	if err := startServices(cfg); err != nil {
		t.Fatalf("startServices() returned error: %v", err)
	}

	st, err := loadState()
	if err != nil {
		t.Fatalf("loadState() returned error: %v", err)
	}
	if st.MountBackend != mountBackendNone {
		t.Fatalf("MountBackend = %q, want %q", st.MountBackend, mountBackendNone)
	}
	if st.LocalPath != "" {
		t.Fatalf("Mountpoint = %q, want empty", st.LocalPath)
	}
	if st.RedisAddr != mr.Addr() {
		t.Fatalf("RedisAddr = %q, want %q", st.RedisAddr, mr.Addr())
	}
}

func TestStartServicesRejectsMissingConfiguredWorkspaceForMountedFilesystem(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNFS
	cfg.NFSBin = "/usr/bin/true"
	cfg.CurrentWorkspace = "missing-workspace"
	cfg.LocalPath = filepath.Join(t.TempDir(), "mnt")

	if err := resolveConfigPaths(&cfg); err != nil {
		t.Fatalf("resolveConfigPaths() returned error: %v", err)
	}

	err := startServices(cfg)
	if err == nil {
		t.Fatal("startServices() returned nil error, want missing workspace error")
	}
	if !strings.Contains(err.Error(), `workspace "missing-workspace" does not exist`) {
		t.Fatalf("startServices() error = %q, want missing workspace message", err)
	}

	store := newAFSStore(mustRedisClient(t, cfg))
	defer func() { _ = store.rdb.Close() }()

	exists, existsErr := store.workspaceExists(context.Background(), "missing-workspace")
	if existsErr != nil {
		t.Fatalf("workspaceExists(missing-workspace) returned error: %v", existsErr)
	}
	if exists {
		t.Fatal("expected startServices() not to auto-create the missing workspace")
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}

func stringsJoinLines(lines ...string) string {
	var b bytes.Buffer
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
