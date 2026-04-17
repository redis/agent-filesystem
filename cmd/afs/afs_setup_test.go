package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestNormalizeMountBackendSupportsNone(t *testing.T) {
	t.Helper()

	got, err := normalizeMountBackend("none")
	if err != nil {
		t.Fatalf("normalizeMountBackend() returned error: %v", err)
	}
	if got != mountBackendNone {
		t.Fatalf("normalizeMountBackend() = %q, want %q", got, mountBackendNone)
	}
}

func TestResolveConfigPathsFilesystemOnlySkipsMountResolution(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.MountBackend = mountBackendNone
	cfg.LocalPath = "~/mypath"
	cfg.WorkRoot = t.TempDir()

	if err := resolveConfigPaths(&cfg); err != nil {
		t.Fatalf("resolveConfigPaths() returned error: %v", err)
	}
	if cfg.MountBackend != mountBackendNone {
		t.Fatalf("MountBackend = %q, want %q", cfg.MountBackend, mountBackendNone)
	}
	// LocalPath is kept even when backend=none (used for sync mode).
	if cfg.LocalPath == "" {
		t.Fatalf("LocalPath should be preserved when mountBackend=none; got empty")
	}
}

func TestRunSetupWizardAllowsNoMountedFilesystem(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	// First-run flow: Redis → workspace creation → mode → mount surface.
	// Switch to live mount mode, then type "none" so the test exercises the
	// explicit "no mounted filesystem" path.
	input := stringsJoinLines(
		"",        // connection: local
		mr.Addr(), // redis addr
		"",        // redis username default
		"",        // redis password
		"",        // tls default
		"demo",
		"2", // mode: live mount
		"none",
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))

	cfg, err := runSetupWizard(reader, ioDiscard{}, defaultConfig(), true)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if cfg.MountBackend != mountBackendNone {
		t.Fatalf("MountBackend = %q, want %q", cfg.MountBackend, mountBackendNone)
	}
	if cfg.LocalPath != "" {
		t.Fatalf("Mountpoint = %q, want empty", cfg.LocalPath)
	}
	if cfg.CurrentWorkspace != "demo" {
		t.Fatalf("CurrentWorkspace = %q, want %q", cfg.CurrentWorkspace, "demo")
	}
}

func TestRunSetupWizardNoMountSkipsLegacyFilesystemKeyPrompt(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	input := stringsJoinLines(
		"",        // connection: local
		mr.Addr(), // redis addr
		"",        // redis username default
		"",        // redis password
		"",        // tls default
		"demo",
		"2", // mode: live mount
		"none",
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))
	var output bytes.Buffer

	if _, err := runSetupWizard(reader, &output, defaultConfig(), true); err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}

	got := output.String()
	if strings.Contains(got, "filesystem key") || strings.Contains(got, "What Redis key should the mounted filesystem use?") {
		t.Fatalf("no-mount setup output unexpectedly prompted for a filesystem key:\n%s", got)
	}
	if !strings.Contains(got, "Filesystem Mount") || !strings.Contains(got, "Choose local mount point") {
		t.Fatalf("no-mount setup output = %q, want local mount point prompt", got)
	}
	if !strings.Contains(got, "Create your first workspace") {
		t.Fatalf("no-mount setup output = %q, want first-workspace prompt", got)
	}
}

