package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// appendAuthStatusRows appends cloud-auth hint rows when action is needed.
// We intentionally drop the "signed in: yes" confirmation (it's noise — the
// cloud database in the core rows already implies success), the control-plane
// URL (duplicate of configStatusRow), and the cloud database id (already in
// the core "database" row). Only actionable rows (e.g. "needs refresh")
// survive.
func appendAuthStatusRows(rows []boxRow) []boxRow {
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
func appendConfigRows(rows []boxRow, cfg config) []boxRow {
	if row := configSourceStatusRow(cfg); row.Label != "" {
		rows = append(rows, row)
	}
	return append(rows, boxRow{Label: "config file", Value: configPathLabel()})
}

func configSourceStatusRow(cfg config) boxRow {
	productMode, err := effectiveProductMode(cfg)
	if err != nil {
		return boxRow{}
	}

	if productMode == productModeLocal {
		return boxRow{Label: "config source", Value: productModeDisplayLabel(productMode)}
	}

	value := strings.TrimSpace(cfg.URL)
	if value == "" {
		value = "<control plane url not configured>"
	}
	return boxRow{Label: "config source", Value: productModeDisplayLabel(productMode) + ": " + value}
}

func commandContextRows(cfg config, workspace string) []boxRow {
	rows := make([]boxRow, 0, 2)
	if strings.TrimSpace(workspace) != "" {
		rows = append(rows, boxRow{Label: "workspace", Value: workspace})
	}
	rows = append(rows, boxRow{Label: "database", Value: configRemoteLabel(cfg)})
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

// statusRows returns the consistent core rows: workspace, local, database,
// and mode. Mount backend is included only for FUSE/NFS. In cloud mode the
// database row shows the cloud database id instead of the local Redis
// endpoint so users see the database they're actually talking to.
func statusRows(cfg config, workspace, localPath, mode, backendName, redisAddr string, redisDB int) []boxRow {
	var rows []boxRow
	if ws := strings.TrimSpace(workspace); ws != "" {
		rows = append(rows, boxRow{Label: "workspace", Value: ws})
	}
	if localPath != "" {
		rows = append(rows, boxRow{Label: "local", Value: localPath})
	}
	rows = append(rows, boxRow{Label: "database", Value: databaseStatusLabel(cfg, redisAddr, redisDB)})
	rows = append(rows, boxRow{Label: "mode", Value: mode})
	if backendName != "" && backendName != mountBackendNone {
		rows = append(rows, boxRow{Label: "mount backend", Value: userModeLabel(backendName)})
	}
	if account := strings.TrimSpace(cfg.Account); account != "" {
		if productMode, _ := effectiveProductMode(cfg); productMode == productModeCloud {
			rows = append(rows, boxRow{Label: "account", Value: account})
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
func appendUptimeRows(rows []boxRow, st state) []boxRow {
	rows = append(rows, boxRow{Label: "uptime", Value: formatDuration(time.Since(st.StartedAt))})
	if st.ReadOnly {
		rows = append(rows, boxRow{Label: "readonly", Value: "yes"})
	}
	return rows
}

func appendConnectedAgentRows(rows []boxRow, cfg config, st state) []boxRow {
	if strings.TrimSpace(st.SessionID) == "" {
		return rows
	}
	if id := strings.TrimSpace(cfg.ID); id != "" {
		rows = append(rows, boxRow{Label: "agent id", Value: id})
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

// cmdStatus dispatches to one of three status renderers:
//   - cmdStatusNotRunning: no state file exists (afs is stopped)
//   - cmdStatusSync:       running in sync mode
//   - cmdStatusMount:      running in mount mode
func cmdStatus() error {
	st, err := loadState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cmdStatusNotRunning()
		}
		return err
	}
	if strings.TrimSpace(st.Mode) == modeSync {
		cmdStatusSync(st)
		return nil
	}
	return cmdStatusMount(st)
}

// cmdStatusNotRunning renders the status box when no state file exists.
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

	title := clr(ansiDim, "○") + " " + clr(ansiBold, "AFS Not Running")
	rows := statusRows(cfg, cfg.CurrentWorkspace, localSurfacePath(cfg), mode, backendName, cfg.RedisAddr, cfg.RedisDB)
	rows = appendConfigRows(rows, cfg)
	rows = appendAuthStatusRows(rows)
	rows = append(rows, boxRow{Label: "start", Value: clr(ansiOrange, "afs up")})
	printBox(title, rows)
	return nil
}

// cmdStatusSync renders the status box for a running sync daemon.
func cmdStatusSync(st state) {
	workspace := strings.TrimSpace(st.CurrentWorkspace)
	alive := st.SyncPID > 0 && processAlive(st.SyncPID)
	cfg := loadConfigOrDefault()
	if err := resolveConfigPaths(&cfg); err != nil {
		cfg.WorkRoot = defaultWorkRoot()
	}

	title := statusTitleForAlive(alive, st.SyncPID)
	rows := statusRows(cfg, workspace, st.LocalPath, modeSync, "", st.RedisAddr, st.RedisDB)
	rows = appendConfigRows(rows, cfg)
	rows = appendAuthStatusRows(rows)
	rows = appendConnectedAgentRows(rows, cfg, st)
	rows = appendUptimeRows(rows, st)
	if snap := loadSyncStateForStatus(workspace); snap != nil {
		rows = append(rows, boxRow{Label: "entries", Value: fmt.Sprintf("%d", len(snap.Entries))})
		if !snap.UpdatedAt.IsZero() {
			rows = append(rows, boxRow{Label: "last sync", Value: relativeTime(snap.UpdatedAt)})
		}
	}
	printBox(title, rows)
}

// cmdStatusMount renders the status box for mount mode (running).
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

	rows := statusRows(cfg, workspace, localPath, modeMount, backendName, st.RedisAddr, st.RedisDB)
	rows = appendConfigRows(rows, cfg)
	rows = appendAuthStatusRows(rows)
	rows = appendConnectedAgentRows(rows, cfg, st)
	rows = appendUptimeRows(rows, st)
	if st.ArchivePath != "" {
		rows = append(rows, boxRow{Label: "archive", Value: st.ArchivePath})
	}
	printBox(title, rows)
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
	title := statusTitle(markerSuccess, 0)
	if !mounted {
		title = statusTitle(clr(ansiYellow, "○"), 0)
	}
	rows := statusRows(cfg, cfg.CurrentWorkspace, localPath, mode, backendName, cfg.RedisAddr, cfg.RedisDB)

	if cfg.ReadOnly {
		rows = append(rows, boxRow{Label: "readonly", Value: "yes"})
	}
	if backendName == mountBackendNone {
		rows = append(rows, boxRow{})
		rows = append(rows, boxRow{Label: "create", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" workspace create <workspace>")})
		rows = append(rows, boxRow{Label: "import", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" workspace import <workspace> <directory>")})
		printBox(title, rows)
		return
	}
	rows = append(rows, boxRow{})
	rows = append(rows, boxRow{Label: "try", Value: clr(ansiOrange, "ls "+cfg.LocalPath)})
	rows = append(rows, boxRow{Label: "stop", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" down")})
	printBox(title, rows)
}
