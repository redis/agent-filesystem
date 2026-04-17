package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testPostgresDSNEnvVar = "AFS_TEST_POSTGRES_DSN"

func TestCatalogStoreContractSQLite(t *testing.T) {
	runCatalogStoreContractTests(t, func(t *testing.T) catalogStore {
		t.Helper()

		configPath := filepath.Join(t.TempDir(), "afs.config.json")
		store, err := openWorkspaceCatalog(configPath)
		if err != nil {
			t.Fatalf("openWorkspaceCatalog() returned error: %v", err)
		}
		return store
	})
}

func TestCatalogStoreContractPostgres(t *testing.T) {
	baseDSN := strings.TrimSpace(os.Getenv(testPostgresDSNEnvVar))
	if baseDSN == "" {
		t.Skipf("%s is not set", testPostgresDSNEnvVar)
	}

	runCatalogStoreContractTests(t, func(t *testing.T) catalogStore {
		t.Helper()

		admin, err := sql.Open("pgx", baseDSN)
		if err != nil {
			t.Fatalf("sql.Open(admin postgres) returned error: %v", err)
		}
		t.Cleanup(func() {
			_ = admin.Close()
		})

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := admin.PingContext(ctx); err != nil {
			t.Fatalf("PingContext(admin postgres) returned error: %v", err)
		}

		dbName := fmt.Sprintf("afs_test_%d", time.Now().UTC().UnixNano())
		if _, err := admin.ExecContext(ctx, `CREATE DATABASE `+dbName); err != nil {
			t.Fatalf("CREATE DATABASE %s returned error: %v", dbName, err)
		}
		t.Cleanup(func() {
			dropCtx, dropCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer dropCancel()
			if _, err := admin.ExecContext(dropCtx, `DROP DATABASE IF EXISTS `+dbName); err != nil {
				t.Fatalf("DROP DATABASE %s returned error: %v", dbName, err)
			}
		})

		dsn, err := postgresTestDSNWithDatabase(baseDSN, dbName)
		if err != nil {
			t.Fatalf("postgresTestDSNWithDatabase() returned error: %v", err)
		}

		t.Setenv(catalogDriverEnvVar, catalogDriverPostgres)
		t.Setenv(catalogDSNEnvVar, dsn)
		t.Setenv("POSTGRES_URL_NON_POOLING", "")
		t.Setenv("POSTGRES_URL", "")
		t.Setenv("DATABASE_URL", "")

		store, err := openCatalogStore("/ignored/afs.config.json")
		if err != nil {
			t.Fatalf("openCatalogStore(postgres) returned error: %v", err)
		}
		return store
	})
}

