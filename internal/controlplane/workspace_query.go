package controlplane

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/redis/agent-filesystem/internal/mcptools"
	"github.com/redis/agent-filesystem/internal/queryindex"
	"github.com/redis/agent-filesystem/internal/querysearch"
	afsclient "github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

type WorkspaceQueryIndexStatusRequest struct {
	Workspace string `json:"workspace,omitempty"`
	Path      string `json:"path,omitempty"`
}

type WorkspaceQueryIndexRebuildRequest struct {
	Workspace string `json:"workspace,omitempty"`
	Path      string `json:"path,omitempty"`
	Force     bool   `json:"force,omitempty"`
	Wait      bool   `json:"wait,omitempty"`
}

type WorkspaceQueryIndexStatus struct {
	Workspace         string            `json:"workspace"`
	Path              string            `json:"path,omitempty"`
	State             string            `json:"state"`
	Message           string            `json:"message,omitempty"`
	Keyword           queryindex.Status `json:"keyword"`
	EmbeddingsEnabled bool              `json:"embeddings_enabled"`
	Model             string            `json:"model,omitempty"`
	ChunkStrategy     string            `json:"chunk_strategy,omitempty"`
}

type WorkspaceQueryIndexRebuildResponse struct {
	Workspace string                    `json:"workspace"`
	Path      string                    `json:"path,omitempty"`
	Keyword   queryindex.RebuildResult  `json:"keyword"`
	Status    WorkspaceQueryIndexStatus `json:"status"`
	Message   string                    `json:"message,omitempty"`
}

func (s *Service) QueryWorkspace(ctx context.Context, workspace string, request mcptools.FileQueryRequest) (mcptools.FileQueryResponse, error) {
	displayWorkspace := strings.TrimSpace(request.Workspace)
	if displayWorkspace == "" {
		displayWorkspace = workspace
	}
	if request.Mode == "" {
		request.Mode = mcptools.FileQueryModeHybrid
	}
	if request.Path == "" {
		request.Path = "/"
	}
	if request.Limit == 0 {
		request.Limit = 10
	}
	request.Workspace = displayWorkspace

	cfg, err := s.GetWorkspaceConfig(ctx, workspace)
	if err != nil {
		return mcptools.FileQueryResponse{}, err
	}
	if request.Mode == mcptools.FileQueryModeSemantic {
		return semanticUnavailableResponse(request, cfg), nil
	}

	if _, _, _, err := EnsureWorkspaceRoot(ctx, s.store, workspace); err != nil {
		return mcptools.FileQueryResponse{}, err
	}
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return mcptools.FileQueryResponse{}, err
	}
	fsKey := WorkspaceFSKey(workspaceStorageID(meta))
	spec := querysearch.KeywordSpecFromRequest(request)
	if len(spec.Positive) == 0 {
		return mcptools.FileQueryResponse{}, fmt.Errorf("query must include at least one searchable keyword")
	}

	warnings := workspaceQueryWarnings(request, cfg)
	results, searchErr := queryindex.Search(ctx, s.store.rdb, fsKey, queryindex.SearchSpec{
		Positive:    spec.Positive,
		Negative:    spec.Negative,
		SearchTypes: spec.SearchTypes,
	}, queryindex.SearchOptions{
		Path:           request.Path,
		Limit:          request.Limit,
		All:            request.All,
		MinScore:       request.MinScore,
		CandidateLimit: request.CandidateLimit,
		Full:           request.Full,
	})
	explain := make([]mcptools.FileQueryExplain, 0)
	switch {
	case searchErr == nil:
		explain = append(explain, mcptools.FileQueryExplain{
			Stage:   "keyword",
			Message: "Redis Search BM25 over query chunk projection.",
			Values:  map[string]any{"backend": "redissearch"},
		})
	case errors.Is(searchErr, queryindex.ErrSearchUnavailable):
		explain = append(explain, mcptools.FileQueryExplain{
			Stage:   "keyword",
			Message: "Redis Search is unavailable; falling back to direct keyword ranking.",
			Values:  map[string]any{"backend": "fallback", "reason": "redissearch_unavailable"},
		})
	case errors.Is(searchErr, queryindex.ErrProjectionStale):
		warnings = append(warnings, "Query projection is still indexing; falling back to direct keyword ranking.")
	default:
		warnings = append(warnings, "Query projection failed; falling back to direct keyword ranking: "+searchErr.Error())
	}
	if searchErr != nil {
		targets, err := collectWorkspaceQueryTargets(ctx, s.store.rdb, fsKey, request.Path)
		if err != nil {
			return mcptools.FileQueryResponse{}, err
		}
		results = querysearch.RankKeywordTargets(targets, spec, querysearch.KeywordOptions{
			Limit:    request.Limit,
			All:      request.All,
			MinScore: request.MinScore,
			Full:     request.Full,
		})
		explain = append(explain, mcptools.FileQueryExplain{
			Stage:   "keyword",
			Message: "Direct workspace content fallback.",
			Values:  map[string]any{"backend": "fallback"},
		})
	}
	if !request.Explain {
		explain = nil
	}
	return mcptools.FileQueryResponse{
		Status:    mcptools.FileQueryStatusOK,
		Workspace: displayWorkspace,
		Path:      request.Path,
		Query:     request.Query,
		Searches:  request.Searches,
		Intent:    request.Intent,
		Results:   results,
		Warnings:  warnings,
		Explain:   explain,
	}, nil
}

