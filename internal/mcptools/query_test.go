package mcptools

import (
	"strings"
	"testing"
)

func TestParseFileQueryDocumentPlainQuery(t *testing.T) {
	doc, err := ParseFileQueryDocument(" how does auth attach tenant scope? ")
	if err != nil {
		t.Fatalf("ParseFileQueryDocument() returned error: %v", err)
	}
	if doc.Typed {
		t.Fatal("Typed = true, want false")
	}
	if doc.Query != "how does auth attach tenant scope?" {
		t.Fatalf("Query = %q, want trimmed plain query", doc.Query)
	}
	if len(doc.Searches) != 0 || doc.Intent != "" {
		t.Fatalf("unexpected typed fields: searches=%#v intent=%q", doc.Searches, doc.Intent)
	}
}

func TestParseFileQueryDocumentExplicitExpandQuery(t *testing.T) {
	doc, err := ParseFileQueryDocument("expand: how does auth attach tenant scope?")
	if err != nil {
		t.Fatalf("ParseFileQueryDocument() returned error: %v", err)
	}
	if doc.Typed {
		t.Fatal("Typed = true, want false")
	}
	if doc.Query != "how does auth attach tenant scope?" {
		t.Fatalf("Query = %q, want expand text", doc.Query)
	}
}

func TestParseFileQueryDocumentTypedClauses(t *testing.T) {
	raw := `intent: AFS live mount setup
lex: "mount backend"
vec: where does setup choose between NFS and FUSE?
hyde: The setup command stores a selected live mount backend.`
	doc, err := ParseFileQueryDocument(raw)
	if err != nil {
		t.Fatalf("ParseFileQueryDocument() returned error: %v", err)
	}
	if !doc.Typed {
		t.Fatal("Typed = false, want true")
	}
	if doc.Intent != "AFS live mount setup" {
		t.Fatalf("Intent = %q, want AFS live mount setup", doc.Intent)
	}
	if len(doc.Searches) != 3 {
		t.Fatalf("Searches len = %d, want 3: %#v", len(doc.Searches), doc.Searches)
	}
	if doc.Searches[0] != (FileQuerySearch{Type: FileQuerySearchLex, Query: `"mount backend"`}) {
		t.Fatalf("lex clause = %#v", doc.Searches[0])
	}
	if doc.Searches[1].Type != FileQuerySearchVec || !strings.Contains(doc.Searches[1].Query, "NFS") {
		t.Fatalf("vec clause = %#v", doc.Searches[1])
	}
	if doc.Searches[2].Type != FileQuerySearchHyde {
		t.Fatalf("hyde clause = %#v", doc.Searches[2])
	}
}

func TestParseFileQueryDocumentRejectsMultiLinePlainQuery(t *testing.T) {
	_, err := ParseFileQueryDocument("authentication\nflow")
	if err == nil {
		t.Fatal("expected multi-line plain query error, got nil")
	}
	if !strings.Contains(err.Error(), "multi-line query documents must use") {
		t.Fatalf("error = %q, want multi-line query document message", err)
	}
}

func TestParseFileQueryDocumentRejectsMixedTypedAndUntypedLines(t *testing.T) {
	_, err := ParseFileQueryDocument("lex: checkpoint\nplain continuation")
	if err == nil {
		t.Fatal("expected mixed typed/untyped error, got nil")
	}
	if !strings.Contains(err.Error(), "missing a lex:") {
		t.Fatalf("error = %q, want mixed typed/untyped message", err)
	}
}

func TestParseFileQueryDocumentRejectsExpandMixedWithTypedLines(t *testing.T) {
	_, err := ParseFileQueryDocument("expand: checkpoints\nlex: checkpoint")
	if err == nil {
		t.Fatal("expected expand mixed with typed error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot mix expand with typed lines") {
		t.Fatalf("error = %q, want expand mix message", err)
	}
}

func TestParseFileQueryDocumentRejectsUnbalancedQuotes(t *testing.T) {
	_, err := ParseFileQueryDocument(`lex: "checkpoint`)
	if err == nil {
		t.Fatal("expected unbalanced quote error, got nil")
	}
	if !strings.Contains(err.Error(), "unbalanced double quote") {
		t.Fatalf("error = %q, want unbalanced double quote", err)
	}
}

func TestParseFileQueryDocumentRejectsIntentOnly(t *testing.T) {
	_, err := ParseFileQueryDocument("intent: workspace setup")
	if err == nil {
		t.Fatal("expected intent-only error, got nil")
	}
	if !strings.Contains(err.Error(), "at least one lex, vec, or hyde") {
		t.Fatalf("error = %q, want missing search clause", err)
	}
}

func TestParseFileQueryDocumentRejectsDuplicateIntent(t *testing.T) {
	_, err := ParseFileQueryDocument("intent: one\nlex: checkpoint\nintent: two")
	if err == nil {
		t.Fatal("expected duplicate intent error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicates intent") {
		t.Fatalf("error = %q, want duplicate intent", err)
	}
}

func TestFileQueryRequestFromArgsParsesTypedQuery(t *testing.T) {
	request, err := FileQueryRequestFromArgs(map[string]any{
		"workspace": "repo",
		"path":      "/docs",
		"query":     "intent: workspace setup\nlex: checkpoint\nvec: how do I save a snapshot?",
		"limit":     float64(5),
	}, "")
	if err != nil {
		t.Fatalf("FileQueryRequestFromArgs() returned error: %v", err)
	}
	if request.Workspace != "repo" || request.Path != "/docs" || request.Limit != 5 {
		t.Fatalf("request = %+v, want workspace/path/limit", request)
	}
	if request.Mode != FileQueryModeHybrid || request.Intent != "workspace setup" {
		t.Fatalf("request mode/intent = %q/%q, want query/workspace setup", request.Mode, request.Intent)
	}
	if len(request.Searches) != 2 || request.Searches[1].Type != FileQuerySearchVec {
		t.Fatalf("searches = %#v, want lex/vec", request.Searches)
	}
}

func TestFileQueryRequestFromArgsRejectsTypedKeywordMode(t *testing.T) {
	_, err := FileQueryRequestFromArgs(map[string]any{
		"mode":  FileQueryModeKeyword,
		"query": "lex: checkpoint",
	}, "repo")
	if err == nil {
		t.Fatal("expected typed keyword mode error, got nil")
	}
	if !strings.Contains(err.Error(), "typed query documents require mode=query") {
		t.Fatalf("error = %q, want typed mode guidance", err)
	}
}
