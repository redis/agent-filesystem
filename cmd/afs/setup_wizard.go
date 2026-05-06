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

// cmdSetup walks the user through the local runtime mode. Workspace selection
// and local directories belong to `afs ws mount`. It deliberately does not
// start services.
func cmdSetup() error {
	if st, err := loadState(); err == nil {
		if (st.MountPID > 0 && processAlive(st.MountPID)) || (st.SyncPID > 0 && processAlive(st.SyncPID)) {
			unmountCmd := filepath.Base(os.Args[0]) + " down"
			if localPath := strings.TrimSpace(st.LocalPath); localPath != "" {
				unmountCmd = filepath.Base(os.Args[0]) + " ws unmount " + shellQuote(localPath)
			}
			return fmt.Errorf("afs is currently mounted\nRun '%s' first", unmountCmd)
		}
	}

	printBanner()

	cfg := defaultConfig()
	firstRun := true
	if loaded, err := loadConfig(); err == nil {
		cfg = loaded
		firstRun = false
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	fmt.Println("  " + clr(ansiDim, "AFS can expose workspaces through sync or a live mount."))
	fmt.Println("  " + clr(ansiDim, "Choose the default mode here; mount a workspace separately."))
	fmt.Println()
	if firstRun {
		fmt.Println("  " + clr(ansiBold, "Let's get you set up."))
	} else {
		fmt.Println("  " + clr(ansiBold, "Let's update your configuration."))
	}
	fmt.Println()

	if strings.TrimSpace(cfg.ProductMode) == "" {
		cfg.ProductMode = productModeLocal
	}

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
	fmt.Printf("  %s Run %s to mount a workspace\n\n", clr(ansiDim, "▸"), clr(ansiOrange, filepath.Base(os.Args[0])+" ws mount"))
	return nil
}

// runSetupWizard runs the interactive setup flow. Setup owns the default mode
// only; workspace selection and local paths are per-mount runtime state.
func runSetupWizard(r *bufio.Reader, out io.Writer, cfg config, firstRun bool) (config, error) {
	if firstRun {
		return runFullSetupWizard(r, out, cfg)
	}
	return runEditSetupWizard(r, out, cfg)
}

func runEditSetupWizard(r *bufio.Reader, out io.Writer, cfg config) (config, error) {
	return runModeSetupWizard(r, out, cfg)
}

func runFullSetupWizard(r *bufio.Reader, out io.Writer, cfg config) (config, error) {
	return runModeSetupWizard(r, out, cfg)
}

func runModeSetupWizard(r *bufio.Reader, out io.Writer, cfg config) (config, error) {
	if strings.TrimSpace(cfg.Mode) == "" {
		cfg.Mode = modeSync
	}
	if err := promptModeSetup(r, out, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// promptModeSetup lets the user pick between the Dropbox-style sync daemon
// and the legacy live FUSE/NFS mount. Sync is the recommended default.
func promptModeSetup(r *bufio.Reader, out io.Writer, cfg *config) error {
	fmt.Fprintln(out, "  "+clr(ansiBold+ansiCyan, "▸")+" "+clr(ansiBold, "Mode"))
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  How should AFS expose the workspace locally?")
	fmt.Fprintln(out)

	current, err := effectiveMode(*cfg)
	if err != nil {
		current = modeSync
	}
	connection, err := effectiveProductMode(*cfg)
	if err != nil {
		connection = productModeLocal
	}

	if connection == productModeSelfHosted {
		fmt.Fprintln(out, "    "+clr(ansiCyan, "1")+"  "+clr(ansiBold, "Sync")+" "+clr(ansiDim, "(recommended)  — local-first sync from a self-managed control plane"))
		fmt.Fprintln(out, "    "+clr(ansiCyan, "2")+"  "+clr(ansiBold, "Live Mount")+"     — FUSE/NFS mount using the control plane's live workspace root")
		fmt.Fprintln(out)
	}
	if connection != productModeSelfHosted {
		fmt.Fprintln(out, "    "+clr(ansiCyan, "1")+"  "+clr(ansiBold, "Sync")+" "+clr(ansiDim, "(recommended)  — Dropbox-style local-first sync to a real folder"))
		fmt.Fprintln(out, "    "+clr(ansiCyan, "2")+"  "+clr(ansiBold, "Live Mount")+"     — FUSE/NFS mount backed directly by Redis")
		fmt.Fprintln(out)
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
	case "2", "mount", "live", "live mount":
		cfg.Mode = modeMount
	default:
		fmt.Fprintln(out, "  "+clr(ansiYellow, "Unknown choice ")+clr(ansiBold, choice)+clr(ansiDim, "; keeping ")+clr(ansiBold, current))
		fmt.Fprintln(out)
	}
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
