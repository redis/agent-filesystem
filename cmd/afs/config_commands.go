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
	mountBackend         optionalString
	mountpoint           optionalString
	readonly             optionalBool
}

type upOptions struct {
	foreground  bool
	mode        optionalString
	overrides   configOverrides
	positionals []string
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
	case "set":
		return cmdConfigSet(args[2:])
	case "list":
		return cmdConfigList(args[2:])
	case "unset":
		return cmdConfigUnset(args[2:])
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
	if len(args) != 2 {
		return fmt.Errorf("%s", configSetUsageText(filepath.Base(os.Args[0])))
	}
	if err := setConfigKey(&cfg, args[0], args[1]); err != nil {
		return err
	}
	if err := prepareConfigForSave(&cfg); err != nil {
		return err
	}
	if err := validateConfiguredMountpoint(cfg); err != nil {
		return err
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}

	value, err := getConfigKey(cfg, args[0])
	if err != nil {
		return err
	}
	rows := []boxRow{
		{Label: "key", Value: normalizeConfigKey(args[0])},
		{Label: "value", Value: value},
		{Label: "config", Value: clr(ansiDim, compactDisplayPath(configPath()))},
	}
	printBox(markerSuccess+" "+clr(ansiBold, "config updated"), rows)
	return nil
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
	printBox(clr(ansiBold, "config"), []boxRow{
		{Label: "key", Value: normalizeConfigKey(key)},
		{Label: "value", Value: value},
		{Label: "config", Value: configPathLabel()},
	})
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
	rows := []boxRow{{Label: "source", Value: source}}
	for _, entry := range configKeys() {
		value, err := getConfigKey(cfg, entry)
		if err != nil {
			return err
		}
		rows = append(rows, boxRow{Label: entry, Value: value})
	}
	rows = append(rows, boxRow{Label: "config file", Value: configPathLabel()})
	printBox(clr(ansiBold, "config"), rows)
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
	if err := prepareConfigForSave(&cfg); err != nil {
		return err
	}
	if err := validateConfiguredMountpoint(cfg); err != nil {
		return err
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}
	value, err := getConfigKey(cfg, args[0])
	if err != nil {
		return err
	}
	printBox(markerSuccess+" "+clr(ansiBold, "config updated"), []boxRow{
		{Label: "key", Value: normalizeConfigKey(args[0])},
		{Label: "value", Value: value},
		{Label: "config", Value: clr(ansiDim, compactDisplayPath(configPath()))},
	})
	return nil
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
		return cfg, fmt.Errorf("%s", upUsageText(filepath.Base(os.Args[0])))
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
		return fmt.Errorf("mode is set to none\nRun '%s up --mode sync', '%s up --mode mount', or update config first", filepath.Base(os.Args[0]), filepath.Base(os.Args[0]))
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
		bin := filepath.Base(os.Args[0])
		return false, fmt.Errorf("no current workspace is selected for 'up'\nRun '%s workspace use <workspace>' or '%s up <workspace> <mountpoint>'", bin, bin)
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

func parseUpOptions(args []string) (upOptions, error) {
	var opts upOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--interactive" || arg == "-i":
			opts.foreground = true
		case arg == "--mode":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for %q", arg)
			}
			i++
			opts.mode.value = strings.TrimSpace(args[i])
			opts.mode.set = true
		case strings.HasPrefix(arg, "--mode="):
			opts.mode.value = strings.TrimSpace(strings.TrimPrefix(arg, "--mode="))
			opts.mode.set = true
		case arg == "--redis-url":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for %q", arg)
			}
			i++
			opts.overrides.redisURL.value = strings.TrimSpace(args[i])
			opts.overrides.redisURL.set = true
		case strings.HasPrefix(arg, "--redis-url="):
			opts.overrides.redisURL.value = strings.TrimSpace(strings.TrimPrefix(arg, "--redis-url="))
			opts.overrides.redisURL.set = true
		case arg == "--config-source" || arg == "--control" || arg == "--connection" || arg == "--product-mode":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for %q", arg)
			}
			i++
			opts.overrides.connection.value = strings.TrimSpace(args[i])
			opts.overrides.connection.set = true
		case strings.HasPrefix(arg, "--config-source="):
			opts.overrides.connection.value = strings.TrimSpace(strings.TrimPrefix(arg, "--config-source="))
			opts.overrides.connection.set = true
		case strings.HasPrefix(arg, "--control="):
			opts.overrides.connection.value = strings.TrimSpace(strings.TrimPrefix(arg, "--control="))
			opts.overrides.connection.set = true
		case strings.HasPrefix(arg, "--connection="):
			opts.overrides.connection.value = strings.TrimSpace(strings.TrimPrefix(arg, "--connection="))
			opts.overrides.connection.set = true
		case strings.HasPrefix(arg, "--product-mode="):
			opts.overrides.connection.value = strings.TrimSpace(strings.TrimPrefix(arg, "--product-mode="))
			opts.overrides.connection.set = true
		case arg == "--control-plane-url":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for %q", arg)
			}
			i++
			opts.overrides.controlPlaneURL.value = strings.TrimSpace(args[i])
			opts.overrides.controlPlaneURL.set = true
		case strings.HasPrefix(arg, "--control-plane-url="):
			opts.overrides.controlPlaneURL.value = strings.TrimSpace(strings.TrimPrefix(arg, "--control-plane-url="))
			opts.overrides.controlPlaneURL.set = true
		case arg == "--control-plane-database":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for %q", arg)
			}
			i++
			opts.overrides.controlPlaneDatabase.value = strings.TrimSpace(args[i])
			opts.overrides.controlPlaneDatabase.set = true
		case strings.HasPrefix(arg, "--control-plane-database="):
			opts.overrides.controlPlaneDatabase.value = strings.TrimSpace(strings.TrimPrefix(arg, "--control-plane-database="))
			opts.overrides.controlPlaneDatabase.set = true
		case arg == "--mount-backend":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for %q", arg)
			}
			i++
			opts.overrides.mountBackend.value = strings.TrimSpace(args[i])
			opts.overrides.mountBackend.set = true
		case strings.HasPrefix(arg, "--mount-backend="):
			opts.overrides.mountBackend.value = strings.TrimSpace(strings.TrimPrefix(arg, "--mount-backend="))
			opts.overrides.mountBackend.set = true
		case arg == "--mountpoint":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for %q", arg)
			}
			i++
			opts.overrides.mountpoint.value = args[i]
			opts.overrides.mountpoint.set = true
		case strings.HasPrefix(arg, "--mountpoint="):
			opts.overrides.mountpoint.value = strings.TrimPrefix(arg, "--mountpoint=")
			opts.overrides.mountpoint.set = true
		case arg == "--readonly":
			opts.overrides.readonly.value = true
			opts.overrides.readonly.set = true
		case strings.HasPrefix(arg, "--readonly="):
			opts.overrides.readonly.set = true
			if err := opts.overrides.readonly.Set(strings.TrimPrefix(arg, "--readonly=")); err != nil {
				return opts, configUsageError("up")
			}
		case strings.HasPrefix(arg, "-"):
			return opts, configUsageError("up")
		default:
			opts.positionals = append(opts.positionals, arg)
		}
	}
	return opts, nil
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
  set <key> <value>       Persist a config value
  list [--json]           List known config values
  unset <key>             Reset a config value to its default

