package controlplane

import (
	"context"
	"testing"
	"time"
)

func TestStoreWorkspaceConfigRoundTripsQueryEmbeddingsAndVersioning(t *testing.T) {
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

	input := WorkspaceConfig{
		Versioning: WorkspaceVersioningPolicy{
			Mode:         WorkspaceVersioningModePaths,
			IncludeGlobs: []string{" src/** ", "src/**"},
		},
		Query: WorkspaceQueryConfig{
			Embeddings: WorkspaceQueryEmbeddingsConfig{
				Enabled:       true,
				Model:         " embeddinggemma ",
				ChunkStrategy: "AUTO",
			},
		},
	}
	if err := store.PutWorkspaceConfig(ctx, "repo", input); err != nil {
		t.Fatalf("PutWorkspaceConfig: %v", err)
	}

	got, err := store.GetWorkspaceConfig(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceConfig: %v", err)
	}
	if got.Versioning.Mode != WorkspaceVersioningModePaths {
		t.Fatalf("versioning mode = %q, want %q", got.Versioning.Mode, WorkspaceVersioningModePaths)
	}
	if len(got.Versioning.IncludeGlobs) != 1 || got.Versioning.IncludeGlobs[0] != "src/**" {
		t.Fatalf("include globs = %#v, want [src/**]", got.Versioning.IncludeGlobs)
	}
	if !got.Query.Embeddings.Enabled {
		t.Fatal("query embeddings enabled = false, want true")
	}
	if got.Query.Embeddings.Model != "embeddinggemma" {
		t.Fatalf("query embeddings model = %q, want embeddinggemma", got.Query.Embeddings.Model)
	}
	if got.Query.Embeddings.ChunkStrategy != WorkspaceQueryChunkStrategyAuto {
		t.Fatalf("query embeddings chunk strategy = %q, want %q", got.Query.Embeddings.ChunkStrategy, WorkspaceQueryChunkStrategyAuto)
	}

	policy, err := store.GetWorkspaceVersioningPolicy(ctx, "repo")
	if err != nil {
		t.Fatalf("GetWorkspaceVersioningPolicy: %v", err)
	}
	if policy.Mode != WorkspaceVersioningModePaths {
		t.Fatalf("policy mode = %q, want %q", policy.Mode, WorkspaceVersioningModePaths)
	}
}

func TestStoreWorkspaceConfigComposesLatestVersioningPolicy(t *testing.T) {
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
	if err := store.PutWorkspaceConfig(ctx, "repo", WorkspaceConfig{
		Query: WorkspaceQueryConfig{
			Embeddings: WorkspaceQueryEmbeddingsConfig{Enabled: true},
		},
	}); err != nil {
		t.Fatalf("PutWorkspaceConfig: %v", err)
	}
	if err := store.PutWorkspaceVersioningPolicy(ctx, "repo", WorkspaceVersioningPolicy{
		Mode: WorkspaceVersioningModeAll,
	}); err != nil {
		t.Fatalf("PutWorkspaceVersioningPolicy: %v", err)
	}

	got, err := store.GetWorkspaceConfig(ctx, "repo")
	if err != nil {
		t.Fatalf("GetWorkspaceConfig: %v", err)
	}
	if got.Versioning.Mode != WorkspaceVersioningModeAll {
		t.Fatalf("versioning mode = %q, want %q", got.Versioning.Mode, WorkspaceVersioningModeAll)
	}
	if !got.Query.Embeddings.Enabled {
		t.Fatal("query embeddings enabled = false, want true")
	}
}

func TestValidateWorkspaceConfigRejectsInvalidChunkStrategy(t *testing.T) {
	cfg := NormalizeWorkspaceConfig(WorkspaceConfig{
		Query: WorkspaceQueryConfig{
			Embeddings: WorkspaceQueryEmbeddingsConfig{ChunkStrategy: "weird"},
		},
	})
	if err := ValidateWorkspaceConfig(cfg); err == nil {
		t.Fatal("expected invalid chunk strategy error, got nil")
	}
}
