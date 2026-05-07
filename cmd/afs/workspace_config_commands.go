package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

type workspaceConfigCommandArgs struct {
	workspace string
	command   string
	rest      []string
	jsonOut   bool
}

func cmdWorkspaceConfig(args []string) error {
	if len(args) < 3 || isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, workspaceConfigUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	parsed, err := parseWorkspaceConfigArgs(args[2:])
	if err != nil {
		return fmt.Errorf("%s\n\n%s", err, workspaceConfigUsageText(filepath.Base(os.Args[0])))
	}

	cfg, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	selection, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, service, parsed.workspace)
	if err != nil {
		return err
	}

	switch parsed.command {
	case "get":
		return cmdWorkspaceConfigGet(service, selection, parsed)
	case "set":
		return cmdWorkspaceConfigSet(service, selection, parsed)
	case "unset":
		return cmdWorkspaceConfigUnset(service, selection, parsed)
	case "list":
		return cmdWorkspaceConfigList(service, selection, parsed)
	default:
		return fmt.Errorf("unknown workspace config subcommand %q\n\n%s", parsed.command, workspaceConfigUsageText(filepath.Base(os.Args[0])))
	}
}

func parseWorkspaceConfigArgs(args []string) (workspaceConfigCommandArgs, error) {
	var parsed workspaceConfigCommandArgs
	if len(args) < 2 {
		return parsed, fmt.Errorf("expected <workspace> <subcommand>")
	}
	parsed.workspace = strings.TrimSpace(args[0])
	parsed.command = strings.TrimSpace(args[1])
	if parsed.workspace == "" {
		return parsed, fmt.Errorf("workspace is required")
	}
	if parsed.command == "" {
		return parsed, fmt.Errorf("workspace config subcommand is required")
	}
	for _, arg := range args[2:] {
		switch arg {
		case "--json":
			parsed.jsonOut = true
		default:
			if strings.HasPrefix(arg, "--") {
				return parsed, fmt.Errorf("unknown flag %q", arg)
			}
			parsed.rest = append(parsed.rest, arg)
		}
	}
	return parsed, nil
}

func cmdWorkspaceConfigGet(service afsControlPlane, selection workspaceSelection, args workspaceConfigCommandArgs) error {
	if len(args.rest) != 1 {
		return fmt.Errorf("%s", workspaceConfigGetUsageText(filepath.Base(os.Args[0])))
	}
	cfg, err := service.GetWorkspaceConfig(context.Background(), selection.ID)
	if err != nil {
		return err
	}
	key := normalizeWorkspaceConfigKey(args.rest[0])
	value, err := getWorkspaceConfigValue(cfg, key)
	if err != nil {
		return err
	}
	if args.jsonOut {
		return encodeWorkspaceConfigJSON(workspaceConfigJSON{
			Workspace: selection.Name,
			ID:        selection.ID,
			Key:       key,
			Value:     value,
		})
	}
	printSection(clr(ansiBold, "workspace config"), []outputRow{
		{Label: "workspace", Value: selection.Name},
		{Label: "key", Value: key},
		{Label: "value", Value: displayWorkspaceConfigValue(value)},
	})
	return nil
}

func cmdWorkspaceConfigSet(service afsControlPlane, selection workspaceSelection, args workspaceConfigCommandArgs) error {
	if len(args.rest) != 2 {
		return fmt.Errorf("%s", workspaceConfigSetUsageText(filepath.Base(os.Args[0])))
	}
	current, err := service.GetWorkspaceConfig(context.Background(), selection.ID)
	if err != nil {
		return err
	}
	key := normalizeWorkspaceConfigKey(args.rest[0])
	next, err := setWorkspaceConfigValue(current, key, args.rest[1])
	if err != nil {
		return err
	}
	updated, err := service.UpdateWorkspaceConfig(context.Background(), selection.ID, next)
	if err != nil {
		return err
	}
	value, err := getWorkspaceConfigValue(updated, key)
	if err != nil {
		return err
	}
	if args.jsonOut {
		return encodeWorkspaceConfigJSON(workspaceConfigJSON{
			Workspace: selection.Name,
			ID:        selection.ID,
			Key:       key,
			Value:     value,
		})
	}
	printSection(markerSuccess+" "+clr(ansiBold, "workspace config updated"), []outputRow{
		{Label: "workspace", Value: selection.Name},
		{Label: "key", Value: key},
		{Label: "value", Value: displayWorkspaceConfigValue(value)},
	})
	return nil
}

