package controlplane

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	catalogDriverEnvVar = "AFS_CATALOG_DRIVER"
	catalogDSNEnvVar    = "AFS_CATALOG_DSN"
	catalogPathEnvVar   = "AFS_CATALOG_PATH"

	catalogDriverSQLite   = "sqlite"
	catalogDriverPostgres = "postgres"
)

// catalogStore is the persistence boundary for the control-plane metadata
// catalog. SQLite remains the default implementation for local/self-hosted
// usage, but Vercel-hosted deployments will need a different backing store.
type catalogStore interface {
	Close() error

	ReplaceDatabaseWorkspaces(ctx context.Context, databaseID string, items []workspaceSummary) ([]workspaceSummary, error)
	UpsertWorkspace(ctx context.Context, item workspaceSummary) (workspaceSummary, error)
	DeleteWorkspace(ctx context.Context, databaseID, workspaceID string) error
	DeleteWorkspaceByName(ctx context.Context, databaseID, name string) error
	DeleteDatabaseWorkspaces(ctx context.Context, databaseID string) error
	PruneDatabases(ctx context.Context, keep []string) error
	ListWorkspaces(ctx context.Context) ([]workspaceSummary, error)
	ListWorkspaceOwners(ctx context.Context, databaseID string) (map[string]catalogOwnerInfo, error)
	CountWorkspacesForOwner(ctx context.Context, databaseID, ownerSubject string) (int, error)
	ResolveWorkspace(ctx context.Context, workspace string) ([]workspaceCatalogRoute, error)
	ResolveWorkspaceInDatabase(ctx context.Context, databaseID, workspace string) (workspaceCatalogRoute, error)

	UpsertSession(ctx context.Context, item sessionCatalogRecord) error
	DeleteSessionsForWorkspace(ctx context.Context, workspaceID string) error
	ListSessionsForWorkspace(ctx context.Context, workspaceID string) ([]sessionCatalogRecord, error)
	ListSessions(ctx context.Context, databaseID string) ([]sessionCatalogRecord, error)
	GetSession(ctx context.Context, sessionID string) (sessionCatalogRecord, error)

	RecordWorkspaceRefresh(ctx context.Context, databaseID, databaseName string, refreshedAt time.Time, refreshErr error) error
	RecordSessionReconcile(ctx context.Context, databaseID, databaseName string, reconciledAt time.Time, reconcileErr error) error
	ListDatabaseHealth(ctx context.Context) (map[string]databaseCatalogHealth, error)
	CountActiveSessionsByDatabase(ctx context.Context) (map[string]int, error)
	ListSessionReconcileTargets(ctx context.Context) ([]sessionReconcileTarget, error)

	ListDatabaseProfiles(ctx context.Context) ([]databaseProfile, error)
	ReplaceDatabaseProfiles(ctx context.Context, profiles []databaseProfile) error

	CreateOnboardingToken(ctx context.Context, item onboardingTokenRecord) error
	ConsumeOnboardingToken(ctx context.Context, token, consumedAt string) (onboardingTokenRecord, error)

	CreateCLIAccessToken(ctx context.Context, item cliAccessTokenRecord) error
	GetCLIAccessToken(ctx context.Context, tokenID string) (cliAccessTokenRecord, error)
	TouchCLIAccessToken(ctx context.Context, tokenID, lastUsedAt string) error
	RevokeCLIAccessTokensByOwner(ctx context.Context, ownerSubject, revokedAt string) error

	CreateMCPAccessToken(ctx context.Context, item mcpAccessTokenRecord) error
	ListAllMCPAccessTokens(ctx context.Context) ([]mcpAccessTokenRecord, error)
	ListMCPAccessTokens(ctx context.Context, databaseID, workspaceID string) ([]mcpAccessTokenRecord, error)
	ListControlPlaneMCPAccessTokens(ctx context.Context) ([]mcpAccessTokenRecord, error)
	GetMCPAccessToken(ctx context.Context, tokenID string) (mcpAccessTokenRecord, error)
	TouchMCPAccessToken(ctx context.Context, tokenID, lastUsedAt string) error
	RevokeMCPAccessToken(ctx context.Context, tokenID, databaseID, workspaceID, revokedAt string) error
	RevokeMCPAccessTokenByID(ctx context.Context, tokenID, revokedAt string) error
}

var _ catalogStore = (*workspaceCatalog)(nil)

func openCatalogStore(configPathOverride string) (catalogStore, error) {
	switch driver := catalogDriverName(); driver {
	case "", catalogDriverSQLite:
		return openWorkspaceCatalog(configPathOverride)
	case catalogDriverPostgres:
		return openPostgresCatalog()
	default:
		return nil, fmt.Errorf("unsupported %s %q", catalogDriverEnvVar, driver)
	}
}

func catalogDriverName() string {
	return strings.ToLower(strings.TrimSpace(os.Getenv(catalogDriverEnvVar)))
}

func catalogStorePath(configPathOverride string) string {
	if env := strings.TrimSpace(os.Getenv(catalogPathEnvVar)); env != "" {
		return filepath.Clean(env)
	}
	cfgPath := configPath(configPathOverride)
	return filepath.Join(filepath.Dir(cfgPath), "afs.catalog.sqlite")
}
