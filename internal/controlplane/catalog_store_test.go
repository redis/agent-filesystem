package controlplane

import "testing"

func TestCatalogDriverNameDefaultsToEmpty(t *testing.T) {
	t.Setenv(catalogDriverEnvVar, "")

	if got := catalogDriverName(); got != "" {
		t.Fatalf("catalogDriverName() = %q, want empty default", got)
	}
}

func TestCatalogStorePathUsesEnvOverride(t *testing.T) {
	t.Setenv(catalogPathEnvVar, "/tmp/vercel/afs.catalog.sqlite")

	if got := catalogStorePath("/ignored/afs.config.json"); got != "/tmp/vercel/afs.catalog.sqlite" {
		t.Fatalf("catalogStorePath() = %q, want env override", got)
	}
}

func TestCatalogStorePathFallsBackToConfigDirectory(t *testing.T) {
	t.Setenv(catalogPathEnvVar, "")

	got := catalogStorePath("/tmp/afs/afs.config.json")
	want := "/tmp/afs/afs.catalog.sqlite"
	if got != want {
		t.Fatalf("catalogStorePath() = %q, want %q", got, want)
	}
}

func TestOpenCatalogStoreRejectsUnknownDriver(t *testing.T) {
	t.Setenv(catalogDriverEnvVar, "mystery")

	_, err := openCatalogStore("/tmp/afs.config.json")
	if err == nil {
		t.Fatal("openCatalogStore() returned nil error, want unsupported driver error")
	}
	if got, want := err.Error(), `unsupported AFS_CATALOG_DRIVER "mystery"`; got != want {
		t.Fatalf("openCatalogStore() error = %q, want %q", got, want)
	}
}

func TestCatalogPostgresDSNUsesExplicitEnv(t *testing.T) {
	t.Setenv(catalogDSNEnvVar, "postgres://catalog-explicit")
	t.Setenv("POSTGRES_URL_NON_POOLING", "postgres://non-pooling")
	t.Setenv("POSTGRES_URL", "postgres://pooled")
	t.Setenv("DATABASE_URL", "postgres://database-url")

	if got := catalogPostgresDSN(); got != "postgres://catalog-explicit" {
		t.Fatalf("catalogPostgresDSN() = %q, want explicit env override", got)
	}
}

func TestCatalogPostgresDSNFallsBackToProviderEnv(t *testing.T) {
	t.Setenv(catalogDSNEnvVar, "")
	t.Setenv("POSTGRES_URL_NON_POOLING", "")
	t.Setenv("POSTGRES_URL", "postgres://pooled")
	t.Setenv("DATABASE_URL", "postgres://database-url")

	if got := catalogPostgresDSN(); got != "postgres://pooled" {
		t.Fatalf("catalogPostgresDSN() = %q, want provider fallback", got)
	}
}

func TestOpenCatalogStorePostgresRequiresDSN(t *testing.T) {
	t.Setenv(catalogDriverEnvVar, catalogDriverPostgres)
	t.Setenv(catalogDSNEnvVar, "")
	t.Setenv("POSTGRES_URL_NON_POOLING", "")
	t.Setenv("POSTGRES_URL", "")
	t.Setenv("DATABASE_URL", "")

	_, err := openCatalogStore("/tmp/afs.config.json")
	if err == nil {
		t.Fatal("openCatalogStore() returned nil error, want missing postgres dsn error")
	}
	if got, want := err.Error(), "postgres catalog requires AFS_CATALOG_DSN, POSTGRES_URL_NON_POOLING, POSTGRES_URL, or DATABASE_URL"; got != want {
		t.Fatalf("openCatalogStore() error = %q, want %q", got, want)
	}
}
