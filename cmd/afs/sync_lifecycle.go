package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

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

	redisPID := 0
	if !cfg.UseExistingRedis {
		s := startStep("Starting Redis server")
		pid, err := startRedisDaemon(cfg)
		if err != nil {
			s.fail(err.Error())
			return err
		}
		redisPID = pid
		s.succeed(fmt.Sprintf("pid %d", pid))
	}

	s := startStep("Connecting to Redis")
	rdb := redis.NewClient(buildRedisOptions(cfg, 4))
	defer rdb.Close()
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		s.fail(fmt.Sprintf("cannot reach %s", cfg.RedisAddr))
		return fmt.Errorf("cannot connect to Redis at %s: %w", cfg.RedisAddr, err)
	}
	s.succeed(cfg.RedisAddr)

	store := newAFSStore(rdb)

	workspaceStep := startStep("Ensuring current workspace")
	workspace, err := ensureMountWorkspace(ctx, cfg, store)
	if err != nil {
		workspaceStep.fail(err.Error())
		return fmt.Errorf("a current workspace is required before AFS can sync: %w", err)
	}
	workspaceStep.succeed(workspace)

	prepareStep := startStep("Opening live workspace")
	mountKey, _, initialized, err := seedWorkspaceMountKey(ctx, store, workspace)
	if err != nil {
		prepareStep.fail(err.Error())
		return err
	}
	if initialized {
		prepareStep.succeed(workspace + " (initialized)")
	} else {
		prepareStep.succeed(workspace)
	}

	// Do the initial reconciliation in the foreground so the user sees
	// progress and the local folder is fully populated before we return.
	bootStep := startStep("Syncing workspace")
	fsClient := client.New(rdb, mountKey)
	daemon, err := newSyncDaemon(syncDaemonConfig{
		Workspace:    workspace,
		LocalRoot:    localRoot,
		FS:           fsClient,
		Store:        store,
		MaxFileBytes: syncSizeCapBytes(cfg),
		Readonly:     cfg.ReadOnly,
		Interactive:  foreground,
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
	bootStep.succeed(fmt.Sprintf("%s synced", workspace))

	if foreground {
		// --interactive: keep the daemon in this process with logs on stderr.
		// Don't stop the daemon we just started — it's already running.
		st := state{
			StartedAt:        time.Now().UTC(),
			ManageRedis:      !cfg.UseExistingRedis,
			RedisAddr:        cfg.RedisAddr,
			RedisDB:          cfg.RedisDB,
			CurrentWorkspace: workspace,
			MountBackend:     mountBackendNone,
			ReadOnly:         cfg.ReadOnly,
			RedisKey:         mountKey,
			RedisLog:         cfg.RedisLog,
			RedisServerBin:   cfg.RedisServerBin,
			Mode:             modeSync,
			SyncPID:          os.Getpid(),
			LocalPath:        localRoot,
			SyncLog:          cfg.SyncLog,
		}
		if !cfg.UseExistingRedis {
			st.RedisPID = redisPID
		}
		if err := saveState(st); err != nil {
			daemon.Stop()
			return err
		}

		printSyncReadyBox(cfg, workspace, localRoot)
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
		cancel()
		daemon.Stop()
		stopStep.succeed("clean")

		if !cfg.UseExistingRedis && redisPID > 0 && processAlive(redisPID) {
			rs := startStep("Stopping Redis server")
			_ = terminatePID(redisPID, 2*time.Second)
			rs.succeed(fmt.Sprintf("pid %d", redisPID))
		}

		cleanStep := startStep("Removing local sync folder")
		if err := os.RemoveAll(localRoot); err != nil {
			cleanStep.fail(err.Error())
		} else {
			cleanStep.succeed(localRoot)
		}
		_ = removeSyncState(workspace)
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
	daemonPID, err := startSyncDaemonProcess(cfg)
	if err != nil {
		daemonStep.fail(err.Error())
		return err
	}
	daemonStep.succeed(fmt.Sprintf("pid %d", daemonPID))

	st := state{
		StartedAt:        time.Now().UTC(),
		ManageRedis:      !cfg.UseExistingRedis,
		RedisAddr:        cfg.RedisAddr,
		RedisDB:          cfg.RedisDB,
		CurrentWorkspace: workspace,
		MountBackend:     mountBackendNone,
		ReadOnly:         cfg.ReadOnly,
		RedisKey:         mountKey,
		RedisLog:         cfg.RedisLog,
		RedisServerBin:   cfg.RedisServerBin,
		Mode:             modeSync,
		SyncPID:          daemonPID,
		LocalPath:        localRoot,
		SyncLog:          cfg.SyncLog,
	}
	if !cfg.UseExistingRedis {
		st.RedisPID = redisPID
	}
	if err := saveState(st); err != nil {
		return err
	}

	printSyncReadyBox(cfg, workspace, localRoot)
	return nil
}

// startSyncDaemonProcess re-execs the current binary with the hidden
// `_sync-daemon` subcommand. The child process inherits the config path
// and runs in a new session (Setsid) so it survives the parent exiting.
func startSyncDaemonProcess(cfg config) (int, error) {
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
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if devNull, err := os.Open(os.DevNull); err == nil {
		defer devNull.Close()
		cmd.Stdin = devNull
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start sync daemon: %w", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}

// runSyncDaemon is the entry point for the `_sync-daemon` child process.
// It connects to Redis, starts the sync daemon, and blocks until SIGTERM.
func runSyncDaemon() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := resolveConfigPaths(&cfg); err != nil {
		return fmt.Errorf("resolve config: %w", err)
	}

	localRoot, err := expandPath(cfg.LocalPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rdb := redis.NewClient(buildRedisOptions(cfg, 4))
	defer rdb.Close()
	pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
	defer pingCancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		return fmt.Errorf("cannot connect to Redis at %s: %w", cfg.RedisAddr, err)
	}

	store := newAFSStore(rdb)
	workspace := strings.TrimSpace(cfg.CurrentWorkspace)
	if workspace == "" {
		return errors.New("no current workspace")
	}
	mountKey, _, _, err := seedWorkspaceMountKey(ctx, store, workspace)
	if err != nil {
		return fmt.Errorf("seed workspace: %w", err)
	}

	fsClient := client.New(rdb, mountKey)
	daemon, err := newSyncDaemon(syncDaemonConfig{
		Workspace:    workspace,
		LocalRoot:    localRoot,
		FS:           fsClient,
		Store:        store,
		MaxFileBytes: syncSizeCapBytes(cfg),
		Readonly:     cfg.ReadOnly,
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

	fmt.Fprintf(os.Stderr, "afs sync daemon: running for workspace %s at %s (pid %d)\n", workspace, localRoot, os.Getpid())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	signal.Stop(sigCh)

	fmt.Fprintf(os.Stderr, "afs sync daemon: shutting down\n")
	cancel()
	daemon.Stop()
	return nil
}

// validateSyncLocalPath blocks dual-writer collisions.
func validateSyncLocalPath(cfg config, localRoot string) error {
	cleanLocal := filepath.Clean(localRoot)
	for _, forbidden := range []string{cfg.WorkRoot, stateDir()} {
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
	rows := statusRows(workspace, localRoot, modeSync, "", cfg.RedisAddr, cfg.RedisDB)
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
	if st.ManageRedis && st.RedisPID > 0 && processAlive(st.RedisPID) {
		s := startStep("Stopping Redis server")
		_ = terminatePID(st.RedisPID, 2*time.Second)
		s.succeed(fmt.Sprintf("pid %d", st.RedisPID))
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

	if err := os.Remove(statePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return true, err
	}
	fmt.Printf("\n  %s afs sync stopped\n\n", clr(ansiDim, "■"))
	return true, nil
}

