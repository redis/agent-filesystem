package main

import (
	"crypto/tls"
	"time"

	"github.com/redis/go-redis/v9"
)

func buildRedisOptions(cfg config, poolSize int) *redis.Options {
	return &redis.Options{
		Addr:     cfg.RedisAddr,
		Username: cfg.RedisUsername,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
		PoolSize: poolSize,
		// Cloud reconcile pipelines can take longer than go-redis's 3s default.
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		TLSConfig:    buildRedisTLSConfig(cfg.RedisTLS),
	}
}

func buildRedisTLSConfig(enabled bool) *tls.Config {
	if !enabled {
		return nil
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
}
