package querysearch

import (
	"strings"
	"testing"

	"github.com/redis/agent-filesystem/internal/mcptools"
)

func TestKeywordSpecFromRequestUsesTypedClauses(t *testing.T) {
	spec := KeywordSpecFromRequest(mcptools.FileQueryRequest{
		Searches: []mcptools.FileQuerySearch{
			{Type: mcptools.FileQuerySearchLex, Query: `"dirty marker" workspace`},
			{Type: mcptools.FileQuerySearchVec, Query: "how does the UI detect unsaved changes?"},
		},
	})
	if !HasSemanticClauses([]mcptools.FileQuerySearch{{Type: mcptools.FileQuerySearchVec, Query: "semantic"}}) {
		t.Fatal("HasSemanticClauses(vec) = false, want true")
	}
	if len(spec.Positive) == 0 {
		t.Fatalf("positive terms = %#v, want searchable terms", spec.Positive)
	}
	if spec.SearchTypes[0] != mcptools.FileQuerySearchLex || spec.SearchTypes[1] != mcptools.FileQuerySearchVec {
		t.Fatalf("search types = %#v, want lex/vec", spec.SearchTypes)
	}
}

func TestRankKeywordTargetsRanksAndFilters(t *testing.T) {
	spec := KeywordSpecFromRequest(mcptools.FileQueryRequest{Query: `checkpoint savepoint -"skip me"`})
	results := RankKeywordTargets([]Target{
		{Path: "/docs/checkpoints.md", Content: []byte("checkpoint savepoint\ncheckpoint savepoint\n")},
		{Path: "/notes/other.md", Content: []byte("checkpoint only\n")},
		{Path: "/docs/skip.md", Content: []byte("checkpoint skip me\n")},
	}, spec, KeywordOptions{Limit: 10})

	if len(results) != 2 {
		t.Fatalf("results = %#v, want 2 non-negative matches", results)
	}
	if results[0].Path != "/docs/checkpoints.md" {
		t.Fatalf("first result = %q, want best keyword hit", results[0].Path)
	}
	if results[0].StartLine != 1 || results[0].Snippet == "" {
		t.Fatalf("first result line/snippet = %d/%q, want best line", results[0].StartLine, results[0].Snippet)
	}
}

func TestRankKeywordTargetsUsesLogicalSnippetFromJSONLRecord(t *testing.T) {
	spec := KeywordSpecFromRequest(mcptools.FileQueryRequest{Query: "module loaded"})
	results := RankKeywordTargets([]Target{
		{Path: "/history.jsonl", Content: []byte(`{"display":"connection refused"} {"display":"module is not loaded"} {"display":"daemon status"}`)},
	}, spec, KeywordOptions{Limit: 10})

	if len(results) != 1 {
		t.Fatalf("results = %#v, want one hit", results)
	}
	if strings.Contains(results[0].Snippet, "connection refused") || !strings.Contains(results[0].Snippet, "module is not loaded") {
		t.Fatalf("snippet = %q, want focused logical record", results[0].Snippet)
	}
}

func TestKeywordSpecFromRequestKeepsAllStopWordNaturalQuery(t *testing.T) {
	spec := KeywordSpecFromRequest(mcptools.FileQueryRequest{Query: "What is this"})
	if len(spec.Positive) == 0 {
		t.Fatalf("positive terms = %#v, want natural-language fallback terms", spec.Positive)
	}
}
