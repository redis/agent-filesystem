package main

import (
	"crypto/tls"
	"testing"
)

func TestBuildRedisOptionsIncludesUsernameAndTLS(t *testing.T) {
	t.Helper()
	cfg := config{
		RedisAddr:     "hosted.redis.example.com:16379",
		RedisUsername: "default",
		RedisPassword: "secret",
		RedisDB:       0,
		RedisTLS:      true,
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
	if opts.PoolSize != 12 {
		t.Fatalf("PoolSize = %d, want 12", opts.PoolSize)
	}
	if opts.TLSConfig == nil {
		t.Fatalf("TLSConfig = nil, want non-nil")
	}
	if opts.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("TLS min version = %d, want %d", opts.TLSConfig.MinVersion, tls.VersionTLS12)
	}
}
