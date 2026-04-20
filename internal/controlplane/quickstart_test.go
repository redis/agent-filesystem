package controlplane

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestQuickstartRequestFallsBackToRedisURL(t *testing.T) {
	t.Setenv("AFS_REDIS_ADDR", "")
	t.Setenv("AFS_REDIS_USERNAME", "")
	t.Setenv("AFS_REDIS_PASSWORD", "")
	t.Setenv("AFS_REDIS_DB", "")
	t.Setenv("AFS_REDIS_TLS", "")
	t.Setenv("REDIS_URL", "rediss://default:secret@cache.example.com:16379/4")

	input := QuickstartRequest{}
	cfg := quickstartRedisConfig(input)

	if cfg.RedisAddr != "cache.example.com:16379" {
		t.Fatalf("RedisAddr = %q, want %q", cfg.RedisAddr, "cache.example.com:16379")
	}
	if cfg.RedisUsername != "default" {
		t.Fatalf("RedisUsername = %q, want %q", cfg.RedisUsername, "default")
	}
	if cfg.RedisPassword != "secret" {
		t.Fatalf("RedisPassword = %q, want %q", cfg.RedisPassword, "secret")
	}
	if cfg.RedisDB != 4 {
		t.Fatalf("RedisDB = %d, want %d", cfg.RedisDB, 4)
	}
	if !cfg.RedisTLS {
		t.Fatal("RedisTLS = false, want true")
	}
}

func TestQuickstartRequestPrefersExplicitInput(t *testing.T) {
	t.Setenv("AFS_REDIS_ADDR", "")
	t.Setenv("REDIS_URL", "rediss://default:secret@cache.example.com:16379/4")

	cfg := quickstartRedisConfig(QuickstartRequest{
		RedisAddr:     "manual.redis.example.com:6379",
		RedisUsername: "alice",
		RedisPassword: "manual-secret",
		RedisDB:       2,
		RedisTLS:      false,
	})

	if cfg.RedisAddr != "manual.redis.example.com:6379" {
		t.Fatalf("RedisAddr = %q, want explicit input", cfg.RedisAddr)
	}
	if cfg.RedisUsername != "alice" {
		t.Fatalf("RedisUsername = %q, want explicit input", cfg.RedisUsername)
	}
	if cfg.RedisPassword != "manual-secret" {
		t.Fatalf("RedisPassword = %q, want explicit input", cfg.RedisPassword)
	}
	if cfg.RedisDB != 2 {
		t.Fatalf("RedisDB = %d, want %d", cfg.RedisDB, 2)
	}
	if cfg.RedisTLS {
		t.Fatal("RedisTLS = true, want explicit false")
	}
}

func TestBootstrapDatabaseProfileFromRedisURL(t *testing.T) {
	t.Setenv("AFS_REDIS_ADDR", "")
	t.Setenv("AFS_REDIS_USERNAME", "")
	t.Setenv("AFS_REDIS_PASSWORD", "")
	t.Setenv("AFS_REDIS_DB", "")
	t.Setenv("AFS_REDIS_TLS", "")
	t.Setenv("REDIS_URL", "rediss://default:secret@cache.example.com:16379/4")

	profile, ok := bootstrapDatabaseProfileFromEnv()
	if !ok {
		t.Fatal("bootstrapDatabaseProfileFromEnv() = false, want true")
	}
	if profile.ID != "afs-cloud" {
		t.Fatalf("profile.ID = %q, want %q", profile.ID, "afs-cloud")
	}
	if profile.Name != quickstartCloudDBName {
		t.Fatalf("profile.Name = %q, want %q", profile.Name, quickstartCloudDBName)
	}
	if profile.RedisAddr != "cache.example.com:16379" {
		t.Fatalf("profile.RedisAddr = %q, want %q", profile.RedisAddr, "cache.example.com:16379")
	}
	if profile.RedisDB != 4 {
		t.Fatalf("profile.RedisDB = %d, want %d", profile.RedisDB, 4)
	}
	if !profile.RedisTLS {
		t.Fatal("profile.RedisTLS = false, want true")
	}
	if profile.Purpose != databasePurposeOnboarding {
		t.Fatalf("profile.Purpose = %q, want %q", profile.Purpose, databasePurposeOnboarding)
	}
}

