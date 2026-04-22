package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/redis/agent-filesystem/internal/version"
	"github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

const syncDaemonBootstrapEnv = "AFS_SYNC_BOOTSTRAP"

type syncBootstrap struct {
	cfg             config
	workspace       string
	redisKey        string
	headCheckpoint  string
	initializedRoot bool
	sessionID       string
	heartbeatEvery  time.Duration
}

type syncDaemonBootstrap struct {
	Config                   config `json:"config"`
	Workspace                string `json:"workspace"`
	RedisKey                 string `json:"redis_key"`
	SessionID                string `json:"session_id,omitempty"`
	HeartbeatIntervalSeconds int    `json:"heartbeat_interval_seconds,omitempty"`
}

func prepareSyncBootstrap(ctx context.Context, cfg config) (syncBootstrap, func(), error) {
	resolvedCfg, service, closeFn, err := openAFSControlPlaneForConfig(ctx, cfg)
	if err != nil {
		return syncBootstrap{}, func() {}, err
	}

	selection, err := currentWorkspaceSelectionFromControlPlane(ctx, resolvedCfg, service)
	if err != nil {
		closeFn()
		return syncBootstrap{}, func() {}, fmt.Errorf("a current workspace is required before AFS can sync: %w", err)
	}

	sessionInput := managedWorkspaceSessionRequest(resolvedCfg)
	session, err := service.CreateWorkspaceSession(ctx, selection.ID, sessionInput)
	if err != nil {
		closeFn()
		return syncBootstrap{}, func() {}, err
	}

	runtimeCfg := resolvedCfg
	runtimeCfg.CurrentWorkspace = selection.Name
	runtimeCfg.CurrentWorkspaceID = selection.ID
	runtimeCfg.DatabaseID = strings.TrimSpace(session.DatabaseID)
	runtimeCfg.RedisAddr = rewriteManagedRedisAddrForLocalhost(runtimeCfg.URL, session.Redis.RedisAddr)
	runtimeCfg.RedisUsername = session.Redis.RedisUsername
	runtimeCfg.RedisPassword = session.Redis.RedisPassword
	runtimeCfg.RedisDB = session.Redis.RedisDB
	runtimeCfg.RedisTLS = session.Redis.RedisTLS

	return syncBootstrap{
		cfg:             runtimeCfg,
		workspace:       selection.Name,
		redisKey:        session.RedisKey,
		headCheckpoint:  session.HeadCheckpointID,
		initializedRoot: session.Initialized,
		sessionID:       strings.TrimSpace(session.SessionID),
		heartbeatEvery:  time.Duration(session.HeartbeatIntervalSeconds) * time.Second,
	}, closeFn, nil
}