func cmdWorkspaceConfigUnset(service afsControlPlane, selection workspaceSelection, args workspaceConfigCommandArgs) error {
	if len(args.rest) != 1 {
		return fmt.Errorf("%s", workspaceConfigUnsetUsageText(filepath.Base(os.Args[0])))
	}
	current, err := service.GetWorkspaceConfig(context.Background(), selection.ID)
	if err != nil {
		return err
	}
	key := normalizeWorkspaceConfigKey(args.rest[0])
	next, err := unsetWorkspaceConfigValue(current, key)
	if err != nil {
		return err
	}
	updated, err := service.UpdateWorkspaceConfig(context.Background(), selection.ID, next)
	if err != nil {
		return err
	}
	value, err := getWorkspaceConfigValue(updated, key)
	if err != nil {
		return err
	}
	if args.jsonOut {
		return encodeWorkspaceConfigJSON(workspaceConfigJSON{
			Workspace: selection.Name,
			ID:        selection.ID,
			Key:       key,
			Value:     value,
		})
	}
	printSection(markerSuccess+" "+clr(ansiBold, "workspace config updated"), []outputRow{
		{Label: "workspace", Value: selection.Name},
		{Label: "key", Value: key},
		{Label: "value", Value: displayWorkspaceConfigValue(value)},
	})
	return nil
}

func cmdWorkspaceConfigList(service afsControlPlane, selection workspaceSelection, args workspaceConfigCommandArgs) error {
	if len(args.rest) != 0 {
		return fmt.Errorf("%s", workspaceConfigListUsageText(filepath.Base(os.Args[0])))
	}
	cfg, err := service.GetWorkspaceConfig(context.Background(), selection.ID)
	if err != nil {
		return err
	}
	values := workspaceConfigValues(cfg)
	if args.jsonOut {
		return encodeWorkspaceConfigJSON(workspaceConfigListJSON{
			Workspace: selection.Name,
			ID:        selection.ID,
			Values:    values,
		})
	}
	rows := []outputRow{{Label: "workspace", Value: selection.Name}}
	for _, key := range workspaceConfigKeys() {
		rows = append(rows, outputRow{Label: key, Value: displayWorkspaceConfigValue(values[key])})
	}
	printSection(clr(ansiBold, "workspace config"), rows)
	return nil
}

type workspaceConfigJSON struct {
	Workspace string `json:"workspace"`
	ID        string `json:"id,omitempty"`
	Key       string `json:"key"`
	Value     any    `json:"value"`
}

type workspaceConfigListJSON struct {
	Workspace string         `json:"workspace"`
	ID        string         `json:"id,omitempty"`
	Values    map[string]any `json:"values"`
}

func encodeWorkspaceConfigJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func workspaceConfigKeys() []string {
	return []string{
		"versioning.mode",
		"versioning.includeGlobs",
		"versioning.excludeGlobs",
		"versioning.maxVersionsPerFile",
		"versioning.maxAgeDays",
		"versioning.maxTotalBytes",
		"versioning.largeFileCutoffBytes",
		"query.embeddings.enabled",
		"query.embeddings.model",
		"query.embeddings.chunkStrategy",
	}
}

func workspaceConfigValues(cfg controlplane.WorkspaceConfig) map[string]any {
	normalized := controlplane.NormalizeWorkspaceConfig(cfg)
	values := map[string]any{
		"versioning.mode":                 normalized.Versioning.Mode,
		"versioning.includeGlobs":         append([]string(nil), normalized.Versioning.IncludeGlobs...),
		"versioning.excludeGlobs":         append([]string(nil), normalized.Versioning.ExcludeGlobs...),
		"versioning.maxVersionsPerFile":   normalized.Versioning.MaxVersionsPerFile,
		"versioning.maxAgeDays":           normalized.Versioning.MaxAgeDays,
		"versioning.maxTotalBytes":        normalized.Versioning.MaxTotalBytes,
		"versioning.largeFileCutoffBytes": normalized.Versioning.LargeFileCutoffBytes,
		"query.embeddings.enabled":        normalized.Query.Embeddings.Enabled,
		"query.embeddings.model":          normalized.Query.Embeddings.Model,
		"query.embeddings.chunkStrategy":  normalized.Query.Embeddings.ChunkStrategy,
	}
	return values
}