func runCatalogStoreContractTests(t *testing.T, openStore func(t *testing.T) catalogStore) {
	t.Helper()

	ctx := context.Background()
	store := openStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("catalog.Close() returned error: %v", err)
		}
	})

	profiles := []databaseProfile{
		{
			ID:            "db-primary",
			Name:          "Primary",
			Description:   "main database",
			RedisAddr:     "redis-primary:6379",
			RedisUsername: "default",
			RedisPassword: "secret",
			RedisDB:       4,
			RedisTLS:      true,
			IsDefault:     true,
		},
		{
			ID:          "db-secondary",
			Name:        "Secondary",
			Description: "backup database",
			RedisAddr:   "redis-secondary:6379",
			RedisDB:     9,
		},
	}
	if err := store.ReplaceDatabaseProfiles(ctx, profiles); err != nil {
		t.Fatalf("ReplaceDatabaseProfiles() returned error: %v", err)
	}
	gotProfiles, err := store.ListDatabaseProfiles(ctx)
	if err != nil {
		t.Fatalf("ListDatabaseProfiles() returned error: %v", err)
	}
	if len(gotProfiles) != 2 {
		t.Fatalf("len(ListDatabaseProfiles()) = %d, want 2", len(gotProfiles))
	}
	if gotProfiles[0].ID != "db-primary" || !gotProfiles[0].RedisTLS || !gotProfiles[0].IsDefault {
		t.Fatalf("primary database profile = %#v, want redis_tls and is_default preserved", gotProfiles[0])
	}

	repoA, err := store.UpsertWorkspace(ctx, workspaceSummary{
		Name:             "repo",
		DatabaseID:       "db-primary",
		DatabaseName:     "Primary",
		CloudAccount:     "Redis Cloud",
		RedisKey:         "workspace:repo",
		Status:           "ready",
		FileCount:        12,
		FolderCount:      3,
		TotalBytes:       2048,
		CheckpointCount:  2,
		DraftState:       "clean",
		LastCheckpointAt: "2026-04-17T06:00:00Z",
		UpdatedAt:        "2026-04-17T06:01:00Z",
		Region:           "us-west-2",
		Source:           sourceBlank,
	})
	if err != nil {
		t.Fatalf("UpsertWorkspace(repo primary) returned error: %v", err)
	}
	if repoA.ID == "" || !strings.HasPrefix(repoA.ID, "ws_") {
		t.Fatalf("repo primary workspace id = %q, want opaque ws_* id", repoA.ID)
	}

	repoAUpdated, err := store.UpsertWorkspace(ctx, workspaceSummary{
		Name:             "repo",
		DatabaseID:       "db-primary",
		DatabaseName:     "Primary",
		CloudAccount:     "Redis Cloud",
		RedisKey:         "workspace:repo",
		Status:           "busy",
		FileCount:        21,
		FolderCount:      5,
		TotalBytes:       4096,
		CheckpointCount:  3,
		DraftState:       "dirty",
		LastCheckpointAt: "2026-04-17T06:05:00Z",
		UpdatedAt:        "2026-04-17T06:06:00Z",
		Region:           "us-west-2",
		Source:           sourceCloudImport,
	})
	if err != nil {
		t.Fatalf("UpsertWorkspace(repo primary update) returned error: %v", err)
	}
	if repoAUpdated.ID != repoA.ID {
		t.Fatalf("updated workspace id = %q, want %q", repoAUpdated.ID, repoA.ID)
	}

	repoB, err := store.UpsertWorkspace(ctx, workspaceSummary{
		Name:             "repo",
		DatabaseID:       "db-secondary",
		DatabaseName:     "Secondary",
		RedisKey:         "workspace:repo",
		Status:           "ready",
		FileCount:        7,
		FolderCount:      2,
		TotalBytes:       1024,
		CheckpointCount:  1,
		DraftState:       "clean",
		LastCheckpointAt: "2026-04-17T06:02:00Z",
		UpdatedAt:        "2026-04-17T06:03:00Z",
		Region:           "us-east-1",
		Source:           sourceBlank,
	})
	if err != nil {
		t.Fatalf("UpsertWorkspace(repo secondary) returned error: %v", err)
	}

	routes, err := store.ResolveWorkspace(ctx, "repo")
	if err != nil {
		t.Fatalf("ResolveWorkspace(repo) returned error: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("len(ResolveWorkspace(repo)) = %d, want 2", len(routes))
	}

	route, err := store.ResolveWorkspaceInDatabase(ctx, "db-primary", "repo")
	if err != nil {
		t.Fatalf("ResolveWorkspaceInDatabase(db-primary, repo) returned error: %v", err)
	}
	if route.WorkspaceID != repoA.ID {
		t.Fatalf("ResolveWorkspaceInDatabase() workspace_id = %q, want %q", route.WorkspaceID, repoA.ID)
	}

	synced, err := store.ReplaceDatabaseWorkspaces(ctx, "db-primary", []workspaceSummary{
		{
			Name:             "repo",
			DatabaseName:     "Primary",
			RedisKey:         "workspace:repo",
			Status:           "ready",
			FileCount:        30,
			FolderCount:      8,
			TotalBytes:       8192,
			CheckpointCount:  4,
			DraftState:       "clean",
			LastCheckpointAt: "2026-04-17T06:10:00Z",
			UpdatedAt:        "2026-04-17T06:11:00Z",
			Region:           "us-west-2",
			Source:           sourceBlank,
		},
		{
			Name:             "docs",
			DatabaseName:     "Primary",
			RedisKey:         "workspace:docs",
			Status:           "ready",
			FileCount:        4,
			FolderCount:      1,
			TotalBytes:       512,
			CheckpointCount:  1,
			DraftState:       "clean",
			LastCheckpointAt: "2026-04-17T06:12:00Z",
			UpdatedAt:        "2026-04-17T06:13:00Z",
			Region:           "us-west-2",
			Source:           sourceBlank,
		},
	})
	if err != nil {
		t.Fatalf("ReplaceDatabaseWorkspaces() returned error: %v", err)
	}
	if len(synced) != 2 {
		t.Fatalf("len(ReplaceDatabaseWorkspaces()) = %d, want 2", len(synced))
	}
	if synced[0].ID != repoA.ID {
		t.Fatalf("repo workspace id after replace = %q, want %q", synced[0].ID, repoA.ID)
	}

	workspaces, err := store.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces() returned error: %v", err)
	}
	if len(workspaces) != 3 {
		t.Fatalf("len(ListWorkspaces()) = %d, want 3", len(workspaces))
	}

	if err := store.PruneDatabases(ctx, []string{"db-primary"}); err != nil {
		t.Fatalf("PruneDatabases() returned error: %v", err)
	}
	routes, err = store.ResolveWorkspace(ctx, repoB.ID)
	if err != nil {
		t.Fatalf("ResolveWorkspace(repo secondary id) returned error: %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("len(ResolveWorkspace(repo secondary id)) = %d, want 0 after prune", len(routes))
	}

	session := sessionCatalogRecord{
		SessionID:       "sess-1",
		WorkspaceID:     repoA.ID,
		DatabaseID:      "db-primary",
		WorkspaceName:   "repo",
		ClientKind:      "sync",
		AFSVersion:      "0.1.0",
		Hostname:        "devbox",
		OperatingSystem: "darwin",
		LocalPath:       "/tmp/repo",
		Readonly:        true,
		State:           workspaceSessionStateStarting,
		StartedAt:       "2026-04-17T06:20:00Z",
		LastSeenAt:      "2026-04-17T06:20:30Z",
		LeaseExpiresAt:  "2026-04-17T06:25:30Z",
		UpdatedAt:       "2026-04-17T06:20:30Z",
	}
	if err := store.UpsertSession(ctx, session); err != nil {
		t.Fatalf("UpsertSession() returned error: %v", err)
	}

	sessions, err := store.ListSessionsForWorkspace(ctx, repoA.ID)
	if err != nil {
		t.Fatalf("ListSessionsForWorkspace() returned error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(ListSessionsForWorkspace()) = %d, want 1 active session", len(sessions))
	}
	if !sessions[0].Readonly {
		t.Fatal("session readonly flag should round-trip")
	}

	sessions, err = store.ListSessions(ctx, "db-primary")
	if err != nil {
		t.Fatalf("ListSessions() returned error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(ListSessions(db-primary)) = %d, want 1 active session", len(sessions))
	}

	if err := store.UpsertSession(ctx, sessionCloseRecord(session)); err != nil {
		t.Fatalf("UpsertSession(closed) returned error: %v", err)
	}
	gotSession, err := store.GetSession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetSession() returned error: %v", err)
	}
	if gotSession.State != workspaceSessionStateClosed {
		t.Fatalf("GetSession() state = %q, want %q after close update", gotSession.State, workspaceSessionStateClosed)
	}

	counts, err := store.CountActiveSessionsByDatabase(ctx)
	if err != nil {
		t.Fatalf("CountActiveSessionsByDatabase() returned error: %v", err)
	}
	if got := counts["db-primary"]; got != 0 {
		t.Fatalf("active session count = %d, want 0 after closed session update", got)
	}

	activeSession := session
	activeSession.SessionID = "sess-2"
	activeSession.State = workspaceSessionStateActive
	activeSession.LastSeenAt = "2026-04-17T06:21:00Z"
	activeSession.LeaseExpiresAt = "2026-04-17T06:26:00Z"
	activeSession.UpdatedAt = "2026-04-17T06:21:00Z"
	activeSession.Readonly = false
	if err := store.UpsertSession(ctx, activeSession); err != nil {
		t.Fatalf("UpsertSession(active) returned error: %v", err)
	}

	counts, err = store.CountActiveSessionsByDatabase(ctx)
	if err != nil {
		t.Fatalf("CountActiveSessionsByDatabase() second call returned error: %v", err)
	}
	if got := counts["db-primary"]; got != 1 {
		t.Fatalf("active session count = %d, want 1", got)
	}

	targets, err := store.ListSessionReconcileTargets(ctx)
	if err != nil {
		t.Fatalf("ListSessionReconcileTargets() returned error: %v", err)
	}
	if len(targets) != 1 || targets[0].DatabaseID != "db-primary" || targets[0].WorkspaceName != "repo" {
		t.Fatalf("ListSessionReconcileTargets() = %#v, want db-primary/repo", targets)
	}

	onboarding := onboardingTokenRecord{
		Token:         "afs_otk_contract",
		DatabaseID:    "db-primary",
		WorkspaceID:   repoA.ID,
		WorkspaceName: "repo",
		CreatedAt:     "2026-04-17T06:25:00Z",
		ExpiresAt:     "2026-04-17T06:40:00Z",
	}
	if err := store.CreateOnboardingToken(ctx, onboarding); err != nil {
		t.Fatalf("CreateOnboardingToken() returned error: %v", err)
	}
	consumed, err := store.ConsumeOnboardingToken(ctx, onboarding.Token, "2026-04-17T06:26:00Z")
	if err != nil {
		t.Fatalf("ConsumeOnboardingToken() returned error: %v", err)
	}
	if consumed.WorkspaceID != repoA.ID || consumed.DatabaseID != "db-primary" {
		t.Fatalf("ConsumeOnboardingToken() = %#v, want db-primary/%s", consumed, repoA.ID)
	}
	if _, err := store.ConsumeOnboardingToken(ctx, onboarding.Token, "2026-04-17T06:27:00Z"); !errors.Is(err, ErrOnboardingTokenInvalid) {
		t.Fatalf("second ConsumeOnboardingToken() error = %v, want ErrOnboardingTokenInvalid", err)
	}

	expired := onboardingTokenRecord{
		Token:         "afs_otk_expired",
		DatabaseID:    "db-primary",
		WorkspaceID:   repoA.ID,
		WorkspaceName: "repo",
		CreatedAt:     "2026-04-17T06:25:00Z",
		ExpiresAt:     "2026-04-17T06:25:30Z",
	}
	if err := store.CreateOnboardingToken(ctx, expired); err != nil {
		t.Fatalf("CreateOnboardingToken(expired) returned error: %v", err)
	}
	if _, err := store.ConsumeOnboardingToken(ctx, expired.Token, "2026-04-17T06:26:00Z"); !errors.Is(err, ErrOnboardingTokenInvalid) {
		t.Fatalf("expired ConsumeOnboardingToken() error = %v, want ErrOnboardingTokenInvalid", err)
	}

	refreshAt := time.Date(2026, time.April, 17, 6, 30, 0, 0, time.UTC)
	if err := store.RecordWorkspaceRefresh(ctx, "db-primary", "Primary", refreshAt, nil); err != nil {
		t.Fatalf("RecordWorkspaceRefresh() returned error: %v", err)
	}
	reconcileErr := errors.New("session sync drift")
	if err := store.RecordSessionReconcile(ctx, "db-primary", "Primary", time.Time{}, reconcileErr); err != nil {
		t.Fatalf("RecordSessionReconcile() returned error: %v", err)
	}

	health, err := store.ListDatabaseHealth(ctx)
	if err != nil {
		t.Fatalf("ListDatabaseHealth() returned error: %v", err)
	}
	item, ok := health["db-primary"]
	if !ok {
		t.Fatal("database health missing db-primary")
	}
	if item.LastWorkspaceRefreshAt != refreshAt.Format(time.RFC3339) {
		t.Fatalf("LastWorkspaceRefreshAt = %q, want %q", item.LastWorkspaceRefreshAt, refreshAt.Format(time.RFC3339))
	}
	if item.LastWorkspaceRefreshError != "" {
		t.Fatalf("LastWorkspaceRefreshError = %q, want empty", item.LastWorkspaceRefreshError)
	}
	if item.LastSessionReconcileAt != "" {
		t.Fatalf("LastSessionReconcileAt = %q, want empty on error update", item.LastSessionReconcileAt)
	}
	if item.LastSessionReconcileError != reconcileErr.Error() {
		t.Fatalf("LastSessionReconcileError = %q, want %q", item.LastSessionReconcileError, reconcileErr.Error())
	}
}

func sessionCloseRecord(item sessionCatalogRecord) sessionCatalogRecord {
	item.State = workspaceSessionStateClosed
	item.ClosedAt = "2026-04-17T06:21:30Z"
	item.CloseReason = "done"
	item.UpdatedAt = item.ClosedAt
	item.LastSeenAt = item.ClosedAt
	return item
}

func postgresTestDSNWithDatabase(baseDSN, databaseName string) (string, error) {
	parsed, err := url.Parse(baseDSN)
	if err != nil {
		return "", err
	}
	parsed.Path = "/" + databaseName
	return parsed.String(), nil
}
