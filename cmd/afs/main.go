package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/redis/agent-filesystem/internal/worktree"
	"github.com/redis/go-redis/v9"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type config struct {
	UseExistingRedis bool   `json:"useExistingRedis"`
	RedisAddr        string `json:"redisAddr"`
	RedisUsername    string `json:"redisUsername"`
	RedisPassword    string `json:"redisPassword"`
	RedisDB          int    `json:"redisDB"`
	RedisTLS         bool   `json:"redisTLS"`
	WorkRoot         string `json:"workRoot"`
	CurrentWorkspace string `json:"currentWorkspace"`
	RedisKey         string `json:"redisKey"`
	LocalPath        string `json:"localPath,omitempty"`
	MountBackend     string `json:"mountBackend"`
	ReadOnly         bool   `json:"readOnly"`
	AllowOther       bool   `json:"allowOther"`
	RedisServerBin   string `json:"redisServerBin"`
	ModulePath       string `json:"modulePath"`
	MountBin         string `json:"mountBin"`
	NFSBin           string `json:"nfsBin"`
	NFSHost          string `json:"nfsHost"`
	NFSPort          int    `json:"nfsPort"`
	RedisLog         string `json:"redisLog"`
	MountLog         string `json:"mountLog"`

	// Sync daemon (Dropbox-style local-first sync).
	// Mode is one of "" (legacy: same as "mount"), "sync", "mount", or "none".
	// When sync, `afs up` starts the background sync daemon instead of mounting
	// the live workspace as FUSE/NFS. Existing configs without this field keep
	// their previous mount-based behavior.
	Mode              string `json:"mode,omitempty"`
	SyncFileSizeCapMB int    `json:"syncFileSizeCapMB,omitempty"`
	SyncLog           string `json:"syncLog,omitempty"`

	// Derived at runtime, not persisted.
	redisHost string
	redisPort int

	// Legacy field migration — decoded from old JSON keys but never written.
	legacyMountpoint    string `json:"mountpoint,omitempty"`
	legacySyncLocalPath string `json:"syncLocalPath,omitempty"`
}

type state struct {
	StartedAt            time.Time `json:"started_at"`
	ManageRedis          bool      `json:"manage_redis"`
	RedisPID             int       `json:"redis_pid"`
	RedisAddr            string    `json:"redis_addr"`
	RedisDB              int       `json:"redis_db"`
	CurrentWorkspace     string    `json:"current_workspace,omitempty"`
	MountedHeadSavepoint string    `json:"mounted_head_savepoint,omitempty"`
	MountPID             int       `json:"mount_pid"`
	MountBackend         string    `json:"mount_backend"`
	ReadOnly             bool      `json:"read_only"`
	MountEndpoint        string    `json:"mount_endpoint,omitempty"`
	LocalPath            string    `json:"local_path,omitempty"`
	CreatedLocalPath     bool      `json:"created_local_path,omitempty"`
	RedisKey             string    `json:"redis_key"`
	RedisLog             string    `json:"redis_log"`
	MountLog             string    `json:"mount_log"`
	RedisServerBin       string    `json:"redis_server_bin"`
	MountBin             string    `json:"mount_bin"`
	ArchivePath          string    `json:"archive_path,omitempty"`

	// Sync daemon mode fields. Populated when `afs up` ran with mode=sync.
	Mode    string `json:"mode,omitempty"`
	SyncPID int    `json:"sync_pid,omitempty"`
	SyncLog string `json:"sync_log,omitempty"`

	// Legacy field migration — decoded from old JSON keys but never written.
	legacyMountpoint        string `json:"mountpoint,omitempty"`
	legacySyncLocalPath     string `json:"sync_local_path,omitempty"`
	legacySyncMode          string `json:"sync_mode,omitempty"`
	legacyCreatedMountpoint bool   `json:"created_mountpoint,omitempty"`
}

type importClient interface {
	Mkdir(ctx context.Context, path string) error
	Echo(ctx context.Context, path string, data []byte) error
	Ln(ctx context.Context, target, linkpath string) error
	Chmod(ctx context.Context, path string, mode uint32) error
	Chown(ctx context.Context, path string, uid, gid uint32) error
	Utimens(ctx context.Context, path string, atimeMs, mtimeMs int64) error
}

type importStats = worktree.ImportStats

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

var cfgPathOverride string
var errMigrationCancelled = errors.New("migration cancelled")
var mainSigCh chan os.Signal // disabled by interactive sync mode so it can handle SIGINT itself

func main() {
	defer showCursor()

	mainSigCh = make(chan os.Signal, 1)
	signal.Notify(mainSigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-mainSigCh
		showCursor()
		fmt.Println()
		os.Exit(130)
	}()

	args := os.Args[1:]
	if len(args) >= 2 && args[0] == "--config" {
		cfgPathOverride = args[1]
		args = args[2:]
	}

	if len(args) < 1 {
		printUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "setup", "se":
		if err := cmdSetup(); err != nil {
			fatal(err)
		}
	case "config", "co":
		if err := cmdConfig(args); err != nil {
			fatal(err)
		}
	case "up", "u":
		if err := cmdUpArgs(args[1:]); err != nil {
			fatal(err)
		}
	case "down", "d":
		if err := cmdDown(); err != nil {
			fatal(err)
		}
	case "status", "st", "s":
		if err := cmdStatus(); err != nil {
			fatal(err)
		}
	case "grep", "g":
		if err := cmdGrep(args); err != nil {
			fatal(err)
		}
	case "mcp", "m":
		if err := cmdMCP(args); err != nil {
			fatal(err)
		}
	case "workspace", "w":
		if err := cmdWorkspace(args); err != nil {
			fatal(err)
		}
	case "checkpoint", "c", "ch":
		if err := cmdCheckpoint(args); err != nil {
			fatal(err)
		}
	case "_sync-daemon":
		// Hidden: re-exec'd by `afs up` in sync mode. Runs the steady-state
		// sync loop until SIGTERM.
		if err := runSyncDaemon(); err != nil {
			fatal(err)
		}
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	printBannerCompact()
	bin := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, `Usage:
  %s [--config <path>] <command>

Commands:
  setup                Interactive setup
  config ...           Show or change basic Redis and mount settings non-interactively
  up [flags]           Start AFS services with optional one-shot overrides
  down                 Stop and unmount
  status               Show current status
  grep [flags] <pat>   Search a workspace directly in Redis
  mcp                  Start the workspace-first MCP server over stdio
  workspace ...        Workspace operations (create, list, current, use, run, clone, delete, import)
  checkpoint ...       Checkpoint operations (create, list, restore)

Config: %s
`, bin, compactDisplayPath(configPath()))
}

func userModeLabel(backendName string) string {
	switch backendName {
	case mountBackendNone:
		return "None"
	case mountBackendFuse:
		return "FUSE"
	case mountBackendNFS:
		return "NFS"
	default:
		return strings.ToUpper(backendName)
	}
}

func redisDatabaseLabel(addr string, db int, tls bool) string {
	scheme := "redis"
	if tls {
		scheme = "rediss"
	}
	return fmt.Sprintf("%s://%s/%d", scheme, addr, db)
}

func statusRemoteLabel(addr string, db int) string {
	return redisDatabaseLabel(addr, db, false)
}

func configRemoteLabel(cfg config) string {
	return redisDatabaseLabel(cfg.RedisAddr, cfg.RedisDB, cfg.RedisTLS)
}

func configPathLabel() string {
	return clr(ansiDim, compactDisplayPath(configPath()))
}

func commandContextRows(cfg config, workspace string) []boxRow {
	rows := make([]boxRow, 0, 2)
	if strings.TrimSpace(workspace) != "" {
		rows = append(rows, boxRow{Label: "workspace", Value: workspace})
	}
	rows = append(rows, boxRow{Label: "database", Value: configRemoteLabel(cfg)})
	return rows
}

func statusTitle(prefix, workspace, localPath string, mounted bool) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return prefix + " " + clr(ansiBold, "AFS no workspace selected")
	}
	if mounted {
		return prefix + " " + clr(ansiBold, fmt.Sprintf("AFS workspace %s mounted at %s", workspace, localPath))
	}
	return prefix + " " + clr(ansiBold, fmt.Sprintf("AFS workspace %s not mounted", workspace))
}

func localSurfacePath(cfg config) string {
	return cfg.LocalPath
}

func statusRows(backendName, redisAddr string, redisDB int) []boxRow {
	mountValue := "none"
	if backendName != mountBackendNone {
		mountValue = userModeLabel(backendName)
	}
	return []boxRow{
		{Label: "database", Value: statusRemoteLabel(redisAddr, redisDB)},
		{Label: "mount backend", Value: mountValue},
		{Label: "config", Value: configPathLabel()},
	}
}

func currentWorkspaceLabel(workspace string) string {
	if strings.TrimSpace(workspace) == "" {
		return "none"
	}
	return workspace
}

