package querysearch

import (
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/redis/agent-filesystem/internal/mcptools"
)

const (
	snippetMaxBytes    = 500
	snippetBeforeLines = 1
	snippetAfterLines  = 2
)

// Target is a readable workspace file considered for ranked retrieval.
type Target struct {
	Path    string
	Content []byte
}

// KeywordOptions controls local keyword ranking and result shaping.
type KeywordOptions struct {
	Limit    int
	All      bool
	MinScore float64
	Full     bool
}

// KeywordSpec is the normalized keyword query used by the fallback ranker.
type KeywordSpec struct {
	Positive    []string
	Negative    []string
	SearchTypes []string
}

// KeywordSpecFromRequest builds a keyword retrieval spec from a file_query
// request. QMD semantic clauses become keyword text when vector retrieval is
// unavailable, preserving a useful fallback without pretending semantics ran.
func KeywordSpecFromRequest(request mcptools.FileQueryRequest) KeywordSpec {
	spec := KeywordSpec{}
	if len(request.Searches) > 0 {
		seenTypes := make(map[string]struct{})
		for _, search := range request.Searches {
			spec.addText(search.Query)
			if _, ok := seenTypes[search.Type]; !ok {
				seenTypes[search.Type] = struct{}{}
				spec.SearchTypes = append(spec.SearchTypes, search.Type)
			}
		}
	} else {
		spec.addText(request.Query)
	}
	if len(spec.SearchTypes) == 0 {
		spec.SearchTypes = []string{"keyword"}
	}
	spec.Positive = uniqueStrings(spec.Positive)
	spec.Negative = uniqueStrings(spec.Negative)
	return spec
}

// HasSemanticClauses reports whether the query document/request asks for vector
// or HYDE retrieval.
func HasSemanticClauses(searches []mcptools.FileQuerySearch) bool {
	for _, search := range searches {
		switch search.Type {
		case mcptools.FileQuerySearchVec, mcptools.FileQuerySearchHyde:
			return true
		}
	}
	return false
}

// RankKeywordTargets ranks files with a deterministic lightweight BM25-like
// fallback until the Redis Search lexical query backend is wired end-to-end.
func RankKeywordTargets(targets []Target, spec KeywordSpec, opts KeywordOptions) []mcptools.FileQueryResult {
	candidates := make([]mcptools.FileQueryResult, 0, len(targets))
	for _, target := range targets {
		content := string(target.Content)
		lowerContent := strings.ToLower(content)
		if containsAny(lowerContent, spec.Negative) {
			continue
		}
		score, line, logicalSnippet := scoreKeywordContent(target.Path, content, spec.Positive)
		if score <= 0 || score < opts.MinScore {
			continue
		}
		endLine := line
		snippet := ""
		metadata := map[string]any(nil)
		if opts.Full {
			snippet = content
			line = 1
			endLine = len(splitTextLines(content))
		} else {
			focused := extractKeywordSnippet(content, spec.Positive, line, snippetMaxBytes)
			snippet = focused.Text
			metadata = map[string]any{
				"snippet_start_line": focused.StartLine,
				"snippet_end_line":   focused.EndLine,
			}
			logicalSnippet = strings.TrimSpace(logicalSnippet)
			physicalMatch := physicalLine(content, line)
			if logicalSnippet != "" &&
				physicalMatch != "" &&
				logicalSnippet != physicalMatch &&
				strings.Contains(physicalMatch, logicalSnippet) &&
				len(logicalSnippet) < len(snippet) &&
				snippetLineScore(logicalSnippet, spec.Positive) > 0 {
				snippet = truncateUTF8(logicalSnippet, snippetMaxBytes)
				metadata["snippet_start_line"] = line
				metadata["snippet_end_line"] = line
			}
		}
		candidates = append(candidates, mcptools.FileQueryResult{
			Path:        target.Path,
			StartLine:   line,
			EndLine:     endLine,
			Score:       score,
			Snippet:     snippet,
			SearchTypes: append([]string(nil), spec.SearchTypes...),
			Metadata:    metadata,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].Score > candidates[j].Score
	})
	if !opts.All && opts.Limit >= 0 && len(candidates) > opts.Limit {
		candidates = candidates[:opts.Limit]
	}
	return candidates
}

