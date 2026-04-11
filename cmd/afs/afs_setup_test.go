package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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
	cfg.Mountpoint = "~/should-be-cleared"
	cfg.WorkRoot = t.TempDir()
	cfg.UseExistingRedis = true

	if err := resolveConfigPaths(&cfg); err != nil {
		t.Fatalf("resolveConfigPaths() returned error: %v", err)
	}
	if cfg.MountBackend != mountBackendNone {
		t.Fatalf("MountBackend = %q, want %q", cfg.MountBackend, mountBackendNone)
	}
	if cfg.Mountpoint != "" {
		t.Fatalf("Mountpoint = %q, want empty string", cfg.Mountpoint)
	}
}

func TestRunSetupWizardAllowsNoMountedFilesystem(t *testing.T) {
	t.Helper()

	input := stringsJoinLines(
		"2", // existing redis
		"",  // redis addr default
		"",  // redis username default
		"",  // redis password
		"",  // tls default
		"",  // no mounted filesystem
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))

	cfg, err := runSetupWizard(reader, ioDiscard{}, defaultConfig(), true)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if cfg.MountBackend != mountBackendNone {
		t.Fatalf("MountBackend = %q, want %q", cfg.MountBackend, mountBackendNone)
	}
	if cfg.Mountpoint != "" {
		t.Fatalf("Mountpoint = %q, want empty", cfg.Mountpoint)
	}
}

func TestRunSetupWizardNoMountSkipsLegacyFilesystemKeyPrompt(t *testing.T) {
	t.Helper()

	input := stringsJoinLines(
		"2", // existing redis
		"",  // redis addr default
		"",  // redis username default
		"",  // redis password
		"",  // tls default
		"",  // no mounted filesystem
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))
	var output bytes.Buffer

	if _, err := runSetupWizard(reader, &output, defaultConfig(), true); err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}

	got := output.String()
	if strings.Contains(got, "filesystem key") || strings.Contains(got, "What Redis key should the mounted filesystem use?") {
		t.Fatalf("no-mount setup output unexpectedly prompted for a filesystem key:\n%s", got)
	}
	if !strings.Contains(got, "Filesystem Mount") || !strings.Contains(got, "Choose local mount point") {
		t.Fatalf("no-mount setup output = %q, want local mount point prompt", got)
	}
	if strings.Contains(got, "Current Workspace") || strings.Contains(got, "current workspace") {
		t.Fatalf("no-mount setup output unexpectedly mentioned current workspace:\n%s", got)
	}
}

func TestRunSetupWizardExistingConfigShowsCurrentSettings(t *testing.T) {
	t.Helper()

	existing := defaultConfig()
	existing.UseExistingRedis = true
	existing.RedisAddr = "redis.example:6379"
	existing.CurrentWorkspace = "demo"
	existing.MountBackend = mountBackendNone
	existing.Mountpoint = ""

	// Menu (edit mode) → pick option 2 (filesystem mount) → keep as none →
	// back to menu → pick 4 to save and exit.
	reader := bufio.NewReader(bytes.NewBufferString(stringsJoinLines(
		"2",
		"",
		"4",
	)))
	var output bytes.Buffer

	if _, err := runSetupWizard(reader, &output, existing, false); err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}

	got := output.String()
	if !strings.Contains(got, "Change Redis connection") || !strings.Contains(got, "redis.example:6379") {
		t.Fatalf("setup output = %q, want redis connection current setting", got)
	}
	if !strings.Contains(got, "Change filesystem mount") || !strings.Contains(got, "none") {
		t.Fatalf("setup output = %q, want filesystem mount current setting", got)
	}
	if !strings.Contains(got, "Change current workspace") || !strings.Contains(got, "demo") {
		t.Fatalf("setup output = %q, want current workspace current setting", got)
	}
}

func TestRunSetupWizardAllowsChangingCurrentWorkspace(t *testing.T) {
	t.Helper()

	existing := defaultConfig()
	existing.UseExistingRedis = true
	existing.CurrentWorkspace = "demo"

	reader := bufio.NewReader(bytes.NewBufferString(stringsJoinLines(
		"3",
		"repo-two",
		"4",
	)))

	cfg, err := runSetupWizard(reader, ioDiscard{}, existing, false)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if cfg.CurrentWorkspace != "repo-two" {
		t.Fatalf("CurrentWorkspace = %q, want %q", cfg.CurrentWorkspace, "repo-two")
	}
}