// ---------------------------------------------------------------------------
// setup — interactive wizard → save config → start
// ---------------------------------------------------------------------------

func cmdSetup() error {
	if st, err := loadState(); err == nil {
		if (st.MountPID > 0 && processAlive(st.MountPID)) || (st.ManageRedis && st.RedisPID > 0 && processAlive(st.RedisPID)) {
			return fmt.Errorf("afs is currently running\nRun '%s down' first", filepath.Base(os.Args[0]))
		}
	}

	printBanner()

	fmt.Println("  " + clr(ansiDim, "AFS stores workspace state in Redis and can optionally expose"))
	fmt.Println("  " + clr(ansiDim, "a mounted filesystem for tools that need one."))
	fmt.Println()
	cfg := defaultConfig()
	firstRun := true
	if loaded, err := loadConfig(); err == nil {
		cfg = loaded
		firstRun = false
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if firstRun {
		fmt.Println("  " + clr(ansiBold, "Let's get you set up."))
	} else {
		fmt.Println("  " + clr(ansiBold, "Let's update your configuration."))
	}
	fmt.Println()

	r := bufio.NewReader(os.Stdin)
	cfg, err := runSetupWizard(r, os.Stdout, cfg, firstRun)
	if err != nil {
		return err
	}

	if err := resolveConfigPaths(&cfg); err != nil {
		return err
	}

	if err := saveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("  %s Saved to %s\n", clr(ansiDim, "▸"), clr(ansiBold, compactDisplayPath(configPath())))
	fmt.Printf("  %s Run %s to start AFS\n\n", clr(ansiDim, "▸"), clr(ansiOrange, filepath.Base(os.Args[0])+" up"))
	return nil
}

// runSetupWizard runs the interactive setup flow. On first run it walks the
// user through Redis + filesystem configuration in order; on subsequent runs
// it shows a menu that loops until the user picks "Done", so they can edit
// Redis connection, filesystem mount, and current workspace in any order
// without being dropped back to the shell after a single choice.
func runSetupWizard(r *bufio.Reader, out io.Writer, cfg config, firstRun bool) (config, error) {
	if firstRun {
		return runFullSetupWizard(r, out, cfg)
	}
	return runEditSetupWizard(r, out, cfg)
}

func runEditSetupWizard(r *bufio.Reader, out io.Writer, cfg config) (config, error) {
	for {
		fmt.Fprintln(out, "  "+clr(ansiBold+ansiCyan, "▸")+" "+clr(ansiBold, "Setup"))
		fmt.Fprintln(out)
		fmt.Fprintln(out, "  What would you like to change?")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "    "+clr(ansiCyan, "1")+"  Change mode "+clr(ansiDim, "("+setupModeLabel(cfg)+")"))
		fmt.Fprintln(out, "    "+clr(ansiCyan, "2")+"  Change Redis connection "+clr(ansiDim, "("+setupRedisConnectionLabel(cfg)+")"))
		fmt.Fprintln(out, "    "+clr(ansiCyan, "3")+"  Change "+setupSurfaceMenuLabel(cfg)+" "+clr(ansiDim, "("+setupLocalSurfaceLabel(cfg)+")"))
		fmt.Fprintln(out, "    "+clr(ansiCyan, "4")+"  Change current workspace "+clr(ansiDim, "("+currentWorkspaceLabel(cfg.CurrentWorkspace)+")"))
		fmt.Fprintln(out, "    "+clr(ansiCyan, "5")+"  Save and exit")
		fmt.Fprintln(out)

		choice, err := promptString(r, out, "  Choose", "5")
		if err != nil {
			return cfg, err
		}
		fmt.Fprintln(out)

		switch strings.TrimSpace(choice) {
		case "1":
			if err := promptModeSetup(r, out, &cfg); err != nil {
				return cfg, err
			}
		case "2":
			if err := promptRedisConnectionSetup(r, out, &cfg); err != nil {
				return cfg, err
			}
		case "3":
			mode, err := effectiveMode(cfg)
			if err != nil {
				return cfg, err
			}
			if mode == modeSync {
				if err := promptSyncLocalPathSetup(r, out, &cfg); err != nil {
					return cfg, err
				}
			} else {
				if err := promptLocalFilesystemSetup(r, out, &cfg, false); err != nil {
					return cfg, err
				}
			}
		case "4":
			if err := promptCurrentWorkspaceSetup(r, out, &cfg); err != nil {
				return cfg, err
			}
		case "5", "":
			return cfg, nil
		default:
			fmt.Fprintln(out, "  "+clr(ansiYellow, "Unknown choice ")+clr(ansiBold, choice)+clr(ansiDim, "; pick 1, 2, 3, 4, or 5."))
			fmt.Fprintln(out)
		}
	}
}