func (s *KeywordSpec) addText(text string) {
	positive, negative := parseKeywordText(text)
	s.Positive = append(s.Positive, positive...)
	s.Negative = append(s.Negative, negative...)
}

func parseKeywordText(text string) ([]string, []string) {
	positive := make([]string, 0)
	negative := make([]string, 0)
	fallbackPositive := make([]string, 0)
	for _, token := range splitKeywordTokens(text) {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		target := &positive
		if strings.HasPrefix(token, "-") {
			target = &negative
			token = strings.TrimPrefix(token, "-")
		}
		token = normalizeKeywordToken(token)
		if token == "" {
			continue
		}
		if target == &positive {
			fallbackPositive = append(fallbackPositive, token)
		}
		if keywordStopWords[token] {
			continue
		}
		*target = append(*target, token)
	}
	if len(positive) == 0 && len(negative) == 0 {
		positive = fallbackPositive
	}
	return positive, negative
}

func splitKeywordTokens(text string) []string {
	tokens := make([]string, 0)
	var b strings.Builder
	inQuote := false
	pendingNegation := false
	for _, r := range text {
		switch {
		case r == '"':
			if inQuote {
				token := b.String()
				b.Reset()
				if pendingNegation {
					token = "-" + token
					pendingNegation = false
				}
				tokens = append(tokens, token)
				inQuote = false
			} else {
				if strings.TrimSpace(b.String()) == "-" {
					pendingNegation = true
				} else {
					tokens = append(tokens, strings.Fields(b.String())...)
				}
				b.Reset()
				inQuote = true
			}
		case inQuote:
			b.WriteRune(r)
		case unicode.IsSpace(r):
			tokens = append(tokens, strings.Fields(b.String())...)
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		if inQuote && pendingNegation {
			tokens = append(tokens, "-"+b.String())
		} else {
			tokens = append(tokens, strings.Fields(b.String())...)
		}
	}
	return tokens
}

func normalizeKeywordToken(token string) string {
	token = strings.ToLower(strings.TrimSpace(token))
	token = strings.Trim(token, ".,;:!?()[]{}<>`'“”‘’")
	return token
}

var keywordStopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "do": true, "does": true, "for": true, "from": true,
	"how": true, "i": true, "in": true, "is": true, "it": true, "of": true,
	"on": true, "or": true, "the": true, "this": true, "to": true, "what": true,
	"when": true, "where": true, "why": true, "with": true,
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func containsAny(content string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(content, term) {
			return true
		}
	}
	return false
}

func scoreKeywordContent(filePath, content string, terms []string) (float64, int, string) {
	lines := splitSearchLines(content)
	bestScore := 0.0
	bestLine := 0
	bestSnippet := ""
	totalScore := 0.0
	lowerPath := strings.ToLower(filePath)
	for _, line := range lines {
		lineScore := scoreKeywordText(strings.ToLower(line.Text), terms)
		if lineScore > 0 {
			totalScore += lineScore
		}
		if lineScore > bestScore {
			bestScore = lineScore
			bestLine = line.Number
			bestSnippet = line.Text
		}
	}
	pathScore := scoreKeywordText(lowerPath, terms) * 0.75
	score := bestScore*2 + math.Log1p(totalScore) + pathScore
	return score, bestLine, bestSnippet
}

func physicalLine(content string, line int) string {
	lines := splitTextLines(content)
	if line <= 0 || line > len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[line-1])
}

type keywordSnippet struct {
	Text      string
	StartLine int
	EndLine   int
}

