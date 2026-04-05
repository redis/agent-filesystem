package redisconn

import (
	"crypto/tls"
	"testing"
)

func TestOptionsIncludesUsernameAndTLS(t *testing.T) {
	t.Helper()
	opts := Options(Config{
		Addr:       "cloud.redis.example.com:16379",
		Username:   "default",
		Password:   "secret",
		DB:         0,
		PoolSize:   16,
		TLSEnabled: true,
	})

	if opts.Addr != "cloud.redis.example.com:16379" {
		t.Fatalf("Addr = %q, want %q", opts.Addr, "cloud.redis.example.com:16379")
	}
	if opts.Username != "default" {
		t.Fatalf("Username = %q, want %q", opts.Username, "default")
	}
	if opts.Password != "secret" {
		t.Fatalf("Password = %q, want %q", opts.Password, "secret")
	}
	if opts.PoolSize != 16 {
		t.Fatalf("PoolSize = %d, want 16", opts.PoolSize)
	}
	if opts.TLSConfig == nil {
		t.Fatalf("TLSConfig = nil, want non-nil")
	}
	if opts.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("TLS min version = %d, want %d", opts.TLSConfig.MinVersion, tls.VersionTLS12)
	}
}