func TestRunSetupWizardSelectsExistingWorkspaceAndDefaultsLocalPath(t *testing.T) {
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
	cfg.RedisAddr = mr.Addr()
	store := newAFSStore(mustRedisClient(t, cfg))
	defer func() { _ = store.rdb.Close() }()

	if err := createEmptyWorkspace(context.Background(), cfg, store, "alpha"); err != nil {
		t.Fatalf("createEmptyWorkspace(alpha) returned error: %v", err)
	}
	if err := createEmptyWorkspace(context.Background(), cfg, store, "beta"); err != nil {
		t.Fatalf("createEmptyWorkspace(beta) returned error: %v", err)
	}

	input := stringsJoinLines(
		"", // connection: local
		mr.Addr(),
		"",
		"",
		"",
		"beta",
		"",
		"",
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))
	var output bytes.Buffer

	got, err := runSetupWizard(reader, &output, defaultConfig(), true)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if got.CurrentWorkspace != "beta" {
		t.Fatalf("CurrentWorkspace = %q, want %q", got.CurrentWorkspace, "beta")
	}
	wantLocalPath := filepath.Join(homeDir, "beta")
	if got.LocalPath != wantLocalPath {
		t.Fatalf("LocalPath = %q, want %q", got.LocalPath, wantLocalPath)
	}
	if got.Mode != modeSync {
		t.Fatalf("Mode = %q, want %q", got.Mode, modeSync)
	}
	rendered := output.String()
	for _, want := range []string{
		"Choose a workspace",
		"Workspace name",
		"Files/Folders",
		"Last updated",
		"0 files/0 folders",
		"0 B",
		"Create a new workspace",
		"Connected",
		"alpha",
		"beta",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("setup output = %q, want substring %q", rendered, want)
		}
	}
}

func TestRunSetupWizardCanCreateNewWorkspaceFromExistingList(t *testing.T) {
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
	cfg.RedisAddr = mr.Addr()
	store := newAFSStore(mustRedisClient(t, cfg))
	defer func() { _ = store.rdb.Close() }()

	if err := createEmptyWorkspace(context.Background(), cfg, store, "alpha"); err != nil {
		t.Fatalf("createEmptyWorkspace(alpha) returned error: %v", err)
	}

	input := stringsJoinLines(
		"", // connection: local
		mr.Addr(),
		"",
		"",
		"",
		"2",
		"gamma",
		"",
		"",
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))

	got, err := runSetupWizard(reader, ioDiscard{}, defaultConfig(), true)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if got.CurrentWorkspace != "gamma" {
		t.Fatalf("CurrentWorkspace = %q, want %q", got.CurrentWorkspace, "gamma")
	}
	wantLocalPath := filepath.Join(homeDir, "gamma")
	if got.LocalPath != wantLocalPath {
		t.Fatalf("LocalPath = %q, want %q", got.LocalPath, wantLocalPath)
	}

	exists, err := store.workspaceExists(context.Background(), "gamma")
	if err != nil {
		t.Fatalf("workspaceExists(gamma) returned error: %v", err)
	}
	if !exists {
		t.Fatal("expected setup to create the selected new workspace")
	}
}

func TestRunSetupWizardCreatesFirstWorkspaceAndDefaultsLocalPath(t *testing.T) {
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

	input := stringsJoinLines(
		"", // connection: local
		mr.Addr(),
		"",
		"",
		"",
		"demo",
		"",
		"",
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))

	got, err := runSetupWizard(reader, ioDiscard{}, defaultConfig(), true)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if got.CurrentWorkspace != "demo" {
		t.Fatalf("CurrentWorkspace = %q, want %q", got.CurrentWorkspace, "demo")
	}
	wantLocalPath := filepath.Join(homeDir, "demo")
	if got.LocalPath != wantLocalPath {
		t.Fatalf("LocalPath = %q, want %q", got.LocalPath, wantLocalPath)
	}
	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	store := newAFSStore(mustRedisClient(t, cfg))
	defer func() { _ = store.rdb.Close() }()

	exists, err := store.workspaceExists(context.Background(), "demo")
	if err != nil {
		t.Fatalf("workspaceExists(demo) returned error: %v", err)
	}
	if !exists {
		t.Fatal("expected first workspace to be created during setup")
	}
}

