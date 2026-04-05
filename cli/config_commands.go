package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type optionalString struct {
	value string
	set   bool
}

func (o *optionalString) String() string { return o.value }

func (o *optionalString) Set(v string) error {
	o.value = v
	o.set = true
	return nil
}

type optionalBool struct {
	value bool
	set   bool
}

func (o *optionalBool) String() string {
	if !o.set {
		return ""
	}
	return strconv.FormatBool(o.value)
}

func (o *optionalBool) Set(v string) error {
	if strings.TrimSpace(v) == "" {
		v = "true"
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return err
	}
	o.value = b
	o.set = true
	return nil
}

func (o *optionalBool) IsBoolFlag() bool { return true }

type configOverrides struct {
	redisURL     optionalString
	mountBackend optionalString
	mountpoint   optionalString
	readonly     optionalBool
}

func cmdConfig(args []string) error {
	if len(args) < 2 {
		printConfigUsage()
		return nil
	}
	if isHelpArg(args[1]) {
		printConfigUsage()
		return nil
	}
	switch args[1] {
	case "show":
		return cmdConfigShow(args[2:])
	case "set":
		return cmdConfigSet(args[2:])
	case "path":
		return cmdConfigPath(args[2:])
	default:
		return fmt.Errorf("unknown config subcommand %q\n\n%s", args[1], configUsageText(filepath.Base(os.Args[0])))
	}
}

func cmdConfigPath(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, configPathUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 0 {
		return fmt.Errorf("%s", configPathUsageText(filepath.Base(os.Args[0])))
	}
	fmt.Println(configPath())
	return nil
}

func cmdConfigShow(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, configShowUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	fs := flag.NewFlagSet("config show", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var jsonOut optionalBool
	fs.Var(&jsonOut, "json", "emit JSON")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s", configShowUsageText(filepath.Base(os.Args[0])))
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("%s", configShowUsageText(filepath.Base(os.Args[0])))
	}

	cfg, hasSavedConfig, err := loadConfigWithPresence()
	if err != nil {
		return err
	}
	if err := prepareConfigForSave(&cfg); err != nil {
		return err
	}

	if jsonOut.set && jsonOut.value {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cfg)
	}

	source := "defaults (not yet saved)"
	if hasSavedConfig {
		source = "saved"
	}
	printBox(clr(ansiBold, "config"), configSummaryRows(cfg, source))
	return nil
}

func cmdConfigSet(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, configSetUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	cfg, _, err := loadConfigWithPresence()
	if err != nil {
		return err
	}

	overrides, jsonOut, err := parseConfigOverrideFlags("config set", args, true)
	if err != nil {
		return err
	}
	if err := applyConfigOverrides(&cfg, overrides); err != nil {
		return err
	}
	if err := prepareConfigForSave(&cfg); err != nil {
		return err
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cfg)
	}

	rows := configSummaryRows(cfg, "saved")
	rows = append(rows, boxRow{Label: "config", Value: clr(ansiDim, configPath())})
	printBox(clr(ansiBGreen, "●")+" "+clr(ansiBold, "config updated"), rows)
	return nil
}

func loadConfigForUp(args []string) (config, error) {
	cfg, _, err := loadConfigWithPresence()
	if err != nil {
		return cfg, err
	}
	overrides, _, err := parseConfigOverrideFlags("up", args, false)
	if err != nil {
		return cfg, err
	}
	if err := applyConfigOverrides(&cfg, overrides); err != nil {
		return cfg, err
	}
	if err := resolveConfigPaths(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func loadConfigWithPresence() (config, bool, error) {
	cfg, err := loadConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultConfig(), false, nil
		}
		return cfg, false, err
	}
	return cfg, true, nil
}

func parseConfigOverrideFlags(command string, args []string, includeJSON bool) (configOverrides, bool, error) {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var overrides configOverrides
	registerConfigOverrideFlags(fs, &overrides)
	var jsonOut optionalBool
	if includeJSON {
		fs.Var(&jsonOut, "json", "emit JSON")
	}
	if err := fs.Parse(args); err != nil {
		return overrides, false, configUsageError(command)
	}
	if fs.NArg() != 0 {
		return overrides, false, configUsageError(command)
	}
	return overrides, jsonOut.set && jsonOut.value, nil
}

func registerConfigOverrideFlags(fs *flag.FlagSet, overrides *configOverrides) {
	fs.Var(&overrides.redisURL, "redis-url", "redis:// or rediss:// URL")
	fs.Var(&overrides.mountBackend, "mount-backend", "auto|none|fuse|nfs")
	fs.Var(&overrides.mountpoint, "mountpoint", "local mountpoint")
	fs.Var(&overrides.readonly, "readonly", "start readonly")
}

func configUsageError(command string) error {
	bin := filepath.Base(os.Args[0])
	switch command {
	case "up":
		return fmt.Errorf("%s", upUsageText(bin))
	case "config set":
		return fmt.Errorf("%s", configSetUsageText(bin))
	default:
		return fmt.Errorf("usage: %s %s", bin, command)
	}
}

func isHelpArg(v string) bool {
	switch strings.TrimSpace(v) {
	case "help", "-h", "--help":
		return true
	default:
		return false
	}
}

