package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

func cmdVersioning(args []string) error {
	if len(args) < 2 || isHelpArg(args[1]) {
		fmt.Fprint(os.Stderr, versioningUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	switch args[1] {
	case "get":
		return cmdVersioningGet(args)
	case "set":
		return cmdVersioningSet(args)
	default:
		return fmt.Errorf("unknown versioning subcommand %q\n\n%s", args[1], versioningUsageText(filepath.Base(os.Args[0])))
	}
}

func cmdVersioningGet(args []string) error {
	if len(args) > 2 && isHelpArg(args[2]) {
		fmt.Fprint(os.Stderr, versioningGetUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 2 && len(args) != 3 {
		return fmt.Errorf("%s", versioningGetUsageText(filepath.Base(os.Args[0])))
	}

	cfg, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	workspace := ""
	if len(args) == 3 {
		workspace = args[2]
	}
	selection, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, service, workspace)
	if err != nil {
		return err
	}
	policy, err := service.GetWorkspaceVersioningPolicy(context.Background(), selection.ID)
	if err != nil {
		return err
	}

	rows := []outputRow{
		{Label: "workspace", Value: selection.Name},
		{Label: "mode", Value: displayPolicyMode(policy.Mode)},
		{Label: "include globs", Value: displayGlobList(policy.IncludeGlobs)},
		{Label: "exclude globs", Value: displayGlobList(policy.ExcludeGlobs)},
		{Label: "max versions/file", Value: displayOptionalInt(policy.MaxVersionsPerFile)},
		{Label: "max age days", Value: displayOptionalInt(policy.MaxAgeDays)},
		{Label: "max total bytes", Value: displayOptionalInt64(policy.MaxTotalBytes)},
		{Label: "large file cutoff", Value: displayOptionalInt64(policy.LargeFileCutoffBytes)},
	}
	printSection(clr(ansiBold, "workspace versioning"), rows)
	return nil
}

type versioningSetArgs struct {
	workspace            string
	mode                 string
	includeGlobs         []string
	excludeGlobs         []string
	maxVersionsPerFile   *int
	maxAgeDays           *int
	maxTotalBytes        *int64
	largeFileCutoffBytes *int64
}

func cmdVersioningSet(args []string) error {
	for _, arg := range args[2:] {
		if isHelpArg(arg) {
			fmt.Fprint(os.Stderr, versioningSetUsageText(filepath.Base(os.Args[0])))
			return nil
		}
	}

	parsed, err := parseVersioningSetArgs(args[2:])
	if err != nil {
		return fmt.Errorf("%s\n\n%s", err, versioningSetUsageText(filepath.Base(os.Args[0])))
	}

	cfg, service, closeStore, err := openAFSControlPlane(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	selection, err := resolveWorkspaceSelectionFromControlPlane(context.Background(), cfg, service, parsed.workspace)
	if err != nil {
		return err
	}
	current, err := service.GetWorkspaceVersioningPolicy(context.Background(), selection.ID)
	if err != nil {
		return err
	}

	next := current
	if parsed.mode != "" {
		next.Mode = parsed.mode
	}
	if parsed.includeGlobs != nil {
		next.IncludeGlobs = parsed.includeGlobs
	}
	if parsed.excludeGlobs != nil {
		next.ExcludeGlobs = parsed.excludeGlobs
	}
	if parsed.maxVersionsPerFile != nil {
		next.MaxVersionsPerFile = *parsed.maxVersionsPerFile
	}
	if parsed.maxAgeDays != nil {
		next.MaxAgeDays = *parsed.maxAgeDays
	}
	if parsed.maxTotalBytes != nil {
		next.MaxTotalBytes = *parsed.maxTotalBytes
	}
	if parsed.largeFileCutoffBytes != nil {
		next.LargeFileCutoffBytes = *parsed.largeFileCutoffBytes
	}

	updated, err := service.UpdateWorkspaceVersioningPolicy(context.Background(), selection.ID, next)
	if err != nil {
		return err
	}

	printSection(markerSuccess+" "+clr(ansiBold, "workspace versioning updated"), []outputRow{
		{Label: "workspace", Value: selection.Name},
		{Label: "mode", Value: updated.Mode},
		{Label: "include globs", Value: displayGlobList(updated.IncludeGlobs)},
		{Label: "exclude globs", Value: displayGlobList(updated.ExcludeGlobs)},
		{Label: "max versions/file", Value: displayOptionalInt(updated.MaxVersionsPerFile)},
		{Label: "max age days", Value: displayOptionalInt(updated.MaxAgeDays)},
		{Label: "max total bytes", Value: displayOptionalInt64(updated.MaxTotalBytes)},
		{Label: "large file cutoff", Value: displayOptionalInt64(updated.LargeFileCutoffBytes)},
	})
	return nil
}

func parseVersioningSetArgs(args []string) (versioningSetArgs, error) {
	var parsed versioningSetArgs
	var workspaceArgs []string

	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--mode":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--mode requires a value")
			}
			index++
			parsed.mode = strings.TrimSpace(args[index])
		case strings.HasPrefix(arg, "--mode="):
			parsed.mode = strings.TrimSpace(strings.TrimPrefix(arg, "--mode="))
		case arg == "--include":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--include requires a value")
			}
			index++
			parsed.includeGlobs = append(parsed.includeGlobs, args[index])
		case strings.HasPrefix(arg, "--include="):
			parsed.includeGlobs = append(parsed.includeGlobs, strings.TrimPrefix(arg, "--include="))
		case arg == "--exclude":
			if index+1 >= len(args) {
				return parsed, fmt.Errorf("--exclude requires a value")
			}
			index++
			parsed.excludeGlobs = append(parsed.excludeGlobs, args[index])
		case strings.HasPrefix(arg, "--exclude="):
			parsed.excludeGlobs = append(parsed.excludeGlobs, strings.TrimPrefix(arg, "--exclude="))
		case arg == "--max-versions-per-file":
			value, nextIndex, err := parseOptionalIntFlag(args, index, arg)
			if err != nil {
				return parsed, err
			}
			index = nextIndex
			parsed.maxVersionsPerFile = &value
		case strings.HasPrefix(arg, "--max-versions-per-file="):
			value, err := strconv.Atoi(strings.TrimPrefix(arg, "--max-versions-per-file="))
			if err != nil {
				return parsed, fmt.Errorf("--max-versions-per-file must be an integer")
			}
			parsed.maxVersionsPerFile = &value
		case arg == "--max-age-days":
			value, nextIndex, err := parseOptionalIntFlag(args, index, arg)
			if err != nil {
				return parsed, err
			}
			index = nextIndex
			parsed.maxAgeDays = &value
		case strings.HasPrefix(arg, "--max-age-days="):
			value, err := strconv.Atoi(strings.TrimPrefix(arg, "--max-age-days="))
			if err != nil {
				return parsed, fmt.Errorf("--max-age-days must be an integer")
			}
			parsed.maxAgeDays = &value
		case arg == "--max-total-bytes":
			value, nextIndex, err := parseOptionalInt64Flag(args, index, arg)
			if err != nil {
				return parsed, err
			}
			index = nextIndex
			parsed.maxTotalBytes = &value
		case strings.HasPrefix(arg, "--max-total-bytes="):
			value, err := strconv.ParseInt(strings.TrimPrefix(arg, "--max-total-bytes="), 10, 64)
			if err != nil {
				return parsed, fmt.Errorf("--max-total-bytes must be an integer")
			}
			parsed.maxTotalBytes = &value
		case arg == "--large-file-cutoff-bytes":
			value, nextIndex, err := parseOptionalInt64Flag(args, index, arg)
			if err != nil {
				return parsed, err
			}
			index = nextIndex
			parsed.largeFileCutoffBytes = &value
		case strings.HasPrefix(arg, "--large-file-cutoff-bytes="):
			value, err := strconv.ParseInt(strings.TrimPrefix(arg, "--large-file-cutoff-bytes="), 10, 64)
			if err != nil {
				return parsed, fmt.Errorf("--large-file-cutoff-bytes must be an integer")
			}
			parsed.largeFileCutoffBytes = &value
		case strings.HasPrefix(arg, "--"):
			return parsed, fmt.Errorf("unknown flag %q", arg)
		default:
			workspaceArgs = append(workspaceArgs, arg)
		}
	}

	if len(workspaceArgs) > 1 {
		return parsed, fmt.Errorf("expected at most one workspace argument, got %d", len(workspaceArgs))
	}
	if len(workspaceArgs) == 1 {
		parsed.workspace = workspaceArgs[0]
	}
	return parsed, nil
}

