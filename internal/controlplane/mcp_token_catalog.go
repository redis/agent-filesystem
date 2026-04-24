package controlplane

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
)

func (c *workspaceCatalog) CreateMCPAccessToken(ctx context.Context, item mcpAccessTokenRecord) error {
	if strings.TrimSpace(item.ID) == "" {
		return fmt.Errorf("mcp token id is required")
	}
	if strings.TrimSpace(item.Scope) == "" {
		return fmt.Errorf("mcp token scope is required")
	}
	if strings.TrimSpace(item.SecretHash) == "" {
		return fmt.Errorf("mcp token secret hash is required")
	}
	// Workspace-scoped tokens require a workspace binding. Control-plane tokens
	// intentionally leave database_id/workspace_id empty.
	if strings.HasPrefix(item.Scope, mcpScopeWorkspacePrefix) {
		if strings.TrimSpace(item.DatabaseID) == "" {
			return fmt.Errorf("mcp token database is required for workspace scope")
		}
		if strings.TrimSpace(item.WorkspaceID) == "" {
			return fmt.Errorf("mcp token workspace is required for workspace scope")
		}
	}
	_, err := c.execContext(ctx, c.rebind(`INSERT INTO mcp_access_tokens (
		id,
		name,
		owner_subject,
		owner_label,
		database_id,
		workspace_id,
		workspace_name,
		scope,
		profile,
		template_slug,
		readonly,
		secret_hash,
		secret,
		created_at,
		last_used_at,
		expires_at,
		revoked_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		strings.TrimSpace(item.ID),
		strings.TrimSpace(item.Name),
		strings.TrimSpace(item.OwnerSubject),
		strings.TrimSpace(item.OwnerLabel),
		strings.TrimSpace(item.DatabaseID),
		strings.TrimSpace(item.WorkspaceID),
		strings.TrimSpace(item.WorkspaceName),
		strings.TrimSpace(item.Scope),
		strings.TrimSpace(item.Profile),
		strings.TrimSpace(item.TemplateSlug),
		boolToCatalogInt(item.Readonly),
		strings.TrimSpace(item.SecretHash),
		strings.TrimSpace(item.Secret),
		strings.TrimSpace(item.CreatedAt),
		strings.TrimSpace(item.LastUsedAt),
		strings.TrimSpace(item.ExpiresAt),
		strings.TrimSpace(item.RevokedAt),
	)
	return err
}

const mcpAccessTokenSelectColumns = `
		id,
		name,
		owner_subject,
		owner_label,
		database_id,
		workspace_id,
		workspace_name,
		scope,
		profile,
		template_slug,
		readonly,
		secret_hash,
		secret,
		created_at,
		last_used_at,
		expires_at,
		revoked_at`

func (c *workspaceCatalog) ListMCPAccessTokens(ctx context.Context, databaseID, workspaceID string) ([]mcpAccessTokenRecord, error) {
	rows, err := c.queryContext(ctx, c.rebind(`SELECT`+mcpAccessTokenSelectColumns+`
	FROM mcp_access_tokens
	WHERE database_id = ? AND workspace_id = ?
	ORDER BY created_at DESC, id ASC`), strings.TrimSpace(databaseID), strings.TrimSpace(workspaceID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMCPAccessTokenRows(rows)
}

func (c *workspaceCatalog) ListAllMCPAccessTokens(ctx context.Context) ([]mcpAccessTokenRecord, error) {
	rows, err := c.queryContext(ctx, c.rebind(`SELECT`+mcpAccessTokenSelectColumns+`
	FROM mcp_access_tokens
	ORDER BY created_at DESC, id ASC`))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMCPAccessTokenRows(rows)
}

// ListControlPlaneMCPAccessTokens returns every control-plane-scoped token.
// Callers filter by owner_subject at the service layer.
func (c *workspaceCatalog) ListControlPlaneMCPAccessTokens(ctx context.Context) ([]mcpAccessTokenRecord, error) {
	rows, err := c.queryContext(ctx, c.rebind(`SELECT`+mcpAccessTokenSelectColumns+`
	FROM mcp_access_tokens
	WHERE scope = ?
	ORDER BY created_at DESC, id ASC`), mcpScopeControlPlane)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMCPAccessTokenRows(rows)
}

func (c *workspaceCatalog) GetMCPAccessToken(ctx context.Context, tokenID string) (mcpAccessTokenRecord, error) {
	rows, err := c.queryContext(ctx, c.rebind(`SELECT`+mcpAccessTokenSelectColumns+`
	FROM mcp_access_tokens
	WHERE id = ?`), strings.TrimSpace(tokenID))
	if err != nil {
		return mcpAccessTokenRecord{}, err
	}
	defer rows.Close()
	items, err := scanMCPAccessTokenRows(rows)
	if err != nil {
		return mcpAccessTokenRecord{}, err
	}
	if len(items) == 0 {
		return mcpAccessTokenRecord{}, os.ErrNotExist
	}
	return items[0], nil
}

func (c *workspaceCatalog) TouchMCPAccessToken(ctx context.Context, tokenID, lastUsedAt string) error {
	_, err := c.execContext(ctx, c.rebind(`UPDATE mcp_access_tokens
		SET last_used_at = ?
		WHERE id = ?`), strings.TrimSpace(lastUsedAt), strings.TrimSpace(tokenID))
	return err
}

func (c *workspaceCatalog) RevokeMCPAccessToken(ctx context.Context, tokenID, databaseID, workspaceID, revokedAt string) error {
	result, err := c.execContext(ctx, c.rebind(`UPDATE mcp_access_tokens
		SET revoked_at = ?
		WHERE id = ? AND database_id = ? AND workspace_id = ? AND revoked_at = ''`),
		strings.TrimSpace(revokedAt),
		strings.TrimSpace(tokenID),
		strings.TrimSpace(databaseID),
		strings.TrimSpace(workspaceID),
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return os.ErrNotExist
	}
	return nil
}

// RevokeMCPAccessTokenByID revokes a token identified only by ID. Used for
// control-plane tokens which have no workspace/database binding, and for the
// user-scoped revocation endpoint where the caller has already confirmed the
// owner via the token record.
func (c *workspaceCatalog) RevokeMCPAccessTokenByID(ctx context.Context, tokenID, revokedAt string) error {
	result, err := c.execContext(ctx, c.rebind(`UPDATE mcp_access_tokens
		SET revoked_at = ?
		WHERE id = ? AND revoked_at = ''`),
		strings.TrimSpace(revokedAt),
		strings.TrimSpace(tokenID),
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return os.ErrNotExist
	}
	return nil
}

func scanMCPAccessTokenRows(rows *sql.Rows) ([]mcpAccessTokenRecord, error) {
	items := make([]mcpAccessTokenRecord, 0)
	for rows.Next() {
		var item mcpAccessTokenRecord
		var readonly int
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.OwnerSubject,
			&item.OwnerLabel,
			&item.DatabaseID,
			&item.WorkspaceID,
			&item.WorkspaceName,
			&item.Scope,
			&item.Profile,
			&item.TemplateSlug,
			&readonly,
			&item.SecretHash,
			&item.Secret,
			&item.CreatedAt,
			&item.LastUsedAt,
			&item.ExpiresAt,
			&item.RevokedAt,
		); err != nil {
			return nil, err
		}
		item.Readonly = readonly != 0
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func boolToCatalogInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
