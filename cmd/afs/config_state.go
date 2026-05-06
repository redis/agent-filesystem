package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var cfgPathOverride string

// configPath resolves the config file beside the executable unless the test
// suite or caller provided an override.
func configPath() string {
	if cfgPathOverride != "" {
		return cfgPathOverride
	}
	return defaultConfigPath()
}

func defaultConfigPath() string {
	exe, err := executablePath()
	if err != nil {
		return "afs.config.json"
	}
	return filepath.Join(filepath.Dir(exe), "afs.config.json")
}

func compactDisplayPath(p string) string {
	if strings.TrimSpace(p) == "" {
		return p
	}

	clean := filepath.Clean(p)
	base := filepath.Base(clean)
	dirBase := filepath.Base(filepath.Dir(clean))
	if base == "." || base == string(filepath.Separator) {
		return clean
	}
	if dirBase == "." || dirBase == string(filepath.Separator) || dirBase == "" {
		return base
	}
	return filepath.Join(dirBase, base)
}

func saveConfig(cfg config) error {
	b, err := json.MarshalIndent(persistedConfigFromRuntime(cfg), "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(configPath(), b, 0o644)
}

type persistedConfig struct {
	Redis        redisConfig          `json:"redis"`
	ControlPlane controlPlaneSettings `json:"controlPlane,omitempty"`
	Agent        agentSettings        `json:"agent,omitempty"`
	Workspace    workspaceSettings    `json:"workspace,omitempty"`
	ProductMode  string               `json:"productMode,omitempty"`
	Mode         string               `json:"mode,omitempty"`
	Sync         *syncSettings        `json:"sync,omitempty"`
	Runtime      *persistedRuntime    `json:"runtime,omitempty"`
}

type persistedRuntime struct {
	Mount mountSettings `json:"mount,omitempty"`
	Logs  logSettings   `json:"logs,omitempty"`
}

func persistedConfigFromRuntime(cfg config) persistedConfig {
	productMode, err := effectiveProductMode(cfg)
	if err != nil {
		productMode = strings.TrimSpace(cfg.ProductMode)
	}
	out := persistedConfig{
		Redis: redisConfig{
			RedisAddr:     strings.TrimSpace(cfg.RedisAddr),
			RedisUsername: strings.TrimSpace(cfg.RedisUsername),
			RedisPassword: cfg.RedisPassword,
			RedisDB:       cfg.RedisDB,
			RedisTLS:      cfg.RedisTLS,
		},
		ProductMode: productMode,
		Mode:        persistedMode(cfg),
		Agent: agentSettings{
			ID:   strings.TrimSpace(cfg.ID),
			Name: strings.TrimSpace(cfg.Name),
		},
	}
	if strings.TrimSpace(cfg.CurrentWorkspace) != "" || strings.TrimSpace(cfg.CurrentWorkspaceID) != "" {
		out.Workspace = workspaceSettings{
			DefaultWorkspace:   strings.TrimSpace(cfg.CurrentWorkspace),
			DefaultWorkspaceID: strings.TrimSpace(cfg.CurrentWorkspaceID),
		}
	}
	if productMode != productModeLocal {
		out.ControlPlane = controlPlaneSettings{
			URL:        strings.TrimSpace(cfg.URL),
			DatabaseID: strings.TrimSpace(cfg.DatabaseID),
			AuthToken:  strings.TrimSpace(cfg.AuthToken),
			Account:    strings.TrimSpace(cfg.Account),
		}
	}
	if cfg.SyncFileSizeCapMB > 0 && cfg.SyncFileSizeCapMB != defaultSyncFileSizeCapMB {
		out.Sync = &syncSettings{SyncFileSizeCapMB: cfg.SyncFileSizeCapMB}
	}
	out.Runtime = &persistedRuntime{
		Mount: persistedMountSettings(cfg.mountSettings),
		Logs:  cfg.logSettings,
	}
	return out
}

func persistedMountSettings(settings mountSettings) mountSettings {
	settings.ReadOnly = false
	return settings
}

func persistedMode(cfg config) string {
	mode, err := effectiveMode(cfg)
	if err != nil {
		return strings.TrimSpace(cfg.Mode)
	}
	if mode == modeSync {
		return ""
	}
	return mode
}

func loadConfig() (config, error) {
	cfg := defaultConfig()
	b, err := os.ReadFile(configPath())
	if err != nil {
		return cfg, err
	}
	var raw persistedConfig
	if err := json.Unmarshal(b, &raw); err != nil {
		return cfg, err
	}
	var legacy config
	if err := json.Unmarshal(b, &legacy); err != nil {
		return cfg, err
	}
	cfg.redisConfig = raw.Redis
	cfg.controlPlaneSettings = raw.ControlPlane
	cfg.agentSettings = raw.Agent
	cfg.ProductMode = raw.ProductMode
	cfg.Mode = raw.Mode
	if raw.Sync != nil {
		cfg.syncSettings = *raw.Sync
	} else if legacy.SyncFileSizeCapMB != 0 {
		cfg.syncSettings = legacy.syncSettings
	}
	cfg.CurrentWorkspace = strings.TrimSpace(raw.Workspace.DefaultWorkspace)
	cfg.CurrentWorkspaceID = strings.TrimSpace(raw.Workspace.DefaultWorkspaceID)

	if raw.Runtime != nil {
		cfg.LocalPath = ""
		cfg.mountSettings = persistedMountSettings(raw.Runtime.Mount)
		cfg.logSettings = raw.Runtime.Logs
	} else {
		cfg.LocalPath = legacy.LocalPath
		cfg.mountSettings = persistedMountSettings(legacy.mountSettings)
		cfg.logSettings = legacy.logSettings
	}
	cfg.WorkRoot = defaultWorkRoot()
	return cfg, nil
}

func defaultConfig() config {
	return config{
		redisConfig: redisConfig{
			RedisAddr: "localhost:6379",
			RedisDB:   0,
		},
		ProductMode: productModeLocal,
		Mode:        modeSync,
		LocalPath:   "~/afs",
		mountSettings: mountSettings{
			MountBackend: mountBackendNone,
			NFSHost:      "127.0.0.1",
			NFSPort:      20490,
		},
		logSettings: logSettings{
			MountLog: "/tmp/afs-mount.log",
			SyncLog:  "/tmp/afs-sync.log",
		},
		syncSettings: syncSettings{
			SyncFileSizeCapMB: defaultSyncFileSizeCapMB,
		},
		WorkRoot: defaultWorkRoot(),
	}
}

func newAgentID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "agt_" + hex.EncodeToString(raw[:]), nil
}

