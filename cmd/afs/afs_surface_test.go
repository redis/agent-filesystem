package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestConfigPathDefaultsToAFSConfig(t *testing.T) {
	t.Helper()

	orig := cfgPathOverride
	cfgPathOverride = ""
	defer func() {
		cfgPathOverride = orig
	}()

	if got := filepath.Base(configPath()); got != "afs.config.json" {
		t.Fatalf("configPath() basename = %q, want %q", got, "afs.config.json")
	}
}

func TestCompactDisplayPathUsesParentAndFilename(t *testing.T) {
	t.Helper()

	got := compactDisplayPath("/Users/example/.afs/afs.config.json")
	want := filepath.Join(".afs", "afs.config.json")
	if got != want {
		t.Fatalf("compactDisplayPath() = %q, want %q", got, want)
	}

	if got := compactDisplayPath("afs.config.json"); got != "afs.config.json" {
		t.Fatalf("compactDisplayPath(single file) = %q, want %q", got, "afs.config.json")
	}
}

func TestStateDirAndWorkRootUseAFSHome(t *testing.T) {
	t.Helper()

	dir := stateDir()
	if !strings.HasSuffix(dir, string(filepath.Separator)+".afs") {
		t.Fatalf("stateDir() = %q, want suffix %q", dir, string(filepath.Separator)+".afs")
	}

	wantWorkRoot := filepath.Join(dir, "workspaces")
	if got := defaultWorkRoot(); got != wantWorkRoot {
		t.Fatalf("defaultWorkRoot() = %q, want %q", got, wantWorkRoot)
	}
}

func TestDefaultConfigUsesAFSDefaults(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	if cfg.WorkRoot != defaultWorkRoot() {
		t.Fatalf("WorkRoot = %q, want %q", cfg.WorkRoot, defaultWorkRoot())
	}
	if cfg.LocalPath != "~/afs" {
		t.Fatalf("Mountpoint = %q, want %q", cfg.LocalPath, "~/afs")
	}
	if cfg.RedisLog != "/tmp/afs-redis.log" {
		t.Fatalf("RedisLog = %q, want %q", cfg.RedisLog, "/tmp/afs-redis.log")
	}
	if cfg.MountLog != "/tmp/afs-mount.log" {
		t.Fatalf("MountLog = %q, want %q", cfg.MountLog, "/tmp/afs-mount.log")
	}
}

func TestExecutablePathResolvesSymlinks(t *testing.T) {
	t.Helper()

	realDir := t.TempDir()
	linkDir := t.TempDir()
	realBin := filepath.Join(realDir, "afs")
	linkBin := filepath.Join(linkDir, "afs")

	if err := os.WriteFile(realBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) returned error: %v", realBin, err)
	}
	if err := os.Symlink(realBin, linkBin); err != nil {
		t.Fatalf("Symlink(%q, %q) returned error: %v", realBin, linkBin, err)
	}

	got := resolveExecutablePath(linkBin)
	gotInfo, err := os.Stat(got)
	if err != nil {
		t.Fatalf("Stat(%q) returned error: %v", got, err)
	}
	wantInfo, err := os.Stat(realBin)
	if err != nil {
		t.Fatalf("Stat(%q) returned error: %v", realBin, err)
	}
	if !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("resolveExecutablePath(%q) = %q, want same file as %q", linkBin, got, realBin)
	}
}

func TestStatusRowsNoMountBackendWhenNone(t *testing.T) {
	t.Helper()

	rows := statusRows("myws", "/tmp/local", "sync", mountBackendNone, "localhost:6379", 0)
	// Expect: workspace, local, database, mode — no mount backend row.
	labels := make([]string, len(rows))
	for i, r := range rows {
		labels[i] = r.Label
	}
	for _, l := range labels {
		if l == "mount backend" {
			t.Fatalf("statusRows() should not include mount backend for backendNone, got labels %v", labels)
		}
	}
	if rows[0].Label != "workspace" || rows[0].Value != "myws" {
		t.Fatalf("rows[0] = %+v, want workspace=myws", rows[0])
	}
	if rows[2].Label != "database" || rows[2].Value != "redis://localhost:6379/0" {
		t.Fatalf("rows[2] = %+v, want database row", rows[2])
	}
}

func TestStatusRowsIncludesMountBackendForFuse(t *testing.T) {
	t.Helper()

	rows := statusRows("myws", "/tmp/local", "mount", mountBackendFuse, "localhost:6379", 0)
	found := false
	for _, r := range rows {
		if r.Label == "mount backend" && r.Value == "FUSE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("statusRows() missing mount backend=FUSE row")
	}
}

func TestStatusTitleShowsRunningWithPID(t *testing.T) {
	t.Helper()

	title := statusTitle("●", 12345)
	want := "AFS Running (daemon 12345)"
	if !strings.Contains(title, want) {
		t.Fatalf("statusTitle() = %q, want substring %q", title, want)
	}
}

