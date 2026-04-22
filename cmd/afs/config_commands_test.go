package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestCmdConfigSetPersistsNonInteractiveSettings(t *testing.T) {
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

	err := cmdConfig([]string{
		"config", "set",
		"--redis-url", "rediss://alice:secret@127.0.0.1:6380/4",
		"--mount-backend", "nfs",
		"--mountpoint", "~/mounted-demo",
	})
	if err != nil {
		t.Fatalf("cmdConfig(set) returned error: %v", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() returned error: %v", err)
	}
	if cfg.RedisAddr != "127.0.0.1:6380" {
		t.Fatalf("RedisAddr = %q, want %q", cfg.RedisAddr, "127.0.0.1:6380")
	}
	if cfg.RedisDB != 4 {
		t.Fatalf("RedisDB = %d, want %d", cfg.RedisDB, 4)
	}
	if !cfg.RedisTLS {
		t.Fatal("RedisTLS = false, want true")
	}
	if cfg.RedisUsername != "alice" {
		t.Fatalf("RedisUsername = %q, want %q", cfg.RedisUsername, "alice")
	}
	if cfg.RedisPassword != "secret" {
		t.Fatalf("RedisPassword = %q, want %q", cfg.RedisPassword, "secret")
	}
	if cfg.MountBackend != mountBackendNFS {
		t.Fatalf("MountBackend = %q, want %q", cfg.MountBackend, mountBackendNFS)
	}
	wantMountpoint := filepath.Join(homeDir, "mounted-demo")
	if cfg.LocalPath != wantMountpoint {
		t.Fatalf("Mountpoint = %q, want %q", cfg.LocalPath, wantMountpoint)
	}
}

func TestCmdConfigShowJSONIncludesConfiguredFields(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.RedisAddr = "redis.example:6380"
	cfg.RedisDB = 7
	cfg.CurrentWorkspace = "demo"
	cfg.MountBackend = mountBackendNone
	saveTempConfig(t, cfg)

	out, err := captureStdout(t, func() error {
		return cmdConfig([]string{"config", "show", "--json"})
	})
	if err != nil {
		t.Fatalf("cmdConfig(show --json) returned error: %v", err)
	}

	var got config
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("Unmarshal(config show json) returned error: %v", err)
	}
	if got.RedisAddr != "redis.example:6380" {
		t.Fatalf("RedisAddr = %q, want %q", got.RedisAddr, "redis.example:6380")
	}
	if got.RedisDB != 7 {
		t.Fatalf("RedisDB = %d, want %d", got.RedisDB, 7)
	}
	if got.CurrentWorkspace != "demo" {
		t.Fatalf("CurrentWorkspace = %q, want %q", got.CurrentWorkspace, "demo")
	}
	if got.MountBackend != mountBackendNone {
		t.Fatalf("MountBackend = %q, want %q", got.MountBackend, mountBackendNone)
	}
}

