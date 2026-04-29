package controlplane

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestNormalizeWorkspaceVersioningPolicyAppliesDefaultsAndDedupesGlobs(t *testing.T) {
	policy := NormalizeWorkspaceVersioningPolicy(WorkspaceVersioningPolicy{
		IncludeGlobs: []string{" src/** ", "src/**", "", "pkg/**"},
		ExcludeGlobs: []string{" **/*.log ", "**/*.log", "tmp/**"},
	})

	if policy.Mode != WorkspaceVersioningModeOff {
		t.Fatalf("mode = %q, want %q", policy.Mode, WorkspaceVersioningModeOff)
	}
	if len(policy.IncludeGlobs) != 2 || policy.IncludeGlobs[0] != "src/**" || policy.IncludeGlobs[1] != "pkg/**" {
		t.Fatalf("include_globs = %#v, want [src/** pkg/**]", policy.IncludeGlobs)
	}
	if len(policy.ExcludeGlobs) != 2 || policy.ExcludeGlobs[0] != "**/*.log" || policy.ExcludeGlobs[1] != "tmp/**" {
		t.Fatalf("exclude_globs = %#v, want [**/*.log tmp/**]", policy.ExcludeGlobs)
	}
}

func TestValidateWorkspaceVersioningPolicy(t *testing.T) {
	valid := NormalizeWorkspaceVersioningPolicy(WorkspaceVersioningPolicy{
		Mode:                 WorkspaceVersioningModePaths,
		IncludeGlobs:         []string{"src/**"},
		ExcludeGlobs:         []string{"tmp/**"},
		MaxVersionsPerFile:   10,
		MaxAgeDays:           30,
		MaxTotalBytes:        1024,
		LargeFileCutoffBytes: 2048,
	})
	if err := ValidateWorkspaceVersioningPolicy(valid); err != nil {
		t.Fatalf("valid policy returned error: %v", err)
	}

	tests := []struct {
		name   string
		policy WorkspaceVersioningPolicy
	}{
		{
			name:   "invalid mode",
			policy: WorkspaceVersioningPolicy{Mode: "weird"},
		},
		{
			name:   "negative per-file limit",
			policy: WorkspaceVersioningPolicy{Mode: WorkspaceVersioningModeOff, MaxVersionsPerFile: -1},
		},
		{
			name:   "negative age",
			policy: WorkspaceVersioningPolicy{Mode: WorkspaceVersioningModeOff, MaxAgeDays: -1},
		},
		{
			name:   "negative total bytes",
			policy: WorkspaceVersioningPolicy{Mode: WorkspaceVersioningModeOff, MaxTotalBytes: -1},
		},
		{
			name:   "negative large file cutoff",
			policy: WorkspaceVersioningPolicy{Mode: WorkspaceVersioningModeOff, LargeFileCutoffBytes: -1},
		},
		{
			name:   "paths mode requires includes",
			policy: WorkspaceVersioningPolicy{Mode: WorkspaceVersioningModePaths},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateWorkspaceVersioningPolicy(test.policy); err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

func TestStoreWorkspaceVersioningPolicyDefaultsOffWhenUnset(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	meta := WorkspaceMeta{
		Version:       1,
		ID:            "ws-1",
		Name:          "repo",
		CreatedAt:     time.Unix(1, 0).UTC(),
		UpdatedAt:     time.Unix(1, 0).UTC(),
		HeadSavepoint: "initial",
	}
	if err := store.PutWorkspaceMeta(ctx, meta); err != nil {
		t.Fatalf("PutWorkspaceMeta: %v", err)
	}

	policy, err := store.GetWorkspaceVersioningPolicy(ctx, "repo")
	if err != nil {
		t.Fatalf("GetWorkspaceVersioningPolicy: %v", err)
	}
	if policy.Mode != WorkspaceVersioningModeOff {
		t.Fatalf("mode = %q, want %q", policy.Mode, WorkspaceVersioningModeOff)
	}
	if len(policy.IncludeGlobs) != 0 || len(policy.ExcludeGlobs) != 0 {
		t.Fatalf("unexpected default globs: include=%#v exclude=%#v", policy.IncludeGlobs, policy.ExcludeGlobs)
	}
}

func TestStoreWorkspaceVersioningPolicyRoundTripsByWorkspaceNameOrID(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	meta := WorkspaceMeta{
		Version:       1,
		ID:            "ws-1",
		Name:          "repo",
		CreatedAt:     time.Unix(1, 0).UTC(),
		UpdatedAt:     time.Unix(1, 0).UTC(),
		HeadSavepoint: "initial",
	}
	if err := store.PutWorkspaceMeta(ctx, meta); err != nil {
		t.Fatalf("PutWorkspaceMeta: %v", err)
	}

	input := WorkspaceVersioningPolicy{
		Mode:                 WorkspaceVersioningModePaths,
		IncludeGlobs:         []string{" src/** ", "pkg/**", "src/**"},
		ExcludeGlobs:         []string{" **/*.log ", "tmp/**"},
		MaxVersionsPerFile:   42,
		MaxAgeDays:           14,
		MaxTotalBytes:        4096,
		LargeFileCutoffBytes: 8192,
	}
	if err := store.PutWorkspaceVersioningPolicy(ctx, "repo", input); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy: %v", err)
	}

	gotByName, err := store.GetWorkspaceVersioningPolicy(ctx, "repo")
	if err != nil {
		t.Fatalf("GetWorkspaceVersioningPolicy(name): %v", err)
	}
	gotByID, err := store.GetWorkspaceVersioningPolicy(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceVersioningPolicy(id): %v", err)
	}

	if !reflect.DeepEqual(gotByName, gotByID) {
		t.Fatalf("policy mismatch by lookup path: name=%+v id=%+v", gotByName, gotByID)
	}
	if gotByName.Mode != WorkspaceVersioningModePaths {
		t.Fatalf("mode = %q, want %q", gotByName.Mode, WorkspaceVersioningModePaths)
	}
	if len(gotByName.IncludeGlobs) != 2 || gotByName.IncludeGlobs[0] != "src/**" || gotByName.IncludeGlobs[1] != "pkg/**" {
		t.Fatalf("include_globs = %#v, want [src/** pkg/**]", gotByName.IncludeGlobs)
	}
	if len(gotByName.ExcludeGlobs) != 2 || gotByName.ExcludeGlobs[0] != "**/*.log" || gotByName.ExcludeGlobs[1] != "tmp/**" {
		t.Fatalf("exclude_globs = %#v, want [**/*.log tmp/**]", gotByName.ExcludeGlobs)
	}
	if gotByName.MaxVersionsPerFile != 42 || gotByName.MaxAgeDays != 14 || gotByName.MaxTotalBytes != 4096 || gotByName.LargeFileCutoffBytes != 8192 {
		t.Fatalf("unexpected retention values: %+v", gotByName)
	}
}

func TestStoreWorkspaceVersioningPolicyRejectsUnknownWorkspace(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	err := store.PutWorkspaceVersioningPolicy(ctx, "missing", WorkspaceVersioningPolicy{Mode: WorkspaceVersioningModeOff})
	if err == nil {
		t.Fatal("expected missing workspace error, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want os.ErrNotExist", err)
	}
}