func (s *Service) QueryIndexStatus(ctx context.Context, workspace string, request WorkspaceQueryIndexStatusRequest) (WorkspaceQueryIndexStatus, error) {
	displayWorkspace := strings.TrimSpace(request.Workspace)
	if displayWorkspace == "" {
		displayWorkspace = workspace
	}
	queryPath := normalizeQueryPath(request.Path)
	cfg, err := s.GetWorkspaceConfig(ctx, workspace)
	if err != nil {
		return WorkspaceQueryIndexStatus{}, err
	}
	fsKey, err := s.workspaceQueryFSKey(ctx, workspace)
	if err != nil {
		return WorkspaceQueryIndexStatus{}, err
	}
	if _, err := queryindex.ProcessPending(ctx, s.store.rdb, fsKey, queryindexStatusCatchupLimit); err != nil {
		return WorkspaceQueryIndexStatus{}, err
	}
	keyword, err := queryindex.Inspect(ctx, s.store.rdb, fsKey, queryPath)
	if err != nil {
		return WorkspaceQueryIndexStatus{}, err
	}
	status := WorkspaceQueryIndexStatus{
		Workspace:         displayWorkspace,
		Path:              queryPath,
		State:             keyword.State,
		Keyword:           keyword,
		EmbeddingsEnabled: cfg.Query.Embeddings.Enabled,
		Model:             cfg.Query.Embeddings.Model,
		ChunkStrategy:     cfg.Query.Embeddings.ChunkStrategy,
	}
	switch keyword.State {
	case "needs_rebuild":
		status.Message = "Existing files are not fully indexed yet. Run query index rebuild --wait to backfill keyword chunks."
	case "indexing":
		status.Message = "Keyword query indexing is in progress."
	case "unavailable":
		status.Message = "Redis Search is unavailable; query will use direct keyword ranking fallback."
	case queryindex.StateReady:
		status.Message = "Keyword query index is ready."
	case queryindex.StateError:
		status.Message = "Keyword query index has errors; rebuild or inspect skipped files."
	}
	return status, nil
}

const queryindexStatusCatchupLimit = 128

