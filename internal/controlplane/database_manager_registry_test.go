package controlplane

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenDatabaseManagerImportsLegacyJSONRegistryIntoSQLite(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "afs.config.json")
	if err := os.WriteFile(configPath, []byte("{\"redis\":{\"addr\":\"localhost:6379\",\"db\":0}}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config) returned error: %v", err)
	}

	legacyPath := filepath.Join(dir, "afs.databases.json")
	legacy := databaseRegistryFile{
		Version: databaseRegistryVersion,
		Databases: []databaseProfile{
			{
				ID:        "db-one",
				Name:      "One",
				RedisAddr: "localhost:6380",
				RedisDB:   0,
			},
			{
				ID:        "db-two",
				Name:      "Two",
				RedisAddr: "localhost:6388",
				RedisDB:   0,
			},
		},
	}
	data, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent(legacy) returned error: %v", err)
	}
	if err := os.WriteFile(legacyPath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile(legacy registry) returned error: %v", err)
	}

	manager, err := OpenDatabaseManager(configPath)
	if err != nil {
		t.Fatalf("OpenDatabaseManager() returned error: %v", err)
	}
	defer manager.Close()

	if got := len(manager.order); got != 2 {
		t.Fatalf("len(manager.order) = %d, want 2", got)
	}

	profiles, err := manager.catalog.ListDatabaseProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListDatabaseProfiles() returned error: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("len(ListDatabaseProfiles()) = %d, want 2", len(profiles))
	}
	if profiles[0].ID != "db-one" || profiles[1].ID != "db-two" {
		t.Fatalf("database registry order = %#v, want db-one then db-two", profiles)
	}
}

func TestOpenDatabaseManagerSeedsSQLiteRegistryWithoutCreatingLegacyJSON(t *testing.T) {
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
	if len(profiles) != 1 {
		t.Fatalf("len(ListDatabaseProfiles()) = %d, want 1", len(profiles))
	}

	if _, err := os.Stat(filepath.Join(dir, "afs.databases.json")); !os.IsNotExist(err) {
		t.Fatalf("afs.databases.json should not be written, stat err = %v", err)
	}
}
