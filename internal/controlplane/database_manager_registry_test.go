package controlplane

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if record.SupportsArrays == nil {
		t.Fatal("record.SupportsArrays = nil, want false for miniredis")
	}
	if *record.SupportsArrays {
		t.Fatal("record.SupportsArrays = true, want false for miniredis")
	}
	if len(record.WorkspaceStorage) != 0 {
		t.Fatalf("len(record.WorkspaceStorage) = %d, want 0 for empty database", len(record.WorkspaceStorage))
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

func TestListAgentSessionsScopesSharedDatabaseSessionsToWorkspaceOwner(t *testing.T) {
	manager, databaseID := newTestManager(t)

	manager.mu.Lock()
	profile := manager.profiles[databaseID]
	profile.OwnerSubject = ""
	profile.OwnerLabel = ""
	manager.profiles[databaseID] = profile
	manager.mu.Unlock()

	aliceCtx := context.WithValue(context.Background(), authIdentityContextKey, AuthIdentity{
		Subject: "alice@example.com",
		Name:    "Alice",
		Email:   "alice@example.com",
	})
	bobCtx := context.WithValue(context.Background(), authIdentityContextKey, AuthIdentity{
		Subject: "bob@example.com",
		Name:    "Bob",
		Email:   "bob@example.com",
	})

	aliceWorkspace, err := manager.CreateWorkspace(aliceCtx, databaseID, createWorkspaceRequest{
		Name:        "alice-repo",
		Description: "Alice workspace",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace(alice) returned error: %v", err)
	}
	bobWorkspace, err := manager.CreateWorkspace(bobCtx, databaseID, createWorkspaceRequest{
		Name:        "bob-repo",
		Description: "Bob workspace",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace(bob) returned error: %v", err)
	}

	if _, err := manager.CreateWorkspaceSession(aliceCtx, databaseID, aliceWorkspace.ID, createWorkspaceSessionRequest{
		AgentID:         "agt_alice",
		ClientKind:      "sync",
		AFSVersion:      "test",
		Hostname:        "alice-mac",
		OperatingSystem: "darwin",
		LocalPath:       "/tmp/alice-repo",
	}); err != nil {
		t.Fatalf("CreateWorkspaceSession(alice) returned error: %v", err)
	}
	if _, err := manager.CreateWorkspaceSession(bobCtx, databaseID, bobWorkspace.ID, createWorkspaceSessionRequest{
		AgentID:         "agt_bob",
		ClientKind:      "sync",
		AFSVersion:      "test",
		Hostname:        "bob-mac",
		OperatingSystem: "darwin",
		LocalPath:       "/tmp/bob-repo",
	}); err != nil {
		t.Fatalf("CreateWorkspaceSession(bob) returned error: %v", err)
	}

	aliceSessions, err := manager.ListAgentSessions(aliceCtx, "")
	if err != nil {
		t.Fatalf("ListAgentSessions(alice) returned error: %v", err)
	}
	if len(aliceSessions.Items) != 1 {
		t.Fatalf("len(ListAgentSessions(alice).Items) = %d, want 1: %#v", len(aliceSessions.Items), aliceSessions.Items)
	}
	if aliceSessions.Items[0].AgentID != "agt_alice" {
		t.Fatalf("alice visible agent = %q, want agt_alice", aliceSessions.Items[0].AgentID)
	}

	bobSessions, err := manager.ListAgentSessions(bobCtx, "")
	if err != nil {
		t.Fatalf("ListAgentSessions(bob) returned error: %v", err)
	}
	if len(bobSessions.Items) != 1 {
		t.Fatalf("len(ListAgentSessions(bob).Items) = %d, want 1: %#v", len(bobSessions.Items), bobSessions.Items)
	}
	if bobSessions.Items[0].AgentID != "agt_bob" {
		t.Fatalf("bob visible agent = %q, want agt_bob", bobSessions.Items[0].AgentID)
	}
}

func TestListAgentSessionsReconcilesDeadSessions(t *testing.T) {
	manager, databaseID := newTestManager(t)
	ctx := context.Background()

	service, _, err := manager.serviceFor(ctx, databaseID)
	if err != nil {
		t.Fatalf("serviceFor() returned error: %v", err)
	}

	session, err := service.CreateWorkspaceSession(ctx, "repo", CreateWorkspaceSessionRequest{
		ClientKind:      "sync",
		AFSVersion:      "dev",
		Hostname:        "devbox",
		OperatingSystem: "darwin",
		LocalPath:       "/tmp/repo",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() returned error: %v", err)
	}
	if _, err := service.HeartbeatWorkspaceSession(ctx, "repo", session.SessionID); err != nil {
		t.Fatalf("HeartbeatWorkspaceSession() returned error: %v", err)
	}

	_, storageID, err := service.store.resolveWorkspaceMeta(ctx, "repo")
	if err != nil {
		t.Fatalf("resolveWorkspaceMeta() returned error: %v", err)
	}
	if err := service.store.rdb.Del(ctx, workspaceSessionLeaseKey(storageID, session.SessionID)).Err(); err != nil {
		t.Fatalf("Del(workspaceSessionLeaseKey) returned error: %v", err)
	}

	response, err := manager.ListAgentSessions(ctx, "")
	if err != nil {
		t.Fatalf("ListAgentSessions() returned error: %v", err)
	}
	if len(response.Items) != 0 {
		t.Fatalf("len(ListAgentSessions().Items) = %d, want 0 after dead session is reconciled", len(response.Items))
	}

	record, err := manager.catalog.GetSession(ctx, session.SessionID)
	if err != nil {
		t.Fatalf("catalog.GetSession() returned error: %v", err)
	}
	if record.State != workspaceSessionStateStale {
		t.Fatalf("catalog session state = %q, want %q", record.State, workspaceSessionStateStale)
	}
	if record.CloseReason != "expired" {
		t.Fatalf("catalog close_reason = %q, want %q", record.CloseReason, "expired")
	}
}

func TestListAgentSessionsHidesExpiredCatalogRowsWithoutLiveSession(t *testing.T) {
	manager, databaseID := newTestManager(t)
	ctx := context.Background()

	routes, err := manager.catalog.ResolveWorkspace(ctx, "repo")
	if err != nil {
		t.Fatalf("ResolveWorkspace() returned error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("len(ResolveWorkspace()) = %d, want 1", len(routes))
	}

	expiredAt := time.Now().UTC().Add(-6 * 24 * time.Hour)
	record := sessionCatalogRecord{
		SessionID:       "sess-dead-catalog",
		WorkspaceID:     routes[0].WorkspaceID,
		DatabaseID:      databaseID,
		WorkspaceName:   "repo",
		ClientKind:      "sync",
		AFSVersion:      "dev",
		Hostname:        "devbox",
		OperatingSystem: "darwin",
		LocalPath:       "/tmp/repo",
		State:           workspaceSessionStateActive,
		StartedAt:       expiredAt.Add(-time.Hour).Format(timeRFC3339),
		LastSeenAt:      expiredAt.Format(timeRFC3339),
		LeaseExpiresAt:  expiredAt.Format(timeRFC3339),
		UpdatedAt:       expiredAt.Format(timeRFC3339),
	}
	if err := manager.catalog.UpsertSession(ctx, record); err != nil {
		t.Fatalf("UpsertSession(expired) returned error: %v", err)
	}

	response, err := manager.ListAgentSessions(ctx, "")
	if err != nil {
		t.Fatalf("ListAgentSessions() returned error: %v", err)
	}
	if len(response.Items) != 0 {
		t.Fatalf("len(ListAgentSessions().Items) = %d, want 0 when catalog row lease is expired", len(response.Items))
	}

	stored, err := manager.catalog.GetSession(ctx, record.SessionID)
	if err != nil {
		t.Fatalf("catalog.GetSession() returned error: %v", err)
	}
	if stored.State != workspaceSessionStateStale {
		t.Fatalf("catalog session state = %q, want %q", stored.State, workspaceSessionStateStale)
	}
	if stored.CloseReason != "expired" {
		t.Fatalf("catalog close_reason = %q, want %q", stored.CloseReason, "expired")
	}
}
