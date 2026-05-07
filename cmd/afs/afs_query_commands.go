package main

import (
	"context"
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/internal/mcptools"
)

type workspaceQueryOptions struct {
	mode           string
	path           string
	limit          int
	all            bool
	minScore       float64
	jsonOut        bool
	filesOnly      bool
	pathsOnly      bool
	markdown       bool
	csvOut         bool
	xmlOut         bool
	full           bool
	lineNumbers    bool
	explain        bool
	candidateLimit int
	noRerank       bool
	keywordOnly    bool
	semanticOnly   bool
	intent         string
	chunkStrategy  string
	query          string
	document       mcptools.FileQueryDocument
}

func cmdQuery(args []string) error {
	return cmdWorkspaceQuery(mcptools.FileQueryModeHybrid, "", args)
}

func cmdFSQuery(workspace string, args []string) error {
	return cmdWorkspaceQuery(mcptools.FileQueryModeHybrid, workspace, args)
}

func cmdWorkspaceQuery(mode, workspace string, args []string) error {
	if len(args) < 1 || args[0] != mode {
		return runWorkspaceQuery(mode, workspace, args)
	}
	return runWorkspaceQuery(mode, workspace, args[1:])
}

func runWorkspaceQuery(mode, workspace string, args []string) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, workspaceQueryUsageText(filepath.Base(os.Args[0]), mode))
		return nil
	}
	if mode == mcptools.FileQueryModeHybrid && isWorkspaceQueryIndexInvocation(args) {
		return runWorkspaceQueryIndex(workspace, args[1:])
	}
	opts, err := parseWorkspaceQueryArgs(mode, args)
	if err != nil {
		return err
	}

	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	defer remote.close()

	cfg, err := workspaceQueryConfig(ctx, remote.controlPlane, remote.selection)
	if err != nil {
		return err
	}
	request := workspaceQueryRequest(remote.selection, opts)
	if opts.mode == mcptools.FileQueryModeKeyword || opts.mode == mcptools.FileQueryModeHybrid {
		return runWorkspaceKeywordQuery(ctx, remote, opts, request, cfg)
	}
	message := workspaceQueryUnavailableMessage(opts, remote.selection.Name, cfg)
	if opts.jsonOut {
		return encodeWorkspaceQueryUnavailable(request, message)
	}
	return errors.New(message)
}

func isWorkspaceQueryIndexInvocation(args []string) bool {
	if len(args) == 0 || args[0] != "index" {
		return false
	}
	if len(args) == 1 || isHelpArg(args[1]) {
		return true
	}
	switch strings.TrimSpace(args[1]) {
	case "status", "rebuild", "clean":
		return true
	default:
		return false
	}
}

func workspaceQueryConfig(ctx context.Context, service afsControlPlane, selection workspaceSelection) (controlplane.WorkspaceConfig, error) {
	cfg, err := service.GetWorkspaceConfig(ctx, selection.ID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return controlplane.DefaultWorkspaceConfig(), nil
		}
		return controlplane.WorkspaceConfig{}, err
	}
	return cfg, nil
}