func getWorkspaceConfigValue(cfg controlplane.WorkspaceConfig, key string) (any, error) {
	values := workspaceConfigValues(cfg)
	value, ok := values[normalizeWorkspaceConfigKey(key)]
	if !ok {
		return nil, fmt.Errorf("unknown workspace config key %q", key)
	}
	return value, nil
}

func setWorkspaceConfigValue(cfg controlplane.WorkspaceConfig, key, rawValue string) (controlplane.WorkspaceConfig, error) {
	next := cfg
	value := strings.TrimSpace(rawValue)
	switch normalizeWorkspaceConfigKey(key) {
	case "versioning.mode":
		next.Versioning.Mode = value
	case "versioning.includeGlobs":
		next.Versioning.IncludeGlobs = parseWorkspaceConfigStringList(value)
	case "versioning.excludeGlobs":
		next.Versioning.ExcludeGlobs = parseWorkspaceConfigStringList(value)
	case "versioning.maxVersionsPerFile":
		n, err := parseWorkspaceConfigNonNegativeInt(key, value)
		if err != nil {
			return controlplane.WorkspaceConfig{}, err
		}
		next.Versioning.MaxVersionsPerFile = n
	case "versioning.maxAgeDays":
		n, err := parseWorkspaceConfigNonNegativeInt(key, value)
		if err != nil {
			return controlplane.WorkspaceConfig{}, err
		}
		next.Versioning.MaxAgeDays = n
	case "versioning.maxTotalBytes":
		n, err := parseWorkspaceConfigNonNegativeInt64(key, value)
		if err != nil {
			return controlplane.WorkspaceConfig{}, err
		}
		next.Versioning.MaxTotalBytes = n
	case "versioning.largeFileCutoffBytes":
		n, err := parseWorkspaceConfigNonNegativeInt64(key, value)
		if err != nil {
			return controlplane.WorkspaceConfig{}, err
		}
		next.Versioning.LargeFileCutoffBytes = n
	case "query.embeddings.enabled":
		enabled, err := parseWorkspaceConfigBool(key, value)
		if err != nil {
			return controlplane.WorkspaceConfig{}, err
		}
		next.Query.Embeddings.Enabled = enabled
	case "query.embeddings.model":
		next.Query.Embeddings.Model = value
	case "query.embeddings.chunkStrategy":
		next.Query.Embeddings.ChunkStrategy = value
	default:
		return controlplane.WorkspaceConfig{}, fmt.Errorf("unknown workspace config key %q", key)
	}
	next = controlplane.NormalizeWorkspaceConfig(next)
	if err := controlplane.ValidateWorkspaceConfig(next); err != nil {
		return controlplane.WorkspaceConfig{}, err
	}
	return next, nil
}

func unsetWorkspaceConfigValue(cfg controlplane.WorkspaceConfig, key string) (controlplane.WorkspaceConfig, error) {
	next := cfg
	switch normalizeWorkspaceConfigKey(key) {
	case "versioning.mode":
		next.Versioning.Mode = controlplane.WorkspaceVersioningModeOff
	case "versioning.includeGlobs":
		next.Versioning.IncludeGlobs = nil
	case "versioning.excludeGlobs":
		next.Versioning.ExcludeGlobs = nil
	case "versioning.maxVersionsPerFile":
		next.Versioning.MaxVersionsPerFile = 0
	case "versioning.maxAgeDays":
		next.Versioning.MaxAgeDays = 0
	case "versioning.maxTotalBytes":
		next.Versioning.MaxTotalBytes = 0
	case "versioning.largeFileCutoffBytes":
		next.Versioning.LargeFileCutoffBytes = 0
	case "query.embeddings.enabled":
		next.Query.Embeddings.Enabled = false
	case "query.embeddings.model":
		next.Query.Embeddings.Model = ""
	case "query.embeddings.chunkStrategy":
		next.Query.Embeddings.ChunkStrategy = ""
	default:
		return controlplane.WorkspaceConfig{}, fmt.Errorf("unknown workspace config key %q", key)
	}
	next = controlplane.NormalizeWorkspaceConfig(next)
	if err := controlplane.ValidateWorkspaceConfig(next); err != nil {
		return controlplane.WorkspaceConfig{}, err
	}
	return next, nil
}

