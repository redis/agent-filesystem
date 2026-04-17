package controlplane

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func openPostgresCatalog() (*workspaceCatalog, error) {
	dsn := catalogPostgresDSN()
	if dsn == "" {
		return nil, fmt.Errorf("postgres catalog requires %s, POSTGRES_URL_NON_POOLING, POSTGRES_URL, or DATABASE_URL", catalogDSNEnvVar)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	catalog := &workspaceCatalog{db: db, dialect: catalogSQLDialectPostgres}
	if err := catalog.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return catalog, nil
}
