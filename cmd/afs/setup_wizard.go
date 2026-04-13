package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// cmdSetup runs the interactive configuration wizard and writes the config
// file, but deliberately does not start services.
func cmdSetup() error {
	if st, err := loadState(); err == nil {
		if (st.MountPID > 0 && processAlive(st.MountPID)) || (st.SyncPID > 0 && processAlive(st.SyncPID)) {
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
	label := cfg.RedisAddr
	if cfg.RedisTLS {
		label += ", tls"
	}
	if strings.TrimSpace(label) == "" {
		return "Redis not configured"
	}
	return label
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
// it's the FUSE/NFS mountpoint.
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
	fmt.Fprintln(out, "  "+clr(ansiBold+ansiCyan, "▸")+" "+clr(ansiBold, "Redis Connection"))
	fmt.Fprintln(out)

	addr, err := promptString(r, out,
		"  Redis server address\n"+
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
	return nil
}

func promptLocalFilesystemSetup(r *bufio.Reader, out io.Writer, cfg *config, firstRun bool) error {
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

// promptSyncLocalPathSetup edits cfg.LocalPath when the user is already in
// sync mode. It is deliberately narrower than promptLocalFilesystemSetup
// because sync mode doesn't need backend selection or NFS port negotiation.
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
