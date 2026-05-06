package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if cfg.LocalPath != "" {
		t.Fatalf("LocalPath = %q, want empty because mount paths are per-mount state", cfg.LocalPath)
	}
}

func TestSaveConfigPersistsDefaultWorkspaceOutsideLegacyRuntimeKeys(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.RedisAddr = "127.0.0.1:6380"
	cfg.ProductMode = productModeSelfHosted
	cfg.URL = "http://127.0.0.1:8091"
	cfg.DatabaseID = "db-local"
	cfg.CurrentWorkspace = "demo"
	cfg.CurrentWorkspaceID = "ws_demo"
	cfg.LocalPath = "/tmp/demo"
	cfg.Mode = modeMount
	cfg.MountBackend = mountBackendNFS
	cfg.ReadOnly = true
	cfg.Name = "agent"
	saveTempConfig(t, cfg)

	raw, err := os.ReadFile(configPath())
	if err != nil {
		t.Fatalf("ReadFile(config) returned error: %v", err)
	}
	var saved map[string]any
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("Unmarshal(config) returned error: %v", err)
	}
	for _, key := range []string{"currentWorkspace", "currentWorkspaceID", "localPath", "mount", "logs"} {
		if _, ok := saved[key]; ok {
			t.Fatalf("config should not persist runtime key %q: %s", key, string(raw))
		}
	}
	workspace, ok := saved["workspace"].(map[string]any)
	if !ok {
		t.Fatalf("config should persist default workspace under workspace key: %s", string(raw))
	}
	if workspace["default"] != "demo" || workspace["defaultID"] != "ws_demo" {
		t.Fatalf("workspace config = %#v, want demo/ws_demo", workspace)
	}
	runtime, ok := saved["runtime"].(map[string]any)
	if !ok {
		t.Fatalf("config should persist runtime mount/log settings under runtime key: %s", string(raw))
	}
	for _, key := range []string{"currentWorkspace", "currentWorkspaceID"} {
		if _, ok := runtime[key]; ok {
			t.Fatalf("runtime config should not persist workspace key %q: %s", key, string(raw))
		}
	}
	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() returned error: %v", err)
	}
	if loaded.CurrentWorkspace != "demo" || loaded.CurrentWorkspaceID != "ws_demo" {
		t.Fatalf("loaded workspace = %q/%q, want demo/ws_demo", loaded.CurrentWorkspace, loaded.CurrentWorkspaceID)
	}
	if got, ok := saved["mode"].(string); !ok || got != modeMount {
		t.Fatalf("config should persist mode %q, got %v in %s", modeMount, saved["mode"], string(raw))
	}
	runtimeCfg, ok := saved["runtime"].(map[string]any)
	if !ok {
		t.Fatalf("config should keep runtime defaults without per-mount state: %s", string(raw))
	}
	if _, ok := runtimeCfg["localPath"]; ok {
		t.Fatalf("config should not persist runtime.localPath: %s", string(raw))
	}
	mountCfg, ok := runtimeCfg["mount"].(map[string]any)
	if !ok {
		t.Fatalf("config should keep runtime.mount defaults: %s", string(raw))
	}
	if _, ok := mountCfg["readOnly"]; ok {
		t.Fatalf("config should not persist runtime.mount.readOnly: %s", string(raw))
	}
	if _, ok := saved["controlPlane"]; !ok {
		t.Fatalf("config should keep controlPlane settings: %s", string(raw))
	}
	if _, ok := saved["agent"]; !ok {
		t.Fatalf("config should keep agent settings: %s", string(raw))
	}
}

