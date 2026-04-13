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

	cfg, err := loadConfigForUpWithMode(opts.positionals, opts.mode)
	if err != nil {
		return err
	}
	mode, err := effectiveMode(cfg)
	if err != nil {
		return err
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
			fmt.Println()
			fmt.Println("  AFS is not running. Nothing to stop.")
			fmt.Println()
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

	fmt.Printf("\n  %s afs stopped\n\n", clr(ansiDim, "■"))
	return nil
}

func startServices(cfg config) error {
	ctx := context.Background()

	s := startStep("Connecting to Redis")
	rdb := redis.NewClient(buildRedisOptions(cfg, 4))
	defer rdb.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		s.fail(fmt.Sprintf("cannot reach %s", cfg.RedisAddr))
		return fmt.Errorf("cannot connect to Redis at %s: %w", cfg.RedisAddr, err)
	}
	s.succeed(cfg.RedisAddr)

	backend, backendName, err := backendForConfig(cfg)
	if err != nil {
		return err
	}
	if backendName == mountBackendNone {
		st := state{
			StartedAt:        time.Now().UTC(),
			RedisAddr:        cfg.RedisAddr,
			RedisDB:          cfg.RedisDB,
			CurrentWorkspace: cfg.CurrentWorkspace,
			MountBackend:     backendName,
			ReadOnly:         cfg.ReadOnly,
			MountLog:         cfg.MountLog,
			MountBin:         cfg.MountBin,
		}
		if err := saveState(st); err != nil {
			return err
		}
		printReadyBox(cfg, backendName, "")
		return nil
	}

	store := newAFSStore(rdb)
	workspaceStep := startStep("Ensuring current workspace")
	workspace, err := ensureMountWorkspace(ctx, cfg, store)
	if err != nil {
		workspaceStep.fail(err.Error())
		return fmt.Errorf("a current workspace is required before AFS can mount a filesystem: %w", err)
	}
	workspaceStep.succeed(workspace)
	if err := store.checkImportLock(ctx, workspace); err != nil {
		return fmt.Errorf("cannot mount workspace %q: %w", workspace, err)
	}
	prepareStep := startStep("Opening live workspace")
	mountKey, mountedHeadSavepoint, initialized, err := seedWorkspaceMountKey(ctx, store, workspace)
	if err != nil {
		prepareStep.fail(err.Error())
		return err
	}
	if initialized {
		prepareStep.succeed(workspace + " (initialized)")
	} else {
		prepareStep.succeed(workspace)
	}

	mountCfg := cfg
	mountCfg.RedisKey = mountKey
	mountCfg, err = prepareRuntimeMountConfig(mountCfg, backendName)
	if err != nil {
		prepareStep.fail(err.Error())
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
		RedisAddr:            cfg.RedisAddr,
		RedisDB:              cfg.RedisDB,
		CurrentWorkspace:     workspace,
		MountedHeadSavepoint: mountedHeadSavepoint,
		MountPID:             started.PID,
		MountBackend:         backendName,
		ReadOnly:             cfg.ReadOnly,
		MountEndpoint:        started.Endpoint,
		LocalPath:            mountCfg.LocalPath,
		CreatedLocalPath:     createdLocalPath,
		Mode:                 modeMount,
		RedisKey:             mountCfg.RedisKey,
		MountLog:             cfg.MountLog,
		MountBin:             cfg.MountBin,
	}
	if err := saveState(st); err != nil {
		return err
	}

	cfg.CurrentWorkspace = workspace
	printReadyBox(cfg, backendName, started.Endpoint)
	return nil
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
	return redisKey != workspaceRedisKey(workspace)
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
