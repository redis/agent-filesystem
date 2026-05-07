package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/internal/mcptools"
)

func TestParseWorkspaceQueryArgsTypedDocument(t *testing.T) {
	raw := "lex: checkpoint\nvec: how do I save a snapshot?"
	opts, err := parseWorkspaceQueryArgs(mcptools.FileQueryModeHybrid, []string{
		"--path", "/docs",
		"--json",
		"--limit", "5",
		"--candidate-limit", "50",
		"--intent", "workspace snapshots",
		"--chunk-strategy", "auto",
		raw,
	})
	if err != nil {
		t.Fatalf("parseWorkspaceQueryArgs() returned error: %v", err)
	}
	if opts.path != "/docs" || !opts.jsonOut || opts.limit != 5 || opts.candidateLimit != 50 {
		t.Fatalf("opts = %+v, want parsed flags", opts)
	}
	if opts.document.Intent != "workspace snapshots" {
		t.Fatalf("intent = %q, want workspace snapshots", opts.document.Intent)
	}
	if len(opts.document.Searches) != 2 {
		t.Fatalf("searches = %#v, want 2", opts.document.Searches)
	}
	if opts.document.Searches[0].Type != mcptools.FileQuerySearchLex || opts.document.Searches[1].Type != mcptools.FileQuerySearchVec {
		t.Fatalf("search types = %#v, want lex/vec", opts.document.Searches)
	}
}

func TestParseWorkspaceQueryArgsRejectsIntentFlagWithIntentClause(t *testing.T) {
	_, err := parseWorkspaceQueryArgs(mcptools.FileQueryModeHybrid, []string{
		"--intent", "outer",
		"intent: inner\nlex: checkpoint",
	})
	if err == nil {
		t.Fatal("expected duplicate intent error, got nil")
	}
	if !strings.Contains(err.Error(), "--intent cannot be combined") {
		t.Fatalf("error = %q, want intent conflict", err)
	}
}

func TestParseWorkspaceQueryArgsAllowsVectorClausesForQuery(t *testing.T) {
	opts, err := parseWorkspaceQueryArgs(mcptools.FileQueryModeHybrid, []string{
		"lex: checkpoint\nvec: how do I save a snapshot?",
	})
	if err != nil {
		t.Fatalf("parseWorkspaceQueryArgs() returned error: %v", err)
	}
	if len(opts.document.Searches) != 2 || opts.document.Searches[1].Type != mcptools.FileQuerySearchVec {
		t.Fatalf("searches = %+v, want parsed vector clause", opts.document.Searches)
	}
}

func TestParseWorkspaceQueryArgsKeywordSemanticModes(t *testing.T) {
	keyword, err := parseWorkspaceQueryArgs(mcptools.FileQueryModeHybrid, []string{
		"--keyword",
		"checkpoint savepoint",
	})
	if err != nil {
		t.Fatalf("parseWorkspaceQueryArgs(--keyword) returned error: %v", err)
	}
	if keyword.mode != mcptools.FileQueryModeKeyword || keyword.document.Query != "checkpoint savepoint" {
		t.Fatalf("keyword opts = %+v, want keyword mode", keyword)
	}

	semantic, err := parseWorkspaceQueryArgs(mcptools.FileQueryModeHybrid, []string{
		"--semantic",
		"how do I save a snapshot?",
	})
	if err != nil {
		t.Fatalf("parseWorkspaceQueryArgs(--semantic) returned error: %v", err)
	}
	if semantic.mode != mcptools.FileQueryModeSemantic || semantic.document.Query != "how do I save a snapshot?" {
		t.Fatalf("semantic opts = %+v, want semantic mode", semantic)
	}
}

func TestParseWorkspaceQueryArgsRejectsModeFlagsWithTypedDocuments(t *testing.T) {
	_, err := parseWorkspaceQueryArgs(mcptools.FileQueryModeHybrid, []string{
		"--semantic",
		"vec: how do I save a snapshot?",
	})
	if err == nil {
		t.Fatal("expected semantic typed document error, got nil")
	}
	if !strings.Contains(err.Error(), "plain search text only") {
		t.Fatalf("error = %q, want plain text guidance", err)
	}
}

