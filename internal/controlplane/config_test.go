package controlplane

import (
	"crypto/tls"
	"testing"
	"time"
)

func TestBuildRedisOptionsIncludesReadTimeoutAndTLS(t *testing.T) {
	t.Helper()

	cfg := Config{
		RedisConfig: RedisConfig{
			RedisAddr:     "hosted.redis.example.com:16379",
			RedisUsername: "default",
			RedisPassword: "secret",
			RedisDB:       7,
			RedisTLS:      true,
		},
	}

	opts := buildRedisOptions(cfg, 12)
	if opts.Addr != cfg.RedisAddr {
		t.Fatalf("Addr = %q, want %q", opts.Addr, cfg.RedisAddr)
	}
	if opts.Username != cfg.RedisUsername {
		t.Fatalf("Username = %q, want %q", opts.Username, cfg.RedisUsername)
	}
	if opts.Password != cfg.RedisPassword {
		t.Fatalf("Password = %q, want %q", opts.Password, cfg.RedisPassword)
	}
	if opts.DB != cfg.RedisDB {
		t.Fatalf("DB = %d, want %d", opts.DB, cfg.RedisDB)
	}
	if opts.PoolSize != 12 {
		t.Fatalf("PoolSize = %d, want 12", opts.PoolSize)
	}
	if opts.ReadTimeout != 30*time.Second {
		t.Fatalf("ReadTimeout = %s, want 30s", opts.ReadTimeout)
	}
	if opts.WriteTimeout != 30*time.Second {
		t.Fatalf("WriteTimeout = %s, want 30s", opts.WriteTimeout)
	}
	if opts.TLSConfig == nil {
		t.Fatalf("TLSConfig = nil, want non-nil")
	}
	if opts.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("TLS min version = %d, want %d", opts.TLSConfig.MinVersion, tls.VersionTLS12)
	}
}

func TestRedisConfigFromURLParsesTLSUsernamePasswordAndDB(t *testing.T) {
	cfg, ok := redisConfigFromURL("rediss://default:secret@cache.example.com:16379/5")
	if !ok {
		t.Fatal("redisConfigFromURL() = false, want true")
	}
	if cfg.RedisAddr != "cache.example.com:16379" {
		t.Fatalf("RedisAddr = %q, want %q", cfg.RedisAddr, "cache.example.com:16379")
	}
	if cfg.RedisUsername != "default" {
		t.Fatalf("RedisUsername = %q, want %q", cfg.RedisUsername, "default")
	}
	if cfg.RedisPassword != "secret" {
		t.Fatalf("RedisPassword = %q, want %q", cfg.RedisPassword, "secret")
	}
	if cfg.RedisDB != 5 {
		t.Fatalf("RedisDB = %d, want %d", cfg.RedisDB, 5)
	}
	if !cfg.RedisTLS {
		t.Fatal("RedisTLS = false, want true")
	}
}

func TestApplyEnvOverridesFallsBackToRedisURL(t *testing.T) {
	t.Setenv("AFS_REDIS_ADDR", "")
	t.Setenv("AFS_REDIS_USERNAME", "")
	t.Setenv("AFS_REDIS_PASSWORD", "")
	t.Setenv("AFS_REDIS_DB", "")
	t.Setenv("AFS_REDIS_TLS", "")
	t.Setenv("REDIS_URL", "rediss://default:secret@cache.example.com:16379/9")

	cfg := Config{}
	if ok := applyEnvOverrides(&cfg); !ok {
		t.Fatal("applyEnvOverrides() = false, want true with REDIS_URL")
	}
	if cfg.RedisAddr != "cache.example.com:16379" {
		t.Fatalf("RedisAddr = %q, want %q", cfg.RedisAddr, "cache.example.com:16379")
	}
	if cfg.RedisUsername != "default" {
		t.Fatalf("RedisUsername = %q, want %q", cfg.RedisUsername, "default")
	}
	if cfg.RedisPassword != "secret" {
		t.Fatalf("RedisPassword = %q, want %q", cfg.RedisPassword, "secret")
	}
	if cfg.RedisDB != 9 {
		t.Fatalf("RedisDB = %d, want %d", cfg.RedisDB, 9)
	}
	if !cfg.RedisTLS {
		t.Fatal("RedisTLS = false, want true")
	}
}

func TestApplyEnvOverridesPrefersExplicitAFSRedisEnv(t *testing.T) {
	t.Setenv("AFS_REDIS_ADDR", "manual.redis.example.com:6379")
	t.Setenv("AFS_REDIS_USERNAME", "alice")
	t.Setenv("AFS_REDIS_PASSWORD", "manual-secret")
	t.Setenv("AFS_REDIS_DB", "2")
	t.Setenv("AFS_REDIS_TLS", "false")
	t.Setenv("REDIS_URL", "rediss://default:secret@cache.example.com:16379/9")

	cfg := Config{}
	if ok := applyEnvOverrides(&cfg); !ok {
		t.Fatal("applyEnvOverrides() = false, want true")
	}
	if cfg.RedisAddr != "manual.redis.example.com:6379" {
		t.Fatalf("RedisAddr = %q, want explicit AFS_REDIS_ADDR", cfg.RedisAddr)
	}
	if cfg.RedisUsername != "alice" {
		t.Fatalf("RedisUsername = %q, want explicit username", cfg.RedisUsername)
	}
	if cfg.RedisPassword != "manual-secret" {
		t.Fatalf("RedisPassword = %q, want explicit password", cfg.RedisPassword)
	}
	if cfg.RedisDB != 2 {
		t.Fatalf("RedisDB = %d, want %d", cfg.RedisDB, 2)
	}
	if cfg.RedisTLS {
		t.Fatal("RedisTLS = true, want explicit false")
	}
}
