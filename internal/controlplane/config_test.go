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