func TestCmdConfigSetAgentNamePersistsFriendlyAgentName(t *testing.T) {
	t.Helper()

	configFile := filepath.Join(t.TempDir(), "afs.config.json")
	origConfigPath := cfgPathOverride
	cfgPathOverride = configFile
	t.Cleanup(func() {
		cfgPathOverride = origConfigPath
	})

	if err := cmdConfig([]string{"config", "set", "agent.name", "Claude Code"}); err != nil {
		t.Fatalf("cmdConfig(set agent.name) returned error: %v", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() returned error: %v", err)
	}
	if cfg.Name != "Claude Code" {
		t.Fatalf("agent name = %q, want %q", cfg.Name, "Claude Code")
	}

	value, err := getConfigKey(cfg, "agent.name")
	if err != nil {
		t.Fatalf("getConfigKey(agent.name) returned error: %v", err)
	}
	if value != "Claude Code" {
		t.Fatalf("getConfigKey(agent.name) = %q, want %q", value, "Claude Code")
	}
}

func TestCmdConfigSetPersistsSelfHostedControlPlaneSettings(t *testing.T) {
	t.Helper()

	configFile := filepath.Join(t.TempDir(), "afs.config.json")
	origConfigPath := cfgPathOverride
	cfgPathOverride = configFile
	t.Cleanup(func() {
		cfgPathOverride = origConfigPath
	})

	err := cmdConfig([]string{
		"config", "set",
		"--connection", "self-hosted",
		"--control-plane-url", "http://127.0.0.1:8091/",
		"--control-plane-database", "db-local",
	})
	if err != nil {
		t.Fatalf("cmdConfig(set self-hosted) returned error: %v", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() returned error: %v", err)
	}
	if cfg.ProductMode != productModeSelfHosted {
		t.Fatalf("ProductMode = %q, want %q", cfg.ProductMode, productModeSelfHosted)
	}
	if cfg.URL != "http://127.0.0.1:8091" {
		t.Fatalf("controlPlane.url = %q, want %q", cfg.URL, "http://127.0.0.1:8091")
	}
	if cfg.DatabaseID != "db-local" {
		t.Fatalf("controlPlane.databaseID = %q, want %q", cfg.DatabaseID, "db-local")
	}
}

func TestCmdConfigSetControlPlaneURLClearsStaleScopedSelection(t *testing.T) {
	t.Helper()

	base := defaultConfig()
	base.ProductMode = productModeSelfHosted
	base.URL = "http://old.example:8091"
	base.DatabaseID = "redis-cloud"
	base.CurrentWorkspace = "codex"
	base.CurrentWorkspaceID = "ws_old"
	saveTempConfig(t, base)

	err := cmdConfig([]string{
		"config", "set",
		"--control-plane-url", "http://127.0.0.1:8091/",
	})
	if err != nil {
		t.Fatalf("cmdConfig(set --control-plane-url) returned error: %v", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() returned error: %v", err)
	}
	if cfg.URL != "http://127.0.0.1:8091" {
		t.Fatalf("controlPlane.url = %q, want %q", cfg.URL, "http://127.0.0.1:8091")
	}
	if cfg.DatabaseID != "" {
		t.Fatalf("controlPlane.databaseID = %q, want empty for auto-selection", cfg.DatabaseID)
	}
	if cfg.CurrentWorkspace != "codex" {
		t.Fatalf("CurrentWorkspace = %q, want %q", cfg.CurrentWorkspace, "codex")
	}
	if cfg.CurrentWorkspaceID != "" {
		t.Fatalf("CurrentWorkspaceID = %q, want cleared when switching control planes", cfg.CurrentWorkspaceID)
	}
}

func TestLoadConfigForUpWithOverridesDoesNotRequireSavedConfig(t *testing.T) {
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

	overrides := configOverrides{}
	overrides.controlPlaneURL = optionalString{value: "http://127.0.0.1:8091", set: true}

	cfg, err := loadConfigForUpWithIOAndOverridesAndMode(
		[]string{"getting-started"},
		overrides,
		optionalString{},
		bufio.NewReader(bytes.NewBuffer(nil)),
		&bytes.Buffer{},
		false,
	)
	if err != nil {
		t.Fatalf("loadConfigForUpWithIOAndOverridesAndMode() returned error: %v", err)
	}
	if cfg.ProductMode != productModeSelfHosted {
		t.Fatalf("ProductMode = %q, want %q", cfg.ProductMode, productModeSelfHosted)
	}
	if cfg.URL != "http://127.0.0.1:8091" {
		t.Fatalf("controlPlane.url = %q, want %q", cfg.URL, "http://127.0.0.1:8091")
	}
	if cfg.DatabaseID != "" {
		t.Fatalf("controlPlane.databaseID = %q, want empty so the control plane can resolve the workspace database", cfg.DatabaseID)
	}
	if cfg.CurrentWorkspace != "getting-started" {
		t.Fatalf("CurrentWorkspace = %q, want %q", cfg.CurrentWorkspace, "getting-started")
	}
	if cfg.CurrentWorkspaceID != "" {
		t.Fatalf("CurrentWorkspaceID = %q, want empty for explicit up workspace", cfg.CurrentWorkspaceID)
	}
	if !strings.HasSuffix(cfg.LocalPath, filepath.Join("afs", "getting-started")) {
		t.Fatalf("LocalPath = %q, want suffix %q", cfg.LocalPath, filepath.Join("afs", "getting-started"))
	}
	if _, err := os.Stat(configFile); !os.IsNotExist(err) {
		t.Fatalf("config file stat error = %v, want ErrNotExist because one-shot up overrides should not write config", err)
	}
}

func TestLoadConfigForUpAppliesWorkspaceAndMountpointAndSavesConfig(t *testing.T) {
	t.Helper()

	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv(HOME) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})

	base := defaultConfig()
	base.RedisAddr = "127.0.0.1:6379"
	base.RedisDB = 0
	base.CurrentWorkspace = "alpha"
	base.MountBackend = mountBackendNFS
	base.NFSBin = "/usr/bin/true"
	saveTempConfig(t, base)

	cfg, err := loadConfigForUp([]string{"beta", "~/override"})
	if err != nil {
		t.Fatalf("loadConfigForUp() returned error: %v", err)
	}
	if cfg.CurrentWorkspace != "beta" {
		t.Fatalf("CurrentWorkspace = %q, want %q", cfg.CurrentWorkspace, "beta")
	}
	if cfg.MountBackend != mountBackendNFS {
		t.Fatalf("MountBackend = %q, want %q", cfg.MountBackend, mountBackendNFS)
	}
	wantMountpoint := filepath.Join(homeDir, "override")
	if cfg.LocalPath != wantMountpoint {
		t.Fatalf("Mountpoint = %q, want %q", cfg.LocalPath, wantMountpoint)
	}

	saved, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig(saved) returned error: %v", err)
	}
	if saved.RedisDB != 0 {
		t.Fatalf("saved RedisDB = %d, want %d", saved.RedisDB, 0)
	}
	if saved.CurrentWorkspace != "beta" {
		t.Fatalf("saved CurrentWorkspace = %q, want %q", saved.CurrentWorkspace, "beta")
	}
	if saved.LocalPath != wantMountpoint {
		t.Fatalf("saved Mountpoint = %q, want %q", saved.LocalPath, wantMountpoint)
	}
	if saved.MountBackend != mountBackendNFS {
		t.Fatalf("saved MountBackend = %q, want %q", saved.MountBackend, mountBackendNFS)
	}
}

