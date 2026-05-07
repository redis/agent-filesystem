package mcptools

import (
	"fmt"
	"strings"
)

const (
	FileQueryModeHybrid   = "query"
	FileQueryModeKeyword  = "keyword"
	FileQueryModeSemantic = "semantic"

	FileQuerySearchLex    = "lex"
	FileQuerySearchVec    = "vec"
	FileQuerySearchHyde   = "hyde"
	FileQuerySearchIntent = "intent"

	FileQueryStatusOK          = "ok"
	FileQueryStatusUnavailable = "unavailable"
)

// FileQueryRequest is the shared MCP/CLI contract for ranked workspace
// retrieval. The first implementation slice defines the shape before the
// vector backend exists.
type FileQueryRequest struct {
	Workspace      string            `json:"workspace,omitempty"`
	Path           string            `json:"path,omitempty"`
	Mode           string            `json:"mode,omitempty"`
	Query          string            `json:"query,omitempty"`
	Searches       []FileQuerySearch `json:"searches,omitempty"`
	Intent         string            `json:"intent,omitempty"`
	Limit          int               `json:"limit,omitempty"`
	All            bool              `json:"all,omitempty"`
	MinScore       float64           `json:"min_score,omitempty"`
	Full           bool              `json:"full,omitempty"`
	CandidateLimit int               `json:"candidate_limit,omitempty"`
	Rerank         string            `json:"rerank,omitempty"`
	Explain        bool              `json:"explain,omitempty"`
	ChunkStrategy  string            `json:"chunk_strategy,omitempty"`
}

// FileQuerySearch is one typed query clause from a QMD-style query document.
type FileQuerySearch struct {
	Type  string `json:"type"`
	Query string `json:"query"`
}

// FileQueryInput is the MCP argument shape for file_query.
type FileQueryInput struct {
	Workspace      string            `json:"workspace,omitempty"`
	Path           string            `json:"path,omitempty"`
	Mode           string            `json:"mode,omitempty"`
	Query          string            `json:"query,omitempty"`
	Searches       []FileQuerySearch `json:"searches,omitempty"`
	Intent         string            `json:"intent,omitempty"`
	Limit          int               `json:"limit,omitempty"`
	All            bool              `json:"all,omitempty"`
	MinScore       float64           `json:"min_score,omitempty"`
	Full           bool              `json:"full,omitempty"`
	CandidateLimit int               `json:"candidate_limit,omitempty"`
	Rerank         string            `json:"rerank,omitempty"`
	Explain        bool              `json:"explain,omitempty"`
	ChunkStrategy  string            `json:"chunk_strategy,omitempty"`
}

// FileQueryResponse is the stable response envelope for file_query.
type FileQueryResponse struct {
	Status    string             `json:"status"`
	Workspace string             `json:"workspace,omitempty"`
	Path      string             `json:"path,omitempty"`
	Query     string             `json:"query,omitempty"`
	Searches  []FileQuerySearch  `json:"searches,omitempty"`
	Intent    string             `json:"intent,omitempty"`
	Results   []FileQueryResult  `json:"results"`
	Warnings  []string           `json:"warnings,omitempty"`
	Explain   []FileQueryExplain `json:"explain,omitempty"`
}