func runFullSetupWizard(r *bufio.Reader, out io.Writer, cfg config) (config, error) {
	// First-run default is sync, so a user who just blows through with Enter
	// ends up on the recommended path.
	if strings.TrimSpace(cfg.Mode) == "" {
		cfg.Mode = modeSync
	}
	if err := promptRedisConnectionSetup(r, out, &cfg); err != nil {
		return cfg, err
	}
	if err := promptModeSetup(r, out, &cfg); err != nil {
		return cfg, err
	}
	mode, err := effectiveMode(cfg)
	if err != nil {
		return cfg, err
	}
	if mode == modeSync {
		if err := promptSyncLocalPathSetup(r, out, &cfg); err != nil {
			return cfg, err
		}
	} else {
		if err := promptLocalFilesystemSetup(r, out, &cfg, true); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func setupRedisConnectionLabel(cfg config) string {
	if cfg.UseExistingRedis {
		label := cfg.RedisAddr
		if cfg.RedisTLS {
			label += ", tls"
		}
		if strings.TrimSpace(label) == "" {
			return "existing Redis"
		}
		return label
	}
	return "managed local Redis"
}

func setupLocalModeLabel(cfg config) string {
	backend, err := normalizeMountBackend(cfg.MountBackend)
	if err != nil {
		backend = mountBackendNone
	}
	if backend == mountBackendNone {
		return "none"
	}
	label := strings.ToUpper(backend)
	if backend == mountBackendFuse {
		label = "FUSE"
	}
	if backend == mountBackendNFS {
		label = "NFS"
	}
	if strings.TrimSpace(cfg.LocalPath) != "" {
		return label + " at " + cfg.LocalPath
	}
	return label
}

// setupModeLabel is the right-aligned hint shown next to the "Change mode"
// menu item. We render sync as the default and call it out as recommended
// when the config is still on the legacy empty/mount setting.
func setupModeLabel(cfg config) string {
	resolved, err := effectiveMode(cfg)
	if err != nil {
		return "unknown"
	}
	switch resolved {
	case modeSync:
		return "sync (recommended)"
	case modeMount:
		return "live mount"
	case modeNone:
		return "none"
	}
	return resolved
}

// setupSurfaceMenuLabel names the menu item for the "local surface" choice
// depending on mode. In sync mode it's the local sync path; in mount mode
// it's the FUSE/NFS mountpoint. Avoids a schizophrenic "Change filesystem
// mount" label when the user is in sync mode.
func setupSurfaceMenuLabel(cfg config) string {
	mode, err := effectiveMode(cfg)
	if err != nil {
		return "local path"
	}
	if mode == modeSync {
		return "sync local path"
	}
	return "filesystem mount"
}

// setupLocalSurfaceLabel is the right-side hint for the surface menu item.
// Sync mode shows the sync path; mount mode delegates to setupLocalModeLabel.
func setupLocalSurfaceLabel(cfg config) string {
	mode, err := effectiveMode(cfg)
	if err != nil {
		return setupLocalModeLabel(cfg)
	}
	if mode == modeSync {
		path := strings.TrimSpace(cfg.LocalPath)
		if path == "" {
			return "not configured"
		}
		return path
	}
	return setupLocalModeLabel(cfg)
}

func promptRedisConnectionSetup(r *bufio.Reader, out io.Writer, cfg *config) error {
	// ── Redis connection ────────────────────────────────
	fmt.Fprintln(out, "  "+clr(ansiBold+ansiCyan, "▸")+" "+clr(ansiBold, "Redis Connection"))
	fmt.Fprintln(out)

	fmt.Fprintln(out, "  How would you like to connect AFS?")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "    "+clr(ansiCyan, "1")+"  Start and manage a local Redis server")
	fmt.Fprintln(out, "    "+clr(ansiCyan, "2")+"  Connect to an existing Redis server")
	fmt.Fprintln(out)

	connectionDefault := "1"
	if cfg.UseExistingRedis {
		connectionDefault = "2"
	}
	connectionChoice, err := promptString(r, out, "  Choose", connectionDefault)
	if err != nil {
		return err
	}

	switch strings.TrimSpace(connectionChoice) {
	case "1":
		cfg.UseExistingRedis = false
		cfg.RedisUsername = ""
		cfg.RedisPassword = ""
		cfg.RedisTLS = false
	case "2":
		cfg.UseExistingRedis = true
		addr, err := promptString(r, out,
			"\n  Redis server address\n"+
				"  "+clr(ansiDim, "Format: host:port"), cfg.RedisAddr)
		if err != nil {
			return err
		}
		cfg.RedisAddr = addr

		user, err := promptString(r, out,
			"\n  Redis username\n"+
				"  "+clr(ansiDim, "Leave empty for default or password-only auth"), cfg.RedisUsername)
		if err != nil {
			return err
		}
		cfg.RedisUsername = user

		pwd, err := promptString(r, out,
			"\n  Redis password\n"+
				"  "+clr(ansiDim, "Leave empty if none"), "")
		if err != nil {
			return err
		}
		cfg.RedisPassword = pwd

		tlsEnabled, err := promptYesNo(r, out,
			"\n  Use TLS for the Redis connection?\n"+
				"  "+clr(ansiDim, "Enable this for TLS-enabled Redis deployments"), cfg.RedisTLS)
		if err != nil {
			return err
		}
		cfg.RedisTLS = tlsEnabled
	default:
		return fmt.Errorf("unsupported choice %q", connectionChoice)
	}
	return nil
}

func promptLocalFilesystemSetup(r *bufio.Reader, out io.Writer, cfg *config, firstRun bool) error {
	// ── Filesystem mount ────────────────────────────────
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  "+clr(ansiBold+ansiCyan, "▸")+" "+clr(ansiBold, "Filesystem Mount"))
	fmt.Fprintln(out)
	mountDefault := ""
	if !firstRun {
		backendName, err := normalizeMountBackend(cfg.MountBackend)
		if err != nil {
			return err
		}
		if backendName != mountBackendNone && strings.TrimSpace(cfg.LocalPath) != "" {
			mountDefault = cfg.LocalPath
		}
	}
	promptHint := "  " + clr(ansiDim, "Leave empty for no mounted filesystem. Example: ~/afs")
	if mountDefault != "" {
		promptHint = "  " + clr(ansiDim, "Press enter to keep "+mountDefault+", or type none for no mounted filesystem")
	} else if strings.TrimSpace(cfg.CurrentWorkspace) == "" {
		promptHint = "  " + clr(ansiDim, "Leave empty for no mounted filesystem. If you continue, AFS will ask for a workspace name and create it if needed.")
	}
	mp, err := promptString(r, out,
		"  Choose local mount point\n"+promptHint, mountDefault)
	if err != nil {
		return err
	}
	mp = strings.TrimSpace(mp)
	if strings.EqualFold(mp, "none") || mp == "" {
		cfg.MountBackend = mountBackendNone
		cfg.LocalPath = ""
		return nil
	}
	resolvedMountpoint, err := expandPath(mp)
	if err != nil {
		return err
	}
	if err := validateMountpointPath(resolvedMountpoint); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.CurrentWorkspace) == "" {
		workspaceDefault := strings.TrimSpace(filepath.Base(mp))
		if workspaceDefault == "." || workspaceDefault == string(os.PathSeparator) {
			workspaceDefault = ""
		}
		workspace, err := promptString(r, out,
			"\n  Workspace name\n"+
				"  "+clr(ansiDim, "AFS will create this workspace before mounting if it does not already exist"), workspaceDefault)
		if err != nil {
			return err
		}
		workspace = strings.TrimSpace(workspace)
		if workspace == "" {
			return fmt.Errorf("workspace name cannot be empty when enabling a mounted filesystem")
		}
		if err := validateAFSName("workspace", workspace); err != nil {
			return err
		}
		cfg.CurrentWorkspace = workspace
	}
	cfg.LocalPath = resolvedMountpoint
	cfg.MountBackend = defaultMountBackend()
	fmt.Fprintln(out, "  "+clr(ansiDim, "Using "+userModeLabel(cfg.MountBackend)+" for "+cfg.CurrentWorkspace))
	if cfg.MountBackend == mountBackendNFS {
		if strings.TrimSpace(cfg.NFSHost) == "" {
			cfg.NFSHost = "127.0.0.1"
		}
		if cfg.NFSPort <= 0 {
			cfg.NFSPort = 20490
		}
		suggestedPort, occupied, err := suggestNFSPort(cfg.NFSHost, cfg.NFSPort)
		if err != nil {
			return err
		}
		cfg.NFSPort = suggestedPort
		if occupied {
			fmt.Fprintln(out, "  "+clr(ansiDim, "Port was busy; using "+cfg.NFSHost+":"+strconv.Itoa(cfg.NFSPort)+" instead"))
		} else {
			fmt.Fprintln(out, "  "+clr(ansiDim, "Using NFS endpoint "+cfg.NFSHost+":"+strconv.Itoa(cfg.NFSPort)))
		}
	}

	fmt.Fprintln(out)
	return nil
}

// promptModeSetup lets the user pick between the Dropbox-style sync daemon
// and the legacy live FUSE/NFS mount. Sync is the recommended default.
// Mode selection only writes cfg.Mode; the companion surface (sync local
// path vs mountpoint) is still edited from its own menu item so users don't
// lose their existing path configuration when toggling modes.
func promptModeSetup(r *bufio.Reader, out io.Writer, cfg *config) error {
	fmt.Fprintln(out, "  "+clr(ansiBold+ansiCyan, "▸")+" "+clr(ansiBold, "Mode"))
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  How should AFS expose the workspace locally?")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "    "+clr(ansiCyan, "1")+"  "+clr(ansiBold, "Sync")+" "+clr(ansiDim, "(recommended)  — Dropbox-style local-first sync to a real folder"))
	fmt.Fprintln(out, "    "+clr(ansiCyan, "2")+"  "+clr(ansiBold, "Live Mount")+"     — FUSE/NFS mount backed directly by Redis")
	fmt.Fprintln(out)

	current, err := effectiveMode(*cfg)
	if err != nil {
		current = modeSync
	}
	def := "1"
	if current == modeMount {
		def = "2"
	}

	choice, err := promptString(r, out, "  Choose", def)
	if err != nil {
		return err
	}
	fmt.Fprintln(out)

	switch strings.TrimSpace(choice) {
	case "1", "", "sync":
		cfg.Mode = modeSync
		if strings.TrimSpace(cfg.LocalPath) == "" {
			cfg.LocalPath = "~/afs"
			fmt.Fprintln(out, "  "+clr(ansiDim, "Sync local path defaulted to ")+clr(ansiBold, cfg.LocalPath)+clr(ansiDim, " (edit it from the menu)"))
			fmt.Fprintln(out)
		}
	case "2", "mount", "live", "live mount":
		cfg.Mode = modeMount
	default:
		fmt.Fprintln(out, "  "+clr(ansiYellow, "Unknown choice ")+clr(ansiBold, choice)+clr(ansiDim, "; keeping ")+clr(ansiBold, current))
		fmt.Fprintln(out)
	}
	return nil
}

// promptSyncLocalPathSetup edits cfg.LocalPath when the user is already
// in sync mode. It is a deliberately narrower prompt than
// promptLocalFilesystemSetup because sync mode doesn't need backend
// selection or NFS port negotiation.
func promptSyncLocalPathSetup(r *bufio.Reader, out io.Writer, cfg *config) error {
	fmt.Fprintln(out, "  "+clr(ansiBold+ansiCyan, "▸")+" "+clr(ansiBold, "Sync Local Path"))
	fmt.Fprintln(out)

	defaultValue := strings.TrimSpace(cfg.LocalPath)
	promptHint := "  " + clr(ansiDim, "Enter the local directory AFS should sync to. Example: ~/afs")
	if defaultValue != "" {
		promptHint = "  " + clr(ansiDim, "Press enter to keep "+defaultValue+", or type a new path")
	}

	entered, err := promptString(r, out, "  Local path\n"+promptHint, defaultValue)
	if err != nil {
		return err
	}
	entered = strings.TrimSpace(entered)
	if entered == "" {
		return nil
	}
	expanded, err := expandPath(entered)
	if err != nil {
		return err
	}
	cfg.LocalPath = expanded
	fmt.Fprintln(out, "  "+clr(ansiDim, "Sync will write to ")+clr(ansiBold, expanded))
	fmt.Fprintln(out)
	return nil
}

