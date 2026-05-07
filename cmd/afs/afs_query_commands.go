package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
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
	markdown       bool
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
	if mode == mcptools.FileQueryModeHybrid && args[0] == "index" {
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
		mode:        mode,
		path:        "/",
		limit:       10,
		lineNumbers: true,
	}
	fs := flag.NewFlagSet(mode, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.path, "path", "/", "workspace path scope")
	fs.IntVar(&opts.limit, "limit", 10, "maximum results")
	fs.IntVar(&opts.limit, "n", 10, "maximum results")
	fs.BoolVar(&opts.all, "all", false, "return all results")
	fs.Float64Var(&opts.minScore, "min-score", 0, "minimum score")
	fs.BoolVar(&opts.jsonOut, "json", false, "write JSON output")
	fs.BoolVar(&opts.filesOnly, "files", false, "show only matching files")
	fs.BoolVar(&opts.markdown, "md", false, "write Markdown output")
	fs.BoolVar(&opts.full, "full", false, "include full content")
	fs.BoolVar(&opts.lineNumbers, "line-numbers", true, "include line numbers")
	fs.BoolVar(&opts.explain, "explain", false, "include retrieval explanation")
	fs.IntVar(&opts.candidateLimit, "candidate-limit", 0, "candidate result limit")
	fs.BoolVar(&opts.noRerank, "no-rerank", false, "disable reranking")
	fs.BoolVar(&opts.keywordOnly, "keyword", false, "use BM25 keyword search only")
	fs.BoolVar(&opts.semanticOnly, "semantic", false, "use vector semantic search only")
	fs.StringVar(&opts.intent, "intent", "", "query intent")
	fs.StringVar(&opts.chunkStrategy, "chunk-strategy", "", "chunk strategy")
	if err := fs.Parse(args); err != nil {
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
	switch {
	case opts.keywordOnly:
		opts.mode = mcptools.FileQueryModeKeyword
	case opts.semanticOnly:
		opts.mode = mcptools.FileQueryModeSemantic
	default:
		opts.mode = mode
	}
	opts.query = strings.TrimSpace(strings.Join(fs.Args(), " "))
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
	_ = cfg
	response, err := remote.controlPlane.QueryWorkspace(ctx, remote.selection.ID, request)
	if err != nil {
		return err
	}
	return writeWorkspaceQueryResponse(response, opts)
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
		fmt.Fprintln(os.Stdout, "No query results")
		return nil
	}
	if opts.filesOnly {
		seen := make(map[string]struct{}, len(response.Results))
		for _, result := range response.Results {
			if _, ok := seen[result.Path]; ok {
				continue
			}
			seen[result.Path] = struct{}{}
			fmt.Fprintln(os.Stdout, result.Path)
		}
		return nil
	}
	if opts.markdown {
		for _, result := range response.Results {
			fmt.Fprintf(os.Stdout, "- `%s` (score %.2f)\n", workspaceQueryResultLocation(result, opts), result.Score)
			if snippet := strings.TrimSpace(result.Snippet); snippet != "" {
				fmt.Fprintf(os.Stdout, "  %s\n", snippet)
			}
		}
		return nil
	}
	for i, result := range response.Results {
		location := workspaceQueryResultLocation(result, opts)
		fmt.Fprintf(os.Stdout, "#%d %s", i+1, location)
		if result.Score > 0 {
			fmt.Fprintf(os.Stdout, "  score %.2f", result.Score)
		}
		fmt.Fprintln(os.Stdout)
		writeIndentedQuerySnippet(os.Stdout, result.Snippet)
		if i < len(response.Results)-1 {
			fmt.Fprintln(os.Stdout)
		}
	}
	return nil
}

func workspaceQueryResultLocation(result mcptools.FileQueryResult, opts workspaceQueryOptions) string {
	location := result.Path
	if opts.lineNumbers && result.StartLine > 0 {
		if result.EndLine > result.StartLine {
			location = fmt.Sprintf("%s:%d-%d", result.Path, result.StartLine, result.EndLine)
		} else {
			location = fmt.Sprintf("%s:%d", result.Path, result.StartLine)
		}
	}
	return location
}

func writeIndentedQuerySnippet(w io.Writer, snippet string) {
	snippet = strings.TrimRight(snippet, "\n")
	if strings.TrimSpace(snippet) == "" {
		return
	}
	for _, line := range strings.Split(snippet, "\n") {
		fmt.Fprintf(w, "  %s\n", line)
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
  --files                   Show only files
  --md                      Write Markdown output
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