func parseOptionalIntFlag(args []string, index int, flag string) (int, int, error) {
	if index+1 >= len(args) {
		return 0, index, fmt.Errorf("%s requires a value", flag)
	}
	value, err := strconv.Atoi(args[index+1])
	if err != nil {
		return 0, index, fmt.Errorf("%s must be an integer", flag)
	}
	return value, index + 1, nil
}

func parseOptionalInt64Flag(args []string, index int, flag string) (int64, int, error) {
	if index+1 >= len(args) {
		return 0, index, fmt.Errorf("%s requires a value", flag)
	}
	value, err := strconv.ParseInt(args[index+1], 10, 64)
	if err != nil {
		return 0, index, fmt.Errorf("%s must be an integer", flag)
	}
	return value, index + 1, nil
}

func displayGlobList(globs []string) string {
	if len(globs) == 0 {
		return "none"
	}
	return strings.Join(globs, ", ")
}

func displayPolicyMode(mode string) string {
	if strings.TrimSpace(mode) == "" {
		return controlplane.WorkspaceVersioningModeOff
	}
	return mode
}

func displayOptionalInt(value int) string {
	if value == 0 {
		return "unset"
	}
	return strconv.Itoa(value)
}

func displayOptionalInt64(value int64) string {
	if value == 0 {
		return "unset"
	}
	return strconv.FormatInt(value, 10)
}

func versioningUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s versioning <subcommand>

Subcommands:
  get [workspace]                 Show versioning policy for a workspace
  set [workspace] [flags]         Update versioning policy for a workspace

Run '%s versioning <subcommand> --help' for details.
`, bin, bin)
}

func versioningGetUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s versioning get [workspace]

Show the workspace versioning policy. If [workspace] is omitted, AFS uses the
current workspace selection.
`, bin)
}

func versioningSetUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s versioning set [workspace] [flags]

Update the workspace versioning policy. If [workspace] is omitted, AFS uses the
current workspace selection.

Flags:
  --mode <off|all|paths>
  --include <glob>                Repeat to add multiple include globs
  --exclude <glob>                Repeat to add multiple exclude globs
  --max-versions-per-file <n>
  --max-age-days <n>
  --max-total-bytes <n>
  --large-file-cutoff-bytes <n>
`, bin)
}