func TestLoadConfigForUpRejectsExistingFileMountpointWithoutSavingConfig(t *testing.T) {
	t.Helper()

	configFile := filepath.Join(t.TempDir(), "afs.config.json")
	origConfigPath := cfgPathOverride
	cfgPathOverride = configFile
	t.Cleanup(func() {
		cfgPathOverride = origConfigPath
	})

	base := defaultConfig()
	base.RedisAddr = "127.0.0.1:6379"
	base.RedisDB = 0
	base.CurrentWorkspace = "alpha"
	base.MountBackend = mountBackendNFS
	base.LocalPath = filepath.Join(t.TempDir(), "valid-mountpoint")
	base.NFSBin = "/usr/bin/true"
	if err := saveConfig(base); err != nil {
		t.Fatalf("saveConfig(base) returned error: %v", err)
	}

	mountpointFile := filepath.Join(t.TempDir(), "afs")
	if err := os.WriteFile(mountpointFile, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(mountpoint) returned error: %v", err)
	}

	_, err := loadConfigForUpWithIO([]string{"beta", mountpointFile}, bufio.NewReader(bytes.NewBuffer(nil)), &bytes.Buffer{}, false)
	if err == nil {
		t.Fatal("loadConfigForUpWithIO() returned nil error, want existing-file mountpoint rejection")
	}
	if !strings.Contains(err.Error(), "exists and is not a directory") {
		t.Fatalf("loadConfigForUpWithIO() error = %q, want non-directory message", err)
	}

	saved, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig(saved) returned error: %v", err)
	}
	if saved.CurrentWorkspace != base.CurrentWorkspace {
		t.Fatalf("saved CurrentWorkspace = %q, want %q after rejected mountpoint", saved.CurrentWorkspace, base.CurrentWorkspace)
	}
	if saved.LocalPath != base.LocalPath {
		t.Fatalf("saved Mountpoint = %q, want %q after rejected mountpoint", saved.LocalPath, base.LocalPath)
	}
	if saved.MountBackend != base.MountBackend {
		t.Fatalf("saved MountBackend = %q, want %q after rejected mountpoint", saved.MountBackend, base.MountBackend)
	}
}