func TestRunSetupWizardAllowsClearingCurrentWorkspace(t *testing.T) {
	t.Helper()

	existing := defaultConfig()
	existing.UseExistingRedis = true
	existing.CurrentWorkspace = "demo"

	reader := bufio.NewReader(bytes.NewBufferString(stringsJoinLines(
		"3",
		"none",
		"4",
	)))

	cfg, err := runSetupWizard(reader, ioDiscard{}, existing, false)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if cfg.CurrentWorkspace != "" {
		t.Fatalf("CurrentWorkspace = %q, want empty", cfg.CurrentWorkspace)
	}
}

func TestRunSetupWizardAutoSelectsMountBackendAndUsesCurrentWorkspace(t *testing.T) {
	t.Helper()

	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen(127.0.0.1:0) returned error: %v", err)
	}
	defer busy.Close()
	busyAddr, ok := busy.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("busy listener addr type = %T, want *net.TCPAddr", busy.Addr())
	}

	existing := defaultConfig()
	existing.UseExistingRedis = true
	existing.CurrentWorkspace = "repo"
	existing.MountBackend = mountBackendNone
	existing.Mountpoint = ""
	existing.NFSPort = busyAddr.Port

	input := stringsJoinLines(
		"2", // filesystem mount
		"/tmp/afs-mount",
		"4", // save and exit
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))

	cfg, err := runSetupWizard(reader, ioDiscard{}, existing, false)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if cfg.MountBackend != defaultMountBackend() {
		t.Fatalf("MountBackend = %q, want %q", cfg.MountBackend, defaultMountBackend())
	}
	if cfg.Mountpoint != "/tmp/afs-mount" {
		t.Fatalf("Mountpoint = %q, want %q", cfg.Mountpoint, "/tmp/afs-mount")
	}
	if runtime.GOOS == "darwin" {
		if cfg.NFSHost != "127.0.0.1" {
			t.Fatalf("NFSHost = %q, want %q", cfg.NFSHost, "127.0.0.1")
		}
		if cfg.NFSPort == busyAddr.Port {
			t.Fatalf("NFSPort = %d, want a free port instead of the busy port", cfg.NFSPort)
		}
		if !tcpAddressAvailable(cfg.NFSHost, cfg.NFSPort) {
			t.Fatalf("suggested NFS port %s:%s should be available", cfg.NFSHost, strconv.Itoa(cfg.NFSPort))
		}
	}
}

func TestRunSetupWizardEditModeLoopsUntilSaveAndExit(t *testing.T) {
	t.Helper()

	existing := defaultConfig()
	existing.UseExistingRedis = true
	existing.RedisAddr = "redis.example:6379"
	existing.CurrentWorkspace = "demo"
	existing.MountBackend = mountBackendNone

	// Pick option 3 (workspace) → set it → pick 3 again → clear → finally 4.
	input := stringsJoinLines(
		"3", "repo-two",
		"3", "none",
		"4",
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))
	var output bytes.Buffer

	cfg, err := runSetupWizard(reader, &output, existing, false)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if cfg.CurrentWorkspace != "" {
		t.Fatalf("CurrentWorkspace = %q, want empty after second edit", cfg.CurrentWorkspace)
	}
	if strings.Count(output.String(), "What would you like to change?") < 3 {
		t.Fatalf("menu should have been shown 3 times (two edits + final exit), got:\n%s", output.String())
	}
}