func TestRunSetupWizardSupportsSelfHostedConnection(t *testing.T) {
	t.Helper()

	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv(HOME) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})

	server := newSelfHostedControlPlaneServer(t)

	input := stringsJoinLines(
		"2",        // connection: self-hosted
		server.URL, // control plane URL
		"repo",     // existing workspace
		"",         // keep default mode: sync
		"",         // keep default sync local path
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))

	got, err := runSetupWizard(reader, ioDiscard{}, defaultConfig(), true)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if got.ProductMode != productModeSelfHosted {
		t.Fatalf("ProductMode = %q, want %q", got.ProductMode, productModeSelfHosted)
	}
	if got.URL != server.URL {
		t.Fatalf("controlPlane.url = %q, want %q", got.URL, server.URL)
	}
	if got.CurrentWorkspace != "repo" {
		t.Fatalf("CurrentWorkspace = %q, want %q", got.CurrentWorkspace, "repo")
	}
	wantLocalPath := filepath.Join(homeDir, "repo")
	if got.LocalPath != wantLocalPath {
		t.Fatalf("LocalPath = %q, want %q", got.LocalPath, wantLocalPath)
	}
	if got.Mode != modeSync {
		t.Fatalf("Mode = %q, want %q", got.Mode, modeSync)
	}
}

func TestRunSetupWizardSupportsSelfHostedLiveMountMode(t *testing.T) {
	t.Helper()

	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv(HOME) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})

	server := newSelfHostedControlPlaneServer(t)

	input := stringsJoinLines(
		"2",        // connection: self-hosted
		server.URL, // control plane URL
		"repo",     // existing workspace
		"2",        // mode: live mount
		"",         // keep default mountpoint
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))

	got, err := runSetupWizard(reader, ioDiscard{}, defaultConfig(), true)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if got.ProductMode != productModeSelfHosted {
		t.Fatalf("ProductMode = %q, want %q", got.ProductMode, productModeSelfHosted)
	}
	if got.URL != server.URL {
		t.Fatalf("controlPlane.url = %q, want %q", got.URL, server.URL)
	}
	if got.CurrentWorkspace != "repo" {
		t.Fatalf("CurrentWorkspace = %q, want %q", got.CurrentWorkspace, "repo")
	}
	wantLocalPath := filepath.Join(homeDir, "repo")
	if got.LocalPath != wantLocalPath {
		t.Fatalf("LocalPath = %q, want %q", got.LocalPath, wantLocalPath)
	}
	if got.Mode != modeMount {
		t.Fatalf("Mode = %q, want %q", got.Mode, modeMount)
	}
	if got.MountBackend != defaultMountBackend() {
		t.Fatalf("MountBackend = %q, want %q", got.MountBackend, defaultMountBackend())
	}
}

func TestRunSetupWizardSupportsCloudManagedBootstrapConfig(t *testing.T) {
	t.Helper()

	input := stringsJoinLines(
		"3", // connection: cloud managed
		"https://afs.example.com",
		"", // mode: sync default
		"", // local path default
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))

	got, err := runSetupWizard(reader, ioDiscard{}, defaultConfig(), true)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if got.ProductMode != productModeCloud {
		t.Fatalf("ProductMode = %q, want %q", got.ProductMode, productModeCloud)
	}
	if got.URL != "https://afs.example.com" {
		t.Fatalf("URL = %q, want %q", got.URL, "https://afs.example.com")
	}
	if got.CurrentWorkspace != "" || got.CurrentWorkspaceID != "" {
		t.Fatalf("cloud setup should not preselect a workspace: %#v", got)
	}
}