// startSyncServices is the sync-mode counterpart to startServices. It boots
// Redis (if managed), opens the live workspace root, does the initial
// reconciliation (blocking with progress), then re-execs itself as a
// background daemon process and returns — exactly like mount mode. The parent
// `afs up` process exits and returns control to the shell.
func startSyncServices(cfg config, foreground bool) error {
	if strings.TrimSpace(cfg.LocalPath) == "" {
		return errors.New("localPath is required when mode=sync; run `afs setup` or set localPath in afs.config.json")
	}
	localRoot, err := expandPath(cfg.LocalPath)
	if err != nil {
		return err
	}
	cfg.LocalPath = localRoot

	if err := validateSyncLocalPath(cfg, localRoot); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prepareStep := startStep("Opening workspace session")
	bootstrap, closeSession, err := prepareSyncBootstrap(ctx, cfg)
	if err != nil {
		prepareStep.fail(err.Error())
		return err
	}
	defer closeSession()
	runtimeCfg := bootstrap.cfg
	ctx = withSessionID(ctx, bootstrap.sessionID)
	if bootstrap.initializedRoot {
		prepareStep.succeed(bootstrap.workspace + " (initialized)")
	} else {
		prepareStep.succeed(bootstrap.workspace)
	}

	s := startStep("Connecting to Redis")
	rdb := redis.NewClient(buildRedisOptions(runtimeCfg, 4))
	defer rdb.Close()
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		s.fail(fmt.Sprintf("cannot reach %s", runtimeCfg.RedisAddr))
		return fmt.Errorf("cannot connect to Redis at %s: %w", runtimeCfg.RedisAddr, err)
	}
	s.succeed(runtimeCfg.RedisAddr)

	store := newAFSStore(rdb)

	// Do the initial reconciliation in the foreground so the user sees
	// progress and the local folder is fully populated before we return.
	bootStep := startStep("Syncing workspace")
	fsClient := client.New(rdb, bootstrap.redisKey)
	daemon, err := newSyncDaemon(syncDaemonConfig{
		Workspace:    bootstrap.workspace,
		LocalRoot:    localRoot,
		FS:           fsClient,
		Store:        store,
		MaxFileBytes: syncSizeCapBytes(runtimeCfg),
		Readonly:     runtimeCfg.ReadOnly,
		Interactive:  foreground,
		Rdb:          rdb,
		StorageID:    bootstrap.redisKey,
		SessionID:    bootstrap.sessionID,
		AgentVersion: version.String(),
	})
	if err != nil {
		bootStep.fail(err.Error())
		return err
	}
	progress := func(done, total int64) {
		if total < 0 {
			// Scan phase: total is unknown, done = entries discovered so far.
			bootStep.update(fmt.Sprintf("Scanning workspace · %d entries", done))
		} else {
			bootStep.update(fmt.Sprintf("Syncing workspace · %d/%d files", done, total))
		}
	}
	if err := daemon.StartWithProgress(ctx, progress); err != nil {
		bootStep.fail(err.Error())
		return err
	}
	bootStep.succeed(fmt.Sprintf("%s synced", bootstrap.workspace))

	if foreground {
		// --interactive: keep the daemon in this process with logs on stderr.
		// Don't stop the daemon we just started — it's already running.
		st := state{
			StartedAt:            time.Now().UTC(),
			ProductMode:          runtimeCfg.ProductMode,
			ControlPlaneURL:      runtimeCfg.URL,
			ControlPlaneDatabase: runtimeCfg.DatabaseID,
			SessionID:            bootstrap.sessionID,
			RedisAddr:            runtimeCfg.RedisAddr,
			RedisDB:              runtimeCfg.RedisDB,
			CurrentWorkspace:     bootstrap.workspace,
			CurrentWorkspaceID:   runtimeCfg.CurrentWorkspaceID,
			MountBackend:         mountBackendNone,
			ReadOnly:             runtimeCfg.ReadOnly,
			RedisKey:             bootstrap.redisKey,
			Mode:                 modeSync,
			SyncPID:              os.Getpid(),
			LocalPath:            localRoot,
			SyncLog:              runtimeCfg.SyncLog,
		}
		if err := saveState(st); err != nil {
			daemon.Stop()
			return err
		}
		stopSessionLifecycle, err := startManagedWorkspaceSessionLifecycle(runtimeCfg, bootstrap.workspace, bootstrap.sessionID, bootstrap.heartbeatEvery)
		if err != nil {
			daemon.Stop()
			_ = os.Remove(statePath())
			return err
		}

		printSyncReadyBox(runtimeCfg, bootstrap.workspace, localRoot)
		fmt.Fprintf(os.Stderr, "\n  Running in interactive mode. Ctrl-C to stop.\n\n")

		// Disable main()'s SIGINT handler so we get the signal here.
		if mainSigCh != nil {
			signal.Stop(mainSigCh)
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		signal.Stop(sigCh)

		// Full cleanup — same as `afs down`.
		fmt.Println()
		stopStep := startStep("Stopping sync daemon")
		stopSessionLifecycle()
		cancel()
		daemon.Stop()
		stopStep.succeed("clean")

		cleanStep := startStep("Removing local sync folder")
		if err := os.RemoveAll(localRoot); err != nil {
			cleanStep.fail(err.Error())
		} else {
			cleanStep.succeed(localRoot)
		}
		_ = removeSyncState(bootstrap.workspace)
		if err := os.Remove(statePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "afs sync: cleanup state file: %v\n", err)
		}
		fmt.Printf("\n  %s afs sync stopped\n\n", clr(ansiDim, "■"))
		return nil
	}

	// Background mode (default): stop the in-process daemon, re-exec as a
	// background process, and return to the shell.
	daemon.Stop()

	daemonStep := startStep("Starting background daemon")
	var daemonBootstrap *syncDaemonBootstrap
	if productMode, _ := effectiveProductMode(cfg); productMode != productModeLocal {
		daemonBootstrap = &syncDaemonBootstrap{
			Config:                   runtimeCfg,
			Workspace:                bootstrap.workspace,
			RedisKey:                 bootstrap.redisKey,
			SessionID:                bootstrap.sessionID,
			HeartbeatIntervalSeconds: int(bootstrap.heartbeatEvery / time.Second),
		}
	}
	daemonPID, err := startSyncDaemonProcess(runtimeCfg, daemonBootstrap)
	if err != nil {
		daemonStep.fail(err.Error())
		closeManagedWorkspaceSession(runtimeCfg, bootstrap.workspace, bootstrap.sessionID)
		return err
	}
	daemonStep.succeed(fmt.Sprintf("pid %d", daemonPID))

	st := state{
		StartedAt:            time.Now().UTC(),
		ProductMode:          runtimeCfg.ProductMode,
		ControlPlaneURL:      runtimeCfg.URL,
		ControlPlaneDatabase: runtimeCfg.DatabaseID,
		SessionID:            bootstrap.sessionID,
		RedisAddr:            runtimeCfg.RedisAddr,
		RedisDB:              runtimeCfg.RedisDB,
		CurrentWorkspace:     bootstrap.workspace,
		CurrentWorkspaceID:   runtimeCfg.CurrentWorkspaceID,
		MountBackend:         mountBackendNone,
		ReadOnly:             runtimeCfg.ReadOnly,
		RedisKey:             bootstrap.redisKey,
		Mode:                 modeSync,
		SyncPID:              daemonPID,
		LocalPath:            localRoot,
		SyncLog:              runtimeCfg.SyncLog,
	}
	if err := saveState(st); err != nil {
		return err
	}

	printSyncReadyBox(runtimeCfg, bootstrap.workspace, localRoot)
	return nil
}