Common keys:
  config.source
  controlPlane.url
  controlPlane.database
  redis.url
  mount.backend
  mount.path
  mount.readonly
  workspace.current

Workspace selection:
  Use '%s workspace use <workspace>' and related workspace commands.

Examples:
  %s config get redis.url
  %s config set config.source self-managed
  %s config set mount.path ~/demo
  %s config unset controlPlane.database
  %s config list
`, bin, bin, bin, bin, bin, bin, bin)
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

func configSetUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s config set <key> <value>

Examples:
  %s config set redis.url rediss://user:pass@redis.example:6379/4
  %s config set config.source self-managed
  %s config set controlPlane.url http://127.0.0.1:8091
  %s config set mount.backend nfs
  %s config set mount.readonly true

Notes:
  Keys are case-insensitive.
  Use "self-managed" for the control-plane-backed mode.
`, bin, bin, bin, bin, bin, bin)
}

func configUnsetUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s config unset <key>

Reset a config value to its default or empty state.
`, bin)
}

func upUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s up [flags]
  %s up <workspace> [<mountpoint>]

Start AFS using the saved config.
If <workspace> is provided, AFS saves it and starts. The mountpoint defaults
to ~/afs/<workspace> when omitted. Both are persisted so future runs reuse them.

Flags:
  --mode <sync|mount>
                      Persist a mode override before starting.
  --control-plane-url <http://...|https://...>
                      One-shot self-managed control plane URL override.
  --control-plane-database <database-id>
                      One-shot database override for self-managed mode.
  --redis-url <redis://...|rediss://...>
                      One-shot Redis override for local mode.
  --mount-backend <auto|none|fuse|nfs>
  --mountpoint <path>
  --readonly[=true|false]
                      One-shot local surface overrides for this run.
  --interactive, -i   Run the sync daemon in the foreground with live logs.
                      Ctrl-C stops the daemon. Useful for debugging.

Notes:
  Saved config is the default, but explicit 'up' flags override it for this run.
  Redis connection, mount backend, and readonly mode come from config unless
  you override them on the command line.
  Current workspace must already be selected with '%s workspace use <workspace>'
  unless you pass <workspace> positionally.
  If Redis DB or mountpoint are missing, AFS prompts for them in the terminal.
  Use '%s config set ...' to change Redis or mount settings persistently.

Examples:
  %s up
  %s up --mode sync
  %s up --control-plane-url http://127.0.0.1:8091 getting-started
  %s up --interactive
  %s up claude-code ~/.claude
`, bin, bin, bin, bin, bin, bin, bin, bin, bin)
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

