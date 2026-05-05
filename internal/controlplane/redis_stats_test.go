package controlplane

import (
	"testing"
)

func TestParseRedisInfo_Memory(t *testing.T) {
	raw := "# Server\r\nredis_version:8.0.0-array\r\n# Memory\r\nused_memory:104857600\r\nmaxmemory:524288000\r\nmem_fragmentation_ratio:1.23\r\n"
	stats := parseRedisInfo(raw)
	if stats.RedisVersion != "8.0.0-array" {
		t.Fatalf("RedisVersion = %q, want 8.0.0-array", stats.RedisVersion)
	}
	if stats.UsedMemoryBytes != 104857600 {
		t.Fatalf("UsedMemoryBytes = %d, want 104857600", stats.UsedMemoryBytes)
	}
	if stats.MaxMemoryBytes != 524288000 {
		t.Fatalf("MaxMemoryBytes = %d, want 524288000", stats.MaxMemoryBytes)
	}
	if stats.FragmentationR != 1.23 {
		t.Fatalf("FragmentationR = %v, want 1.23", stats.FragmentationR)
	}
}

func TestParseRedisInfo_ClientsAndStats(t *testing.T) {
	raw := "# Clients\nconnected_clients:17\n# Stats\ninstantaneous_ops_per_sec:420\nkeyspace_hits:900\nkeyspace_misses:100\n"
	stats := parseRedisInfo(raw)
	if stats.ConnectedClients != 17 {
		t.Fatalf("ConnectedClients = %d, want 17", stats.ConnectedClients)
	}
	if stats.OpsPerSec != 420 {
		t.Fatalf("OpsPerSec = %d, want 420", stats.OpsPerSec)
	}
	if stats.CacheHitRate != 0.9 {
		t.Fatalf("CacheHitRate = %v, want 0.9", stats.CacheHitRate)
	}
}

func TestParseRedisInfo_NoHitsOrMissesLeavesHitRateZero(t *testing.T) {
	raw := "keyspace_hits:0\nkeyspace_misses:0\n"
	stats := parseRedisInfo(raw)
	if stats.CacheHitRate != 0 {
		t.Fatalf("CacheHitRate = %v, want 0", stats.CacheHitRate)
	}
}

func TestParseRedisInfo_IgnoresMalformedLines(t *testing.T) {
	raw := "\n# Comment only\nthis-is-not-a-kv\nused_memory:42\n"
	stats := parseRedisInfo(raw)
	if stats.UsedMemoryBytes != 42 {
		t.Fatalf("UsedMemoryBytes = %d, want 42", stats.UsedMemoryBytes)
	}
}
