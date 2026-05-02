package controlplane

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	afsclient "github.com/redis/agent-filesystem/mount/client"
)

type mcpFilePatchOp struct {
	Op            string `json:"op"`
	StartLine     *int   `json:"start_line,omitempty"`
	EndLine       *int   `json:"end_line,omitempty"`
	Old           string `json:"old,omitempty"`
	New           string `json:"new,omitempty"`
	ContextBefore string `json:"context_before,omitempty"`
	ContextAfter  string `json:"context_after,omitempty"`
}

type mcpFilePatchInput struct {
	Path           string           `json:"path"`
	ExpectedSHA256 string           `json:"expected_sha256,omitempty"`
	Patches        []mcpFilePatchOp `json:"patches"`
}

type mcpTextMatch struct {
	Start     int
	End       int
	StartLine int
	EndLine   int
}

type mcpGrepCount struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

type hostedMCPGrepOptions struct {
	path             string
	ignoreCase       bool
	glob             bool
	fixedStrings     bool
	regexp           bool
	wordRegexp       bool
	lineRegexp       bool
	invertMatch      bool
	filesWithMatches bool
	countOnly        bool
	maxCount         int
	pattern          string
}

type hostedMCPGrepMatcher struct {
	regexps       []*regexp.Regexp
	literal       string
	lowerLiteral  string
	globPattern   string
	lowerGlob     string
	ignoreCase    bool
	useGlob       bool
	useRegexp     bool
	useWordOrLine bool
}

type hostedMCPGrepTarget struct {
	path    string
	content []byte
	loaded  bool
}

type mcpWorkspaceVersioningPolicyPatch struct {
	Mode                 *string
	IncludeGlobs         *[]string
	ExcludeGlobs         *[]string
	MaxVersionsPerFile   *int
	MaxAgeDays           *int
	MaxTotalBytes        *int64
	LargeFileCutoffBytes *int64
}

func mcpOptionalText(args map[string]any, key string) (string, error) {
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

func mcpOptionalInt(args map[string]any, key string) (*int, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, nil
	}
	intValue, err := mcpInt(args, key, 0)
	if err != nil {
		return nil, err
	}
	return &intValue, nil
}

