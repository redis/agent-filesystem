package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
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
	redisURL             optionalString
	connection           optionalString
	controlPlaneURL      optionalString
	controlPlaneDatabase optionalString
	mode                 optionalString
	mountBackend         optionalString
	mountpoint           optionalString
	readonly             optionalBool
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
	case "get":
		return cmdConfigGet(args[2:])
	case "show":
		return cmdConfigShow(args[2:])
	case "set":
		return cmdConfigSet(args[2:])
	case "list":
		return cmdConfigList(args[2:])
	case "unset":
		return cmdConfigUnset(args[2:])
	case "reset":
		return cmdConfigReset(args[2:])
	default:
		return fmt.Errorf("unknown config subcommand %q\n\n%s", args[1], configUsageText(filepath.Base(os.Args[0])))
	}
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

	if len(args) == 2 && !strings.HasPrefix(strings.TrimSpace(args[0]), "-") {
		if err := setConfigKey(&cfg, args[0], args[1]); err != nil {
			return err
		}
		return persistConfigAndReport(cfg, []outputRow{
			{Label: "key", Value: normalizeConfigKey(args[0])},
		})
	}

	overrides, _, err := parseConfigOverrideFlags("config set", args, false)
	if err != nil {
		return err
	}
	if !hasConfigOverrides(overrides) {
		return fmt.Errorf("%s", configSetUsageText(filepath.Base(os.Args[0])))
	}
	if err := applyConfigOverrides(&cfg, overrides); err != nil {
		return err
	}
	return persistConfigAndReport(cfg, nil)
}

func cmdConfigGet(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, configGetUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	fs := flag.NewFlagSet("config get", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var jsonOut optionalBool
	fs.Var(&jsonOut, "json", "emit JSON")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s", configGetUsageText(filepath.Base(os.Args[0])))
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("%s", configGetUsageText(filepath.Base(os.Args[0])))
	}

	cfg, _, err := loadConfigWithPresence()
	if err != nil {
		return err
	}
	if err := prepareConfigForSave(&cfg); err != nil {
		return err
	}

	key := fs.Arg(0)
	value, err := getConfigKey(cfg, key)
	if err != nil {
		return err
	}
	if jsonOut.set && jsonOut.value {
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(map[string]string{normalizeConfigKey(key): stripAnsi(value)})
	}
	printSection(clr(ansiBold, "config"), []outputRow{
		{Label: "key", Value: normalizeConfigKey(key)},
		{Label: "value", Value: value},
		{Label: "config", Value: configPathLabel()},
	})
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
		return enc.Encode(persistedConfigFromRuntime(cfg))
	}

	source := "defaults (not yet saved)"
	if hasSavedConfig {
		source = "saved"
	}
	rows := configSummaryRows(cfg, source)
	rows = append(rows, outputRow{Label: "config file", Value: configPathLabel()})
	printSection(clr(ansiBold, "config"), rows)
	return nil
}

func cmdConfigList(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, configListUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	fs := flag.NewFlagSet("config list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var jsonOut optionalBool
	fs.Var(&jsonOut, "json", "emit JSON")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s", configListUsageText(filepath.Base(os.Args[0])))
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("%s", configListUsageText(filepath.Base(os.Args[0])))
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
		return enc.Encode(configListMap(cfg))
	}

	source := "defaults (not yet saved)"
	if hasSavedConfig {
		source = "saved"
	}
	rows := []outputRow{{Label: "source", Value: source}}
	for _, entry := range configKeys() {
		value, err := getConfigKey(cfg, entry)
		if err != nil {
			return err
		}
		rows = append(rows, outputRow{Label: entry, Value: value})
	}
	rows = append(rows, outputRow{Label: "config file", Value: configPathLabel()})
	printSection(clr(ansiBold, "config"), rows)
	return nil
}

func cmdConfigUnset(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, configUnsetUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("%s", configUnsetUsageText(filepath.Base(os.Args[0])))
	}

	cfg, _, err := loadConfigWithPresence()
	if err != nil {
		return err
	}
	if err := unsetConfigKey(&cfg, args[0]); err != nil {
		return err
	}
	return persistConfigAndReport(cfg, []outputRow{
		{Label: "key", Value: normalizeConfigKey(args[0])},
	})
}