func parseWorkspaceQueryArgs(mode string, args []string) (workspaceQueryOptions, error) {
	opts := workspaceQueryOptions{
		mode:  mode,
		path:  "/",
		limit: 10,
	}
	fs := flag.NewFlagSet(mode, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.path, "path", "/", "workspace path scope")
	fs.IntVar(&opts.limit, "limit", 10, "maximum results")
	fs.IntVar(&opts.limit, "n", 10, "maximum results")
	fs.BoolVar(&opts.all, "all", false, "return all results")
	fs.Float64Var(&opts.minScore, "min-score", 0, "minimum score")
	fs.BoolVar(&opts.jsonOut, "json", false, "write JSON output")
	fs.BoolVar(&opts.filesOnly, "files", false, "write QMD-style file result lines")
	fs.BoolVar(&opts.pathsOnly, "paths", false, "show only matching workspace paths")
	fs.BoolVar(&opts.markdown, "md", false, "write Markdown output")
	fs.BoolVar(&opts.csvOut, "csv", false, "write CSV output")
	fs.BoolVar(&opts.xmlOut, "xml", false, "write XML output")
	fs.BoolVar(&opts.full, "full", false, "include full content")
	fs.BoolVar(&opts.lineNumbers, "line-numbers", false, "include line numbers")
	fs.BoolVar(&opts.explain, "explain", false, "include retrieval explanation")
	fs.IntVar(&opts.candidateLimit, "candidate-limit", 0, "candidate result limit")
	fs.BoolVar(&opts.noRerank, "no-rerank", false, "disable reranking")
	fs.BoolVar(&opts.keywordOnly, "keyword", false, "use BM25 keyword search only")
	fs.BoolVar(&opts.semanticOnly, "semantic", false, "use vector semantic search only")
	fs.StringVar(&opts.intent, "intent", "", "query intent")
	fs.StringVar(&opts.chunkStrategy, "chunk-strategy", "", "chunk strategy")
	flagArgs, queryArgs := splitWorkspaceQueryArgs(args)
	if err := fs.Parse(flagArgs); err != nil {
		return opts, fmt.Errorf("%s", workspaceQueryUsageText(filepath.Base(os.Args[0]), mode))
	}
	opts.path = normalizeFSRemotePath(opts.path)
	opts.intent = strings.TrimSpace(opts.intent)
	opts.chunkStrategy = strings.TrimSpace(strings.ToLower(opts.chunkStrategy))
	switch opts.chunkStrategy {
	case "", controlplane.WorkspaceQueryChunkStrategyAuto, controlplane.WorkspaceQueryChunkStrategyRegex:
	default:
		return opts, fmt.Errorf("unsupported chunk strategy %q", opts.chunkStrategy)
	}
	if opts.limit < 0 {
		return opts, fmt.Errorf("limit must be non-negative")
	}
	if opts.candidateLimit < 0 {
		return opts, fmt.Errorf("candidate-limit must be non-negative")
	}
	if opts.minScore < 0 {
		return opts, fmt.Errorf("min-score must be non-negative")
	}
	if opts.keywordOnly && opts.semanticOnly {
		return opts, fmt.Errorf("--keyword and --semantic are mutually exclusive")
	}
	if workspaceQueryFormatCount(opts) > 1 {
		return opts, fmt.Errorf("choose only one output format: --json, --files, --paths, --md, --csv, or --xml")
	}
	switch {
	case opts.keywordOnly:
		opts.mode = mcptools.FileQueryModeKeyword
	case opts.semanticOnly:
		opts.mode = mcptools.FileQueryModeSemantic
	default:
		opts.mode = mode
	}
	opts.query = strings.TrimSpace(strings.Join(queryArgs, " "))
	if opts.query == "" {
		return opts, fmt.Errorf("%s", workspaceQueryUsageText(filepath.Base(os.Args[0]), mode))
	}
	doc, err := mcptools.ParseFileQueryDocument(opts.query)
	if err != nil {
		return opts, err
	}
	if opts.intent != "" && doc.Intent != "" {
		return opts, fmt.Errorf("--intent cannot be combined with an intent: typed query clause")
	}
	if opts.intent != "" {
		doc.Intent = opts.intent
	}
	if opts.mode != mcptools.FileQueryModeHybrid && (doc.Typed || workspaceQueryIsExplicitExpand(opts.query)) {
		return opts, fmt.Errorf("--keyword and --semantic accept plain search text only; use %s query for QMD-style typed query documents", filepath.Base(os.Args[0]))
	}
	opts.document = doc
	return opts, nil
}

func workspaceQueryFormatCount(opts workspaceQueryOptions) int {
	count := 0
	for _, enabled := range []bool{opts.jsonOut, opts.filesOnly, opts.pathsOnly, opts.markdown, opts.csvOut, opts.xmlOut} {
		if enabled {
			count++
		}
	}
	return count
}

