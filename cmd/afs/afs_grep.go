package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/rowantrollope/agent-filesystem/mount/client"
)

const grepTreeMaxDepth = 4096

type grepOptions struct {
	workspace        string
	path             string
	ignoreCase       bool
	glob             bool
	fixedStrings     bool
	extendedRegexp   bool
	basicRegexp      bool
	wordRegexp       bool
	lineRegexp       bool
	invertMatch      bool
	filesWithMatches bool
	countOnly        bool
	showLineNumbers  bool
	maxCount         int
	patterns         []string
}

type grepMode int

const (
	grepModeLiteral grepMode = iota
	grepModeRegex
	grepModeGlob
)

type grepMatcher struct {
	mode             grepMode
	ignoreCase       bool
	patterns         []string
	lowerPatterns    []string
	patternBytes     [][]byte
	lowerPatternData [][]byte
	regexps          []*regexp.Regexp
}

type grepFileTarget struct {
	path    string
	content []byte
	loaded  bool
}

func cmdGrep(args []string) error {
	bin := filepath.Base(os.Args[0])
	if len(args) < 2 || isHelpArg(args[1]) {
		fmt.Fprint(os.Stderr, grepUsageText(bin))
		return nil
	}

	opts, err := parseGrepArgs(args[1:])
	if err != nil {
		return err
	}

	cfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	ctx := context.Background()
	workspace, err := resolveWorkspaceName(ctx, cfg, store, opts.workspace)
	if err != nil {
		return err
	}

	fsKey, exists, err := resolveWorkspaceFSKey(ctx, cfg, store, workspace)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	searchPath := normalizeAFSGrepPath(opts.path)
	fsClient := client.New(store.rdb, fsKey)

	if useFastGrepBackend(opts) {
		return runFastGrep(ctx, fsClient, searchPath, workspace, opts)
	}

	matcher, err := compileGrepMatcher(opts)
	if err != nil {
		return err
	}
	return runAdvancedGrep(ctx, fsClient, searchPath, workspace, opts, matcher)
}

func parseGrepArgs(args []string) (grepOptions, error) {
	opts := grepOptions{showLineNumbers: true}
	var positionals []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--workspace" || arg == "-W":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for %q", arg)
			}
			i++
			opts.workspace = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--workspace="):
			opts.workspace = strings.TrimSpace(strings.TrimPrefix(arg, "--workspace="))
		case arg == "--path":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for %q", arg)
			}
			i++
			opts.path = args[i]
		case strings.HasPrefix(arg, "--path="):
			opts.path = strings.TrimPrefix(arg, "--path=")
		case arg == "-i" || arg == "--ignore-case":
			opts.ignoreCase = true
		case arg == "--glob":
			opts.glob = true
		case arg == "-F" || arg == "--fixed-strings":
			opts.fixedStrings = true
		case arg == "-E" || arg == "--extended-regexp":
			opts.extendedRegexp = true
		case arg == "-G" || arg == "--basic-regexp":
			opts.basicRegexp = true
		case arg == "-w" || arg == "--word-regexp":
			opts.wordRegexp = true
		case arg == "-x" || arg == "--line-regexp":
			opts.lineRegexp = true
		case arg == "-v" || arg == "--invert-match":
			opts.invertMatch = true
		case arg == "-l" || arg == "--files-with-matches":
			opts.filesWithMatches = true
		case arg == "-c" || arg == "--count":
			opts.countOnly = true
		case arg == "-n" || arg == "--line-number":
			opts.showLineNumbers = true
		case arg == "-e" || arg == "--regexp":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for %q", arg)
			}
			i++
			opts.patterns = append(opts.patterns, args[i])
		case strings.HasPrefix(arg, "--regexp="):
			opts.patterns = append(opts.patterns, strings.TrimPrefix(arg, "--regexp="))
		case arg == "-m" || arg == "--max-count":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for %q", arg)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 {
				return opts, fmt.Errorf("invalid value for %q: %q", arg, args[i])
			}
			opts.maxCount = n
		case strings.HasPrefix(arg, "--max-count="):
			raw := strings.TrimPrefix(arg, "--max-count=")
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 {
				return opts, fmt.Errorf("invalid value for %q: %q", "--max-count", raw)
			}
			opts.maxCount = n
		case arg == "--":
			positionals = append(positionals, args[i+1:]...)
			i = len(args)
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, fmt.Errorf("unknown flag %q", arg)
			}
			positionals = append(positionals, arg)
		}
	}

	if len(opts.patterns) == 0 {
		if len(positionals) != 1 {
			return opts, fmt.Errorf("%s", grepUsageText(filepath.Base(os.Args[0])))
		}
		opts.patterns = append(opts.patterns, positionals[0])
	} else if len(positionals) != 0 {
		return opts, fmt.Errorf("unexpected positional arguments: %s", strings.Join(positionals, " "))
	}

	if opts.filesWithMatches && opts.countOnly {
		return opts, errors.New("cannot combine --files-with-matches and --count")
	}
	if opts.maxCount < 0 {
		return opts, errors.New("--max-count must be >= 0")
	}

	modeFlags := 0
	if opts.glob {
		modeFlags++
	}
	if opts.fixedStrings {
		modeFlags++
	}
	if opts.extendedRegexp || opts.basicRegexp {
		modeFlags++
	}
	if modeFlags > 1 {
		return opts, errors.New("choose only one of --glob, --fixed-strings, or regex mode")
	}
	if opts.glob && (opts.wordRegexp || opts.lineRegexp) {
		return opts, errors.New("cannot combine --glob with -w/--word-regexp or -x/--line-regexp")
	}

	return opts, nil
}

func grepUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s grep [flags] <pattern>
  %s grep [flags] -e <pattern>

Search the live Redis-backed AFS namespace for a workspace.
Literal substring matching remains the default for compatibility. Use -E/-G
for regex mode, -F for explicit fixed-string mode, or --glob for AFS glob
matching semantics (*, ?, [a-z], [!x], \ escaping).

Options:
  --workspace <name>  Search a specific workspace (defaults to current)
  --path <path>       Limit the search to a file or directory inside the workspace
  -i, --ignore-case   Case-insensitive matching
  -F                  Treat patterns as fixed strings
  -E                  Use regex mode (RE2 syntax)
  -G                  Use regex mode (RE2 syntax; accepted for grep familiarity)
  -e <pattern>        Add a pattern (repeatable)
  -w                  Match whole words
  -x                  Match whole lines
  -v                  Invert the match
  -l                  Print matching file paths only
  -c                  Print per-file match counts
  -m <num>            Stop after NUM selected lines per file
  -n                  Accepted for grep familiarity; line numbers are shown by default
  --glob              Treat patterns as AFS globs instead of literals

Examples:
  %s grep "hello"
  %s grep -E "error|warning"
  %s grep -w --path /logs token
  %s grep -l -i --workspace repo "disk full"
  %s grep --glob --path /src "*TODO*"