func TestRunSetupWizardExistingConfigShowsCurrentSettings(t *testing.T) {
	t.Helper()

	existing := defaultConfig()
	existing.RedisAddr = "redis.example:6379"
	existing.CurrentWorkspace = "demo"
	existing.Mode = modeMount // keep the menu label on "live mount" for the assertion below
	existing.MountBackend = mountBackendNone
	existing.LocalPath = ""

	// Menu (edit mode, local): 1=mode, 2=workspace, 3=local path/filesystem
	// mount, 4=redis connection, 5=configuration source, 6=save.
	// Pick option 3 (filesystem mount) → keep as none → back to menu → pick 6 to save.
	reader := bufio.NewReader(bytes.NewBufferString(stringsJoinLines(
		"3",
		"",
		"6",
	)))
	var output bytes.Buffer

	if _, err := runSetupWizard(reader, &output, existing, false); err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}

	got := output.String()
	if !strings.Contains(got, "Change mode") || !strings.Contains(got, "live mount") {
		t.Fatalf("setup output = %q, want mode current setting", got)
	}
	if !strings.Contains(got, "Change configuration source") || !strings.Contains(got, "local") {
		t.Fatalf("setup output = %q, want configuration source current setting", got)
	}
	if !strings.Contains(got, "Change Redis connection") || !strings.Contains(got, "redis.example:6379") {
		t.Fatalf("setup output = %q, want redis connection current setting", got)
	}
	if !strings.Contains(got, "Change Filesystem Mount") || !strings.Contains(got, "none") {
		t.Fatalf("setup output = %q, want filesystem mount current setting", got)
	}
	if !strings.Contains(got, "Change current workspace") || !strings.Contains(got, "demo") {
		t.Fatalf("setup output = %q, want current workspace current setting", got)
	}
	if strings.Index(got, "Change current workspace") > strings.Index(got, "Change configuration source") {
		t.Fatalf("setup output = %q, want configuration source after current workspace", got)
	}
}

func TestRunSetupWizardAllowsChangingCurrentWorkspace(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	existing := defaultConfig()
	existing.RedisAddr = mr.Addr()
	existing.CurrentWorkspace = "demo"
	store := newAFSStore(mustRedisClient(t, existing))
	defer func() { _ = store.rdb.Close() }()

	if err := createEmptyWorkspace(context.Background(), existing, store, "demo"); err != nil {
		t.Fatalf("createEmptyWorkspace(demo) returned error: %v", err)
	}
	if err := createEmptyWorkspace(context.Background(), existing, store, "repo-two"); err != nil {
		t.Fatalf("createEmptyWorkspace(repo-two) returned error: %v", err)
	}

	// Edit-menu items (local): 2 = current workspace, 6 = save.
	reader := bufio.NewReader(bytes.NewBufferString(stringsJoinLines(
		"2",
		"repo-two",
		"6",
	)))

	cfg, err := runSetupWizard(reader, ioDiscard{}, existing, false)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if cfg.CurrentWorkspace != "repo-two" {
		t.Fatalf("CurrentWorkspace = %q, want %q", cfg.CurrentWorkspace, "repo-two")
	}
}

func TestRunSetupWizardSwitchingToLocalKeepsExistingRedisConfig(t *testing.T) {
	t.Helper()

	server := newSelfHostedControlPlaneServer(t)

	existing := defaultConfig()
	existing.ProductMode = productModeSelfHosted
	existing.URL = server.URL
	existing.DatabaseID = "db-local"
	existing.RedisAddr = "redis.example:6379"
	existing.RedisUsername = "alice"
	existing.RedisPassword = "secret"
	existing.RedisDB = 4
	existing.RedisTLS = true
	existing.CurrentWorkspace = "repo"

	// Menu (edit mode, managed): 4 = configuration source, 5 = save.
	// Choose local, then save from the re-rendered local menu.
	reader := bufio.NewReader(bytes.NewBufferString(stringsJoinLines(
		"4",
		"1",
		"6",
	)))
	var output bytes.Buffer

	cfg, err := runSetupWizard(reader, &output, existing, false)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if cfg.ProductMode != productModeLocal {
		t.Fatalf("ProductMode = %q, want %q", cfg.ProductMode, productModeLocal)
	}
	if cfg.RedisAddr != "redis.example:6379" {
		t.Fatalf("RedisAddr = %q, want %q", cfg.RedisAddr, "redis.example:6379")
	}
	if cfg.RedisUsername != "alice" {
		t.Fatalf("RedisUsername = %q, want %q", cfg.RedisUsername, "alice")
	}
	if cfg.RedisPassword != "secret" {
		t.Fatalf("RedisPassword = %q, want %q", cfg.RedisPassword, "secret")
	}
	if cfg.RedisDB != 4 {
		t.Fatalf("RedisDB = %d, want %d", cfg.RedisDB, 4)
	}
	if !cfg.RedisTLS {
		t.Fatal("RedisTLS = false, want true")
	}

	got := output.String()
	if strings.Contains(got, "▸ Redis Connection") {
		t.Fatalf("setup output = %q, want no redis connection prompt when switching back to local", got)
	}
	if !strings.Contains(got, "Change Redis connection") || !strings.Contains(got, "redis.example:6379") {
		t.Fatalf("setup output = %q, want redis connection available from the local main menu", got)
	}
}