func extractKeywordSnippet(content string, terms []string, matchLine, maxBytes int) keywordSnippet {
	lines := splitTextLines(content)
	if len(lines) == 0 {
		return keywordSnippet{StartLine: matchLine, EndLine: matchLine}
	}
	matchIndex := matchLine - 1
	if matchIndex < 0 || matchIndex >= len(lines) || snippetLineScore(lines[matchIndex], terms) == 0 {
		matchIndex = bestKeywordSnippetLine(lines, terms)
	}
	start := max(0, matchIndex-snippetBeforeLines)
	end := min(len(lines), matchIndex+snippetAfterLines+1)
	text := strings.TrimRight(strings.Join(lines[start:end], "\n"), "\n")
	if maxBytes > 0 && len(text) > maxBytes {
		text = truncateUTF8(text, maxBytes)
	}
	return keywordSnippet{
		Text:      text,
		StartLine: start + 1,
		EndLine:   end,
	}
}

func bestKeywordSnippetLine(lines []string, terms []string) int {
	bestIndex := 0
	bestScore := -1
	for i, line := range lines {
		score := snippetLineScore(line, terms)
		if score > bestScore {
			bestScore = score
			bestIndex = i
		}
	}
	return bestIndex
}

func snippetLineScore(line string, terms []string) int {
	lower := strings.ToLower(line)
	score := 0
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term == "" {
			continue
		}
		score += strings.Count(lower, term)
	}
	return score
}

func truncateUTF8(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	if maxBytes <= 3 {
		return strings.Repeat(".", maxBytes)
	}
	cut := maxBytes - 3
	for cut > 0 && !utf8.ValidString(text[:cut]) {
		cut--
	}
	return text[:cut] + "..."
}

func scoreKeywordText(text string, terms []string) float64 {
	score := 0.0
	for _, term := range terms {
		count := strings.Count(text, term)
		if count == 0 {
			continue
		}
		weight := 1.0
		if strings.Contains(term, " ") {
			weight = 4.0
		}
		score += float64(count) * weight
	}
	return score
}

type searchLine struct {
	Text   string
	Number int
}

func splitSearchLines(content string) []searchLine {
	physical := splitTextLines(content)
	lines := make([]searchLine, 0, len(physical))
	for i, line := range physical {
		for _, part := range splitLogicalText(line, 8<<10) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			lines = append(lines, searchLine{Text: part, Number: i + 1})
		}
	}
	return lines
}

func splitLogicalText(text string, maxBytes int) []string {
	parts := []string{text}
	parts = splitEachAfter(parts, "} {", 1)
	parts = splitEachAfter(parts, "\\n", len("\\n"))
	if maxBytes > 0 {
		parts = splitEachLongPart(parts, maxBytes)
	}
	return parts
}

func splitEachAfter(parts []string, token string, keep int) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, splitAfterToken(part, token, keep)...)
	}
	return out
}

func splitAfterToken(text, token string, keep int) []string {
	if token == "" || !strings.Contains(text, token) {
		return []string{text}
	}
	parts := make([]string, 0, 2)
	rest := text
	for {
		idx := strings.Index(rest, token)
		if idx < 0 {
			if rest != "" {
				parts = append(parts, rest)
			}
			return parts
		}
		cut := idx + keep
		parts = append(parts, rest[:cut])
		rest = rest[cut:]
	}
}

func splitEachLongPart(parts []string, maxBytes int) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, splitLongPart(part, maxBytes)...)
	}
	return out
}

func splitLongPart(text string, maxBytes int) []string {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return []string{text}
	}
	parts := make([]string, 0, len(text)/maxBytes+1)
	rest := strings.TrimSpace(text)
	for len(rest) > maxBytes {
		cut := lastWhitespaceBefore(rest, maxBytes)
		if cut <= 0 {
			cut = maxBytes
		}
		parts = append(parts, rest[:cut])
		rest = strings.TrimSpace(rest[cut:])
	}
	if rest != "" {
		parts = append(parts, rest)
	}
	return parts
}

func lastWhitespaceBefore(text string, maxBytes int) int {
	cut := 0
	for idx, r := range text {
		if idx > maxBytes {
			break
		}
		if r == ' ' || r == '\t' {
			cut = idx
		}
	}
	return cut
}

func splitTextLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}
