package redisconn

import (
	"crypto/tls"
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	Addr       string
	Username   string
	Password   string
	DB         int
	PoolSize   int
	TLSEnabled bool
}

func Options(cfg Config) *redis.Options {
	return &redis.Options{
		Addr:         cfg.Addr,
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		TLSConfig:    TLSConfig(cfg.TLSEnabled),
	}
}

func TLSConfig(enabled bool) *tls.Config {
	if !enabled {
		return nil
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
}
