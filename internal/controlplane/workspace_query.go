package controlplane

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/redis/agent-filesystem/internal/mcptools"
	"github.com/redis/agent-filesystem/internal/queryembedding"
	"github.com/redis/agent-filesystem/internal/queryindex"
	"github.com/redis/agent-filesystem/internal/querysearch"
	"github.com/redis/agent-filesystem/internal/queryvector"
	afsclient "github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

type WorkspaceQueryIndexStatusRequest struct {
	Workspace string `json:"workspace,omitempty"`
	Path      string `json:"path,omitempty"`
}

type WorkspaceQueryIndexRebuildRequest struct {
	Workspace  string `json:"workspace,omitempty"`
	Path       string `json:"path,omitempty"`
	Force      bool   `json:"force,omitempty"`
	Wait       bool   `json:"wait,omitempty"`
	Embeddings bool   `json:"embeddings,omitempty"`
}

type WorkspaceQueryIndexCleanRequest struct {
	Workspace string `json:"workspace,omitempty"`
	Confirm   bool   `json:"confirm,omitempty"`
}

type WorkspaceQueryIndexStatus struct {
	Workspace  string               `json:"workspace"`
	Path       string               `json:"path,omitempty"`
	State      string               `json:"state"`
	Message    string               `json:"message,omitempty"`
	Keyword    queryindex.Status    `json:"keyword"`
	Embeddings QueryEmbeddingStatus `json:"embeddings"`
}

type QueryEmbeddingStatus struct {
	Enabled   bool   `json:"enabled"`
	Available bool   `json:"available"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Dimension int    `json:"dimension,omitempty"`
	Message   string `json:"message,omitempty"`
}

type WorkspaceQueryIndexRebuildResponse struct {
	Workspace  string                        `json:"workspace"`
	Path       string                        `json:"path,omitempty"`
	Keyword    queryindex.RebuildResult      `json:"keyword"`
	Embeddings *QueryEmbeddingBackfillResult `json:"embeddings,omitempty"`
	Status     WorkspaceQueryIndexStatus     `json:"status"`
	Message    string                        `json:"message,omitempty"`
}

type QueryEmbeddingBackfillResult struct {
	Enabled   bool   `json:"enabled"`
	Available bool   `json:"available"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Dimension int    `json:"dimension,omitempty"`
	Scanned   int    `json:"scanned"`
	Embedded  int    `json:"embedded"`
	Message   string `json:"message,omitempty"`
}

type WorkspaceQueryIndexCleanResponse struct {
	Workspace     string                    `json:"workspace"`
	Cleared       bool                      `json:"cleared"`
	RemovedFiles  int                       `json:"removed_files"`
	RemovedChunks int                       `json:"removed_chunks"`
	Status        WorkspaceQueryIndexStatus `json:"status"`
	Message       string                    `json:"message,omitempty"`
}

func SemanticQueryDisabledMessage(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "Semantic query is disabled for this workspace."
	}
	return fmt.Sprintf("Semantic query is disabled for workspace %q.", workspace)
}

