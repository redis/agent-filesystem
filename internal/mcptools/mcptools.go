// Package mcptools holds the text-patching, argument-parsing, and
// shared-shape helpers used by both MCP surfaces in this repo:
//
//   - cmd/afs/afs_mcp.go            — the local stdio MCP server
//   - internal/controlplane/mcp_*   — the hosted control-plane MCP
//
// These helpers were duplicated byte-for-byte across the two packages
// before this extraction. Changes here apply to both surfaces.
package mcptools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ----- Shared shapes -----

// FileListItem is a workspace file/dir entry returned by file_list and
// file_glob.
type FileListItem struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modified_at,omitempty"`
	Target     string `json:"target,omitempty"`
}

// GrepMatch is a single grep hit returned by file_grep.
type GrepMatch struct {
	Path   string `json:"path"`
	Line   int64  `json:"line,omitempty"`
	Text   string `json:"text"`
	Binary bool   `json:"binary,omitempty"`
}

// GrepCount is a per-file count returned by file_grep with count_only.
type GrepCount struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

// FilePatchOp describes one operation in a file_patch tool call.
type FilePatchOp struct {
	Op            string `json:"op"`
	StartLine     *int   `json:"start_line,omitempty"`
	EndLine       *int   `json:"end_line,omitempty"`
	Old           string `json:"old,omitempty"`
	New           string `json:"new,omitempty"`
	ContextBefore string `json:"context_before,omitempty"`
	ContextAfter  string `json:"context_after,omitempty"`
}

// FilePatchInput is the request body for the file_patch tool.
type FilePatchInput struct {
	Path           string        `json:"path"`
	ExpectedSHA256 string        `json:"expected_sha256,omitempty"`
	Patches        []FilePatchOp `json:"patches"`
}

// TextMatch is the position of a single matched substring within a file's
// text content.
type TextMatch struct {
	Start     int
	End       int
	StartLine int
	EndLine   int
}

// ----- Argument parsers -----

// RequiredString returns a non-empty trimmed string, or an error if the
// argument is missing, the wrong type, or blank after trimming.
func RequiredString(args map[string]any, key string) (string, error) {
	value, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing required argument %q", key)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("argument %q must be a string", key)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("argument %q must not be empty", key)
	}
	return text, nil
}

// RequiredText returns a string argument without trimming. When
// allowEmpty is false, blank-after-trimming counts as missing.
func RequiredText(args map[string]any, key string, allowEmpty bool) (string, error) {
	value, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing required argument %q", key)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("argument %q must be a string", key)
	}
	if !allowEmpty && strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("argument %q must not be empty", key)
	}
	return text, nil
}

// OptionalString returns a trimmed string, "" if the argument is absent.
func OptionalString(args map[string]any, key string) (string, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("argument %q must be a string", key)
	}
	return strings.TrimSpace(text), nil
}

// OptionalText returns the raw string without trimming, "" if absent.
func OptionalText(args map[string]any, key string) (string, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("argument %q must be a string", key)
	}
	return text, nil
}

// StringDefault returns OptionalString or fallback if blank.
func StringDefault(args map[string]any, key, fallback string) (string, error) {
	value, err := OptionalString(args, key)
	if err != nil {
		return "", err
	}
	if value == "" {
		return fallback, nil
	}
	return value, nil
}

// Bool returns a boolean argument or fallback if absent.
func Bool(args map[string]any, key string, fallback bool) (bool, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return fallback, nil
	}
	switch v := value.(type) {
	case bool:
		return v, nil
	default:
		return false, fmt.Errorf("argument %q must be a boolean", key)
	}
}

// Int returns an integer argument or fallback if absent. Accepts JSON
// number variants (float64), Go int, and int64.
func Int(args map[string]any, key string, fallback int) (int, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return fallback, nil
	}
	switch v := value.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case int64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("argument %q must be an integer", key)
	}
}

// OptionalInt returns a *int that is nil when the argument is absent.
func OptionalInt(args map[string]any, key string) (*int, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, nil
	}
	intValue, err := Int(args, key, 0)
	if err != nil {
		return nil, err
	}
	return &intValue, nil
}

// OptionalInt64 returns a *int64 that is nil when the argument is absent.
func OptionalInt64(args map[string]any, key string) (*int64, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case float64:
		result := int64(v)
		return &result, nil
	case int:
		result := int64(v)
		return &result, nil
	case int64:
		return &v, nil
	default:
		return nil, fmt.Errorf("argument %q must be an integer", key)
	}
}