func TestLoadConfigForUpWithoutConfigSuggestsSetup(t *testing.T) {
	t.Helper()

	configFile := filepath.Join(t.TempDir(), "afs.config.json")
	origConfigPath := cfgPathOverride
	cfgPathOverride = configFile
	t.Cleanup(func() {
		cfgPathOverride = origConfigPath
	})

	_, err := loadConfigForUpWithIO([]string{}, bufio.NewReader(bytes.NewBuffer(nil)), &bytes.Buffer{}, true)
	if err == nil {
		t.Fatal("loadConfigForUpWithIO() returned nil error, want missing config guidance")
	}

	if !strings.Contains(err.Error(), "no configuration found") {
		t.Fatalf("loadConfigForUpWithIO() error = %q, want missing config message", err)
	}

	want := "Run '" + filepath.Base(os.Args[0]) + " setup' to get started"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("loadConfigForUpWithIO() error = %q, want setup guidance %q", err, want)
	}
}

func TestLoadConfigForUpPromptsForMissingDatabaseAndMountpoint(t *testing.T) {
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

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), DB: 7})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.RedisDB = 7
	if err := createEmptyWorkspace(context.Background(), cfg, newAFSStore(rdb), "demo"); err != nil {
		t.Fatalf("createEmptyWorkspace() returned error: %v", err)
	}

	raw := `{
  "redis": {
    "addr": "` + mr.Addr() + `"
  },
  "currentWorkspace": "demo",
  "mount": {
    "backend": "nfs",
    "nfsBin": "/usr/bin/true"
  }
}`
	if err := os.WriteFile(configFile, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile(config) returned error: %v", err)
	}

	var output bytes.Buffer
	got, err := loadConfigForUpWithIO(
		[]string{},
		bufio.NewReader(bytes.NewBufferString(stringsJoinLines("7", "/tmp/afs-demo"))),
		&output,
		true,
	)
	if err != nil {
		t.Fatalf("loadConfigForUpWithIO() returned error: %v", err)
	}

	if got.RedisDB != 7 {
		t.Fatalf("RedisDB = %d, want %d", got.RedisDB, 7)
	}
	if got.CurrentWorkspace != "demo" {
		t.Fatalf("CurrentWorkspace = %q, want %q", got.CurrentWorkspace, "demo")
	}
	if got.LocalPath != "/tmp/afs-demo" {
		t.Fatalf("Mountpoint = %q, want %q", got.LocalPath, "/tmp/afs-demo")
	}
	if strings.Contains(output.String(), "Available workspace: demo") {
		t.Fatalf("prompt output = %q, want no workspace prompt", output.String())
	}

	saved, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig(saved) returned error: %v", err)
	}
	if saved.RedisDB != 7 {
		t.Fatalf("saved RedisDB = %d, want %d", saved.RedisDB, 7)
	}
	if saved.CurrentWorkspace != "demo" {
		t.Fatalf("saved CurrentWorkspace = %q, want %q", saved.CurrentWorkspace, "demo")
	}
	if saved.LocalPath != "/tmp/afs-demo" {
		t.Fatalf("saved Mountpoint = %q, want %q", saved.LocalPath, "/tmp/afs-demo")
	}
}

