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
	"strconv"
	"strings"
	"syscall"
	"time"

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
	RuntimeMode      string `json:"runtimeMode"`
	RedisKey         string `json:"redisKey"`
	Mountpoint       string `json:"mountpoint"`
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

	// Derived at runtime, not persisted.
	redisHost string
	redisPort int
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
	Mountpoint           string    `json:"mountpoint"`
	RedisKey             string    `json:"redis_key"`
	RedisLog             string    `json:"redis_log"`
	MountLog             string    `json:"mount_log"`
	RedisServerBin       string    `json:"redis_server_bin"`
	MountBin             string    `json:"mount_bin"`
	ArchivePath          string    `json:"archive_path,omitempty"`
}

type importClient interface {
	Mkdir(ctx context.Context, path string) error
	Echo(ctx context.Context, path string, data []byte) error
	Ln(ctx context.Context, target, linkpath string) error
	Chmod(ctx context.Context, path string, mode uint32) error
	Chown(ctx context.Context, path string, uid, gid uint32) error
	Utimens(ctx context.Context, path string, atimeMs, mtimeMs int64) error
}

type importStats struct {
	Files    int
	Dirs     int
	Symlinks int
	Ignored  int
	Bytes    int64
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

var cfgPathOverride string
var errMigrationCancelled = errors.New("migration cancelled")

func main() {
	defer showCursor()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
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
	case "setup":
		if err := cmdSetup(); err != nil {
			fatal(err)
		}
	case "config":
		if err := cmdConfig(args); err != nil {
			fatal(err)
		}
	case "up":
		if err := cmdUpArgs(args[1:]); err != nil {
			fatal(err)
		}
	case "down":
		if err := cmdDown(); err != nil {
			fatal(err)
		}
	case "status":
		if err := cmdStatus(); err != nil {
			fatal(err)
		}
	case "workspace":
		if err := cmdWorkspace(args); err != nil {
			fatal(err)
		}
	case "checkpoint":
		if err := cmdCheckpoint(args); err != nil {
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
  workspace ...        Workspace operations (create, list, current, use, run, clone, fork, delete, import)
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

func statusRemoteLabel(addr string, db int) string {
	return fmt.Sprintf("redis://%s (db %d)", addr, db)
}

func statusTitle(prefix, backendName, workspace, localPath string) string {
	if backendName == mountBackendNone {
		return prefix + " " + clr(ansiBold, "afs no mounted filesystem")
	}
	return prefix + " " + clr(ansiBold, fmt.Sprintf("Workspace: %s mounted at %s (via %s)", currentWorkspaceLabel(workspace), localPath, userModeLabel(backendName)))
}

func localSurfacePath(cfg config, backendName string) string {
	if backendName == mountBackendNone {
		return cfg.WorkRoot
	}
	return cfg.Mountpoint
}

func statusRows(backendName, localPath, redisAddr string, redisDB int, currentWorkspace string) []boxRow {
	if backendName == mountBackendNone {
		return []boxRow{
			{Label: "redis", Value: statusRemoteLabel(redisAddr, redisDB)},
			{Label: "current workspace", Value: currentWorkspaceLabel(currentWorkspace)},
			{Label: "config", Value: clr(ansiDim, compactDisplayPath(configPath()))},
		}
	}
	return []boxRow{
		{Label: "local filesystem", Value: localPath},
		{Label: "redis", Value: statusRemoteLabel(redisAddr, redisDB)},
		{Label: "current workspace", Value: currentWorkspaceLabel(currentWorkspace)},
		{Label: "config", Value: clr(ansiDim, compactDisplayPath(configPath()))},
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
	cfg, migrateDir, err := runSetupWizard(r, os.Stdout, cfg, firstRun)
	if err != nil {
		return err
	}

	if err := resolveConfigPaths(&cfg); err != nil {
		return err
	}

	if err := saveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("  %s Saved to %s\n\n", clr(ansiDim, "▸"), clr(ansiCyan, compactDisplayPath(configPath())))

	if migrateDir != "" {
		return performMigration(cfg, migrateDir, r)
	}
	return startServices(cfg)
}

func runSetupWizard(r *bufio.Reader, out io.Writer, cfg config, firstRun bool) (config, string, error) {
	if firstRun {
		return runFullSetupWizard(r, out, cfg)
	}

	fmt.Fprintln(out, "  "+clr(ansiBold+ansiCyan, "▸")+" "+clr(ansiBold, "Setup"))
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  What would you like to change?")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "    "+clr(ansiCyan, "1")+"  Change Redis connection "+clr(ansiDim, "("+setupRedisConnectionLabel(cfg)+")"))
	fmt.Fprintln(out, "    "+clr(ansiCyan, "2")+"  Change filesystem mount "+clr(ansiDim, "("+setupLocalModeLabel(cfg)+")"))
	fmt.Fprintln(out, "    "+clr(ansiCyan, "3")+"  Change current workspace "+clr(ansiDim, "("+currentWorkspaceLabel(cfg.CurrentWorkspace)+")"))
	fmt.Fprintln(out)

	choice, err := promptString(r, out, "  Choose", "1")
	if err != nil {
		return cfg, "", err
	}

	switch strings.TrimSpace(choice) {
	case "1":
		if err := promptRedisConnectionSetup(r, out, &cfg); err != nil {
			return cfg, "", err
		}
		return cfg, "", nil
	case "2":
		migrateDir, err := promptLocalFilesystemSetup(r, out, &cfg, false)
		if err != nil {
			return cfg, "", err
		}
		return cfg, migrateDir, nil
	case "3":
		if err := promptCurrentWorkspaceSetup(r, out, &cfg); err != nil {
			return cfg, "", err
		}
		return cfg, "", nil
	default:
		return cfg, "", fmt.Errorf("unsupported choice %q", choice)
	}
}

func runFullSetupWizard(r *bufio.Reader, out io.Writer, cfg config) (config, string, error) {
	if err := promptRedisConnectionSetup(r, out, &cfg); err != nil {
		return cfg, "", err
	}
	migrateDir, err := promptLocalFilesystemSetup(r, out, &cfg, true)
	if err != nil {
		return cfg, "", err
	}
	return cfg, migrateDir, nil
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
	if strings.TrimSpace(cfg.Mountpoint) != "" {
		return label + " at " + cfg.Mountpoint
	}
	return label
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

func promptLocalFilesystemSetup(r *bufio.Reader, out io.Writer, cfg *config, firstRun bool) (string, error) {
	// ── Filesystem mount ────────────────────────────────
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  "+clr(ansiBold+ansiCyan, "▸")+" "+clr(ansiBold, "Filesystem Mount"))
	fmt.Fprintln(out)
	mountDefault := ""
	if !firstRun {
		backendName, err := normalizeMountBackend(cfg.MountBackend)
		if err != nil {
			return "", err
		}
		if backendName != mountBackendNone && strings.TrimSpace(cfg.Mountpoint) != "" {
			mountDefault = cfg.Mountpoint
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
		return "", err
	}
	mp = strings.TrimSpace(mp)
	if strings.EqualFold(mp, "none") || mp == "" {
		cfg.MountBackend = mountBackendNone
		cfg.Mountpoint = ""
		return "", nil
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
			return "", err
		}
		workspace = strings.TrimSpace(workspace)
		if workspace == "" {
			return "", fmt.Errorf("workspace name cannot be empty when enabling a mounted filesystem")
		}
		if err := validateAFSName("workspace", workspace); err != nil {
			return "", err
		}
		cfg.CurrentWorkspace = workspace
	}
	cfg.Mountpoint, err = expandPath(mp)
	if err != nil {
		return "", err
	}
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
			return "", err
		}
		cfg.NFSPort = suggestedPort
		if occupied {
			fmt.Fprintln(out, "  "+clr(ansiDim, "Port was busy; using "+cfg.NFSHost+":"+strconv.Itoa(cfg.NFSPort)+" instead"))
		} else {
			fmt.Fprintln(out, "  "+clr(ansiDim, "Using NFS endpoint "+cfg.NFSHost+":"+strconv.Itoa(cfg.NFSPort)))
		}
	}

	fmt.Fprintln(out)
	return "", nil
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

	if st, err := loadState(); err == nil {
		if (st.MountPID > 0 && processAlive(st.MountPID)) || (st.ManageRedis && st.RedisPID > 0 && processAlive(st.RedisPID)) {
			return cmdStatus()
		}
	}

	cfg, err := loadConfigForUp(args)
	if err != nil {
		return err
	}
	if err := cleanupStaleMount(cfg); err != nil {
		return err
	}

	printBanner()
	return startServices(cfg)
}

func cleanupStaleMount(cfg config) error {
	if cfg.MountBackend == mountBackendNone || strings.TrimSpace(cfg.Mountpoint) == "" {
		return nil
	}
	entry, mounted := mountTableEntry(cfg.Mountpoint)
	if !mounted {
		return nil
	}
	if !isAFSMountEntry(entry) {
		return fmt.Errorf("mountpoint %s is already mounted by another filesystem\n  mount entry: %s", cfg.Mountpoint, entry)
	}

	backend, _, err := backendForConfig(cfg)
	if err != nil {
		return err
	}

	s := startStep("Cleaning stale mount")
	if err := backend.Unmount(cfg.Mountpoint); err != nil {
		s.fail(err.Error())
		return fmt.Errorf("stale AFS mount at %s could not be unmounted: %w", cfg.Mountpoint, err)
	}
	s.succeed(cfg.Mountpoint)
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
	if st.MountBackend != mountBackendNone && !st.ReadOnly && strings.TrimSpace(st.CurrentWorkspace) != "" && strings.TrimSpace(st.RedisKey) != "" {
		redisCfg := cfg
		redisCfg.RedisAddr = st.RedisAddr
		redisCfg.RedisDB = st.RedisDB

		rdb = redis.NewClient(buildRedisOptions(redisCfg, 8))
		pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = rdb.Ping(pingCtx).Err()
		cancel()
		if err != nil {
			fmt.Printf("  %s mounted workspace could not be saved; continuing shutdown (%v)\n", clr(ansiYellow, "!"), err)
		} else {
			expectedHead := strings.TrimSpace(st.MountedHeadSavepoint)
			if expectedHead == "" {
				store := newAFSStore(rdb)
				workspaceMeta, metaErr := store.getWorkspaceMeta(context.Background(), st.CurrentWorkspace)
				if metaErr != nil {
					fmt.Printf("  %s mounted workspace could not be saved; continuing shutdown (%v)\n", clr(ansiYellow, "!"), metaErr)
				} else {
					expectedHead = workspaceMeta.HeadSavepoint
				}
			}

			if expectedHead != "" {
				s := startStep("Saving mounted workspace")
				saved, syncErr := syncMountedWorkspaceBack(context.Background(), cfg, newAFSStore(rdb), rdb, st.CurrentWorkspace, expectedHead)
				if syncErr != nil {
					s.fail(syncErr.Error())
					fmt.Printf("  %s mounted changes for %s were not saved; continuing shutdown\n", clr(ansiYellow, "!"), st.CurrentWorkspace)
				} else if saved {
					s.succeed(st.CurrentWorkspace)
				} else {
					s.succeed("no changes")
				}
			} else {
				fmt.Printf("  %s mounted workspace head is unavailable; continuing shutdown without saving\n", clr(ansiYellow, "!"))
			}
		}
	}

	// Always attempt unmount — even if the daemon crashed, the stale mount
	// may still be in the mount table and block access to the mountpoint.
	if backend.IsMounted(st.Mountpoint) {
		s := startStep("Unmounting filesystem")
		if err := backend.Unmount(st.Mountpoint); err != nil {
			s.fail(err.Error())
			// Don't return — continue cleanup so the user isn't stuck
			fmt.Printf("  %s manual cleanup: umount -f %s\n", clr(ansiYellow, "!"), st.Mountpoint)
		} else {
			s.succeed(st.Mountpoint)
		}
	}

	if st.MountPID > 0 && processAlive(st.MountPID) {
		s := startStep("Stopping mount daemon")
		_ = terminatePID(st.MountPID, 2*time.Second)
		s.succeed(fmt.Sprintf("pid %d", st.MountPID))
	}

	if rdb != nil && strings.TrimSpace(st.RedisKey) != "" && !backend.IsMounted(st.Mountpoint) {
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
			if !backend.IsMounted(st.Mountpoint) {
				s := startStep("Restoring original directory")
				_ = os.Remove(st.Mountpoint) // remove empty mountpoint dir
				if err := os.Rename(st.ArchivePath, st.Mountpoint); err != nil {
					s.fail(err.Error())
					fmt.Printf("  %s archive preserved at %s\n", clr(ansiYellow, "!"), st.ArchivePath)
				} else {
					s.succeed(st.Mountpoint)
				}
			} else {
				fmt.Printf("  %s mount still active, archive preserved at %s\n",
					clr(ansiYellow, "!"), st.ArchivePath)
			}
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
			title := clr(ansiDim, "○") + " afs is not running"
			printBox(title, []boxRow{
				{Label: "start", Value: clr(ansiCyan, "afs up")},
			})
			return nil
		}
		return err
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
	localPath := localSurfacePath(cfg, backendName)
	if backendName != mountBackendNone && strings.TrimSpace(st.Mountpoint) != "" {
		localPath = st.Mountpoint
	}
	if backendName == mountBackendNone {
		title := statusTitle(clr(ansiBGreen, "●"), backendName, currentWorkspace, localPath)
		rows := statusRows(backendName, localPath, st.RedisAddr, st.RedisDB, currentWorkspace)
		rows = append(rows, boxRow{Label: "uptime", Value: formatDuration(time.Since(st.StartedAt))})
		if st.ReadOnly {
			rows = append(rows, boxRow{Label: "readonly", Value: "yes"})
		}
		printBox(title, rows)
		return nil
	}
	mounted := backend.IsMounted(st.Mountpoint)
	mountAlive := st.MountPID > 0 && processAlive(st.MountPID)

	var title string
	if mounted && mountAlive {
		title = statusTitle(clr(ansiBGreen, "●"), backendName, currentWorkspace, localPath)
	} else {
		title = statusTitle(clr(ansiYellow, "○"), backendName, currentWorkspace, localPath)
	}

	rows := statusRows(backendName, localPath, st.RedisAddr, st.RedisDB, currentWorkspace)
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
	workspace, created, err := ensureMountWorkspace(ctx, cfg, store)
	if err != nil {
		workspaceStep.fail(err.Error())
		return fmt.Errorf("a current workspace is required before AFS can mount a filesystem: %w", err)
	}
	if created {
		workspaceStep.succeed(workspace + " (created)")
	} else {
		workspaceStep.succeed(workspace)
	}
	prepareStep := startStep("Preparing mounted workspace")
	mountKey, mountedHeadSavepoint, err := seedWorkspaceMountKey(ctx, cfg, store, rdb, workspace)
	if err != nil {
		prepareStep.fail(err.Error())
		return err
	}
	prepareStep.succeed(workspace)

	mountCfg := cfg
	mountCfg.RedisKey = mountKey

	s = startStep("Mounting filesystem")
	if err := os.MkdirAll(mountCfg.Mountpoint, 0o755); err != nil {
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
	s.succeed(mountCfg.Mountpoint)

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
		Mountpoint:           mountCfg.Mountpoint,
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
	localPath := localSurfacePath(cfg, backendName)
	title := statusTitle(clr(ansiBGreen, "●"), backendName, cfg.CurrentWorkspace, localPath)
	rows := statusRows(backendName, localPath, cfg.RedisAddr, cfg.RedisDB, cfg.CurrentWorkspace)

	if cfg.ReadOnly {
		rows = append(rows, boxRow{Label: "readonly", Value: "yes"})
	}
	if backendName == mountBackendNone {
		rows = append(rows, boxRow{})
		rows = append(rows, boxRow{Label: "create", Value: clr(ansiCyan, filepath.Base(os.Args[0])+" workspace create <workspace>")})
		rows = append(rows, boxRow{Label: "import", Value: clr(ansiCyan, filepath.Base(os.Args[0])+" workspace import <workspace> <directory>")})
		printBox(title, rows)
		return
	}
	rows = append(rows, boxRow{})
	rows = append(rows, boxRow{Label: "try", Value: clr(ansiCyan, "ls "+cfg.Mountpoint)})
	rows = append(rows, boxRow{Label: "stop", Value: clr(ansiCyan, filepath.Base(os.Args[0])+" down")})
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
	rows := statusRows(planBackendName, sourceDir, cfg.RedisAddr, cfg.RedisDB, cfg.CurrentWorkspace)
	rows = append(rows,
		boxRow{Label: "archive", Value: archiveDir},
		boxRow{},
		boxRow{Value: clr(ansiDim, "1.") + " Import all files into Redis"},
		boxRow{Value: clr(ansiDim, "2.") + " Move original to archive"},
		boxRow{Value: clr(ansiDim, "3.") + " Mount AFS in place"},
	)
	if ignorer != nil {
		rows = append(rows[:5], append([]boxRow{{Label: "ignore", Value: ignorer.path}}, rows[5:]...)...)
		rows[7].Value = clr(ansiDim, "1.") + " Import files into Redis (respecting " + filepath.Base(ignorer.path) + ")"
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
	step.succeed(cfg.Mountpoint)

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
		Mountpoint:     cfg.Mountpoint,
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
	title := clr(ansiBGreen, "●") + " " + clr(ansiBold, "migration complete")
	readyRows := statusRows(backendName, localSurfacePath(cfg, backendName), cfg.RedisAddr, cfg.RedisDB, cfg.CurrentWorkspace)
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
		boxRow{Label: "try", Value: clr(ansiCyan, "ls "+cfg.Mountpoint)},
		boxRow{Label: "stop", Value: clr(ansiCyan, filepath.Base(os.Args[0])+" down")},
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

		info, err := os.Lstat(path)
		if err != nil {
			return err
		}

		switch {
		case d.Type()&os.ModeSymlink != 0:
			stats.Symlinks++
		case d.IsDir():
			stats.Dirs++
		default:
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
	return cfg, nil
}

func defaultConfig() config {
	return config{
		RedisAddr:    "localhost:6379",
		RedisDB:      0,
		WorkRoot:     defaultWorkRoot(),
		RuntimeMode:  "host",
		RedisKey:     "myfs",
		Mountpoint:   "~/afs",
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

	if cfg.Mountpoint != "" {
		mp, err := expandPath(cfg.Mountpoint)
		if err != nil {
			return err
		}
		cfg.Mountpoint = mp
	}
	if strings.TrimSpace(cfg.WorkRoot) == "" {
		cfg.WorkRoot = defaultWorkRoot()
	}
	workRoot, err := expandPath(cfg.WorkRoot)
	if err != nil {
		return err
	}
	cfg.WorkRoot = workRoot
	if strings.TrimSpace(cfg.RuntimeMode) == "" {
		cfg.RuntimeMode = "host"
	}
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
		cfg.Mountpoint = ""
	} else {
		if strings.TrimSpace(cfg.Mountpoint) == "" {
			mp, err := expandPath(def.Mountpoint)
			if err != nil {
				return err
			}
			cfg.Mountpoint = mp
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

	host, port, err := splitAddr(cfg.RedisAddr)
	if err != nil {
		return err
	}
	cfg.redisHost = host
	cfg.redisPort = port
	return nil
}

func resolveConfigPaths(cfg *config) error {
	dir := exeDir()
	if err := prepareConfigForSave(cfg); err != nil {
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