func SemanticQueryUnavailableMessage(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "Semantic query is not available in this build yet."
	}
	return fmt.Sprintf("Semantic query is not available in this build yet for workspace %q.", workspace)
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

	if request.Mode == mcptools.FileQueryModeSemantic {
		return s.queryWorkspaceSemantic(ctx, workspace, request)
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

	warnings := workspaceQueryWarnings(request)
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

func (s *Service) queryWorkspaceSemantic(ctx context.Context, workspace string, request mcptools.FileQueryRequest) (mcptools.FileQueryResponse, error) {
	if _, _, _, err := EnsureWorkspaceRoot(ctx, s.store, workspace); err != nil {
		return mcptools.FileQueryResponse{}, err
	}
	meta, err := s.store.GetWorkspaceMeta(ctx, workspace)
	if err != nil {
		return mcptools.FileQueryResponse{}, err
	}
	queryText := workspaceSemanticQueryText(request)
	if queryText == "" {
		return mcptools.FileQueryResponse{}, fmt.Errorf("semantic query must include text")
	}
	provider, err := queryembedding.NewProviderFromEnv("")
	if err != nil {
		return semanticUnavailableResponseWithMessage(request, queryEmbeddingUnavailableMessage(err)), nil
	}
	vectorResult, err := queryvector.Search(ctx, s.store.rdb, WorkspaceFSKey(workspaceStorageID(meta)), provider, queryText, queryvector.SearchOptions{
		Path:           request.Path,
		Limit:          request.Limit,
		All:            request.All,
		MinScore:       request.MinScore,
		CandidateLimit: request.CandidateLimit,
		Full:           request.Full,
	})
	if err != nil {
		if errors.Is(err, queryembedding.ErrUnavailable) {
			return semanticUnavailableResponseWithMessage(request, queryEmbeddingUnavailableMessage(err)), nil
		}
		return mcptools.FileQueryResponse{}, err
	}
	warnings := append([]string(nil), vectorResult.Warnings...)
	if len(vectorResult.Results) == 0 && vectorResult.Stats.ChunksScanned == 0 {
		return semanticUnavailableResponseWithMessage(request, "No semantic embeddings are indexed for this query scope. Run query index create --embeddings --wait to build them."), nil
	}
	explain := []mcptools.FileQueryExplain(nil)
	if request.Explain {
		explain = append(explain, mcptools.FileQueryExplain{
			Stage:   "semantic",
			Message: "Vector-ranked retrieval over query chunk embeddings.",
			Values: map[string]any{
				"backend":          vectorResult.Stats.Backend,
				"model":            vectorResult.Stats.Model,
				"dimension":        vectorResult.Stats.Dimension,
				"search_available": vectorResult.Stats.SearchAvailable,
				"chunks_scanned":   vectorResult.Stats.ChunksScanned,
				"chunks_embedded":  vectorResult.Stats.ChunksEmbedded,
			},
		})
	}
	return mcptools.FileQueryResponse{
		Status:    mcptools.FileQueryStatusOK,
		Workspace: request.Workspace,
		Path:      request.Path,
		Query:     request.Query,
		Searches:  request.Searches,
		Intent:    request.Intent,
		Results:   vectorResult.Results,
		Warnings:  warnings,
		Explain:   explain,
	}, nil
}

func workspaceSemanticQueryText(request mcptools.FileQueryRequest) string {
	if strings.TrimSpace(request.Query) != "" {
		return strings.TrimSpace(request.Query)
	}
	parts := make([]string, 0, len(request.Searches)+1)
	for _, search := range request.Searches {
		switch search.Type {
		case mcptools.FileQuerySearchVec, mcptools.FileQuerySearchHyde, mcptools.FileQuerySearchLex:
			if text := strings.TrimSpace(search.Query); text != "" {
				parts = append(parts, text)
			}
		}
	}
	if request.Intent != "" {
		parts = append(parts, request.Intent)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func queryEmbeddingStatus() QueryEmbeddingStatus {
	provider, err := queryembedding.NewProviderFromEnv("")
	if err == nil {
		return QueryEmbeddingStatus{
			Enabled:   true,
			Available: true,
			Provider:  provider.Name(),
			Model:     provider.Model(),
			Dimension: provider.Dimension(),
			Message:   "Semantic embeddings are ready.",
		}
	}
	providerName, model := queryEmbeddingConfiguredProviderModel()
	return QueryEmbeddingStatus{
		Enabled:  true,
		Provider: providerName,
		Model:    model,
		Message:  queryEmbeddingUnavailableMessage(err),
	}
}

func queryEmbeddingConfiguredProviderModel() (string, string) {
	provider := strings.TrimSpace(strings.ToLower(os.Getenv("AFS_EMBED_PROVIDER")))
	model := strings.TrimSpace(os.Getenv("AFS_EMBED_MODEL"))
	if strings.HasPrefix(strings.ToLower(model), "openai:") {
		provider = "openai"
	}
	if strings.HasPrefix(strings.ToLower(model), "hf:") || strings.HasPrefix(strings.ToLower(model), "local:") {
		provider = "local"
	}
	if provider == "" {
		provider = queryembedding.DefaultProvider
	}
	if model == "" && provider == "openai" {
		model = "openai:" + queryembedding.DefaultOpenAIModel
	}
	if model == "" && provider == "local" {
		model = queryembedding.DefaultLocalModel
	}
	if provider == "openai" && model != "" && !strings.HasPrefix(strings.ToLower(model), "openai:") {
		model = "openai:" + model
	}
	return provider, model
}

func queryEmbeddingUnavailableMessage(err error) string {
	if err == nil {
		return "Semantic embeddings are unavailable."
	}
	var openAIErr *queryembedding.OpenAIAPIError
	if errors.As(err, &openAIErr) {
		return queryEmbeddingOpenAIUnavailableMessage(openAIErr)
	}
	msg := strings.TrimSpace(err.Error())
	if strings.Contains(msg, "OPENAI_API_KEY") {
		return "Semantic embeddings are unavailable. Set OPENAI_API_KEY in the control-plane environment."
	}
	if strings.HasPrefix(msg, queryembedding.ErrUnavailable.Error()+":") {
		msg = strings.TrimSpace(strings.TrimPrefix(msg, queryembedding.ErrUnavailable.Error()+":"))
	}
	if msg == "" {
		return "Semantic embeddings are unavailable."
	}
	return "Semantic embeddings are unavailable. " + msg
}

func queryEmbeddingOpenAIUnavailableMessage(err *queryembedding.OpenAIAPIError) string {
	if err == nil {
		return "Semantic embeddings are unavailable."
	}
	model := strings.TrimSpace(err.Model)
	if model == "" {
		model = queryembedding.DefaultOpenAIModel
	}
	if err.ModelUnavailable() {
		return fmt.Sprintf("Semantic embeddings are unavailable. OpenAI model %q is not available to this project; set AFS_EMBED_MODEL in the control-plane environment to an available embeddings model and restart the control plane.", model)
	}
	if err.Message != "" {
		return fmt.Sprintf("Semantic embeddings are unavailable. OpenAI embeddings request failed with HTTP %d: %s", err.StatusCode, err.Message)
	}
	return fmt.Sprintf("Semantic embeddings are unavailable. OpenAI embeddings request failed with HTTP %d.", err.StatusCode)
}

func (s *Service) QueryIndexStatus(ctx context.Context, workspace string, request WorkspaceQueryIndexStatusRequest) (WorkspaceQueryIndexStatus, error) {
	displayWorkspace := strings.TrimSpace(request.Workspace)
	if displayWorkspace == "" {
		displayWorkspace = workspace
	}
	queryPath := normalizeQueryPath(request.Path)
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
	embeddings := queryEmbeddingStatus()
	status := WorkspaceQueryIndexStatus{
		Workspace:  displayWorkspace,
		Path:       queryPath,
		State:      keyword.State,
		Keyword:    keyword,
		Embeddings: embeddings,
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
	keywordWait := request.Wait || request.Embeddings
	result, err := queryindex.Rebuild(ctx, s.store.rdb, fsKey, queryindex.RebuildOptions{
		Path:  queryPath,
		Force: request.Force,
		Wait:  keywordWait,
	})
	if err != nil {
		return WorkspaceQueryIndexRebuildResponse{}, err
	}
	var embeddings *QueryEmbeddingBackfillResult
	if request.Embeddings {
		embeddingResult := s.backfillQueryEmbeddings(ctx, fsKey, queryPath)
		embeddings = &embeddingResult
	}
	status, err := s.QueryIndexStatus(ctx, workspace, WorkspaceQueryIndexStatusRequest{
		Workspace: displayWorkspace,
		Path:      queryPath,
	})
	if err != nil {
		return WorkspaceQueryIndexRebuildResponse{}, err
	}
	return WorkspaceQueryIndexRebuildResponse{
		Workspace:  displayWorkspace,
		Path:       queryPath,
		Keyword:    result,
		Embeddings: embeddings,
		Status:     status,
		Message:    queryIndexRebuildMessage(result.Enqueued, embeddings),
	}, nil
}

func (s *Service) backfillQueryEmbeddings(ctx context.Context, fsKey, queryPath string) QueryEmbeddingBackfillResult {
	provider, err := queryembedding.NewProviderFromEnv("")
	if err != nil {
		return QueryEmbeddingBackfillResult{
			Enabled: true,
			Message: queryEmbeddingUnavailableMessage(err),
		}
	}
	result := QueryEmbeddingBackfillResult{
		Enabled:   true,
		Available: true,
		Provider:  provider.Name(),
		Model:     provider.Model(),
		Dimension: provider.Dimension(),
	}
	stats, err := queryvector.Backfill(ctx, s.store.rdb, fsKey, provider, queryvector.SearchOptions{Path: queryPath})
	result.Scanned = stats.Scanned
	result.Embedded = stats.Embedded
	if err != nil {
		result.Available = false
		result.Message = queryEmbeddingUnavailableMessage(err)
		return result
	}
	result.Message = fmt.Sprintf("Indexed %d semantic embedding chunk(s).", stats.Embedded)
	return result
}

func queryIndexRebuildMessage(enqueued int, embeddings *QueryEmbeddingBackfillResult) string {
	if embeddings != nil {
		return fmt.Sprintf("Enqueued %d file(s) for keyword query indexing and indexed %d semantic embedding chunk(s).", enqueued, embeddings.Embedded)
	}
	return fmt.Sprintf("Enqueued %d file(s) for keyword query indexing.", enqueued)
}

func (s *Service) CleanQueryIndex(ctx context.Context, workspace string, request WorkspaceQueryIndexCleanRequest) (WorkspaceQueryIndexCleanResponse, error) {
	displayWorkspace := strings.TrimSpace(request.Workspace)
	if displayWorkspace == "" {
		displayWorkspace = workspace
	}
	if !request.Confirm {
		return WorkspaceQueryIndexCleanResponse{}, fmt.Errorf("query index clean requires confirm=true")
	}
	before, err := s.QueryIndexStatus(ctx, workspace, WorkspaceQueryIndexStatusRequest{
		Workspace: displayWorkspace,
		Path:      "/",
	})
	if err != nil {
		return WorkspaceQueryIndexCleanResponse{}, err
	}
	fsKey, err := s.workspaceQueryFSKey(ctx, workspace)
	if err != nil {
		return WorkspaceQueryIndexCleanResponse{}, err
	}
	if err := queryindex.ResetWorkspace(ctx, s.store.rdb, fsKey); err != nil {
		return WorkspaceQueryIndexCleanResponse{}, err
	}
	status, err := s.QueryIndexStatus(ctx, workspace, WorkspaceQueryIndexStatusRequest{
		Workspace: displayWorkspace,
		Path:      "/",
	})
	if err != nil {
		return WorkspaceQueryIndexCleanResponse{}, err
	}
	return WorkspaceQueryIndexCleanResponse{
		Workspace:     displayWorkspace,
		Cleared:       true,
		RemovedFiles:  before.Keyword.Files,
		RemovedChunks: before.Keyword.Chunks,
		Status:        status,
		Message:       fmt.Sprintf("Cleared generated query index data for %d file(s) and %d chunk(s). Workspace files were not changed.", before.Keyword.Files, before.Keyword.Chunks),
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

func semanticUnavailableResponseWithMessage(request mcptools.FileQueryRequest, message string) mcptools.FileQueryResponse {
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

func workspaceQueryWarnings(request mcptools.FileQueryRequest) []string {
	warnings := make([]string, 0)
	if request.Mode == mcptools.FileQueryModeHybrid && querysearch.HasSemanticClauses(request.Searches) {
		warnings = append(warnings, "Hybrid vector/rerank retrieval is not ready yet; semantic clauses were keyword-ranked. Use --semantic for vector-only retrieval.")
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