`, bin, bin, bin, bin, bin, bin, bin)
}

func useFastGrepBackend(opts grepOptions) bool {
	if len(opts.patterns) != 1 {
		return false
	}
	if opts.extendedRegexp || opts.basicRegexp {
		return false
	}
	if opts.wordRegexp || opts.lineRegexp || opts.invertMatch {
		return false
	}
	if opts.filesWithMatches || opts.countOnly || opts.maxCount > 0 {
		return false
	}
	return true
}

func runFastGrep(ctx context.Context, fs client.Client, searchPath, workspace string, opts grepOptions) error {
	searchPattern := opts.patterns[0]
	if !opts.glob {
		searchPattern = literalGlobPattern(searchPattern)
	}

	matches, err := fs.Grep(ctx, searchPath, searchPattern, opts.ignoreCase)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return fmt.Errorf("path %q does not exist in workspace %q", searchPath, workspace)
		}
		return err
	}

	for _, match := range matches {
		if match.LineNum == 0 {
			fmt.Printf("%s:%s\n", match.Path, match.Line)
			continue
		}
		fmt.Printf("%s:%d:%s\n", match.Path, match.LineNum, match.Line)
	}
	return nil
}

func compileGrepMatcher(opts grepOptions) (*grepMatcher, error) {
	matcher := &grepMatcher{
		ignoreCase: opts.ignoreCase,
	}

	switch {
	case opts.glob:
		matcher.mode = grepModeGlob
		matcher.patterns = append([]string(nil), opts.patterns...)
		if opts.ignoreCase {
			matcher.lowerPatterns = make([]string, len(opts.patterns))
			for i, pattern := range opts.patterns {
				matcher.lowerPatterns[i] = strings.ToLower(pattern)
			}
		}
		return matcher, nil

	case opts.extendedRegexp || opts.basicRegexp:
		matcher.mode = grepModeRegex
		for _, pattern := range opts.patterns {
			compiled, err := compileGrepRegexp(pattern, false, opts)
			if err != nil {
				return nil, err
			}
			matcher.regexps = append(matcher.regexps, compiled)
		}
		return matcher, nil

	default:
		matcher.mode = grepModeLiteral
		if opts.wordRegexp || opts.lineRegexp {
			for _, pattern := range opts.patterns {
				compiled, err := compileGrepRegexp(pattern, true, opts)
				if err != nil {
					return nil, err
				}
				matcher.regexps = append(matcher.regexps, compiled)
			}
			return matcher, nil
		}

		matcher.patterns = append([]string(nil), opts.patterns...)
		matcher.patternBytes = make([][]byte, len(opts.patterns))
		for i, pattern := range opts.patterns {
			matcher.patternBytes[i] = []byte(pattern)
		}
		if opts.ignoreCase {
			matcher.lowerPatterns = make([]string, len(opts.patterns))
			matcher.lowerPatternData = make([][]byte, len(opts.patterns))
			for i, pattern := range opts.patterns {
				lower := strings.ToLower(pattern)
				matcher.lowerPatterns[i] = lower
				matcher.lowerPatternData[i] = []byte(lower)
			}
		}
		return matcher, nil
	}
}

func compileGrepRegexp(pattern string, literal bool, opts grepOptions) (*regexp.Regexp, error) {
	if literal {
		pattern = regexp.QuoteMeta(pattern)
	}
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
	return compiled, nil
}

func runAdvancedGrep(ctx context.Context, fs client.Client, searchPath, workspace string, opts grepOptions, matcher *grepMatcher) error {
	targets, err := collectGrepTargets(ctx, fs, searchPath)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return fmt.Errorf("path %q does not exist in workspace %q", searchPath, workspace)
		}
		return err
	}

	for _, target := range targets {
		content := target.content
		if !target.loaded {
			content, err = fs.Cat(ctx, target.path)
			if err != nil {
				return err
			}
		}
		if err := grepProcessFile(target.path, content, opts, matcher); err != nil {
			return err
		}
	}
	return nil
}

func collectGrepTargets(ctx context.Context, fs client.Client, searchPath string) ([]grepFileTarget, error) {
	tree, treeErr := fs.Tree(ctx, searchPath, grepTreeMaxDepth)
	if treeErr == nil {
		targets := make([]grepFileTarget, 0, len(tree))
		for _, entry := range tree {
			if entry.Type != "file" {
				continue
			}
			targets = append(targets, grepFileTarget{path: entry.Path})
		}
		return targets, nil
	}

	content, catErr := fs.Cat(ctx, searchPath)
	if catErr == nil {
		return []grepFileTarget{{
			path:    searchPath,
			content: content,
			loaded:  true,
		}}, nil
	}
	if errors.Is(catErr, redis.Nil) {
		return nil, redis.Nil
	}
	return nil, catErr
}

func grepProcessFile(filePath string, content []byte, opts grepOptions, matcher *grepMatcher) error {
	if grepBinaryPrefix(content) {
		return grepProcessBinaryFile(filePath, content, opts, matcher)
	}

	lines := strings.Split(string(content), "\n")
	matches := 0
	printedFile := false

	for i, line := range lines {
		selected := matcher.matchLine(line)
		if opts.invertMatch {
			selected = !selected
		}
		if !selected {
			continue
		}

		matches++
		if opts.filesWithMatches {
			if !printedFile {
				fmt.Println(filePath)
			}
			return nil
		}
		if !opts.countOnly {
			if opts.showLineNumbers {
				fmt.Printf("%s:%d:%s\n", filePath, i+1, line)
			} else {
				fmt.Printf("%s:%s\n", filePath, line)
			}
		}
		if opts.maxCount > 0 && matches >= opts.maxCount {
			break
		}
	}

	if opts.countOnly {
		fmt.Printf("%s:%d\n", filePath, matches)
	}
	return nil
}

func grepProcessBinaryFile(filePath string, content []byte, opts grepOptions, matcher *grepMatcher) error {
	selected := matcher.matchBytes(content)
	if opts.invertMatch {
		selected = false
	}
	if !selected {
		if opts.countOnly {
			fmt.Printf("%s:0\n", filePath)
		}
		return nil
	}

	if opts.filesWithMatches {
		fmt.Println(filePath)
		return nil
	}
	if opts.countOnly {
		fmt.Printf("%s:1\n", filePath)
		return nil
	}
	fmt.Printf("%s:Binary file matches\n", filePath)
	return nil
}

func grepBinaryPrefix(content []byte) bool {
	checkLen := len(content)
	if checkLen > 8192 {
		checkLen = 8192
	}
	return bytes.IndexByte(content[:checkLen], '\x00') >= 0
}

func (m *grepMatcher) matchLine(line string) bool {
	switch {
	case len(m.regexps) > 0:
		for _, re := range m.regexps {
			if re.MatchString(line) {
				return true
			}
		}
		return false

	case m.mode == grepModeGlob:
		text := line
		patterns := m.patterns
		if m.ignoreCase {
			text = strings.ToLower(text)
			patterns = m.lowerPatterns
		}
		for _, pattern := range patterns {
			if afsGlobMatch(pattern, text) {
				return true
			}
		}
		return false

	default:
		if m.ignoreCase {
			text := strings.ToLower(line)
			for _, pattern := range m.lowerPatterns {
				if strings.Contains(text, pattern) {
					return true
				}
			}
			return false
		}
		for _, pattern := range m.patterns {
			if strings.Contains(line, pattern) {
				return true
			}
		}
		return false
	}
}

func (m *grepMatcher) matchBytes(content []byte) bool {
	switch {
	case len(m.regexps) > 0:
		for _, re := range m.regexps {
			if re.Match(content) {
				return true
			}
		}
		return false

	case m.mode == grepModeGlob:
		text := string(content)
		patterns := m.patterns
		if m.ignoreCase {
			text = strings.ToLower(text)
			patterns = m.lowerPatterns
		}
		for _, pattern := range patterns {
			if afsGlobMatch(pattern, text) {
				return true
			}
		}
		return false

	default:
		if m.ignoreCase {
			lower := bytes.ToLower(content)
			for _, pattern := range m.lowerPatternData {
				if bytes.Contains(lower, pattern) {
					return true
				}
			}
			return false
		}
		for _, pattern := range m.patternBytes {
			if bytes.Contains(content, pattern) {
				return true
			}
		}
		return false
	}
}

func resolveWorkspaceFSKey(ctx context.Context, cfg config, store *afsStore, workspace string) (string, bool, error) {
	service := controlPlaneServiceFromStore(cfg, store)
	detail, err := service.GetWorkspace(ctx, workspace)
	if err != nil {
		return "", false, err
	}

	for _, candidate := range grepNamespaceCandidates(workspace, detail.RedisKey) {
		exists, err := filesystemNamespaceExists(ctx, store.rdb, candidate)
		if err != nil {
			return "", false, err
		}
		if exists {
			return candidate, true, nil
		}
	}

	return defaultWorkspaceFSKey(workspace, detail.RedisKey), false, nil
}

func grepNamespaceCandidates(workspace, redisKey string) []string {
	raw := strings.TrimSpace(redisKey)
	trimmed := strings.TrimPrefix(raw, "afs:")
	return uniqueNonEmpty(
		trimmed,
		workspace,
		raw,
		mountRedisKeyForWorkspace(workspace),
	)
}

func defaultWorkspaceFSKey(workspace, redisKey string) string {
	candidates := grepNamespaceCandidates(workspace, redisKey)
	if len(candidates) == 0 {
		return workspace
	}
	return candidates[0]
}

func filesystemNamespaceExists(ctx context.Context, rdb *redis.Client, fsKey string) (bool, error) {
	if strings.TrimSpace(fsKey) == "" {
		return false, nil
	}
	rootKey := fmt.Sprintf("afs:{%s}:inode:1", fsKey)
	infoKey := fmt.Sprintf("afs:{%s}:info", fsKey)
	n, err := rdb.Exists(ctx, rootKey, infoKey).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func literalGlobPattern(pattern string) string {
	return "*" + escapeGlobLiteral(pattern) + "*"
}

func escapeGlobLiteral(pattern string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"*", "\\*",
		"?", "\\?",
		"[", "\\[",
		"]", "\\]",
	)
	return replacer.Replace(pattern)
}

func normalizeAFSGrepPath(p string) string {
	if strings.TrimSpace(p) == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	clean := path.Clean(p)
	if clean == "." {
		return "/"
	}
	return clean
}

func uniqueNonEmpty(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
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

func afsGlobMatch(pattern, str string) bool {
	px, sx := 0, 0
	starPx, starSx := -1, -1

	for sx < len(str) {
		if px < len(pattern) {
			switch pattern[px] {
			case '*':
				starPx = px
				starSx = sx
				px++
				continue
			case '?':
				px++
				sx++
				continue
			case '[':
				if matched, newPx := afsMatchClass(pattern, px, str[sx]); matched {
					px = newPx
					sx++
					continue
				}
			case '\\':
				px++
				if px < len(pattern) && pattern[px] == str[sx] {
					px++
					sx++
					continue
				}
			default:
				if pattern[px] == str[sx] {
					px++
					sx++
					continue
				}
			}
		}
		if starPx >= 0 {
			px = starPx + 1
			starSx++
			sx = starSx
			continue
		}
		return false
	}

	for px < len(pattern) && pattern[px] == '*' {
		px++
	}
	return px == len(pattern)
}

func afsMatchClass(pattern string, px int, ch byte) (bool, int) {
	if px >= len(pattern) || pattern[px] != '[' {
		return false, px
	}
	px++

	negate := false
	if px < len(pattern) && (pattern[px] == '!' || pattern[px] == '^') {
		negate = true
		px++
	}

	matched := false
	first := true
	for px < len(pattern) {
		if pattern[px] == ']' && !first {
			px++
			if negate {
				return !matched, px
			}
			return matched, px
		}
		first = false

		c := pattern[px]
		if c == '\\' && px+1 < len(pattern) {
			px++
			c = pattern[px]
		}
		px++

		if px+1 < len(pattern) && pattern[px] == '-' && pattern[px+1] != ']' {
			px++
			d := pattern[px]
			if d == '\\' && px+1 < len(pattern) {
				px++
				d = pattern[px]
			}
			px++

			lo, hi := c, d
			if lo > hi {
				lo, hi = hi, lo
			}
			if ch >= lo && ch <= hi {
				matched = true
			}
		} else if ch == c {
			matched = true
		}
	}

	return false, px
}
