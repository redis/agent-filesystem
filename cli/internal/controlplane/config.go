package controlplane

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	RedisAddr     string `json:"redisAddr"`
	RedisUsername string `json:"redisUsername"`
	RedisPassword string `json:"redisPassword"`
	RedisDB       int    `json:"redisDB"`
	RedisTLS      bool   `json:"redisTLS"`
}

func LoadConfig(configPathOverride string) (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(configPath(configPathOverride))
	if err != nil {
		return cfg, fmt.Errorf("no configuration found\nCreate %s or run afs setup first", configPath(configPathOverride))
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if err := prepareConfig(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func OpenStore(ctx context.Context, cfg Config) (*Store, func(), error) {
	rdb := redis.NewClient(buildRedisOptions(cfg, 8))
	closeFn := func() {
		_ = rdb.Close()
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		closeFn()
		return nil, func() {}, fmt.Errorf("cannot connect to Redis at %s: %w", cfg.RedisAddr, err)
	}
	return NewStore(rdb), closeFn, nil
}

func buildRedisOptions(cfg Config, poolSize int) *redis.Options {
	return &redis.Options{
		Addr:         cfg.RedisAddr,
		Username:     cfg.RedisUsername,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		PoolSize:     poolSize,
		WriteTimeout: 30 * time.Second,
		TLSConfig:    buildRedisTLSConfig(cfg.RedisTLS),
	}
}

func buildRedisTLSConfig(enabled bool) *tls.Config {
	if !enabled {
		return nil
	}
	return &tls.Config{MinVersion: tls.VersionTLS12}
}

func configPath(configPathOverride string) string {
	if configPathOverride != "" {
		return configPathOverride
	}
	exe, err := executablePath()
	if err != nil {
		return "afs.config.json"
	}
	return filepath.Join(filepath.Dir(exe), "afs.config.json")
}

func defaultConfig() Config {
	return Config{
		RedisAddr: "localhost:6379",
		RedisDB:   0,
	}
}

func prepareConfig(cfg *Config) error {
	if strings.TrimSpace(cfg.RedisAddr) == "" {
		cfg.RedisAddr = defaultConfig().RedisAddr
	}
	if cfg.RedisDB < 0 {
		return fmt.Errorf("redis db must be >= 0")
	}
	if _, _, err := splitAddr(cfg.RedisAddr); err != nil {
		return err
	}
	return nil
}

func splitAddr(addr string) (string, int, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid address %q (expected host:port)", addr)
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, err
	}
	return parts[0], port, nil
}

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved, nil
	}
	return exe, nil
}