func promptCurrentWorkspaceSetup(r *bufio.Reader, out io.Writer, cfg *config) error {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  "+clr(ansiBold+ansiCyan, "▸")+" "+clr(ansiBold, "Current Workspace"))
	fmt.Fprintln(out)

	promptHint := "  " + clr(ansiDim, "Enter a workspace name, or type none to clear the current workspace")
	defaultValue := strings.TrimSpace(cfg.CurrentWorkspace)
	if defaultValue != "" {
		promptHint = "  " + clr(ansiDim, "Press enter to keep "+defaultValue+", or type none to clear the current workspace")
	}

	workspace, err := promptString(r, out, "  Workspace name\n"+promptHint, defaultValue)
	if err != nil {
		return err
	}
	workspace = strings.TrimSpace(workspace)
	if strings.EqualFold(workspace, "none") {
		cfg.CurrentWorkspace = ""
		return nil
	}
	if workspace == "" {
		cfg.CurrentWorkspace = ""
		return nil
	}
	if err := validateAFSName("workspace", workspace); err != nil {
		return err
	}
	cfg.CurrentWorkspace = workspace
	return nil
}

func suggestNFSPort(host string, preferred int) (int, bool, error) {
	if preferred <= 0 {
		preferred = 20490
	}
	if tcpAddressAvailable(host, preferred) {
		return preferred, false, nil
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return 0, true, err
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || addr.Port <= 0 {
		return 0, true, fmt.Errorf("failed to allocate a free TCP port for %s", host)
	}
	return addr.Port, true, nil
}

func tcpAddressAvailable(host string, port int) bool {
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func prepareRuntimeMountConfig(cfg config, backendName string) (config, error) {
	if backendName != mountBackendNFS {
		return cfg, nil
	}
	if strings.TrimSpace(cfg.NFSHost) == "" {
		cfg.NFSHost = "127.0.0.1"
	}
	if cfg.NFSPort <= 0 {
		cfg.NFSPort = 20490
	}
	port, _, err := suggestNFSPort(cfg.NFSHost, cfg.NFSPort)
	if err != nil {
		return cfg, err
	}
	cfg.NFSPort = port
	return cfg, nil
}

// ---------------------------------------------------------------------------
// up — load config and start services
// ---------------------------------------------------------------------------

func cmdUp() error {
	return cmdUpArgs(nil)
}

func cmdUpArgs(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, upUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	// Parse --interactive flag (runs sync daemon in foreground with live logs).
	foreground := false
	var filteredArgs []string
	for _, a := range args {
		if a == "--interactive" || a == "-i" {
			foreground = true
		} else {
			filteredArgs = append(filteredArgs, a)
		}
	}
	args = filteredArgs

	if st, err := loadState(); err == nil {
		if (st.MountPID > 0 && processAlive(st.MountPID)) || (st.ManageRedis && st.RedisPID > 0 && processAlive(st.RedisPID)) || (st.SyncPID > 0 && processAlive(st.SyncPID)) {
			return cmdStatus()
		}
	}

	cfg, err := loadConfigForUp(args)
	if err != nil {
		return err
	}
	mode, err := effectiveMode(cfg)
	if err != nil {
		return err
	}
	if mode == modeSync {
		return startSyncServices(cfg, foreground)
	}
	if err := cleanupStaleMount(cfg); err != nil {
		return err
	}
	return startServices(cfg)
}

func cleanupStaleMount(cfg config) error {
	if cfg.MountBackend == mountBackendNone || strings.TrimSpace(cfg.LocalPath) == "" {
		return nil
	}
	entry, mounted := mountTableEntry(cfg.LocalPath)
	if !mounted {
		return nil
	}
	if !isAFSMountEntry(entry) {
		return fmt.Errorf("mountpoint %s is already mounted by another filesystem\n  mount entry: %s", cfg.LocalPath, entry)
	}

	backend, _, err := backendForConfig(cfg)
	if err != nil {
		return err
	}

	s := startStep("Cleaning stale mount")
	if err := backend.Unmount(cfg.LocalPath); err != nil {
		s.fail(err.Error())
		return fmt.Errorf("stale AFS mount at %s could not be unmounted: %w", cfg.LocalPath, err)
	}
	s.succeed(cfg.LocalPath)
	return nil
}

func isAFSMountEntry(entry string) bool {
	v := strings.ToLower(entry)
	return strings.Contains(v, "fuse.agent-filesystem") || strings.Contains(v, "agent-filesystem on ") || strings.Contains(v, " agent-filesystem ")
}

// ---------------------------------------------------------------------------
// down — stop services
// ---------------------------------------------------------------------------

func cmdDown() error {
	st, err := loadState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println()
			fmt.Println("  AFS is not running. Nothing to stop.")
			fmt.Println()
			return nil
		}
		return err
	}

	if handled, err := stopSyncServicesIfActive(st); handled || err != nil {
		return err
	}

	fmt.Println()

	backend, _, err := backendForState(st)
	if err != nil {
		return err
	}
	cfg := loadConfigOrDefault()
	if strings.TrimSpace(cfg.WorkRoot) == "" {
		cfg.WorkRoot = defaultWorkRoot()
	}
	if workRoot, err := expandPath(cfg.WorkRoot); err == nil {
		cfg.WorkRoot = workRoot
	}
	if strings.TrimSpace(st.CurrentWorkspace) != "" {
		cfg.CurrentWorkspace = st.CurrentWorkspace
	}

	var rdb *redis.Client

	// Always attempt unmount — even if the daemon crashed, the stale mount
	// may still be in the mount table and block access to the mountpoint.
	if backend.IsMounted(st.LocalPath) {
		s := startStep("Unmounting filesystem")
		if err := backend.Unmount(st.LocalPath); err != nil {
			s.fail(err.Error())
			// Don't return — continue cleanup so the user isn't stuck
			fmt.Printf("  %s manual cleanup: umount -f %s\n", clr(ansiYellow, "!"), st.LocalPath)
		} else {
			s.succeed(st.LocalPath)
		}
	}

	if st.MountPID > 0 && processAlive(st.MountPID) {
		s := startStep("Stopping mount daemon")
		_ = terminatePID(st.MountPID, 2*time.Second)
		s.succeed(fmt.Sprintf("pid %d", st.MountPID))
	}
	if orphanPIDs, err := orphanMountDaemonPIDs(st); err == nil && len(orphanPIDs) > 0 {
		s := startStep("Stopping orphaned mount daemons")
		stopped := make([]string, 0, len(orphanPIDs))
		for _, pid := range orphanPIDs {
			if !processAlive(pid) {
				continue
			}
			if err := terminatePID(pid, 2*time.Second); err == nil {
				stopped = append(stopped, strconv.Itoa(pid))
			}
		}
		if len(stopped) > 0 {
			s.succeed(strings.Join(stopped, ", "))
		} else {
			s.fail("none stopped")
		}
	}

	if shouldCleanLegacyMountCache(st) && !backend.IsMounted(st.LocalPath) {
		redisCfg := cfg
		redisCfg.RedisAddr = st.RedisAddr
		redisCfg.RedisDB = st.RedisDB
		rdb = redis.NewClient(buildRedisOptions(redisCfg, 4))
		s := startStep("Cleaning mount cache")
		if err := deleteNamespace(context.Background(), rdb, st.RedisKey); err != nil {
			s.fail(err.Error())
			fmt.Printf("  %s mount cache preserved in Redis key %s\n", clr(ansiYellow, "!"), st.RedisKey)
		} else {
			s.succeed(st.RedisKey)
		}
		_ = rdb.Close()
	} else if rdb != nil {
		_ = rdb.Close()
	}

	if st.ManageRedis && st.RedisPID > 0 && processAlive(st.RedisPID) {
		s := startStep("Stopping Redis server")
		_ = terminatePID(st.RedisPID, 2*time.Second)
		s.succeed(fmt.Sprintf("pid %d", st.RedisPID))
	}

	// Restore the original directory from the archive if this was a migration
	if st.ArchivePath != "" {
		if _, err := os.Stat(st.ArchivePath); err == nil {
			// Only restore if the mountpoint is now empty/unmounted
			if !backend.IsMounted(st.LocalPath) {
				s := startStep("Restoring original directory")
				_ = os.Remove(st.LocalPath) // remove empty mountpoint dir
				if err := os.Rename(st.ArchivePath, st.LocalPath); err != nil {
					s.fail(err.Error())
					fmt.Printf("  %s archive preserved at %s\n", clr(ansiYellow, "!"), st.ArchivePath)
				} else {
					s.succeed(st.LocalPath)
				}
			} else {
				fmt.Printf("  %s mount still active, archive preserved at %s\n",
					clr(ansiYellow, "!"), st.ArchivePath)
			}
		}
	}

	if st.CreatedLocalPath && st.ArchivePath == "" && !backend.IsMounted(st.LocalPath) {
		removeErr := removeEmptyMountpoint(st.LocalPath)
		if removeErr != nil {
			fmt.Printf("  %s empty mountpoint at %s could not be removed automatically (%v)\n",
				clr(ansiYellow, "!"), st.LocalPath, removeErr)
		}
	}

	if err := os.Remove(statePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	fmt.Printf("\n  %s afs stopped\n\n", clr(ansiDim, "■"))
	return nil
}

// ---------------------------------------------------------------------------
// status — show current state
// ---------------------------------------------------------------------------

func cmdStatus() error {
	st, err := loadState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := loadConfigOrDefault()
			if err2 := resolveConfigPaths(&cfg); err2 != nil {
				cfg.WorkRoot = defaultWorkRoot()
			}
			backendName := cfg.MountBackend
			if backendName == "" {
				backendName = mountBackendNone
			}
			localPath := localSurfacePath(cfg)
			mode, _ := effectiveMode(cfg)
			title := clr(ansiDim, "○") + " " + clr(ansiBold, "afs is not running")
			var rows []boxRow
			if strings.TrimSpace(cfg.CurrentWorkspace) != "" {
				rows = append(rows, boxRow{Label: "workspace", Value: cfg.CurrentWorkspace})
			}
			if localPath != "" {
				rows = append(rows, boxRow{Label: "local", Value: localPath})
			}
			rows = append(rows, boxRow{Label: "database", Value: statusRemoteLabel(cfg.RedisAddr, cfg.RedisDB)})
			if mode == modeMount {
				rows = append(rows, boxRow{Label: "mount backend", Value: userModeLabel(backendName)})
			}
			rows = append(rows,
				boxRow{Label: "mode", Value: mode},
				boxRow{Label: "config", Value: configPathLabel()},
				boxRow{Label: "start", Value: clr(ansiOrange, "afs up")},
			)
			printBox(title, rows)
			return nil
		}
		return err
	}

	if statusSyncIfActive(st) {
		return nil
	}

	backend, backendName, err := backendForState(st)
	if err != nil {
		return err
	}
	cfg := loadConfigOrDefault()
	if err := resolveConfigPaths(&cfg); err != nil {
		cfg.WorkRoot = defaultWorkRoot()
	}
	currentWorkspace := cfg.CurrentWorkspace
	if strings.TrimSpace(st.CurrentWorkspace) != "" {
		currentWorkspace = st.CurrentWorkspace
	}
	localPath := localSurfacePath(cfg)
	if backendName != mountBackendNone && strings.TrimSpace(st.LocalPath) != "" {
		localPath = st.LocalPath
	}
	if backendName == mountBackendNone {
		title := statusTitle(clr(ansiYellow, "○"), currentWorkspace, localPath, false)
		rows := statusRows(backendName, st.RedisAddr, st.RedisDB)
		rows = append(rows, boxRow{Label: "uptime", Value: formatDuration(time.Since(st.StartedAt))})
		if st.ReadOnly {
			rows = append(rows, boxRow{Label: "readonly", Value: "yes"})
		}
		printBox(title, rows)
		return nil
	}
	mounted := backend.IsMounted(st.LocalPath)
	mountAlive := st.MountPID > 0 && processAlive(st.MountPID)

	var title string
	if mounted && mountAlive {
		title = statusTitle(markerSuccess, currentWorkspace, localPath, true)
	} else {
		title = statusTitle(clr(ansiYellow, "○"), currentWorkspace, localPath, false)
	}

	rows := statusRows(backendName, st.RedisAddr, st.RedisDB)
	rows = append(rows, boxRow{Label: "uptime", Value: formatDuration(time.Since(st.StartedAt))})
	if st.ReadOnly {
		rows = append(rows, boxRow{Label: "readonly", Value: "yes"})
	}

	if st.ArchivePath != "" {
		rows = append(rows, boxRow{Label: "archive", Value: st.ArchivePath})
	}

	printBox(title, rows)
	return nil
}

