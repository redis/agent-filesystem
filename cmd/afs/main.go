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
	case "--skill":
		if err := cmdSkill([]string{"skill", "show"}); err != nil {
			fatal(err)
		}
	case "auth":
		if err := cmdAuth(args); err != nil {
			fatal(err)
		}
	case "tokens":
		if err := cmdTokens(args); err != nil {
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
	case "daemon":
		if err := cmdDaemon(args); err != nil {
			fatal(err)
		}
	case "vol":
		if err := cmdVolume(args); err != nil {
			fatal(err)
		}
	case "fs":
		if err := cmdFS(args); err != nil {
			fatal(err)
		}
	case "grep":
		if err := cmdFSGrep("", args[1:]); err != nil {
			fatal(err)
		}
	case "query":
		if err := cmdQuery(args); err != nil {
			fatal(err)
		}
	case "mcp":
		if err := cmdMCP(args); err != nil {
			fatal(err)
		}
	case "skill":
		if err := cmdSkill(args); err != nil {
			fatal(err)
		}
	case "ws":
		if err := cmdWorkspace(args); err != nil {
			fatal(err)
		}
	case "mount":
		if err := cmdRootMountArgs(args[1:]); err != nil {
			fatal(err)
		}
	case "unmount":
		if err := cmdRootUnmountArgs(args[1:]); err != nil {
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
		if isVolumeRootShortcut(args[0]) {
			if err := cmdVolume(volumeRootShortcutArgs(args)); err != nil {
				fatal(err)
			}
			return
		}
		fmt.Fprint(os.Stderr, formatCLIError(fmt.Errorf("unknown command %q", args[0])))
		printUsage()
		os.Exit(1)
	}
}

func isVolumeRootShortcut(command string) bool {
	switch command {
	case "create", "list", "clone", "default",
		"set-default", "unset-default", "info", "import", "fork",
		"delete":
		return true
	default:
		return false
	}
}

func volumeRootShortcutArgs(args []string) []string {
	rewritten := make([]string, 0, len(args)+1)
	rewritten = append(rewritten, "vol")
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
	fmt.Fprintf(w, "  %sstatus%s             %sshow AFS status and local mounts%s\n\n", bold, reset, dim, reset)

	fmt.Fprintf(w, "  %sws%s (workspace)     %smount, create and manage workspaces%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %sfs%s (filesystem)    %sread, search, and safely write workspace files%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %scp%s (checkpoint)    %screate, list, show, diff, restore%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %svol%s (volume) - %sdirect support for underlying volumes - create, import, list, etc.%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %slog%s                %sWorkspace file-change log%s\n\n", bold, reset, dim, reset)

	fmt.Fprintf(w, "  %sauth%s               %slogin, logout, and inspect authentication%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %stokens%s             %screate scoped CLI access tokens%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %ssetup%s              %sconfigure the default local mode%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %sconfig%s             %sget, set, list, unset, reset config%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %sdatabase%s           %sadvanced database operations%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %sdaemon%s             %sstatus and stop local AFS mount sessions%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %smcp%s                %sstart the MCP server%s\n", bold, reset, dim, reset)
	fmt.Fprintf(w, "  %sskill%s              %sshow or install the packaged AFS skill%s\n\n", bold, reset, dim, reset)

	fmt.Fprintf(w, "%sAgent Workspace Shortcuts:%s\n", bold, reset)
	fmt.Fprintf(w, "  %s%s mount%s and %s%s unmount%s map to Agent Workspace manifests (%s ws mount/unmount).\n\n", orange, bin, reset, orange, bin, reset, bin)

	fmt.Fprintf(w, "%sVolume Shortcuts:%s\n", bold, reset)
	fmt.Fprintf(w, "  %sOmit \"vol\" for:%s create, list, clone, default, set-default,\n", dim, reset)
	fmt.Fprintf(w, "                 unset-default, info, import, fork, delete\n")
	fmt.Fprintf(w, "  %sExample:%s %s%s create demo%s  %s(same as %s vol create demo)%s\n\n", dim, reset, orange, bin, reset, dim, bin, reset)

	fmt.Fprintf(w, "%sFilesystem Shortcuts:%s\n", bold, reset)
	fmt.Fprintf(w, "  %sOmit \"fs\" for:%s grep, query\n", dim, reset)
	fmt.Fprintf(w, "  %sNote:%s shortcuts use the \"default\" workspace; use %s fs <workspace> <command> to choose one.\n", dim, reset, bin)
	fmt.Fprintf(w, "  %sExample:%s %s%s grep DirtyHint%s  %s(same as %s fs grep DirtyHint)%s\n\n", dim, reset, orange, bin, reset, dim, bin, reset)

	fmt.Fprintf(w, "%sExamples:%s\n", bold, reset)
	fmt.Fprintf(w, "  %s%s auth login%s\n    Sign in to AFS Cloud via browser.\n", orange, bin, reset)
	fmt.Fprintf(w, "  %s%s mount coding-agent ~/coding-agent%s\n    Mount an Agent Workspace to a local root.\n", orange, bin, reset)
	fmt.Fprintf(w, "  %s%s vol mount getting-started ~/getting-started%s\n    Mount a single volume to a local folder.\n\n", orange, bin, reset)

	fmt.Fprintf(w, "%sAI Agents:%s\n", bold, reset)
	fmt.Fprintf(w, "  - Run `%s mcp` to expose the MCP server (stdio) to agents.\n", bin)
	fmt.Fprintf(w, "  - `%s skill install` installs the AFS skill into ./.agents/skills/afs.\n", bin)
	fmt.Fprintf(w, "  - Use `%s skill install --global` for ~/.agents/skills/afs.\n", bin)
	fmt.Fprintf(w, "  - `%s --skill` is kept as an alias for `%s skill show`.\n", bin, bin)
	fmt.Fprintf(w, "  - Advanced: `%s mcp --volume <name> --profile <profile>` scopes agent access.\n\n", bin)

	fmt.Fprintf(w, "%sConfig:%s %s%s%s\n", bold, reset, dim, compactDisplayPath(configPath()), reset)
}

func setupUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s setup

Open the interactive setup flow for the default local mode.
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
