package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	mountBackendAuto = "auto"
	mountBackendNone = "none"
	mountBackendFuse = "fuse"
	mountBackendNFS  = "nfs"
)

type mountStartResult struct {
	PID      int
	Endpoint string
}

type mountBackend interface {
	Name() string
	Start(cfg config) (mountStartResult, error)
	WaitForMount(cfg config, started mountStartResult, timeout time.Duration) error
	IsMounted(mountpoint string) bool
	Unmount(mountpoint string) error
}

func defaultMountBackend() string {
	if runtime.GOOS == "darwin" {
		return mountBackendNFS
	}
	return mountBackendFuse
}

func normalizeMountBackend(v string) (string, error) {
	b := strings.ToLower(strings.TrimSpace(v))
	if b == "" || b == mountBackendAuto {
		return defaultMountBackend(), nil
	}
	switch b {
	case mountBackendNone, mountBackendFuse, mountBackendNFS:
		return b, nil
	default:
		return "", fmt.Errorf("unsupported mount backend %q (expected auto, none, fuse, or nfs)", v)
	}
}

func backendForConfig(cfg config) (mountBackend, string, error) {
	name, err := normalizeMountBackend(cfg.MountBackend)
	if err != nil {
		return nil, "", err
	}
	b, err := backendByName(name)
	if err != nil {
		return nil, "", err
	}
	return b, name, nil
}

func backendForState(st state) (mountBackend, string, error) {
	name := st.MountBackend
	if name == "" {
		name = mountBackendFuse
	}
	b, err := backendByName(name)
	if err != nil {
		return nil, "", err
	}
	return b, name, nil
}

func backendByName(name string) (mountBackend, error) {
	switch name {
	case mountBackendNone:
		return noMountBackend{}, nil
	case mountBackendFuse:
		return fuseBackend{}, nil
	case mountBackendNFS:
		return nfsBackend{}, nil
	default:
		return nil, fmt.Errorf("unsupported mount backend %q", name)
	}
}

type noMountBackend struct{}

func (n noMountBackend) Name() string { return mountBackendNone }

func (n noMountBackend) Start(cfg config) (mountStartResult, error) {
	return mountStartResult{}, nil
}

func (n noMountBackend) WaitForMount(cfg config, started mountStartResult, timeout time.Duration) error {
	return nil
}

func (n noMountBackend) IsMounted(mountpoint string) bool {
	return false
}

func (n noMountBackend) Unmount(mountpoint string) error {
	return nil
}

type fuseBackend struct{}

func (f fuseBackend) Name() string { return mountBackendFuse }

