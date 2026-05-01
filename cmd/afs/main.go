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
	case "auth":
		if err := cmdAuth(args); err != nil {
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
	case "status":
		if err := cmdStatusArgs(args[1:]); err != nil {
			fatal(err)
		}
	case "fs":
		if err := cmdFS(args); err != nil {
			fatal(err)
		}
	case "mcp":
		if err := cmdMCP(args); err != nil {
			fatal(err)
		}
	case "ws":
		if err := cmdWorkspace(args); err != nil {
			fatal(err)
		}
	case "database":
		if err := cmdDatabase(args); err != nil {
			fatal(err)
		}
	case "cp":
		if err := cmdCheckpoint(args); err != nil {
			fatal(err)
		}
	case "log":
		if err := cmdLog(args); err != nil {
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
		if isWorkspaceRootShortcut(args[0]) {
			if err := cmdWorkspace(workspaceRootShortcutArgs(args)); err != nil {
				fatal(err)
			}
			return
		}
		fmt.Fprint(os.Stderr, formatCLIError(fmt.Errorf("unknown command %q", args[0])))
		printUsage()
		os.Exit(1)
	}
}

func isWorkspaceRootShortcut(command string) bool {
	switch command {
	case "mount", "unmount", "create", "list", "info", "import", "fork", "delete":
		return true
	default:
		return false
	}
}

func workspaceRootShortcutArgs(args []string) []string {
	rewritten := make([]string, 0, len(args)+1)
	rewritten = append(rewritten, "ws")
	rewritten = append(rewritten, args...)
	return rewritten
}

func printUsage() {
	bin := filepath.Base(os.Args[0])
	w := os.Stderr
	dim := ansiDim
	bold := ansiBold
	orange := ansiOrange
	reset := ansiReset

	printBrandHeader(w)
	fmt.Fprintf(w, "%sUsage:%s %s [options] [command]\n\n", bold, reset, bin)

	fmt.Fprintf(w, "%sOptions:%s\n", bold, reset)
	fmt.Fprintf(w, "  %s--config <path>%s      %sOverride afs.config.json path%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %s-h, --help%s           %sDisplay help for command%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %s-V, --version%s        %sOutput the version number%s\n\n", bold, reset, dim, reset)

	fmt.Fprintf(w, "%sCommands:%s\n", bold, reset)
	fmt.Fprintf(w, "  %sstatus%s             %sShow AFS status and local workspace mounts%s\n\n", bold, reset, dim, reset)

	fmt.Fprintf(w, "  %sws%s (workspace)     %sMount, unmount, create, list, import, fork, delete%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %sfs%s (filesystem)    %sRead, search, and safely write workspace files%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %scp%s (checkpoint)    %sCreate, list, show, diff, restore%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %slog%s                %sWorkspace file-change log%s\n\n", bold, reset, dim, reset)

	fmt.Fprintf(w, "  %sauth%s               %sLogin, logout, and inspect authentication%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %ssetup%s              %sConfigure basic connection defaults%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %sconfig%s             %sGet, set, list, unset, reset config%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %sdatabase%s           %sAdvanced database operations%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %smcp%s                %sStart the MCP server%s\n\n", bold, reset, dim, reset)

	fmt.Fprintf(w, "%sExamples:%s\n", bold, reset)
	fmt.Fprintf(w, "  %s%s auth login%s\n    Sign in to AFS Cloud via browser.\n", orange, bin, reset)
	fmt.Fprintf(w, "  %s%s ws mount getting-started ~/getting-started%s\n    Mount a workspace to a local folder.\n", orange, bin, reset)
	fmt.Fprintf(w, "  %s%s ws unmount getting-started%s\n    Stop managing that workspace; keep local files.\n\n", orange, bin, reset)

	fmt.Fprintf(w, "%sCommon Flows:%s\n", bold, reset)
	fmt.Fprintf(w, "  %sFresh setup:%s %s%s auth login%s → %s%s ws mount getting-started ~/getting-started%s\n", dim, reset, orange, bin, reset, orange, bin, reset)
	fmt.Fprintf(w, "  %sNew workspace:%s %s%s ws create demo%s → %s%s ws mount demo ~/demo%s\n", dim, reset, orange, bin, reset, orange, bin, reset)
	fmt.Fprintf(w, "  %sImport existing files:%s %s%s ws import --mount-at-source demo ~/src/demo%s\n\n", dim, reset, orange, bin, reset)

	fmt.Fprintf(w, "%sConfig:%s %s%s%s\n", bold, reset, dim, compactDisplayPath(configPath()), reset)
}

func setupUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s setup

Open the interactive setup flow for connection settings.
`, bin)
}

func statusUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s status [--verbose]

Show AFS daemon status, configuration, and mounted workspaces.

Flags:
  --verbose, -v   Include control-plane, session, and process details
`, bin)
}
