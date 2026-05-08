package controlplane

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/queryembedding"
	"github.com/redis/agent-filesystem/internal/queryindex"
	"github.com/redis/go-redis/v9"
)

func TestQueryIndexStatusDrainsPendingWork(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	store := NewStore(rdb)
	service := NewService(Config{}, store)
	now := time.Now().UTC()
	meta := WorkspaceMeta{
		Version:          formatVersion,
		Name:             "repo",
		CreatedAt:        now,
		UpdatedAt:        now,
		HeadSavepoint:    initialCheckpointName,
		DefaultSavepoint: initialCheckpointName,
	}
	if err := store.PutWorkspaceMeta(ctx, meta); err != nil {
		t.Fatalf("PutWorkspaceMeta() returned error: %v", err)
	}

	manifestValue := Manifest{
		Version:   formatVersion,
		Workspace: "repo",
		Savepoint: initialCheckpointName,
		Entries: map[string]ManifestEntry{
			"/": {Type: "dir", Mode: 0o755, MtimeMs: now.UnixMilli()},
		},
	}
	for i := 0; i < 13; i++ {
		filePath := fmt.Sprintf("/docs/file-%02d.md", i+1)
		content := []byte(fmt.Sprintf("file %02d query index content\n", i+1))
		manifestValue.Entries[filePath] = ManifestEntry{
			Type:    "file",
			Mode:    0o644,
			MtimeMs: now.UnixMilli(),
			Size:    int64(len(content)),
			Inline:  base64.StdEncoding.EncodeToString(content),
		}
	}
	savepoint := SavepointMeta{
		Version:    formatVersion,
		ID:         initialCheckpointName,
		Name:       initialCheckpointName,
		Workspace:  "repo",
		CreatedAt:  now,
		FileCount:  13,
		DirCount:   1,
		TotalBytes: 13,
	}
	if err := store.PutSavepoint(ctx, savepoint, manifestValue); err != nil {
		t.Fatalf("PutSavepoint() returned error: %v", err)
	}

	if _, _, _, err := EnsureWorkspaceRoot(ctx, store, "repo"); err != nil {
		t.Fatalf("EnsureWorkspaceRoot() returned error: %v", err)
	}
	before, err := queryindex.Inspect(ctx, rdb, WorkspaceFSKey("repo"), "/")
	if err != nil {
		t.Fatalf("Inspect(before) returned error: %v", err)
	}
	if before.Pending != 13 {
		t.Fatalf("Inspect(before).Pending = %d, want 13", before.Pending)
	}

	status, err := service.QueryIndexStatus(ctx, "repo", WorkspaceQueryIndexStatusRequest{Path: "/"})
	if err != nil {
		t.Fatalf("QueryIndexStatus() returned error: %v", err)
	}
	if status.Keyword.Pending != 0 || status.Keyword.Stale != 0 {
		t.Fatalf("QueryIndexStatus() keyword = %+v, want drained pending/stale work", status.Keyword)
	}
	if status.Keyword.Ready != 13 {
		t.Fatalf("QueryIndexStatus().Keyword.Ready = %d, want 13", status.Keyword.Ready)
	}
}

func TestQueryEmbeddingUnavailableMessageCleansOpenAIModelAccessError(t *testing.T) {
	err := fmt.Errorf("%w: %w", queryembedding.ErrUnavailable, &queryembedding.OpenAIAPIError{
		StatusCode: 403,
		Code:       "model_not_found",
		Message:    "Project `proj_test` does not have access to model `text-embedding-3-small`",
		Model:      "text-embedding-3-small",
	})

	got := queryEmbeddingUnavailableMessage(err)
	for _, want := range []string{
		"OpenAI model \"text-embedding-3-small\" is not available to this project",
		"set AFS_EMBED_MODEL in the control-plane environment",
		"restart the control plane",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("queryEmbeddingUnavailableMessage() = %q, want substring %q", got, want)
		}
	}
	for _, unwanted := range []string{"proj_test", "{", "model_not_found"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("queryEmbeddingUnavailableMessage() = %q, did not want substring %q", got, unwanted)
		}
	}

	if !errors.Is(err, queryembedding.ErrUnavailable) {
		t.Fatalf("test error = %v, want ErrUnavailable", err)
	}
}

