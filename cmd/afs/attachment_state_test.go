package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAttachmentRegistryRoundTripAndPathLookup(t *testing.T) {
	t.Helper()

	withTempHome(t)
	root := t.TempDir()
	rec := attachmentRecord{
		ID:        "att_test",
		Workspace: "notes",
		LocalPath: filepath.Join(root, "notes"),
		Mode:      modeSync,
		PID:       123,
		StartedAt: time.Now().UTC(),
	}
	reg := attachmentRegistry{}
	upsertAttachment(&reg, rec)
	if err := saveAttachmentRegistry(reg); err != nil {
		t.Fatalf("saveAttachmentRegistry() returned error: %v", err)
	}

	loaded, err := loadAttachmentRegistry()
	if err != nil {
		t.Fatalf("loadAttachmentRegistry() returned error: %v", err)
	}
	got, ok := attachmentByPath(loaded, rec.LocalPath)
	if !ok {
		t.Fatalf("attachmentByPath(%s) returned false", rec.LocalPath)
	}
	if got.Workspace != "notes" {
		t.Fatalf("Workspace = %q, want notes", got.Workspace)
	}
}

func TestAttachmentPathConflictDetectsNestedPaths(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	child := filepath.Join(parent, "child")
	reg := attachmentRegistry{Attachments: []attachmentRecord{{
		Workspace: "notes",
		LocalPath: parent,
	}}}
	if _, ok := attachmentPathConflict(reg, child); !ok {
		t.Fatalf("attachmentPathConflict() returned false for nested path")
	}
}

func TestCmdStatusPrintsAlignedAttachmentTable(t *testing.T) {
	t.Helper()

	withTempHome(t)
	reg := attachmentRegistry{Attachments: []attachmentRecord{
		{
			ID:        "att_beta",
			Workspace: "beta-workspace",
			LocalPath: "/tmp/beta-workspace",
			Mode:      modeSync,
			StartedAt: time.Now().UTC(),
		},
		{
			ID:        "att_alpha",
			Workspace: "alpha",
			LocalPath: "/tmp/alpha",
			Mode:      modeSync,
			StartedAt: time.Now().UTC(),
		},
	}}
	if err := saveAttachmentRegistry(reg); err != nil {
		t.Fatalf("saveAttachmentRegistry() returned error: %v", err)
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
	statusLine := indexLine(lines, "AFS Daemon Not Running")
	attachmentLine := indexLine(lines, "Attached workspaces")
	if statusLine < 0 {
		t.Fatalf("status output missing daemon status section:\n%s", out)
	}
	if attachmentLine < 0 {
		t.Fatalf("status output missing attachment section:\n%s", out)
	}
	if statusLine > attachmentLine {
		t.Fatalf("daemon status should print before attachments:\n%s", out)
	}
	if attachmentLine+3 >= len(lines) {
		t.Fatalf("attachment table incomplete:\n%s", out)
	}
	headerPathCol := strings.Index(lines[attachmentLine+1], "Path")
	firstPathCol := strings.Index(lines[attachmentLine+2], "/tmp/alpha")
	secondPathCol := strings.Index(lines[attachmentLine+3], "/tmp/beta-workspace")
	if headerPathCol < 0 || firstPathCol != headerPathCol || secondPathCol != headerPathCol {
		t.Fatalf("path column not aligned:\n%s", out)
	}
}

func TestCmdStatusVerboseIncludesConnectionDetails(t *testing.T) {
	t.Helper()

	withTempHome(t)
	rec := attachmentRecord{
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
	if err := saveAttachmentRegistry(attachmentRegistry{Attachments: []attachmentRecord{rec}}); err != nil {
		t.Fatalf("saveAttachmentRegistry() returned error: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return cmdStatusWithOptions(statusOptions{verbose: true})
	})
	if err != nil {
		t.Fatalf("cmdStatusWithOptions(verbose) returned error: %v", err)
	}
	for _, want := range []string{
		"Attached workspaces",
		"control plane  http://127.0.0.1:8091",
		"database       local-dev",
		"session        sess_123",
		"attachment     att_alpha",
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