func (s *Service) RebuildQueryIndex(ctx context.Context, workspace string, request WorkspaceQueryIndexRebuildRequest) (WorkspaceQueryIndexRebuildResponse, error) {
	displayWorkspace := strings.TrimSpace(request.Workspace)
	if displayWorkspace == "" {
		displayWorkspace = workspace
	}
	queryPath := normalizeQueryPath(request.Path)
	if _, err := s.GetWorkspaceConfig(ctx, workspace); err != nil {
		return WorkspaceQueryIndexRebuildResponse{}, err
	}
	fsKey, err := s.workspaceQueryFSKey(ctx, workspace)
	if err != nil {
		return WorkspaceQueryIndexRebuildResponse{}, err
	}
	result, err := queryindex.Rebuild(ctx, s.store.rdb, fsKey, queryindex.RebuildOptions{
		Path:  queryPath,
		Force: request.Force,
		Wait:  request.Wait,
	})
	if err != nil {
		return WorkspaceQueryIndexRebuildResponse{}, err
	}
	status, err := s.QueryIndexStatus(ctx, workspace, WorkspaceQueryIndexStatusRequest{
		Workspace: displayWorkspace,
		Path:      queryPath,
	})
	if err != nil {
		return WorkspaceQueryIndexRebuildResponse{}, err
	}
	return WorkspaceQueryIndexRebuildResponse{
		Workspace: displayWorkspace,
		Path:      queryPath,
		Keyword:   result,
		Status:    status,
		Message:   fmt.Sprintf("Enqueued %d file(s) for keyword query indexing.", result.Enqueued),
	}, nil
}

func (s *Service) workspaceQueryFSKey(ctx context.Context, workspace string) (string, error) {
	if _, _, _, err := EnsureWorkspaceRoot(ctx, s.store, workspace); err != nil {
		return "", err
	}
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return "", err
	}
	return WorkspaceFSKey(workspaceStorageID(meta)), nil
}

func semanticUnavailableResponse(request mcptools.FileQueryRequest, cfg WorkspaceConfig) mcptools.FileQueryResponse {
	message := "Semantic query is not available in this build yet."
	if !cfg.Query.Embeddings.Enabled {
		message = "Semantic query is disabled for this workspace."
	}
	return mcptools.FileQueryResponse{
		Status:    mcptools.FileQueryStatusUnavailable,
		Workspace: request.Workspace,
		Path:      request.Path,
		Query:     request.Query,
		Searches:  request.Searches,
		Intent:    request.Intent,
		Results:   []mcptools.FileQueryResult{},
		Warnings:  []string{message},
	}
}

func workspaceQueryWarnings(request mcptools.FileQueryRequest, cfg WorkspaceConfig) []string {
	warnings := make([]string, 0)
	if request.Mode == mcptools.FileQueryModeHybrid && cfg.Query.Embeddings.Enabled {
		warnings = append(warnings, "Hybrid vector/rerank retrieval is not ready yet; showing keyword-ranked results.")
	}
	if request.Mode == mcptools.FileQueryModeHybrid && !cfg.Query.Embeddings.Enabled && querysearch.HasSemanticClauses(request.Searches) {
		warnings = append(warnings, "Embeddings are disabled; vec:/hyde: clauses were used as keyword text only.")
	}
	return warnings
}

func collectWorkspaceQueryTargets(ctx context.Context, rdb *redis.Client, fsKey, rawPath string) ([]querysearch.Target, error) {
	fsClient := afsclient.New(rdb, fsKey)
	normalizedPath := normalizeQueryPath(rawPath)
	stat, err := fsClient.Stat(ctx, normalizedPath)
	if err != nil {
		return nil, err
	}
	if stat == nil {
		return nil, os.ErrNotExist
	}
	if stat.Type == "file" {
		data, err := fsClient.Cat(ctx, normalizedPath)
		if err != nil {
			return nil, err
		}
		if isBinary(data) {
			return []querysearch.Target{}, nil
		}
		return []querysearch.Target{{Path: normalizedPath, Content: data}}, nil
	}
	if stat.Type != "dir" {
		return []querysearch.Target{}, nil
	}
	items := make([]treeItem, 0)
	if err := appendWorkingCopyTreeItems(ctx, fsClient, normalizedPath, 4096, &items); err != nil {
		return nil, err
	}
	targets := make([]querysearch.Target, 0, len(items))
	for _, item := range items {
		if item.Kind != "file" {
			continue
		}
		data, err := fsClient.Cat(ctx, item.Path)
		if err != nil {
			return nil, err
		}
		if isBinary(data) {
			continue
		}
		targets = append(targets, querysearch.Target{Path: item.Path, Content: data})
	}
	return targets, nil
}

func normalizeQueryPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	clean := path.Clean(p)
	if clean == "." {
		return "/"
	}
	return clean
}
