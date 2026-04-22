package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

type mountBootstrap struct {
	cfg             config
	workspace       string
	redisKey        string
	headCheckpoint  string
	initializedRoot bool
	sessionID       string
	heartbeatEvery  time.Duration
}

// cmdUp loads config, applies one-shot overrides, and starts the selected
// local surface.
func cmdUp() error {
	return cmdUpArgs(nil)
}

func cmdUpArgs(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, upUsageText(filepath.Base(os.Args[0])))
		return nil
	}

	opts, err := parseUpOptions(args)
	if err != nil {
		return err
	}

	if st, err := loadState(); err == nil {
		if (st.MountPID > 0 && processAlive(st.MountPID)) || (st.SyncPID > 0 && processAlive(st.SyncPID)) {
			return cmdStatus()
		}
	}

	cfg, err := loadConfigForUpWithOverridesAndMode(opts.positionals, opts.overrides, opts.mode)
	if err != nil {
		return err
	}
	productMode, err := effectiveProductMode(cfg)
	if err != nil {
		return err
	}
	mode, err := effectiveMode(cfg)
	if err != nil {
		return err
	}
	switch productMode {
	case productModeLocal:
		// Direct mode supports the existing sync/mount behavior below.
	case productModeSelfHosted, productModeCloud:
		if mode == modeSync {
			return startSyncServices(cfg, opts.foreground)
		}
		if mode == modeMount {
			if err := cleanupStaleMount(cfg); err != nil {
				return err
			}
			return startServices(cfg)
		}
		return fmt.Errorf("'afs up' in %s mode currently supports sync or mount only\nRun '%s up --mode sync', '%s up --mode mount', or update config first", productMode, filepath.Base(os.Args[0]), filepath.Base(os.Args[0]))
	default:
		return fmt.Errorf("'afs up' is not supported yet in %s mode\nUse workspace/checkpoint commands through the control plane for now", productMode)
	}
	if mode == modeSync {
		return startSyncServices(cfg, opts.foreground)
	}
	if mode == modeNone {
		cfg.MountBackend = mountBackendNone
		return startServices(cfg)
	}
	if err := cleanupStaleMount(cfg); err != nil {
		return err
	}
	return startServices(cfg)
}

func cleanupStaleMount(cfg config) error {
	if cfg.MountBackend == mountBackendNone || strings.TrimSpace(cfg.LocalPath) == "" {
		return nil
	}
	entry, mounted := mountTableEntry(cfg.LocalPath)
	if !mounted {
		return nil
	}
	if !isAFSMountEntry(entry) {
		return fmt.Errorf("mountpoint %s is already mounted by another filesystem\n  mount entry: %s", cfg.LocalPath, entry)
	}

	backend, _, err := backendForConfig(cfg)
	if err != nil {
		return err
	}

	s := startStep("Cleaning stale mount")
	if err := backend.Unmount(cfg.LocalPath); err != nil {
		s.fail(err.Error())
		return fmt.Errorf("stale AFS mount at %s could not be unmounted: %w", cfg.LocalPath, err)
	}
	s.succeed(cfg.LocalPath)
	return nil
}

func isAFSMountEntry(entry string) bool {
	v := strings.ToLower(entry)
	return strings.Contains(v, "fuse.agent-filesystem") || strings.Contains(v, "agent-filesystem on ") || strings.Contains(v, " agent-filesystem ")
}