func defaultAgentName() string {
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(hostname)
}

func ensureAgentIdentity(cfg *config) (bool, error) {
	if cfg == nil {
		return false, nil
	}
	changed := false
	if strings.TrimSpace(cfg.ID) != "" {
		cfg.ID = strings.TrimSpace(cfg.ID)
	} else {
		agentID, err := newAgentID()
		if err != nil {
			return false, err
		}
		cfg.ID = agentID
		changed = true
	}
	if strings.TrimSpace(cfg.Name) != "" {
		cfg.Name = strings.TrimSpace(cfg.Name)
	} else if name := defaultAgentName(); name != "" {
		cfg.Name = name
		changed = true
	}
	return changed, nil
}

func loadConfigOrDefault() config {
	cfg, err := loadConfig()
	if err == nil {
		return cfg
	}
	return defaultConfig()
}

func prepareConfigForSave(cfg *config) error {
	def := defaultConfig()
	productMode, err := effectiveProductMode(*cfg)
	if err != nil {
		return err
	}
	cfg.ProductMode = productMode

	if productMode == productModeLocal && strings.TrimSpace(cfg.RedisAddr) == "" {
		cfg.RedisAddr = def.RedisAddr
	}
	if cfg.RedisDB < 0 {
		return fmt.Errorf("redis db must be >= 0")
	}
	if productMode != productModeLocal {
		controlPlaneURL, err := normalizeControlPlaneURL(cfg.URL)
		if err != nil {
			return err
		}
		cfg.URL = controlPlaneURL
		cfg.DatabaseID = strings.TrimSpace(cfg.DatabaseID)
		cfg.AuthToken = strings.TrimSpace(cfg.AuthToken)
	}
	if productMode != productModeCloud {
		cfg.AuthToken = ""
	}
	cfg.ID = strings.TrimSpace(cfg.ID)
	cfg.Name = strings.TrimSpace(cfg.Name)

	if cfg.LocalPath != "" {
		mp, err := expandPath(cfg.LocalPath)
		if err != nil {
			return err
		}
		cfg.LocalPath = mp
	}

	cfg.WorkRoot = defaultWorkRoot()
	if strings.TrimSpace(cfg.MountLog) == "" {
		cfg.MountLog = def.MountLog
	}
	if strings.TrimSpace(cfg.SyncLog) == "" {
		cfg.SyncLog = def.SyncLog
	}

	backendName, err := normalizeMountBackend(cfg.MountBackend)
	if err != nil {
		return err
	}
	cfg.MountBackend = backendName

	if backendName != mountBackendNone {
		if strings.TrimSpace(cfg.LocalPath) == "" {
			mp, err := expandPath(def.LocalPath)
			if err != nil {
				return err
			}
			cfg.LocalPath = mp
		}
		if backendName == mountBackendNFS {
			if cfg.NFSHost == "" {
				cfg.NFSHost = "127.0.0.1"
			}
			if cfg.NFSPort <= 0 {
				cfg.NFSPort = 20490
			}
		}
	}

	if strings.TrimSpace(cfg.CurrentWorkspace) != "" {
		if err := validateAFSName("workspace", strings.TrimSpace(cfg.CurrentWorkspace)); err != nil {
			return err
		}
		cfg.CurrentWorkspace = strings.TrimSpace(cfg.CurrentWorkspace)
	}
	cfg.CurrentWorkspaceID = strings.TrimSpace(cfg.CurrentWorkspaceID)
	if productMode == productModeLocal {
		cfg.CurrentWorkspaceID = ""
	}

	mode, err := effectiveMode(*cfg)
	if err != nil {
		return err
	}
	cfg.Mode = mode

	if cfg.SyncFileSizeCapMB < 0 {
		return fmt.Errorf("sync.fileSizeCapMB must be >= 0")
	}

	if productMode == productModeLocal {
		if _, _, err := splitAddr(cfg.RedisAddr); err != nil {
			return err
		}
	}
	return nil
}

