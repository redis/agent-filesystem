package controlplane

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type workspaceCatalog struct {
	db *sql.DB
}

type workspaceCatalogRoute struct {
	DatabaseID  string
	WorkspaceID string
	Name        string
}

func openWorkspaceCatalog(configPathOverride string) (*workspaceCatalog, error) {
	path := workspaceCatalogPath(configPathOverride)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path))
	if err != nil {
		return nil, err
	}

	catalog := &workspaceCatalog{db: db}
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
			PRIMARY KEY (database_id, workspace_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workspace_catalog_name ON workspace_catalog(name)`,
		`CREATE INDEX IF NOT EXISTS idx_workspace_catalog_updated_at ON workspace_catalog(updated_at DESC)`,
	}
	for _, statement := range statements {
		if _, err := c.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (c *workspaceCatalog) ReplaceDatabaseWorkspaces(ctx context.Context, databaseID string, items []workspaceSummary) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_catalog WHERE database_id = ?`, strings.TrimSpace(databaseID)); err != nil {
		return err
	}

	statement, err := tx.PrepareContext(ctx, workspaceCatalogUpsertSQL)
	if err != nil {
		return err
	}
	defer statement.Close()

	for _, item := range items {
		if _, err := statement.ExecContext(
			ctx,
			strings.TrimSpace(item.DatabaseID),
			strings.TrimSpace(item.ID),
			strings.TrimSpace(item.Name),
			strings.TrimSpace(item.DatabaseName),
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
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (c *workspaceCatalog) UpsertWorkspace(ctx context.Context, item workspaceSummary) error {
	_, err := c.db.ExecContext(
		ctx,
		workspaceCatalogUpsertSQL,
		strings.TrimSpace(item.DatabaseID),
		strings.TrimSpace(item.ID),
		strings.TrimSpace(item.Name),
		strings.TrimSpace(item.DatabaseName),
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
	)
	return err
}

func (c *workspaceCatalog) DeleteWorkspace(ctx context.Context, databaseID, workspaceID string) error {
	_, err := c.db.ExecContext(ctx, `DELETE FROM workspace_catalog WHERE database_id = ? AND workspace_id = ?`, strings.TrimSpace(databaseID), strings.TrimSpace(workspaceID))
	return err
}

func (c *workspaceCatalog) DeleteDatabaseWorkspaces(ctx context.Context, databaseID string) error {
	_, err := c.db.ExecContext(ctx, `DELETE FROM workspace_catalog WHERE database_id = ?`, strings.TrimSpace(databaseID))
	return err
}

func (c *workspaceCatalog) PruneDatabases(ctx context.Context, keep []string) error {
	if len(keep) == 0 {
		_, err := c.db.ExecContext(ctx, `DELETE FROM workspace_catalog`)
		return err
	}

	placeholders := make([]string, 0, len(keep))
	args := make([]any, 0, len(keep))
	for _, id := range keep {
		placeholders = append(placeholders, "?")
		args = append(args, strings.TrimSpace(id))
	}

	query := `DELETE FROM workspace_catalog WHERE database_id NOT IN (` + strings.Join(placeholders, ", ") + `)`
	_, err := c.db.ExecContext(ctx, query, args...)
	return err
}

func (c *workspaceCatalog) ListWorkspaces(ctx context.Context) ([]workspaceSummary, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT
			workspace_id,
			name,
			cloud_account,
			database_id,
			database_name,
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
			source
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
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (c *workspaceCatalog) ResolveWorkspace(ctx context.Context, workspace string) ([]workspaceCatalogRoute, error) {
	rows, err := c.db.QueryContext(
		ctx,
		`SELECT database_id, workspace_id, name
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
		if err := rows.Scan(&route.DatabaseID, &route.WorkspaceID, &route.Name); err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, rows.Err()
}

func workspaceCatalogPath(configPathOverride string) string {
	cfgPath := configPath(configPathOverride)
	return filepath.Join(filepath.Dir(cfgPath), "afs.catalog.sqlite")
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
		ID:               detail.ID,
		Name:             detail.Name,
		CloudAccount:     detail.CloudAccount,
		DatabaseID:       detail.DatabaseID,
		DatabaseName:     detail.DatabaseName,
		RedisKey:         detail.RedisKey,
		Status:           detail.Status,
		FileCount:        detail.FileCount,
		FolderCount:      detail.FolderCount,
		TotalBytes:       detail.TotalBytes,
		CheckpointCount:  detail.CheckpointCount,
		DraftState:       detail.DraftState,
		LastCheckpointAt: lastCheckpointAt,
		UpdatedAt:        detail.UpdatedAt,
		Region:           detail.Region,
		Source:           detail.Source,
	}
}

const workspaceCatalogUpsertSQL = `INSERT INTO workspace_catalog (
	database_id,
	workspace_id,
	name,
	database_name,
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
	source
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(database_id, workspace_id) DO UPDATE SET
	name = excluded.name,
	database_name = excluded.database_name,
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
	source = excluded.source`
