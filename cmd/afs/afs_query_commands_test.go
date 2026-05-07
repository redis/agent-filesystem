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

func TestParseWorkspaceQueryArgsAllowsTrailingFlags(t *testing.T) {
	opts, err := parseWorkspaceQueryArgs(mcptools.FileQueryModeHybrid, []string{
		"ralph loops",
		"-n", "5",
		"--json",
	})
	if err != nil {
		t.Fatalf("parseWorkspaceQueryArgs() returned error: %v", err)
	}
	if opts.query != "ralph loops" || opts.limit != 5 || !opts.jsonOut {
		t.Fatalf("opts = %+v, want query text with trailing flags parsed", opts)
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
		Status:    mcptools.FileQueryStatusOK,
		Workspace: "repo",
		Results: []mcptools.FileQueryResult{{
			Path:      "/docs/checkpoints.md",
			StartLine: 4,
			EndLine:   6,
			Score:     0.82,
			Snippet:   "checkpoint savepoint\nrestore workspace",
			SearchTypes: []string{
				mcptools.FileQuerySearchLex,
			},
		}},
	}

	output, err := captureStdout(t, func() error {
		return writeWorkspaceQueryResponse(response, opts)
	})
	if err != nil {
		t.Fatalf("writeWorkspaceQueryResponse() returned error: %v", err)
	}
	if !strings.Contains(output, "afs://repo/docs/checkpoints.md:4-6  #") || !strings.Contains(output, "\nScore: 82%  Source: lex") {
		t.Fatalf("output = %q, want ranked result header", output)
	}
	if strings.Contains(output, "/docs/checkpoints.md:4:checkpoint") {
		t.Fatalf("output = %q, should not look like grep output", output)
	}
	if !strings.Contains(output, "@@ -4,3 @@\n4: checkpoint savepoint\n5: restore workspace") {
		t.Fatalf("output = %q, want line-numbered snippet block", output)
	}
}

func TestWriteWorkspaceQueryResponseMarkdownUsesQMDSections(t *testing.T) {
	opts := workspaceQueryOptions{lineNumbers: true, markdown: true}
	response := mcptools.FileQueryResponse{
		Status:    mcptools.FileQueryStatusOK,
		Workspace: "repo",
		Results: []mcptools.FileQueryResult{{
			Path:      "/docs/checkpoints.md",
			StartLine: 7,
			Score:     0.5,
			Snippet:   "checkpoint savepoint",
		}},
	}

	output, err := captureStdout(t, func() error {
		return writeWorkspaceQueryResponse(response, opts)
	})
	if err != nil {
		t.Fatalf("writeWorkspaceQueryResponse() returned error: %v", err)
	}
	for _, want := range []string{
		"---\n# /docs/checkpoints.md:7",
		"**score:** 50%",
		"**file:** `afs://repo/docs/checkpoints.md:7`",
		"@@ -7,1 @@\n7: checkpoint savepoint",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, missing %q", output, want)
		}
	}
}

func TestWriteWorkspaceQueryResponseCoalescesSameLineChunks(t *testing.T) {
	opts := workspaceQueryOptions{lineNumbers: true}
	response := mcptools.FileQueryResponse{
		Status: mcptools.FileQueryStatusOK,
		Results: []mcptools.FileQueryResult{
			{
				Path:        "/history.jsonl",
				StartLine:   146,
				EndLine:     146,
				Score:       0.28,
				Snippet:     "module not loaded",
				SearchTypes: []string{mcptools.FileQuerySearchLex},
			},
			{
				Path:        "/history.jsonl",
				StartLine:   146,
				EndLine:     146,
				Score:       0.14,
				Snippet:     "Load it with redis-cli",
				SearchTypes: []string{mcptools.FileQuerySearchHyde},
			},
		},
	}

	output, err := captureStdout(t, func() error {
		return writeWorkspaceQueryResponse(response, opts)
	})
	if err != nil {
		t.Fatalf("writeWorkspaceQueryResponse() returned error: %v", err)
	}
	if strings.Count(output, "afs://workspace/history.jsonl:146") != 1 {
		t.Fatalf("output = %q, want duplicate same-line chunks coalesced", output)
	}
	if !strings.Contains(output, "Source: lex+hyde") {
		t.Fatalf("output = %q, want merged sources", output)
	}
	if !strings.Contains(output, "module not loaded") || !strings.Contains(output, "Load it with redis-cli") {
		t.Fatalf("output = %q, want merged snippets", output)
	}
	if !strings.Contains(output, "146: module not loaded\n     ...") {
		t.Fatalf("output = %q, want same-line continuation without fake line numbers", output)
	}
}

