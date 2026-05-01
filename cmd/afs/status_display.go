package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// appendAuthStatusRows appends cloud-auth hint rows when action is needed.
// We intentionally drop the "signed in: yes" confirmation (it's noise — the
// cloud database in the core rows already implies success), the control-plane
// URL (duplicate of configStatusRow), and the cloud database id (already in
// the core "database" row). Only actionable rows (e.g. "needs refresh")
// survive.
func appendAuthStatusRows(rows []outputRow) []outputRow {
	bin := filepath.Base(os.Args[0])
	info, _ := authConnectionInfo(bin)
	for _, row := range info {
		switch row.Label {
		case "hint":
			rows = append(rows, row)
		case "signed in":
			if row.Value != "yes" {
				rows = append(rows, row)
			}
		}
	}
	return rows
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

func redisDatabaseLabel(addr string, db int, tls bool) string {
	scheme := "redis"
	if tls {
		scheme = "rediss"
	}
	return fmt.Sprintf("%s://%s/%d", scheme, addr, db)
}

func statusRemoteLabel(addr string, db int) string {
	return redisDatabaseLabel(addr, db, false)
}

func configRemoteLabel(cfg config) string {
	productMode, err := effectiveProductMode(cfg)
	if err == nil && productMode != productModeLocal {
		label := strings.TrimSpace(cfg.URL)
		if label == "" {
			label = "<control plane url not configured>"
		}
		if db := strings.TrimSpace(cfg.DatabaseID); db != "" {
			return fmt.Sprintf("%s (%s)", label, db)
		}
		return label + " (auto database)"
	}
	return redisDatabaseLabel(cfg.RedisAddr, cfg.RedisDB, cfg.RedisTLS)
}

func configPathLabel() string {
	return clr(ansiGray, compactDisplayPath(configPath()))
}

// appendConfigRows adds user-facing config metadata rows without overloading
// "config" to mean both the configuration source and the config file path.
func appendConfigRows(rows []outputRow, cfg config) []outputRow {
	if row := configSourceStatusRow(cfg); row.Label != "" {
		rows = append(rows, row)
	}
	return append(rows, outputRow{Label: "config file", Value: configPathLabel()})
}

func configSourceStatusRow(cfg config) outputRow {
	productMode, err := effectiveProductMode(cfg)
	if err != nil {
		return outputRow{}
	}

	if productMode == productModeLocal {
		return outputRow{Label: "config source", Value: productModeDisplayLabel(productMode)}
	}

	value := strings.TrimSpace(cfg.URL)
	if value == "" {
		value = "<control plane url not configured>"
	}
	return outputRow{Label: "control plane", Value: value}
}

func statusDisplayRows(cfg config, rows []outputRow) []outputRow {
	ordered := make([]outputRow, 0, len(rows)+2)
	if row := configSourceStatusRow(cfg); row.Label != "" {
		ordered = append(ordered, row)
	}

	var databaseRows []outputRow
	for _, row := range rows {
		if row.Label == "database" {
			databaseRows = append(databaseRows, row)
			continue
		}
		ordered = append(ordered, row)
	}
	ordered = append(ordered, outputRow{Label: "config file", Value: configPathLabel()})
	ordered = append(ordered, databaseRows...)
	return ordered
}

func commandContextRows(cfg config, workspace string) []outputRow {
	rows := make([]outputRow, 0, 2)
	if strings.TrimSpace(workspace) != "" {
		rows = append(rows, outputRow{Label: "workspace", Value: workspace})
	}
	rows = append(rows, outputRow{Label: "database", Value: configRemoteLabel(cfg)})
	return rows
}

func statusTitle(prefix string, pid int) string {

	if pid > 0 {
		return prefix + " " + clr(ansiBold, fmt.Sprintf("AFS Running (pid %d)", pid))
	}
	return prefix + " " + clr(ansiBold, "AFS Running")
}

func localSurfacePath(cfg config) string {
	return cfg.LocalPath
}

// statusRows returns the consistent core rows. Sync mode no longer reports
// saved workspace/local values because attachments are the source of truth.
// Mount backend is included only for FUSE/NFS. In cloud mode the database row
// shows the cloud database id instead of the local Redis endpoint so users see
// the database they're actually talking to.
func statusRows(cfg config, workspace, localPath, mode, backendName, redisAddr string, redisDB int) []outputRow {
	var rows []outputRow
	if mode != modeSync {
		if ws := strings.TrimSpace(workspace); ws != "" {
			rows = append(rows, outputRow{Label: "workspace", Value: ws})
		}
		if localPath != "" {
			rows = append(rows, outputRow{Label: "local", Value: localPath})
		}
	}
	rows = append(rows, outputRow{Label: "database", Value: databaseStatusLabel(cfg, redisAddr, redisDB)})
	rows = append(rows, outputRow{Label: "mode", Value: mode})
	if backendName != "" && backendName != mountBackendNone {
		rows = append(rows, outputRow{Label: "mount backend", Value: userModeLabel(backendName)})
	}
	if account := strings.TrimSpace(cfg.Account); account != "" {
		if productMode, _ := effectiveProductMode(cfg); productMode == productModeCloud {
			rows = append(rows, outputRow{Label: "account", Value: account})
		}
	}
	return rows
}

// databaseStatusLabel picks the right label for the "database" row: in cloud
// mode the friendly database id (e.g. "afs-cloud"), otherwise the local
// Redis endpoint.
func databaseStatusLabel(cfg config, redisAddr string, redisDB int) string {
	productMode, _ := effectiveProductMode(cfg)
	if productMode == productModeCloud {
		if id := strings.TrimSpace(cfg.DatabaseID); id != "" {
			return id
		}
	}
	return statusRemoteLabel(redisAddr, redisDB)
}

// statusTitleForAlive returns a green-check or yellow-circle title with an
// optional daemon PID.
func statusTitleForAlive(alive bool, pid int) string {
	if alive {
		return statusTitle(markerSuccess, pid)
	}
	return statusTitle(clr(ansiYellow, "○"), 0)
}

// appendUptimeRows appends the uptime row and, if set, the readonly row.
func appendUptimeRows(rows []outputRow, st state) []outputRow {
	rows = append(rows, outputRow{Label: "uptime", Value: formatDuration(time.Since(st.StartedAt))})
	if st.ReadOnly {
		rows = append(rows, outputRow{Label: "readonly", Value: "yes"})
	}
	return rows
}

func appendConnectedAgentRows(rows []outputRow, cfg config, st state) []outputRow {
	if strings.TrimSpace(st.SessionID) == "" {
		return rows
	}
	if id := strings.TrimSpace(cfg.ID); id != "" {
		rows = append(rows, outputRow{Label: "agent id", Value: id})
	}
	return rows
}

// loadSyncStateForStatus loads the sync snapshot for a workspace, returning
// nil if the workspace is empty or the state cannot be loaded.
func loadSyncStateForStatus(workspace string) *SyncState {
	if workspace == "" {
		return nil
	}
	st, err := loadSyncState(workspace)
	if err != nil {
		return nil
	}
	return st
}

func currentWorkspaceLabel(workspace string) string {
	if strings.TrimSpace(workspace) == "" {
		return "none"
	}
	return workspace
}

// cmdStatus dispatches to the status renderer for the current local runtime:
// no state, sync attachment, or live mount.
type statusOptions struct {
	verbose bool
}

func cmdStatusArgs(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, statusUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	opts, err := parseStatusOptions(args)
	if err != nil {
		return err
	}
	return cmdStatusWithOptions(opts)
}

func parseStatusOptions(args []string) (statusOptions, error) {
	var opts statusOptions
	for _, arg := range args {
		switch arg {
		case "--verbose", "-v":
			opts.verbose = true
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, fmt.Errorf("unknown flag %q\n\n%s", arg, statusUsageText(filepath.Base(os.Args[0])))
			}
			return opts, fmt.Errorf("%s", statusUsageText(filepath.Base(os.Args[0])))
		}
	}
	return opts, nil
}

