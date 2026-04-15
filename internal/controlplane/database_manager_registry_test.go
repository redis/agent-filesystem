package controlplane

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenDatabaseManagerStartsWithEmptyRegistry(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "afs.config.json")
	cfg := Config{
		RedisConfig: RedisConfig{
			RedisAddr: "localhost:6380",
			RedisDB:   0,
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal(cfg) returned error: %v", err)
	}
	if err := os.WriteFile(configPath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile(config) returned error: %v", err)
	}

	manager, err := OpenDatabaseManager(configPath)
	if err != nil {
		t.Fatalf("OpenDatabaseManager() returned error: %v", err)
	}
	defer manager.Close()

	profiles, err := manager.catalog.ListDatabaseProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListDatabaseProfiles() returned error: %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("len(ListDatabaseProfiles()) = %d, want 0 (control plane should start empty)", len(profiles))
	}

	if _, err := os.Stat(filepath.Join(dir, "afs.databases.json")); !os.IsNotExist(err) {
		t.Fatalf("afs.databases.json should not be written, stat err = %v", err)
	}
}