func normalizeWorkspaceConfigKey(key string) string {
	key = strings.TrimSpace(key)
	compact := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
	switch compact {
	case "versioning.mode", "versioningmode":
		return "versioning.mode"
	case "versioning.include", "versioning.includes", "versioning.includeglob", "versioning.includeglobs":
		return "versioning.includeGlobs"
	case "versioning.exclude", "versioning.excludes", "versioning.excludeglob", "versioning.excludeglobs":
		return "versioning.excludeGlobs"
	case "versioning.maxversionsperfile", "versioning.maxversions", "versioning.maxversioncount":
		return "versioning.maxVersionsPerFile"
	case "versioning.maxagedays", "versioning.maxage":
		return "versioning.maxAgeDays"
	case "versioning.maxtotalbytes", "versioning.maxbytes":
		return "versioning.maxTotalBytes"
	case "versioning.largefilecutoffbytes", "versioning.largefilecutoff":
		return "versioning.largeFileCutoffBytes"
	case "query.embeddings.enabled", "query.embeddingsenabled", "query.embedding.enabled", "query.embeddingenabled":
		return "query.embeddings.enabled"
	case "query.embeddings.model", "query.embeddingsmodel", "query.embedding.model", "query.embeddingmodel":
		return "query.embeddings.model"
	case "query.embeddings.chunkstrategy", "query.embeddingschunkstrategy", "query.embedding.chunkstrategy", "query.embeddingchunkstrategy":
		return "query.embeddings.chunkStrategy"
	default:
		return strings.TrimSpace(key)
	}
}

func parseWorkspaceConfigStringList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "none") || strings.EqualFold(value, "unset") {
		return nil
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n'
	})
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		item := strings.TrimSpace(field)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func parseWorkspaceConfigNonNegativeInt(key, value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", normalizeWorkspaceConfigKey(key))
	}
	return n, nil
}

func parseWorkspaceConfigNonNegativeInt64(key, value string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", normalizeWorkspaceConfigKey(key))
	}
	return n, nil
}

func parseWorkspaceConfigBool(key, value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on", "enabled":
		return true, nil
	case "false", "0", "no", "off", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", normalizeWorkspaceConfigKey(key))
	}
}

func displayWorkspaceConfigValue(value any) string {
	switch v := value.(type) {
	case []string:
		if len(v) == 0 {
			return "none"
		}
		return strings.Join(v, ", ")
	case string:
		if strings.TrimSpace(v) == "" {
			return "unset"
		}
		return v
	case int:
		if v == 0 {
			return "unset"
		}
		return strconv.Itoa(v)
	case int64:
		if v == 0 {
			return "unset"
		}
		return strconv.FormatInt(v, 10)
	case bool:
		return strconv.FormatBool(v)
	default:
		return fmt.Sprint(v)
	}
}

func workspaceConfigUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws config <workspace> <subcommand>

Subcommands:
  get <key> [--json]              Read a workspace config value
  set <key> <value> [--json]      Update a workspace config value
  unset <key> [--json]            Reset a workspace config value
  list [--json]                   List workspace config values

Common keys:
  versioning.mode
  versioning.includeGlobs
  versioning.excludeGlobs
  versioning.maxVersionsPerFile
  versioning.maxAgeDays
  versioning.maxTotalBytes
  versioning.largeFileCutoffBytes
  query.embeddings.enabled
  query.embeddings.model
  query.embeddings.chunkStrategy

Examples:
  %s ws config repo get versioning.mode
  %s ws config repo set versioning.mode all
  %s ws config repo set versioning.includeGlobs 'src/**,docs/**'
  %s ws config repo set query.embeddings.enabled true
  %s ws config repo unset versioning.maxAgeDays --json
`, bin, bin, bin, bin, bin, bin)
}

func workspaceConfigGetUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws config <workspace> get <key> [--json]
`, bin)
}

func workspaceConfigSetUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws config <workspace> set <key> <value> [--json]
`, bin)
}

func workspaceConfigUnsetUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws config <workspace> unset <key> [--json]
`, bin)
}

func workspaceConfigListUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s ws config <workspace> list [--json]
`, bin)
}
