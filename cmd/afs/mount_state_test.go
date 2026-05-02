package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMountRegistryRoundTripAndPathLookup(t *testing.T) {
	t.Helper()

	withTempHome(t)
	root := t.TempDir()
	rec := mountRecord{
		ID:        "att_test",
		Workspace: "notes",
		LocalPath: filepath.Join(root, "notes"),
		Mode:      modeSync,
		PID:       123,
		StartedAt: time.Now().UTC(),
	}
	reg := mountRegistry{}
	upsertMount(&reg, rec)
	if err := saveMountRegistry(reg); err != nil {
		t.Fatalf("saveMountRegistry() returned error: %v", err)
	}

	loaded, err := loadMountRegistry()
	if err != nil {
		t.Fatalf("loadMountRegistry() returned error: %v", err)
	}
	got, ok := mountByPath(loaded, rec.LocalPath)
	if !ok {
		t.Fatalf("mountByPath(%s) returned false", rec.LocalPath)
	}
	if got.Workspace != "notes" {
		t.Fatalf("Workspace = %q, want notes", got.Workspace)
	}
}

func TestMountPathConflictDetectsNestedPaths(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	child := filepath.Join(parent, "child")
	reg := mountRegistry{Mounts: []mountRecord{{
		Workspace: "notes",
		LocalPath: parent,
	}}}
	if _, ok := mountPathConflict(reg, child); !ok {
		t.Fatalf("mountPathConflict() returned false for nested path")
	}
}

func TestCmdStatusPrintsAlignedMountTable(t *testing.T) {
	t.Helper()

	withTempHome(t)
	reg := mountRegistry{Mounts: []mountRecord{
		{
			ID:        "att_beta",
			Workspace: "beta-workspace",
			LocalPath: "/tmp/beta-workspace",
			Mode:      modeSync,
			PID:       os.Getpid(),
			StartedAt: time.Now().UTC(),
		},
		{
			ID:        "att_alpha",
			Workspace: "alpha",
			LocalPath: "/tmp/alpha",
			Mode:      modeSync,
			PID:       os.Getpid(),
			StartedAt: time.Now().UTC(),
		},
	}}
	if err := saveMountRegistry(reg); err != nil {
		t.Fatalf("saveMountRegistry() returned error: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return cmdStatusWithOptions(statusOptions{})
	})
	if err != nil {
		t.Fatalf("cmdStatusWithOptions() returned error: %v", err)
	}
	if strings.Contains(out, "\t") {
		t.Fatalf("status output contains tabs, want fixed-width columns:\n%s", out)
	}
	lines := nonEmptyLines(out)
	statusLine := indexLine(lines, "AFS Running")
	mountLine := indexLine(lines, "Mounted workspaces")
	if statusLine < 0 {
		t.Fatalf("status output missing daemon status section:\n%s", out)
	}
	if mountLine < 0 {
		t.Fatalf("status output missing mount section:\n%s", out)
	}
	if statusLine > mountLine {
		t.Fatalf("daemon status should print before mount table:\n%s", out)
	}
	if strings.Contains(stripAnsi(out), "AFS Not Running") {
		t.Fatalf("status output should not say daemon is absent when mount daemons are running:\n%s", out)
	}
	if mountLine+3 >= len(lines) {
		t.Fatalf("mount table incomplete:\n%s", out)
	}
	headerPathCol := strings.Index(lines[mountLine+1], "Path")
	firstPathCol := strings.Index(lines[mountLine+2], "/tmp/alpha")
	secondPathCol := strings.Index(lines[mountLine+3], "/tmp/beta-workspace")
	if headerPathCol < 0 || firstPathCol != headerPathCol || secondPathCol != headerPathCol {
		t.Fatalf("path column not aligned:\n%s", out)
	}
}

func TestCmdStatusDoesNotListStoppedRecordsAsMounted(t *testing.T) {
	t.Helper()

	withTempHome(t)
	reg := mountRegistry{Mounts: []mountRecord{{
		ID:        "att_alpha",
		Workspace: "alpha",
		LocalPath: "/tmp/alpha",
		Mode:      modeSync,
		PID:       0,
		StartedAt: time.Now().UTC(),
	}}}
	if err := saveMountRegistry(reg); err != nil {
		t.Fatalf("saveMountRegistry() returned error: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return cmdStatusWithOptions(statusOptions{})
	})
	if err != nil {
		t.Fatalf("cmdStatusWithOptions() returned error: %v", err)
	}
	clean := stripAnsi(out)
	for _, want := range []string{
		"AFS Not Running",
		"No mounted workspaces.",
		"Stopped workspace records",
		"alpha",
	} {
		if !strings.Contains(clean, want) {
			t.Fatalf("status output missing %q:\n%s", want, clean)
		}
	}
	if strings.Contains(clean, "\nMounted workspaces\n") {
		t.Fatalf("stopped records should not be listed as mounted:\n%s", clean)
	}
}