func splitWorkspaceQueryArgs(args []string) ([]string, []string) {
	flagArgs := make([]string, 0, len(args))
	queryArgs := make([]string, 0, len(args))
	queryStarted := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			queryArgs = append(queryArgs, args[i+1:]...)
			break
		}
		kind, ok := workspaceQueryFlagKind(arg)
		if ok {
			flagArgs = append(flagArgs, arg)
			if kind == "value" && !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && !queryStarted {
			flagArgs = append(flagArgs, arg)
			continue
		}
		queryStarted = true
		queryArgs = append(queryArgs, arg)
	}
	return flagArgs, queryArgs
}

func workspaceQueryFlagKind(arg string) (string, bool) {
	if arg == "" || arg == "-" || !strings.HasPrefix(arg, "-") {
		return "", false
	}
	name := strings.TrimLeft(arg, "-")
	if name == "" {
		return "", false
	}
	if before, _, ok := strings.Cut(name, "="); ok {
		name = before
	}
	switch name {
	case "all", "json", "files", "paths", "md", "csv", "xml", "full", "line-numbers", "explain", "no-rerank", "keyword", "semantic":
		return "bool", true
	case "path", "limit", "n", "min-score", "candidate-limit", "intent", "chunk-strategy":
		return "value", true
	default:
		return "", false
	}
}

func workspaceQueryRequest(selection workspaceSelection, opts workspaceQueryOptions) mcptools.FileQueryRequest {
	request := mcptools.FileQueryRequest{
		Workspace:      selection.Name,
		Path:           opts.path,
		Mode:           opts.mode,
		Limit:          opts.limit,
		All:            opts.all,
		MinScore:       opts.minScore,
		Full:           opts.full,
		CandidateLimit: opts.candidateLimit,
		Explain:        opts.explain,
		ChunkStrategy:  opts.chunkStrategy,
	}
	if opts.noRerank {
		request.Rerank = "none"
	} else {
		request.Rerank = "auto"
	}
	if opts.document.Typed {
		request.Searches = append([]mcptools.FileQuerySearch(nil), opts.document.Searches...)
		request.Intent = opts.document.Intent
	} else {
		request.Query = opts.document.Query
		request.Intent = opts.document.Intent
	}
	return request
}

func runWorkspaceKeywordQuery(ctx context.Context, remote *fsRemoteWorkspace, opts workspaceQueryOptions, request mcptools.FileQueryRequest, cfg controlplane.WorkspaceConfig) error {
	response, err := remote.controlPlane.QueryWorkspace(ctx, remote.selection.ID, request)
	if err != nil {
		return err
	}
	if !opts.jsonOut && opts.mode == mcptools.FileQueryModeHybrid && !cfg.Query.Embeddings.Enabled && !workspaceQueryHasSemanticClauses(request.Searches) {
		fmt.Fprintln(os.Stderr, "Warning: Embeddings disabled; using keyword retrieval.")
	}
	return writeWorkspaceQueryResponse(response, opts)
}

func workspaceQueryHasSemanticClauses(searches []mcptools.FileQuerySearch) bool {
	for _, search := range searches {
		switch search.Type {
		case mcptools.FileQuerySearchVec, mcptools.FileQuerySearchHyde:
			return true
		}
	}
	return false
}