func TestRunSetupWizardAllowsCreatingWorkspaceFromCurrentWorkspaceMenu(t *testing.T) {
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
	existing := defaultConfig()
	existing.RedisAddr = mr.Addr()
	existing.CurrentWorkspace = "demo"
	store := newAFSStore(mustRedisClient(t, existing))
	defer func() { _ = store.rdb.Close() }()

	if err := createEmptyWorkspace(context.Background(), existing, store, "demo"); err != nil {
		t.Fatalf("createEmptyWorkspace(demo) returned error: %v", err)
	}

	// Edit-menu items (local): 2 = current workspace, 6 = save.
	reader := bufio.NewReader(bytes.NewBufferString(stringsJoinLines(
		"2",
		"2",
		"repo-three",
		"6",
	)))

	cfg, err := runSetupWizard(reader, ioDiscard{}, existing, false)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if cfg.CurrentWorkspace != "repo-three" {
		t.Fatalf("CurrentWorkspace = %q, want %q", cfg.CurrentWorkspace, "repo-three")
	}
	wantLocalPath := filepath.Join("~", "repo-three")
	if cfg.LocalPath != wantLocalPath {
		t.Fatalf("LocalPath = %q, want %q", cfg.LocalPath, wantLocalPath)
	}
	exists, err := store.workspaceExists(context.Background(), "repo-three")
	if err != nil {
		t.Fatalf("workspaceExists(repo-three) returned error: %v", err)
	}
	if !exists {
		t.Fatal("expected edit-mode workspace menu to create repo-three")
	}
}

func TestRunSetupWizardAutoSelectsMountBackendAndUsesCurrentWorkspace(t *testing.T) {
	t.Helper()

	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen(127.0.0.1:0) returned error: %v", err)
	}
	defer busy.Close()
	busyAddr, ok := busy.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("busy listener addr type = %T, want *net.TCPAddr", busy.Addr())
	}

	existing := defaultConfig()
	existing.CurrentWorkspace = "repo"
	existing.Mode = modeMount // stay on live mount in the edit menu
	existing.MountBackend = mountBackendNone
	existing.LocalPath = ""
	existing.NFSPort = busyAddr.Port

	// Edit-menu items (local): 3 = filesystem mount, 6 = save and exit.
	input := stringsJoinLines(
		"3", // filesystem mount
		"/tmp/afs-mount",
		"6", // save and exit
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))

	cfg, err := runSetupWizard(reader, ioDiscard{}, existing, false)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if cfg.MountBackend != defaultMountBackend() {
		t.Fatalf("MountBackend = %q, want %q", cfg.MountBackend, defaultMountBackend())
	}
	if cfg.LocalPath != "/tmp/afs-mount" {
		t.Fatalf("Mountpoint = %q, want %q", cfg.LocalPath, "/tmp/afs-mount")
	}
	if runtime.GOOS == "darwin" {
		if cfg.NFSHost != "127.0.0.1" {
			t.Fatalf("NFSHost = %q, want %q", cfg.NFSHost, "127.0.0.1")
		}
		if cfg.NFSPort == busyAddr.Port {
			t.Fatalf("NFSPort = %d, want a free port instead of the busy port", cfg.NFSPort)
		}
		if !tcpAddressAvailable(cfg.NFSHost, cfg.NFSPort) {
			t.Fatalf("suggested NFS port %s:%s should be available", cfg.NFSHost, strconv.Itoa(cfg.NFSPort))
		}
	}
}