func TestStatusTitleShowsRunningWithoutPID(t *testing.T) {
	t.Helper()

	title := statusTitle("●", 0)
	want := "AFS Running"
	if !strings.Contains(title, want) {
		t.Fatalf("statusTitle() = %q, want substring %q", title, want)
	}
	if strings.Contains(title, "daemon") {
		t.Fatalf("statusTitle() = %q, should not contain daemon when pid=0", title)
	}
}

func TestPrintReadyBoxUsesMountedWorkspaceTitle(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.RedisAddr = "localhost:6379"
	cfg.RedisDB = 0
	cfg.CurrentWorkspace = "newfiles"
	cfg.LocalPath = "/Users/rowantrollope/abc"

	out, err := captureStdout(t, func() error {
		printReadyBox(cfg, mountBackendNFS, "")
		return nil
	})
	if err != nil {
		t.Fatalf("printReadyBox() returned error: %v", err)
	}

	want := "AFS Running"
	if !strings.Contains(out, want) {
		t.Fatalf("printReadyBox() output = %q, want substring %q", out, want)
	}
	if !strings.Contains(out, "newfiles") {
		t.Fatalf("printReadyBox() output = %q, want workspace name in rows", out)
	}
}

func TestPrintReadyBoxKeepsVisibleLinesWithinEightyColumns(t *testing.T) {
	t.Helper()

	origColorTerm := colorTerm
	colorTerm = true
	t.Cleanup(func() {
		colorTerm = origColorTerm
	})

	cfg := defaultConfig()
	cfg.RedisAddr = "localhost:6379"
	cfg.RedisDB = 0
	cfg.CurrentWorkspace = "workspace-with-a-very-long-name-for-status-output"
	cfg.LocalPath = "/Users/example/Library/Application Support/Agent Filesystem/projects/customer-success/super-long-nested-workspace-path"

	out, err := captureStdout(t, func() error {
		printReadyBox(cfg, mountBackendNFS, "")
		return nil
	})
	if err != nil {
		t.Fatalf("printReadyBox() returned error: %v", err)
	}

	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if strings.TrimSpace(stripAnsi(line)) == "" {
			continue
		}
		if width := runeWidth(line); width > maxCLIWidth {
			t.Fatalf("line width = %d, want <= %d: %q", width, maxCLIWidth, stripAnsi(line))
		}
	}
}

func TestCenterBannerTextForOutputCentersTextWithinBannerWidth(t *testing.T) {
	t.Helper()

	got := stripAnsi(centerBannerTextForOutput(io.Discard, "AFS"))
	want := bannerIndent + strings.Repeat(" ", (bannerWidth-len("AFS"))/2) + "AFS"
	if got != want {
		t.Fatalf("centerBannerTextForOutput(AFS) = %q, want %q", got, want)
	}

	subtitle := "Agent Filesystem"
	got = stripAnsi(centerBannerTextForOutput(io.Discard, subtitle))
	want = bannerIndent + strings.Repeat(" ", (bannerWidth-len(subtitle))/2) + subtitle
	if got != want {
		t.Fatalf("centerBannerTextForOutput(subtitle) = %q, want %q", got, want)
	}
}

func TestPrintBannerCompactIncludesSubtitle(t *testing.T) {
	t.Helper()

	out, err := captureStderr(t, func() error {
		printBannerCompact()
		return nil
	})
	if err != nil {
		t.Fatalf("printBannerCompact() returned error: %v", err)
	}
	if !strings.Contains(out, "Agent Filesystem") {
		t.Fatalf("printBannerCompact() output = %q, want subtitle", out)
	}
}

func TestCmdUpShowsStatusWhenAlreadyRunning(t *testing.T) {
	t.Helper()

	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv(HOME) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})

	st := state{
		StartedAt:        time.Now().UTC(),
		ManageRedis:      true,
		RedisPID:         os.Getpid(),
		RedisAddr:        "localhost:6379",
		RedisDB:          0,
		CurrentWorkspace: "demo",
		MountBackend:     mountBackendNone,
	}
	if err := saveState(st); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	out, err := captureStdout(t, cmdUp)
	if err != nil {
		t.Fatalf("cmdUp() returned error: %v", err)
	}
	if !strings.Contains(out, "AFS Running") {
		t.Fatalf("cmdUp() output = %q, want status output", out)
	}
	if strings.Contains(out, "Run 'afs down' first") {
		t.Fatalf("cmdUp() output still contains old already-running error: %q", out)
	}
}

