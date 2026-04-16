package main

import (
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
)

const mountSessionBootstrapEnv = "AFS_MOUNT_SESSION_BOOTSTRAP"

type mountSessionBootstrap struct {
	Config                   config `json:"config"`
	Workspace                string `json:"workspace"`
	SessionID                string `json:"session_id,omitempty"`
	HeartbeatIntervalSeconds int    `json:"heartbeat_interval_seconds,omitempty"`
	MountPID                 int    `json:"mount_pid"`
}

func startMountSessionProcess(cfg config, bootstrap mountSessionBootstrap) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("cannot find own executable: %w", err)
	}

	logPath := cfg.MountLog
	if strings.TrimSpace(logPath) == "" {
		logPath = "/tmp/afs-mount.log"
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return 0, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer logFile.Close()

	args := []string{"_mount-session"}
	if cfgPathOverride != "" {
		args = []string{"--config", cfgPathOverride, "_mount-session"}
	}

	cmd := exec.Command(exe, args...)
	cmd.Env = os.Environ()
	bootstrapPath, err := writeMountSessionBootstrap(bootstrap)
	if err != nil {
		return 0, err
	}
	cmd.Env = append(cmd.Env, mountSessionBootstrapEnv+"="+bootstrapPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if devNull, err := os.Open(os.DevNull); err == nil {
		defer devNull.Close()
		cmd.Stdin = devNull
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		_ = os.Remove(bootstrapPath)
		return 0, fmt.Errorf("start mount session helper: %w", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}

func runMountSessionDaemon() error {
	cfg, workspace, sessionID, heartbeatEvery, mountPID, err := loadMountSessionRuntime()
	if err != nil {
		return err
	}
	if heartbeatEvery <= 0 {
		heartbeatEvery = 20 * time.Second
	}

	stopSessionLifecycle, err := startManagedWorkspaceSessionLifecycle(cfg, workspace, sessionID, heartbeatEvery)
	if err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			stopSessionLifecycle()
			return nil
		case <-ticker.C:
			if mountPID > 0 && !processAlive(mountPID) {
				stopSessionLifecycle()
				return nil
			}
		}
	}
}

func loadMountSessionRuntime() (config, string, string, time.Duration, int, error) {
	bootstrap, ok, err := loadMountSessionBootstrap()
	if err != nil {
		return config{}, "", "", 0, 0, err
	}
	if !ok {
		return config{}, "", "", 0, 0, errors.New("mount session bootstrap is missing")
	}

	cfg := bootstrap.Config
	if err := resolveConfigPaths(&cfg); err != nil {
		return config{}, "", "", 0, 0, fmt.Errorf("resolve mount bootstrap config: %w", err)
	}
	workspace := strings.TrimSpace(bootstrap.Workspace)
	if workspace == "" {
		return config{}, "", "", 0, 0, errors.New("mount session bootstrap is missing workspace")
	}
	sessionID := strings.TrimSpace(bootstrap.SessionID)
	if sessionID == "" {
		return config{}, "", "", 0, 0, errors.New("mount session bootstrap is missing session id")
	}
	return cfg, workspace, sessionID, time.Duration(bootstrap.HeartbeatIntervalSeconds) * time.Second, bootstrap.MountPID, nil
}

func writeMountSessionBootstrap(bootstrap mountSessionBootstrap) (string, error) {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return "", err
	}
	raw, err := json.Marshal(bootstrap)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp(stateDir(), ".mount-session-bootstrap-*.json")
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

func loadMountSessionBootstrap() (mountSessionBootstrap, bool, error) {
	path := strings.TrimSpace(os.Getenv(mountSessionBootstrapEnv))
	if path == "" {
		return mountSessionBootstrap{}, false, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return mountSessionBootstrap{}, false, fmt.Errorf("read mount session bootstrap: %w", err)
	}
	_ = os.Remove(path)
	var bootstrap mountSessionBootstrap
	if err := json.Unmarshal(raw, &bootstrap); err != nil {
		return mountSessionBootstrap{}, false, fmt.Errorf("parse mount session bootstrap: %w", err)
	}
	return bootstrap, true, nil
}