func TestRunSetupWizardEditModeLoopsUntilSaveAndExit(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	existing := defaultConfig()
	existing.RedisAddr = mr.Addr()
	existing.CurrentWorkspace = "demo"
	existing.MountBackend = mountBackendNone
	store := newAFSStore(mustRedisClient(t, existing))
	defer func() { _ = store.rdb.Close() }()

	if err := createEmptyWorkspace(context.Background(), existing, store, "demo"); err != nil {
		t.Fatalf("createEmptyWorkspace(demo) returned error: %v", err)
	}
	if err := createEmptyWorkspace(context.Background(), existing, store, "repo-two"); err != nil {
		t.Fatalf("createEmptyWorkspace(repo-two) returned error: %v", err)
	}

	// Edit-menu items (local): 2 = current workspace, 6 = save. Exercise the loop
	// by picking workspace twice, then saving.
	input := stringsJoinLines(
		"2", "repo-two",
		"2", "demo",
		"6",
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))
	var output bytes.Buffer

	cfg, err := runSetupWizard(reader, &output, existing, false)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if cfg.CurrentWorkspace != "demo" {
		t.Fatalf("CurrentWorkspace = %q, want %q after second edit", cfg.CurrentWorkspace, "demo")
	}
	if strings.Count(output.String(), "What would you like to change?") < 3 {
		t.Fatalf("menu should have been shown 3 times (two edits + final exit), got:\n%s", output.String())
	}
}

func TestRunSetupWizardEditModeUnknownChoiceReprompts(t *testing.T) {
	t.Helper()

	existing := defaultConfig()
	existing.CurrentWorkspace = "demo"

	// Unknown choice "9" → menu re-displayed → pick 6 to save and exit.
	input := stringsJoinLines("9", "6")
	reader := bufio.NewReader(bytes.NewBufferString(input))
	var output bytes.Buffer

	if _, err := runSetupWizard(reader, &output, existing, false); err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if !strings.Contains(output.String(), "Unknown choice") {
		t.Fatalf("expected unknown-choice warning, got:\n%s", output.String())
	}
	if strings.Count(output.String(), "What would you like to change?") < 2 {
		t.Fatalf("menu should have been shown at least twice after unknown choice")
	}
}

func TestCmdSetupDoesNotStartServices(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv(HOME) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})

	configFile := filepath.Join(t.TempDir(), "afs.config.json")
	origConfigPath := cfgPathOverride
	cfgPathOverride = configFile
	t.Cleanup(func() {
		cfgPathOverride = origConfigPath
	})

	stdinPath := filepath.Join(t.TempDir(), "setup-input.txt")
	input := stringsJoinLines(
		"", // connection: local
		mr.Addr(),
		"", // default user
		"", // no password
		"", // no tls
		"demo",
		"", // mode: sync
		"", // default local path
	)
	if err := os.WriteFile(stdinPath, []byte(input), 0o644); err != nil {
		t.Fatalf("WriteFile(stdin) returned error: %v", err)
	}
	stdinFile, err := os.Open(stdinPath)
	if err != nil {
		t.Fatalf("Open(stdin) returned error: %v", err)
	}
	defer stdinFile.Close()

	stdoutFile, err := os.CreateTemp(t.TempDir(), "setup-stdout-*.txt")
	if err != nil {
		t.Fatalf("CreateTemp(stdout) returned error: %v", err)
	}
	defer stdoutFile.Close()

	origStdin := os.Stdin
	origStdout := os.Stdout
	os.Stdin = stdinFile
	os.Stdout = stdoutFile
	t.Cleanup(func() {
		os.Stdin = origStdin
		os.Stdout = origStdout
	})

	if err := cmdSetup(); err != nil {
		t.Fatalf("cmdSetup() returned error: %v", err)
	}

	if _, statErr := os.Stat(configFile); statErr != nil {
		t.Fatalf("config file stat error = %v, want saved config", statErr)
	}

	// cmdSetup must not have produced any running-state file.
	if _, statErr := os.Stat(statePath()); statErr == nil {
		t.Fatal("cmdSetup() should not have written state (it should not start services)")
	} else if !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("state stat error = %v, want ErrNotExist", statErr)
	}

	if _, err := stdoutFile.Seek(0, 0); err != nil {
		t.Fatalf("Seek(stdout) returned error: %v", err)
	}
	outputBytes, err := os.ReadFile(stdoutFile.Name())
	if err != nil {
		t.Fatalf("ReadFile(stdout) returned error: %v", err)
	}
	output := string(outputBytes)
	if !strings.Contains(output, "Run ") || !strings.Contains(output, " up") {
		t.Fatalf("cmdSetup() output should mention running 'afs up' afterward; got:\n%s", output)
	}
}

