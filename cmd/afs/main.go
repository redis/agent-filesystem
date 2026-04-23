package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/redis/agent-filesystem/internal/version"
)

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
	case "login":
		if err := cmdLogin(args[1:]); err != nil {
			fatal(err)
		}
	case "logout":
		if err := cmdLogout(args[1:]); err != nil {
			fatal(err)
		}
	case "setup":
		if len(args) > 1 && isHelpArg(args[1]) {
			fmt.Fprint(os.Stderr, setupUsageText(filepath.Base(os.Args[0])))
			return
		}
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
		if len(args) > 1 && isHelpArg(args[1]) {
			fmt.Fprint(os.Stderr, downUsageText(filepath.Base(os.Args[0])))
			return
		}
		if err := cmdDown(); err != nil {
			fatal(err)
		}
	case "status":
		if len(args) > 1 && isHelpArg(args[1]) {
			fmt.Fprint(os.Stderr, statusUsageText(filepath.Base(os.Args[0])))
			return
		}
		if err := cmdStatus(); err != nil {
			fatal(err)
		}
	case "sync":
		if err := cmdSync(args); err != nil {
			fatal(err)
		}
	case "grep":
		if err := cmdGrep(args); err != nil {
			fatal(err)
		}
	case "mcp":
		if err := cmdMCP(args); err != nil {
			fatal(err)
		}
	case "workspace":
		if err := cmdWorkspace(args); err != nil {
			fatal(err)
		}
	case "database":
		if err := cmdDatabase(args); err != nil {
			fatal(err)
		}
	case "checkpoint":
		if err := cmdCheckpoint(args); err != nil {
			fatal(err)
		}
	case "session":
		if err := cmdSession(args); err != nil {
			fatal(err)
		}
	case "reset":
		if len(args) > 1 && isHelpArg(args[1]) {
			fmt.Fprint(os.Stderr, resetUsageText(filepath.Base(os.Args[0])))
			return
		}
		if err := cmdReset(); err != nil {
			fatal(err)
		}
	case "_sync-daemon":
		if err := runSyncDaemon(); err != nil {
			fatal(err)
		}
	case "_mount-session":
		if err := runMountSessionDaemon(); err != nil {
			fatal(err)
		}
	case "version", "--version", "-V":
		fmt.Fprintln(os.Stdout, "afs "+version.String())
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	bin := filepath.Base(os.Args[0])
	w := os.Stderr
	dim := ansiDim
	bold := ansiBold
	orange := ansiOrange
	reset := ansiReset

	printBrandHeader(w)
	fmt.Fprintf(w, "  %sFast Filesystem for AI Agents%s\n\n", dim, reset)

	fmt.Fprintf(w, "%sUsage:%s %s [options] [command]\n\n", bold, reset, bin)

	fmt.Fprintf(w, "%sOptions:%s\n", bold, reset)
	fmt.Fprintf(w, "  %s--config <path>%s      %sOverride afs.config.json path%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %s-h, --help%s           %sDisplay help for command%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %s-V, --version%s        %sOutput the version number%s\n\n", bold, reset, dim, reset)

	fmt.Fprintf(w, "%sCommands:%s\n", bold, reset)
	// Setup / auth
	fmt.Fprintf(w, "  %slogin%s                %sConnect this CLI to a control plane%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %slogout%s               %sDrop the cloud login; return to local-only%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %ssetup%s                %sInteractive workspace + local-path setup%s\n", bold, reset, dim, reset)
	// Lifecycle
	fmt.Fprintf(w, "  %sup%s [flags]           %sStart sync/mount for the current workspace%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %sdown%s                 %sStop and unmount%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %sstatus%s               %sShow connection, workspace, and sync status%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %ssync%s                 %sExplicit operations for a running sync daemon%s\n", bold, reset, dim, reset)
	// Data
	fmt.Fprintf(w, "  %sworkspace%s            %sWorkspace ops — create, list, use, clone, fork, delete, import%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %sdatabase%s             %sDatabase ops — list, use%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %scheckpoint%s           %sCheckpoint ops — create, list, restore%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %ssession%s              %sSession ops — log, summary%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %sgrep%s <pattern>       %sSearch a workspace in Redis%s\n", bold, reset, dim, reset)
	// Integrations
	fmt.Fprintf(w, "  %sconfig%s               %sConfig helpers — get, set, list, unset%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %sreset%s                %sReset local config and state (keeps the CLI installed)%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %smcp%s                  %sStart the workspace-first MCP server over stdio%s\n\n", bold, reset, dim, reset)

	fmt.Fprintf(w, "%sExamples:%s\n", bold, reset)
	fmt.Fprintf(w, "  %s%s login%s\n    Sign in to AFS Cloud via browser.\n", orange, bin, reset)
	fmt.Fprintf(w, "  %s%s setup%s\n    Guided workspace setup for a fresh install.\n", orange, bin, reset)
	fmt.Fprintf(w, "  %s%s up%s\n    Start syncing the current workspace.\n\n", orange, bin, reset)

	fmt.Fprintf(w, "%sCommon Flows:%s\n", bold, reset)
	fmt.Fprintf(w, "  %sFresh setup:%s %s%s login%s → %s%s setup%s → %s%s up%s\n", dim, reset, orange, bin, reset, orange, bin, reset, orange, bin, reset)
	fmt.Fprintf(w, "  %sNew workspace:%s %s%s workspace create demo%s → %s%s workspace use demo%s → %s%s up%s\n", dim, reset, orange, bin, reset, orange, bin, reset, orange, bin, reset)
	fmt.Fprintf(w, "  %sImport existing code:%s %s%s workspace import demo ~/src/demo%s → %s%s up demo ~/src/demo%s\n\n", dim, reset, orange, bin, reset, orange, bin, reset)

	fmt.Fprintf(w, "%sConfig:%s %s%s%s\n", bold, reset, dim, compactDisplayPath(configPath()), reset)
}

func setupUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s setup

Open the interactive setup flow for workspace, local path, and connection settings.
`, bin)
}

func downUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s down

Stop AFS, unmount the local surface, and clean up the active runtime state.
`, bin)
}

func statusUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s status

Show connection, workspace, and sync or mount status for this machine.
`, bin)
}

func resetUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s reset

Reset local config and runtime state, while keeping the CLI installed.
If AFS is running, this command stops it first.
`, bin)
}