func TestLoadConfigIgnoresLegacyRuntimeLocalPathAndReadOnly(t *testing.T) {
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
  "runtime": {
    "localPath": "/tmp/legacy",
    "mount": {
      "backend": "nfs",
      "readOnly": true,
      "nfsHost": "127.0.0.1",
      "nfsPort": 20490
    }
  }
}`
	if err := os.WriteFile(configFile, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile(config) returned error: %v", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() returned error: %v", err)
	}
	if cfg.LocalPath != "" {
		t.Fatalf("LocalPath = %q, want empty", cfg.LocalPath)
	}
	if cfg.ReadOnly {
		t.Fatal("ReadOnly = true, want false because readonly is per mount")
	}
	if cfg.MountBackend != mountBackendNFS {
		t.Fatalf("MountBackend = %q, want nfs", cfg.MountBackend)
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

	var got persistedConfig
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("Unmarshal(config show json) returned error: %v", err)
	}
	if got.Redis.RedisAddr != "redis.example:6380" {
		t.Fatalf("RedisAddr = %q, want %q", got.Redis.RedisAddr, "redis.example:6380")
	}
	if got.Redis.RedisDB != 7 {
		t.Fatalf("RedisDB = %d, want %d", got.Redis.RedisDB, 7)
	}
	if got.Workspace.DefaultWorkspace != "demo" {
		t.Fatalf("workspace.default = %q, want %q", got.Workspace.DefaultWorkspace, "demo")
	}
	if got.Workspace.DefaultWorkspaceID != "" {
		t.Fatalf("workspace.defaultID = %q, want empty", got.Workspace.DefaultWorkspaceID)
	}
}

func TestCmdConfigSetModePersistsRuntimeMode(t *testing.T) {
	t.Helper()

	configFile := filepath.Join(t.TempDir(), "afs.config.json")
	origConfigPath := cfgPathOverride
	cfgPathOverride = configFile
	t.Cleanup(func() {
		cfgPathOverride = origConfigPath
	})

	if err := cmdConfig([]string{"config", "set", "mode", "mount"}); err != nil {
		t.Fatalf("cmdConfig(set mode mount) returned error: %v", err)
	}
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() returned error: %v", err)
	}
	if cfg.Mode != modeMount {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, modeMount)
	}
	value, err := getConfigKey(cfg, "mode")
	if err != nil {
		t.Fatalf("getConfigKey(mode) returned error: %v", err)
	}
	if value != modeMount {
		t.Fatalf("getConfigKey(mode) = %q, want %q", value, modeMount)
	}

	if err := cmdConfig([]string{"config", "set", "--mode", "sync"}); err != nil {
		t.Fatalf("cmdConfig(set --mode sync) returned error: %v", err)
	}
	cfg, err = loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() after --mode returned error: %v", err)
	}
	if got, err := effectiveMode(cfg); err != nil || got != modeSync {
		t.Fatalf("effectiveMode() = %q, %v; want %q", got, err, modeSync)
	}
}

func TestCmdConfigResetRemovesConfigAndState(t *testing.T) {
	t.Helper()

	withTempHome(t)
	configFile := filepath.Join(t.TempDir(), "afs.config.json")
	origConfigPath := cfgPathOverride
	cfgPathOverride = configFile
	t.Cleanup(func() {
		cfgPathOverride = origConfigPath
	})

	if err := os.WriteFile(configFile, []byte(`{"mode":"mount"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(config) returned error: %v", err)
	}
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		t.Fatalf("MkdirAll(stateDir) returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir(), "mounts.json"), []byte(`{"mounts":[]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(state) returned error: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return cmdConfig([]string{"config", "reset"})
	})
	if err != nil {
		t.Fatalf("cmdConfig(reset) returned error: %v", err)
	}
	if _, err := os.Stat(configFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config file still exists after reset: %v", err)
	}
	if _, err := os.Stat(stateDir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state dir still exists after reset: %v", err)
	}
	if !strings.Contains(out, "local state reset") {
		t.Fatalf("cmdConfig(reset) output = %q, want local state reset", out)
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
	if saved.LocalPath != "" {
		t.Fatalf("saved LocalPath = %q, want empty because mount paths are per-mount state", saved.LocalPath)
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
	if saved.LocalPath != "" {
		t.Fatalf("saved LocalPath = %q, want empty after rejected mountpoint", saved.LocalPath)
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
	  "workspace": {
	    "default": "demo"
	  },
	  "runtime": {
	    "mount": {
	      "backend": "nfs",
	      "nfsBin": "/usr/bin/true"
	    }
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
	if saved.LocalPath != "" {
		t.Fatalf("saved LocalPath = %q, want empty because mount paths are per-mount state", saved.LocalPath)
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
	if !strings.Contains(err.Error(), "workspace is required") {
		t.Fatalf("loadConfigForUpWithIO() error = %q, want missing workspace message", err)
	}
	if !strings.Contains(err.Error(), "ws mount <workspace> <directory>") {
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
		"config.source",
		"controlPlane.url",
		"redis.url",
		"sync.fileSizeCapMB",
		"config set controlPlane.url",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("config help output = %q, want substring %q", out, want)
		}
	}
	if strings.Contains(out, "Legacy shortcuts") {
		t.Fatalf("config help output still includes legacy shortcuts:\n%s", out)
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
		"Default workspace is managed with",
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
	base.MountBackend = mountBackendNFS
	base.NFSBin = "/usr/bin/true"
	saveTempConfig(t, base)

	mode := optionalString{value: modeMount, set: true}
	mountpoint := filepath.Join(t.TempDir(), "mnt")
	cfg, err := loadConfigForUpWithMode([]string{"alpha", mountpoint}, mode)
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

func TestCmdStatusHelpListsVerboseFlag(t *testing.T) {
	t.Helper()

	out, err := captureStderr(t, func() error {
		return cmdStatusArgs([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("cmdStatusArgs(--help) returned error: %v", err)
	}

	for _, want := range []string{
		"status [--verbose]",
		"--verbose, -v",
		"control-plane, session, and process details",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("status help output = %q, want substring %q", out, want)
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
		"versioning <get|set>",
		"default",
		"set-default <workspace>",
		"unset-default",
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
	for _, removed := range []string{"use <workspace>", "current                                      Show"} {
		if strings.Contains(out, removed) {
			t.Fatalf("workspace help output = %q, did not expect removed subcommand %q", out, removed)
		}
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

func TestCmdWorkspaceUseAndCurrentAreNotSupported(t *testing.T) {
	t.Helper()

	for _, subcommand := range []string{"use", "current"} {
		err := cmdWorkspace([]string{"workspace", subcommand})
		if err == nil {
			t.Fatalf("cmdWorkspace(%s) returned nil error, want unsupported-command error", subcommand)
		}
		if !strings.Contains(err.Error(), `unknown workspace subcommand "`+subcommand+`"`) {
			t.Fatalf("cmdWorkspace(%s) error = %q, want unsupported-command error", subcommand, err)
		}
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
		"diff [workspace] <base> <target>",
		"restore [workspace] <checkpoint>",
		"checkpoint diff demo initial before-refactor",
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
		return cmdCheckpoint([]string{"cp", "restore", "--help"})
	})
	if err != nil {
		t.Fatalf("cmdCheckpoint(restore --help) returned error: %v", err)
	}

	for _, want := range []string{
		"Restore workspace state to the selected checkpoint",
		"cp restore [workspace] <checkpoint>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("checkpoint restore help output = %q, want substring %q", out, want)
		}
	}
}