func TestRunSetupWizardMountWithoutCurrentWorkspacePromptsForWorkspace(t *testing.T) {
	t.Helper()

	existing := defaultConfig()
	existing.CurrentWorkspace = ""
	existing.Mode = modeMount // edit-mode test, staying on live mount
	existing.MountBackend = mountBackendNone

	// Edit-menu items (local): 3 = filesystem mount, 6 = save.
	input := stringsJoinLines(
		"3",        // filesystem mount
		"/tmp/afs", // requested mountpoint
		"newfiles", // workspace name to create/use
		"6",        // save and exit
	)
	reader := bufio.NewReader(bytes.NewBufferString(input))

	cfg, err := runSetupWizard(reader, ioDiscard{}, existing, false)
	if err != nil {
		t.Fatalf("runSetupWizard() returned error: %v", err)
	}
	if cfg.CurrentWorkspace != "newfiles" {
		t.Fatalf("CurrentWorkspace = %q, want %q", cfg.CurrentWorkspace, "newfiles")
	}
	if cfg.LocalPath != "/tmp/afs" {
		t.Fatalf("Mountpoint = %q, want %q", cfg.LocalPath, "/tmp/afs")
	}
	if cfg.MountBackend != defaultMountBackend() {
		t.Fatalf("MountBackend = %q, want %q", cfg.MountBackend, defaultMountBackend())
	}
}

func TestPromptLocalFilesystemSetupRejectsExistingFileMountpoint(t *testing.T) {
	t.Helper()

	mountpointFile := filepath.Join(t.TempDir(), "afs")
	if err := os.WriteFile(mountpointFile, []byte("binary"), 0o755); err != nil {
		t.Fatalf("WriteFile(mountpoint) returned error: %v", err)
	}

	cfg := defaultConfig()
	cfg.CurrentWorkspace = "demo"
	cfg.MountBackend = mountBackendNone
	cfg.LocalPath = ""

	reader := bufio.NewReader(bytes.NewBufferString(stringsJoinLines(mountpointFile)))
	err := promptLocalFilesystemSetup(reader, ioDiscard{}, &cfg, false)
	if err == nil {
		t.Fatal("promptLocalFilesystemSetup() returned nil error, want existing-file mountpoint rejection")
	}
	if !strings.Contains(err.Error(), "exists and is not a directory") {
		t.Fatalf("promptLocalFilesystemSetup() error = %q, want non-directory message", err)
	}
	if cfg.MountBackend != mountBackendNone {
		t.Fatalf("MountBackend = %q, want %q after rejected mountpoint", cfg.MountBackend, mountBackendNone)
	}
	if cfg.LocalPath != "" {
		t.Fatalf("Mountpoint = %q, want empty after rejected mountpoint", cfg.LocalPath)
	}
}

