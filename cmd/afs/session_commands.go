package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

func cmdLog(args []string) error {
	if len(args) > 1 && isHelpArg(args[1]) {
		fmt.Fprint(os.Stderr, logUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) < 2 {
		return cmdSessionLog(nil)
	}

	switch args[1] {
	case "summary":
		return cmdSessionSummary(args[2:])
	default:
		return cmdSessionLog(args[1:])
	}
}

func cmdSession(args []string) error {
	if len(args) < 2 || isHelpArg(args[1]) {
		fmt.Fprint(os.Stderr, sessionUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	switch args[1] {
	case "log":
		return cmdSessionLog(args[2:])
	case "summary":
		return cmdSessionSummary(args[2:])
	default:
		return fmt.Errorf("unknown session subcommand %q\n\n%s", args[1], sessionUsageText(filepath.Base(os.Args[0])))
	}
}

type sessionLogFlags struct {
	workspace       string
	sessionID       string
	sessionExplicit bool
	limit           int
	follow          bool
	all             bool
}

func parseSessionLogArgs(args []string) (sessionLogFlags, error) {
	flags := sessionLogFlags{limit: 50}
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--all":
			flags.all = true
		case a == "--follow", a == "-f":
			flags.follow = true
		case a == "--limit":
			if i+1 >= len(args) {
				return flags, fmt.Errorf("--limit requires a value")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				return flags, fmt.Errorf("--limit must be a positive integer")
			}
			flags.limit = n
		case strings.HasPrefix(a, "--limit="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--limit="))
			if err != nil || n <= 0 {
				return flags, fmt.Errorf("--limit must be a positive integer")
			}
			flags.limit = n
		case a == "--workspace", a == "-w":
			if i+1 >= len(args) {
				return flags, fmt.Errorf("--workspace requires a value")
			}
			i++
			flags.workspace = args[i]
		case strings.HasPrefix(a, "--workspace="):
			flags.workspace = strings.TrimPrefix(a, "--workspace=")
		case strings.HasPrefix(a, "-"):
			return flags, fmt.Errorf("unknown flag %q", a)
		default:
			positionals = append(positionals, a)
		}
	}
	if len(positionals) > 1 {
		return flags, fmt.Errorf("expected at most one session id, got %d", len(positionals))
	}
	if len(positionals) == 1 {
		flags.sessionID = positionals[0]
		flags.sessionExplicit = true
	}
	return flags, nil
}

func cmdSessionLog(args []string) error {
	for _, a := range args {
		if isHelpArg(a) {
			fmt.Fprint(os.Stderr, sessionLogUsageText(filepath.Base(os.Args[0])))
			return nil
		}
	}

	flags, err := parseSessionLogArgs(args)
	if err != nil {
		return fmt.Errorf("%s\n\n%s", err, sessionLogUsageText(filepath.Base(os.Args[0])))
	}

	ctx := context.Background()
	cfg, service, closeFn, err := openAFSControlPlane(ctx)
	if err != nil {
		return err
	}
	defer closeFn()

	selection, err := resolveCommandWorkspaceSelectionFromControlPlane(ctx, cfg, service, flags.workspace)
	if err != nil {
		return err
	}
	if flags.sessionID == "" && !flags.all {
		flags.sessionID = defaultLogSessionID(selection)
	}

	if flags.follow {
		return followChangelog(ctx, service, selection.Name, flags)
	}

	req := controlplane.ChangelogListRequest{
		SessionID: flags.sessionID,
		Limit:     flags.limit,
		Reverse:   true,
	}
	resp, err := service.ListChangelog(ctx, selection.Name, req)
	if err != nil {
		return err
	}

	printChangelogEntries(selection.Name, flags.sessionID, flags.sessionExplicit, resp.Entries, flags.all)
	return nil
}

func followChangelog(ctx context.Context, service afsControlPlane, workspace string, flags sessionLogFlags) error {
	// Initial back-fill: newest flags.limit entries, newest-last for readable tailing.
	initial, err := service.ListChangelog(ctx, workspace, controlplane.ChangelogListRequest{
		SessionID: flags.sessionID,
		Limit:     flags.limit,
		Reverse:   true,
	})
	if err != nil {
		return err
	}
	// Reverse-sort back to chronological order so the tail reads naturally.
	entries := reverseEntries(initial.Entries)
	printChangelogEntries(workspace, flags.sessionID, flags.sessionExplicit, entries, flags.all)

	var cursor string
	if len(entries) > 0 {
		cursor = entries[len(entries)-1].ID
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			resp, err := service.ListChangelog(ctx, workspace, controlplane.ChangelogListRequest{
				SessionID: flags.sessionID,
				Since:     cursor,
				Limit:     flags.limit,
			})
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				fmt.Fprintf(os.Stderr, "afs log: refresh failed: %v\n", err)
				continue
			}
			if len(resp.Entries) == 0 {
				continue
			}
			// /changes with a Since cursor is inclusive of the cursor ID;
			// skip the duplicate head.
			fresh := resp.Entries
			if cursor != "" && len(fresh) > 0 && fresh[0].ID == cursor {
				fresh = fresh[1:]
			}
			for _, entry := range fresh {
				printChangelogRow(entry, flags.all)
				cursor = entry.ID
			}
		}
	}
}