func cmdConfigReset(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, configResetUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 0 {
		return fmt.Errorf("%s", configResetUsageText(filepath.Base(os.Args[0])))
	}
	return cmdReset()
}

func loadConfigForUp(args []string) (config, error) {
	return loadConfigForUpWithOverridesAndMode(args, configOverrides{}, optionalString{})
}

func loadConfigForUpWithMode(args []string, mode optionalString) (config, error) {
	return loadConfigForUpWithOverridesAndMode(args, configOverrides{}, mode)
}

func loadConfigForUpWithOverridesAndMode(args []string, overrides configOverrides, mode optionalString) (config, error) {
	return loadConfigForUpWithIOAndOverridesAndMode(args, overrides, mode, bufio.NewReader(os.Stdin), os.Stdout, isInteractiveTerminal())
}

type upConfigPresence struct {
	filePresent      bool
	redisDBPresent   bool
	localPathPresent bool
}

func loadConfigForUpWithIO(args []string, r *bufio.Reader, out io.Writer, allowPrompt bool) (config, error) {
	return loadConfigForUpWithIOAndOverridesAndMode(args, configOverrides{}, optionalString{}, r, out, allowPrompt)
}

func loadConfigForUpWithIOAndMode(args []string, mode optionalString, r *bufio.Reader, out io.Writer, allowPrompt bool) (config, error) {
	return loadConfigForUpWithIOAndOverridesAndMode(args, configOverrides{}, mode, r, out, allowPrompt)
}