func TestCmdStatusShowsUnmanagedSyncDaemon(t *testing.T) {
	t.Helper()

	withTempHome(t)
	orig := statusSyncDaemonPIDs
	statusSyncDaemonPIDs = func() ([]int, error) {
		return []int{4242}, nil
	}
	t.Cleanup(func() {
		statusSyncDaemonPIDs = orig
	})

	out, err := captureStdout(t, func() error {
		return cmdStatusWithOptions(statusOptions{})
	})
	if err != nil {
		t.Fatalf("cmdStatusWithOptions() returned error: %v", err)
	}
	clean := stripAnsi(out)
	for _, want := range []string{
		"AFS Running (PID: 4242)",
		"mode               sync",
		"unmanaged daemons",
	} {
		if !strings.Contains(clean, want) {
			t.Fatalf("status output missing %q:\n%s", want, clean)
		}
	}
}

func TestParseSyncDaemonPIDs(t *testing.T) {
	t.Helper()

	out := strings.Join([]string{
		"  101 /usr/local/bin/afs _sync-daemon",
		"  102 /Users/example/git/agent-filesystem/afs --config /tmp/afs.config.json _sync-daemon",
		"  103 /usr/local/bin/afs status",
		"  104 /bin/zsh -c rg afs _sync-daemon",
		"  101 /usr/local/bin/afs _sync-daemon",
	}, "\n")
	got := parseSyncDaemonPIDs(out)
	want := []int{101, 102}
	if len(got) != len(want) {
		t.Fatalf("parseSyncDaemonPIDs() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseSyncDaemonPIDs() = %#v, want %#v", got, want)
		}
	}
}

func TestParseSyncDaemonPIDsForConfigRequiresMatchingImplicitConfig(t *testing.T) {
	t.Helper()

	out := strings.Join([]string{
		"  101 /usr/local/bin/afs _sync-daemon",
		"  102 /Users/example/git/agent-filesystem/afs --config /tmp/current/afs.config.json _sync-daemon",
		"  103 /opt/afs/afs _sync-daemon",
	}, "\n")

	got := parseSyncDaemonPIDsForConfig(out, "/tmp/current/afs.config.json")
	want := []int{102}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseSyncDaemonPIDsForConfig() = %#v, want %#v", got, want)
	}

	got = parseSyncDaemonPIDsForConfig(out, "/opt/afs/afs.config.json")
	want = []int{103}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseSyncDaemonPIDsForConfig() = %#v, want %#v", got, want)
	}
}

func TestCmdStatusVerboseIncludesConnectionDetails(t *testing.T) {
	t.Helper()

	withTempHome(t)
	rec := mountRecord{
		ID:                   "att_alpha",
		Workspace:            "alpha",
		LocalPath:            "/tmp/alpha",
		Mode:                 modeSync,
		ProductMode:          productModeSelfHosted,
		ControlPlaneURL:      "http://127.0.0.1:8091",
		ControlPlaneDatabase: "local-dev",
		SessionID:            "sess_123",
		PID:                  12345,
		StartedAt:            time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
	}
	if err := saveMountRegistry(mountRegistry{Mounts: []mountRecord{rec}}); err != nil {
		t.Fatalf("saveMountRegistry() returned error: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return cmdStatusWithOptions(statusOptions{verbose: true})
	})
	if err != nil {
		t.Fatalf("cmdStatusWithOptions(verbose) returned error: %v", err)
	}
	for _, want := range []string{
		"Mounted workspaces",
		"control plane  http://127.0.0.1:8091",
		"database       local-dev",
		"session        sess_123",
		"mount     att_alpha",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("status verbose output missing %q:\n%s", want, out)
		}
	}
}

func nonEmptyLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func indexLine(lines []string, want string) int {
	for i, line := range lines {
		if strings.Contains(line, want) {
			return i
		}
	}
	return -1
}