func TestLoadConfigForUpRejectsMissingWorkspaceEvenWhenPromptingAllowed(t *testing.T) {
	t.Helper()

	configFile := filepath.Join(t.TempDir(), "afs.config.json")
	origConfigPath := cfgPathOverride
	cfgPathOverride = configFile
	t.Cleanup(func() {
		cfgPathOverride = origConfigPath
	})

	raw := `{
  "redis": {
    "addr": "127.0.0.1:6379"
  },
  "mount": {
    "backend": "nfs",
    "nfsBin": "/usr/bin/true"
  }
}`
	if err := os.WriteFile(configFile, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile(config) returned error: %v", err)
	}

	_, err := loadConfigForUpWithIO(
		[]string{},
		bufio.NewReader(bytes.NewBufferString(stringsJoinLines("7", "/tmp/afs-demo"))),
		&bytes.Buffer{},
		true,
	)
	if err == nil {
		t.Fatal("loadConfigForUpWithIO() returned nil error, want missing workspace error")
	}
	if !strings.Contains(err.Error(), "no current workspace is selected for 'up'") {
		t.Fatalf("loadConfigForUpWithIO() error = %q, want missing workspace message", err)
	}
	if !strings.Contains(err.Error(), "workspace use <workspace>") {
		t.Fatalf("loadConfigForUpWithIO() error = %q, want workspace selection guidance", err)
	}
}

func TestCmdConfigHelpListsConfigurableSettings(t *testing.T) {
	t.Helper()

	out, err := captureStderr(t, func() error {
		return cmdConfig([]string{"config", "--help"})
	})
	if err != nil {
		t.Fatalf("cmdConfig(--help) returned error: %v", err)
	}

	for _, want := range []string{
		"Redis connection",
		"Configuration source",
		"--config-source",
		"--redis-url",
		"--mount-backend",
		"--mountpoint",
		"workspace use <workspace>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("config help output = %q, want substring %q", out, want)
		}
	}
}

func TestCmdConfigSetHelpListsDetailedFlags(t *testing.T) {
	t.Helper()

	out, err := captureStderr(t, func() error {
		return cmdConfig([]string{"config", "set", "--help"})
	})
	if err != nil {
		t.Fatalf("cmdConfig(set --help) returned error: %v", err)
	}

	for _, want := range []string{
		"--redis-url <redis://...|rediss://...>",
		"--config-source local|self-hosted|cloud",
		"--mount-backend auto|none|fuse|nfs",
		"Current workspace is not configured here",
		"runtime paths stay available in afs.config.json",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("config set help output = %q, want substring %q", out, want)
		}
	}
}

func TestLoadConfigForUpAcceptsSinglePositionalArgument(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.Mode = modeSync
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = "http://127.0.0.1:8091"
	cfg.DatabaseID = "local-development"
	cfg.CurrentWorkspaceID = "ws_old"
	saveTempConfig(t, cfg)

	result, err := loadConfigForUp([]string{"my-workspace"})
	if err != nil {
		t.Fatalf("loadConfigForUp() returned error: %v", err)
	}
	if result.CurrentWorkspace != "my-workspace" {
		t.Fatalf("CurrentWorkspace = %q, want %q", result.CurrentWorkspace, "my-workspace")
	}
	// The mountpoint should be auto-derived under ~/afs/<workspace>.
	if !strings.HasSuffix(result.LocalPath, filepath.Join("afs", "my-workspace")) {
		t.Fatalf("LocalPath = %q, want suffix %q", result.LocalPath, filepath.Join("afs", "my-workspace"))
	}
	if result.CurrentWorkspaceID != "" {
		t.Fatalf("CurrentWorkspaceID = %q, want cleared for positional workspace override", result.CurrentWorkspaceID)
	}
}

func TestLoadConfigForUpRejectsMountOverrideWhenMountsAreDisabledInConfig(t *testing.T) {
	t.Helper()

	base := defaultConfig()
	base.Mode = modeMount
	base.MountBackend = mountBackendNone
	saveTempConfig(t, base)

	_, err := loadConfigForUp([]string{"claude-code", "~/claude"})
	if err == nil {
		t.Fatal("loadConfigForUp() returned nil error, want disabled mount backend error")
	}
	if !strings.Contains(err.Error(), "filesystem mounts are disabled in config") {
		t.Fatalf("loadConfigForUp() error = %q, want disabled mount backend message", err)
	}
}