func cmdStatus() error {
	return cmdStatusWithOptions(statusOptions{})
}

func cmdStatusWithOptions(opts statusOptions) error {
	reg, regErr := loadAttachmentRegistry()
	st, err := loadState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := cmdStatusNotRunning(); err != nil {
				return err
			}
		} else {
			return err
		}
	} else if strings.TrimSpace(st.Mode) == modeSync {
		cmdStatusSync(st)
	} else if err := cmdStatusMount(st); err != nil {
		return err
	}

	if regErr == nil && len(reg.Attachments) > 0 {
		printAttachmentStatus(reg, opts.verbose)
	}
	return nil
}

func printAttachmentStatus(reg attachmentRegistry, verbose bool) {
	if len(reg.Attachments) == 0 {
		fmt.Println()
		fmt.Println("not attached")
		fmt.Println()
		return
	}
	attachments := sortedAttachmentRecords(reg.Attachments)
	fmt.Println()
	fmt.Println("Attached workspaces")
	fmt.Println()
	printPlainTable([]string{"Workspace", "Status", "Mode", "Path"}, attachmentSummaryRows(attachments))
	if !verbose {
		fmt.Println()
		return
	}
	for _, rec := range attachments {
		fmt.Println()
		printAttachmentVerbose(rec)
	}
	fmt.Println()
}

