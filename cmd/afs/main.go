package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
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
	case "setup", "se":
		if err := cmdSetup(); err != nil {
			fatal(err)
		}
	case "config", "co":
		if err := cmdConfig(args); err != nil {
			fatal(err)
		}
	case "auth", "au":
		if err := cmdAuth(args); err != nil {
			fatal(err)
		}
	case "login", "lo":
		if err := cmdOnboard(args[1:]); err != nil {
			fatal(err)
		}
	case "onboard", "ob":
		if err := cmdOnboard(args[1:]); err != nil {
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
  config ...           Show or change basic Redis and local surface settings
  auth ...             Log into or out of a hosted control plane
  onboard              Preferred browser-first AFS Cloud onboarding
  login                Alias for 'onboard'
  up [flags]           Start AFS services with optional one-shot overrides
  down                 Stop and unmount
  status               Show current status
  grep [flags] <pat>   Search a workspace directly in Redis
  mcp                  Start the workspace-first MCP server over stdio
  workspace ...        Workspace operations (create, list, current, use, clone, fork, delete, import)
  database ...         Database operations (list, use)
  checkpoint ...       Checkpoint operations (create, list, restore)

Config: %s
`, bin, compactDisplayPath(configPath()))
}