func validateConfiguredMountpoint(cfg config) error {
	backendName, err := normalizeMountBackend(cfg.MountBackend)
	if err != nil {
		return err
	}
	if backendName == mountBackendNone || strings.TrimSpace(cfg.LocalPath) == "" {
		return nil
	}
	return validateMountpointPath(cfg.LocalPath)
}

func validateMountpointPath(mountpoint string) error {
	if strings.TrimSpace(mountpoint) == "" {
		return nil
	}

	clean := filepath.Clean(mountpoint)
	info, err := os.Stat(clean)
	switch {
	case err == nil:
		if !info.IsDir() {
			return fmt.Errorf("mountpoint %s exists and is not a directory; choose an existing directory or a new path that AFS can create", clean)
		}
		return nil
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("check mountpoint %s: %w", clean, err)
	}

	parent, err := nearestExistingMountParent(clean)
	if err != nil {
		return err
	}
	probeDir, err := os.MkdirTemp(parent, ".afs-mountpoint-check-*")
	if err != nil {
		return fmt.Errorf("mountpoint %s cannot be created as a directory: %w", clean, err)
	}
	if err := os.Remove(probeDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove mountpoint probe %s: %w", probeDir, err)
	}
	return nil
}

func nearestExistingMountParent(mountpoint string) (string, error) {
	current := filepath.Clean(mountpoint)
	for {
		info, err := os.Stat(current)
		switch {
		case err == nil:
			if !info.IsDir() {
				if current == mountpoint {
					return "", fmt.Errorf("mountpoint %s exists and is not a directory; choose an existing directory or a new path that AFS can create", mountpoint)
				}
				return "", fmt.Errorf("mountpoint %s cannot be created because %s exists and is not a directory", mountpoint, current)
			}
			return current, nil
		case !errors.Is(err, os.ErrNotExist):
			return "", fmt.Errorf("check mountpoint %s: %w", current, err)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("mountpoint %s cannot be created because no parent directory exists", mountpoint)
		}
		current = parent
	}
}

func resolveConfigPaths(cfg *config) error {
	dir := exeDir()
	if err := prepareConfigForSave(cfg); err != nil {
		return err
	}
	if err := validateConfiguredMountpoint(*cfg); err != nil {
		return err
	}

	if cfg.MountBackend == mountBackendNone {
		return nil
	}

	switch cfg.MountBackend {
	case mountBackendFuse:
		if cfg.MountBin == "" {
			defMountBin := filepath.Join(dir, "mount", "agent-filesystem-mount")
			if _, err := os.Stat(defMountBin); err != nil {
				defMountBin = "agent-filesystem-mount"
			}
			resolved, err := resolveBinary(defMountBin)
			if err != nil {
				return fmt.Errorf("cannot find agent-filesystem-mount binary\n  Build it with: make mount")
			}
			cfg.MountBin = resolved
		}
	case mountBackendNFS:
		if cfg.NFSHost == "" {
			cfg.NFSHost = "127.0.0.1"
		}
		if cfg.NFSPort <= 0 {
			cfg.NFSPort = 20490
		}
		if cfg.NFSBin == "" {
			defNFSBin := filepath.Join(dir, "mount", "agent-filesystem-nfs")
			if _, err := os.Stat(defNFSBin); err != nil {
				defNFSBin = "agent-filesystem-nfs"
			}
			resolved, err := resolveBinary(defNFSBin)
			if err != nil {
				return fmt.Errorf("cannot find agent-filesystem-nfs binary\n  Build it with: make mount")
			}
			cfg.NFSBin = resolved
		}
	}

	return nil
}

func stateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".afs")
}

func defaultWorkRoot() string {
	return filepath.Join(stateDir(), "workspaces")
}

func defaultStatePath() string {
	return filepath.Join(stateDir(), "state.json")
}

func statePathForConfig(configFile string) string {
	cleanConfig := cleanConfigPath(configFile)
	if cleanConfig == "" || cleanConfig == cleanConfigPath(defaultConfigPath()) {
		return defaultStatePath()
	}
	sum := sha256.Sum256([]byte(cleanConfig))
	return filepath.Join(stateDir(), "configs", hex.EncodeToString(sum[:8])+".json")
}

func statePath() string {
	return statePathForConfig(configPath())
}

func saveState(st state) error {
	target := statePath()
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(target, b, 0o600)
}

func loadState() (state, error) {
	if st, err := loadStateFromPath(statePath()); err == nil {
		return st, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return state{}, err
	}
	if sameConfigPath(statePath(), defaultStatePath()) {
		return state{}, os.ErrNotExist
	}
	return loadStateFromPath(defaultStatePath())
}

func loadStateFromPath(path string) (state, error) {
	var st state
	b, err := os.ReadFile(path)
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return st, err
	}
	return st, nil
}
