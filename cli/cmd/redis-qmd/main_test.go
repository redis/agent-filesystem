package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCLIUsesConfigDefaults(t *testing.T) {
	configFile := writeQMDConfig(t, `{
  "redisAddr": "redis.example:6380",
  "redisDB": 7,
  "redisKey": "demo"
}`)

	opts, args, err := parseCLI([]string{"--config", configFile, "doctor"})
	if err != nil {
		t.Fatalf("parseCLI() returned error: %v", err)
	}
	if len(args) != 1 || args[0] != "doctor" {
		t.Fatalf("args = %v, want [doctor]", args)
	}
	if opts.redisConfig.RedisAddr != "redis.example:6380" {
		t.Fatalf("RedisAddr = %q, want %q", opts.redisConfig.RedisAddr, "redis.example:6380")
	}
	if opts.redisConfig.RedisDB != 7 {
		t.Fatalf("RedisDB = %d, want %d", opts.redisConfig.RedisDB, 7)
	}
	if opts.key != "demo" {
		t.Fatalf("key = %q, want %q", opts.key, "demo")
	}
}

func TestParseCLIFlagsOverrideConfigDefaults(t *testing.T) {
	configFile := writeQMDConfig(t, `{
  "redisAddr": "redis.example:6380",
  "redisDB": 7,
  "redisKey": "demo"
}`)

	opts, args, err := parseCLI([]string{
		"--config", configFile,
		"--addr", "127.0.0.1:6390",
		"--db", "3",
		"--key", "override",
		"--index", "custom-index",
		"search", "TODO",
	})
	if err != nil {
		t.Fatalf("parseCLI() returned error: %v", err)
	}
	if len(args) != 2 || args[0] != "search" || args[1] != "TODO" {
		t.Fatalf("args = %v, want [search TODO]", args)
	}
	if opts.redisConfig.RedisAddr != "127.0.0.1:6390" {
		t.Fatalf("RedisAddr = %q, want %q", opts.redisConfig.RedisAddr, "127.0.0.1:6390")
	}
	if opts.redisConfig.RedisDB != 3 {
		t.Fatalf("RedisDB = %d, want %d", opts.redisConfig.RedisDB, 3)
	}
	if opts.key != "override" {
		t.Fatalf("key = %q, want %q", opts.key, "override")
	}
	if opts.index != "custom-index" {
		t.Fatalf("index = %q, want %q", opts.index, "custom-index")
	}
}

func TestParseCLIUsesStandaloneDefaultsWhenConfigMissing(t *testing.T) {
	opts, args, err := parseCLI([]string{"--key", "demo", "doctor"})
	if err != nil {
		t.Fatalf("parseCLI() returned error: %v", err)
	}
	if len(args) != 1 || args[0] != "doctor" {
		t.Fatalf("args = %v, want [doctor]", args)
	}
	if opts.redisConfig.RedisAddr != "127.0.0.1:6379" {
		t.Fatalf("RedisAddr = %q, want %q", opts.redisConfig.RedisAddr, "127.0.0.1:6379")
	}
	if opts.redisConfig.RedisDB != 0 {
		t.Fatalf("RedisDB = %d, want %d", opts.redisConfig.RedisDB, 0)
	}
}

func TestParseCLIRequiresKeyWithoutConfigDefault(t *testing.T) {
	_, _, err := parseCLI([]string{"doctor"})
	if err == nil {
		t.Fatal("parseCLI() returned nil error, want missing key error")
	}
	if !strings.Contains(err.Error(), "--key is required") {
		t.Fatalf("parseCLI() error = %q, want missing key message", err)
	}
}

func writeQMDConfig(t *testing.T, contents string) string {
	t.Helper()

	configFile := filepath.Join(t.TempDir(), "afs.config.json")
	if err := os.WriteFile(configFile, []byte(contents), 0o644); err != nil {
		t.Fatalf("os.WriteFile() returned error: %v", err)
	}
	return configFile
}
