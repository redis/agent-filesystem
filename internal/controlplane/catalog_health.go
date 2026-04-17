package controlplane

import (
	"context"
	"strings"
	"time"
)

type databaseCatalogHealth struct {
	DatabaseID                string
	DatabaseName              string
	LastWorkspaceRefreshAt    string
	LastWorkspaceRefreshError string
	LastSessionReconcileAt    string
	LastSessionReconcileError string
	UpdatedAt                 string
}

type sessionReconcileTarget struct {
	DatabaseID    string
	WorkspaceName string
}

func (c *workspaceCatalog) RecordWorkspaceRefresh(ctx context.Context, databaseID, databaseName string, refreshedAt time.Time, refreshErr error) error {
	if c == nil || c.db == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	refreshAt := ""
	refreshError := ""
	if refreshErr == nil {
		refreshAt = refreshedAt.UTC().Format(time.RFC3339)
	} else {
		refreshError = strings.TrimSpace(refreshErr.Error())
	}
	_, err := c.execContext(
		ctx,
		databaseWorkspaceHealthUpsertSQL,
		strings.TrimSpace(databaseID),
		strings.TrimSpace(databaseName),
		refreshAt,
		refreshError,
		now,
	)
	return err
}

func (c *workspaceCatalog) RecordSessionReconcile(ctx context.Context, databaseID, databaseName string, reconciledAt time.Time, reconcileErr error) error {
	if c == nil || c.db == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	reconcileAt := ""
	reconcileError := ""
	if reconcileErr == nil {
		reconcileAt = reconciledAt.UTC().Format(time.RFC3339)
	} else {
		reconcileError = strings.TrimSpace(reconcileErr.Error())
	}
	_, err := c.execContext(
		ctx,
		databaseSessionHealthUpsertSQL,
		strings.TrimSpace(databaseID),
		strings.TrimSpace(databaseName),
		reconcileAt,
		reconcileError,
		now,
	)
	return err
}

func (c *workspaceCatalog) ListDatabaseHealth(ctx context.Context) (map[string]databaseCatalogHealth, error) {
	if c == nil || c.db == nil {
		return map[string]databaseCatalogHealth{}, nil
	}
	rows, err := c.queryContext(
		ctx,
		`SELECT
			database_id,
			database_name,
			last_workspace_refresh_at,
			last_workspace_refresh_error,
			last_session_reconcile_at,
			last_session_reconcile_error,
			updated_at
		FROM database_catalog_health`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make(map[string]databaseCatalogHealth)
	for rows.Next() {
		var item databaseCatalogHealth
		if err := rows.Scan(
			&item.DatabaseID,
			&item.DatabaseName,
			&item.LastWorkspaceRefreshAt,
			&item.LastWorkspaceRefreshError,
			&item.LastSessionReconcileAt,
			&item.LastSessionReconcileError,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items[item.DatabaseID] = item
	}
	return items, rows.Err()
}

func (c *workspaceCatalog) CountActiveSessionsByDatabase(ctx context.Context) (map[string]int, error) {
	if c == nil || c.db == nil {
		return map[string]int{}, nil
	}
	rows, err := c.queryContext(
		ctx,
		`SELECT database_id, COUNT(*)
		 FROM session_catalog
		 WHERE state IN (?, ?)
		 GROUP BY database_id`,
		workspaceSessionStateStarting,
		workspaceSessionStateActive,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var databaseID string
		var count int
		if err := rows.Scan(&databaseID, &count); err != nil {
			return nil, err
		}
		counts[databaseID] = count
	}
	return counts, rows.Err()
}

func (c *workspaceCatalog) ListSessionReconcileTargets(ctx context.Context) ([]sessionReconcileTarget, error) {
	if c == nil || c.db == nil {
		return nil, nil
	}
	rows, err := c.queryContext(
		ctx,
		`SELECT DISTINCT database_id, workspace_name
		 FROM session_catalog
		 WHERE state IN (?, ?)
		 ORDER BY database_id, workspace_name`,
		workspaceSessionStateStarting,
		workspaceSessionStateActive,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]sessionReconcileTarget, 0)
	for rows.Next() {
		var item sessionReconcileTarget
		if err := rows.Scan(&item.DatabaseID, &item.WorkspaceName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const databaseWorkspaceHealthUpsertSQL = `INSERT INTO database_catalog_health (
	database_id,
	database_name,
	last_workspace_refresh_at,
	last_workspace_refresh_error,
	last_session_reconcile_at,
	last_session_reconcile_error,
	updated_at
) VALUES (?, ?, ?, ?, '', '', ?)
ON CONFLICT(database_id) DO UPDATE SET
	database_name = excluded.database_name,
	last_workspace_refresh_at = CASE
		WHEN excluded.last_workspace_refresh_error = '' THEN excluded.last_workspace_refresh_at
		ELSE database_catalog_health.last_workspace_refresh_at
	END,
	last_workspace_refresh_error = excluded.last_workspace_refresh_error,
	updated_at = excluded.updated_at`

const databaseSessionHealthUpsertSQL = `INSERT INTO database_catalog_health (
	database_id,
	database_name,
	last_workspace_refresh_at,
	last_workspace_refresh_error,
	last_session_reconcile_at,
	last_session_reconcile_error,
	updated_at
) VALUES (?, ?, '', '', ?, ?, ?)
ON CONFLICT(database_id) DO UPDATE SET
	database_name = excluded.database_name,
	last_session_reconcile_at = CASE
		WHEN excluded.last_session_reconcile_error = '' THEN excluded.last_session_reconcile_at
		ELSE database_catalog_health.last_session_reconcile_at
	END,
	last_session_reconcile_error = excluded.last_session_reconcile_error,
	updated_at = excluded.updated_at`