func sortedAttachmentRecords(records []attachmentRecord) []attachmentRecord {
	attachments := append([]attachmentRecord(nil), records...)
	sort.SliceStable(attachments, func(i, j int) bool {
		left := strings.ToLower(attachments[i].Workspace)
		right := strings.ToLower(attachments[j].Workspace)
		if left == right {
			return attachments[i].LocalPath < attachments[j].LocalPath
		}
		return left < right
	})
	return attachments
}

func attachmentSummaryRows(attachments []attachmentRecord) [][]string {
	rows := make([][]string, 0, len(attachments))
	for _, rec := range attachments {
		mode := strings.TrimSpace(rec.Mode)
		if mode == "" {
			mode = "unknown"
		}
		rows = append(rows, []string{rec.Workspace, attachmentStatus(rec), mode, rec.LocalPath})
	}
	return rows
}

func printAttachmentVerbose(rec attachmentRecord) {
	rows := []outputRow{
		{Label: "workspace", Value: rec.Workspace},
		{Label: "status", Value: attachmentStatus(rec)},
		{Label: "mode", Value: fallbackString(rec.Mode, "unknown")},
		{Label: "path", Value: rec.LocalPath},
	}
	if rec.PID > 0 {
		rows = append(rows, outputRow{Label: "pid", Value: fmt.Sprintf("%d", rec.PID)})
	}
	productMode := strings.TrimSpace(rec.ProductMode)
	if productMode != "" {
		rows = append(rows, outputRow{Label: "config source", Value: productModeDisplayLabel(productMode)})
	}
	if controlPlaneURL := strings.TrimSpace(rec.ControlPlaneURL); controlPlaneURL != "" {
		rows = append(rows, outputRow{Label: "control plane", Value: controlPlaneURL})
		if db := strings.TrimSpace(rec.ControlPlaneDatabase); db != "" {
			rows = append(rows, outputRow{Label: "database", Value: db})
		}
	} else if redisAddr := strings.TrimSpace(rec.RedisAddr); redisAddr != "" {
		rows = append(rows, outputRow{Label: "redis", Value: redisDatabaseLabel(redisAddr, rec.RedisDB, false)})
	}
	if sessionID := strings.TrimSpace(rec.SessionID); sessionID != "" {
		rows = append(rows, outputRow{Label: "session", Value: sessionID})
	}
	if attachmentID := strings.TrimSpace(rec.ID); attachmentID != "" {
		rows = append(rows, outputRow{Label: "attachment", Value: attachmentID})
	}
	if !rec.StartedAt.IsZero() {
		rows = append(rows, outputRow{Label: "started", Value: formatDisplayTimestamp(rec.StartedAt.UTC().Format(time.RFC3339))})
	}
	printSection(rec.Workspace, rows)
}

func fallbackString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

// cmdStatusNotRunning renders status when no state file exists.
func cmdStatusNotRunning() error {
	cfg := loadConfigOrDefault()
	if err := resolveConfigPaths(&cfg); err != nil {
		cfg.WorkRoot = defaultWorkRoot()
	}
	backendName := cfg.MountBackend
	if backendName == "" {
		backendName = mountBackendNone
	}
	mode, _ := effectiveMode(cfg)

	title := clr(ansiDim, "○") + " " + clr(ansiBold, "AFS Daemon Not Running")
	rows := statusDisplayRows(cfg, statusRows(cfg, cfg.CurrentWorkspace, localSurfacePath(cfg), mode, backendName, cfg.RedisAddr, cfg.RedisDB))
	rows = appendAuthStatusRows(rows)
	printSection(title, rows)
	return nil
}

