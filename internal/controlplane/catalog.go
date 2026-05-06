package controlplane

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type workspaceCatalog struct {
	db      *sql.DB
	dialect catalogSQLDialect
}

type workspaceCatalogRoute struct {
	DatabaseID   string
	WorkspaceID  string
	Name         string
	OwnerSubject string
}

func openWorkspaceCatalog(configPathOverride string) (*workspaceCatalog, error) {
	path := catalogStorePath(configPathOverride)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path))
	if err != nil {
		return nil, err
	}

	catalog := &workspaceCatalog{db: db, dialect: catalogSQLDialectSQLite}
	if err := catalog.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return catalog, nil
}

func (c *workspaceCatalog) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

func (c *workspaceCatalog) migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS workspace_catalog (
			database_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			name TEXT NOT NULL,
			database_name TEXT NOT NULL DEFAULT '',
			owner_subject TEXT NOT NULL DEFAULT '',
			owner_label TEXT NOT NULL DEFAULT '',
			database_management_type TEXT NOT NULL DEFAULT '',
			cloud_account TEXT NOT NULL DEFAULT '',
			redis_key TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			file_count INTEGER NOT NULL DEFAULT 0,
			folder_count INTEGER NOT NULL DEFAULT 0,
			total_bytes INTEGER NOT NULL DEFAULT 0,
			checkpoint_count INTEGER NOT NULL DEFAULT 0,
			draft_state TEXT NOT NULL DEFAULT '',
			last_checkpoint_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT '',
			region TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			template_slug TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (database_id, workspace_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workspace_catalog_name ON workspace_catalog(name)`,
		`CREATE INDEX IF NOT EXISTS idx_workspace_catalog_database_name ON workspace_catalog(database_id, name)`,
		`CREATE INDEX IF NOT EXISTS idx_workspace_catalog_updated_at ON workspace_catalog(updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS session_catalog (
			session_id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			database_id TEXT NOT NULL,
			workspace_name TEXT NOT NULL DEFAULT '',
			agent_id TEXT NOT NULL DEFAULT '',
			agent_name TEXT NOT NULL DEFAULT '',
			session_name TEXT NOT NULL DEFAULT '',
			label TEXT NOT NULL DEFAULT '',
			client_kind TEXT NOT NULL DEFAULT '',
			afs_version TEXT NOT NULL DEFAULT '',
			hostname TEXT NOT NULL DEFAULT '',
			os TEXT NOT NULL DEFAULT '',
			local_path TEXT NOT NULL DEFAULT '',
			readonly INTEGER NOT NULL DEFAULT 0,
			state TEXT NOT NULL,
			started_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			lease_expires_at TEXT NOT NULL,
			closed_at TEXT NOT NULL DEFAULT '',
			close_reason TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_session_catalog_workspace_id ON session_catalog(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_session_catalog_database_id ON session_catalog(database_id)`,
		`CREATE INDEX IF NOT EXISTS idx_session_catalog_state_lease ON session_catalog(state, lease_expires_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_session_catalog_last_seen ON session_catalog(last_seen_at DESC)`,
		`CREATE TABLE IF NOT EXISTS database_catalog_health (
			database_id TEXT PRIMARY KEY,
			database_name TEXT NOT NULL DEFAULT '',
			last_workspace_refresh_at TEXT NOT NULL DEFAULT '',
			last_workspace_refresh_error TEXT NOT NULL DEFAULT '',
			last_session_reconcile_at TEXT NOT NULL DEFAULT '',
			last_session_reconcile_error TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS database_registry (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			owner_subject TEXT NOT NULL DEFAULT '',
			owner_label TEXT NOT NULL DEFAULT '',
			management_type TEXT NOT NULL DEFAULT '',
			purpose TEXT NOT NULL DEFAULT '',
			redis_addr TEXT NOT NULL,
			redis_username TEXT NOT NULL DEFAULT '',
			redis_password TEXT NOT NULL DEFAULT '',
			redis_db INTEGER NOT NULL DEFAULT 0,
			redis_tls INTEGER NOT NULL DEFAULT 0,
			is_default INTEGER NOT NULL DEFAULT 0,
			order_index INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_database_registry_order ON database_registry(order_index ASC)`,
		`CREATE TABLE IF NOT EXISTS onboarding_tokens (
			token TEXT PRIMARY KEY,
			owner_subject TEXT NOT NULL DEFAULT '',
			owner_label TEXT NOT NULL DEFAULT '',
			database_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			workspace_name TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			consumed_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_onboarding_tokens_expires_at ON onboarding_tokens(expires_at)`,
		`CREATE TABLE IF NOT EXISTS cli_access_tokens (
			id TEXT PRIMARY KEY,
			owner_subject TEXT NOT NULL DEFAULT '',
			owner_label TEXT NOT NULL DEFAULT '',
			database_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			workspace_name TEXT NOT NULL DEFAULT '',
			secret_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			last_used_at TEXT NOT NULL DEFAULT '',
			expires_at TEXT NOT NULL DEFAULT '',
			revoked_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cli_access_tokens_owner ON cli_access_tokens(owner_subject, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS mcp_access_tokens (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			owner_subject TEXT NOT NULL DEFAULT '',
			owner_label TEXT NOT NULL DEFAULT '',
			database_id TEXT NOT NULL DEFAULT '',
			workspace_id TEXT NOT NULL DEFAULT '',
			workspace_name TEXT NOT NULL DEFAULT '',
			scope TEXT NOT NULL DEFAULT '',
			profile TEXT NOT NULL DEFAULT '',
			template_slug TEXT NOT NULL DEFAULT '',
			readonly INTEGER NOT NULL DEFAULT 0,
			secret_hash TEXT NOT NULL,
			secret TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			last_used_at TEXT NOT NULL DEFAULT '',
			expires_at TEXT NOT NULL DEFAULT '',
			revoked_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_access_tokens_workspace ON mcp_access_tokens(database_id, workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_access_tokens_owner ON mcp_access_tokens(owner_subject, created_at DESC)`,
		// idx_mcp_access_tokens_scope is created in the alter/backfill block
		// so legacy DBs (which don't have the scope column yet) can add the
		// column first; indexing happens right after.
	}
	for _, statement := range statements {
		if _, err := c.execContext(ctx, statement); err != nil {
			return err
		}
	}
	if c.dialect == catalogSQLDialectPostgres {
		alterations := []string{
			`ALTER TABLE database_registry ADD COLUMN IF NOT EXISTS is_default INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE database_registry ADD COLUMN IF NOT EXISTS owner_subject TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE database_registry ADD COLUMN IF NOT EXISTS owner_label TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE database_registry ADD COLUMN IF NOT EXISTS management_type TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE database_registry ADD COLUMN IF NOT EXISTS purpose TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE workspace_catalog ADD COLUMN IF NOT EXISTS owner_subject TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE workspace_catalog ADD COLUMN IF NOT EXISTS owner_label TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE workspace_catalog ADD COLUMN IF NOT EXISTS database_management_type TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE session_catalog ADD COLUMN IF NOT EXISTS agent_id TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE session_catalog ADD COLUMN IF NOT EXISTS agent_name TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE session_catalog ADD COLUMN IF NOT EXISTS session_name TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE session_catalog ADD COLUMN IF NOT EXISTS label TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE onboarding_tokens ADD COLUMN IF NOT EXISTS owner_subject TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE onboarding_tokens ADD COLUMN IF NOT EXISTS owner_label TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE mcp_access_tokens ADD COLUMN IF NOT EXISTS profile TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE workspace_catalog ADD COLUMN IF NOT EXISTS template_slug TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE mcp_access_tokens ADD COLUMN IF NOT EXISTS template_slug TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE mcp_access_tokens ADD COLUMN IF NOT EXISTS secret TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE mcp_access_tokens ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT ''`,
			`UPDATE mcp_access_tokens SET scope = 'workspace:' || workspace_id WHERE scope = '' AND workspace_id <> ''`,
			`CREATE INDEX IF NOT EXISTS idx_mcp_access_tokens_scope ON mcp_access_tokens(scope)`,
		}
		for _, statement := range alterations {
			if _, err := c.execContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	}
	sqliteAlterations := []string{
		`ALTER TABLE database_registry ADD COLUMN is_default INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE database_registry ADD COLUMN owner_subject TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE database_registry ADD COLUMN owner_label TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE database_registry ADD COLUMN management_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE database_registry ADD COLUMN purpose TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE workspace_catalog ADD COLUMN owner_subject TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE workspace_catalog ADD COLUMN owner_label TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE workspace_catalog ADD COLUMN database_management_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE session_catalog ADD COLUMN agent_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE session_catalog ADD COLUMN agent_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE session_catalog ADD COLUMN session_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE session_catalog ADD COLUMN label TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE onboarding_tokens ADD COLUMN owner_subject TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE onboarding_tokens ADD COLUMN owner_label TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE mcp_access_tokens ADD COLUMN profile TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE workspace_catalog ADD COLUMN template_slug TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE mcp_access_tokens ADD COLUMN template_slug TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE mcp_access_tokens ADD COLUMN secret TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE mcp_access_tokens ADD COLUMN scope TEXT NOT NULL DEFAULT ''`,
	}
	for _, statement := range sqliteAlterations {
		if _, err := c.execContext(ctx, statement); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return err
		}
	}
	sqliteBackfills := []string{
		`UPDATE mcp_access_tokens SET scope = 'workspace:' || workspace_id WHERE scope = '' AND workspace_id <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_access_tokens_scope ON mcp_access_tokens(scope)`,
	}
	for _, statement := range sqliteBackfills {
		if _, err := c.execContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (c *workspaceCatalog) ReplaceDatabaseWorkspaces(ctx context.Context, databaseID string, items []workspaceSummary) ([]workspaceSummary, error) {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	statement, err := tx.PrepareContext(ctx, c.rebind(workspaceCatalogUpsertSQL))
	if err != nil {
		return nil, err
	}
	defer statement.Close()

	existingByIdentity, err := catalogRoutesByIdentity(ctx, tx, c.rebind, databaseID)
	if err != nil {
		return nil, err
	}
	existingTemplateSlugs, err := catalogTemplateSlugsByID(ctx, tx, c.rebind, databaseID)
	if err != nil {
		return nil, err
	}
	keepIDs := make(map[string]struct{}, len(items))
	synced := make([]workspaceSummary, 0, len(items))
	for _, item := range items {
		item.DatabaseID = strings.TrimSpace(databaseID)
		if strings.TrimSpace(item.TemplateSlug) == "" {
			if existing, ok := existingTemplateSlugs[strings.TrimSpace(item.ID)]; ok {
				item.TemplateSlug = existing
			}
		}
		item, err = upsertCatalogWorkspaceSummary(ctx, tx, statement, c.rebind, existingByIdentity, item)
		if err != nil {
			return nil, err
		}
		keepIDs[item.ID] = struct{}{}
		synced = append(synced, item)
	}

	if err := deleteCatalogDatabaseRowsNotInIDs(ctx, tx, c.rebind, databaseID, keepIDs); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return synced, nil
}

func (c *workspaceCatalog) UpsertWorkspace(ctx context.Context, item workspaceSummary) (workspaceSummary, error) {
	statement, err := c.prepareContext(ctx, workspaceCatalogUpsertSQL)
	if err != nil {
		return workspaceSummary{}, err
	}
	defer statement.Close()

	existingByIdentity, err := catalogRoutesByIdentity(ctx, c.db, c.rebind, item.DatabaseID)
	if err != nil {
		return workspaceSummary{}, err
	}
	if strings.TrimSpace(item.TemplateSlug) == "" {
		existingTemplateSlugs, err := catalogTemplateSlugsByID(ctx, c.db, c.rebind, item.DatabaseID)
		if err != nil {
			return workspaceSummary{}, err
		}
		if existing, ok := existingTemplateSlugs[strings.TrimSpace(item.ID)]; ok {
			item.TemplateSlug = existing
		}
	}
	return upsertCatalogWorkspaceSummary(ctx, c.db, statement, c.rebind, existingByIdentity, item)
}

func (c *workspaceCatalog) ListWorkspaceOwners(ctx context.Context, databaseID string) (map[string]catalogOwnerInfo, error) {
	if c == nil || c.db == nil {
		return map[string]catalogOwnerInfo{}, nil
	}
	return catalogOwnersByName(ctx, c.db, c.rebind, databaseID)
}

func (c *workspaceCatalog) CountWorkspacesForOwner(ctx context.Context, databaseID, ownerSubject string) (int, error) {
	if c == nil || c.db == nil {
		return 0, nil
	}
	row := c.db.QueryRowContext(
		ctx,
		c.rebind(`SELECT COUNT(*) FROM workspace_catalog WHERE database_id = ? AND owner_subject = ?`),
		strings.TrimSpace(databaseID),
		strings.TrimSpace(ownerSubject),
	)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (c *workspaceCatalog) DeleteWorkspace(ctx context.Context, databaseID, workspaceID string) error {
	_, err := c.execContext(ctx, `DELETE FROM workspace_catalog WHERE database_id = ? AND workspace_id = ?`, strings.TrimSpace(databaseID), strings.TrimSpace(workspaceID))
	return err
}

func (c *workspaceCatalog) DeleteWorkspaceByName(ctx context.Context, databaseID, name string) error {
	_, err := c.execContext(ctx, `DELETE FROM workspace_catalog WHERE database_id = ? AND name = ?`, strings.TrimSpace(databaseID), strings.TrimSpace(name))
	return err
}

func (c *workspaceCatalog) DeleteDatabaseWorkspaces(ctx context.Context, databaseID string) error {
	_, err := c.execContext(ctx, `DELETE FROM workspace_catalog WHERE database_id = ?`, strings.TrimSpace(databaseID))
	return err
}

func (c *workspaceCatalog) PruneDatabases(ctx context.Context, keep []string) error {
	if len(keep) == 0 {
		_, err := c.execContext(ctx, `DELETE FROM workspace_catalog`)
		return err
	}

	placeholders := make([]string, 0, len(keep))
	args := make([]any, 0, len(keep))
	for _, id := range keep {
		placeholders = append(placeholders, "?")
		args = append(args, strings.TrimSpace(id))
	}

	query := `DELETE FROM workspace_catalog WHERE database_id NOT IN (` + strings.Join(placeholders, ", ") + `)`
	_, err := c.execContext(ctx, query, args...)
	return err
}

func (c *workspaceCatalog) ListWorkspaces(ctx context.Context) ([]workspaceSummary, error) {
	rows, err := c.queryContext(ctx, `SELECT
			workspace_id,
			name,
			cloud_account,
			database_id,
			database_name,
			owner_subject,
			owner_label,
			database_management_type,
			redis_key,
			status,
			file_count,
			folder_count,
			total_bytes,
			checkpoint_count,
			draft_state,
			last_checkpoint_at,
			updated_at,
			region,
			source,
			template_slug
		FROM workspace_catalog
		ORDER BY updated_at DESC, lower(name), database_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]workspaceSummary, 0)
	for rows.Next() {
		var item workspaceSummary
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.CloudAccount,
			&item.DatabaseID,
			&item.DatabaseName,
			&item.OwnerSubject,
			&item.OwnerLabel,
			&item.DatabaseManagementType,
			&item.RedisKey,
			&item.Status,
			&item.FileCount,
			&item.FolderCount,
			&item.TotalBytes,
			&item.CheckpointCount,
			&item.DraftState,
			&item.LastCheckpointAt,
			&item.UpdatedAt,
			&item.Region,
			&item.Source,
			&item.TemplateSlug,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (c *workspaceCatalog) ResolveWorkspace(ctx context.Context, workspace string) ([]workspaceCatalogRoute, error) {
	rows, err := c.queryContext(
		ctx,
		`SELECT database_id, workspace_id, name, owner_subject
		 FROM workspace_catalog
		 WHERE workspace_id = ? OR name = ?
		 ORDER BY database_id, workspace_id`,
		strings.TrimSpace(workspace),
		strings.TrimSpace(workspace),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	routes := make([]workspaceCatalogRoute, 0)
	for rows.Next() {
		var route workspaceCatalogRoute
		if err := rows.Scan(&route.DatabaseID, &route.WorkspaceID, &route.Name, &route.OwnerSubject); err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, rows.Err()
}

func (c *workspaceCatalog) ResolveWorkspaceInDatabase(ctx context.Context, databaseID, workspace string) (workspaceCatalogRoute, error) {
	rows, err := c.queryContext(
		ctx,
		`SELECT database_id, workspace_id, name, owner_subject
		 FROM workspace_catalog
		 WHERE database_id = ? AND (workspace_id = ? OR name = ?)
		 ORDER BY workspace_id`,
		strings.TrimSpace(databaseID),
		strings.TrimSpace(workspace),
		strings.TrimSpace(workspace),
	)
	if err != nil {
		return workspaceCatalogRoute{}, err
	}
	defer rows.Close()

	var routes []workspaceCatalogRoute
	for rows.Next() {
		var route workspaceCatalogRoute
		if err := rows.Scan(&route.DatabaseID, &route.WorkspaceID, &route.Name, &route.OwnerSubject); err != nil {
			return workspaceCatalogRoute{}, err
		}
		routes = append(routes, route)
	}
	if err := rows.Err(); err != nil {
		return workspaceCatalogRoute{}, err
	}
	switch len(routes) {
	case 0:
		return workspaceCatalogRoute{}, os.ErrNotExist
	case 1:
		return routes[0], nil
	default:
		return workspaceCatalogRoute{}, fmt.Errorf("workspace %q is ambiguous in database %q", workspace, databaseID)
	}
}

func workspaceCatalogIdentityKey(name, ownerSubject string) string {
	return strings.TrimSpace(ownerSubject) + "\x1f" + strings.TrimSpace(name)
}

func catalogTemplateSlugsByID(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, rebind func(string) string, databaseID string) (map[string]string, error) {
	rows, err := queryer.QueryContext(
		ctx,
		rebind(`SELECT workspace_id, template_slug
		 FROM workspace_catalog
		 WHERE database_id = ?`),
		strings.TrimSpace(databaseID),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	slugs := make(map[string]string)
	for rows.Next() {
		var workspaceID, slug string
		if err := rows.Scan(&workspaceID, &slug); err != nil {
			return nil, err
		}
		if strings.TrimSpace(slug) != "" {
			slugs[workspaceID] = slug
		}
	}
	return slugs, rows.Err()
}

func catalogRoutesByIdentity(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, rebind func(string) string, databaseID string) (map[string][]workspaceCatalogRoute, error) {
	rows, err := queryer.QueryContext(
		ctx,
		rebind(`SELECT database_id, workspace_id, name, owner_subject
		 FROM workspace_catalog
		 WHERE database_id = ?`),
		strings.TrimSpace(databaseID),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	routes := make(map[string][]workspaceCatalogRoute)
	for rows.Next() {
		var route workspaceCatalogRoute
		if err := rows.Scan(&route.DatabaseID, &route.WorkspaceID, &route.Name, &route.OwnerSubject); err != nil {
			return nil, err
		}
		key := workspaceCatalogIdentityKey(route.Name, route.OwnerSubject)
		routes[key] = append(routes[key], route)
	}
	return routes, rows.Err()
}

type catalogOwnerInfo struct {
	Subject string
	Label   string
}

func catalogOwnersByName(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, rebind func(string) string, databaseID string) (map[string]catalogOwnerInfo, error) {
	rows, err := queryer.QueryContext(
		ctx,
		rebind(`SELECT workspace_id, owner_subject, owner_label
		 FROM workspace_catalog
		 WHERE database_id = ?`),
		strings.TrimSpace(databaseID),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	owners := make(map[string]catalogOwnerInfo)
	for rows.Next() {
		var workspaceID, subject, label string
		if err := rows.Scan(&workspaceID, &subject, &label); err != nil {
			return nil, err
		}
		owners[workspaceID] = catalogOwnerInfo{Subject: subject, Label: label}
	}
	return owners, rows.Err()
}

func upsertCatalogWorkspaceSummary(
	ctx context.Context,
	execer interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	},
	statement interface {
		ExecContext(context.Context, ...any) (sql.Result, error)
	},
	rebind func(string) string,
	existingByIdentity map[string][]workspaceCatalogRoute,
	item workspaceSummary,
) (workspaceSummary, error) {
	item.DatabaseID = strings.TrimSpace(item.DatabaseID)
	item.Name = strings.TrimSpace(item.Name)
	if item.DatabaseID == "" {
		return workspaceSummary{}, fmt.Errorf("workspace database id is required")
	}
	if item.Name == "" {
		return workspaceSummary{}, fmt.Errorf("workspace name is required")
	}
	identityKey := workspaceCatalogIdentityKey(item.Name, item.OwnerSubject)

	assignedID := strings.TrimSpace(item.ID)
	if assignedID == "" {
		if existing, ok := existingByIdentity[identityKey]; ok && len(existing) > 0 {
			assignedID = strings.TrimSpace(existing[0].WorkspaceID)
		}
	}
	if assignedID == "" {
		var err error
		assignedID, err = newOpaqueWorkspaceID()
		if err != nil {
			return workspaceSummary{}, err
		}
	}

	if existing, ok := existingByIdentity[identityKey]; ok {
		for _, route := range existing {
			if strings.TrimSpace(route.WorkspaceID) == assignedID {
				continue
			}
			if _, err := execer.ExecContext(
				ctx,
				rebind(`DELETE FROM workspace_catalog WHERE database_id = ? AND workspace_id = ?`),
				item.DatabaseID,
				strings.TrimSpace(route.WorkspaceID),
			); err != nil {
				return workspaceSummary{}, err
			}
		}
	}

	item.ID = assignedID
	if _, err := statement.ExecContext(
		ctx,
		item.DatabaseID,
		item.ID,
		item.Name,
		strings.TrimSpace(item.DatabaseName),
		strings.TrimSpace(item.OwnerSubject),
		strings.TrimSpace(item.OwnerLabel),
		strings.TrimSpace(item.DatabaseManagementType),
		strings.TrimSpace(item.CloudAccount),
		strings.TrimSpace(item.RedisKey),
		strings.TrimSpace(item.Status),
		item.FileCount,
		item.FolderCount,
		item.TotalBytes,
		item.CheckpointCount,
		strings.TrimSpace(item.DraftState),
		strings.TrimSpace(item.LastCheckpointAt),
		strings.TrimSpace(item.UpdatedAt),
		strings.TrimSpace(item.Region),
		strings.TrimSpace(item.Source),
		strings.TrimSpace(item.TemplateSlug),
	); err != nil {
		return workspaceSummary{}, err
	}
	return item, nil
}

func deleteCatalogDatabaseRowsNotInIDs(
	ctx context.Context,
	execer interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	},
	rebind func(string) string,
	databaseID string,
	keepIDs map[string]struct{},
) error {
	databaseID = strings.TrimSpace(databaseID)
	if len(keepIDs) == 0 {
		_, err := execer.ExecContext(ctx, rebind(`DELETE FROM workspace_catalog WHERE database_id = ?`), databaseID)
		return err
	}

	placeholders := make([]string, 0, len(keepIDs))
	args := make([]any, 0, len(keepIDs)+1)
	args = append(args, databaseID)
	for workspaceID := range keepIDs {
		placeholders = append(placeholders, "?")
		args = append(args, workspaceID)
	}
	query := `DELETE FROM workspace_catalog WHERE database_id = ? AND workspace_id NOT IN (` + strings.Join(placeholders, ", ") + `)`
	_, err := execer.ExecContext(ctx, rebind(query), args...)
	return err
}

func newOpaqueWorkspaceID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "ws_" + hex.EncodeToString(raw[:]), nil
}

func workspaceCatalogPath(configPathOverride string) string {
	return catalogStorePath(configPathOverride)
}

func workspaceSummaryFromDetail(detail workspaceDetail) workspaceSummary {
	lastCheckpointAt := ""
	for _, checkpoint := range detail.Checkpoints {
		if checkpoint.IsHead || checkpoint.ID == detail.HeadCheckpointID {
			lastCheckpointAt = checkpoint.CreatedAt
			break
		}
	}

	return workspaceSummary{
		ID:                     detail.ID,
		Name:                   detail.Name,
		CloudAccount:           detail.CloudAccount,
		DatabaseID:             detail.DatabaseID,
		DatabaseName:           detail.DatabaseName,
		OwnerSubject:           detail.OwnerSubject,
		OwnerLabel:             detail.OwnerLabel,
		DatabaseManagementType: detail.DatabaseManagementType,
		DatabaseCanEdit:        detail.DatabaseCanEdit,
		DatabaseCanDelete:      detail.DatabaseCanDelete,
		RedisKey:               detail.RedisKey,
		Status:                 detail.Status,
		FileCount:              detail.FileCount,
		FolderCount:            detail.FolderCount,
		TotalBytes:             detail.TotalBytes,
		CheckpointCount:        detail.CheckpointCount,
		DraftState:             detail.DraftState,
		LastCheckpointAt:       lastCheckpointAt,
		UpdatedAt:              detail.UpdatedAt,
		Region:                 detail.Region,
		Source:                 detail.Source,
		TemplateSlug:           detail.TemplateSlug,
	}
}

const workspaceCatalogUpsertSQL = `INSERT INTO workspace_catalog (
	database_id,
	workspace_id,
	name,
	database_name,
	owner_subject,
	owner_label,
	database_management_type,
	cloud_account,
	redis_key,
	status,
	file_count,
	folder_count,
	total_bytes,
	checkpoint_count,
	draft_state,
	last_checkpoint_at,
	updated_at,
	region,
	source,
	template_slug
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(database_id, workspace_id) DO UPDATE SET
	name = excluded.name,
	database_name = excluded.database_name,
	owner_subject = CASE WHEN excluded.owner_subject = '' THEN workspace_catalog.owner_subject ELSE excluded.owner_subject END,
	owner_label = CASE WHEN excluded.owner_label = '' THEN workspace_catalog.owner_label ELSE excluded.owner_label END,
	database_management_type = excluded.database_management_type,
	cloud_account = excluded.cloud_account,
	redis_key = excluded.redis_key,
	status = excluded.status,
	file_count = excluded.file_count,
	folder_count = excluded.folder_count,
	total_bytes = excluded.total_bytes,
	checkpoint_count = excluded.checkpoint_count,
	draft_state = excluded.draft_state,
	last_checkpoint_at = excluded.last_checkpoint_at,
	updated_at = excluded.updated_at,
	region = excluded.region,
	source = excluded.source,
	template_slug = CASE WHEN excluded.template_slug = '' THEN workspace_catalog.template_slug ELSE excluded.template_slug END`
