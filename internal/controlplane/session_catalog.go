package controlplane

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"
)

type sessionCatalogRecord struct {
	SessionID       string
	WorkspaceID     string
	DatabaseID      string
	WorkspaceName   string
	AgentID         string
	ClientKind      string
	AFSVersion      string
	Hostname        string
	OperatingSystem string
	LocalPath       string
	Readonly        bool
	State           string
	StartedAt       string
	LastSeenAt      string
	LeaseExpiresAt  string
	ClosedAt        string
	CloseReason     string
	UpdatedAt       string
}

func (c *workspaceCatalog) UpsertSession(ctx context.Context, item sessionCatalogRecord) error {
	if c == nil || c.db == nil {
		return nil
	}
	if strings.TrimSpace(item.SessionID) == "" {
		return fmt.Errorf("session id is required")
	}
	if strings.TrimSpace(item.WorkspaceID) == "" {
		return fmt.Errorf("workspace id is required")
	}
	if strings.TrimSpace(item.DatabaseID) == "" {
		return fmt.Errorf("database id is required")
	}
	if strings.TrimSpace(item.State) == "" {
		return fmt.Errorf("session state is required")
	}
	if strings.TrimSpace(item.StartedAt) == "" {
		return fmt.Errorf("session started_at is required")
	}
	if strings.TrimSpace(item.LastSeenAt) == "" {
		return fmt.Errorf("session last_seen_at is required")
	}
	if strings.TrimSpace(item.LeaseExpiresAt) == "" {
		return fmt.Errorf("session lease_expires_at is required")
	}
	if strings.TrimSpace(item.UpdatedAt) == "" {
		return fmt.Errorf("session updated_at is required")
	}

	readonly := 0
	if item.Readonly {
		readonly = 1
	}

	_, err := c.execContext(
		ctx,
		sessionCatalogUpsertSQL,
		strings.TrimSpace(item.SessionID),
		strings.TrimSpace(item.WorkspaceID),
		strings.TrimSpace(item.DatabaseID),
		strings.TrimSpace(item.WorkspaceName),
		strings.TrimSpace(item.AgentID),
		strings.TrimSpace(item.ClientKind),
		strings.TrimSpace(item.AFSVersion),
		strings.TrimSpace(item.Hostname),
		strings.TrimSpace(item.OperatingSystem),
		strings.TrimSpace(item.LocalPath),
		readonly,
		strings.TrimSpace(item.State),
		strings.TrimSpace(item.StartedAt),
		strings.TrimSpace(item.LastSeenAt),
		strings.TrimSpace(item.LeaseExpiresAt),
		strings.TrimSpace(item.ClosedAt),
		strings.TrimSpace(item.CloseReason),
		strings.TrimSpace(item.UpdatedAt),
	)
	return err
}