// ---------------------------------------------------------------------------
// Service lifecycle
// ---------------------------------------------------------------------------

func startServices(cfg config) error {
	ctx := context.Background()

	redisPID := 0
	if !cfg.UseExistingRedis {
		s := startStep("Starting Redis server")
		pid, err := startRedisDaemon(cfg)
		if err != nil {
			s.fail(err.Error())
			return err
		}
		redisPID = pid
		s.succeed(fmt.Sprintf("pid %d", pid))
	}

	s := startStep("Connecting to Redis")
	rdb := redis.NewClient(buildRedisOptions(cfg, 4))
	defer rdb.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		s.fail(fmt.Sprintf("cannot reach %s", cfg.RedisAddr))
		return fmt.Errorf("cannot connect to Redis at %s: %w", cfg.RedisAddr, err)
	}
	s.succeed(cfg.RedisAddr)

	backend, backendName, err := backendForConfig(cfg)
	if err != nil {
		return err
	}
	if backendName == mountBackendNone {
		st := state{
			StartedAt:        time.Now().UTC(),
			ManageRedis:      !cfg.UseExistingRedis,
			RedisAddr:        cfg.RedisAddr,
			RedisDB:          cfg.RedisDB,
			CurrentWorkspace: cfg.CurrentWorkspace,
			MountBackend:     backendName,
			ReadOnly:         cfg.ReadOnly,
			RedisKey:         cfg.RedisKey,
			RedisLog:         cfg.RedisLog,
			MountLog:         cfg.MountLog,
			RedisServerBin:   cfg.RedisServerBin,
			MountBin:         cfg.MountBin,
		}
		if !cfg.UseExistingRedis {
			st.RedisPID = redisPID
		}
		if err := saveState(st); err != nil {
			return err
		}
		printReadyBox(cfg, backendName, "")
		return nil
	}

	store := newAFSStore(rdb)
	workspaceStep := startStep("Ensuring current workspace")
	workspace, err := ensureMountWorkspace(ctx, cfg, store)
	if err != nil {
		workspaceStep.fail(err.Error())
		return fmt.Errorf("a current workspace is required before AFS can mount a filesystem: %w", err)
	}
	workspaceStep.succeed(workspace)
	if err := store.checkImportLock(ctx, workspace); err != nil {
		return fmt.Errorf("cannot mount workspace %q: %w", workspace, err)
	}
	prepareStep := startStep("Opening live workspace")
	mountKey, mountedHeadSavepoint, initialized, err := seedWorkspaceMountKey(ctx, store, workspace)
	if err != nil {
		prepareStep.fail(err.Error())
		return err
	}
	if initialized {
		prepareStep.succeed(workspace + " (initialized)")
	} else {
		prepareStep.succeed(workspace)
	}

	mountCfg := cfg
	mountCfg.RedisKey = mountKey
	mountCfg, err = prepareRuntimeMountConfig(mountCfg, backendName)
	if err != nil {
		prepareStep.fail(err.Error())
		return err
	}

	s = startStep("Mounting filesystem")
	createdLocalPath := false
	if _, statErr := os.Stat(mountCfg.LocalPath); errors.Is(statErr, os.ErrNotExist) {
		createdLocalPath = true
	} else if statErr != nil {
		s.fail(statErr.Error())
		return fmt.Errorf("check mountpoint: %w", statErr)
	}
	if err := os.MkdirAll(mountCfg.LocalPath, 0o755); err != nil {
		s.fail(err.Error())
		return fmt.Errorf("create mountpoint: %w", err)
	}

	started, err := backend.Start(mountCfg)
	if err != nil {
		s.fail(err.Error())
		return err
	}
	if err := backend.WaitForMount(mountCfg, started, 6*time.Second); err != nil {
		s.fail("timeout")
		return fmt.Errorf("mount did not become ready: %w", err)
	}
	s.succeed(mountCfg.LocalPath)

	st := state{
		StartedAt:            time.Now().UTC(),
		ManageRedis:          !cfg.UseExistingRedis,
		RedisAddr:            cfg.RedisAddr,
		RedisDB:              cfg.RedisDB,
		CurrentWorkspace:     workspace,
		MountedHeadSavepoint: mountedHeadSavepoint,
		MountPID:             started.PID,
		MountBackend:         backendName,
		ReadOnly:             cfg.ReadOnly,
		MountEndpoint:        started.Endpoint,
		LocalPath:            mountCfg.LocalPath,
		CreatedLocalPath:     createdLocalPath,
		Mode:                 modeMount,
		RedisKey:             mountCfg.RedisKey,
		RedisLog:             cfg.RedisLog,
		MountLog:             cfg.MountLog,
		RedisServerBin:       cfg.RedisServerBin,
		MountBin:             cfg.MountBin,
	}
	if !cfg.UseExistingRedis {
		st.RedisPID = redisPID
	}
	if err := saveState(st); err != nil {
		return err
	}

	cfg.CurrentWorkspace = workspace
	printReadyBox(cfg, backendName, started.Endpoint)
	return nil
}