// cmdStatusSync renders status for a running sync daemon.
func cmdStatusSync(st state) {
	alive := st.SyncPID > 0 && processAlive(st.SyncPID)
	cfg := loadConfigOrDefault()
	if err := resolveConfigPaths(&cfg); err != nil {
		cfg.WorkRoot = defaultWorkRoot()
	}
	title := statusTitleForAlive(alive, st.SyncPID)
	rows := statusDisplayRows(cfg, statusRows(cfg, st.CurrentWorkspace, st.LocalPath, modeSync, "", st.RedisAddr, st.RedisDB))
	rows = appendAuthStatusRows(rows)
	rows = appendConnectedAgentRows(rows, cfg, st)
	rows = appendUptimeRows(rows, st)
	if snap := loadSyncStateForStatus(st.CurrentWorkspace); snap != nil {
		rows = append(rows, outputRow{Label: "entries", Value: fmt.Sprintf("%d", len(snap.Entries))})
		if !snap.UpdatedAt.IsZero() {
			rows = append(rows, outputRow{Label: "last sync", Value: relativeTime(snap.UpdatedAt)})
		}
	}
	printSection(title, rows)
}

// cmdStatusMount renders status for mount mode.
func cmdStatusMount(st state) error {
	backend, backendName, err := backendForState(st)
	if err != nil {
		return err
	}
	cfg := loadConfigOrDefault()
	if err := resolveConfigPaths(&cfg); err != nil {
		cfg.WorkRoot = defaultWorkRoot()
	}

	workspace := cfg.CurrentWorkspace
	if ws := strings.TrimSpace(st.CurrentWorkspace); ws != "" {
		workspace = ws
	}
	localPath := localSurfacePath(cfg)
	if backendName != mountBackendNone && strings.TrimSpace(st.LocalPath) != "" {
		localPath = st.LocalPath
	}

	alive := false
	if backendName != mountBackendNone {
		alive = backend.IsMounted(st.LocalPath) && st.MountPID > 0 && processAlive(st.MountPID)
	}
	title := statusTitleForAlive(alive, st.MountPID)

	rows := statusDisplayRows(cfg, statusRows(cfg, workspace, localPath, modeMount, backendName, st.RedisAddr, st.RedisDB))
	rows = appendAuthStatusRows(rows)
	rows = appendConnectedAgentRows(rows, cfg, st)
	rows = appendUptimeRows(rows, st)
	if st.ArchivePath != "" {
		rows = append(rows, outputRow{Label: "archive", Value: st.ArchivePath})
	}
	printSection(title, rows)
	return nil
}

// relativeTime renders a timestamp like "12s ago", "5m ago", "3h ago", or
// "2d ago". Past times only — future or zero return absolute fallback.
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	if d < 0 {
		return t.UTC().Format(time.RFC3339)
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}

func printReadyBox(cfg config, backendName, _ string) {
	localPath := localSurfacePath(cfg)
	mode, _ := effectiveMode(cfg)
	mounted := backendName != mountBackendNone
	if mounted {
		mode = modeMount
	}
	title := statusTitle(markerSuccess, 0)
	if !mounted {
		title = statusTitle(clr(ansiYellow, "○"), 0)
	}
	rows := statusRows(cfg, cfg.CurrentWorkspace, localPath, mode, backendName, cfg.RedisAddr, cfg.RedisDB)

	if cfg.ReadOnly {
		rows = append(rows, outputRow{Label: "readonly", Value: "yes"})
	}
	if backendName == mountBackendNone {
		rows = append(rows, outputRow{})
		rows = append(rows, outputRow{Label: "create", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" workspace create <workspace>")})
		rows = append(rows, outputRow{Label: "import", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" workspace import <workspace> <directory>")})
		printSection(title, rows)
		return
	}
	rows = append(rows, outputRow{})
	rows = append(rows, outputRow{Label: "try", Value: clr(ansiOrange, "ls "+shellQuote(localPath))})
	rows = append(rows, outputRow{Label: "detach", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" ws detach "+shellQuote(localPath))})
	printSection(title, rows)
}
