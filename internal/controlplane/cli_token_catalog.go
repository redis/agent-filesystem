package controlplane

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
)

func (c *workspaceCatalog) CreateCLIAccessToken(ctx context.Context, item cliAccessTokenRecord) error {
	if strings.TrimSpace(item.ID) == "" {
		return fmt.Errorf("cli token id is required")
	}
	if strings.TrimSpace(item.DatabaseID) == "" {
		return fmt.Errorf("cli token database is required")
	}
	if strings.TrimSpace(item.WorkspaceID) == "" {
		return fmt.Errorf("cli token workspace is required")
	}
	if strings.TrimSpace(item.SecretHash) == "" {
		return fmt.Errorf("cli token secret hash is required")
	}
	_, err := c.execContext(ctx, c.rebind(`INSERT INTO cli_access_tokens (
		id,
		owner_subject,
		owner_label,
		database_id,
		workspace_id,
		workspace_name,
		secret_hash,
		created_at,
		last_used_at,
		expires_at,
		revoked_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		strings.TrimSpace(item.ID),
		strings.TrimSpace(item.OwnerSubject),
		strings.TrimSpace(item.OwnerLabel),
		strings.TrimSpace(item.DatabaseID),
		strings.TrimSpace(item.WorkspaceID),
		strings.TrimSpace(item.WorkspaceName),
		strings.TrimSpace(item.SecretHash),
		strings.TrimSpace(item.CreatedAt),
		strings.TrimSpace(item.LastUsedAt),
		strings.TrimSpace(item.ExpiresAt),
		strings.TrimSpace(item.RevokedAt),
	)
	return err
}

func (c *workspaceCatalog) GetCLIAccessToken(ctx context.Context, tokenID string) (cliAccessTokenRecord, error) {
	rows, err := c.queryContext(ctx, c.rebind(`SELECT
		id,
		owner_subject,
		owner_label,
		database_id,
		workspace_id,
		workspace_name,
		secret_hash,
		created_at,
		last_used_at,
		expires_at,
		revoked_at
	FROM cli_access_tokens
	WHERE id = ?`), strings.TrimSpace(tokenID))
	if err != nil {
		return cliAccessTokenRecord{}, err
	}
	defer rows.Close()
	items, err := scanCLIAccessTokenRows(rows)
	if err != nil {
		return cliAccessTokenRecord{}, err
	}
	if len(items) == 0 {
		return cliAccessTokenRecord{}, os.ErrNotExist
	}
	return items[0], nil
}

func (c *workspaceCatalog) TouchCLIAccessToken(ctx context.Context, tokenID, lastUsedAt string) error {
	_, err := c.execContext(ctx, c.rebind(`UPDATE cli_access_tokens
		SET last_used_at = ?
		WHERE id = ?`), strings.TrimSpace(lastUsedAt), strings.TrimSpace(tokenID))
	return err
}

func (c *workspaceCatalog) RevokeCLIAccessTokensByOwner(ctx context.Context, ownerSubject, revokedAt string) error {
	_, err := c.execContext(ctx, c.rebind(`UPDATE cli_access_tokens
		SET revoked_at = ?
		WHERE owner_subject = ? AND revoked_at = ''`),
		strings.TrimSpace(revokedAt),
		strings.TrimSpace(ownerSubject),
	)
	return err
}

func scanCLIAccessTokenRows(rows *sql.Rows) ([]cliAccessTokenRecord, error) {
	items := make([]cliAccessTokenRecord, 0)
	for rows.Next() {
		var item cliAccessTokenRecord
		if err := rows.Scan(
			&item.ID,
			&item.OwnerSubject,
			&item.OwnerLabel,
			&item.DatabaseID,
			&item.WorkspaceID,
			&item.WorkspaceName,
			&item.SecretHash,
			&item.CreatedAt,
			&item.LastUsedAt,
			&item.ExpiresAt,
			&item.RevokedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
