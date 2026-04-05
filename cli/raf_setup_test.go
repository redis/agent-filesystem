package main

import (
	"bufio"
	"bytes"
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

	cfg, migrateDir, err := runSetupWizard(reader, ioDiscard{}, defaultConfig(), true)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if migrateDir != "" {
		t.Fatalf("migrateDir = %q, want empty", migrateDir)
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

	if _, _, err := runSetupWizard(reader, &output, defaultConfig(), true); err != nil {
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

	reader := bufio.NewReader(bytes.NewBufferString("2\n\n"))
	var output bytes.Buffer

	if _, _, err := runSetupWizard(reader, &output, existing, false); err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}

	got := output.String()
	if !strings.Contains(got, "Change Redis connection") || !strings.Contains(got, "redis.example:6379") {
		t.Fatalf("setup output = %q, want redis connection current setting", got)
	}
	if !strings.Contains(got, "Change filesystem mount") || !strings.Contains(got, "none") {
		t.Fatalf("setup output = %q, want filesystem mount current setting", got)
	}
	if strings.Contains(got, "Change current workspace") || strings.Contains(got, "Current Workspace") {
		t.Fatalf("setup output = %q, current workspace should not be editable in setup", got)
	}
}

func TestLoadConfigMigratesLegacyDefaultWorkspaceToCurrentWorkspace(t *testing.T) {
	t.Helper()

	configFile := filepath.Join(t.TempDir(), "raf.config.json")
	orig := cfgPathOverride
	cfgPathOverride = configFile
	t.Cleanup(func() {
		cfgPathOverride = orig
	})

	raw := `{"redisAddr":"localhost:6379","defaultWorkspace":"legacy-workspace"}`
	if err := os.WriteFile(configFile, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile(config) returned error: %v", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() returned error: %v", err)
	}
	if cfg.CurrentWorkspace != "legacy-workspace" {
		t.Fatalf("CurrentWorkspace = %q, want %q", cfg.CurrentWorkspace, "legacy-workspace")
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
		"2", // filesystem mount only
		"/tmp/raf-mount",
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))

	cfg, migrateDir, err := runSetupWizard(reader, ioDiscard{}, existing, false)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if migrateDir != "" {
		t.Fatalf("migrateDir = %q, want empty", migrateDir)
	}
	if cfg.MountBackend != defaultMountBackend() {
		t.Fatalf("MountBackend = %q, want %q", cfg.MountBackend, defaultMountBackend())
	}
	if cfg.Mountpoint != "/tmp/raf-mount" {
		t.Fatalf("Mountpoint = %q, want %q", cfg.Mountpoint, "/tmp/raf-mount")
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

func TestRunSetupWizardMountWithoutCurrentWorkspacePromptsForWorkspace(t *testing.T) {
	t.Helper()

	existing := defaultConfig()
	existing.UseExistingRedis = true
	existing.CurrentWorkspace = ""
	existing.MountBackend = mountBackendNone

	input := stringsJoinLines(
		"2",        // filesystem mount only
		"/tmp/raf", // requested mountpoint
		"newfiles", // workspace name to create/use
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))

	cfg, migrateDir, err := runSetupWizard(reader, ioDiscard{}, existing, false)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if migrateDir != "" {
		t.Fatalf("migrateDir = %q, want empty", migrateDir)
	}
	if cfg.CurrentWorkspace != "newfiles" {
		t.Fatalf("CurrentWorkspace = %q, want %q", cfg.CurrentWorkspace, "newfiles")
	}
	if cfg.Mountpoint != "/tmp/raf" {
		t.Fatalf("Mountpoint = %q, want %q", cfg.Mountpoint, "/tmp/raf")
	}
	if cfg.MountBackend != defaultMountBackend() {
		t.Fatalf("MountBackend = %q, want %q", cfg.MountBackend, defaultMountBackend())
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
	cfg.WorkRoot = filepath.Join(homeDir, ".raf", "workspaces")

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