func configSummaryRows(cfg config, source string) []boxRow {
	mountValue := "none"
	if cfg.MountBackend != mountBackendNone {
		mountValue = fmt.Sprintf("%s at %s", userModeLabel(cfg.MountBackend), cfg.LocalPath)
	}

	productMode, _ := effectiveProductMode(cfg)
	rows := []boxRow{
		{Label: "source", Value: source},
		{Label: "config source", Value: userFacingConfigSource(cfg)},
		{Label: "database", Value: publicRedisURL(cfg)},
		{Label: "workspace", Value: currentWorkspaceLabel(selectedWorkspaceName(cfg))},
		{Label: "mount", Value: mountValue},
		{Label: "readonly", Value: strconv.FormatBool(cfg.ReadOnly)},
	}
	if productMode != productModeLocal {
		rows[2] = boxRow{Label: "control plane", Value: configRemoteLabel(cfg)}
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
		"controlPlane.url",
		"controlPlane.database",
		"redis.url",
		"mount.backend",
		"mount.path",
		"mount.readonly",
		"workspace.current",
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

func normalizeConfigKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	key = strings.ReplaceAll(key, " ", "")
	switch strings.ToLower(key) {
	case "config.source", "configsource", "connection", "productmode":
		return "config.source"
	case "controlplane.url", "controlplaneurl":
		return "controlPlane.url"
	case "controlplane.database", "controlplanedatabase", "controlplane.databaseid", "controlplanedatabaseid":
		return "controlPlane.database"
	case "redis.url", "redisurl":
		return "redis.url"
	case "mount.backend", "mountbackend":
		return "mount.backend"
	case "mount.path", "mountpoint", "localpath", "mountpath":
		return "mount.path"
	case "mount.readonly", "readonly", "mountreadonly":
		return "mount.readonly"
	case "workspace.current", "currentworkspace", "workspace":
		return "workspace.current"
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
	case "controlPlane.database":
		if strings.TrimSpace(cfg.DatabaseID) == "" {
			return clr(ansiDim, "auto"), nil
		}
		return cfg.DatabaseID, nil
	case "redis.url":
		return publicRedisURL(cfg), nil
	case "mount.backend":
		if strings.TrimSpace(cfg.MountBackend) == "" {
			return mountBackendNone, nil
		}
		return cfg.MountBackend, nil
	case "mount.path":
		if strings.TrimSpace(cfg.LocalPath) == "" {
			return clr(ansiDim, "unset"), nil
		}
		return cfg.LocalPath, nil
	case "mount.readonly":
		return strconv.FormatBool(cfg.ReadOnly), nil
	case "workspace.current":
		return currentWorkspaceLabel(selectedWorkspaceName(cfg)), nil
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
	case "controlPlane.database":
		cfg.DatabaseID = strings.TrimSpace(value)
	case "redis.url":
		if err := applyRedisURL(cfg, strings.TrimSpace(value)); err != nil {
			return err
		}
	case "mount.backend":
		cfg.MountBackend = strings.TrimSpace(value)
	case "mount.path":
		cfg.LocalPath = strings.TrimSpace(value)
	case "mount.readonly":
		b, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("mount.readonly must be true or false")
		}
		cfg.ReadOnly = b
	case "workspace.current":
		cfg.CurrentWorkspace = strings.TrimSpace(value)
		cfg.CurrentWorkspaceID = ""
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
	case "controlPlane.database":
		cfg.DatabaseID = ""
	case "redis.url":
		cfg.RedisAddr = def.RedisAddr
		cfg.RedisUsername = ""
		cfg.RedisPassword = ""
		cfg.RedisDB = def.RedisDB
		cfg.RedisTLS = false
	case "mount.backend":
		cfg.MountBackend = def.MountBackend
	case "mount.path":
		cfg.LocalPath = def.LocalPath
	case "mount.readonly":
		cfg.ReadOnly = false
	case "workspace.current":
		cfg.CurrentWorkspace = ""
		cfg.CurrentWorkspaceID = ""
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
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