func loadConfigForUpWithIOAndOverridesAndMode(args []string, overrides configOverrides, mode optionalString, r *bufio.Reader, out io.Writer, allowPrompt bool) (config, error) {
	cfg, presence, err := loadConfigWithUpPresence()
	if err != nil {
		return cfg, err
	}
	if !presence.filePresent && !upHasExplicitInputs(args, overrides, mode) {
		return cfg, fmt.Errorf("no configuration found\nRun '%s setup' to get started", filepath.Base(os.Args[0]))
	}

	if err := validateUpModeOverride(mode); err != nil {
		return cfg, err
	}
	if err := applyConfigOverrides(&cfg, overrides); err != nil {
		return cfg, err
	}
	presence = upPresenceWithOverrides(presence, overrides)

	changed := mode.set
	if mode.set {
		cfg.Mode = strings.TrimSpace(mode.value)
	}
	switch len(args) {
	case 0:
		var promptedChanged bool
		promptedChanged, err = promptForMissingUpConfig(&cfg, presence, r, out, allowPrompt)
		if err != nil {
			return cfg, err
		}
		changed = changed || promptedChanged
	case 1:
		mountpoint, err := defaultMountpointForWorkspace(args[0])
		if err != nil {
			return cfg, err
		}
		if err := applyUpWorkspaceAndMountpoint(&cfg, args[0], mountpoint); err != nil {
			return cfg, err
		}
		changed = true
	case 2:
		if err := applyUpWorkspaceAndMountpoint(&cfg, args[0], args[1]); err != nil {
			return cfg, err
		}
		changed = true
	default:
		return cfg, fmt.Errorf("expected at most <workspace> <directory>\nUse '%s ws mount <workspace> <directory>'", filepath.Base(os.Args[0]))
	}

	if err := validateUpModeSelection(cfg); err != nil {
		return cfg, err
	}
	if changed && !hasConfigOverrides(overrides) {
		if err := persistConfigForUp(&cfg); err != nil {
			return cfg, err
		}
	}
	if err := resolveConfigPaths(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func prepareMountedConfig(cfg config, workspace, mountpoint string) (config, error) {
	if err := applyUpWorkspaceAndMountpoint(&cfg, workspace, mountpoint); err != nil {
		return cfg, err
	}
	if err := resolveConfigPaths(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// defaultMountpointForWorkspace returns ~/afs/<workspace> and verifies the
// path is available (not already occupied by a non-directory or a mount).
func defaultMountpointForWorkspace(workspace string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	mountpoint := filepath.Join(home, "afs", workspace)

	info, err := os.Stat(mountpoint)
	if err == nil {
		// Path exists — only accept it if it's a directory.
		if !info.IsDir() {
			return "", fmt.Errorf("default mountpoint %s already exists and is not a directory; specify a mountpoint explicitly", mountpoint)
		}
	}
	// Path doesn't exist or is a directory — both are fine.
	return mountpoint, nil
}

func applyUpWorkspaceAndMountpoint(cfg *config, workspace, mountpoint string) error {
	if cfg == nil {
		return fmt.Errorf("missing config")
	}
	if err := validateAFSName("workspace", workspace); err != nil {
		return err
	}

	mode, err := effectiveMode(*cfg)
	if err != nil {
		return err
	}
	if mode == modeNone {
		return fmt.Errorf("mode is set to none; update config first")
	}
	if mode == modeMount {
		backendName, err := normalizeMountBackend(cfg.MountBackend)
		if err != nil {
			return err
		}
		if backendName == mountBackendNone {
			return fmt.Errorf("filesystem mounts are disabled in config\nRun '%s config set --mount-backend nfs --mountpoint %s' or similar first", filepath.Base(os.Args[0]), mountpoint)
		}
	}

	cfg.CurrentWorkspace = workspace
	cfg.CurrentWorkspaceID = ""
	cfg.LocalPath = mountpoint
	return nil
}

func validateUpModeSelection(cfg config) error {
	mode, err := effectiveMode(cfg)
	if err != nil {
		return err
	}
	if mode != modeMount {
		return nil
	}
	backendName, err := normalizeMountBackend(cfg.MountBackend)
	if err != nil {
		return err
	}
	if backendName == mountBackendNone {
		bin := filepath.Base(os.Args[0])
		return fmt.Errorf("mode=mount requires a configured mount backend\nRun '%s config set --mount-backend nfs --mountpoint <path>' or rerun '%s setup'", bin, bin)
	}
	return nil
}

func validateUpModeOverride(mode optionalString) error {
	if !mode.set {
		return nil
	}
	switch strings.TrimSpace(mode.value) {
	case modeSync, modeMount:
		return nil
	default:
		return fmt.Errorf("unsupported value for --mode %q (expected sync or mount)", mode.value)
	}
}

func loadConfigWithUpPresence() (config, upConfigPresence, error) {
	cfg := defaultConfig()
	var presence upConfigPresence

	raw, err := os.ReadFile(configPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, presence, nil
		}
		return cfg, presence, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, presence, err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return cfg, presence, err
	}

	presence.filePresent = true
	if rawRedis, ok := fields["redis"]; ok {
		var redisFields map[string]json.RawMessage
		if err := json.Unmarshal(rawRedis, &redisFields); err != nil {
			return cfg, presence, err
		}
		_, presence.redisDBPresent = redisFields["db"]
	}
	_, presence.localPathPresent = fields["localPath"]
	return cfg, presence, nil
}

func upHasExplicitInputs(args []string, overrides configOverrides, mode optionalString) bool {
	return len(args) > 0 || mode.set || hasConfigOverrides(overrides)
}

func hasConfigOverrides(overrides configOverrides) bool {
	return overrides.redisURL.set ||
		overrides.connection.set ||
		overrides.controlPlaneURL.set ||
		overrides.controlPlaneDatabase.set ||
		overrides.mode.set ||
		overrides.mountBackend.set ||
		overrides.mountpoint.set ||
		overrides.readonly.set
}

func upPresenceWithOverrides(presence upConfigPresence, overrides configOverrides) upConfigPresence {
	if hasConfigOverrides(overrides) {
		presence.filePresent = true
	}
	if overrides.redisURL.set {
		presence.redisDBPresent = true
	}
	if overrides.mountpoint.set {
		presence.localPathPresent = true
	}
	return presence
}

func promptForMissingUpConfig(cfg *config, presence upConfigPresence, r *bufio.Reader, out io.Writer, allowPrompt bool) (bool, error) {
	if cfg == nil {
		return false, fmt.Errorf("missing config")
	}

	productMode, err := effectiveProductMode(*cfg)
	if err != nil {
		return false, err
	}
	mode, err := effectiveMode(*cfg)
	if err != nil {
		return false, err
	}

	missingDatabase := productMode == productModeLocal && (!presence.filePresent || !presence.redisDBPresent)
	missingWorkspace := mode != modeNone && strings.TrimSpace(cfg.CurrentWorkspace) == ""
	missingLocalPath := mode != modeNone && (!presence.filePresent || !presence.localPathPresent || strings.TrimSpace(cfg.LocalPath) == "")
	if !missingDatabase && !missingWorkspace && !missingLocalPath {
		return false, nil
	}
	if missingWorkspace {
		return false, fmt.Errorf("workspace is required\nRun '%s ws mount <workspace> <directory>'", filepath.Base(os.Args[0]))
	}
	if !allowPrompt {
		return false, fmt.Errorf("config is missing settings required for 'up'\nRun '%s setup' or use an interactive terminal so AFS can prompt for the missing database and local path", filepath.Base(os.Args[0]))
	}

	changed := false
	if missingDatabase {
		value, err := promptString(r, out,
			"  Redis database\n  "+clr(ansiDim, "Choose the Redis database number for this AFS config"),
			strconv.Itoa(cfg.RedisDB))
		if err != nil {
			return false, err
		}
		db, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return false, fmt.Errorf("invalid redis database %q", value)
		}
		if db < 0 {
			return false, fmt.Errorf("redis db must be >= 0")
		}
		cfg.RedisDB = db
		changed = true
	}

	if missingLocalPath {
		defaultLocalPath := strings.TrimSpace(cfg.LocalPath)
		if defaultLocalPath == "" {
			defaultLocalPath = defaultConfig().LocalPath
		}
		mountpoint, err := promptString(r, out,
			"  Local mountpoint\n  "+clr(ansiDim, "Directory where the workspace should be mounted"),
			defaultLocalPath)
		if err != nil {
			return false, err
		}
		mountpoint = strings.TrimSpace(mountpoint)
		if mountpoint == "" {
			return false, fmt.Errorf("local path cannot be empty when starting a mounted filesystem")
		}
		resolvedMountpoint, err := expandPath(mountpoint)
		if err != nil {
			return false, err
		}
		if err := validateMountpointPath(resolvedMountpoint); err != nil {
			return false, err
		}
		cfg.LocalPath = resolvedMountpoint
		changed = true
	}

	return changed, nil
}

func suggestUpWorkspace(cfg config) (string, string) {
	names, err := existingWorkspaceNames(cfg)
	if err != nil || len(names) == 0 {
		return "", "Enter a workspace name to mount"
	}
	if len(names) == 1 {
		return names[0], "Available workspace: " + names[0]
	}
	return names[0], "Available workspaces: " + strings.Join(names, ", ")
}

func existingWorkspaceNames(cfg config) ([]string, error) {
	redisCfg := cfg
	if err := prepareConfigForSave(&redisCfg); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rdb := redis.NewClient(buildRedisOptions(redisCfg, 4))
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, nil
	}
	workspaces, err := newAFSStore(rdb).listWorkspaces(ctx)
	if err != nil {
		return nil, nil
	}

	names := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		if strings.TrimSpace(workspace.Name) != "" {
			names = append(names, workspace.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func persistConfigForUp(cfg *config) error {
	if cfg == nil {
		return fmt.Errorf("missing config")
	}
	persisted := *cfg
	if err := prepareConfigForSave(&persisted); err != nil {
		return err
	}
	if err := validateConfiguredMountpoint(persisted); err != nil {
		return err
	}
	if err := saveConfig(persisted); err != nil {
		return err
	}
	*cfg = persisted
	return nil
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
	fs.Var(&overrides.connection, "config-source", "local|self-managed|cloud")
	fs.Var(&overrides.connection, "control", "alias for --config-source")
	fs.Var(&overrides.connection, "connection", "alias for --config-source")
	fs.Var(&overrides.connection, "product-mode", "alias for --config-source")
	fs.Var(&overrides.controlPlaneURL, "control-plane-url", "http:// or https:// control plane URL")
	fs.Var(&overrides.controlPlaneDatabase, "control-plane-database", "database id for self-managed control plane mode")
	fs.Var(&overrides.mode, "mode", "sync|mount")
	fs.Var(&overrides.mountBackend, "mount-backend", "auto|none|fuse|nfs")
	fs.Var(&overrides.mountpoint, "mountpoint", "local mountpoint")
	fs.Var(&overrides.readonly, "readonly", "start readonly")
}

func configUsageError(command string) error {
	bin := filepath.Base(os.Args[0])
	switch command {
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
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s config <subcommand>

Subcommands:
  get <key> [--json]      Read a config value
  show [--json]           Show the full saved config
  set <key> <value>       Persist a config value
  list [--json]           List known config values
  unset <key>             Reset a config value to its default
  reset                   Reset local config and runtime state

Common keys:
  config.source
  agent.name
  controlPlane.url
  controlPlane.database
  mode
  redis.url
  sync.fileSizeCapMB

Examples:
  %s config get redis.url
  %s config show --json
  %s config set config.source self-managed
  %s config set mode mount
  %s config set controlPlane.url http://127.0.0.1:8091
  %s config set agent.name "Claude Code"
  %s config unset controlPlane.database
  %s config reset
  %s config list
`, bin, bin, bin, bin, bin, bin, bin, bin, bin, bin)
}

func configGetUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s config get <key> [--json]

Options:
  --json              Emit JSON output
`, bin)
}

func configListUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s config list [--json]

List the known AFS config values.
`, bin)
}

func configShowUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s config show [--json]

Show the saved AFS config.
`, bin)
}

func configSetUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s config set <key> <value>
  %s config set [flags]

Flags:
  --redis-url <redis://...|rediss://...>
  --config-source local|self-hosted|cloud
  --control-plane-url <http://...|https://...>
  --control-plane-database <id>
  --mode sync|mount
  --mount-backend auto|none|fuse|nfs
  --mountpoint <path>
  --readonly

Examples:
  %s config set redis.url rediss://user:pass@redis.example:6379/4
  %s config set config.source self-managed
  %s config set mode mount
  %s config set agent.name "Claude Code"
  %s config set sync.fileSizeCapMB 4096
  %s config set controlPlane.url http://127.0.0.1:8091

Notes:
  Keys are case-insensitive.
  Use "self-managed" for the control-plane-backed mode.
  Workspace mounts are runtime state; use '%s ws mount <workspace> <directory>'.
  Current workspace is not configured here.
  Mode and other runtime paths stay available in afs.config.json.
`, bin, bin, bin, bin, bin, bin, bin, bin, bin)
}

func configUnsetUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s config unset <key>

Reset a config value to its default or empty state.
`, bin)
}

func configResetUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s config reset

Reset local config and runtime state, while keeping the CLI installed.
If AFS is running, this command stops it first.
`, bin)
}

func applyConfigOverrides(cfg *config, overrides configOverrides) error {
	if overrides.redisURL.set {
		if err := applyRedisURL(cfg, overrides.redisURL.value); err != nil {
			return err
		}
	}
	if overrides.connection.set {
		mode, err := parseUserFacingConfigSource(overrides.connection.value)
		if err != nil {
			return err
		}
		cfg.ProductMode = mode
		if cfg.ProductMode == productModeLocal {
			cfg.DatabaseID = ""
			cfg.CurrentWorkspaceID = ""
		}
	}
	if overrides.controlPlaneURL.set {
		cfg.URL = strings.TrimSpace(overrides.controlPlaneURL.value)
		// Providing a control plane URL implies self-hosted mode.
		if !overrides.connection.set {
			cfg.ProductMode = productModeSelfHosted
		}
		if !overrides.controlPlaneDatabase.set {
			cfg.DatabaseID = ""
		}
		cfg.CurrentWorkspaceID = ""
	}
	if overrides.controlPlaneDatabase.set {
		cfg.DatabaseID = strings.TrimSpace(overrides.controlPlaneDatabase.value)
		cfg.CurrentWorkspaceID = ""
	}
	if overrides.mode.set {
		mode, err := parseConfigMode(overrides.mode.value)
		if err != nil {
			return err
		}
		cfg.Mode = mode
	}

	if overrides.mountBackend.set {
		cfg.MountBackend = strings.TrimSpace(overrides.mountBackend.value)
	}
	if overrides.mountpoint.set {
		cfg.LocalPath = overrides.mountpoint.value
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

func configSummaryRows(cfg config, source string) []outputRow {
	productMode, _ := effectiveProductMode(cfg)
	rows := []outputRow{
		{Label: "source", Value: source},
		{Label: "connection", Value: userFacingConfigSource(cfg)},
		{Label: "mode", Value: configModeLabel(cfg)},
		{Label: "database", Value: publicRedisURL(cfg)},
		{Label: "agent", Value: agentConfigLabel(cfg)},
	}
	if productMode != productModeLocal {
		rows[3] = outputRow{Label: "control plane", Value: configRemoteLabel(cfg)}
	}
	if cfg.SyncFileSizeCapMB > 0 && cfg.SyncFileSizeCapMB != defaultSyncFileSizeCapMB {
		rows = append(rows, outputRow{Label: "sync file cap", Value: strconv.Itoa(cfg.SyncFileSizeCapMB) + " MB"})
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

func configKeys() []string {
	return []string{
		"config.source",
		"agent.name",
		"controlPlane.url",
		"controlPlane.database",
		"mode",
		"redis.url",
		"sync.fileSizeCapMB",
	}
}

func configListMap(cfg config) map[string]string {
	out := make(map[string]string, len(configKeys()))
	for _, key := range configKeys() {
		value, err := getConfigKey(cfg, key)
		if err == nil {
			out[key] = stripAnsi(value)
		}
	}
	out["config.file"] = configPath()
	return out
}

func persistConfigAndReport(cfg config, prefixRows []outputRow) error {
	if err := prepareConfigForSave(&cfg); err != nil {
		return err
	}
	if err := validateConfiguredMountpoint(cfg); err != nil {
		return err
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}

	rows := append([]outputRow{}, prefixRows...)
	if len(prefixRows) == 1 && prefixRows[0].Label == "key" {
		value, err := getConfigKey(cfg, prefixRows[0].Value)
		if err != nil {
			return err
		}
		rows = append(rows, outputRow{Label: "value", Value: value})
	} else {
		rows = append(rows, configSummaryRows(cfg, "saved")...)
	}
	rows = append(rows, outputRow{Label: "config", Value: clr(ansiDim, compactDisplayPath(configPath()))})
	printSection(markerSuccess+" "+clr(ansiBold, "config updated"), rows)
	return nil
}

func normalizeConfigKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	key = strings.ReplaceAll(key, " ", "")
	switch strings.ToLower(key) {
	case "config.source", "configsource", "connection", "productmode":
		return "config.source"
	case "agent.name", "agentname":
		return "agent.name"
	case "controlplane.url", "controlplaneurl":
		return "controlPlane.url"
	case "controlplane.database", "controlplanedatabase", "controlplane.databaseid", "controlplanedatabaseid":
		return "controlPlane.database"
	case "mode", "mountmode", "localmode", "runtime.mode", "runtimemode":
		return "mode"
	case "redis.url", "redisurl":
		return "redis.url"
	case "sync.filesizecapmb", "sync.file.size.cap.mb", "syncfilecap", "syncfilesizecapmb":
		return "sync.fileSizeCapMB"
	default:
		return strings.TrimSpace(key)
	}
}

func getConfigKey(cfg config, key string) (string, error) {
	switch normalizeConfigKey(key) {
	case "config.source":
		return userFacingConfigSource(cfg), nil
	case "controlPlane.url":
		if strings.TrimSpace(cfg.URL) == "" {
			return clr(ansiDim, "unset"), nil
		}
		return cfg.URL, nil
	case "agent.name":
		if strings.TrimSpace(cfg.Name) == "" {
			return clr(ansiDim, "unset"), nil
		}
		return cfg.Name, nil
	case "controlPlane.database":
		if strings.TrimSpace(cfg.DatabaseID) == "" {
			return clr(ansiDim, "auto"), nil
		}
		return cfg.DatabaseID, nil
	case "mode":
		return configModeLabel(cfg), nil
	case "redis.url":
		return publicRedisURL(cfg), nil
	case "sync.fileSizeCapMB":
		mb := cfg.SyncFileSizeCapMB
		if mb <= 0 {
			mb = defaultSyncFileSizeCapMB
		}
		return strconv.Itoa(mb), nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}

func setConfigKey(cfg *config, key, value string) error {
	key = normalizeConfigKey(key)
	switch key {
	case "config.source":
		mode, err := parseUserFacingConfigSource(value)
		if err != nil {
			return err
		}
		cfg.ProductMode = mode
		if mode == productModeLocal {
			cfg.URL = ""
			cfg.DatabaseID = ""
			cfg.AuthToken = ""
			cfg.Account = ""
			cfg.CurrentWorkspaceID = ""
		}
	case "controlPlane.url":
		cfg.ProductMode = productModeSelfHosted
		cfg.URL = strings.TrimSpace(value)
		cfg.DatabaseID = ""
		cfg.CurrentWorkspaceID = ""
	case "agent.name":
		cfg.Name = strings.TrimSpace(value)
	case "controlPlane.database":
		cfg.DatabaseID = strings.TrimSpace(value)
	case "mode":
		mode, err := parseConfigMode(value)
		if err != nil {
			return err
		}
		cfg.Mode = mode
	case "redis.url":
		if err := applyRedisURL(cfg, strings.TrimSpace(value)); err != nil {
			return err
		}
	case "sync.fileSizeCapMB":
		mb, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || mb < 0 {
			return fmt.Errorf("sync.fileSizeCapMB must be a non-negative integer")
		}
		cfg.SyncFileSizeCapMB = mb
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

func unsetConfigKey(cfg *config, key string) error {
	def := defaultConfig()
	switch normalizeConfigKey(key) {
	case "config.source":
		cfg.ProductMode = def.ProductMode
		cfg.URL = ""
		cfg.DatabaseID = ""
		cfg.AuthToken = ""
		cfg.Account = ""
		cfg.CurrentWorkspaceID = ""
	case "controlPlane.url":
		cfg.URL = ""
		cfg.CurrentWorkspaceID = ""
	case "agent.name":
		cfg.Name = ""
	case "controlPlane.database":
		cfg.DatabaseID = ""
	case "mode":
		cfg.Mode = def.Mode
	case "redis.url":
		cfg.RedisAddr = def.RedisAddr
		cfg.RedisUsername = ""
		cfg.RedisPassword = ""
		cfg.RedisDB = def.RedisDB
		cfg.RedisTLS = false
	case "sync.fileSizeCapMB":
		cfg.SyncFileSizeCapMB = def.SyncFileSizeCapMB
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

func configModeLabel(cfg config) string {
	mode, err := effectiveMode(cfg)
	if err != nil {
		return strings.TrimSpace(cfg.Mode)
	}
	return mode
}

func parseConfigMode(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case modeSync, "":
		return modeSync, nil
	case modeMount:
		return modeMount, nil
	default:
		return "", fmt.Errorf("mode must be one of: sync, mount")
	}
}

func userFacingConfigSource(cfg config) string {
	mode, _ := effectiveProductMode(cfg)
	switch mode {
	case productModeSelfHosted:
		return "self-managed"
	default:
		return mode
	}
}

func parseUserFacingConfigSource(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case productModeLocal, "":
		return productModeLocal, nil
	case "self-managed", productModeSelfHosted, "selfmanaged":
		return productModeSelfHosted, nil
	case productModeCloud:
		return productModeCloud, nil
	default:
		return "", fmt.Errorf("config.source must be one of: local, self-managed, cloud")
	}
}

func agentConfigLabel(cfg config) string {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return clr(ansiDim, "unset")
	}
	if id := strings.TrimSpace(cfg.ID); id != "" {
		return fmt.Sprintf("%s (%s)", name, id)
	}
	return name
}