// FileQueryResult is one ranked chunk/file hit.
type FileQueryResult struct {
	Path        string         `json:"path"`
	ChunkID     string         `json:"chunk_id,omitempty"`
	StartLine   int            `json:"start_line,omitempty"`
	EndLine     int            `json:"end_line,omitempty"`
	Score       float64        `json:"score,omitempty"`
	Snippet     string         `json:"snippet,omitempty"`
	SearchTypes []string       `json:"search_types,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// FileQueryExplain records retrieval/ranking evidence for explain output.
type FileQueryExplain struct {
	Stage   string         `json:"stage"`
	Message string         `json:"message,omitempty"`
	Values  map[string]any `json:"values,omitempty"`
}

// FileQueryDocument is the parsed form of a user-supplied query string.
type FileQueryDocument struct {
	Original string            `json:"original"`
	Query    string            `json:"query,omitempty"`
	Searches []FileQuerySearch `json:"searches,omitempty"`
	Intent   string            `json:"intent,omitempty"`
	Typed    bool              `json:"typed"`
}

// FileQueryRequestFromArgs parses MCP arguments into the same request contract
// used by the CLI query command.
func FileQueryRequestFromArgs(args map[string]any, defaultWorkspace string) (FileQueryRequest, error) {
	var input FileQueryInput
	if err := DecodeArgs(args, &input); err != nil {
		return FileQueryRequest{}, err
	}
	request := FileQueryRequest{
		Workspace:      strings.TrimSpace(input.Workspace),
		Path:           strings.TrimSpace(input.Path),
		Mode:           strings.TrimSpace(strings.ToLower(input.Mode)),
		Intent:         strings.TrimSpace(input.Intent),
		Limit:          input.Limit,
		All:            input.All,
		MinScore:       input.MinScore,
		Full:           input.Full,
		CandidateLimit: input.CandidateLimit,
		Rerank:         strings.TrimSpace(strings.ToLower(input.Rerank)),
		Explain:        input.Explain,
		ChunkStrategy:  strings.TrimSpace(strings.ToLower(input.ChunkStrategy)),
	}
	if request.Workspace == "" {
		request.Workspace = strings.TrimSpace(defaultWorkspace)
	}
	if request.Path == "" {
		request.Path = "/"
	}
	if request.Mode == "" {
		request.Mode = FileQueryModeHybrid
	}
	switch request.Mode {
	case FileQueryModeHybrid, FileQueryModeKeyword, FileQueryModeSemantic:
	default:
		return FileQueryRequest{}, fmt.Errorf("mode must be one of query, keyword, or semantic")
	}
	if request.Limit == 0 {
		request.Limit = 10
	}
	if request.Limit < 0 {
		return FileQueryRequest{}, fmt.Errorf("limit must be non-negative")
	}
	if request.CandidateLimit < 0 {
		return FileQueryRequest{}, fmt.Errorf("candidate_limit must be non-negative")
	}
	if request.MinScore < 0 {
		return FileQueryRequest{}, fmt.Errorf("min_score must be non-negative")
	}
	switch request.Rerank {
	case "":
		request.Rerank = "auto"
	case "auto", "none":
	default:
		return FileQueryRequest{}, fmt.Errorf("rerank must be auto or none")
	}
	switch request.ChunkStrategy {
	case "", "auto", "regex":
	default:
		return FileQueryRequest{}, fmt.Errorf("chunk_strategy must be auto or regex")
	}
	if strings.TrimSpace(input.Query) != "" && len(input.Searches) > 0 {
		return FileQueryRequest{}, fmt.Errorf("query cannot be combined with searches")
	}
	if len(input.Searches) > 0 {
		if request.Mode != FileQueryModeHybrid {
			return FileQueryRequest{}, fmt.Errorf("searches require mode=query; use plain query text for keyword or semantic mode")
		}
		for i, search := range input.Searches {
			normalized, err := normalizeFileQuerySearch(search)
			if err != nil {
				return FileQueryRequest{}, fmt.Errorf("searches[%d]: %w", i, err)
			}
			request.Searches = append(request.Searches, normalized)
		}
		if len(request.Searches) == 0 {
			return FileQueryRequest{}, fmt.Errorf("searches must include at least one lex, vec, or hyde clause")
		}
		return request, nil
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return FileQueryRequest{}, fmt.Errorf("query is required")
	}
	doc, err := ParseFileQueryDocument(query)
	if err != nil {
		return FileQueryRequest{}, err
	}
	if request.Intent != "" && doc.Intent != "" {
		return FileQueryRequest{}, fmt.Errorf("intent cannot be combined with an intent: typed query clause")
	}
	if request.Mode != FileQueryModeHybrid && doc.Typed {
		return FileQueryRequest{}, fmt.Errorf("typed query documents require mode=query; use plain query text for keyword or semantic mode")
	}
	if doc.Typed {
		request.Searches = append([]FileQuerySearch(nil), doc.Searches...)
		if request.Intent == "" {
			request.Intent = doc.Intent
		}
	} else {
		request.Query = doc.Query
	}
	return request, nil
}

func normalizeFileQuerySearch(search FileQuerySearch) (FileQuerySearch, error) {
	typ := strings.TrimSpace(strings.ToLower(search.Type))
	query := strings.TrimSpace(search.Query)
	if query == "" {
		return FileQuerySearch{}, fmt.Errorf("query must not be empty")
	}
	if err := validateFileQueryClauseText(query); err != nil {
		return FileQuerySearch{}, err
	}
	switch typ {
	case FileQuerySearchLex, FileQuerySearchVec, FileQuerySearchHyde:
		return FileQuerySearch{Type: typ, Query: query}, nil
	default:
		return FileQuerySearch{}, fmt.Errorf("type must be lex, vec, or hyde")
	}
}

// ParseFileQueryDocument parses a QMD-style query. A query is either a single
// expand query, optionally prefixed with expand:, or a typed document using one
// typed clause per non-empty line:
//
//	lex: exact terms
//	vec: semantic terms
//	hyde: hypothetical answer text
//	intent: extra search intent
func ParseFileQueryDocument(raw string) (FileQueryDocument, error) {
	original := strings.TrimSpace(raw)
	if original == "" {
		return FileQueryDocument{}, fmt.Errorf("query must not be empty")
	}
	doc := FileQueryDocument{Original: original}
	lines := nonEmptyFileQueryLines(original)

	hasTyped := false
	hasExpand := false
	for _, line := range lines {
		if isFileQueryExpandLine(line.text) {
			hasExpand = true
			continue
		}
		if _, _, ok := parseFileQueryTypedLine(line.text); ok {
			hasTyped = true
		}
	}

	if hasExpand {
		if len(lines) > 1 {
			for _, line := range lines {
				if isFileQueryExpandLine(line.text) {
					return FileQueryDocument{}, fmt.Errorf("line %d starts with expand:, but query documents cannot mix expand with typed lines", line.number)
				}
			}
		}
		_, query, _ := strings.Cut(strings.TrimSpace(lines[0].text), ":")
		query = strings.TrimSpace(query)
		if query == "" {
			return FileQueryDocument{}, fmt.Errorf("expand query must include text")
		}
		if err := validateFileQueryClauseText(query); err != nil {
			return FileQueryDocument{}, err
		}
		doc.Query = query
		return doc, nil
	}

	if !hasTyped {
		if len(lines) > 1 {
			return FileQueryDocument{}, fmt.Errorf("multi-line query documents must use lex:, vec:, hyde:, or intent: prefixes")
		}
		query := strings.TrimSpace(lines[0].text)
		if err := validateFileQueryClauseText(query); err != nil {
			return FileQueryDocument{}, err
		}
		doc.Query = query
		return doc, nil
	}

	doc.Typed = true
	for _, line := range lines {
		trimmed := strings.TrimSpace(line.text)
		if trimmed == "" {
			continue
		}
		typ, value, ok := parseFileQueryTypedLine(line.text)
		if !ok {
			return FileQueryDocument{}, fmt.Errorf("line %d is missing a lex:, vec:, hyde:, or intent: prefix", line.number)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return FileQueryDocument{}, fmt.Errorf("line %d %s query must not be empty", line.number, typ)
		}
		if err := validateFileQueryClauseText(value); err != nil {
			return FileQueryDocument{}, fmt.Errorf("line %d: %w", line.number, err)
		}
		if typ == FileQuerySearchIntent {
			if doc.Intent != "" {
				return FileQueryDocument{}, fmt.Errorf("line %d duplicates intent", line.number)
			}
			doc.Intent = value
			continue
		}
		doc.Searches = append(doc.Searches, FileQuerySearch{Type: typ, Query: value})
	}
	if len(doc.Searches) == 0 {
		return FileQueryDocument{}, fmt.Errorf("typed query document must include at least one lex, vec, or hyde clause")
	}
	return doc, nil
}

type fileQueryLine struct {
	number int
	text   string
}

func nonEmptyFileQueryLines(query string) []fileQueryLine {
	rawLines := strings.Split(query, "\n")
	lines := make([]fileQueryLine, 0, len(rawLines))
	for i, line := range rawLines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, fileQueryLine{number: i + 1, text: line})
	}
	return lines
}

func isFileQueryExpandLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(strings.ToLower(trimmed), "expand:")
}

func parseFileQueryTypedLine(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	before, after, ok := strings.Cut(trimmed, ":")
	if !ok {
		return "", "", false
	}
	switch strings.ToLower(strings.TrimSpace(before)) {
	case FileQuerySearchLex:
		return FileQuerySearchLex, after, true
	case FileQuerySearchVec:
		return FileQuerySearchVec, after, true
	case FileQuerySearchHyde:
		return FileQuerySearchHyde, after, true
	case FileQuerySearchIntent:
		return FileQuerySearchIntent, after, true
	default:
		return "", "", false
	}
}

func validateFileQueryClauseText(text string) error {
	escaped := false
	inDoubleQuote := false
	for _, r := range text {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			inDoubleQuote = !inDoubleQuote
		}
	}
	if inDoubleQuote {
		return fmt.Errorf("unbalanced double quote")
	}
	return nil
}