func TestRunSetupWizardEditModeUnknownChoiceReprompts(t *testing.T) {
	t.Helper()

	existing := defaultConfig()
	existing.UseExistingRedis = true
	existing.CurrentWorkspace = "demo"

	// Unknown choice "9" → menu re-displayed → pick 4 to exit.
	input := stringsJoinLines("9", "4")
	reader := bufio.NewReader(bytes.NewBufferString(input))
	var output bytes.Buffer

	if _, err := runSetupWizard(reader, &output, existing, false); err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if !strings.Contains(output.String(), "Unknown choice") {
		t.Fatalf("expected unknown-choice warning, got:\n%s", output.String())
	}
	if strings.Count(output.String(), "What would you like to change?") < 2 {
		t.Fatalf("menu should have been shown at least twice after unknown choice")
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
	input := stringsJoinLines(
		"2", // existing Redis
		"",  // default addr
		"",  // default user
		"",  // no password
		"",  // no tls
		"",  // no mounted filesystem
	)
	if err := os.WriteFile(stdinPath, []byte(input), 0o644); err != nil {
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

	// cmdSetup must not have produced any running-state file.
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
	if !strings.Contains(output, "Run ") || !strings.Contains(output, " up") {
		t.Fatalf("cmdSetup() output should mention running 'afs up' afterward; got:\n%s", output)
	}
}

func TestRunSetupWizardMountWithoutCurrentWorkspacePromptsForWorkspace(t *testing.T) {
	t.Helper()

	existing := defaultConfig()
	existing.UseExistingRedis = true
	existing.CurrentWorkspace = ""
	existing.MountBackend = mountBackendNone

	input := stringsJoinLines(
		"2",        // filesystem mount
		"/tmp/afs", // requested mountpoint
		"newfiles", // workspace name to create/use
		"4",        // save and exit
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))

	cfg, err := runSetupWizard(reader, ioDiscard{}, existing, false)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if cfg.CurrentWorkspace != "newfiles" {
		t.Fatalf("CurrentWorkspace = %q, want %q", cfg.CurrentWorkspace, "newfiles")
	}
	if cfg.Mountpoint != "/tmp/afs" {
		t.Fatalf("Mountpoint = %q, want %q", cfg.Mountpoint, "/tmp/afs")
	}
	if cfg.MountBackend != defaultMountBackend() {
		t.Fatalf("MountBackend = %q, want %q", cfg.MountBackend, defaultMountBackend())
	}
}

func TestPromptLocalFilesystemSetupRejectsExistingFileMountpoint(t *testing.T) {
	t.Helper()

	mountpointFile := filepath.Join(t.TempDir(), "afs")
	if err := os.WriteFile(mountpointFile, []byte("binary"), 0o755); err != nil {
		t.Fatalf("WriteFile(mountpoint) returned error: %v", err)
	}

	cfg := defaultConfig()
	cfg.UseExistingRedis = true
	cfg.CurrentWorkspace = "demo"
	cfg.MountBackend = mountBackendNone
	cfg.Mountpoint = ""

	reader := bufio.NewReader(bytes.NewBufferString(stringsJoinLines(mountpointFile)))
	err := promptLocalFilesystemSetup(reader, ioDiscard{}, &cfg, false)
	if err == nil {
		t.Fatal("promptLocalFilesystemSetup() returned nil error, want existing-file mountpoint rejection")
	}
	if !strings.Contains(err.Error(), "exists and is not a directory") {
		t.Fatalf("promptLocalFilesystemSetup() error = %q, want non-directory message", err)
	}
	if cfg.MountBackend != mountBackendNone {
		t.Fatalf("MountBackend = %q, want %q after rejected mountpoint", cfg.MountBackend, mountBackendNone)
	}
	if cfg.Mountpoint != "" {
		t.Fatalf("Mountpoint = %q, want empty after rejected mountpoint", cfg.Mountpoint)
	}
}

func TestCmdSetupRejectsExistingFileMountpointBeforeSavingConfig(t *testing.T) {
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

	mountpointFile := filepath.Join(t.TempDir(), "afs")
	if err := os.WriteFile(mountpointFile, []byte("binary"), 0o755); err != nil {
		t.Fatalf("WriteFile(mountpoint) returned error: %v", err)
	}

	stdinPath := filepath.Join(t.TempDir(), "setup-input.txt")
	input := stringsJoinLines(
		"2",            // existing redis
		"",             // redis addr default
		"",             // redis username default
		"",             // redis password
		"",             // tls default
		mountpointFile, // invalid mountpoint
	)
	if err := os.WriteFile(stdinPath, []byte(input), 0o644); err != nil {
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

	err = cmdSetup()
	if err == nil {
		t.Fatal("cmdSetup() returned nil error, want existing-file mountpoint rejection")
	}
	if !strings.Contains(err.Error(), "exists and is not a directory") {
		t.Fatalf("cmdSetup() error = %q, want non-directory message", err)
	}
	if _, statErr := os.Stat(configFile); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("config file stat error = %v, want not-exist after rejected setup", statErr)
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
	cfg.UseExistingRedis = true
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
	if st.Mountpoint != "" {
		t.Fatalf("Mountpoint = %q, want empty", st.Mountpoint)
	}
	if st.RedisAddr != mr.Addr() {
		t.Fatalf("RedisAddr = %q, want %q", st.RedisAddr, mr.Addr())
	}
}

func TestStartServicesRejectsMissingConfiguredWorkspaceForMountedFilesystem(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.UseExistingRedis = true
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNFS
	cfg.NFSBin = "/usr/bin/true"
	cfg.CurrentWorkspace = "missing-workspace"
	cfg.Mountpoint = filepath.Join(t.TempDir(), "mnt")

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