func cmdDown() error {
	st, err := loadState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			printBox(clr(ansiBold, "AFS not running"), []boxRow{
				{Label: "status", Value: "already stopped"},
				{Label: "next", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" up")},
			})
			return nil
		}
		return err
	}

	if handled, err := stopSyncServicesIfActive(st); handled || err != nil {
		return err
	}

	fmt.Println()

	backend, _, err := backendForState(st)
	if err != nil {
		return err
	}
	cfg := loadConfigOrDefault()
	if strings.TrimSpace(st.CurrentWorkspace) != "" {
		cfg.CurrentWorkspace = st.CurrentWorkspace
	}

	var rdb *redis.Client

	// Always attempt unmount — even if the daemon crashed, the stale mount
	// may still be in the mount table and block access to the mountpoint.
	if backend.IsMounted(st.LocalPath) {
		s := startStep("Unmounting filesystem")
		if err := backend.Unmount(st.LocalPath); err != nil {
			s.fail(err.Error())
			fmt.Printf("  %s manual cleanup: umount -f %s\n", clr(ansiYellow, "!"), st.LocalPath)
		} else {
			s.succeed(st.LocalPath)
		}
	}

	if st.MountPID > 0 && processAlive(st.MountPID) {
		s := startStep("Stopping mount daemon")
		_ = terminatePID(st.MountPID, 2*time.Second)
		s.succeed(fmt.Sprintf("pid %d", st.MountPID))
	}
	if orphanPIDs, err := orphanMountDaemonPIDs(st); err == nil && len(orphanPIDs) > 0 {
		s := startStep("Stopping orphaned mount daemons")
		stopped := make([]string, 0, len(orphanPIDs))
		for _, pid := range orphanPIDs {
			if !processAlive(pid) {
				continue
			}
			if err := terminatePID(pid, 2*time.Second); err == nil {
				stopped = append(stopped, strconv.Itoa(pid))
			}
		}
		if len(stopped) > 0 {
			s.succeed(strings.Join(stopped, ", "))
		} else {
			s.fail("none stopped")
		}
	}

	closeManagedWorkspaceSession(configFromState(st), strings.TrimSpace(st.CurrentWorkspace), strings.TrimSpace(st.SessionID))

	if shouldCleanLegacyMountCache(st) && !backend.IsMounted(st.LocalPath) {
		redisCfg := cfg
		redisCfg.RedisAddr = st.RedisAddr
		redisCfg.RedisDB = st.RedisDB
		rdb = redis.NewClient(buildRedisOptions(redisCfg, 4))
		s := startStep("Cleaning mount cache")
		if err := deleteNamespace(context.Background(), rdb, st.RedisKey); err != nil {
			s.fail(err.Error())
			fmt.Printf("  %s mount cache preserved in Redis key %s\n", clr(ansiYellow, "!"), st.RedisKey)
		} else {
			s.succeed(st.RedisKey)
		}
		_ = rdb.Close()
	} else if rdb != nil {
		_ = rdb.Close()
	}

	if st.ArchivePath != "" {
		if _, err := os.Stat(st.ArchivePath); err == nil {
			if !backend.IsMounted(st.LocalPath) {
				s := startStep("Restoring original directory")
				_ = os.Remove(st.LocalPath)
				if err := os.Rename(st.ArchivePath, st.LocalPath); err != nil {
					s.fail(err.Error())
					fmt.Printf("  %s archive preserved at %s\n", clr(ansiYellow, "!"), st.ArchivePath)
				} else {
					s.succeed(st.LocalPath)
				}
			} else {
				fmt.Printf("  %s mount still active, archive preserved at %s\n", clr(ansiYellow, "!"), st.ArchivePath)
			}
		}
	}

	if st.CreatedLocalPath && st.ArchivePath == "" && !backend.IsMounted(st.LocalPath) {
		removeErr := removeEmptyMountpoint(st.LocalPath)
		if removeErr != nil {
			fmt.Printf("  %s empty mountpoint at %s could not be removed automatically (%v)\n", clr(ansiYellow, "!"), st.LocalPath, removeErr)
		}
	}

	if err := os.Remove(statePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	printBox(markerSuccess+" "+clr(ansiBold, "afs stopped"), []boxRow{
		{Label: "workspace", Value: currentWorkspaceLabel(st.CurrentWorkspace)},
		{Label: "local", Value: st.LocalPath},
	})
	return nil
}