func printConfigUsage() {
	fmt.Fprint(os.Stderr, configUsageText(filepath.Base(os.Args[0])))
}

func configUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s config <subcommand>

Subcommands:
  show [--json]       Show the current config
  set [flags...]      Persist config changes non-interactively
  path                Print the config file path

Configurable settings:
  Redis connection    --redis-url
  Filesystem mount    --mount-backend, --mountpoint, --readonly

Workspace selection:
  Use '%s workspace use <workspace>' and related workspace commands.

Examples:
  %s config show --json
  %s config set --redis-url rediss://user:pass@redis.example:6379/4
  %s config set --mount-backend none
  %s config set --mount-backend nfs --mountpoint ~/demo

Run '%s config set --help' for the full flag list.
`, bin, bin, bin, bin, bin, bin, bin)
}

func configShowUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s config show [--json]

Options:
  --json              Emit the resolved config as JSON
`, bin)
}

func configPathUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s config path

Print the config file path AFS is using.
`, bin)
}

func configSetUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s config set [--json] [flags...]

Basics:
  --redis-url <redis://...|rediss://...>
  --mount-backend auto|none|fuse|nfs
  --mountpoint <path>
  --readonly[=true|false]

Output:
  --json              Print the saved config as JSON after updating it

Notes:
  Current workspace is not configured here. Use '%s workspace use <workspace>'.
  Advanced fields like runtime paths stay available in afs.config.json if needed.

Examples:
  %s config set --redis-url rediss://user:pass@redis.example:6379/4
  %s config set --mount-backend none
  %s config set --mount-backend nfs --mountpoint ~/demo
`, bin, bin, bin, bin, bin)
}

func upUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s up [flags...]

Start AFS using the saved config, with optional one-shot overrides.
These flags apply to the current run only and do not rewrite %s.

Basics:
  --redis-url <redis://...|rediss://...>
  --mount-backend auto|none|fuse|nfs
  --mountpoint <path>
  --readonly[=true|false]

Notes:
  Current workspace is not set here. Use '%s workspace use <workspace>'.
  Advanced fields like runtime paths stay available in afs.config.json if needed.

Examples:
  %s up --redis-url rediss://user:pass@redis.example:6379/4
  %s up --mount-backend none
  %s up --mount-backend nfs --mountpoint ~/demo
`, bin, configPath(), bin, bin, bin, bin)
}

func applyConfigOverrides(cfg *config, overrides configOverrides) error {
	if overrides.redisURL.set {
		if err := applyRedisURL(cfg, overrides.redisURL.value); err != nil {
			return err
		}
	}

	if overrides.mountBackend.set {
		cfg.MountBackend = strings.TrimSpace(overrides.mountBackend.value)
	}
	if overrides.mountpoint.set {
		cfg.Mountpoint = overrides.mountpoint.value
	}
	if overrides.readonly.set {
		cfg.ReadOnly = overrides.readonly.value
	}
	return nil
}

func applyRedisURL(cfg *config, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("parse redis url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "redis":
		cfg.RedisTLS = false
	case "rediss":
		cfg.RedisTLS = true
	default:
		return fmt.Errorf("unsupported redis url scheme %q (expected redis or rediss)", u.Scheme)
	}
	if strings.TrimSpace(u.Host) == "" {
		return fmt.Errorf("redis url must include host:port")
	}
	cfg.UseExistingRedis = true
	cfg.RedisAddr = u.Host
	if u.User != nil {
		cfg.RedisUsername = u.User.Username()
		if password, ok := u.User.Password(); ok {
			cfg.RedisPassword = password
		}
	}
	if queryDB := strings.TrimSpace(u.Query().Get("db")); queryDB != "" {
		db, err := strconv.Atoi(queryDB)
		if err != nil {
			return fmt.Errorf("parse redis db from query: %w", err)
		}
		cfg.RedisDB = db
	}
	if pathDB := strings.Trim(strings.TrimSpace(u.Path), "/"); pathDB != "" {
		db, err := strconv.Atoi(pathDB)
		if err != nil {
			return fmt.Errorf("parse redis db from path: %w", err)
		}
		cfg.RedisDB = db
	}
	return nil
}

func configSummaryRows(cfg config, source string) []boxRow {
	mountValue := "none"
	if cfg.MountBackend != mountBackendNone {
		mountValue = fmt.Sprintf("%s at %s", userModeLabel(cfg.MountBackend), cfg.Mountpoint)
	}

	rows := []boxRow{
		{Label: "source", Value: source},
		{Label: "redis", Value: publicRedisURL(cfg)},
		{Label: "filesystem mount", Value: mountValue},
		{Label: "readonly", Value: strconv.FormatBool(cfg.ReadOnly)},
	}
	return rows
}

func publicRedisURL(cfg config) string {
	scheme := "redis"
	if cfg.RedisTLS {
		scheme = "rediss"
	}
	userinfo := ""
	if strings.TrimSpace(cfg.RedisUsername) != "" {
		userinfo = url.User(cfg.RedisUsername).String() + "@"
	}
	return fmt.Sprintf("%s://%s%s/%d", scheme, userinfo, cfg.RedisAddr, cfg.RedisDB)
}
