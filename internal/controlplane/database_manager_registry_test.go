package controlplane

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	if record.ManagementType != databaseManagementSystemManaged {
		t.Fatalf("record.ManagementType = %q, want %q", record.ManagementType, databaseManagementSystemManaged)
	}
	if record.Purpose != databasePurposeOnboarding {
		t.Fatalf("record.Purpose = %q, want %q", record.Purpose, databasePurposeOnboarding)
	}
	if record.CanEdit {
		t.Fatal("record.CanEdit = true, want false")
	}
	if record.CanDelete {
		t.Fatal("record.CanDelete = true, want false")
	}
	if !record.CanCreateWorkspaces {
		t.Fatal("record.CanCreateWorkspaces = false, want true (free-tier quota gates creation, not the flag)")
	}
	if record.OwnerLabel != "Starter Database" {
		t.Fatalf("record.OwnerLabel = %q, want %q", record.OwnerLabel, "Starter Database")
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
	if profiles[0].ManagementType != databaseManagementSystemManaged {
		t.Fatalf("profiles[0].ManagementType = %q, want %q", profiles[0].ManagementType, databaseManagementSystemManaged)
	}
	if profiles[0].Purpose != databasePurposeOnboarding {
		t.Fatalf("profiles[0].Purpose = %q, want %q", profiles[0].Purpose, databasePurposeOnboarding)
	}
}

func TestManagedDatabaseCannotBeEditedOrDeleted(t *testing.T) {
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

	if _, err := manager.ListDatabases(context.Background()); err != nil {
		t.Fatalf("ListDatabases() returned error: %v", err)
	}

	_, err = manager.UpsertDatabase(context.Background(), "afs-cloud", upsertDatabaseRequest{
		Name:      "Changed",
		RedisAddr: mr.Addr(),
		RedisDB:   7,
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be edited") {
		t.Fatalf("UpsertDatabase(managed) error = %v, want managed edit rejection", err)
	}

	err = manager.DeleteDatabase("afs-cloud")
	if err == nil || !strings.Contains(err.Error(), "cannot be deleted") {
		t.Fatalf("DeleteDatabase(managed) error = %v, want managed delete rejection", err)
	}

	// CreateWorkspace on the onboarding database is no longer hard-blocked;
	// free-tier quota is enforced per-user at create time instead. With no
	// auth context (and no catalog quota check), an unauthenticated create
	// should succeed.
	if _, err = manager.CreateWorkspace(context.Background(), "afs-cloud", createWorkspaceRequest{
		Name:   "free-tier-allowed",
		Source: sourceRef{Kind: SourceBlank},
	}); err != nil {
		t.Fatalf("CreateWorkspace(onboarding, no-auth) returned error: %v", err)
	}
}

func TestListDatabasesDedupesLegacyOnboardingProfiles(t *testing.T) {
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

	manager.mu.Lock()
	manager.profiles["afs-cloud-legacy"] = databaseProfile{
		ID:             "afs-cloud-legacy",
		Name:           quickstartCloudDBName,
		Description:    "Legacy onboarding database record.",
		OwnerSubject:   "rowan@example.com",
		ManagementType: databaseManagementSystemManaged,
		Purpose:        databasePurposeOnboarding,
		RedisAddr:      mr.Addr(),
		RedisDB:        7,
	}
	manager.order = append(manager.order, "afs-cloud-legacy")
	manager.mu.Unlock()

	response, err := manager.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("ListDatabases() returned error: %v", err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("len(ListDatabases().Items) = %d, want 1 canonical onboarding database", len(response.Items))
	}
	if response.Items[0].ID != quickstartCloudDBID {
		t.Fatalf("record.ID = %q, want %q", response.Items[0].ID, quickstartCloudDBID)
	}

	profiles, err := manager.catalog.ListDatabaseProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListDatabaseProfiles() returned error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("len(ListDatabaseProfiles()) = %d, want 1 after dedupe", len(profiles))
	}
	if profiles[0].ID != quickstartCloudDBID {
		t.Fatalf("profiles[0].ID = %q, want %q", profiles[0].ID, quickstartCloudDBID)
	}
}