func TestLoadConfigForUpAllowsLocalPathOverrideInSyncModeWhenMountsDisabled(t *testing.T) {
	t.Helper()

	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv(HOME) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})

	base := defaultConfig()
	base.Mode = modeSync
	base.MountBackend = mountBackendNone
	base.CurrentWorkspace = "alpha"
	saveTempConfig(t, base)

	cfg, err := loadConfigForUp([]string{"beta", "~/claude"})
	if err != nil {
		t.Fatalf("loadConfigForUp() returned error: %v", err)
	}
	if cfg.CurrentWorkspace != "beta" {
		t.Fatalf("CurrentWorkspace = %q, want %q", cfg.CurrentWorkspace, "beta")
	}
	wantLocalPath := filepath.Join(homeDir, "claude")
	if cfg.LocalPath != wantLocalPath {
		t.Fatalf("LocalPath = %q, want %q", cfg.LocalPath, wantLocalPath)
	}
	if cfg.Mode != modeSync {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, modeSync)
	}
}

func TestLoadConfigForUpAppliesModeOverrideAndSavesConfig(t *testing.T) {
	t.Helper()

	base := defaultConfig()
	base.Mode = modeSync
	base.CurrentWorkspace = "alpha"
	base.LocalPath = t.TempDir()
	base.MountBackend = mountBackendNFS
	base.NFSBin = "/usr/bin/true"
	saveTempConfig(t, base)

	mode := optionalString{value: modeMount, set: true}
	cfg, err := loadConfigForUpWithMode([]string{}, mode)
	if err != nil {
		t.Fatalf("loadConfigForUpWithMode() returned error: %v", err)
	}
	if cfg.Mode != modeMount {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, modeMount)
	}

	saved, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig(saved) returned error: %v", err)
	}
	if saved.Mode != modeMount {
		t.Fatalf("saved Mode = %q, want %q", saved.Mode, modeMount)
	}
}

