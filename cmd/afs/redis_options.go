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

func buildSearchRedisOptions(base *redis.Client) *redis.Options {
	opts := *base.Options()
	// go-redis v9 gates RediSearch response handling behind either RESP2 or
	// RESP3+UnstableResp3. A RESP2 clone keeps the search path isolated from
	// the rest of the client's protocol settings.
	opts.Protocol = 2
	opts.UnstableResp3 = false
	return &opts
}

func newSearchRedisClient(base *redis.Client) *redis.Client {
	return redis.NewClient(buildSearchRedisOptions(base))
}

func buildRedisTLSConfig(enabled bool) *tls.Config {
	if !enabled {
		return nil
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
}