func TestWriteWorkspaceQueryResponseMatchesQMDEmptyFormats(t *testing.T) {
	response := mcptools.FileQueryResponse{Status: mcptools.FileQueryStatusOK}

	plain, err := captureStdout(t, func() error {
		return writeWorkspaceQueryResponse(response, workspaceQueryOptions{})
	})
	if err != nil {
		t.Fatalf("writeWorkspaceQueryResponse(plain) returned error: %v", err)
	}
	if plain != "No results found.\n" {
		t.Fatalf("plain empty output = %q, want QMD-style no-results message", plain)
	}

	for name, opts := range map[string]workspaceQueryOptions{
		"md":    {markdown: true},
		"files": {filesOnly: true},
		"paths": {pathsOnly: true},
	} {
		output, err := captureStdout(t, func() error {
			return writeWorkspaceQueryResponse(response, opts)
		})
		if err != nil {
			t.Fatalf("writeWorkspaceQueryResponse(%s) returned error: %v", name, err)
		}
		if output != "" {
			t.Fatalf("%s empty output = %q, want empty output", name, output)
		}
	}

	csvOutput, err := captureStdout(t, func() error {
		return writeWorkspaceQueryResponse(response, workspaceQueryOptions{csvOut: true})
	})
	if err != nil {
		t.Fatalf("writeWorkspaceQueryResponse(csv) returned error: %v", err)
	}
	if csvOutput != "docid,score,file,title,context,line,snippet\n" {
		t.Fatalf("csv empty output = %q, want header only", csvOutput)
	}

	xmlOutput, err := captureStdout(t, func() error {
		return writeWorkspaceQueryResponse(response, workspaceQueryOptions{xmlOut: true})
	})
	if err != nil {
		t.Fatalf("writeWorkspaceQueryResponse(xml) returned error: %v", err)
	}
	if xmlOutput != "<results></results>\n" {
		t.Fatalf("xml empty output = %q, want empty results element", xmlOutput)
	}
}

