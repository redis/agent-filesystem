package controlplane

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alicebob/miniredis/v2"
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

func TestListDatabasesBootstrapsManagedRedisProfile(t *testing.T) {
	mr := miniredis.RunT(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "afs.config.json")

	t.Setenv("AFS_REDIS_ADDR", "")
	t.Setenv("AFS_REDIS_USERNAME", "")
	t.Setenv("AFS_REDIS_PASSWORD", "")
	t.Setenv("AFS_REDIS_DB", "")
	t.Setenv("AFS_REDIS_TLS", "")
	t.Setenv("REDIS_URL", "redis://"+mr.Addr()+"/7")

	manager, err := OpenDatabaseManager(configPath)
	if err != nil {
		t.Fatalf("OpenDatabaseManager() returned error: %v", err)
	}
	defer manager.Close()

	response, err := manager.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("ListDatabases() returned error: %v", err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("len(ListDatabases().Items) = %d, want 1", len(response.Items))
	}

	record := response.Items[0]
	if record.ID != "afs-cloud" {
		t.Fatalf("record.ID = %q, want %q", record.ID, "afs-cloud")
	}
	if record.Name != quickstartCloudDBName {
		t.Fatalf("record.Name = %q, want %q", record.Name, quickstartCloudDBName)
	}
	if record.Description != "Managed Redis Cloud data plane for hosted Agent Filesystem." {
		t.Fatalf("record.Description = %q", record.Description)
	}
	if record.RedisAddr != mr.Addr() {
		t.Fatalf("record.RedisAddr = %q, want %q", record.RedisAddr, mr.Addr())
	}
	if record.RedisDB != 7 {
		t.Fatalf("record.RedisDB = %d, want %d", record.RedisDB, 7)
	}
	if !record.IsDefault {
		t.Fatal("record.IsDefault = false, want true")
	}

	profiles, err := manager.catalog.ListDatabaseProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListDatabaseProfiles() returned error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("len(ListDatabaseProfiles()) = %d, want 1", len(profiles))
	}
	if profiles[0].ID != "afs-cloud" {
		t.Fatalf("profiles[0].ID = %q, want %q", profiles[0].ID, "afs-cloud")
	}
}
