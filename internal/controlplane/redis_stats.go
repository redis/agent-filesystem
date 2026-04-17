package controlplane

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RedisStats captures a snapshot of Redis server health for a single database
// profile. All fields are best-effort: if the server is unreachable or the
// relevant INFO line is missing, the field is left zero.
type RedisStats struct {
	// Memory
	UsedMemoryBytes int64   `json:"used_memory_bytes"`
	MaxMemoryBytes  int64   `json:"max_memory_bytes"` // 0 = no limit configured
	FragmentationR  float64 `json:"fragmentation_ratio,omitempty"`

	// Keyspace (DBSIZE for the currently selected DB index)
	KeyCount int64 `json:"key_count"`

	// Throughput & cache efficiency
	OpsPerSec        int64   `json:"ops_per_sec"`
	CacheHitRate     float64 `json:"cache_hit_rate,omitempty"` // 0.0–1.0; 0 if no hits/misses observed
	ConnectedClients int64   `json:"connected_clients"`

	// Sampling metadata
	SampledAt string `json:"sampled_at,omitempty"`
}

// CollectRedisStats issues a single INFO + DBSIZE round-trip and returns the
// parsed snapshot. Called by the background poller in DatabaseManager.
func (s *Store) CollectRedisStats(ctx context.Context) (RedisStats, error) {
	raw, err := s.rdb.Info(ctx, "memory", "clients", "stats").Result()
	if err != nil {
		return RedisStats{}, fmt.Errorf("redis INFO: %w", err)
	}

	dbsize, err := s.rdb.DBSize(ctx).Result()
	if err != nil {
		return RedisStats{}, fmt.Errorf("redis DBSIZE: %w", err)
	}

	stats := parseRedisInfo(raw)
	stats.KeyCount = dbsize
	stats.SampledAt = time.Now().UTC().Format(time.RFC3339)
	return stats, nil
}

// parseRedisInfo extracts the fields we care about from the plain-text INFO
// response. Exported (via the un-exported name but with a test file in the
// same package) so we can unit-test without a live Redis.
func parseRedisInfo(raw string) RedisStats {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	var (
		stats        RedisStats
		keyspaceHits int64
		keyspaceMiss int64
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		switch key {
		case "used_memory":
			stats.UsedMemoryBytes = parseInt64(value)
		case "maxmemory":
			stats.MaxMemoryBytes = parseInt64(value)
		case "mem_fragmentation_ratio":
			stats.FragmentationR = parseFloat64(value)
		case "connected_clients":
			stats.ConnectedClients = parseInt64(value)
		case "instantaneous_ops_per_sec":
			stats.OpsPerSec = parseInt64(value)
		case "keyspace_hits":
			keyspaceHits = parseInt64(value)
		case "keyspace_misses":
			keyspaceMiss = parseInt64(value)
		}
	}

	if total := keyspaceHits + keyspaceMiss; total > 0 {
		stats.CacheHitRate = float64(keyspaceHits) / float64(total)
	}
	return stats
}

func parseInt64(value string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func parseFloat64(value string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return f
}