func encodeWorkspaceQueryUnavailable(request mcptools.FileQueryRequest, message string) error {
	response := mcptools.FileQueryResponse{
		Status:    mcptools.FileQueryStatusUnavailable,
		Workspace: request.Workspace,
		Path:      request.Path,
		Query:     request.Query,
		Searches:  request.Searches,
		Intent:    request.Intent,
		Results:   []mcptools.FileQueryResult{},
		Warnings:  []string{message},
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(response)
}

func writeWorkspaceQueryResponse(response mcptools.FileQueryResponse, opts workspaceQueryOptions) error {
	if opts.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	}
	for _, warning := range response.Warnings {
		if strings.TrimSpace(warning) != "" {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
		}
	}
	if len(response.Results) == 0 {
		switch {
		case opts.filesOnly || opts.pathsOnly || opts.markdown:
			return nil
		case opts.csvOut:
			return writeWorkspaceQueryCSV(response, nil, opts)
		case opts.xmlOut:
			fmt.Fprintln(os.Stdout, "<results></results>")
			return nil
		}
		fmt.Fprintln(os.Stdout, "No results found.")
		return nil
	}
	results := coalesceWorkspaceQueryResults(response.Results)
	if opts.pathsOnly {
		seen := make(map[string]struct{}, len(results))
		for _, result := range results {
			if _, ok := seen[result.Path]; ok {
				continue
			}
			seen[result.Path] = struct{}{}
			fmt.Fprintln(os.Stdout, result.Path)
		}
		return nil
	}
	if opts.filesOnly {
		return writeWorkspaceQueryFiles(response, results)
	}
	if opts.csvOut {
		return writeWorkspaceQueryCSV(response, results, opts)
	}
	if opts.xmlOut {
		return writeWorkspaceQueryXML(response, results, opts)
	}
	if opts.markdown {
		return writeWorkspaceQueryMarkdown(response, results, opts)
	}
	for i, result := range results {
		fmt.Fprintf(os.Stdout, "%s", workspaceQueryResultURI(response.Workspace, result))
		if docID := workspaceQueryResultDocID(result); docID != "" {
			fmt.Fprintf(os.Stdout, "  #%s", docID)
		}
		fmt.Fprintln(os.Stdout)
		fmt.Fprintf(os.Stdout, "Score: %s", workspaceQueryScoreLabel(result.Score))
		if source := workspaceQueryResultSource(result); source != "" {
			fmt.Fprintf(os.Stdout, "  Source: %s", source)
		}
		fmt.Fprintln(os.Stdout)
		if strings.TrimSpace(result.Snippet) != "" {
			fmt.Fprintln(os.Stdout)
			fmt.Fprintln(os.Stdout, workspaceQuerySnippetHeader(result))
			writeWorkspaceQuerySnippet(os.Stdout, result, opts)
		}
		if i < len(results)-1 {
			fmt.Fprintln(os.Stdout)
			fmt.Fprintln(os.Stdout)
		}
	}
	return nil
}

func writeWorkspaceQueryFiles(response mcptools.FileQueryResponse, results []mcptools.FileQueryResult) error {
	for _, result := range results {
		docID := workspaceQueryResultDocID(result)
		fmt.Fprintf(os.Stdout, "#%s,%s,%s\n", docID, workspaceQueryScoreNumber(result.Score), workspaceQueryResultURI(response.Workspace, result))
	}
	return nil
}