func TestQuickstartCreatesPerSubjectWorkspaceOnOnboardingDatabase(t *testing.T) {
	mr := miniredis.RunT(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "afs.config.json")

	t.Setenv("AFS_REDIS_ADDR", "")
	t.Setenv("AFS_REDIS_USERNAME", "")
	t.Setenv("AFS_REDIS_PASSWORD", "")
	t.Setenv("AFS_REDIS_DB", "")
	t.Setenv("AFS_REDIS_TLS", "")
	t.Setenv("REDIS_URL", "redis://"+mr.Addr()+"/4")

	manager, err := OpenDatabaseManager(configPath)
	if err != nil {
		t.Fatalf("OpenDatabaseManager() returned error: %v", err)
	}
	t.Cleanup(manager.Close)

	ctxA := context.WithValue(context.Background(), authIdentityContextKey, AuthIdentity{Subject: "user-alice"})
	respA, err := manager.Quickstart(ctxA, QuickstartRequest{})
	if err != nil {
		t.Fatalf("Quickstart(alice) returned error: %v", err)
	}
	if respA.DatabaseID != quickstartCloudDBID {
		t.Fatalf("Quickstart(alice) DatabaseID = %q, want %q", respA.DatabaseID, quickstartCloudDBID)
	}
	if respA.Workspace.Name == quickstartWorkspaceName {
		t.Fatalf("Quickstart(alice) workspace name = %q, want per-subject suffix", respA.Workspace.Name)
	}
	if !strings.HasPrefix(respA.Workspace.Name, quickstartWorkspaceName+"-") {
		t.Fatalf("Quickstart(alice) workspace name = %q, want prefix %q", respA.Workspace.Name, quickstartWorkspaceName+"-")
	}

	ctxB := context.WithValue(context.Background(), authIdentityContextKey, AuthIdentity{Subject: "user-bob"})
	respB, err := manager.Quickstart(ctxB, QuickstartRequest{})
	if err != nil {
		t.Fatalf("Quickstart(bob) returned error: %v", err)
	}
	if respB.Workspace.Name == respA.Workspace.Name {
		t.Fatalf("Quickstart(bob) workspace name = %q, want different from alice", respB.Workspace.Name)
	}

	// Calling Quickstart again for alice should be idempotent — return the
	// same workspace by name (the opaque catalog ID is intentionally unstable
	// for onboarding databases, so the response exposes the name as the ID).
	respA2, err := manager.Quickstart(ctxA, QuickstartRequest{})
	if err != nil {
		t.Fatalf("Quickstart(alice) second call returned error: %v", err)
	}
	if respA2.WorkspaceID != respA.WorkspaceID {
		t.Fatalf("Quickstart(alice) second call WorkspaceID = %q, want %q", respA2.WorkspaceID, respA.WorkspaceID)
	}
	if respA2.Workspace.Name != respA.Workspace.Name {
		t.Fatalf("Quickstart(alice) second call Name = %q, want %q", respA2.Workspace.Name, respA.Workspace.Name)
	}
}

func TestQuickstartWithEmptySubjectUsesFlatWorkspaceName(t *testing.T) {
	mr := miniredis.RunT(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "afs.config.json")

	t.Setenv("AFS_REDIS_ADDR", "")
	t.Setenv("AFS_REDIS_USERNAME", "")
	t.Setenv("AFS_REDIS_PASSWORD", "")
	t.Setenv("AFS_REDIS_DB", "")
	t.Setenv("AFS_REDIS_TLS", "")
	t.Setenv("REDIS_URL", "redis://"+mr.Addr()+"/4")

	manager, err := OpenDatabaseManager(configPath)
	if err != nil {
		t.Fatalf("OpenDatabaseManager() returned error: %v", err)
	}
	t.Cleanup(manager.Close)

	resp, err := manager.Quickstart(context.Background(), QuickstartRequest{})
	if err != nil {
		t.Fatalf("Quickstart(anonymous) returned error: %v", err)
	}
	if resp.Workspace.Name != quickstartWorkspaceName {
		t.Fatalf("Quickstart(anonymous) workspace name = %q, want %q", resp.Workspace.Name, quickstartWorkspaceName)
	}
}