func startServices(cfg config) error {
	ctx := context.Background()

	productMode, err := effectiveProductMode(cfg)
	if err != nil {
		return err
	}

	runtimeCfg := cfg
	workspace := strings.TrimSpace(cfg.CurrentWorkspace)
	mountKey := ""
	mountedHeadSavepoint := ""
	managedSessionClose := func() {}
	managedSessionActive := false
	managedSessionID := ""
	managedHeartbeatEvery := time.Duration(0)

	if productMode == productModeSelfHosted || productMode == productModeCloud {
		prepareStep := startStep("Opening workspace session")
		bootstrap, closeFn, err := prepareMountBootstrap(ctx, cfg)
		if err != nil {
			prepareStep.fail(err.Error())
			return err
		}
		defer closeFn()

		runtimeCfg = bootstrap.cfg
		workspace = bootstrap.workspace
		mountKey = bootstrap.redisKey
		mountedHeadSavepoint = bootstrap.headCheckpoint
		managedSessionID = strings.TrimSpace(bootstrap.sessionID)
		managedHeartbeatEvery = bootstrap.heartbeatEvery
		if managedSessionID != "" {
			managedSessionActive = true
			managedSessionClose = func() {
				closeManagedWorkspaceSession(runtimeCfg, workspace, managedSessionID)
			}
		}
		if bootstrap.initializedRoot {
			prepareStep.succeed(workspace + " (initialized)")
		} else {
			prepareStep.succeed(workspace)
		}
	}
	defer func() {
		if managedSessionActive {
			managedSessionClose()
		}
	}()

	s := startStep("Connecting to Redis")
	rdb := redis.NewClient(buildRedisOptions(runtimeCfg, 4))
	defer rdb.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		s.fail(fmt.Sprintf("cannot reach %s", runtimeCfg.RedisAddr))
		return fmt.Errorf("cannot connect to Redis at %s: %w", runtimeCfg.RedisAddr, err)
	}
	s.succeed(runtimeCfg.RedisAddr)

	backend, backendName, err := backendForConfig(runtimeCfg)
	if err != nil {
		return err
	}
	if backendName == mountBackendNone {
		if managedSessionActive {
			managedSessionClose()
			managedSessionActive = false
			managedSessionID = ""
		}
		st := state{
			StartedAt:            time.Now().UTC(),
			ProductMode:          runtimeCfg.ProductMode,
			ControlPlaneURL:      runtimeCfg.URL,
			ControlPlaneDatabase: runtimeCfg.DatabaseID,
			SessionID:            managedSessionID,
			RedisAddr:            runtimeCfg.RedisAddr,
			RedisDB:              runtimeCfg.RedisDB,
			CurrentWorkspace:     workspace,
			CurrentWorkspaceID:   runtimeCfg.CurrentWorkspaceID,
			MountedHeadSavepoint: mountedHeadSavepoint,
			MountBackend:         backendName,
			ReadOnly:             runtimeCfg.ReadOnly,
			MountLog:             runtimeCfg.MountLog,
			MountBin:             runtimeCfg.MountBin,
		}
		if err := saveState(st); err != nil {
			return err
		}
		runtimeCfg.CurrentWorkspace = workspace
		printReadyBox(runtimeCfg, backendName, "")
		return nil
	}

	store := newAFSStore(rdb)
	if productMode == productModeLocal {
		workspaceStep := startStep("Ensuring current workspace")
		workspace, err = ensureMountWorkspace(ctx, runtimeCfg, store)
		if err != nil {
			workspaceStep.fail(err.Error())
			return fmt.Errorf("a current workspace is required before AFS can mount a filesystem: %w", err)
		}
		workspaceStep.succeed(workspace)
		if err := store.checkImportLock(ctx, workspace); err != nil {
			return fmt.Errorf("cannot mount workspace %q: %w", workspace, err)
		}
		prepareStep := startStep("Opening live workspace")
		seededKey, headSavepoint, initialized, err := seedWorkspaceMountKey(ctx, store, workspace)
		if err != nil {
			prepareStep.fail(err.Error())
			return err
		}
		mountKey = seededKey
		mountedHeadSavepoint = headSavepoint
		if initialized {
			prepareStep.succeed(workspace + " (initialized)")
		} else {
			prepareStep.succeed(workspace)
		}
	} else {
		if err := store.checkImportLock(ctx, workspace); err != nil {
			return fmt.Errorf("cannot mount workspace %q: %w", workspace, err)
		}
	}

	mountCfg := runtimeCfg
	mountCfg.RedisKey = mountKey
	mountCfg, err = prepareRuntimeMountConfig(mountCfg, backendName)
	if err != nil {
		return err
	}

	s = startStep("Mounting filesystem")
	createdLocalPath := false
	if _, statErr := os.Stat(mountCfg.LocalPath); errors.Is(statErr, os.ErrNotExist) {
		createdLocalPath = true
	} else if statErr != nil {
		s.fail(statErr.Error())
		return fmt.Errorf("check mountpoint: %w", statErr)
	}
	if err := os.MkdirAll(mountCfg.LocalPath, 0o755); err != nil {
		s.fail(err.Error())
		return fmt.Errorf("create mountpoint: %w", err)
	}

	started, err := backend.Start(mountCfg)
	if err != nil {
		s.fail(err.Error())
		return err
	}
	if err := backend.WaitForMount(mountCfg, started, 6*time.Second); err != nil {
		s.fail("timeout")
		return fmt.Errorf("mount did not become ready: %w", err)
	}
	s.succeed(mountCfg.LocalPath)

	st := state{
		StartedAt:            time.Now().UTC(),
		ProductMode:          runtimeCfg.ProductMode,
		ControlPlaneURL:      runtimeCfg.URL,
		ControlPlaneDatabase: runtimeCfg.DatabaseID,
		SessionID:            managedSessionID,
		RedisAddr:            runtimeCfg.RedisAddr,
		RedisDB:              runtimeCfg.RedisDB,
		CurrentWorkspace:     workspace,
		CurrentWorkspaceID:   runtimeCfg.CurrentWorkspaceID,
		MountedHeadSavepoint: mountedHeadSavepoint,
		MountPID:             started.PID,
		MountBackend:         backendName,
		ReadOnly:             runtimeCfg.ReadOnly,
		MountEndpoint:        started.Endpoint,
		LocalPath:            mountCfg.LocalPath,
		CreatedLocalPath:     createdLocalPath,
		Mode:                 modeMount,
		RedisKey:             mountCfg.RedisKey,
		MountLog:             runtimeCfg.MountLog,
		MountBin:             runtimeCfg.MountBin,
	}
	if err := saveState(st); err != nil {
		return err
	}

	if managedSessionID != "" && managedHeartbeatEvery > 0 && started.PID > 0 {
		sessionStep := startStep("Starting mount session helper")
		helperPID, err := startMountSessionProcess(runtimeCfg, mountSessionBootstrap{
			Config:                   runtimeCfg,
			Workspace:                workspace,
			SessionID:                managedSessionID,
			HeartbeatIntervalSeconds: int(managedHeartbeatEvery / time.Second),
			MountPID:                 started.PID,
		})
		if err != nil {
			sessionStep.fail(err.Error())
		} else {
			sessionStep.succeed(fmt.Sprintf("pid %d", helperPID))
		}
	}

	managedSessionActive = false

	runtimeCfg.CurrentWorkspace = workspace
	printReadyBox(runtimeCfg, backendName, started.Endpoint)
	return nil
}