func reverseEntries(in []controlplane.ChangelogEntryRow) []controlplane.ChangelogEntryRow {
	out := make([]controlplane.ChangelogEntryRow, len(in))
	for i := range in {
		out[len(in)-1-i] = in[i]
	}
	return out
}

func printChangelogEntries(workspace, sessionID string, sessionExplicit bool, entries []controlplane.ChangelogEntryRow, showSession bool) {
	fmt.Println()
	fmt.Printf("workspace: %s\n", workspace)
	if sessionExplicit && strings.TrimSpace(sessionID) != "" {
		fmt.Printf("session: %s\n", strings.TrimSpace(sessionID))
	}
	fmt.Println()
	if len(entries) == 0 {
		fmt.Println(clr(ansiDim, "(no changes recorded)"))
		fmt.Println()
		return
	}
	for _, row := range entries {
		printChangelogRow(row, showSession)
	}
	fmt.Println()
}

func printChangelogRow(row controlplane.ChangelogEntryRow, showSession bool) {
	when := row.OccurredAt
	if when == "" {
		when = row.ID
	} else if t, err := time.Parse(time.RFC3339, when); err == nil {
		when = t.Local().Format("15:04:05")
	}
	delta := formatDeltaBytes(row.DeltaBytes)
	path := row.Path
	if row.PrevPath != "" {
		path = row.PrevPath + " → " + path
	}
	prefix := fmt.Sprintf("%s  %s  %s  %s",
		clr(ansiDim, when),
		colorForChangelogOp(row),
		clr(ansiDim, delta),
		path,
	)
	if showSession && row.SessionID != "" {
		prefix += clr(ansiDim, "  session "+row.SessionID)
	}
	fmt.Println(prefix)
}

const changelogOpWidth = 13

func changelogDisplayOp(row controlplane.ChangelogEntryRow) string {
	switch row.Op {
	case "put":
		if changelogRowHasPrevious(row) {
			return "Update"
		}
		return "Create"
	case "delete":
		return "Delete"
	case "mkdir":
		return "Create folder"
	case "rmdir":
		return "Delete folder"
	case "symlink":
		if changelogRowHasPrevious(row) {
			return "Update link"
		}
		return "Create link"
	case "chmod":
		return "Change mode"
	case "":
		return "?"
	default:
		return row.Op
	}
}

func changelogRowHasPrevious(row controlplane.ChangelogEntryRow) bool {
	return strings.TrimSpace(row.PrevHash) != "" || strings.TrimSpace(row.PrevPath) != ""
}

func colorForChangelogOp(row controlplane.ChangelogEntryRow) string {
	label := padLeft(changelogDisplayOp(row), changelogOpWidth)
	switch row.Op {
	case "put", "mkdir", "symlink":
		return clr(ansiBold+ansiGreen, label)
	case "delete", "rmdir":
		return clr(ansiBold+ansiOrange, label)
	case "chmod":
		return clr(ansiBold, label)
	default:
		return label
	}
}

func padLeft(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return strings.Repeat(" ", n-len(s)) + s
}

func defaultLogSessionID(selection workspaceSelection) string {
	if sessionID := mountSessionIDForWorkspace(selection); sessionID != "" {
		return sessionID
	}
	st, err := loadState()
	if err != nil {
		return ""
	}
	if selection.Name != "" && strings.TrimSpace(st.CurrentWorkspace) != "" && strings.TrimSpace(st.CurrentWorkspace) != selection.Name {
		return ""
	}
	if selection.ID != "" && strings.TrimSpace(st.CurrentWorkspaceID) != "" && strings.TrimSpace(st.CurrentWorkspaceID) != selection.ID {
		return ""
	}
	return strings.TrimSpace(st.SessionID)
}

func mountSessionIDForWorkspace(selection workspaceSelection) string {
	reg, err := loadMountRegistry()
	if err != nil {
		return ""
	}
	var matches []string
	for _, rec := range reg.Mounts {
		if strings.TrimSpace(rec.SessionID) == "" {
			continue
		}
		if selection.ID != "" && strings.TrimSpace(rec.WorkspaceID) == selection.ID {
			matches = append(matches, strings.TrimSpace(rec.SessionID))
			continue
		}
		if selection.Name != "" && strings.TrimSpace(rec.Workspace) == selection.Name {
			matches = append(matches, strings.TrimSpace(rec.SessionID))
		}
	}
	if len(matches) != 1 {
		return ""
	}
	return matches[0]
}

