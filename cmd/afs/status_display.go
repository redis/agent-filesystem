package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
	return redisDatabaseLabel(cfg.RedisAddr, cfg.RedisDB, cfg.RedisTLS)
}

func configPathLabel() string {
	return clr(ansiDim, compactDisplayPath(configPath()))
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
		return prefix + " " + clr(ansiBold, fmt.Sprintf("AFS Running (daemon %d)", pid))
	}
	return prefix + " " + clr(ansiBold, "AFS Running")
}

func localSurfacePath(cfg config) string {
	return cfg.LocalPath
}

// statusRows returns the consistent core rows: workspace, local, database,
// and mode. Mount backend is included only for FUSE/NFS.
func statusRows(workspace, localPath, mode, backendName, redisAddr string, redisDB int) []boxRow {
	var rows []boxRow
	if ws := strings.TrimSpace(workspace); ws != "" {
		rows = append(rows, boxRow{Label: "workspace", Value: ws})
	}
	if localPath != "" {
		rows = append(rows, boxRow{Label: "local", Value: localPath})
	}
	rows = append(rows, boxRow{Label: "database", Value: statusRemoteLabel(redisAddr, redisDB)})
	rows = append(rows, boxRow{Label: "mode", Value: mode})
	if backendName != "" && backendName != mountBackendNone {
		rows = append(rows, boxRow{Label: "mount backend", Value: userModeLabel(backendName)})
	}
	return rows
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
	rows := statusRows(cfg.CurrentWorkspace, localSurfacePath(cfg), mode, backendName, cfg.RedisAddr, cfg.RedisDB)
	rows = append(rows,
		boxRow{Label: "config", Value: configPathLabel()},
		boxRow{Label: "start", Value: clr(ansiOrange, "afs up")},
	)
	printBox(title, rows)
	return nil
}

// cmdStatusSync renders the status box for a running sync daemon.
func cmdStatusSync(st state) {
	workspace := strings.TrimSpace(st.CurrentWorkspace)
	alive := st.SyncPID > 0 && processAlive(st.SyncPID)

	title := statusTitleForAlive(alive, st.SyncPID)
	rows := statusRows(workspace, st.LocalPath, modeSync, "", st.RedisAddr, st.RedisDB)
	rows = append(rows, boxRow{Label: "config", Value: configPathLabel()})
	rows = appendUptimeRows(rows, st)
	if snap := loadSyncStateForStatus(workspace); snap != nil {
		rows = append(rows, boxRow{Label: "entries", Value: fmt.Sprintf("%d", len(snap.Entries))})
		if !snap.UpdatedAt.IsZero() {
			rows = append(rows, boxRow{Label: "last sync", Value: snap.UpdatedAt.UTC().Format(time.RFC3339)})
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

	rows := statusRows(workspace, localPath, modeMount, backendName, st.RedisAddr, st.RedisDB)
	rows = append(rows, boxRow{Label: "config", Value: configPathLabel()})
	rows = appendUptimeRows(rows, st)
	if st.ArchivePath != "" {
		rows = append(rows, boxRow{Label: "archive", Value: st.ArchivePath})
	}
	printBox(title, rows)
	return nil
}

func printReadyBox(cfg config, backendName, _ string) {
	localPath := localSurfacePath(cfg)
	mode, _ := effectiveMode(cfg)
	mounted := backendName != mountBackendNone
	title := statusTitle(markerSuccess, 0)
	if !mounted {
		title = statusTitle(clr(ansiYellow, "○"), 0)
	}
	rows := statusRows(cfg.CurrentWorkspace, localPath, mode, backendName, cfg.RedisAddr, cfg.RedisDB)

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
