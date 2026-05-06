package controlplane

import (
	"context"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/redis/agent-filesystem/internal/mcptools"
	afsclient "github.com/redis/agent-filesystem/mount/client"
)

// Aliases for shared MCP helpers and types. The canonical definitions
// live in internal/mcptools — see that package for behavior. Local names
// are retained so existing call sites in this package keep working.
type (
	mcpFilePatchOp    = mcptools.FilePatchOp
	mcpFilePatchInput = mcptools.FilePatchInput
	mcpTextMatch      = mcptools.TextMatch
	mcpGrepCount      = mcptools.GrepCount
	mcpGrepMatch      = mcptools.GrepMatch
	mcpFileListItem   = mcptools.FileListItem
)

var (
	mcpRequiredString       = mcptools.RequiredString
	mcpRequiredText         = mcptools.RequiredText
	mcpOptionalString       = mcptools.OptionalString
	mcpOptionalText         = mcptools.OptionalText
	mcpStringDefault        = mcptools.StringDefault
	mcpBool                 = mcptools.Bool
	mcpInt                  = mcptools.Int
	mcpOptionalInt          = mcptools.OptionalInt
	mcpOptionalInt64        = mcptools.OptionalInt64
	mcpOptionalStringSlice  = mcptools.OptionalStringSlice
	decodeMCPArgs           = mcptools.DecodeArgs
	textSHA256              = mcptools.TextSHA256
	countTextMatches        = mcptools.CountTextMatches
	findSingleTextMatch     = mcptools.FindSingleTextMatch
	matchMatchesConstraints = mcptools.MatchMatchesConstraints
	lineNumberAtOffset      = mcptools.LineNumberAtOffset
	textEndLine             = mcptools.TextEndLine
	applyMCPTextPatch       = mcptools.ApplyTextPatch
	insertOffsetForLine     = mcptools.InsertOffsetForLine
	deleteContentLines      = mcptools.DeleteContentLines
	splitTextLines          = mcptools.SplitTextLines
)

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

func hostedMCPCollectGrepMatches(filePath string, content []byte, opts hostedMCPGrepOptions, matcher *hostedMCPGrepMatcher) []mcptools.GrepMatch {
	if grepBinaryPrefix(content) {
		return nil
	}
	out := make([]mcptools.GrepMatch, 0)
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
		out = append(out, mcptools.GrepMatch{
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
