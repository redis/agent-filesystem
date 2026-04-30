package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadSyncDaemonBootstrapFromEnv(t *testing.T) {
	t.Helper()

	homeDir := withTempHome(t)
	bootstrap := syncDaemonBootstrap{
		Config: config{
			ProductMode:      productModeSelfHosted,
			CurrentWorkspace: "repo",
			LocalPath:        filepath.Join(homeDir, "repo"),
			redisConfig: redisConfig{
				RedisAddr: "127.0.0.1:6379",
				RedisDB:   2,
			},
		},
		Workspace: "repo",
		RedisKey:  "repo",
	}

	path, err := writeSyncDaemonBootstrap(bootstrap)
	if err != nil {
		t.Fatalf("writeSyncDaemonBootstrap() returned error: %v", err)
	}
	if err := os.Setenv(syncDaemonBootstrapEnv, path); err != nil {
		t.Fatalf("Setenv(%s) returned error: %v", syncDaemonBootstrapEnv, err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv(syncDaemonBootstrapEnv)
	})

	loaded, ok, err := loadSyncDaemonBootstrap()
	if err != nil {
		t.Fatalf("loadSyncDaemonBootstrap() returned error: %v", err)
	}
	if !ok {
		t.Fatal("loadSyncDaemonBootstrap() reported bootstrap missing")
	}
	if loaded.Workspace != "repo" {
		t.Fatalf("loaded workspace = %q, want %q", loaded.Workspace, "repo")
	}
	if loaded.RedisKey != "repo" {
		t.Fatalf("loaded redis key = %q, want %q", loaded.RedisKey, "repo")
	}
	if loaded.Config.RedisAddr != "127.0.0.1:6379" {
		t.Fatalf("loaded redis addr = %q, want %q", loaded.Config.RedisAddr, "127.0.0.1:6379")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected bootstrap file to be removed after load, stat err = %v", err)
	}
}

func TestWaitForSyncDaemonReady(t *testing.T) {
	t.Helper()
	withTempHome(t)

	path, err := reserveSyncDaemonReadyPath()
	if err != nil {
		t.Fatalf("reserveSyncDaemonReadyPath() returned error: %v", err)
	}
	go func() {
		time.Sleep(10 * time.Millisecond)
		_ = writeSyncDaemonReady(path, nil)
	}()

	if err := waitForSyncDaemonReady(nil, path, time.Second); err != nil {
		t.Fatalf("waitForSyncDaemonReady() returned error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected ready file to be removed after wait, stat err = %v", err)
	}
}

func TestWaitForSyncDaemonReadyReportsChildError(t *testing.T) {
	t.Helper()
	withTempHome(t)

	path, err := reserveSyncDaemonReadyPath()
	if err != nil {
		t.Fatalf("reserveSyncDaemonReadyPath() returned error: %v", err)
	}
	if err := writeSyncDaemonReady(path, errors.New("watcher failed")); err != nil {
		t.Fatalf("writeSyncDaemonReady() returned error: %v", err)
	}

	err = waitForSyncDaemonReady(nil, path, time.Second)
	if err == nil {
		t.Fatal("waitForSyncDaemonReady() returned nil error, want child error")
	}
	if !strings.Contains(err.Error(), "watcher failed") {
		t.Fatalf("waitForSyncDaemonReady() error = %q, want child error detail", err)
	}
}