func TestListDatabasesDedupesLegacyUserManagedBootstrapProfile(t *testing.T) {
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

	manager.mu.Lock()
	manager.profiles["afs-cloud-duplicate"] = databaseProfile{
		ID:             "afs-cloud-duplicate",
		Name:           quickstartCloudDBName,
		Description:    "Legacy user-managed duplicate.",
		OwnerSubject:   "user_123",
		OwnerLabel:     "user_123",
		ManagementType: databaseManagementUserManaged,
		Purpose:        databasePurposeGeneral,
		RedisAddr:      mr.Addr(),
		RedisDB:        7,
		RedisTLS:       false,
		IsDefault:      true,
	}
	manager.order = append(manager.order, "afs-cloud-duplicate")
	manager.mu.Unlock()

	response, err := manager.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("ListDatabases() returned error: %v", err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("len(ListDatabases().Items) = %d, want 1 canonical onboarding database", len(response.Items))
	}
	if response.Items[0].ID != quickstartCloudDBID {
		t.Fatalf("record.ID = %q, want %q", response.Items[0].ID, quickstartCloudDBID)
	}
}

func TestReservedAFSCloudNameCannotBeUsedForUserManagedDatabase(t *testing.T) {
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

	if _, err := manager.ListDatabases(context.Background()); err != nil {
		t.Fatalf("ListDatabases() returned error: %v", err)
	}

	_, err = manager.UpsertDatabase(context.Background(), "", upsertDatabaseRequest{
		Name:      quickstartCloudDBName,
		RedisAddr: mr.Addr(),
		RedisDB:   8,
	})
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("UpsertDatabase(reserved name) error = %v, want reserved-name rejection", err)
	}
}

func TestOnboardingDatabaseHidesLegacyGettingStartedWorkspaces(t *testing.T) {
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

	databases, err := manager.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("ListDatabases() returned error: %v", err)
	}
	if len(databases.Items) != 1 {
		t.Fatalf("len(ListDatabases().Items) = %d, want 1", len(databases.Items))
	}

	service, profile, err := manager.serviceFor(context.Background(), quickstartCloudDBID)
	if err != nil {
		t.Fatalf("serviceFor(afs-cloud) returned error: %v", err)
	}
	if _, err := createQuickstartWorkspace(context.Background(), service, profile, quickstartWorkspaceName); err != nil {
		t.Fatalf("createQuickstartWorkspace() returned error: %v", err)
	}

	databases, err = manager.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("ListDatabases() after seed returned error: %v", err)
	}
	if databases.Items[0].WorkspaceCount != 1 {
		t.Fatalf("WorkspaceCount = %d, want 1 for onboarding database", databases.Items[0].WorkspaceCount)
	}

	workspaces, err := manager.ListAllWorkspaceSummaries(context.Background())
	if err != nil {
		t.Fatalf("ListAllWorkspaceSummaries() returned error: %v", err)
	}
	if len(workspaces.Items) != 1 {
		t.Fatalf("len(ListAllWorkspaceSummaries().Items) = %d, want 1 starter workspace", len(workspaces.Items))
	}
	if workspaces.Items[0].Name != quickstartWorkspaceName {
		t.Fatalf("workspaces.Items[0].Name = %q, want %q", workspaces.Items[0].Name, quickstartWorkspaceName)
	}
}

func TestListAgentSessionsSkipsOrphanedDatabaseRecords(t *testing.T) {
	manager, _ := newTestManager(t)

	manager.mu.Lock()
	delete(manager.profiles, "secondary")
	manager.order = withoutValue(manager.order, "secondary")
	manager.mu.Unlock()

	if err := manager.catalog.UpsertSession(context.Background(), sessionCatalogRecord{
		SessionID:       "sess-orphaned",
		WorkspaceID:     "ws-orphaned",
		DatabaseID:      "secondary",
		WorkspaceName:   "getting-started",
		ClientKind:      "sync",
		AFSVersion:      "vdev",
		Hostname:        "example",
		OperatingSystem: "darwin",
		LocalPath:       "/tmp/getting-started",
		State:           workspaceSessionStateActive,
		StartedAt:       "2026-04-19T00:00:00Z",
		LastSeenAt:      "2026-04-19T00:01:00Z",
		LeaseExpiresAt:  "2026-04-19T00:02:00Z",
		UpdatedAt:       "2026-04-19T00:01:00Z",
	}); err != nil {
		t.Fatalf("UpsertSession(orphaned) returned error: %v", err)
	}

	response, err := manager.ListAgentSessions(context.Background(), "")
	if err != nil {
		t.Fatalf("ListAgentSessions() returned error: %v", err)
	}
	if len(response.Items) != 0 {
		t.Fatalf("len(ListAgentSessions().Items) = %d, want 0 when session database no longer exists", len(response.Items))
	}
}
