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
	if !cfg.UseExistingRedis {
		t.Fatal("UseExistingRedis = false, want true")
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
	if cfg.Mountpoint != wantMountpoint {
		t.Fatalf("Mountpoint = %q, want %q", cfg.Mountpoint, wantMountpoint)
	}
}

func TestCmdConfigShowJSONIncludesConfiguredFields(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.UseExistingRedis = true
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
	base.UseExistingRedis = true
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
	if cfg.Mountpoint != wantMountpoint {
		t.Fatalf("Mountpoint = %q, want %q", cfg.Mountpoint, wantMountpoint)
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
	if saved.Mountpoint != wantMountpoint {
		t.Fatalf("saved Mountpoint = %q, want %q", saved.Mountpoint, wantMountpoint)
	}
	if saved.MountBackend != mountBackendNFS {
		t.Fatalf("saved MountBackend = %q, want %q", saved.MountBackend, mountBackendNFS)
	}
}

func TestLoadConfigForUpPromptsForMissingDatabaseWorkspaceAndMountpoint(t *testing.T) {
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
	cfg.UseExistingRedis = true
	cfg.RedisAddr = mr.Addr()
	cfg.RedisDB = 7
	if err := createEmptyWorkspace(context.Background(), cfg, newAFSStore(rdb), "demo"); err != nil {
		t.Fatalf("createEmptyWorkspace() returned error: %v", err)
	}

	raw := `{
  "useExistingRedis": true,
  "redisAddr": "` + mr.Addr() + `",
  "mountBackend": "nfs",
  "nfsBin": "/usr/bin/true"
}`
	if err := os.WriteFile(configFile, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile(config) returned error: %v", err)
	}

	var output bytes.Buffer
	got, err := loadConfigForUpWithIO(
		[]string{},
		bufio.NewReader(bytes.NewBufferString(stringsJoinLines("7", "", "/tmp/afs-demo"))),
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
	if got.Mountpoint != "/tmp/afs-demo" {
		t.Fatalf("Mountpoint = %q, want %q", got.Mountpoint, "/tmp/afs-demo")
	}
	if !strings.Contains(output.String(), "Available workspace: demo") {
		t.Fatalf("prompt output = %q, want available workspace hint", output.String())
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
	if saved.Mountpoint != "/tmp/afs-demo" {
		t.Fatalf("saved Mountpoint = %q, want %q", saved.Mountpoint, "/tmp/afs-demo")
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
		"--mount-backend auto|none|fuse|nfs",
		"Current workspace is not configured here",
		"runtime paths stay available in afs.config.json",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("config set help output = %q, want substring %q", out, want)
		}
	}
}

func TestLoadConfigForUpRejectsSinglePositionalArgument(t *testing.T) {
	t.Helper()

	_, err := loadConfigForUp([]string{"claude-code"})
	if err == nil {
		t.Fatal("loadConfigForUp() returned nil error, want usage error")
	}
	if !strings.Contains(err.Error(), "up <workspace> <mountpoint>") {
		t.Fatalf("loadConfigForUp() error = %q, want positional usage", err)
	}
}

func TestLoadConfigForUpRejectsMountOverrideWhenMountsAreDisabledInConfig(t *testing.T) {
	t.Helper()

	base := defaultConfig()
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

func TestCmdUpHelpListsPositionalOverrides(t *testing.T) {
	t.Helper()

	out, err := captureStderr(t, func() error {
		return cmdUpArgs([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("cmdUpArgs(--help) returned error: %v", err)
	}

	for _, want := range []string{
		"up <workspace> <mountpoint>",
		"Redis connection, mount backend, and readonly mode come from config",
		"AFS prompts for them in the terminal",
		"config set",
		"up claude-code ~/.claude",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("up help output = %q, want substring %q", out, want)
		}
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
		"run [workspace] [--readonly] -- <command...>",
		"import [--force] [--mount-at-source] <workspace> <directory>",
		"workspace create demo",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("workspace help output = %q, want substring %q", out, want)
		}
	}
}

func TestCmdWorkspaceRunHelpExplainsBehavior(t *testing.T) {
	t.Helper()

	out, err := captureStderr(t, func() error {
		return cmdWorkspace([]string{"workspace", "run", "--help"})
	})
	if err != nil {
		t.Fatalf("cmdWorkspace(run --help) returned error: %v", err)
	}

	for _, want := range []string{
		"save changes back to Redis unless --readonly is set",
		"If <workspace> is omitted",
		"workspace use <workspace>",
		"workspace run demo -- /bin/sh",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("workspace run help output = %q, want substring %q", out, want)
		}
	}
}

func TestCmdWorkspaceUseAndCurrentManageSelectionOutsideConfigCommand(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	cfg := defaultConfig()
	cfg.UseExistingRedis = true
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
	if !strings.Contains(out, "demo") {
		t.Fatalf("workspace current output = %q, want current workspace", out)
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

func TestCmdCheckpointRestoreHelpExplainsArchiveBehavior(t *testing.T) {
	t.Helper()

	out, err := captureStderr(t, func() error {
		return cmdCheckpoint([]string{"checkpoint", "restore", "--help"})
	})
	if err != nil {
		t.Fatalf("cmdCheckpoint(restore --help) returned error: %v", err)
	}

	for _, want := range []string{
		"Restore the workspace working copy to the selected checkpoint",
		"checkpoint restore [workspace] <checkpoint>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("checkpoint restore help output = %q, want substring %q", out, want)
		}
	}
}