func (f fuseBackend) Start(cfg config) (mountStartResult, error) {
	if err := os.MkdirAll(filepathDir(cfg.MountLog), 0o755); err != nil {
		return mountStartResult{}, err
	}
	logFile, err := os.OpenFile(cfg.MountLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return mountStartResult{}, err
	}
	defer logFile.Close()

	args := []string{
		"--redis", cfg.RedisAddr,
		"--db", strconv.Itoa(cfg.RedisDB),
		"--foreground",
		cfg.RedisKey,
		cfg.Mountpoint,
	}
	if cfg.RedisUsername != "" {
		args = append([]string{"--user", cfg.RedisUsername}, args...)
	}
	if cfg.RedisPassword != "" {
		args = append([]string{"--password", cfg.RedisPassword}, args...)
	}
	if cfg.RedisTLS {
		args = append([]string{"--tls"}, args...)
	}
	if cfg.ReadOnly {
		args = append([]string{"--readonly"}, args...)
	}
	if cfg.AllowOther {
		args = append([]string{"--allow-other"}, args...)
	}

	cmd := exec.Command(cfg.MountBin, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if devNull, err := os.Open(os.DevNull); err == nil {
		defer devNull.Close()
		cmd.Stdin = devNull
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return mountStartResult{}, fmt.Errorf("start mount failed: %w", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return mountStartResult{PID: pid}, nil
}

func (f fuseBackend) WaitForMount(cfg config, _ mountStartResult, timeout time.Duration) error {
	return waitForMountpoint(cfg.Mountpoint, timeout, f.IsMounted)
}

func (f fuseBackend) IsMounted(mountpoint string) bool {
	return mountTableContains(mountpoint)
}

func (f fuseBackend) Unmount(mountpoint string) error {
	cmds := [][]string{{"fusermount", "-u", mountpoint}, {"fusermount", "-uz", mountpoint}, {"umount", mountpoint}}
	if runtime.GOOS == "darwin" {
		cmds = append(cmds, []string{"umount", "-f", mountpoint}, []string{"diskutil", "unmount", "force", mountpoint})
	} else {
		cmds = append(cmds, []string{"umount", "-l", mountpoint})
	}
	for _, c := range cmds {
		if exec.Command(c[0], c[1:]...).Run() == nil {
			return nil
		}
	}
	return errors.New("all unmount commands failed")
}

type nfsBackend struct{}

func (n nfsBackend) Name() string { return mountBackendNFS }

func nfsExportPath(redisKey string) string {
	trimmed := strings.Trim(redisKey, " /")
	if trimmed == "" {
		return "/fs"
	}
	return "/" + trimmed
}

func (n nfsBackend) Start(cfg config) (mountStartResult, error) {
	if err := os.MkdirAll(filepathDir(cfg.MountLog), 0o755); err != nil {
		return mountStartResult{}, err
	}
	logFile, err := os.OpenFile(cfg.MountLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return mountStartResult{}, err
	}
	defer logFile.Close()

	host := cfg.NFSHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.NFSPort
	if port <= 0 {
		port = 20490
	}
	listenAddr := net.JoinHostPort(host, strconv.Itoa(port))
	probe, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return mountStartResult{}, fmt.Errorf("nfs listen address %s is already in use: %w", listenAddr, err)
	}
	_ = probe.Close()
	export := nfsExportPath(cfg.RedisKey)

	args := []string{
		"--redis", cfg.RedisAddr,
		"--db", strconv.Itoa(cfg.RedisDB),
		"--listen", listenAddr,
		"--export", export,
		"--foreground",
	}
	if cfg.RedisUsername != "" {
		args = append([]string{"--user", cfg.RedisUsername}, args...)
	}
	if cfg.RedisPassword != "" {
		args = append([]string{"--password", cfg.RedisPassword}, args...)
	}
	if cfg.RedisTLS {
		args = append([]string{"--tls"}, args...)
	}
	if cfg.ReadOnly {
		args = append([]string{"--readonly"}, args...)
	}

	cmd := exec.Command(cfg.NFSBin, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if devNull, err := os.Open(os.DevNull); err == nil {
		defer devNull.Close()
		cmd.Stdin = devNull
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return mountStartResult{}, fmt.Errorf("start nfs gateway failed: %w", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	endpoint := fmt.Sprintf("%s:%s", host, export)
	return mountStartResult{PID: pid, Endpoint: endpoint}, nil
}

func (n nfsBackend) WaitForMount(cfg config, started mountStartResult, timeout time.Duration) error {
	addr := cfg.NFSHost
	if addr == "" {
		addr = "127.0.0.1"
	}
	port := cfg.NFSPort
	if port <= 0 {
		port = 20490
	}
	server := net.JoinHostPort(addr, strconv.Itoa(port))

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", server, 250*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(150 * time.Millisecond)
	}

	var lastMountErr error
	for time.Now().Before(deadline) {
		if started.PID > 0 && !processAlive(started.PID) {
			return fmt.Errorf("nfs gateway exited before mount completed; see %s", cfg.MountLog)
		}
		if n.IsMounted(cfg.Mountpoint) {
			return nil
		}
		if err := n.mountLocal(cfg, started.Endpoint); err != nil {
			lastMountErr = err
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if err := waitForMountpoint(cfg.Mountpoint, 1200*time.Millisecond, n.IsMounted); err == nil {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastMountErr != nil {
		return lastMountErr
	}
	return waitForMountpoint(cfg.Mountpoint, timeout, n.IsMounted)
}

func (n nfsBackend) mountLocal(cfg config, endpoint string) error {
	serverPath := endpoint
	if serverPath == "" {
		host := cfg.NFSHost
		if host == "" {
			host = "127.0.0.1"
		}
		serverPath = fmt.Sprintf("%s:%s", host, nfsExportPath(cfg.RedisKey))
	}
	port := cfg.NFSPort
	if port <= 0 {
		port = 20490
	}
	if err := os.MkdirAll(cfg.Mountpoint, 0o755); err != nil {
		return fmt.Errorf("create mountpoint: %w", err)
	}

	if runtime.GOOS == "darwin" {
		opts := darwinNFSMountOptions(port)
		cmd := exec.Command("/sbin/mount_nfs", "-o", opts, serverPath, cfg.Mountpoint)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("mount_nfs failed: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	opts := fmt.Sprintf("vers=3,tcp,port=%d,mountport=%d,nolock", port, port)
	cmd := exec.Command("mount", "-t", "nfs", "-o", opts, serverPath, cfg.Mountpoint)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mount -t nfs failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (n nfsBackend) IsMounted(mountpoint string) bool {
	return mountTableContains(mountpoint)
}

func (n nfsBackend) Unmount(mountpoint string) error {
	cmds := [][]string{{"umount", mountpoint}}
	if runtime.GOOS == "darwin" {
		// macOS: -f for force unmount; diskutil as a last resort
		cmds = append(cmds,
			[]string{"umount", "-f", mountpoint},
			[]string{"diskutil", "unmount", "force", mountpoint},
		)
	} else {
		// Linux: -l for lazy unmount of stale NFS mounts
		cmds = append(cmds, []string{"umount", "-l", mountpoint})
	}
	for _, c := range cmds {
		if exec.Command(c[0], c[1:]...).Run() == nil {
			return nil
		}
	}
	return errors.New("all unmount commands failed")
}

func waitForMountpoint(mountpoint string, timeout time.Duration, mountedFn func(string) bool) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if mountedFn(mountpoint) {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return errors.New("timeout waiting for mount")
}

func mountTableContains(mountpoint string) bool {
	_, ok := mountTableEntry(mountpoint)
	return ok
}

func mountTableEntry(mountpoint string) (string, bool) {
	out, err := exec.Command("mount").Output()
	if err == nil {
		needle := " on " + mountpoint + " "
		for _, ln := range strings.Split(string(out), "\n") {
			if strings.Contains(ln, needle) {
				return ln, true
			}
		}
	}

	if runtime.GOOS == "linux" {
		b, err := os.ReadFile("/proc/mounts")
		if err == nil {
			for _, ln := range strings.Split(string(b), "\n") {
				fields := strings.Fields(ln)
				if len(fields) >= 2 && fields[1] == mountpoint {
					return ln, true
				}
			}
		}
	}
	return "", false
}

func filepathDir(p string) string {
	if p == "" {
		return "."
	}
	return filepath.Dir(p)
}

func darwinNFSMountOptions(port int) string {
	// Disable attribute caching and force synchronous writes for localhost AFS
	// mounts so data is visible immediately after close. Also disable negative
	// name caching to reduce transient "both names missing" windows during
	// rapid rename churn on macOS.
	return fmt.Sprintf("vers=3,tcp,port=%d,mountport=%d,nolocks,noac,nonegnamecache,sync", port, port)
}