func mcpOptionalInt64(args map[string]any, key string) (*int64, error) {
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

func mcpOptionalStringSlice(args map[string]any, key string) (*[]string, error) {
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

func mcpWorkspaceVersioningPolicyPatchFromArgs(args map[string]any) (mcpWorkspaceVersioningPolicyPatch, error) {
	var patch mcpWorkspaceVersioningPolicyPatch

	if rawMode, ok := args["mode"]; ok && rawMode != nil {
		mode, err := mcpOptionalString(args, "mode")
		if err != nil {
			return patch, err
		}
		patch.Mode = &mode
	}
	includeGlobs, err := mcpOptionalStringSlice(args, "include_globs")
	if err != nil {
		return patch, err
	}
	patch.IncludeGlobs = includeGlobs
	excludeGlobs, err := mcpOptionalStringSlice(args, "exclude_globs")
	if err != nil {
		return patch, err
	}
	patch.ExcludeGlobs = excludeGlobs
	maxVersionsPerFile, err := mcpOptionalInt(args, "max_versions_per_file")
	if err != nil {
		return patch, err
	}
	patch.MaxVersionsPerFile = maxVersionsPerFile
	maxAgeDays, err := mcpOptionalInt(args, "max_age_days")
	if err != nil {
		return patch, err
	}
	patch.MaxAgeDays = maxAgeDays
	maxTotalBytes, err := mcpOptionalInt64(args, "max_total_bytes")
	if err != nil {
		return patch, err
	}
	patch.MaxTotalBytes = maxTotalBytes
	largeFileCutoffBytes, err := mcpOptionalInt64(args, "large_file_cutoff_bytes")
	if err != nil {
		return patch, err
	}
	patch.LargeFileCutoffBytes = largeFileCutoffBytes

	return patch, nil
}

func applyMCPWorkspaceVersioningPolicyPatch(base WorkspaceVersioningPolicy, patch mcpWorkspaceVersioningPolicyPatch) WorkspaceVersioningPolicy {
	next := base
	if patch.Mode != nil {
		next.Mode = *patch.Mode
	}
	if patch.IncludeGlobs != nil {
		next.IncludeGlobs = append([]string(nil), (*patch.IncludeGlobs)...)
	}
	if patch.ExcludeGlobs != nil {
		next.ExcludeGlobs = append([]string(nil), (*patch.ExcludeGlobs)...)
	}
	if patch.MaxVersionsPerFile != nil {
		next.MaxVersionsPerFile = *patch.MaxVersionsPerFile
	}
	if patch.MaxAgeDays != nil {
		next.MaxAgeDays = *patch.MaxAgeDays
	}
	if patch.MaxTotalBytes != nil {
		next.MaxTotalBytes = *patch.MaxTotalBytes
	}
	if patch.LargeFileCutoffBytes != nil {
		next.LargeFileCutoffBytes = *patch.LargeFileCutoffBytes
	}
	return next
}

func decodeMCPArgs(args map[string]any, target any) error {
	body, err := json.Marshal(args)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func textSHA256(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func countTextMatches(content, old, contextBefore, contextAfter string, startLine, endLine *int) int {
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
		match := mcpTextMatch{
			Start:     matchStart,
			End:       matchEnd,
			StartLine: lineNumberAtOffset(content, matchStart),
			EndLine:   textEndLine(lineNumberAtOffset(content, matchStart), old),
		}
		if matchMatchesConstraints(content, match, contextBefore, contextAfter, startLine, endLine) {
			count++
		}
		offset = matchStart + len(old)
	}
	return count
}

func findSingleTextMatch(content, old, contextBefore, contextAfter string, startLine, endLine *int) (mcpTextMatch, error) {
	if old == "" {
		return mcpTextMatch{}, errors.New("old text must not be empty")
	}
	var (
		match  mcpTextMatch
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
		current := mcpTextMatch{
			Start:     matchStart,
			End:       matchEnd,
			StartLine: lineNumberAtOffset(content, matchStart),
			EndLine:   textEndLine(lineNumberAtOffset(content, matchStart), old),
		}
		if matchMatchesConstraints(content, current, contextBefore, contextAfter, startLine, endLine) {
			match = current
			found = true
			count++
		}
		offset = matchStart + len(old)
	}
	switch {
	case !found:
		return mcpTextMatch{}, errors.New("target text not found with the requested constraints")
	case count > 1:
		return mcpTextMatch{}, fmt.Errorf("target text matched %d times; refine with start_line or surrounding context", count)
	default:
		return match, nil
	}
}

func matchMatchesConstraints(content string, match mcpTextMatch, contextBefore, contextAfter string, startLine, endLine *int) bool {
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

func lineNumberAtOffset(content string, offset int) int {
	if offset <= 0 {
		return 1
	}
	return strings.Count(content[:offset], "\n") + 1
}

func textEndLine(startLine int, text string) int {
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

func applyMCPTextPatch(content string, patch mcpFilePatchOp) (string, map[string]any, error) {
	switch patch.Op {
	case "replace":
		if patch.Old == "" {
			return "", nil, errors.New("replace patch requires old")
		}
		match, err := findSingleTextMatch(content, patch.Old, patch.ContextBefore, patch.ContextAfter, patch.StartLine, patch.EndLine)
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
		insertOffset, actualLine, err := insertOffsetForLine(content, *patch.StartLine)
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
			match, err := findSingleTextMatch(content, patch.Old, patch.ContextBefore, patch.ContextAfter, patch.StartLine, patch.EndLine)
			if err != nil {
				return "", nil, err
			}
			return content[:match.Start] + content[match.End:], map[string]any{
				"op":         patch.Op,
				"start_line": match.StartLine,
				"end_line":   match.EndLine,
			}, nil
		case patch.StartLine != nil && patch.EndLine != nil:
			next, deleted, err := deleteContentLines(content, *patch.StartLine, *patch.EndLine)
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

func insertOffsetForLine(content string, startLine int) (int, int, error) {
	if startLine < -1 {
		return 0, 0, errors.New("start_line must be >= -1")
	}
	if startLine == -1 {
		return len(content), -1, nil
	}
	if startLine == 0 {
		return 0, 0, nil
	}
	lines := splitTextLines(content)
	if startLine > len(lines) {
		return 0, 0, fmt.Errorf("start_line %d is beyond EOF", startLine)
	}
	offset := 0
	for i := 0; i < startLine; i++ {
		offset += len(lines[i])
	}
	return offset, startLine, nil
}

func deleteContentLines(content string, start, end int) (string, int, error) {
	if start <= 0 || end < start {
		return "", 0, errors.New("start_line and end_line must be >= 1 and end_line must be >= start_line")
	}
	lines := splitTextLines(content)
	if start > len(lines) {
		return "", 0, fmt.Errorf("start_line %d is beyond EOF", start)
	}
	if end > len(lines) {
		end = len(lines)
	}
	next := strings.Join(append(lines[:start-1], lines[end:]...), "")
	return next, end - start + 1, nil
}

func splitTextLines(content string) []string {
	if content == "" {
		return []string{}
	}
	lines := strings.SplitAfter(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func readWorkspaceTextContent(ctx context.Context, fsClient afsclient.Client, normalizedPath string, stat *afsclient.StatResult) (string, error) {
	if stat.Type != "file" {
		return "", fmt.Errorf("path %q is not a regular file", normalizedPath)
	}
	content, err := fsClient.Cat(ctx, normalizedPath)
	if err != nil {
		return "", err
	}
	if grepBinaryPrefix(content) {
		return "", fmt.Errorf("path %q is binary", normalizedPath)
	}
	return string(content), nil
}

func compileHostedMCPGrepMatcher(opts hostedMCPGrepOptions) (*hostedMCPGrepMatcher, error) {
	matcher := &hostedMCPGrepMatcher{ignoreCase: opts.ignoreCase}
	switch {
	case opts.regexp:
		pattern := opts.pattern
		if opts.wordRegexp {
			pattern = `(?:^|[^[:alnum:]_])(?:` + pattern + `)(?:$|[^[:alnum:]_])`
		}
		if opts.lineRegexp {
			pattern = `^(?:` + pattern + `)$`
		}
		if opts.ignoreCase {
			pattern = `(?i)` + pattern
		}
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regular expression %q: %w", pattern, err)
		}
		matcher.useRegexp = true
		matcher.regexps = []*regexp.Regexp{compiled}
		return matcher, nil
	case opts.wordRegexp || opts.lineRegexp:
		pattern := regexp.QuoteMeta(opts.pattern)
		if opts.wordRegexp {
			pattern = `(?:^|[^[:alnum:]_])(?:` + pattern + `)(?:$|[^[:alnum:]_])`
		}
		if opts.lineRegexp {
			pattern = `^(?:` + pattern + `)$`
		}
		if opts.ignoreCase {
			pattern = `(?i)` + pattern
		}
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		matcher.useRegexp = true
		matcher.useWordOrLine = true
		matcher.regexps = []*regexp.Regexp{compiled}
		return matcher, nil
	case opts.glob:
		matcher.useGlob = true
		matcher.globPattern = opts.pattern
		if opts.ignoreCase {
			matcher.lowerGlob = strings.ToLower(opts.pattern)
		}
	default:
		matcher.literal = opts.pattern
		if opts.ignoreCase {
			matcher.lowerLiteral = strings.ToLower(opts.pattern)
		}
	}
	return matcher, nil
}

func hostedMCPGrepLineMatches(line string, matcher *hostedMCPGrepMatcher) bool {
	switch {
	case matcher.useRegexp:
		return matcher.regexps[0].MatchString(line)
	case matcher.useGlob:
		pattern := matcher.globPattern
		target := line
		if matcher.ignoreCase {
			pattern = matcher.lowerGlob
			target = strings.ToLower(line)
		}
		ok, err := path.Match(pattern, target)
		return err == nil && ok
	default:
		if matcher.ignoreCase {
			return strings.Contains(strings.ToLower(line), matcher.lowerLiteral)
		}
		return strings.Contains(line, matcher.literal)
	}
}

func hostedMCPGrepFileHasMatch(content []byte, opts hostedMCPGrepOptions, matcher *hostedMCPGrepMatcher) bool {
	if grepBinaryPrefix(content) {
		return false
	}
	lines := splitTextLines(string(content))
	for _, line := range lines {
		text := strings.TrimSuffix(line, "\n")
		matched := hostedMCPGrepLineMatches(text, matcher)
		if opts.invertMatch {
			matched = !matched
		}
		if matched {
			return true
		}
	}
	return false
}

func hostedMCPGrepFileMatchCount(content []byte, opts hostedMCPGrepOptions, matcher *hostedMCPGrepMatcher) int {
	if grepBinaryPrefix(content) {
		return 0
	}
	count := 0
	lines := splitTextLines(string(content))
	for _, line := range lines {
		text := strings.TrimSuffix(line, "\n")
		matched := hostedMCPGrepLineMatches(text, matcher)
		if opts.invertMatch {
			matched = !matched
		}
		if matched {
			count++
			if opts.maxCount > 0 && count >= opts.maxCount {
				break
			}
		}
	}
	return count
}

func hostedMCPCollectGrepMatches(filePath string, content []byte, opts hostedMCPGrepOptions, matcher *hostedMCPGrepMatcher) []mcpGrepMatch {
	if grepBinaryPrefix(content) {
		return nil
	}
	out := make([]mcpGrepMatch, 0)
	lines := splitTextLines(string(content))
	for idx, line := range lines {
		text := strings.TrimSuffix(line, "\n")
		matched := hostedMCPGrepLineMatches(text, matcher)
		if opts.invertMatch {
			matched = !matched
		}
		if !matched {
			continue
		}
		out = append(out, mcpGrepMatch{
			Path: filePath,
			Line: int64(idx + 1),
			Text: text,
		})
		if opts.maxCount > 0 && len(out) >= opts.maxCount {
			break
		}
	}
	return out
}

func collectHostedMCPGrepTargets(ctx context.Context, fs afsclient.Client, searchPath string) ([]hostedMCPGrepTarget, error) {
	stat, err := fs.Stat(ctx, searchPath)
	if err != nil {
		return nil, err
	}
	if stat == nil {
		return nil, os.ErrNotExist
	}
	if stat.Type != "dir" {
		content, err := fs.Cat(ctx, searchPath)
		if err != nil {
			return nil, err
		}
		return []hostedMCPGrepTarget{{path: searchPath, content: content, loaded: true}}, nil
	}

	tree, err := fs.Tree(ctx, searchPath, 4096)
	if err != nil {
		return nil, err
	}
	targets := make([]hostedMCPGrepTarget, 0)
	for _, node := range tree {
		if node.Path == searchPath || node.Type != "file" {
			continue
		}
		targets = append(targets, hostedMCPGrepTarget{path: node.Path})
	}
	return targets, nil
}