func TestCmdSetupRejectsExistingFileMountpointBeforeSavingConfig(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv(HOME) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})

	configFile := filepath.Join(t.TempDir(), "afs.config.json")
	origConfigPath := cfgPathOverride
	cfgPathOverride = configFile
	t.Cleanup(func() {
		cfgPathOverride = origConfigPath
	})

	mountpointFile := filepath.Join(t.TempDir(), "afs")
	if err := os.WriteFile(mountpointFile, []byte("binary"), 0o755); err != nil {
		t.Fatalf("WriteFile(mountpoint) returned error: %v", err)
	}

	stdinPath := filepath.Join(t.TempDir(), "setup-input.txt")
	input := stringsJoinLines(
		"", // connection: local
		mr.Addr(),
		"", // redis username default
		"", // redis password
		"", // tls default
		"demo",
		"2",            // mode: live mount (so we reach the mountpoint prompt)
		mountpointFile, // invalid mountpoint
	)
	if err := os.WriteFile(stdinPath, []byte(input), 0o644); err != nil {
		t.Fatalf("WriteFile(stdin) returned error: %v", err)
	}

	stdinFile, err := os.Open(stdinPath)
	if err != nil {
		t.Fatalf("Open(stdin) returned error: %v", err)
	}
	defer stdinFile.Close()

	stdoutFile, err := os.CreateTemp(t.TempDir(), "setup-stdout-*.txt")
	if err != nil {
		t.Fatalf("CreateTemp(stdout) returned error: %v", err)
	}
	defer stdoutFile.Close()

	origStdin := os.Stdin
	origStdout := os.Stdout
	os.Stdin = stdinFile
	os.Stdout = stdoutFile
	t.Cleanup(func() {
		os.Stdin = origStdin
		os.Stdout = origStdout
	})

	err = cmdSetup()
	if err == nil {
		t.Fatal("cmdSetup() returned nil error, want existing-file mountpoint rejection")
	}
	if !strings.Contains(err.Error(), "exists and is not a directory") {
		t.Fatalf("cmdSetup() error = %q, want non-directory message", err)
	}
	if _, statErr := os.Stat(configFile); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("config file stat error = %v, want not-exist after rejected setup", statErr)
	}
}

func TestStartServicesFilesystemOnlyUsesRedisWithoutMountpoint(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)
	homeDir := t.TempDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv(HOME) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNone
	cfg.WorkRoot = filepath.Join(homeDir, ".afs", "workspaces")

	if err := resolveConfigPaths(&cfg); err != nil {
		t.Fatalf("resolveConfigPaths() returned error: %v", err)
	}
	if err := startServices(cfg); err != nil {
		t.Fatalf("startServices() returned error: %v", err)
	}

	st, err := loadState()
	if err != nil {
		t.Fatalf("loadState() returned error: %v", err)
	}
	if st.MountBackend != mountBackendNone {
		t.Fatalf("MountBackend = %q, want %q", st.MountBackend, mountBackendNone)
	}
	if st.LocalPath != "" {
		t.Fatalf("Mountpoint = %q, want empty", st.LocalPath)
	}
	if st.RedisAddr != mr.Addr() {
		t.Fatalf("RedisAddr = %q, want %q", st.RedisAddr, mr.Addr())
	}
}

func TestStartServicesRejectsMissingConfiguredWorkspaceForMountedFilesystem(t *testing.T) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNFS
	cfg.NFSBin = "/usr/bin/true"
	cfg.CurrentWorkspace = "missing-workspace"
	cfg.LocalPath = filepath.Join(t.TempDir(), "mnt")

	if err := resolveConfigPaths(&cfg); err != nil {
		t.Fatalf("resolveConfigPaths() returned error: %v", err)
	}

	err := startServices(cfg)
	if err == nil {
		t.Fatal("startServices() returned nil error, want missing workspace error")
	}
	if !strings.Contains(err.Error(), `workspace "missing-workspace" does not exist`) {
		t.Fatalf("startServices() error = %q, want missing workspace message", err)
	}

	store := newAFSStore(mustRedisClient(t, cfg))
	defer func() { _ = store.rdb.Close() }()

	exists, existsErr := store.workspaceExists(context.Background(), "missing-workspace")
	if existsErr != nil {
		t.Fatalf("workspaceExists(missing-workspace) returned error: %v", existsErr)
	}
	if exists {
		t.Fatal("expected startServices() not to auto-create the missing workspace")
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}

func stringsJoinLines(lines ...string) string {
	var b bytes.Buffer
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