func TestCmdUpHelpListsPositionalOverrides(t *testing.T) {
	t.Helper()

	out, err := captureStderr(t, func() error {
		return cmdUpArgs([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("cmdUpArgs(--help) returned error: %v", err)
	}

	for _, want := range []string{
		"up <workspace> [<mountpoint>]",
		"--mode <sync|mount>",
		"Redis connection, mount backend, and readonly mode come from config",
		"Current workspace must already be selected",
		"If Redis DB or mountpoint are missing",
		"config set",
		"up --mode sync",
		"up claude-code ~/.claude",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("up help output = %q, want substring %q", out, want)
		}
	}
}

func TestLoadConfigForUpRejectsUnsupportedModeOverride(t *testing.T) {
	t.Helper()

	base := defaultConfig()
	base.CurrentWorkspace = "alpha"
	base.LocalPath = t.TempDir()
	saveTempConfig(t, base)

	mode := optionalString{value: modeNone, set: true}
	_, err := loadConfigForUpWithMode([]string{}, mode)
	if err == nil {
		t.Fatal("loadConfigForUpWithMode() returned nil error, want unsupported mode error")
	}
	if !strings.Contains(err.Error(), `expected sync or mount`) {
		t.Fatalf("loadConfigForUpWithMode() error = %q, want sync-or-mount guidance", err)
	}
}

func TestCmdWorkspaceHelpListsSubcommands(t *testing.T) {
	t.Helper()

	out, err := captureStderr(t, func() error {
		return cmdWorkspace([]string{"workspace", "--help"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(--help) returned error: %v", err)
	}

	for _, want := range []string{
		"workspace <subcommand>",
		"current",
		"use <workspace>",
		"clone [workspace] <directory>",
		"fork [source-workspace] <new-workspace>",
		"import [--force] [--mount-at-source] <workspace> <directory>",
		"workspace create demo",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("workspace help output = %q, want substring %q", out, want)
		}
	}
	if strings.Contains(out, "run [workspace]") {
		t.Fatalf("workspace help output = %q, did not expect removed run subcommand", out)
	}
}

func TestCmdWorkspaceRunReportsRemovedCommand(t *testing.T) {
	t.Helper()

	err := cmdWorkspace([]string{"workspace", "run", "--help"})
	if err == nil {
		t.Fatal("cmdWorkspace(run --help) returned nil error, want removed-command error")
	}
	for _, want := range []string{
		`unknown workspace subcommand "run"`,
		"workspace <subcommand>",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("cmdWorkspace(run --help) error = %q, want substring %q", err, want)
		}
	}
}

func TestCmdWorkspaceUseAndCurrentManageSelectionOutsideConfigCommand(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNone
	saveTempConfig(t, cfg)

	loadedCfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()
	if err := createEmptyWorkspace(context.Background(), loadedCfg, store, "demo"); err != nil {
		t.Fatalf("createEmptyWorkspace(demo) returned error: %v", err)
	}

	if err := cmdWorkspace([]string{"workspace", "use", "demo"}); err != nil {
		t.Fatalf("cmdWorkspace(use) returned error: %v", err)
	}

	saved, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() returned error: %v", err)
	}
	if saved.CurrentWorkspace != "demo" {
		t.Fatalf("CurrentWorkspace = %q, want %q", saved.CurrentWorkspace, "demo")
	}

	out, err := captureStdout(t, func() error {
		return cmdWorkspace([]string{"workspace", "current"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(current) returned error: %v", err)
	}
	for _, want := range []string{"current workspace on redis://", "demo", "afs.config.json"} {
		if !strings.Contains(out, want) {
			t.Fatalf("workspace current output = %q, want substring %q", out, want)
		}
	}
}

func TestCmdWorkspaceUseRejectsSwitchWhileDifferentWorkspaceMounted(t *testing.T) {
	t.Helper()

	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv(HOME) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})

	mr := miniredis.RunT(t)
	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNFS
	cfg.NFSBin = "/usr/bin/true"
	cfg.CurrentWorkspace = "alpha"
	saveTempConfig(t, cfg)

	loadedCfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}
	defer closeStore()
	for _, name := range []string{"alpha", "beta"} {
		if err := createEmptyWorkspace(context.Background(), loadedCfg, store, name); err != nil {
			t.Fatalf("createEmptyWorkspace(%s) returned error: %v", name, err)
		}
	}

	if err := saveState(state{
		StartedAt:        time.Now().UTC(),
		CurrentWorkspace: "alpha",
		MountBackend:     mountBackendNFS,
		MountPID:         os.Getpid(),
		LocalPath:        "/tmp/afs-alpha",
	}); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	err = cmdWorkspace([]string{"workspace", "use", "beta"})
	if err == nil {
		t.Fatal("cmdWorkspace(use beta) returned nil error, want mounted workspace warning")
	}
	for _, want := range []string{`active workspace "alpha"`, "down' before selecting", `"beta"`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("cmdWorkspace(use beta) error = %q, want substring %q", err, want)
		}
	}

	saved, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() returned error: %v", err)
	}
	if saved.CurrentWorkspace != "alpha" {
		t.Fatalf("CurrentWorkspace = %q, want %q after rejected switch", saved.CurrentWorkspace, "alpha")
	}
}

func TestCmdCheckpointHelpListsSubcommands(t *testing.T) {
	t.Helper()

	out, err := captureStderr(t, func() error {
		return cmdCheckpoint([]string{"checkpoint", "--help"})
	})
	if err != nil {
		t.Fatalf("cmdCheckpoint(--help) returned error: %v", err)
	}

	for _, want := range []string{
		"checkpoint <subcommand>",
		"create [workspace] [checkpoint]",
		"restore [workspace] <checkpoint>",
		"checkpoint restore demo initial",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("checkpoint help output = %q, want substring %q", out, want)
		}
	}
}

func TestCmdCheckpointRestoreHelpExplainsLiveRestoreBehavior(t *testing.T) {
	t.Helper()

	out, err := captureStderr(t, func() error {
		return cmdCheckpoint([]string{"checkpoint", "restore", "--help"})
	})
	if err != nil {
		t.Fatalf("cmdCheckpoint(restore --help) returned error: %v", err)
	}

	for _, want := range []string{
		"Restore the workspace live state to the selected checkpoint",
		"checkpoint restore [workspace] <checkpoint>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("checkpoint restore help output = %q, want substring %q", out, want)
		}
	}
}
