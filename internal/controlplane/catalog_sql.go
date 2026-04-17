package controlplane

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
)

type catalogSQLDialect string

const (
	catalogSQLDialectSQLite   catalogSQLDialect = "sqlite"
	catalogSQLDialectPostgres catalogSQLDialect = "postgres"
)

func (c *workspaceCatalog) execContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.db.ExecContext(ctx, c.rebind(query), args...)
}

func (c *workspaceCatalog) queryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return c.db.QueryContext(ctx, c.rebind(query), args...)
}

func (c *workspaceCatalog) queryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return c.db.QueryRowContext(ctx, c.rebind(query), args...)
}

func (c *workspaceCatalog) prepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return c.db.PrepareContext(ctx, c.rebind(query))
}

func (c *workspaceCatalog) rebind(query string) string {
	if c == nil || c.dialect != catalogSQLDialectPostgres {
		return query
	}

	var b strings.Builder
	b.Grow(len(query) + 16)
	placeholder := 1
	for _, r := range query {
		if r == '?' {
			fmt.Fprintf(&b, "$%d", placeholder)
			placeholder++
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func catalogPostgresDSN() string {
	for _, key := range []string{
		catalogDSNEnvVar,
		"POSTGRES_URL_NON_POOLING",
		"POSTGRES_URL",
		"DATABASE_URL",
	} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