func TestCmdUpDoesNotPrintBannerWhenStarting(t *testing.T) {
	t.Helper()

	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv(HOME) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.UseExistingRedis = true
	cfg.RedisAddr = mr.Addr()
	cfg.Mode = modeMount // this test specifically exercises the no-mount legacy path
	cfg.MountBackend = mountBackendNone
	cfg.CurrentWorkspace = "demo"
	cfg.WorkRoot = t.TempDir()
	saveTempConfig(t, cfg)

	out, err := captureStdout(t, func() error {
		return cmdUpArgs([]string{})
	})
	if err != nil {
		t.Fatalf("cmdUpArgs() returned error: %v", err)
	}
	if strings.Contains(out, "Agent Filesystem") || strings.Contains(out, "AFS\n") {
		t.Fatalf("cmdUpArgs() output = %q, did not expect banner", out)
	}
	if !strings.Contains(out, "AFS Running") {
		t.Fatalf("cmdUpArgs() output = %q, want ready output", out)
	}
}

func TestCmdDownStopsWithoutSavingMountedWorkspace(t *testing.T) {
	t.Helper()

	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv(HOME) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})

	mr := miniredis.RunT(t)
	mountpoint := filepath.Join(t.TempDir(), "mount")
	if err := os.MkdirAll(mountpoint, 0o755); err != nil {
		t.Fatalf("MkdirAll(mountpoint) returned error: %v", err)
	}

	st := state{
		StartedAt:            time.Now().UTC(),
		ManageRedis:          false,
		RedisAddr:            mr.Addr(),
		RedisDB:              0,
		CurrentWorkspace:     "demo",
		MountedHeadSavepoint: "initial",
		MountBackend:         mountBackendFuse,
		LocalPath:            mountpoint,
		CreatedLocalPath:     true,
		RedisKey:             workspaceRedisKey("demo"),
	}
	if err := saveState(st); err != nil {
		t.Fatalf("saveState() returned error: %v", err)
	}

	out, err := captureStdout(t, cmdDown)
	if err != nil {
		t.Fatalf("cmdDown() returned error: %v", err)
	}

	if strings.Contains(out, "Saving mounted workspace") {
		t.Fatalf("cmdDown() output = %q, want no mounted workspace save step", out)
	}
	if !strings.Contains(out, "afs stopped") {
		t.Fatalf("cmdDown() output = %q, want stopped message", out)
	}
	if _, err := os.Stat(mountpoint); !os.IsNotExist(err) {
		t.Fatalf("mountpoint should be removed after cmdDown(), stat err = %v", err)
	}
	if _, err := os.Stat(statePath()); !os.IsNotExist(err) {
		t.Fatalf("statePath() should be removed after cmdDown(), stat err = %v", err)
	}
}

func TestParseOrphanMountDaemonPIDsMatchesNFSDaemonsForSameWorkspace(t *testing.T) {
	t.Helper()

	st := state{
		MountPID:     222,
		MountBackend: mountBackendNFS,
		RedisAddr:    "redis.example:6379",
		RedisDB:      0,
		RedisKey:     "claude",
	}

	psOutput := strings.Join([]string{
		"111 /Users/example/agent-filesystem-nfs --redis redis.example:6379 --db 0 --listen 127.0.0.1:20490 --export /claude --foreground",
		"222 /Users/example/agent-filesystem-nfs --redis redis.example:6379 --db 0 --listen 127.0.0.1:20491 --export /claude --foreground",
		"333 /Users/example/agent-filesystem-nfs --redis redis.example:6379 --db 0 --listen 127.0.0.1:20492 --export /other --foreground",
		"444 /Users/example/agent-filesystem-nfs --redis other.example:6379 --db 0 --listen 127.0.0.1:20493 --export /claude --foreground",
	}, "\n")

	got := parseOrphanMountDaemonPIDs(st, psOutput)
	if len(got) != 1 || got[0] != 111 {
		t.Fatalf("parseOrphanMountDaemonPIDs() = %#v, want [111]", got)
	}
}

func TestParseOrphanMountDaemonPIDsMatchesFuseDaemonsForSameMountpoint(t *testing.T) {
	t.Helper()

	st := state{
		MountPID:     200,
		MountBackend: mountBackendFuse,
		RedisKey:     "demo",
		LocalPath:    "/tmp/demo",
	}

	psOutput := strings.Join([]string{
		"101 /Users/example/agent-filesystem-mount --foreground demo /tmp/demo",
		"200 /Users/example/agent-filesystem-mount --foreground demo /tmp/demo",
		"303 /Users/example/agent-filesystem-mount --foreground demo /tmp/other",
	}, "\n")

	got := parseOrphanMountDaemonPIDs(st, psOutput)
	if len(got) != 1 || got[0] != 101 {
		t.Fatalf("parseOrphanMountDaemonPIDs() = %#v, want [101]", got)
	}
}

func captureStderr(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	origStderr := os.Stderr
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() returned error: %v", err)
	}
	os.Stderr = writePipe
	runErr := fn()
	_ = writePipe.Close()
	os.Stderr = origStderr

	out, readErr := io.ReadAll(readPipe)
	_ = readPipe.Close()
	if readErr != nil {
		t.Fatalf("io.ReadAll() returned error: %v", readErr)
	}
	return string(out), runErr
}
