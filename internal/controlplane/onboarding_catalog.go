package controlplane

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
)

type onboardingTokenRecord struct {
	Token         string
	DatabaseID    string
	WorkspaceID   string
	WorkspaceName string
	CreatedAt     string
	ExpiresAt     string
	ConsumedAt    string
}

func (c *workspaceCatalog) CreateOnboardingToken(ctx context.Context, item onboardingTokenRecord) error {
	if c == nil || c.db == nil {
		return nil
	}
	if strings.TrimSpace(item.Token) == "" {
		return fmt.Errorf("onboarding token is required")
	}
	if strings.TrimSpace(item.DatabaseID) == "" {
		return fmt.Errorf("onboarding database id is required")
	}
	if strings.TrimSpace(item.WorkspaceID) == "" {
		return fmt.Errorf("onboarding workspace id is required")
	}
	if strings.TrimSpace(item.CreatedAt) == "" {
		return fmt.Errorf("onboarding created_at is required")
	}
	if strings.TrimSpace(item.ExpiresAt) == "" {
		return fmt.Errorf("onboarding expires_at is required")
	}

	_, err := c.execContext(
		ctx,
		`INSERT INTO onboarding_tokens (
			token,
			database_id,
			workspace_id,
			workspace_name,
			created_at,
			expires_at,
			consumed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(item.Token),
		strings.TrimSpace(item.DatabaseID),
		strings.TrimSpace(item.WorkspaceID),
		strings.TrimSpace(item.WorkspaceName),
		strings.TrimSpace(item.CreatedAt),
		strings.TrimSpace(item.ExpiresAt),
		strings.TrimSpace(item.ConsumedAt),
	)
	return err
}

func (c *workspaceCatalog) ConsumeOnboardingToken(ctx context.Context, token, consumedAt string) (onboardingTokenRecord, error) {
	if c == nil || c.db == nil {
		return onboardingTokenRecord{}, os.ErrNotExist
	}
	resolvedToken := strings.TrimSpace(token)
	if resolvedToken == "" {
		return onboardingTokenRecord{}, fmt.Errorf("onboarding token is required")
	}
	resolvedConsumedAt := strings.TrimSpace(consumedAt)
	if resolvedConsumedAt == "" {
		return onboardingTokenRecord{}, fmt.Errorf("onboarding consumed_at is required")
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return onboardingTokenRecord{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	row := tx.QueryRowContext(ctx, c.rebind(`SELECT
			token,
			database_id,
			workspace_id,
			workspace_name,
			created_at,
			expires_at,
			consumed_at
		FROM onboarding_tokens
		WHERE token = ?`), resolvedToken)

	var item onboardingTokenRecord
	if err := row.Scan(
		&item.Token,
		&item.DatabaseID,
		&item.WorkspaceID,
		&item.WorkspaceName,
		&item.CreatedAt,
		&item.ExpiresAt,
		&item.ConsumedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return onboardingTokenRecord{}, os.ErrNotExist
		}
		return onboardingTokenRecord{}, err
	}

	if strings.TrimSpace(item.ConsumedAt) != "" {
		return onboardingTokenRecord{}, ErrOnboardingTokenInvalid
	}
	if strings.TrimSpace(item.ExpiresAt) <= resolvedConsumedAt {
		return onboardingTokenRecord{}, ErrOnboardingTokenInvalid
	}

	result, err := tx.ExecContext(ctx, c.rebind(`UPDATE onboarding_tokens
		SET consumed_at = ?
		WHERE token = ?
		  AND consumed_at = ''`), resolvedConsumedAt, resolvedToken)
	if err != nil {
		return onboardingTokenRecord{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return onboardingTokenRecord{}, err
	}
	if affected != 1 {
		return onboardingTokenRecord{}, ErrOnboardingTokenInvalid
	}

	item.ConsumedAt = resolvedConsumedAt
	if err := tx.Commit(); err != nil {
		return onboardingTokenRecord{}, err
	}
	return item, nil
}