func printReadyBox(cfg config, backendName, _ string) {
	localPath := localSurfacePath(cfg)
	titlePrefix := markerSuccess
	mounted := backendName != mountBackendNone
	if !mounted {
		titlePrefix = clr(ansiYellow, "○")
	}
	title := statusTitle(titlePrefix, cfg.CurrentWorkspace, localPath, mounted)
	rows := statusRows(backendName, cfg.RedisAddr, cfg.RedisDB)

	if cfg.ReadOnly {
		rows = append(rows, boxRow{Label: "readonly", Value: "yes"})
	}
	if backendName == mountBackendNone {
		rows = append(rows, boxRow{})
		rows = append(rows, boxRow{Label: "create", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" workspace create <workspace>")})
		rows = append(rows, boxRow{Label: "import", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" workspace import <workspace> <directory>")})
		printBox(title, rows)
		return
	}
	rows = append(rows, boxRow{})
	rows = append(rows, boxRow{Label: "try", Value: clr(ansiOrange, "ls "+cfg.LocalPath)})
	rows = append(rows, boxRow{Label: "stop", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" down")})
	printBox(title, rows)
}

func performMigration(cfg config, sourceDir string, r *bufio.Reader) (err error) {
	archiveDir := sourceDir + ".archive"
	ignorer, err := loadMigrationIgnore(sourceDir)
	if err != nil {
		return err
	}

	migrationStartedAt := time.Now()
	rollback := false
	var rdb *redis.Client
	var backend mountBackend
	var mountStarted mountStartResult
	cleanupNamespaceOnImportFailure := false
	defer func() {
		if err != nil && cleanupNamespaceOnImportFailure && rdb != nil {
			_ = deleteNamespace(context.Background(), rdb, cfg.RedisKey)
		}
		if err == nil || errors.Is(err, errMigrationCancelled) {
			return
		}
		if rollback {
			if mountStarted.PID > 0 && processAlive(mountStarted.PID) {
				_ = terminatePID(mountStarted.PID, 2*time.Second)
			}
			if backend != nil {
				_ = backend.Unmount(sourceDir)
			}
			_ = os.RemoveAll(sourceDir)
			_ = os.Remove(sourceDir)
			if backend != nil {
				_ = backend.Unmount(sourceDir)
			}
			_ = os.RemoveAll(sourceDir)
			_ = os.Remove(sourceDir)
			if renameErr := os.Rename(archiveDir, sourceDir); renameErr != nil {
				err = fmt.Errorf("%w\nYour original directory could not be restored automatically. It currently lives at: %s\nRollback error: %v", err, archiveDir, renameErr)
				return
			}
		}
		err = fmt.Errorf("%w\nYour original directory is untouched and still lives at: %s", err, sourceDir)
	}()

	planBackendName, err := normalizeMountBackend(cfg.MountBackend)
	if err != nil {
		return err
	}
	planTitle := clr(ansiBold, "Migration plan")
	rows := []boxRow{
		{Label: "source", Value: sourceDir},
		{Label: "workspace", Value: currentWorkspaceLabel(cfg.CurrentWorkspace)},
		{Label: "database", Value: configRemoteLabel(cfg)},
		{Label: "mountpoint", Value: sourceDir},
		{Label: "mount backend", Value: userModeLabel(planBackendName)},
		boxRow{Label: "archive", Value: archiveDir},
		boxRow{Label: "config", Value: configPathLabel()},
		boxRow{},
		boxRow{Value: clr(ansiDim, "1.") + " Import all files into Redis"},
		boxRow{Value: clr(ansiDim, "2.") + " Move original to archive"},
		boxRow{Value: clr(ansiDim, "3.") + " Mount AFS in place"},
	}
	if ignorer != nil {
		rows = append(rows[:6], append([]boxRow{{Label: "ignore", Value: ignorer.path}}, rows[6:]...)...)
		rows[9].Value = clr(ansiDim, "1.") + " Import files into Redis (respecting " + filepath.Base(ignorer.path) + ")"
	}
	printBox(planTitle, rows)

	ok, err := promptYesNo(r, os.Stdout, "  Proceed?", false)
	if err != nil {
		return err
	}
	if !ok {
		return errMigrationCancelled
	}
	fmt.Println()

	redisPID := 0
	if !cfg.UseExistingRedis {
		s := startStep("Starting Redis server")
		pid, err := startRedisDaemon(cfg)
		if err != nil {
			s.fail(err.Error())
			return err
		}
		redisPID = pid
		s.succeed(fmt.Sprintf("pid %d", pid))
	}

	step := startStep("Connecting to Redis")
	connectCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ctx := context.Background()

	rdb = redis.NewClient(buildRedisOptions(cfg, 8))
	defer rdb.Close()

	if err := rdb.Ping(connectCtx).Err(); err != nil {
		step.fail(fmt.Sprintf("cannot reach %s", cfg.RedisAddr))
		return fmt.Errorf("cannot connect to Redis at %s: %w", cfg.RedisAddr, err)
	}
	step.succeed(cfg.RedisAddr)

	backend, backendName, err := backendForConfig(cfg)
	if err != nil {
		return err
	}

	existingKey, err := namespaceExists(ctx, rdb, cfg.RedisKey)
	if err != nil {
		return err
	}
	overwriteExisting := false
	if existingKey {
		ok, err := promptYesNo(r, os.Stdout,
			fmt.Sprintf("  Redis key %q already exists. Overwrite?", cfg.RedisKey), false)
		if err != nil {
			return err
		}
		if !ok {
			return errMigrationCancelled
		}
		overwriteExisting = true
	}

	step = startStep("Scanning source directory")
	total, err := scanDirectory(sourceDir, ignorer)
	if err != nil {
		step.fail(err.Error())
		return err
	}
	scanDetail := fmt.Sprintf("%d files, %d dirs", total.Files, total.Dirs)
	if total.Symlinks > 0 {
		scanDetail += fmt.Sprintf(", %d symlinks", total.Symlinks)
	}
	if total.Ignored > 0 {
		scanDetail += fmt.Sprintf(", %d ignored", total.Ignored)
	}
	scanDetail += fmt.Sprintf(", %s", formatBytes(total.Bytes))
	scanDuration := step.elapsed()
	step.succeed(scanDetail)

	step = startStep("Checking Redis capacity")
	capacity, err := checkRedisCapacity(ctx, rdb, total)
	if err != nil {
		step.fail(err.Error())
		return err
	}
	capacityDuration := step.elapsed()
	if capacity.Verifiable {
		if capacity.Available < capacity.EstimatedRequired {
			detail := fmt.Sprintf("requires %s, available %s", formatBytes(capacity.EstimatedRequired), formatBytes(maxInt64(capacity.Available, 0)))
			step.fail(detail)
			return fmt.Errorf("redis does not appear to have enough memory for this migration\nestimated required: %s\navailable: %s\nsource data: %s", formatBytes(capacity.EstimatedRequired), formatBytes(maxInt64(capacity.Available, 0)), formatBytes(total.Bytes))
		}
		step.succeed(fmt.Sprintf("estimated %s required, %s available", formatBytes(capacity.EstimatedRequired), formatBytes(capacity.Available)))
	} else {
		step.succeed(fmt.Sprintf("estimated %s required; maxmemory unavailable", formatBytes(capacity.EstimatedRequired)))
	}

	var clearDuration time.Duration
	if overwriteExisting {
		step = startStep("Clearing existing Redis key")
		if err := deleteNamespace(ctx, rdb, cfg.RedisKey); err != nil {
			step.fail(err.Error())
			return fmt.Errorf("delete namespace: %w", err)
		}
		clearDuration = step.elapsed()
		step.succeed(cfg.RedisKey)
	}

	cleanupNamespaceOnImportFailure = true
	if err := createMigrationNamespace(ctx, rdb, cfg.RedisKey); err != nil {
		return err
	}

	var importedDirs int
	var dirDuration time.Duration
	if total.Dirs > 0 {
		step = startStep("Creating directories")
		importedDirs, dirDuration, err = importDirectoriesBatched(ctx, rdb, cfg.RedisKey, sourceDir, ignorer, func(progress importStats) {
			step.update(formatDirImportLabel(progress, total, step.elapsed()))
		})
		if err != nil {
			step.fail(err.Error())
			if isRedisOOM(err) {
				return fmt.Errorf("redis ran out of memory during directory import: %w\nThis source needs more Redis memory than the current server allows.", err)
			}
			return err
		}
		step.succeed(fmt.Sprintf("%d dirs, %s", importedDirs, formatStepDuration(dirDuration)))
	}

	step = startStep("Importing files")
	files, links, importedBytes, fileDuration, err := importFilesBatched(ctx, rdb, cfg.RedisKey, sourceDir, ignorer, func(progress importStats) {
		step.update(formatFileImportLabel(progress, total, step.elapsed()))
	})
	if err != nil {
		step.fail(err.Error())
		if isRedisOOM(err) {
			return fmt.Errorf("redis ran out of memory during file import: %w\nThis source needs more Redis memory than the current server allows.", err)
		}
		return err
	}
	cleanupNamespaceOnImportFailure = false
	importDetail := fmt.Sprintf("%d files", files)
	if links > 0 {
		importDetail += fmt.Sprintf(", %d symlinks", links)
	}
	if total.Ignored > 0 {
		importDetail += fmt.Sprintf(", %d ignored", total.Ignored)
	}
	importDetail += fmt.Sprintf(", %s, %s", formatBytes(importedBytes), formatStepDuration(fileDuration))
	step.succeed(importDetail)

	if _, err := os.Stat(archiveDir); err == nil {
		return fmt.Errorf("archive path already exists: %s", archiveDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	step = startStep("Archiving original directory")
	if err := os.Rename(sourceDir, archiveDir); err != nil {
		step.fail(err.Error())
		return fmt.Errorf("archive failed: %w", err)
	}
	archiveDuration := step.elapsed()
	step.succeed(archiveDir)

	rollback = true

	step = startStep("Mounting filesystem")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		step.fail(err.Error())
		return err
	}
	cfg, err = prepareRuntimeMountConfig(cfg, backendName)
	if err != nil {
		step.fail(err.Error())
		return err
	}

	started, err := backend.Start(cfg)
	if err != nil {
		step.fail(err.Error())
		return err
	}
	mountStarted = started
	if err := backend.WaitForMount(cfg, started, 8*time.Second); err != nil {
		step.fail("timeout")
		return err
	}
	mountDuration := step.elapsed()
	step.succeed(cfg.LocalPath)

	st := state{
		StartedAt:      time.Now().UTC(),
		ManageRedis:    !cfg.UseExistingRedis,
		RedisPID:       redisPID,
		RedisAddr:      cfg.RedisAddr,
		RedisDB:        cfg.RedisDB,
		MountPID:       started.PID,
		MountBackend:   backendName,
		ReadOnly:       cfg.ReadOnly,
		MountEndpoint:  started.Endpoint,
		LocalPath:      cfg.LocalPath,
		Mode:           modeMount,
		RedisKey:       cfg.RedisKey,
		RedisLog:       cfg.RedisLog,
		MountLog:       cfg.MountLog,
		RedisServerBin: cfg.RedisServerBin,
		MountBin:       cfg.MountBin,
		ArchivePath:    archiveDir,
	}
	if err := saveState(st); err != nil {
		return err
	}
	rollback = false

	totalDuration := time.Since(migrationStartedAt)
	title := markerSuccess + " " + clr(ansiBold, "migration complete")
	readyRows := append(commandContextRows(cfg, currentWorkspaceLabel(cfg.CurrentWorkspace)),
		boxRow{Label: "mountpoint", Value: localSurfacePath(cfg)},
		boxRow{Label: "mount backend", Value: userModeLabel(backendName)},
		boxRow{Label: "config", Value: configPathLabel()},
	)
	readyRows = append(readyRows,
		boxRow{Label: "archive", Value: archiveDir},
		boxRow{Label: "data", Value: fmt.Sprintf("%d files, %d dirs, %d symlinks, %s", files, importedDirs, links, formatBytes(importedBytes))},
		boxRow{Label: "scan", Value: formatStepDuration(scanDuration)},
		boxRow{Label: "capacity", Value: formatStepDuration(capacityDuration)},
	)
	if cfg.ReadOnly {
		readyRows = append(readyRows, boxRow{Label: "readonly", Value: "yes"})
	}
	if clearDuration > 0 {
		readyRows = append(readyRows, boxRow{Label: "clear", Value: formatStepDuration(clearDuration)})
	}
	readyRows = append(readyRows,
		boxRow{Label: "dirs", Value: formatStepDuration(dirDuration)},
		boxRow{Label: "files", Value: formatStepDuration(fileDuration)},
		boxRow{Label: "archive in", Value: formatStepDuration(archiveDuration)},
		boxRow{Label: "mount in", Value: formatStepDuration(mountDuration)},
		boxRow{Label: "total", Value: formatStepDuration(totalDuration)},
		boxRow{Label: "rate", Value: formatMigrationThroughput(importedBytes, dirDuration+fileDuration)},
		boxRow{},
		boxRow{Label: "try", Value: clr(ansiOrange, "ls "+cfg.LocalPath)},
		boxRow{Label: "stop", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" down")},
	)
	printBox(title, readyRows)
	return nil
}