func TestRebuildQueryIndexCanCreateSemanticEmbeddings(t *testing.T) {
	t.Setenv("AFS_EMBED_PROVIDER", "test")
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})
	store := NewStore(rdb)
	service := NewService(Config{}, store)
	now := time.Now().UTC()
	meta := WorkspaceMeta{
		Version:          formatVersion,
		Name:             "repo",
		CreatedAt:        now,
		UpdatedAt:        now,
		HeadSavepoint:    initialCheckpointName,
		DefaultSavepoint: initialCheckpointName,
	}
	if err := store.PutWorkspaceMeta(ctx, meta); err != nil {
		t.Fatalf("PutWorkspaceMeta() returned error: %v", err)
	}
	content := []byte("semantic checkpoint guide\n")
	manifestValue := Manifest{
		Version:   formatVersion,
		Workspace: "repo",
		Savepoint: initialCheckpointName,
		Entries: map[string]ManifestEntry{
			"/":              {Type: "dir", Mode: 0o755, MtimeMs: now.UnixMilli()},
			"/docs/guide.md": {Type: "file", Mode: 0o644, MtimeMs: now.UnixMilli(), Size: int64(len(content)), Inline: base64.StdEncoding.EncodeToString(content)},
		},
	}
	if err := store.PutSavepoint(ctx, SavepointMeta{
		Version:   formatVersion,
		ID:        initialCheckpointName,
		Name:      initialCheckpointName,
		Workspace: "repo",
		CreatedAt: now,
		FileCount: 1,
		DirCount:  1,
	}, manifestValue); err != nil {
		t.Fatalf("PutSavepoint() returned error: %v", err)
	}
	if _, _, _, err := EnsureWorkspaceRoot(ctx, store, "repo"); err != nil {
		t.Fatalf("EnsureWorkspaceRoot() returned error: %v", err)
	}

	response, err := service.RebuildQueryIndex(ctx, "repo", WorkspaceQueryIndexRebuildRequest{Path: "/", Embeddings: true})
	if err != nil {
		t.Fatalf("RebuildQueryIndex() returned error: %v", err)
	}
	if response.Embeddings == nil || response.Embeddings.Embedded == 0 || !response.Embeddings.Available {
		t.Fatalf("embedding response = %+v, want created embeddings", response.Embeddings)
	}
	chunkKeys, err := rdb.Keys(ctx, queryindex.ChunkPrefix(WorkspaceFSKey("repo"))+"*").Result()
	if err != nil {
		t.Fatalf("KEYS chunks returned error: %v", err)
	}
	if len(chunkKeys) == 0 {
		t.Fatal("expected query chunks")
	}
	model, err := rdb.HGet(ctx, chunkKeys[0], "embedding_model").Result()
	if err != nil || model == "" {
		t.Fatalf("embedding_model = %q, %v; want stored model", model, err)
	}
}

func TestCleanQueryIndexResetsGeneratedData(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	store := NewStore(rdb)
	service := NewService(Config{}, store)
	now := time.Now().UTC()
	meta := WorkspaceMeta{
		Version:          formatVersion,
		Name:             "repo",
		CreatedAt:        now,
		UpdatedAt:        now,
		HeadSavepoint:    initialCheckpointName,
		DefaultSavepoint: initialCheckpointName,
	}
	if err := store.PutWorkspaceMeta(ctx, meta); err != nil {
		t.Fatalf("PutWorkspaceMeta() returned error: %v", err)
	}

	content := []byte("checkpoint recovery\n")
	manifestValue := Manifest{
		Version:   formatVersion,
		Workspace: "repo",
		Savepoint: initialCheckpointName,
		Entries: map[string]ManifestEntry{
			"/":                    {Type: "dir", Mode: 0o755, MtimeMs: now.UnixMilli()},
			"/docs/checkpoints.md": {Type: "file", Mode: 0o644, MtimeMs: now.UnixMilli(), Size: int64(len(content)), Inline: base64.StdEncoding.EncodeToString(content)},
		},
	}
	savepoint := SavepointMeta{
		Version:    formatVersion,
		ID:         initialCheckpointName,
		Name:       initialCheckpointName,
		Workspace:  "repo",
		CreatedAt:  now,
		FileCount:  1,
		DirCount:   1,
		TotalBytes: int64(len(content)),
	}
	if err := store.PutSavepoint(ctx, savepoint, manifestValue); err != nil {
		t.Fatalf("PutSavepoint() returned error: %v", err)
	}

	if _, _, _, err := EnsureWorkspaceRoot(ctx, store, "repo"); err != nil {
		t.Fatalf("EnsureWorkspaceRoot() returned error: %v", err)
	}
	before, err := service.QueryIndexStatus(ctx, "repo", WorkspaceQueryIndexStatusRequest{Path: "/"})
	if err != nil {
		t.Fatalf("QueryIndexStatus(before) returned error: %v", err)
	}
	if before.Keyword.Chunks == 0 {
		t.Fatalf("QueryIndexStatus(before) = %+v, want indexed chunks", before.Keyword)
	}

	cleaned, err := service.CleanQueryIndex(ctx, "repo", WorkspaceQueryIndexCleanRequest{Confirm: true})
	if err != nil {
		t.Fatalf("CleanQueryIndex() returned error: %v", err)
	}
	if !cleaned.Cleared || cleaned.RemovedFiles != 1 || cleaned.RemovedChunks == 0 {
		t.Fatalf("CleanQueryIndex() = %+v, want cleared response with removed files/chunks", cleaned)
	}
	if cleaned.Status.Keyword.Chunks != 0 {
		t.Fatalf("CleanQueryIndex() status keyword = %+v, want zero chunks after clean", cleaned.Status.Keyword)
	}
}
