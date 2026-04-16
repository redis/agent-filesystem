package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMountSessionBootstrapFromEnv(t *testing.T) {
	t.Helper()

	homeDir := withTempHome(t)
	bootstrap := mountSessionBootstrap{
		Config: config{
			ProductMode:      productModeSelfHosted,
			CurrentWorkspace: "repo",
			LocalPath:        filepath.Join(homeDir, "repo"),
			redisConfig: redisConfig{
				RedisAddr: "127.0.0.1:6379",
				RedisDB:   2,
			},
		},
		Workspace:                "repo",
		SessionID:                "sess_123",
		HeartbeatIntervalSeconds: 20,
		MountPID:                 4242,
	}

	path, err := writeMountSessionBootstrap(bootstrap)
	if err != nil {
		t.Fatalf("writeMountSessionBootstrap() returned error: %v", err)
	}
	if err := os.Setenv(mountSessionBootstrapEnv, path); err != nil {
		t.Fatalf("Setenv(%s) returned error: %v", mountSessionBootstrapEnv, err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv(mountSessionBootstrapEnv)
	})

	loaded, ok, err := loadMountSessionBootstrap()
	if err != nil {
		t.Fatalf("loadMountSessionBootstrap() returned error: %v", err)
	}
	if !ok {
		t.Fatal("loadMountSessionBootstrap() reported bootstrap missing")
	}
	if loaded.Workspace != "repo" {
		t.Fatalf("loaded workspace = %q, want %q", loaded.Workspace, "repo")
	}
	if loaded.SessionID != "sess_123" {
		t.Fatalf("loaded session id = %q, want %q", loaded.SessionID, "sess_123")
	}
	if loaded.MountPID != 4242 {
		t.Fatalf("loaded mount pid = %d, want %d", loaded.MountPID, 4242)
	}
	if loaded.Config.RedisAddr != "127.0.0.1:6379" {
		t.Fatalf("loaded redis addr = %q, want %q", loaded.Config.RedisAddr, "127.0.0.1:6379")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected bootstrap file to be removed after load, stat err = %v", err)
	}
}