func formatDeltaBytes(delta int64) string {
	if delta == 0 {
		return "      ·"
	}
	sign := "+"
	val := delta
	if delta < 0 {
		sign = "−"
		val = -delta
	}
	switch {
	case val < 1024:
		return fmt.Sprintf("%s%4dB ", sign, val)
	case val < 1024*1024:
		return fmt.Sprintf("%s%4dK ", sign, val/1024)
	case val < 1024*1024*1024:
		return fmt.Sprintf("%s%4dM ", sign, val/(1024*1024))
	default:
		return fmt.Sprintf("%s%4dG ", sign, val/(1024*1024*1024))
	}
}

func cmdSessionSummary(args []string) error {
	for _, a := range args {
		if isHelpArg(a) {
			fmt.Fprint(os.Stderr, sessionSummaryUsageText(filepath.Base(os.Args[0])))
			return nil
		}
	}

	// Summary endpoint is only implemented server-side; CLI reads the changelog
	// and rolls up locally. Accepts [session-id] [--workspace <name>].
	flags, err := parseSessionLogArgs(args)
	if err != nil {
		return fmt.Errorf("%s\n\n%s", err, sessionSummaryUsageText(filepath.Base(os.Args[0])))
	}

	ctx := context.Background()
	cfg, service, closeFn, err := openAFSControlPlane(ctx)
	if err != nil {
		return err
	}
	defer closeFn()

	selection, err := resolveCommandWorkspaceSelectionFromControlPlane(ctx, cfg, service, flags.workspace)
	if err != nil {
		return err
	}
	if flags.sessionID == "" {
		flags.sessionID = defaultLogSessionID(selection)
	}
	if flags.sessionID == "" {
		return fmt.Errorf("no active session; pass a session id explicitly or mount a workspace first")
	}

	resp, err := service.ListChangelog(ctx, selection.Name, controlplane.ChangelogListRequest{
		SessionID: flags.sessionID,
		Limit:     1000,
	})
	if err != nil {
		return err
	}

	counts := map[string]int{}
	var added, removed int64
	for _, row := range resp.Entries {
		counts[changelogDisplayOp(row)]++
		if row.DeltaBytes > 0 {
			added += row.DeltaBytes
		} else if row.DeltaBytes < 0 {
			removed += -row.DeltaBytes
		}
	}

	rows := []outputRow{
		{Label: "workspace", Value: selection.Name},
		{Label: "session", Value: flags.sessionID},
		{Label: "entries", Value: strconv.Itoa(len(resp.Entries))},
	}
	seenLabels := map[string]struct{}{}
	for _, label := range []string{"Create", "Update", "Delete", "Create folder", "Delete folder", "Create link", "Update link", "Change mode"} {
		if n, ok := counts[label]; ok {
			rows = append(rows, outputRow{Label: strings.ToLower(label), Value: strconv.Itoa(n)})
			seenLabels[label] = struct{}{}
		}
	}
	extraLabels := make([]string, 0)
	for label := range counts {
		if _, ok := seenLabels[label]; !ok {
			extraLabels = append(extraLabels, label)
		}
	}
	sort.Strings(extraLabels)
	for _, label := range extraLabels {
		rows = append(rows, outputRow{Label: strings.ToLower(label), Value: strconv.Itoa(counts[label])})
	}
	rows = append(rows, outputRow{Label: "bytes added", Value: formatBytes(added)})
	rows = append(rows, outputRow{Label: "bytes removed", Value: formatBytes(removed)})
	printSection(clr(ansiBold, "log summary"), rows)
	return nil
}

func logUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s log [session-id] [flags]
  %s log summary [session-id] [flags]

Subcommands:
  summary [session-id]       Totals for a session (files + bytes)

Flags:
  --workspace, -w <name>     Read log entries for a specific workspace
  --limit <n>                Number of recent entries to show (default 50)
  --follow, -f               Stream new entries as they arrive
  --all                      Show every session (not just the current one)

Examples:
  %s log
  %s log --follow
  %s log <session-id>
  %s log summary <session-id>
`, bin, bin, bin, bin, bin, bin)
}

func sessionUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s session <subcommand>

Subcommands:
  log [session-id] [flags]     Show file-change history for a session
  summary [session-id] [flags] Show per-session totals

Run '%s session <subcommand> --help' for details.
`, bin, bin)
}

func sessionLogUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s log [session-id] [flags]

Show file-change history for an AFS session. Defaults to the session for the
mounted workspace on this machine.

Flags:
  --workspace, -w <name>   Read log entries for a specific workspace
  --limit <n>              Number of recent entries to show (default 50)
  --follow, -f             Stream new entries every 2 seconds
  --all                    Include entries from other sessions
`, bin)
}

func sessionSummaryUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s log summary [session-id] [flags]

Show per-session totals (file counts by op, bytes added/removed).
Defaults to the session for the mounted workspace on this machine.

Flags:
  --workspace, -w <name>   Summarize a specific workspace
`, bin)
}