func TestParseWorkspaceQueryArgsRejectsKeywordAndSemanticTogether(t *testing.T) {
	_, err := parseWorkspaceQueryArgs(mcptools.FileQueryModeHybrid, []string{
		"--keyword",
		"--semantic",
		"checkpoint",
	})
	if err == nil {
		t.Fatal("expected mutually exclusive flags error, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error = %q, want mutually exclusive guidance", err)
	}
}

func TestCmdQuerySemanticReportsEmbeddingsDisabled(t *testing.T) {
	_, _, closeStore := setupAFSGrepTest(t)
	defer closeStore()

	err := cmdQuery([]string{"query", "--semantic", "semantic mount setup"})
	if err == nil {
		t.Fatal("cmdQuery(--semantic) returned nil, want unavailable error")
	}
	if !strings.Contains(err.Error(), "semantic query is disabled") {
		t.Fatalf("error = %q, want disabled message", err)
	}
	if !strings.Contains(err.Error(), "query.embeddings.enabled true") {
		t.Fatalf("error = %q, want enable command", err)
	}
}

func TestCmdQueryJSONReturnsKeywordResults(t *testing.T) {
	_, store, closeStore := setupAFSGrepTest(t)
	defer closeStore()

	writeLiveAFSFile(t, store, "repo", "/docs/checkpoints.md", "Checkpoints save workspace snapshots.\nUse savepoints to recover work.\n")
	writeLiveAFSFile(t, store, "repo", "/notes/auth.md", "Auth attaches tenant scope to a workspace.\n")

	output, err := captureStdout(t, func() error {
		return cmdQuery([]string{"query", "--json", "how do checkpoints work?"})
	})
	if err != nil {
		t.Fatalf("cmdQuery() returned error: %v", err)
	}

	var response mcptools.FileQueryResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("Unmarshal(response) returned error: %v\n%s", err, output)
	}
	if response.Status != mcptools.FileQueryStatusOK {
		t.Fatalf("status = %q, want ok", response.Status)
	}
	if response.Workspace != "repo" || response.Query != "how do checkpoints work?" {
		t.Fatalf("response = %+v, want repo query", response)
	}
	if len(response.Results) == 0 {
		t.Fatalf("response results = %#v, want keyword result", response.Results)
	}
	if response.Results[0].Path != "/docs/checkpoints.md" {
		t.Fatalf("first result path = %q, want checkpoints doc", response.Results[0].Path)
	}
	if len(response.Warnings) != 0 {
		t.Fatalf("response warnings = %#v, want no warning for plain keyword fallback", response.Warnings)
	}
}

func TestWriteWorkspaceQueryResponseUsesRankedBlockOutput(t *testing.T) {
	opts := workspaceQueryOptions{lineNumbers: true}
	response := mcptools.FileQueryResponse{
		Status: mcptools.FileQueryStatusOK,
		Results: []mcptools.FileQueryResult{{
			Path:      "/docs/checkpoints.md",
			StartLine: 4,
			EndLine:   6,
			Score:     1.25,
			Snippet:   "checkpoint savepoint\nrestore workspace",
		}},
	}

	output, err := captureStdout(t, func() error {
		return writeWorkspaceQueryResponse(response, opts)
	})
	if err != nil {
		t.Fatalf("writeWorkspaceQueryResponse() returned error: %v", err)
	}
	if !strings.Contains(output, "#1 /docs/checkpoints.md:4-6  score 1.25") {
		t.Fatalf("output = %q, want ranked result header", output)
	}
	if strings.Contains(output, "/docs/checkpoints.md:4:checkpoint") {
		t.Fatalf("output = %q, should not look like grep output", output)
	}
	if !strings.Contains(output, "  checkpoint savepoint\n  restore workspace") {
		t.Fatalf("output = %q, want indented snippet block", output)
	}
}