func (c *workspaceCatalog) ListSessionsForWorkspace(ctx context.Context, workspaceID string) ([]sessionCatalogRecord, error) {
	if c == nil || c.db == nil {
		return nil, nil
	}
	rows, err := c.queryContext(
		ctx,
		`SELECT
			session_id,
			workspace_id,
			database_id,
			workspace_name,
			agent_id,
			client_kind,
			afs_version,
			hostname,
			os,
			local_path,
			readonly,
			state,
			started_at,
			last_seen_at,
			lease_expires_at,
			closed_at,
			close_reason,
			updated_at
		FROM session_catalog
		WHERE workspace_id = ?
		  AND state IN (?, ?)
		ORDER BY last_seen_at DESC, session_id`,
		strings.TrimSpace(workspaceID),
		workspaceSessionStateStarting,
		workspaceSessionStateActive,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]sessionCatalogRecord, 0)
	for rows.Next() {
		var item sessionCatalogRecord
		var readonly int
		if err := rows.Scan(
			&item.SessionID,
			&item.WorkspaceID,
			&item.DatabaseID,
			&item.WorkspaceName,
			&item.AgentID,
			&item.ClientKind,
			&item.AFSVersion,
			&item.Hostname,
			&item.OperatingSystem,
			&item.LocalPath,
			&readonly,
			&item.State,
			&item.StartedAt,
			&item.LastSeenAt,
			&item.LeaseExpiresAt,
			&item.ClosedAt,
			&item.CloseReason,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Readonly = readonly != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

func (c *workspaceCatalog) DeleteSessionsForWorkspace(ctx context.Context, workspaceID string) error {
	if c == nil || c.db == nil {
		return nil
	}
	resolvedWorkspaceID := strings.TrimSpace(workspaceID)
	if resolvedWorkspaceID == "" {
		return nil
	}
	_, err := c.execContext(
		ctx,
		`DELETE FROM session_catalog WHERE workspace_id = ?`,
		resolvedWorkspaceID,
	)
	return err
}

func (c *workspaceCatalog) ListSessions(ctx context.Context, databaseID string) ([]sessionCatalogRecord, error) {
	if c == nil || c.db == nil {
		return nil, nil
	}

	resolvedDatabaseID := strings.TrimSpace(databaseID)
	query := `SELECT
			session_id,
			workspace_id,
			database_id,
			workspace_name,
			agent_id,
			client_kind,
			afs_version,
			hostname,
			os,
			local_path,
			readonly,
			state,
			started_at,
			last_seen_at,
			lease_expires_at,
			closed_at,
			close_reason,
			updated_at
		FROM session_catalog
		WHERE state IN (?, ?)`
	args := []any{
		workspaceSessionStateStarting,
		workspaceSessionStateActive,
	}
	if resolvedDatabaseID != "" {
		query += ` AND database_id = ?`
		args = append(args, resolvedDatabaseID)
	}
	query += ` ORDER BY last_seen_at DESC, session_id`

	rows, err := c.queryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]sessionCatalogRecord, 0)
	for rows.Next() {
		var item sessionCatalogRecord
		var readonly int
		if err := rows.Scan(
			&item.SessionID,
			&item.WorkspaceID,
			&item.DatabaseID,
			&item.WorkspaceName,
			&item.AgentID,
			&item.ClientKind,
			&item.AFSVersion,
			&item.Hostname,
			&item.OperatingSystem,
			&item.LocalPath,
			&readonly,
			&item.State,
			&item.StartedAt,
			&item.LastSeenAt,
			&item.LeaseExpiresAt,
			&item.ClosedAt,
			&item.CloseReason,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Readonly = readonly != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

func (c *workspaceCatalog) GetSession(ctx context.Context, sessionID string) (sessionCatalogRecord, error) {
	if c == nil || c.db == nil {
		return sessionCatalogRecord{}, os.ErrNotExist
	}
	row := c.queryRowContext(
		ctx,
		`SELECT
			session_id,
			workspace_id,
			database_id,
			workspace_name,
			agent_id,
			client_kind,
			afs_version,
			hostname,
			os,
			local_path,
			readonly,
			state,
			started_at,
			last_seen_at,
			lease_expires_at,
			closed_at,
			close_reason,
			updated_at
		FROM session_catalog
		WHERE session_id = ?`,
		strings.TrimSpace(sessionID),
	)

	var item sessionCatalogRecord
	var readonly int
	if err := row.Scan(
		&item.SessionID,
		&item.WorkspaceID,
		&item.DatabaseID,
		&item.WorkspaceName,
		&item.AgentID,
		&item.ClientKind,
		&item.AFSVersion,
		&item.Hostname,
		&item.OperatingSystem,
		&item.LocalPath,
		&readonly,
		&item.State,
		&item.StartedAt,
		&item.LastSeenAt,
		&item.LeaseExpiresAt,
		&item.ClosedAt,
		&item.CloseReason,
		&item.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return sessionCatalogRecord{}, os.ErrNotExist
		}
		return sessionCatalogRecord{}, err
	}
	item.Readonly = readonly != 0
	return item, nil
}

func workspaceSessionCatalogRecord(route workspaceCatalogRoute, meta WorkspaceMeta, record WorkspaceSessionRecord, closeReason string) sessionCatalogRecord {
	closedAt := ""
	if record.State == workspaceSessionStateClosed || record.State == workspaceSessionStateStale {
		closedAt = record.LastSeenAt.UTC().Format(timeRFC3339)
	}
	return sessionCatalogRecord{
		SessionID:       record.SessionID,
		WorkspaceID:     route.WorkspaceID,
		DatabaseID:      route.DatabaseID,
		WorkspaceName:   defaultString(route.Name, meta.Name),
		AgentID:         record.AgentID,
		ClientKind:      record.ClientKind,
		AFSVersion:      record.AFSVersion,
		Hostname:        record.Hostname,
		OperatingSystem: record.OperatingSystem,
		LocalPath:       record.LocalPath,
		Readonly:        record.Readonly,
		State:           record.State,
		StartedAt:       record.StartedAt.UTC().Format(timeRFC3339),
		LastSeenAt:      record.LastSeenAt.UTC().Format(timeRFC3339),
		LeaseExpiresAt:  record.LeaseExpiresAt.UTC().Format(timeRFC3339),
		ClosedAt:        closedAt,
		CloseReason:     strings.TrimSpace(closeReason),
		UpdatedAt:       record.LastSeenAt.UTC().Format(timeRFC3339),
	}
}

func sessionCatalogRecordLeaseExpired(record sessionCatalogRecord, now time.Time) bool {
	leaseText := strings.TrimSpace(record.LeaseExpiresAt)
	if leaseText == "" {
		return true
	}
	leaseExpiresAt, err := time.Parse(timeRFC3339, leaseText)
	if err != nil {
		return true
	}
	return !leaseExpiresAt.After(now.UTC())
}

func staleSessionCatalogRecord(record sessionCatalogRecord, now time.Time, closeReason string) sessionCatalogRecord {
	nowText := now.UTC().Format(timeRFC3339)
	record.State = workspaceSessionStateStale
	record.CloseReason = strings.TrimSpace(closeReason)
	record.UpdatedAt = nowText
	if strings.TrimSpace(record.ClosedAt) == "" {
		record.ClosedAt = nowText
	}
	if strings.TrimSpace(record.LeaseExpiresAt) == "" {
		record.LeaseExpiresAt = nowText
	}
	return record
}

const sessionCatalogUpsertSQL = `INSERT INTO session_catalog (
	session_id,
	workspace_id,
	database_id,
	workspace_name,
	agent_id,
	client_kind,
	afs_version,
	hostname,
	os,
	local_path,
	readonly,
	state,
	started_at,
	last_seen_at,
	lease_expires_at,
	closed_at,
	close_reason,
	updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(session_id) DO UPDATE SET
	workspace_id = excluded.workspace_id,
	database_id = excluded.database_id,
	workspace_name = excluded.workspace_name,
	agent_id = excluded.agent_id,
	client_kind = excluded.client_kind,
	afs_version = excluded.afs_version,
	hostname = excluded.hostname,
	os = excluded.os,
	local_path = excluded.local_path,
	readonly = excluded.readonly,
	state = excluded.state,
	started_at = excluded.started_at,
	last_seen_at = excluded.last_seen_at,
	lease_expires_at = excluded.lease_expires_at,
	closed_at = excluded.closed_at,
	close_reason = excluded.close_reason,
	updated_at = excluded.updated_at`
