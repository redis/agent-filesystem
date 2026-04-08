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
	if cfg.RuntimeMode != "host" {
		t.Fatalf("RuntimeMode = %q, want %q", cfg.RuntimeMode, "host")
	}
	if cfg.Mountpoint != "~/afs" {
		t.Fatalf("Mountpoint = %q, want %q", cfg.Mountpoint, "~/afs")
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

func TestStatusRowsUseNoMountLabelsWhenFilesystemMountIsNone(t *testing.T) {
	t.Helper()

	orig := cfgPathOverride
	cfgPathOverride = filepath.Join("/Users/example/Library/Application Support/AFS", ".afs", "afs.config.json")
	defer func() {
		cfgPathOverride = orig
	}()

	rows := statusRows(mountBackendNone, "/tmp/workspaces", "localhost:6379", 0, "demo")
	if len(rows) != 3 {
		t.Fatalf("len(statusRows()) = %d, want 3", len(rows))
	}
	if rows[0].Label != "redis" {
		t.Fatalf("rows[0].Label = %q, want %q", rows[0].Label, "redis")
	}
	if rows[0].Value != "redis://localhost:6379 (db 0)" {
		t.Fatalf("rows[0].Value = %q, want %q", rows[0].Value, "redis://localhost:6379 (db 0)")
	}
	if rows[1].Label != "current workspace" || rows[1].Value != "demo" {
		t.Fatalf("rows[1] = %+v, want current workspace row", rows[1])
	}
	if rows[2].Label != "config" || !strings.Contains(rows[2].Value, filepath.Join(".afs", "afs.config.json")) {
		t.Fatalf("rows[2] = %+v, want config row", rows[2])
	}
	if strings.Contains(stripAnsi(rows[2].Value), "/Users/example/") {
		t.Fatalf("rows[2] = %+v, did not expect full absolute path", rows[2])
	}
}

func TestStatusRowsKeepFilesystemLabelsForMountMode(t *testing.T) {
	t.Helper()

	orig := cfgPathOverride
	cfgPathOverride = filepath.Join("/Users/example/Library/Application Support/AFS", ".afs", "afs.config.json")
	defer func() {
		cfgPathOverride = orig
	}()

	rows := statusRows(mountBackendFuse, "/tmp/mount", "localhost:6379", 0, "")
	if len(rows) != 4 {
		t.Fatalf("len(statusRows()) = %d, want 4", len(rows))
	}
	if rows[0].Label != "local filesystem" || rows[0].Value != "/tmp/mount" {
		t.Fatalf("rows[0] = %+v, want local filesystem row", rows[0])
	}
	if rows[1].Label != "redis" || rows[1].Value != "redis://localhost:6379 (db 0)" {
		t.Fatalf("rows[1] = %+v, want redis row", rows[1])
	}
	if rows[2].Label != "current workspace" || rows[2].Value != "none" {
		t.Fatalf("rows[2] = %+v, want current workspace row", rows[2])
	}
	if rows[3].Label != "config" || !strings.Contains(rows[3].Value, filepath.Join(".afs", "afs.config.json")) {
		t.Fatalf("rows[3] = %+v, want config row", rows[3])
	}
	if strings.Contains(stripAnsi(rows[3].Value), "/Users/example/") {
		t.Fatalf("rows[3] = %+v, did not expect full absolute path", rows[3])
	}
}

func TestStatusTitleUsesMountedWorkspaceWording(t *testing.T) {
	t.Helper()

	title := statusTitle("●", mountBackendNFS, "newfiles", "/Users/rowantrollope/abc")
	want := "Workspace: newfiles mounted at /Users/rowantrollope/abc (via NFS)"
	if !strings.Contains(title, want) {
		t.Fatalf("statusTitle() = %q, want substring %q", title, want)
	}
	if strings.Contains(title, "afs nfs mount") {
		t.Fatalf("statusTitle() = %q, should not use legacy mount wording", title)
	}
}

func TestPrintReadyBoxUsesMountedWorkspaceTitle(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.RedisAddr = "localhost:6379"
	cfg.RedisDB = 0
	cfg.CurrentWorkspace = "newfiles"
	cfg.Mountpoint = "/Users/rowantrollope/abc"

	out, err := captureStdout(t, func() error {
		printReadyBox(cfg, mountBackendNFS, "")
		return nil
	})
	if err != nil {
		t.Fatalf("printReadyBox() returned error: %v", err)
	}

	want := "Workspace: newfiles mounted at /Users/rowantrollope/abc (via NFS)"
	if !strings.Contains(out, want) {
		t.Fatalf("printReadyBox() output = %q, want substring %q", out, want)
	}
	if strings.Contains(out, "Successfully mounted workspace") {
		t.Fatalf("printReadyBox() output = %q, should not use old success wording", out)
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
	cfg.Mountpoint = "/Users/example/Library/Application Support/Agent Filesystem/projects/customer-success/super-long-nested-workspace-path"

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
	if !strings.Contains(out, "afs no mounted filesystem") {
		t.Fatalf("cmdUp() output = %q, want status output", out)
	}
	if strings.Contains(out, "Run 'afs down' first") {
		t.Fatalf("cmdUp() output still contains old already-running error: %q", out)
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
		Mountpoint:           mountpoint,
		CreatedMountpoint:    true,
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