// ---------------------------------------------------------------------------
// Directory import
// ---------------------------------------------------------------------------

func importDirectory(ctx context.Context, fsClient importClient, source string, ignorer *migrationIgnore, onProgress func(importStats)) (int, int, int, int64, int, error) {
	var files, dirs, symlinks, ignored int
	var importedBytes int64
	err := filepath.WalkDir(source, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}

		skip, err := ignorer.matches(source, path, d)
		if err != nil {
			return err
		}
		if skip {
			ignored++
			if onProgress != nil {
				onProgress(importStats{
					Files:    files,
					Dirs:     dirs,
					Symlinks: symlinks,
					Ignored:  ignored,
					Bytes:    importedBytes,
				})
			}
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		rel, relErr := filepath.Rel(source, path)
		if relErr != nil {
			return relErr
		}
		redisPath := "/" + filepath.ToSlash(rel)

		info, err := os.Lstat(path)
		if err != nil {
			return err
		}

		switch {
		case d.Type()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := fsClient.Ln(ctx, target, redisPath); err != nil {
				return fmt.Errorf("ln %s: %w", redisPath, err)
			}
			symlinks++
		case d.IsDir():
			if err := fsClient.Mkdir(ctx, redisPath); err != nil {
				return fmt.Errorf("mkdir %s: %w", redisPath, err)
			}
			dirs++
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if err := fsClient.Echo(ctx, redisPath, data); err != nil {
				return fmt.Errorf("echo %s: %w", redisPath, err)
			}
			files++
			importedBytes += int64(len(data))
		}

		if err := applyMetadata(ctx, fsClient, redisPath, info); err != nil {
			return err
		}
		if onProgress != nil {
			onProgress(importStats{
				Files:    files,
				Dirs:     dirs,
				Symlinks: symlinks,
				Ignored:  ignored,
				Bytes:    importedBytes,
			})
		}
		return nil
	})
	return files, dirs, symlinks, importedBytes, ignored, err
}

func scanDirectory(source string, ignorer *migrationIgnore) (importStats, error) {
	var stats importStats
	err := filepath.WalkDir(source, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}

		skip, err := ignorer.matches(source, path, d)
		if err != nil {
			return err
		}
		if skip {
			stats.Ignored++
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		switch {
		case d.Type()&os.ModeSymlink != 0:
			stats.Symlinks++
		case d.IsDir():
			stats.Dirs++
		default:
			info, err := d.Info()
			if err != nil {
				return err
			}
			stats.Files++
			stats.Bytes += info.Size()
		}
		return nil
	})
	return stats, err
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	value := float64(size) / unit
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	unitIndex := 0
	for value >= unit && unitIndex < len(units)-1 {
		value /= unit
		unitIndex++
	}
	return fmt.Sprintf("%.1f %s", value, units[unitIndex])
}

func applyMetadata(ctx context.Context, fsClient importClient, path string, info os.FileInfo) error {
	if err := fsClient.Chmod(ctx, path, uint32(info.Mode().Perm())); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		if err := fsClient.Chown(ctx, path, st.Uid, st.Gid); err != nil {
			return fmt.Errorf("chown %s: %w", path, err)
		}
		aSec, aNsec := statAtime(st)
		mSec, mNsec := statMtime(st)
		atimeMs := aSec*1000 + aNsec/1_000_000
		mtimeMs := mSec*1000 + mNsec/1_000_000
		if err := fsClient.Utimens(ctx, path, atimeMs, mtimeMs); err != nil {
			return fmt.Errorf("utimens %s: %w", path, err)
		}
	}
	return nil
}

func shouldCleanLegacyMountCache(st state) bool {
	redisKey := strings.TrimSpace(st.RedisKey)
	if redisKey == "" {
		return false
	}
	workspace := strings.TrimSpace(st.CurrentWorkspace)
	if workspace == "" {
		return true
	}
	return redisKey != workspaceRedisKey(workspace)
}

func removeEmptyMountpoint(path string) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
		return nil
	}
	return err
}

// ---------------------------------------------------------------------------
// Daemon management
// ---------------------------------------------------------------------------