func writeWorkspaceQueryCSV(response mcptools.FileQueryResponse, results []mcptools.FileQueryResult, opts workspaceQueryOptions) error {
	writer := csv.NewWriter(os.Stdout)
	if err := writer.Write([]string{"docid", "score", "file", "title", "context", "line", "snippet"}); err != nil {
		return err
	}
	for _, result := range results {
		snippet := strings.TrimRight(result.Snippet, "\n")
		if opts.lineNumbers {
			snippet = workspaceQuerySnippetString(result, opts)
		}
		record := []string{
			"#" + workspaceQueryResultDocID(result),
			workspaceQueryScoreNumber(result.Score),
			workspaceQueryResultURI(response.Workspace, result),
			path.Base(result.Path),
			workspaceQueryResultSource(result),
			workspaceQueryLineLabel(result),
			snippet,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func writeWorkspaceQueryXML(response mcptools.FileQueryResponse, results []mcptools.FileQueryResult, opts workspaceQueryOptions) error {
	fmt.Fprintln(os.Stdout, "<results>")
	for _, result := range results {
		snippet := strings.TrimRight(result.Snippet, "\n")
		if opts.lineNumbers {
			snippet = workspaceQuerySnippetString(result, opts)
		}
		fmt.Fprintf(os.Stdout, "  <file docid=%q name=%q score=%q line=%q", "#"+workspaceQueryResultDocID(result), workspaceQueryResultURI(response.Workspace, result), workspaceQueryScoreNumber(result.Score), workspaceQueryLineLabel(result))
		if source := workspaceQueryResultSource(result); source != "" {
			fmt.Fprintf(os.Stdout, " source=%q", source)
		}
		fmt.Fprintln(os.Stdout, ">")
		fmt.Fprintln(os.Stdout, escapeXML(snippet))
		fmt.Fprintln(os.Stdout, "  </file>")
	}
	fmt.Fprintln(os.Stdout, "</results>")
	return nil
}

func writeWorkspaceQueryMarkdown(response mcptools.FileQueryResponse, results []mcptools.FileQueryResult, opts workspaceQueryOptions) error {
	for _, result := range results {
		location := workspaceQueryResultLocation(result, opts)
		fmt.Fprintf(os.Stdout, "---\n# %s\n", location)
		if docID := workspaceQueryResultDocID(result); docID != "" {
			fmt.Fprintf(os.Stdout, "**docid:** `#%s`\n", docID)
		}
		fmt.Fprintf(os.Stdout, "**score:** %s\n", workspaceQueryScoreLabel(result.Score))
		if source := workspaceQueryResultSource(result); source != "" {
			fmt.Fprintf(os.Stdout, "**source:** %s\n", source)
		}
		fmt.Fprintf(os.Stdout, "**file:** `%s`\n", workspaceQueryResultURI(response.Workspace, result))
		if strings.TrimSpace(result.Snippet) != "" {
			fmt.Fprintln(os.Stdout)
			fmt.Fprintln(os.Stdout, workspaceQuerySnippetHeader(result))
			writeWorkspaceQuerySnippet(os.Stdout, result, opts)
		}
		fmt.Fprintln(os.Stdout)
	}
	return nil
}

func coalesceWorkspaceQueryResults(results []mcptools.FileQueryResult) []mcptools.FileQueryResult {
	out := make([]mcptools.FileQueryResult, 0, len(results))
	for _, result := range results {
		merged := false
		for i := range out {
			if !workspaceQueryResultsOverlap(out[i], result) {
				continue
			}
			out[i] = mergeWorkspaceQueryResults(out[i], result)
			merged = true
			break
		}
		if !merged {
			out = append(out, result)
		}
	}
	return out
}

func workspaceQueryResultsOverlap(a, b mcptools.FileQueryResult) bool {
	if a.Path == "" || a.Path != b.Path || a.StartLine <= 0 || b.StartLine <= 0 {
		return false
	}
	aStart, aEnd := workspaceQueryLineRange(a)
	bStart, bEnd := workspaceQueryLineRange(b)
	return bStart <= aEnd+1 && aStart <= bEnd+1
}

func mergeWorkspaceQueryResults(a, b mcptools.FileQueryResult) mcptools.FileQueryResult {
	aStart, aEnd := workspaceQueryLineRange(a)
	bStart, bEnd := workspaceQueryLineRange(b)
	if bStart < aStart {
		a.StartLine = bStart
	}
	if bEnd > aEnd {
		a.EndLine = bEnd
	}
	if b.Score > a.Score {
		a.Score = b.Score
	}
	a.Snippet = mergeWorkspaceQuerySnippets(a.Snippet, b.Snippet)
	a.SearchTypes = mergeWorkspaceQuerySearchTypes(a.SearchTypes, b.SearchTypes)
	return a
}

func workspaceQueryLineRange(result mcptools.FileQueryResult) (int, int) {
	start := result.StartLine
	end := result.EndLine
	if end < start {
		end = start
	}
	return start, end
}

func mergeWorkspaceQuerySnippets(a, b string) string {
	a = strings.TrimRight(a, "\n")
	b = strings.TrimRight(b, "\n")
	switch {
	case strings.TrimSpace(a) == "":
		return b
	case strings.TrimSpace(b) == "":
		return a
	case strings.Contains(a, b):
		return a
	case strings.Contains(b, a):
		return b
	default:
		return a + "\n...\n" + b
	}
}

func mergeWorkspaceQuerySearchTypes(a, b []string) []string {
	if len(a) == 0 {
		return append([]string(nil), b...)
	}
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, value := range append(a, b...) {
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

func workspaceQueryResultLocation(result mcptools.FileQueryResult, opts workspaceQueryOptions) string {
	location := result.Path
	if result.StartLine > 0 {
		if result.EndLine > result.StartLine {
			location = fmt.Sprintf("%s:%d-%d", result.Path, result.StartLine, result.EndLine)
		} else {
			location = fmt.Sprintf("%s:%d", result.Path, result.StartLine)
		}
	}
	return location
}

func workspaceQueryLineLabel(result mcptools.FileQueryResult) string {
	if result.StartLine <= 0 {
		return ""
	}
	if result.EndLine > result.StartLine {
		return fmt.Sprintf("%d-%d", result.StartLine, result.EndLine)
	}
	return fmt.Sprintf("%d", result.StartLine)
}

func workspaceQueryScoreLabel(score float64) string {
	if score <= 0 || math.IsNaN(score) || math.IsInf(score, 0) {
		return "0%"
	}
	if score <= 1 {
		return fmt.Sprintf("%d%%", int(math.Round(score*100)))
	}
	return fmt.Sprintf("%.2f", score)
}

func workspaceQueryResultSource(result mcptools.FileQueryResult) string {
	if len(result.SearchTypes) > 0 {
		return strings.Join(result.SearchTypes, "+")
	}
	if backend, ok := result.Metadata["backend"].(string); ok {
		return backend
	}
	return ""
}

func workspaceQueryResultURI(workspace string, result mcptools.FileQueryResult) string {
	workspace = strings.Trim(strings.TrimSpace(workspace), "/")
	if workspace == "" {
		workspace = "workspace"
	}
	remotePath := normalizeFSRemotePath(result.Path)
	uri := "afs://" + workspace + remotePath
	if result.StartLine > 0 {
		if result.EndLine > result.StartLine {
			uri += fmt.Sprintf(":%d-%d", result.StartLine, result.EndLine)
		} else {
			uri += fmt.Sprintf(":%d", result.StartLine)
		}
	}
	return uri
}

func workspaceQueryResultDocID(result mcptools.FileQueryResult) string {
	if id := strings.TrimSpace(result.ChunkID); id != "" {
		return shortHash(id)
	}
	sum := sha1.Sum([]byte(fmt.Sprintf("%s:%d:%d:%s", result.Path, result.StartLine, result.EndLine, result.Snippet)))
	return hex.EncodeToString(sum[:])[:6]
}

func shortHash(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:6]
}

func workspaceQueryScoreNumber(score float64) string {
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return "0.00"
	}
	return fmt.Sprintf("%.2f", score)
}

func workspaceQuerySnippetHeader(result mcptools.FileQueryResult) string {
	start, end := workspaceQuerySnippetLineRange(result)
	if start <= 0 {
		return "@@ -1,1 @@"
	}
	count := max(1, end-start+1)
	return fmt.Sprintf("@@ -%d,%d @@", start, count)
}

func workspaceQuerySnippetString(result mcptools.FileQueryResult, opts workspaceQueryOptions) string {
	var b strings.Builder
	writeWorkspaceQuerySnippet(&b, result, opts)
	return strings.TrimRight(b.String(), "\n")
}

func writeWorkspaceQuerySnippet(w io.Writer, result mcptools.FileQueryResult, opts workspaceQueryOptions) {
	snippet := strings.TrimRight(result.Snippet, "\n")
	if strings.TrimSpace(snippet) == "" {
		return
	}
	lines := strings.Split(snippet, "\n")
	width := 0
	startLine, endLine := workspaceQuerySnippetLineRange(result)
	if opts.lineNumbers && startLine > 0 {
		width = len(fmt.Sprintf("%d", max(startLine+len(lines)-1, endLine)))
	}
	for i, line := range lines {
		if width > 0 {
			if endLine > startLine {
				fmt.Fprintf(w, "%*d: %s\n", width, startLine+i, line)
			} else if i == 0 {
				fmt.Fprintf(w, "%*d: %s\n", width, startLine, line)
			} else {
				fmt.Fprintf(w, "%*s  %s\n", width, "", line)
			}
			continue
		}
		fmt.Fprintf(w, "%s\n", line)
	}
}

func escapeXML(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	return value
}

func workspaceQuerySnippetLineRange(result mcptools.FileQueryResult) (int, int) {
	start := result.StartLine
	end := result.EndLine
	if value, ok := workspaceQueryMetadataInt(result.Metadata, "snippet_start_line"); ok {
		start = value
	}
	if value, ok := workspaceQueryMetadataInt(result.Metadata, "snippet_end_line"); ok {
		end = value
	}
	if end < start {
		end = start
	}
	return start, end
}

func workspaceQueryMetadataInt(metadata map[string]any, key string) (int, bool) {
	if metadata == nil {
		return 0, false
	}
	switch value := metadata[key].(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	default:
		return 0, false
	}
}

func workspaceQueryUnavailableMessage(opts workspaceQueryOptions, workspace string, cfg controlplane.WorkspaceConfig) string {
	switch opts.mode {
	case mcptools.FileQueryModeKeyword:
		return fmt.Sprintf("keyword query is not ready yet for workspace %q\nIt will use BM25 ranking through RedisSearch. Until then, use '%s grep <pattern>' for exact line matches.", workspace, filepath.Base(os.Args[0]))
	case mcptools.FileQueryModeSemantic:
		if !cfg.Query.Embeddings.Enabled {
			return fmt.Sprintf("semantic query is disabled for workspace %q\nEnable it with: %s ws config %s set query.embeddings.enabled true", workspace, filepath.Base(os.Args[0]), workspace)
		}
		return fmt.Sprintf("semantic query is not ready yet for workspace %q\nThe vector backend will land in the next retrieval slice.", workspace)
	default:
		if !cfg.Query.Embeddings.Enabled {
			return fmt.Sprintf("workspace query is not ready yet for workspace %q\nPlain query will use hybrid ranking and fall back to BM25 keywords when embeddings are disabled.\nUntil then, use '%s grep <pattern>' for exact line matches. Use '%s query --semantic <query>' when you specifically want vector-only search.", workspace, filepath.Base(os.Args[0]), filepath.Base(os.Args[0]))
		}
		return fmt.Sprintf("workspace query is not ready yet for workspace %q\nThe QMD-style hybrid query backend will land in the next retrieval slice.", workspace)
	}
}

func workspaceQueryIsExplicitExpand(query string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(query)), "expand:")
}

func workspaceQueryUsageText(bin, mode string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s query [flags] <query>
  %s fs [workspace] query [flags] <query>
  %s query index <status|rebuild|clean> [flags]

QMD-style hybrid + rerank workspace query.
Plain text runs hybrid retrieval by default. Use --keyword for keyword-ranked
retrieval only, or --semantic for vector-only semantic search.

If embeddings are disabled, default query falls back to keyword ranked results.
Use grep when you know the exact text.

Typed query documents:
  lex: lexical terms
  vec: semantic terms (uses embeddings)
  hyde: hypothetical answer text (uses embeddings)
  intent: extra search intent

Flags:
  --path <path>             Scope search to a workspace path
  -n, --limit <num>         Maximum results
  --all                     Return all results
  --min-score <num>         Minimum score
  --json                    Write JSON output
  --files                   Write QMD-style #id,score,afs://path lines
  --paths                   Show only matching workspace paths
  --csv                     Write CSV output
  --md                      Write Markdown output
  --xml                     Write XML output
  --full                    Include full content
  --line-numbers            Include line numbers
  --explain                 Include retrieval explanation
  --candidate-limit <num>   Candidate result limit
  --no-rerank               Disable reranking
  --keyword                 Keyword-ranked search only
  --semantic                Vector semantic search only
  --intent <text>           Search intent
  --chunk-strategy <name>   Chunk strategy: auto or regex

Examples:
  %s query "how do checkpoints work?"
  %s query --keyword "checkpoint savepoint"
  %s query --semantic "how do I save a snapshot?"
  %s query index status
  %s fs repo query $'lex: checkpoint\nvec: how do I save a snapshot?'
`, bin, bin, bin, bin, bin, bin, bin, bin)
}
