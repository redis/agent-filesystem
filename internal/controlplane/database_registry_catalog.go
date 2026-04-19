package controlplane

import (
	"context"
	"strings"
)

func (c *workspaceCatalog) ListDatabaseProfiles(ctx context.Context) ([]databaseProfile, error) {
	if c == nil || c.db == nil {
		return nil, nil
	}
	rows, err := c.queryContext(
		ctx,
		`SELECT
			id,
			name,
			description,
			owner_subject,
			owner_label,
			management_type,
			redis_addr,
			redis_username,
			redis_password,
			redis_db,
			redis_tls,
			is_default
		FROM database_registry
		ORDER BY order_index ASC, lower(name), id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]databaseProfile, 0)
	for rows.Next() {
		var item databaseProfile
		var redisTLS int
		var isDefault int
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Description,
			&item.OwnerSubject,
			&item.OwnerLabel,
			&item.ManagementType,
			&item.RedisAddr,
			&item.RedisUsername,
			&item.RedisPassword,
			&item.RedisDB,
			&redisTLS,
			&isDefault,
		); err != nil {
			return nil, err
		}
		item.RedisTLS = redisTLS != 0
		item.IsDefault = isDefault != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

func (c *workspaceCatalog) ReplaceDatabaseProfiles(ctx context.Context, profiles []databaseProfile) error {
	if c == nil || c.db == nil {
		return nil
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM database_registry`); err != nil {
		return err
	}

	statement, err := tx.PrepareContext(ctx, c.rebind(databaseRegistryUpsertSQL))
	if err != nil {
		return err
	}
	defer statement.Close()

	for index, profile := range profiles {
		redisTLS := 0
		if profile.RedisTLS {
			redisTLS = 1
		}
		isDefault := 0
		if profile.IsDefault {
			isDefault = 1
		}
		if _, err := statement.ExecContext(
			ctx,
			strings.TrimSpace(profile.ID),
			strings.TrimSpace(profile.Name),
			strings.TrimSpace(profile.Description),
			strings.TrimSpace(profile.OwnerSubject),
			strings.TrimSpace(profile.OwnerLabel),
			strings.TrimSpace(profile.ManagementType),
			strings.TrimSpace(profile.RedisAddr),
			strings.TrimSpace(profile.RedisUsername),
			profile.RedisPassword,
			profile.RedisDB,
			redisTLS,
			isDefault,
			index,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

const databaseRegistryUpsertSQL = `INSERT INTO database_registry (
	id,
	name,
	description,
	owner_subject,
	owner_label,
	management_type,
	redis_addr,
	redis_username,
	redis_password,
	redis_db,
	redis_tls,
	is_default,
	order_index
 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	description = excluded.description,
	owner_subject = excluded.owner_subject,
	owner_label = excluded.owner_label,
	management_type = excluded.management_type,
	redis_addr = excluded.redis_addr,
	redis_username = excluded.redis_username,
	redis_password = excluded.redis_password,
	redis_db = excluded.redis_db,
	redis_tls = excluded.redis_tls,
	is_default = excluded.is_default,
	order_index = excluded.order_index`