func TestCmdQuerySemanticClausesMentionEmbeddingsWithoutMakingQueryVectorOnly(t *testing.T) {
	_, store, closeStore := setupAFSGrepTest(t)
	defer closeStore()

	writeLiveAFSFile(t, store, "repo", "/docs/checkpoints.md", "checkpoint save snapshot\ncheckpoint restore savepoint\n")

	output, err := captureStdout(t, func() error {
		return cmdQuery([]string{"query", "--json", "lex: checkpoint\nvec: how do I save a snapshot?"})
	})
	if err != nil {
		t.Fatalf("cmdQuery() returned error: %v", err)
	}
	var response mcptools.FileQueryResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("Unmarshal(response) returned error: %v\n%s", err, output)
	}
	if response.Status != mcptools.FileQueryStatusOK {
		t.Fatalf("status = %q, want ok", response.Status)
	}
	if len(response.Results) == 0 || response.Results[0].Path != "/docs/checkpoints.md" {
		t.Fatalf("results = %#v, want checkpoints result", response.Results)
	}
	if len(response.Warnings) != 1 ||
		!strings.Contains(response.Warnings[0], "Embeddings are disabled") ||
		!strings.Contains(response.Warnings[0], "vec:/hyde: clauses were used as keyword text") {
		t.Fatalf("warnings = %#v, want semantic-clause fallback warning", response.Warnings)
	}
}

func TestCmdFSQuerySemanticRoutesExplicitWorkspace(t *testing.T) {
	_, _, closeStore := setupAFSGrepTest(t)
	defer closeStore()

	err := cmdFS([]string{"fs", "repo", "query", "--semantic", "semantic mount setup"})
	if err == nil {
		t.Fatal("cmdFS(query --semantic) returned nil, want unavailable error")
	}
	if !strings.Contains(err.Error(), `workspace "repo"`) {
		t.Fatalf("error = %q, want explicit workspace", err)
	}
}

func TestCmdQueryIndexStatusReportsWorkspaceConfig(t *testing.T) {
	_, _, closeStore := setupAFSGrepTest(t)
	defer closeStore()

	output, err := captureStdout(t, func() error {
		return cmdQuery([]string{"query", "index", "status", "--json"})
	})
	if err != nil {
		t.Fatalf("cmdQuery(index status) returned error: %v", err)
	}

	var status controlplane.WorkspaceQueryIndexStatus
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		t.Fatalf("Unmarshal(status) returned error: %v\n%s", err, output)
	}
	if status.Workspace != "repo" || status.Keyword.Files != 0 || status.EmbeddingsEnabled {
		t.Fatalf("status = %+v, want empty repo keyword status with embeddings disabled", status)
	}
}

func TestWorkspaceQueryConfigFallsBackWhenConfigRouteIsMissing(t *testing.T) {
	cfg, err := workspaceQueryConfig(context.Background(), stubAFSControlPlane{
		workspaceConfigErr: os.ErrNotExist,
	}, workspaceSelection{ID: "ws_repo", Name: "repo"})
	if err != nil {
		t.Fatalf("workspaceQueryConfig() returned error: %v", err)
	}
	if cfg.Versioning.Mode != "off" {
		t.Fatalf("versioning mode = %q, want off", cfg.Versioning.Mode)
	}
	if cfg.Query.Embeddings.Enabled {
		t.Fatal("embeddings enabled = true, want false default")
	}
}

func TestWorkspaceQueryUsageDocumentsModes(t *testing.T) {
	queryUsage := workspaceQueryUsageText("afs", mcptools.FileQueryModeHybrid)
	for _, want := range []string{
		"QMD-style hybrid + rerank workspace query",
		"Use --keyword for keyword-ranked",
		"--semantic",
		"falls back to keyword ranked results",
		"lex: lexical terms",
		"vec: semantic terms",
		"hyde: hypothetical answer text",
	} {
		if !strings.Contains(queryUsage, want) {
			t.Fatalf("query usage missing %q:\n%s", want, queryUsage)
		}
	}
	for _, notWant := range []string{"vsearch", "Ranked lexical workspace query"} {
		if strings.Contains(queryUsage, notWant) {
			t.Fatalf("query usage should not mention %q:\n%s", notWant, queryUsage)
		}
	}
}
