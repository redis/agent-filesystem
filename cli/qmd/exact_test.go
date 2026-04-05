package qmd

import "testing"

func TestExactPhraseText(t *testing.T) {
	t.Run("pure quoted phrase", func(t *testing.T) {
		got, ok := exactPhraseText(`"memory checkpoint persisted"`)
		if !ok {
			t.Fatal("expected exact phrase to be detected")
		}
		if got != "memory checkpoint persisted" {
			t.Fatalf("phrase = %q", got)
		}
	})

	t.Run("mixed query is not exact phrase", func(t *testing.T) {
		if _, ok := exactPhraseText(`"disk full" AND retry`); ok {
			t.Fatal("unexpected exact phrase detection for mixed query")
		}
	})
}

func TestExactPhraseCandidateQuery(t *testing.T) {
	p := ParsedQuery{
		TextQuery:  `"auth token refresh failed"`,
		PathPrefix: "/bench/sessions/agent",
		TypeFilter: "file",
	}

	got := exactPhraseCandidateQuery(p, "auth token refresh failed")
	want := `@type:{file} @path_ancestors:{\/bench\/sessions\/agent}`
	if got != want {
		t.Fatalf("candidate query = %q, want %q", got, want)
	}
}

func TestFilterExactPhraseHits(t *testing.T) {
	hits := []SearchHit{
		{Path: "/a.md", Content: "memory checkpoint persisted for follow up"},
		{Path: "/b.md", Content: "memory checkpoint recorded for review"},
	}

	filtered := filterExactPhraseHits(hits, "memory checkpoint persisted")
	if len(filtered) != 1 {
		t.Fatalf("filtered hits = %d", len(filtered))
	}
	if filtered[0].Path != "/a.md" {
		t.Fatalf("unexpected hit path %q", filtered[0].Path)
	}
}

func TestFilterExactPhraseHitsKeepsEveryExactMatch(t *testing.T) {
	hits := []SearchHit{
		{Path: "/bench/sessions/agent/a.md", Content: "memory checkpoint persisted for follow up"},
		{Path: "/bench/chat/support/b.md", Content: "memory checkpoint persisted for follow up"},
	}

	filtered := filterExactPhraseHits(hits, "memory checkpoint persisted")
	if len(filtered) != 2 {
		t.Fatalf("filtered hits = %d", len(filtered))
	}
}

func TestSliceHits(t *testing.T) {
	hits := []SearchHit{
		{Path: "/a.md"},
		{Path: "/b.md"},
		{Path: "/c.md"},
	}

	sliced := sliceHits(hits, QueryOptions{Limit: 1, Offset: 1})
	if len(sliced) != 1 {
		t.Fatalf("slice length = %d", len(sliced))
	}
	if sliced[0].Path != "/b.md" {
		t.Fatalf("unexpected sliced hit %q", sliced[0].Path)
	}
}