func startRedisDaemon(cfg config) (int, error) {
	pidfile := fmt.Sprintf("/tmp/afs-%d.pid", cfg.redisPort)
	args := []string{
		"--port", strconv.Itoa(cfg.redisPort),
		"--save", "",
		"--appendonly", "no",
		"--daemonize", "yes",
		"--pidfile", pidfile,
		"--logfile", cfg.RedisLog,
		"--dir", "/tmp",
		"--dbfilename", fmt.Sprintf("afs-%d.rdb", cfg.redisPort),
	}
	cmd := exec.Command(cfg.RedisServerBin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("start redis failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		pidBytes, err := os.ReadFile(pidfile)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
			if err == nil && pid > 0 {
				return pid, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0, errors.New("redis started but pidfile was not found")
}

func deleteNamespace(ctx context.Context, rdb *redis.Client, fsKey string) error {
	patterns := []string{
		"afs:{" + fsKey + "}:*",
		"rfs:{" + fsKey + "}:*",
	}
	for _, pattern := range patterns {
		var cursor uint64
		for {
			keys, next, err := rdb.Scan(ctx, cursor, pattern, 500).Result()
			if err != nil {
				return err
			}
			if len(keys) > 0 {
				if err := rdb.Del(ctx, keys...).Err(); err != nil {
					return err
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	return nil
}

func terminatePID(pid int, timeout time.Duration) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	_ = p.Signal(syscall.SIGTERM)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = p.Signal(syscall.SIGKILL)
	return nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func orphanMountDaemonPIDs(st state) ([]int, error) {
	out, err := exec.Command("ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return nil, err
	}
	return parseOrphanMountDaemonPIDs(st, string(out)), nil
}

func parseOrphanMountDaemonPIDs(st state, psOutput string) []int {
	backendName := strings.TrimSpace(st.MountBackend)
	if backendName == "" || backendName == mountBackendNone {
		return nil
	}

	var matches []int
	for _, rawLine := range strings.Split(psOutput, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 || pid == st.MountPID {
			continue
		}
		command := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
		if mountDaemonMatchesState(backendName, st, command) {
			matches = append(matches, pid)
		}
	}
	sort.Ints(matches)
	return matches
}

func mountDaemonMatchesState(backendName string, st state, command string) bool {
	switch backendName {
	case mountBackendNFS:
		if !strings.Contains(command, "agent-filesystem-nfs") {
			return false
		}
		return strings.Contains(command, "--redis "+st.RedisAddr) &&
			strings.Contains(command, "--db "+strconv.Itoa(st.RedisDB)) &&
			strings.Contains(command, "--export "+nfsExportPath(st.RedisKey))
	case mountBackendFuse:
		if !strings.Contains(command, "agent-filesystem-mount") {
			return false
		}
		return strings.Contains(command, " "+st.RedisKey+" "+st.LocalPath)
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Config persistence (~/.afs/config.json)
// ---------------------------------------------------------------------------

func configPath() string {
	if cfgPathOverride != "" {
		return cfgPathOverride
	}
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
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), b, 0o644)
}

func loadConfig() (config, error) {
	cfg := defaultConfig()
	b, err := os.ReadFile(configPath())
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	// Migrate legacy field names.
	if cfg.LocalPath == "" {
		if cfg.legacySyncLocalPath != "" {
			cfg.LocalPath = cfg.legacySyncLocalPath
		} else if cfg.legacyMountpoint != "" {
			cfg.LocalPath = cfg.legacyMountpoint
		}
	}
	return cfg, nil
}

func defaultConfig() config {
	return config{
		RedisAddr:    "localhost:6379",
		RedisDB:      0,
		WorkRoot:     defaultWorkRoot(),
		RedisKey:     "myfs",
		LocalPath:    "~/afs",
		MountBackend: mountBackendAuto,
		NFSHost:      "127.0.0.1",
		NFSPort:      20490,
		RedisLog:     "/tmp/afs-redis.log",
		MountLog:     "/tmp/afs-mount.log",
	}
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
	if strings.TrimSpace(cfg.RedisAddr) == "" {
		cfg.RedisAddr = def.RedisAddr
	}
	if cfg.RedisDB < 0 {
		return fmt.Errorf("redis db must be >= 0")
	}

	if cfg.LocalPath != "" {
		mp, err := expandPath(cfg.LocalPath)
		if err != nil {
			return err
		}
		cfg.LocalPath = mp
	}
	if strings.TrimSpace(cfg.WorkRoot) == "" {
		cfg.WorkRoot = defaultWorkRoot()
	}
	workRoot, err := expandPath(cfg.WorkRoot)
	if err != nil {
		return err
	}
	cfg.WorkRoot = workRoot
	if strings.TrimSpace(cfg.RedisLog) == "" {
		cfg.RedisLog = def.RedisLog
	}
	if strings.TrimSpace(cfg.MountLog) == "" {
		cfg.MountLog = def.MountLog
	}
	if strings.TrimSpace(cfg.RedisKey) == "" {
		cfg.RedisKey = def.RedisKey
	}

	backendName, err := normalizeMountBackend(cfg.MountBackend)
	if err != nil {
		return err
	}
	cfg.MountBackend = backendName

	if backendName == mountBackendNone {
		// No mount backend — LocalPath is used for sync mode only; don't clear it.
	} else {
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

	// Sync mode validation. Mode is left empty for legacy configs; effectiveMode
	// translates that to "mount" at runtime.
	if strings.TrimSpace(cfg.Mode) != "" {
		if _, err := effectiveMode(*cfg); err != nil {
			return err
		}
	}
	if cfg.SyncFileSizeCapMB < 0 {
		return fmt.Errorf("syncFileSizeCapMB must be >= 0")
	}
	if strings.TrimSpace(cfg.SyncLog) == "" {
		cfg.SyncLog = "/tmp/afs-sync.log"
	}

	host, port, err := splitAddr(cfg.RedisAddr)
	if err != nil {
		return err
	}
	cfg.redisHost = host
	cfg.redisPort = port
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

	backendName := cfg.MountBackend
	if backendName != mountBackendNone {
		switch backendName {
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
	}

	if !cfg.UseExistingRedis {
		if cfg.RedisServerBin == "" {
			resolved, err := resolveBinary(defaultRedisBin())
			if err != nil {
				return fmt.Errorf("cannot find redis-server binary\n  Install Redis or set useExistingRedis to true in config")
			}
			cfg.RedisServerBin = resolved
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// State persistence (~/.afs/state.json)
// ---------------------------------------------------------------------------

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

func statePath() string {
	return filepath.Join(stateDir(), "state.json")
}

func saveState(st state) error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(), b, 0o600)
}

func loadState() (state, error) {
	var st state
	b, err := os.ReadFile(statePath())
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return st, err
	}
	// Migrate legacy field names.
	if st.LocalPath == "" {
		if st.legacySyncLocalPath != "" {
			st.LocalPath = st.legacySyncLocalPath
		} else if st.legacyMountpoint != "" {
			st.LocalPath = st.legacyMountpoint
		}
	}
	if st.Mode == "" && st.legacySyncMode != "" {
		st.Mode = st.legacySyncMode
	}
	if !st.CreatedLocalPath && st.legacyCreatedMountpoint {
		st.CreatedLocalPath = st.legacyCreatedMountpoint
	}
	return st, nil
}

// ---------------------------------------------------------------------------
// Prompt helpers
// ---------------------------------------------------------------------------

func promptString(r *bufio.Reader, out io.Writer, label, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, clr(ansiCyan, def))
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(line)
	if v == "" {
		return def, nil
	}
	return v, nil
}

func promptYesNo(r *bufio.Reader, out io.Writer, label string, def bool) (bool, error) {
	defMark := "y/N"
	if def {
		defMark = "Y/n"
	}
	fmt.Fprintf(out, "%s [%s]: ", label, clr(ansiCyan, defMark))
	line, err := r.ReadString('\n')
	if err != nil {
		return false, err
	}
	v := strings.ToLower(strings.TrimSpace(line))
	if v == "" {
		return def, nil
	}
	if v == "y" || v == "yes" {
		return true, nil
	}
	if v == "n" || v == "no" {
		return false, nil
	}
	return def, nil
}

// ---------------------------------------------------------------------------
// Path / binary helpers
// ---------------------------------------------------------------------------

func splitAddr(addr string) (string, int, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid address %q (expected host:port)", addr)
	}
	p, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, err
	}
	return parts[0], p, nil
}

func expandPath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, p[2:])
	}
	return filepath.Abs(p)
}

func resolveBinary(p string) (string, error) {
	if strings.Contains(p, "/") {
		return expandPath(p)
	}
	lp, err := exec.LookPath(p)
	if err != nil {
		return "", fmt.Errorf("binary %q not found in PATH", p)
	}
	return lp, nil
}

func exeDir() string {
	exe, err := executablePath()
	if err != nil {
		cwd, _ := os.Getwd()
		return cwd
	}
	return filepath.Dir(exe)
}

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return resolveExecutablePath(exe), nil
}

func resolveExecutablePath(exe string) string {
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

func defaultRedisBin() string {
	candidate := filepath.Join(os.Getenv("HOME"), "git", "redis", "src", "redis-server")
	if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
		return candidate
	}
	if lp, err := exec.LookPath("redis-server"); err == nil {
		return lp
	}
	return "redis-server"
}

func fatal(err error) {
	showCursor()
	if colorTerm {
		fmt.Fprintf(os.Stderr, "\n  %s%serror:%s %v\n\n", ansiBold, ansiRed, ansiReset, err)
	} else {
		fmt.Fprintf(os.Stderr, "\n  error: %v\n\n", err)
	}
	os.Exit(1)
}