func TestWriteWorkspaceQueryResponseStructuredFormats(t *testing.T) {
	response := mcptools.FileQueryResponse{
		Status:    mcptools.FileQueryStatusOK,
		Workspace: "repo",
		Results: []mcptools.FileQueryResult{{
			Path:        "/docs/checkpoints.md",
			StartLine:   3,
			Score:       0.75,
			Snippet:     "checkpoint savepoint",
			SearchTypes: []string{mcptools.FileQuerySearchLex},
		}},
	}

	filesOutput, err := captureStdout(t, func() error {
		return writeWorkspaceQueryResponse(response, workspaceQueryOptions{filesOnly: true})
	})
	if err != nil {
		t.Fatalf("writeWorkspaceQueryResponse(files) returned error: %v", err)
	}
	if !strings.Contains(filesOutput, "#") || !strings.Contains(filesOutput, ",0.75,afs://repo/docs/checkpoints.md:3") {
		t.Fatalf("files output = %q, want QMD-style id,score,uri", filesOutput)
	}

	pathsOutput, err := captureStdout(t, func() error {
		return writeWorkspaceQueryResponse(response, workspaceQueryOptions{pathsOnly: true})
	})
	if err != nil {
		t.Fatalf("writeWorkspaceQueryResponse(paths) returned error: %v", err)
	}
	if pathsOutput != "/docs/checkpoints.md\n" {
		t.Fatalf("paths output = %q, want path-only compatibility output", pathsOutput)
	}

	csvOutput, err := captureStdout(t, func() error {
		return writeWorkspaceQueryResponse(response, workspaceQueryOptions{csvOut: true})
	})
	if err != nil {
		t.Fatalf("writeWorkspaceQueryResponse(csv) returned error: %v", err)
	}
	if !strings.Contains(csvOutput, "docid,score,file,title,context,line,snippet\n") ||
		!strings.Contains(csvOutput, "0.75,afs://repo/docs/checkpoints.md:3,checkpoints.md,lex,3,checkpoint savepoint") {
		t.Fatalf("csv output = %q, want QMD-style csv row", csvOutput)
	}

	xmlOutput, err := captureStdout(t, func() error {
		return writeWorkspaceQueryResponse(response, workspaceQueryOptions{xmlOut: true})
	})
	if err != nil {
		t.Fatalf("writeWorkspaceQueryResponse(xml) returned error: %v", err)
	}
	if !strings.Contains(xmlOutput, `<results>`) ||
		!strings.Contains(xmlOutput, `name="afs://repo/docs/checkpoints.md:3"`) ||
		!strings.Contains(xmlOutput, "checkpoint savepoint") {
		t.Fatalf("xml output = %q, want QMD-style xml result", xmlOutput)
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
		"--files",
		"--paths",
		"--csv",
		"--xml",
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

func TestCmdQueryContractCoversHybridFallbacksAndIndexDisambiguation(t *testing.T) {
	_, store, closeStore := setupAFSGrepTest(t)
	defer closeStore()

	writeLiveAFSFile(t, store, "repo", "/docs/index.md", "Index files explain how query ranking works.\nUse checkpoint terms for workspace recovery.\n")
	writeLiveAFSFile(t, store, "repo", "/docs/checkpoints.md", "Checkpoint savepoint recovery lives in this guide.\nRestore snapshots from the checkpoint list.\n")
	writeLiveAFSFile(t, store, "repo", "/archive/checkpoints.md", "Checkpoint content outside the scoped docs directory.\n")
	writeLiveAFSFile(t, store, "repo", "/notes/auth.md", "Auth attaches tenant scope to a workspace.\n")

	t.Run("request defaults and explicit switches", func(t *testing.T) {
		defaultOpts, err := parseWorkspaceQueryArgs(mcptools.FileQueryModeHybrid, []string{"checkpoint recovery"})
		if err != nil {
			t.Fatalf("parseWorkspaceQueryArgs(default) returned error: %v", err)
		}
		defaultRequest := workspaceQueryRequest(workspaceSelection{ID: "ws_repo", Name: "repo"}, defaultOpts)
		if defaultRequest.Mode != mcptools.FileQueryModeHybrid || defaultRequest.Rerank != "auto" || defaultRequest.Path != "/" || defaultRequest.Limit != 10 {
			t.Fatalf("default request = %+v, want hybrid auto rerank at root with default limit", defaultRequest)
		}

		opts, err := parseWorkspaceQueryArgs(mcptools.FileQueryModeHybrid, []string{
			"--path", "docs",
			"--limit", "3",
			"--candidate-limit", "11",
			"--min-score", "0.25",
			"--no-rerank",
			"--explain",
			"--full",
			"lex: checkpoint\nhyde: snapshot recovery\nintent: docs only",
		})
		if err != nil {
			t.Fatalf("parseWorkspaceQueryArgs(explicit) returned error: %v", err)
		}
		request := workspaceQueryRequest(workspaceSelection{ID: "ws_repo", Name: "repo"}, opts)
		if request.Workspace != "repo" || request.Path != "/docs" || request.Mode != mcptools.FileQueryModeHybrid {
			t.Fatalf("request identity = %+v, want repo hybrid query scoped to /docs", request)
		}
		if request.Limit != 3 || request.CandidateLimit != 11 || request.MinScore != 0.25 || request.Rerank != "none" || !request.Full || !request.Explain {
			t.Fatalf("request flags = %+v, want explicit query controls preserved", request)
		}
		if request.Intent != "docs only" || len(request.Searches) != 2 || request.Searches[0].Type != mcptools.FileQuerySearchLex || request.Searches[1].Type != mcptools.FileQuerySearchHyde {
			t.Fatalf("request typed document = %+v, want lex/hyde plus intent", request)
		}
	})

	t.Run("hybrid JSON fallback is path scoped and explains backend", func(t *testing.T) {
		output, err := captureStdout(t, func() error {
			return cmdQuery([]string{
				"query",
				"--json",
				"--path", "docs",
				"--limit", "2",
				"--explain",
				"lex: checkpoint\nhyde: snapshot recovery\nintent: docs only",
			})
		})
		if err != nil {
			t.Fatalf("cmdQuery(typed JSON) returned error: %v", err)
		}

		var response mcptools.FileQueryResponse
		if err := json.Unmarshal([]byte(output), &response); err != nil {
			t.Fatalf("Unmarshal(response) returned error: %v\n%s", err, output)
		}
		if response.Status != mcptools.FileQueryStatusOK || response.Workspace != "repo" || response.Path != "/docs" {
			t.Fatalf("response envelope = %+v, want ok repo response scoped to /docs", response)
		}
		if response.Intent != "docs only" || len(response.Searches) != 2 {
			t.Fatalf("response typed document = %+v, want typed searches and intent echoed", response)
		}
		if len(response.Results) == 0 || len(response.Results) > 2 {
			t.Fatalf("results = %#v, want one or two scoped docs results", response.Results)
		}
		for _, result := range response.Results {
			if !strings.HasPrefix(result.Path, "/docs/") {
				t.Fatalf("result path = %q, want path scoped to /docs", result.Path)
			}
			if len(result.SearchTypes) != 2 || result.SearchTypes[0] != mcptools.FileQuerySearchLex || result.SearchTypes[1] != mcptools.FileQuerySearchHyde {
				t.Fatalf("result search types = %#v, want lex/hyde fallback evidence", result.SearchTypes)
			}
		}
		if len(response.Warnings) != 1 || !strings.Contains(response.Warnings[0], "Embeddings are disabled") {
			t.Fatalf("warnings = %#v, want semantic fallback warning", response.Warnings)
		}
		if len(response.Explain) == 0 {
			t.Fatalf("explain = %#v, want backend explanation", response.Explain)
		}
	})

	t.Run("plain query starting with index is not treated as index subcommand", func(t *testing.T) {
		output, err := captureStdout(t, func() error {
			return cmdQuery([]string{"query", "index", "files"})
		})
		if err != nil {
			t.Fatalf("cmdQuery(index files) returned error: %v", err)
		}
		if !strings.Contains(output, "afs://repo/docs/index.md:1") {
			t.Fatalf("output = %q, want ranked result for natural query starting with index", output)
		}
	})

	t.Run("semantic JSON reports unavailable without failing command", func(t *testing.T) {
		output, err := captureStdout(t, func() error {
			return cmdQuery([]string{"query", "--semantic", "--json", "workspace recovery"})
		})
		if err != nil {
			t.Fatalf("cmdQuery(semantic JSON) returned error: %v", err)
		}

		var response mcptools.FileQueryResponse
		if err := json.Unmarshal([]byte(output), &response); err != nil {
			t.Fatalf("Unmarshal(semantic response) returned error: %v\n%s", err, output)
		}
		if response.Status != mcptools.FileQueryStatusUnavailable || len(response.Results) != 0 {
			t.Fatalf("semantic response = %+v, want unavailable with empty results", response)
		}
		if len(response.Warnings) != 1 || !strings.Contains(response.Warnings[0], "semantic query is disabled") {
			t.Fatalf("semantic warnings = %#v, want disabled guidance", response.Warnings)
		}
	})

	t.Run("paths output is unique and omits snippets", func(t *testing.T) {
		output, err := captureStdout(t, func() error {
			return cmdQuery([]string{"query", "--paths", "--path", "docs", "checkpoint"})
		})
		if err != nil {
			t.Fatalf("cmdQuery(paths) returned error: %v", err)
		}
		lines := strings.Fields(strings.TrimSpace(output))
		if len(lines) == 0 {
			t.Fatalf("output = %q, want at least one file path", output)
		}
		seen := make(map[string]struct{}, len(lines))
		for _, line := range lines {
			if !strings.HasPrefix(line, "/docs/") {
				t.Fatalf("files output line = %q, want scoped file path only", line)
			}
			if _, ok := seen[line]; ok {
				t.Fatalf("files output = %q, path %q appeared more than once", output, line)
			}
			seen[line] = struct{}{}
		}
		if strings.Contains(output, "score") || strings.Contains(output, "Checkpoint savepoint") {
			t.Fatalf("paths output = %q, want paths without ranked snippets", output)
		}
	})
}
