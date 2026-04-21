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
	case "login", "lo":
		if err := cmdLogin(args[1:]); err != nil {
			fatal(err)
		}
	case "logout":
		if err := cmdLogout(args[1:]); err != nil {
			fatal(err)
		}
	case "setup", "se":
		if err := cmdSetup(); err != nil {
			fatal(err)
		}
	case "config", "co":
		if err := cmdConfig(args); err != nil {
			fatal(err)
		}
	case "onboard", "ob":
		// Deprecated alias; forwards to login with a one-line notice.
		if err := cmdOnboard(args[1:]); err != nil {
			fatal(err)
		}
	case "auth", "au":
		// Deprecated group; forwards each subcommand to its top-level equivalent.
		if err := cmdAuth(args); err != nil {
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
	case "database", "db":
		if err := cmdDatabase(args); err != nil {
			fatal(err)
		}
	case "checkpoint", "c", "ch":
		if err := cmdCheckpoint(args); err != nil {
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
	fmt.Fprintf(os.Stderr, "\n🗂  Agent Filesystem %s — Syncable, checkpointed workspaces for AI agents.\n\n", version.String())
	fmt.Fprintf(os.Stderr, `Usage: %s [options] [command]

Options:
  --config <path>      Override afs.config.json path
  -h, --help           Display help for command
  -V, --version        Output the version number

Commands:
  Hint: commands suffixed with * have subcommands. Run <command> --help for details.

  login                Connect this CLI to a control plane (cloud or self-hosted)
  logout               Drop the cloud login and return to local-only mode
  setup                Interactive setup of workspace and local paths
  config *             Non-interactive config helpers (get/set/list/unset)
  up [flags]           Start sync/mount services for the current workspace
  down                 Stop and unmount
  status               Show connection, workspace, and sync status
  workspace *          Workspace ops (create, list, use, clone, fork, delete, import)
  database *           Database ops (list, use)
  checkpoint *         Checkpoint ops (create, list, restore)
  grep [flags] <pat>   Search a workspace directly in Redis
  mcp                  Start the workspace-first MCP server over stdio

Examples:
  %s login
    Sign in to AFS Cloud via browser.
  %s login --self-hosted
    Point this CLI at %s (override with --url).
  %s setup
    Guided workspace setup for a fresh install.
  %s up
    Start syncing the current workspace.

Config: %s
`, bin, bin, bin, defaultSelfHostedControlPlaneURL, bin, bin, compactDisplayPath(configPath()))
}