func prepareMountBootstrap(ctx context.Context, cfg config) (mountBootstrap, func(), error) {
	resolvedCfg, service, closeFn, err := openAFSControlPlaneForConfig(ctx, cfg)
	if err != nil {
		return mountBootstrap{}, func() {}, err
	}

	selection, err := currentWorkspaceSelectionFromControlPlane(ctx, resolvedCfg, service)
	if err != nil {
		closeFn()
		return mountBootstrap{}, func() {}, fmt.Errorf("a current workspace is required before AFS can mount a filesystem: %w", err)
	}

	sessionInput := managedWorkspaceSessionRequest(resolvedCfg)
	session, err := service.CreateWorkspaceSession(ctx, selection.ID, sessionInput)
	if err != nil {
		closeFn()
		return mountBootstrap{}, func() {}, err
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

	return mountBootstrap{
		cfg:             runtimeCfg,
		workspace:       selection.Name,
		redisKey:        session.RedisKey,
		headCheckpoint:  session.HeadCheckpointID,
		initializedRoot: session.Initialized,
		sessionID:       strings.TrimSpace(session.SessionID),
		heartbeatEvery:  time.Duration(session.HeartbeatIntervalSeconds) * time.Second,
	}, closeFn, nil
}

func shouldCleanLegacyMountCache(st state) bool {
	redisKey := strings.TrimSpace(st.RedisKey)
	if redisKey == "" {
		return false
	}
	workspace := strings.TrimSpace(st.CurrentWorkspace)
	if workspace == "" {
		return true
	}
	ref := strings.TrimSpace(st.CurrentWorkspaceID)
	if ref == "" {
		ref = workspace
	}
	return redisKey != workspaceRedisKey(ref)
}

func removeEmptyMountpoint(path string) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
		return nil
	}
	return err
}

func deleteNamespace(ctx context.Context, rdb *redis.Client, fsKey string) error {
	patterns := []string{
		"afs:{" + fsKey + "}:*",
		"rfs:{" + fsKey + "}:*",
	}
	for _, pattern := range patterns {
		var cursor uint64
		for {
			keys, next, err := rdb.Scan(ctx, cursor, pattern, 500).Result()
			if err != nil {
				return err
			}
			if len(keys) > 0 {
				if err := rdb.Del(ctx, keys...).Err(); err != nil {
					return err
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	return nil
}

func terminatePID(pid int, timeout time.Duration) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	_ = p.Signal(syscall.SIGTERM)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = p.Signal(syscall.SIGKILL)
	return nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func orphanMountDaemonPIDs(st state) ([]int, error) {
	out, err := exec.Command("ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return nil, err
	}
	return parseOrphanMountDaemonPIDs(st, string(out)), nil
}

func parseOrphanMountDaemonPIDs(st state, psOutput string) []int {
	backendName := strings.TrimSpace(st.MountBackend)
	if backendName == "" || backendName == mountBackendNone {
		return nil
	}

	var matches []int
	for _, rawLine := range strings.Split(psOutput, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 || pid == st.MountPID {
			continue
		}
		command := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
		if mountDaemonMatchesState(backendName, st, command) {
			matches = append(matches, pid)
		}
	}
	sort.Ints(matches)
	return matches
}

func mountDaemonMatchesState(backendName string, st state, command string) bool {
	switch backendName {
	case mountBackendNFS:
		if !strings.Contains(command, "agent-filesystem-nfs") {
			return false
		}
		return strings.Contains(command, "--redis "+st.RedisAddr) &&
			strings.Contains(command, "--db "+strconv.Itoa(st.RedisDB)) &&
			strings.Contains(command, "--export "+nfsExportPath(st.RedisKey))
	case mountBackendFuse:
		if !strings.Contains(command, "agent-filesystem-mount") {
			return false
		}
		return strings.Contains(command, " "+st.RedisKey+" "+st.LocalPath)
	default:
		return false
	}
}