// startSyncDaemonProcess re-execs the current binary with the hidden
// `_sync-daemon` subcommand. The child process inherits the config path
// and runs in a new session (Setsid) so it survives the parent exiting.
func startSyncDaemonProcess(cfg config, bootstrap *syncDaemonBootstrap) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("cannot find own executable: %w", err)
	}

	logPath := cfg.SyncLog
	if strings.TrimSpace(logPath) == "" {
		logPath = "/tmp/afs-sync.log"
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return 0, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer logFile.Close()

	args := []string{"_sync-daemon"}
	if cfgPathOverride != "" {
		args = []string{"--config", cfgPathOverride, "_sync-daemon"}
	}

	cmd := exec.Command(exe, args...)
	cmd.Env = os.Environ()
	bootstrapPath := ""
	if bootstrap != nil {
		bootstrapPath, err = writeSyncDaemonBootstrap(*bootstrap)
		if err != nil {
			return 0, err
		}
		cmd.Env = append(cmd.Env, syncDaemonBootstrapEnv+"="+bootstrapPath)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if devNull, err := os.Open(os.DevNull); err == nil {
		defer devNull.Close()
		cmd.Stdin = devNull
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		if bootstrapPath != "" {
			_ = os.Remove(bootstrapPath)
		}
		return 0, fmt.Errorf("start sync daemon: %w", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}

// runSyncDaemon is the entry point for the `_sync-daemon` child process.
// It connects to Redis, starts the sync daemon, and blocks until SIGTERM.
func runSyncDaemon() error {
	cfg, workspace, mountKey, sessionID, heartbeatEvery, err := loadSyncDaemonRuntime()
	if err != nil {
		return err
	}

	localRoot, err := expandPath(cfg.LocalPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = withSessionID(ctx, sessionID)

	rdb := redis.NewClient(buildRedisOptions(cfg, 4))
	defer rdb.Close()
	pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
	defer pingCancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		return fmt.Errorf("cannot connect to Redis at %s: %w", cfg.RedisAddr, err)
	}

	store := newAFSStore(rdb)

	fsClient := client.New(rdb, mountKey)
	daemon, err := newSyncDaemon(syncDaemonConfig{
		Workspace:    workspace,
		LocalRoot:    localRoot,
		FS:           fsClient,
		Store:        store,
		MaxFileBytes: syncSizeCapBytes(cfg),
		Readonly:     cfg.ReadOnly,
		Rdb:          rdb,
		StorageID:    mountKey,
		SessionID:    sessionID,
		AgentVersion: version.String(),
	})
	if err != nil {
		return err
	}
	// Skip the initial reconcile — the parent process already did it moments
	// ago. Go straight to the steady-state goroutines so the subscription
	// pump starts receiving events immediately.
	if err := daemon.StartSteadyStateOnly(ctx); err != nil {
		return err
	}
	stopSessionLifecycle, err := startManagedWorkspaceSessionLifecycle(cfg, workspace, sessionID, heartbeatEvery)
	if err != nil {
		daemon.Stop()
		return err
	}

	fmt.Fprintf(os.Stderr, "afs sync daemon: running for workspace %s at %s (pid %d)\n", workspace, localRoot, os.Getpid())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	signal.Stop(sigCh)

	fmt.Fprintf(os.Stderr, "afs sync daemon: shutting down\n")
	stopSessionLifecycle()
	cancel()
	daemon.Stop()
	return nil
}

func loadSyncDaemonRuntime() (config, string, string, string, time.Duration, error) {
	bootstrap, ok, err := loadSyncDaemonBootstrap()
	if err != nil {
		return config{}, "", "", "", 0, err
	}
	if ok {
		cfg := bootstrap.Config
		if err := resolveConfigPaths(&cfg); err != nil {
			return config{}, "", "", "", 0, fmt.Errorf("resolve bootstrap config: %w", err)
		}
		if strings.TrimSpace(bootstrap.Workspace) == "" {
			return config{}, "", "", "", 0, errors.New("sync bootstrap is missing workspace")
		}
		if strings.TrimSpace(bootstrap.RedisKey) == "" {
			return config{}, "", "", "", 0, errors.New("sync bootstrap is missing redis key")
		}
		return cfg, strings.TrimSpace(bootstrap.Workspace), strings.TrimSpace(bootstrap.RedisKey), strings.TrimSpace(bootstrap.SessionID), time.Duration(bootstrap.HeartbeatIntervalSeconds) * time.Second, nil
	}

	cfg, err := loadConfig()
	if err != nil {
		return config{}, "", "", "", 0, fmt.Errorf("load config: %w", err)
	}
	if err := resolveConfigPaths(&cfg); err != nil {
		return config{}, "", "", "", 0, fmt.Errorf("resolve config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	bootstrapState, closeFn, err := prepareSyncBootstrap(ctx, cfg)
	if err != nil {
		return config{}, "", "", "", 0, err
	}
	defer closeFn()
	return bootstrapState.cfg, bootstrapState.workspace, bootstrapState.redisKey, bootstrapState.sessionID, bootstrapState.heartbeatEvery, nil
}

func writeSyncDaemonBootstrap(bootstrap syncDaemonBootstrap) (string, error) {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return "", err
	}
	raw, err := json.Marshal(bootstrap)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp(stateDir(), ".sync-bootstrap-*.json")
	if err != nil {
		return "", err
	}
	name := file.Name()
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

func loadSyncDaemonBootstrap() (syncDaemonBootstrap, bool, error) {
	path := strings.TrimSpace(os.Getenv(syncDaemonBootstrapEnv))
	if path == "" {
		return syncDaemonBootstrap{}, false, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return syncDaemonBootstrap{}, false, fmt.Errorf("read sync bootstrap: %w", err)
	}
	_ = os.Remove(path)
	var bootstrap syncDaemonBootstrap
	if err := json.Unmarshal(raw, &bootstrap); err != nil {
		return syncDaemonBootstrap{}, false, fmt.Errorf("parse sync bootstrap: %w", err)
	}
	return bootstrap, true, nil
}

// validateSyncLocalPath blocks dual-writer collisions.
func validateSyncLocalPath(cfg config, localRoot string) error {
	cleanLocal := filepath.Clean(localRoot)
	for _, forbidden := range []string{defaultWorkRoot(), stateDir()} {
		if strings.TrimSpace(forbidden) == "" {
			continue
		}
		clean := filepath.Clean(forbidden)
		if cleanLocal == clean {
			return fmt.Errorf("syncLocalPath %q collides with %q; choose a different directory", cleanLocal, clean)
		}
		rel, err := filepath.Rel(clean, cleanLocal)
		if err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
			return fmt.Errorf("syncLocalPath %q is inside %q; sync would conflict with afs internal storage", cleanLocal, clean)
		}
	}
	return nil
}

func printSyncReadyBox(cfg config, workspace, localRoot string) {
	title := statusTitle(markerSuccess, 0)
	rows := statusRows(cfg, workspace, localRoot, modeSync, "", cfg.RedisAddr, cfg.RedisDB)
	if cfg.ReadOnly {
		rows = append(rows, boxRow{Label: "readonly", Value: "yes"})
	}
	rows = append(rows, boxRow{})
	rows = append(rows, boxRow{Label: "stop", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" down")})
	printBox(title, rows)
}

// stopSyncServicesIfActive performs the cleanup that cmdDown needs when the
// running daemon was started in sync mode.
func stopSyncServicesIfActive(st state) (bool, error) {
	if strings.TrimSpace(st.Mode) != modeSync {
		return false, nil
	}

	fmt.Println()

	if st.SyncPID > 0 && processAlive(st.SyncPID) {
		s := startStep("Stopping sync daemon")
		if err := terminatePID(st.SyncPID, 5*time.Second); err != nil {
			s.fail(err.Error())
		} else {
			s.succeed(fmt.Sprintf("pid %d", st.SyncPID))
		}
	}
	// Remove the local sync folder — the source of truth is Redis, and
	// `afs up` will re-populate it. Same semantics as mount mode removing
	// the mountpoint.
	if localPath := strings.TrimSpace(st.LocalPath); localPath != "" {
		s := startStep("Removing local sync folder")
		if err := os.RemoveAll(localPath); err != nil {
			s.fail(err.Error())
			fmt.Printf("  %s local sync folder preserved at %s\n", clr(ansiYellow, "!"), localPath)
		} else {
			s.succeed(localPath)
		}
	}

	// Clean up sync state file.
	if workspace := strings.TrimSpace(st.CurrentWorkspace); workspace != "" {
		_ = removeSyncState(workspace)
	}
	closeManagedWorkspaceSession(configFromState(st), strings.TrimSpace(st.CurrentWorkspace), strings.TrimSpace(st.SessionID))

	if err := os.Remove(statePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return true, err
	}
	fmt.Printf("\n  %s afs sync stopped\n\n", clr(ansiDim, "■"))
	return true, nil
}