// OptionalStringSlice returns a *[]string that is nil when the argument
// is absent. Accepts both []string and []any.
func OptionalStringSlice(args map[string]any, key string) (*[]string, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case []string:
		copied := append([]string(nil), typed...)
		return &copied, nil
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("argument %q must be an array of strings", key)
			}
			values = append(values, text)
		}
		return &values, nil
	default:
		return nil, fmt.Errorf("argument %q must be an array of strings", key)
	}
}

// DecodeArgs round-trips an args map through JSON to populate a typed
// target struct. Convenient for tools that accept a richer body shape.
func DecodeArgs(args map[string]any, target any) error {
	body, err := json.Marshal(args)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

// ----- Text-patching helpers -----

// TextSHA256 returns the lowercase hex SHA-256 of content. Used as the
// expected_sha256 verifier on file_patch.
func TextSHA256(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// CountTextMatches counts occurrences of old in content that satisfy the
// optional surrounding-context and line-range constraints.
func CountTextMatches(content, old, contextBefore, contextAfter string, startLine, endLine *int) int {
	if old == "" {
		return 0
	}
	count := 0
	offset := 0
	for {
		index := strings.Index(content[offset:], old)
		if index < 0 {
			break
		}
		matchStart := offset + index
		matchEnd := matchStart + len(old)
		match := TextMatch{
			Start:     matchStart,
			End:       matchEnd,
			StartLine: LineNumberAtOffset(content, matchStart),
			EndLine:   TextEndLine(LineNumberAtOffset(content, matchStart), old),
		}
		if MatchMatchesConstraints(content, match, contextBefore, contextAfter, startLine, endLine) {
			count++
		}
		offset = matchStart + len(old)
	}
	return count
}

// FindSingleTextMatch locates the unique occurrence of old satisfying the
// constraints, returning an error if zero or more than one match.
func FindSingleTextMatch(content, old, contextBefore, contextAfter string, startLine, endLine *int) (TextMatch, error) {
	if old == "" {
		return TextMatch{}, errors.New("old text must not be empty")
	}
	var (
		match  TextMatch
		found  bool
		offset int
		count  int
	)
	for {
		index := strings.Index(content[offset:], old)
		if index < 0 {
			break
		}
		matchStart := offset + index
		matchEnd := matchStart + len(old)
		current := TextMatch{
			Start:     matchStart,
			End:       matchEnd,
			StartLine: LineNumberAtOffset(content, matchStart),
			EndLine:   TextEndLine(LineNumberAtOffset(content, matchStart), old),
		}
		if MatchMatchesConstraints(content, current, contextBefore, contextAfter, startLine, endLine) {
			match = current
			found = true
			count++
		}
		offset = matchStart + len(old)
	}
	switch {
	case !found:
		return TextMatch{}, errors.New("target text not found with the requested constraints")
	case count > 1:
		return TextMatch{}, fmt.Errorf("target text matched %d times; refine with start_line or surrounding context", count)
	default:
		return match, nil
	}
}

// MatchMatchesConstraints reports whether match satisfies the optional
// startLine, endLine, contextBefore, contextAfter constraints.
func MatchMatchesConstraints(content string, match TextMatch, contextBefore, contextAfter string, startLine, endLine *int) bool {
	if startLine != nil && match.StartLine != *startLine {
		return false
	}
	if endLine != nil && match.EndLine != *endLine {
		return false
	}
	if contextBefore != "" && !strings.HasSuffix(content[:match.Start], contextBefore) {
		return false
	}
	if contextAfter != "" && !strings.HasPrefix(content[match.End:], contextAfter) {
		return false
	}
	return true
}

// LineNumberAtOffset returns the 1-indexed line number containing the
// given byte offset in content.
func LineNumberAtOffset(content string, offset int) int {
	if offset <= 0 {
		return 1
	}
	return strings.Count(content[:offset], "\n") + 1
}

// TextEndLine returns the 1-indexed line number that contains the last
// character of text when it begins on startLine.
func TextEndLine(startLine int, text string) int {
	if text == "" {
		return startLine
	}
	newlineCount := strings.Count(text, "\n")
	if newlineCount == 0 {
		return startLine
	}
	if strings.HasSuffix(text, "\n") {
		return startLine + newlineCount - 1
	}
	return startLine + newlineCount
}

// ApplyTextPatch applies one FilePatchOp to content and returns the new
// content along with a metadata map describing what changed.
func ApplyTextPatch(content string, patch FilePatchOp) (string, map[string]any, error) {
	switch patch.Op {
	case "replace":
		if patch.Old == "" {
			return "", nil, errors.New("replace patch requires old")
		}
		match, err := FindSingleTextMatch(content, patch.Old, patch.ContextBefore, patch.ContextAfter, patch.StartLine, patch.EndLine)
		if err != nil {
			return "", nil, err
		}
		return content[:match.Start] + patch.New + content[match.End:], map[string]any{
			"op":         patch.Op,
			"start_line": match.StartLine,
			"end_line":   match.EndLine,
		}, nil
	case "insert":
		if patch.New == "" {
			return "", nil, errors.New("insert patch requires new")
		}
		if patch.StartLine == nil {
			return "", nil, errors.New("insert patch requires start_line")
		}
		insertOffset, actualLine, err := InsertOffsetForLine(content, *patch.StartLine)
		if err != nil {
			return "", nil, err
		}
		if patch.ContextBefore != "" && !strings.HasSuffix(content[:insertOffset], patch.ContextBefore) {
			return "", nil, errors.New("insert patch context_before did not match")
		}
		if patch.ContextAfter != "" && !strings.HasPrefix(content[insertOffset:], patch.ContextAfter) {
			return "", nil, errors.New("insert patch context_after did not match")
		}
		return content[:insertOffset] + patch.New + content[insertOffset:], map[string]any{
			"op":         patch.Op,
			"start_line": actualLine,
		}, nil
	case "delete":
		switch {
		case patch.Old != "":
			match, err := FindSingleTextMatch(content, patch.Old, patch.ContextBefore, patch.ContextAfter, patch.StartLine, patch.EndLine)
			if err != nil {
				return "", nil, err
			}
			return content[:match.Start] + content[match.End:], map[string]any{
				"op":         patch.Op,
				"start_line": match.StartLine,
				"end_line":   match.EndLine,
			}, nil
		case patch.StartLine != nil && patch.EndLine != nil:
			next, deleted, err := DeleteContentLines(content, *patch.StartLine, *patch.EndLine)
			if err != nil {
				return "", nil, err
			}
			return next, map[string]any{
				"op":            patch.Op,
				"start_line":    *patch.StartLine,
				"end_line":      *patch.EndLine,
				"deleted_lines": deleted,
			}, nil
		default:
			return "", nil, errors.New("delete patch requires old or both start_line and end_line")
		}
	default:
		return "", nil, fmt.Errorf("unsupported patch op %q", patch.Op)
	}
}

// InsertOffsetForLine returns the byte offset and resolved line number
// at which a startLine-positioned insert should land.
//
// startLine semantics:
//   - 0  → insert at byte 0 (before everything)
//   - -1 → insert at end of file
//   - >0 → insert before line N, where lines are 1-indexed
func InsertOffsetForLine(content string, startLine int) (int, int, error) {
	if startLine < -1 {
		return 0, 0, errors.New("start_line must be >= -1")
	}
	if startLine == -1 {
		return len(content), -1, nil
	}
	if startLine == 0 {
		return 0, 0, nil
	}
	lines := SplitTextLines(content)
	if startLine > len(lines) {
		return 0, 0, fmt.Errorf("start_line %d is beyond EOF", startLine)
	}
	offset := 0
	for i := 0; i < startLine; i++ {
		offset += len(lines[i])
	}
	return offset, startLine, nil
}

// DeleteContentLines removes lines [start..end] (inclusive, 1-indexed)
// from content and returns the new content along with the count of lines
// actually removed.
func DeleteContentLines(content string, start, end int) (string, int, error) {
	if start <= 0 || end < start {
		return "", 0, errors.New("start_line and end_line must be >= 1 and end_line must be >= start_line")
	}
	lines := SplitTextLines(content)
	if start > len(lines) {
		return "", 0, fmt.Errorf("start_line %d is beyond EOF", start)
	}
	if end > len(lines) {
		end = len(lines)
	}
	next := strings.Join(append(lines[:start-1], lines[end:]...), "")
	return next, end - start + 1, nil
}

// SplitTextLines splits content into individual lines preserving
// trailing newlines, so reassembly via strings.Join(..., "") returns
// the original text exactly.
func SplitTextLines(content string) []string {
	if content == "" {
		return []string{}
	}
	lines := strings.SplitAfter(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
