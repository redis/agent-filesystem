package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

const defaultFSMultiGetMaxBytes = 10 << 10

type fsGetOptions struct {
	fromLine    int
	limit       int
	lineNumbers bool
}

type fsMultiGetOptions struct {
	limit       int
	maxBytes    int64
	jsonOut     bool
	csvOut      bool
	markdown    bool
	xmlOut      bool
	filesOnly   bool
	lineNumbers bool
	pattern     string
}

type fsMultiGetResult struct {
	File       string `json:"file"`
	Title      string `json:"title"`
	Body       string `json:"body,omitempty"`
	Skipped    bool   `json:"skipped,omitempty"`
	SkipReason string `json:"reason,omitempty"`
}

type fsRemoteWorkspace struct {
	cfg          config
	close        func()
	selection    workspaceSelection
	controlPlane afsControlPlane
}

func openFSRemoteWorkspace(ctx context.Context, workspace string) (*fsRemoteWorkspace, error) {
	cfg, service, closeFn, err := openAFSControlPlane(ctx)
	if err != nil {
		return nil, err
	}
	selection, err := resolveFSWorkspaceSelection(ctx, cfg, service, workspace)
	if err != nil {
		closeFn()
		return nil, err
	}
	return &fsRemoteWorkspace{
		cfg:          cfg,
		close:        closeFn,
		selection:    selection,
		controlPlane: service,
	}, nil
}

func resolveFSWorkspaceSelection(ctx context.Context, cfg config, service afsControlPlane, workspace string) (workspaceSelection, error) {
	return resolveCommandWorkspaceSelectionFromControlPlane(ctx, cfg, service, workspace)
}

func cmdFSList(workspace string, args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, fsListUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	var jsonOut bool
	var filesOnly bool
	fs := flag.NewFlagSet("fs ls", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&jsonOut, "json", false, "write JSON output")
	fs.BoolVar(&filesOnly, "files", false, "show only workspace paths")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s", fsListUsageText(filepath.Base(os.Args[0])))
	}
	if jsonOut && filesOnly {
		return fmt.Errorf("--json and --files are mutually exclusive")
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("%s", fsListUsageText(filepath.Base(os.Args[0])))
	}
	targetPath := "/"
	if fs.NArg() == 1 {
		targetPath = normalizeFSRemotePath(fs.Arg(0))
	}

	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	defer remote.close()

	var entries []controlplane.TreeItem
	tree, err := remote.controlPlane.GetTree(ctx, remote.selection.ID, "working-copy", targetPath, 1)
	if err != nil {
		file, fileErr := remote.controlPlane.GetFileContent(ctx, remote.selection.ID, "working-copy", targetPath)
		if fileErr != nil {
			return err
		}
		entries = []controlplane.TreeItem{fsTreeItemFromFile(file)}
	} else {
		entries = tree.Items
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind == "dir" && entries[j].Kind != "dir" {
			return true
		}
		if entries[i].Kind != "dir" && entries[j].Kind == "dir" {
			return false
		}
		return entries[i].Name < entries[j].Name
	})

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Workspace string                  `json:"workspace"`
			Path      string                  `json:"path"`
			Items     []controlplane.TreeItem `json:"items"`
		}{
			Workspace: remote.selection.Name,
			Path:      targetPath,
			Items:     entries,
		})
	}
	if filesOnly {
		for _, entry := range entries {
			fmt.Println(entry.Path)
		}
		return nil
	}

	fmt.Println()
	fmt.Println("workspace: " + remote.selection.Name)
	fmt.Println("path: " + targetPath)
	fmt.Println()
	if len(entries) == 0 {
		fmt.Println("(empty)")
	} else {
		printPlainTable([]string{"Name", "Type", "Size", "Modified"}, fsListRows(entries))
	}
	fmt.Println()
	return nil
}

func fsTreeItemFromFile(file controlplane.FileContentResponse) controlplane.TreeItem {
	return controlplane.TreeItem{
		Path:       file.Path,
		Name:       path.Base(file.Path),
		Kind:       file.Kind,
		Size:       file.Size,
		ModifiedAt: file.ModifiedAt,
		Target:     file.Target,
	}
}

func fsListRows(entries []controlplane.TreeItem) [][]string {
	rows := make([][]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, []string{
			entry.Name,
			fsDisplayType(entry.Kind),
			fsDisplaySize(entry),
			formatFSMtime(entry.ModifiedAt),
		})
	}
	return rows
}

func cmdFSCat(workspace string, args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, fsCatUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("%s", fsCatUsageText(filepath.Base(os.Args[0])))
	}
	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	defer remote.close()

	file, err := remote.controlPlane.GetFileContent(ctx, remote.selection.ID, "working-copy", normalizeFSRemotePath(args[0]))
	if err != nil {
		return err
	}
	if file.Binary {
		return fmt.Errorf("path %q is binary and cannot be printed as text", file.Path)
	}
	_, err = io.WriteString(os.Stdout, file.Content)
	return err
}

func cmdFSGet(workspace string, args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, fsGetUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	targetPath, opts, err := parseFSGetArgs(args)
	if err != nil {
		return err
	}
	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	defer remote.close()

	file, err := remote.controlPlane.GetFileContent(ctx, remote.selection.ID, "working-copy", targetPath)
	if err != nil {
		return err
	}
	if file.Binary {
		return fmt.Errorf("path %q is binary and cannot be printed as text", file.Path)
	}
	content := sliceFSContentLines(file.Content, opts.fromLine, opts.limit, false)
	if opts.lineNumbers {
		content = addFSLineNumbers(content, opts.fromLine)
	}
	_, err = io.WriteString(os.Stdout, content)
	if err == nil && !strings.HasSuffix(content, "\n") {
		fmt.Fprintln(os.Stdout)
	}
	return err
}

func parseFSGetArgs(args []string) (string, fsGetOptions, error) {
	opts := fsGetOptions{fromLine: 1}
	positionals := make([]string, 0, 1)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--line-numbers":
			opts.lineNumbers = true
		case arg == "--from":
			value, ok := nextFSFlagValue(args, &i)
			if !ok {
				return "", opts, fmt.Errorf("--from requires a line number")
			}
			n, err := parsePositiveIntFlag("--from", value)
			if err != nil {
				return "", opts, err
			}
			opts.fromLine = n
		case strings.HasPrefix(arg, "--from="):
			n, err := parsePositiveIntFlag("--from", strings.TrimPrefix(arg, "--from="))
			if err != nil {
				return "", opts, err
			}
			opts.fromLine = n
		case arg == "-l" || arg == "--limit":
			value, ok := nextFSFlagValue(args, &i)
			if !ok {
				return "", opts, fmt.Errorf("%s requires a line count", arg)
			}
			n, err := parsePositiveIntFlag(arg, value)
			if err != nil {
				return "", opts, err
			}
			opts.limit = n
		case strings.HasPrefix(arg, "-l="):
			n, err := parsePositiveIntFlag("-l", strings.TrimPrefix(arg, "-l="))
			if err != nil {
				return "", opts, err
			}
			opts.limit = n
		case strings.HasPrefix(arg, "--limit="):
			n, err := parsePositiveIntFlag("--limit", strings.TrimPrefix(arg, "--limit="))
			if err != nil {
				return "", opts, err
			}
			opts.limit = n
		case strings.HasPrefix(arg, "-"):
			return "", opts, fmt.Errorf("unknown get flag %q", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) != 1 {
		return "", opts, fmt.Errorf("%s", fsGetUsageText(filepath.Base(os.Args[0])))
	}
	targetPath, suffixLine, err := splitFSPathLineSuffix(positionals[0])
	if err != nil {
		return "", opts, err
	}
	if suffixLine > 0 && opts.fromLine == 1 {
		opts.fromLine = suffixLine
	}
	return normalizeFSRemotePath(targetPath), opts, nil
}

func cmdFSMultiGet(workspace string, args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, fsMultiGetUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	opts, err := parseFSMultiGetArgs(args)
	if err != nil {
		return err
	}
	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	defer remote.close()

	results, err := collectFSMultiGetResults(ctx, remote, opts)
	if err != nil {
		return err
	}
	return writeFSMultiGetResults(results, opts)
}

func parseFSMultiGetArgs(args []string) (fsMultiGetOptions, error) {
	opts := fsMultiGetOptions{maxBytes: defaultFSMultiGetMaxBytes}
	positionals := make([]string, 0, 1)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			opts.jsonOut = true
		case arg == "--csv":
			opts.csvOut = true
		case arg == "--md":
			opts.markdown = true
		case arg == "--xml":
			opts.xmlOut = true
		case arg == "--files":
			opts.filesOnly = true
		case arg == "--line-numbers":
			opts.lineNumbers = true
		case arg == "-l" || arg == "--limit":
			value, ok := nextFSFlagValue(args, &i)
			if !ok {
				return opts, fmt.Errorf("%s requires a line count", arg)
			}
			n, err := parsePositiveIntFlag(arg, value)
			if err != nil {
				return opts, err
			}
			opts.limit = n
		case strings.HasPrefix(arg, "-l="):
			n, err := parsePositiveIntFlag("-l", strings.TrimPrefix(arg, "-l="))
			if err != nil {
				return opts, err
			}
			opts.limit = n
		case strings.HasPrefix(arg, "--limit="):
			n, err := parsePositiveIntFlag("--limit", strings.TrimPrefix(arg, "--limit="))
			if err != nil {
				return opts, err
			}
			opts.limit = n
		case arg == "--max-bytes":
			value, ok := nextFSFlagValue(args, &i)
			if !ok {
				return opts, fmt.Errorf("--max-bytes requires a byte count")
			}
			n, err := parsePositiveInt64Flag("--max-bytes", value)
			if err != nil {
				return opts, err
			}
			opts.maxBytes = n
		case strings.HasPrefix(arg, "--max-bytes="):
			n, err := parsePositiveInt64Flag("--max-bytes", strings.TrimPrefix(arg, "--max-bytes="))
			if err != nil {
				return opts, err
			}
			opts.maxBytes = n
		case strings.HasPrefix(arg, "-"):
			return opts, fmt.Errorf("unknown multi-get flag %q", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) != 1 {
		return opts, fmt.Errorf("%s", fsMultiGetUsageText(filepath.Base(os.Args[0])))
	}
	if fsMultiGetFormatCount(opts) > 1 {
		return opts, fmt.Errorf("choose only one output format: --json, --csv, --md, --xml, or --files")
	}
	opts.pattern = strings.TrimSpace(positionals[0])
	return opts, nil
}

func fsMultiGetFormatCount(opts fsMultiGetOptions) int {
	count := 0
	for _, enabled := range []bool{opts.jsonOut, opts.csvOut, opts.markdown, opts.xmlOut, opts.filesOnly} {
		if enabled {
			count++
		}
	}
	return count
}

func cmdFSFind(workspace string, args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, fsFindUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	targetPath, pattern, typeFilter, err := parseFSFindArgs(args)
	if err != nil {
		return err
	}
	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	defer remote.close()

	tree, err := remote.controlPlane.GetTree(ctx, remote.selection.ID, "working-copy", targetPath, 1000)
	if err != nil {
		return err
	}
	matches := fsFindMatches(tree.Items, pattern, typeFilter)
	sort.Strings(matches)
	for _, match := range matches {
		fmt.Println(match)
	}
	return nil
}

func cmdFSGrep(workspace string, args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, grepUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	opts, err := parseGrepArgs(args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(workspace) != "" {
		opts.workspace = strings.TrimSpace(workspace)
	}

	matcher, err := compileGrepMatcher(opts)
	if err != nil {
		return err
	}

	ctx := context.Background()
	remote, err := openFSRemoteWorkspace(ctx, opts.workspace)
	if err != nil {
		return err
	}
	defer remote.close()

	targets, err := collectControlPlaneGrepTargets(ctx, remote, normalizeAFSGrepPath(opts.path))
	if err != nil {
		return err
	}
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].path < targets[j].path
	})
	for _, target := range targets {
		if err := grepProcessFile(target.path, target.content, opts, matcher); err != nil {
			return err
		}
	}
	return nil
}

func collectControlPlaneGrepTargets(ctx context.Context, remote *fsRemoteWorkspace, searchPath string) ([]grepFileTarget, error) {
	tree, err := remote.controlPlane.GetTree(ctx, remote.selection.ID, "working-copy", searchPath, grepTreeMaxDepth)
	if err == nil {
		targets := make([]grepFileTarget, 0, len(tree.Items))
		for _, item := range tree.Items {
			if item.Kind != "file" {
				continue
			}
			file, err := remote.controlPlane.GetFileContent(ctx, remote.selection.ID, "working-copy", item.Path)
			if err != nil {
				return nil, err
			}
			if file.Binary {
				continue
			}
			targets = append(targets, grepFileTarget{
				path:    file.Path,
				content: []byte(file.Content),
				loaded:  true,
			})
		}
		return targets, nil
	}

	file, fileErr := remote.controlPlane.GetFileContent(ctx, remote.selection.ID, "working-copy", searchPath)
	if fileErr != nil {
		return nil, err
	}
	if file.Binary {
		return []grepFileTarget{}, nil
	}
	return []grepFileTarget{{
		path:    file.Path,
		content: []byte(file.Content),
		loaded:  true,
	}}, nil
}

func collectFSMultiGetResults(ctx context.Context, remote *fsRemoteWorkspace, opts fsMultiGetOptions) ([]fsMultiGetResult, error) {
	paths, err := resolveFSMultiGetPaths(ctx, remote, opts.pattern)
	if err != nil {
		return nil, err
	}
	results := make([]fsMultiGetResult, 0, len(paths))
	for _, filePath := range paths {
		normalized := normalizeFSRemotePath(filePath)
		file, err := remote.controlPlane.GetFileContent(ctx, remote.selection.ID, "working-copy", normalized)
		if err != nil {
			return nil, err
		}
		uri := fsWorkspaceURI(remote.selection.Name, file.Path)
		result := fsMultiGetResult{
			File:  uri,
			Title: path.Base(file.Path),
		}
		if file.Binary {
			result.Skipped = true
			result.SkipReason = "binary file"
			results = append(results, result)
			continue
		}
		if opts.maxBytes > 0 && int64(len(file.Content)) > opts.maxBytes {
			result.Skipped = true
			result.SkipReason = fmt.Sprintf("File too large (%s > %s). Use 'afs fs %s get %s' to retrieve.", formatBytes(int64(len(file.Content))), formatBytes(opts.maxBytes), remote.selection.Name, file.Path)
			results = append(results, result)
			continue
		}
		body := sliceFSContentLines(file.Content, 1, opts.limit, opts.limit > 0)
		if opts.lineNumbers {
			body = addFSLineNumbers(body, 1)
		}
		result.Body = body
		results = append(results, result)
	}
	return results, nil
}

func resolveFSMultiGetPaths(ctx context.Context, remote *fsRemoteWorkspace, pattern string) ([]string, error) {
	if strings.TrimSpace(pattern) == "" {
		return nil, fmt.Errorf("multi-get pattern cannot be empty")
	}
	if fsMultiGetIsCommaList(pattern) {
		parts := strings.Split(pattern, ",")
		paths := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				paths = append(paths, normalizeFSRemotePath(part))
			}
		}
		return paths, nil
	}
	if !fsPatternHasGlob(pattern) {
		return []string{normalizeFSRemotePath(pattern)}, nil
	}
	tree, err := remote.controlPlane.GetTree(ctx, remote.selection.ID, "working-copy", "/", 1000)
	if err != nil {
		return nil, err
	}
	matches := make([]string, 0)
	for _, item := range tree.Items {
		if item.Kind != "file" {
			continue
		}
		if fsPathMatchesPattern(item.Path, pattern) {
			matches = append(matches, item.Path)
		}
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no files matched pattern: %s", pattern)
	}
	return matches, nil
}

func fsMultiGetIsCommaList(pattern string) bool {
	return strings.Contains(pattern, ",") && !fsPatternHasGlob(pattern)
}

func fsPatternHasGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

func fsPathMatchesPattern(filePath, pattern string) bool {
	filePath = normalizeFSRemotePath(filePath)
	pattern = strings.TrimSpace(pattern)
	candidates := []string{
		filePath,
		strings.TrimPrefix(filePath, "/"),
	}
	for _, candidate := range candidates {
		if ok, err := path.Match(pattern, candidate); err == nil && ok {
			return true
		}
		if ok, err := path.Match(strings.TrimPrefix(pattern, "/"), candidate); err == nil && ok {
			return true
		}
	}
	return false
}

func writeFSMultiGetResults(results []fsMultiGetResult, opts fsMultiGetOptions) error {
	switch {
	case opts.jsonOut:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	case opts.csvOut:
		return writeFSMultiGetCSV(results)
	case opts.markdown:
		return writeFSMultiGetMarkdown(results)
	case opts.xmlOut:
		return writeFSMultiGetXML(results)
	case opts.filesOnly:
		for _, result := range results {
			if result.Skipped {
				fmt.Fprintf(os.Stdout, "%s,[SKIPPED]\n", result.File)
				continue
			}
			fmt.Fprintln(os.Stdout, result.File)
		}
		return nil
	default:
		for _, result := range results {
			fmt.Fprintln(os.Stdout)
			fmt.Fprintln(os.Stdout, strings.Repeat("=", 60))
			fmt.Fprintln(os.Stdout, "File: "+result.File)
			fmt.Fprintln(os.Stdout, strings.Repeat("=", 60))
			fmt.Fprintln(os.Stdout)
			if result.Skipped {
				fmt.Fprintf(os.Stdout, "[SKIPPED: %s]\n", result.SkipReason)
				continue
			}
			fmt.Fprintln(os.Stdout, result.Body)
		}
		return nil
	}
}

func writeFSMultiGetCSV(results []fsMultiGetResult) error {
	writer := csv.NewWriter(os.Stdout)
	if err := writer.Write([]string{"file", "title", "context", "skipped", "body"}); err != nil {
		return err
	}
	for _, result := range results {
		body := result.Body
		if result.Skipped {
			body = result.SkipReason
		}
		if err := writer.Write([]string{result.File, result.Title, "", strconv.FormatBool(result.Skipped), body}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func writeFSMultiGetMarkdown(results []fsMultiGetResult) error {
	for _, result := range results {
		fmt.Fprintf(os.Stdout, "## %s\n\n", result.File)
		if result.Title != "" && result.Title != result.File {
			fmt.Fprintf(os.Stdout, "**Title:** %s\n\n", result.Title)
		}
		if result.Skipped {
			fmt.Fprintf(os.Stdout, "> %s\n\n", result.SkipReason)
			continue
		}
		fmt.Fprintln(os.Stdout, "```")
		fmt.Fprintln(os.Stdout, result.Body)
		fmt.Fprintln(os.Stdout, "```")
		fmt.Fprintln(os.Stdout)
	}
	return nil
}

func writeFSMultiGetXML(results []fsMultiGetResult) error {
	fmt.Fprintln(os.Stdout, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintln(os.Stdout, "<documents>")
	for _, result := range results {
		fmt.Fprintln(os.Stdout, "  <document>")
		fmt.Fprintf(os.Stdout, "    <file>%s</file>\n", escapeXML(result.File))
		fmt.Fprintf(os.Stdout, "    <title>%s</title>\n", escapeXML(result.Title))
		if result.Skipped {
			fmt.Fprintln(os.Stdout, "    <skipped>true</skipped>")
			fmt.Fprintf(os.Stdout, "    <reason>%s</reason>\n", escapeXML(result.SkipReason))
		} else {
			fmt.Fprintf(os.Stdout, "    <body>%s</body>\n", escapeXML(result.Body))
		}
		fmt.Fprintln(os.Stdout, "  </document>")
	}
	fmt.Fprintln(os.Stdout, "</documents>")
	return nil
}

func fsWorkspaceURI(workspace, filePath string) string {
	workspace = strings.Trim(strings.TrimSpace(workspace), "/")
	if workspace == "" {
		workspace = "workspace"
	}
	return "afs://" + workspace + normalizeFSRemotePath(filePath)
}

func splitFSPathLineSuffix(raw string) (string, int, error) {
	raw = strings.TrimSpace(raw)
	lastColon := strings.LastIndex(raw, ":")
	if lastColon <= 0 || lastColon == len(raw)-1 {
		return raw, 0, nil
	}
	line, err := strconv.Atoi(raw[lastColon+1:])
	if err != nil || line <= 0 {
		return raw, 0, nil
	}
	return raw[:lastColon], line, nil
}

func nextFSFlagValue(args []string, index *int) (string, bool) {
	if *index+1 >= len(args) {
		return "", false
	}
	*index = *index + 1
	return args[*index], true
}

func parsePositiveIntFlag(name, value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return n, nil
}

func parsePositiveInt64Flag(name, value string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return n, nil
}

func sliceFSContentLines(content string, fromLine, limit int, appendTruncation bool) string {
	if fromLine <= 0 {
		fromLine = 1
	}
	if limit < 0 {
		limit = 0
	}
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") && len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}
	start := fromLine - 1
	if start >= len(lines) {
		return ""
	}
	end := len(lines)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	out := strings.Join(lines[start:end], "\n")
	if strings.HasSuffix(content, "\n") && end == len(lines) {
		out += "\n"
	}
	if appendTruncation && limit > 0 && end < len(lines) {
		out += fmt.Sprintf("\n\n[... truncated %d more lines]", len(lines)-end)
	}
	return out
}

func addFSLineNumbers(content string, startLine int) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	width := len(strconv.Itoa(startLine + len(lines) - 1))
	for i, line := range lines {
		lines[i] = fmt.Sprintf("%*d: %s", width, startLine+i, line)
	}
	out := strings.Join(lines, "\n")
	if strings.HasSuffix(content, "\n") {
		out += "\n"
	}
	return out
}

func fsFindMatches(items []controlplane.TreeItem, pattern, typeFilter string) []string {
	matches := make([]string, 0)
	for _, item := range items {
		if typeFilter != "" && item.Kind != typeFilter {
			continue
		}
		name := item.Name
		if name == "" {
			name = path.Base(item.Path)
		}
		ok, err := path.Match(pattern, name)
		if err != nil {
			ok = name == pattern
		}
		if ok {
			matches = append(matches, item.Path)
		}
	}
	return matches
}

func parseFSFindArgs(args []string) (string, string, string, error) {
	targetPath := "/"
	pattern := "*"
	typeFilter := ""
	positionals := make([]string, 0, 1)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-name":
			if i+1 >= len(args) {
				return "", "", "", errors.New("-name requires a pattern")
			}
			i++
			pattern = args[i]
		case "-type":
			if i+1 >= len(args) {
				return "", "", "", errors.New("-type requires f, d, l, file, dir, or symlink")
			}
			i++
			parsed, err := parseFSFindType(args[i])
			if err != nil {
				return "", "", "", err
			}
			typeFilter = parsed
		case "-print":
			continue
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", "", fmt.Errorf("unknown find flag %q", arg)
			}
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) > 1 {
		return "", "", "", fmt.Errorf("%s", fsFindUsageText(filepath.Base(os.Args[0])))
	}
	if len(positionals) == 1 {
		targetPath = normalizeFSRemotePath(positionals[0])
	}
	return targetPath, pattern, typeFilter, nil
}

func parseFSFindType(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "f", "file":
		return "file", nil
	case "d", "dir":
		return "dir", nil
	case "l", "symlink":
		return "symlink", nil
	case "":
		return "", nil
	default:
		return "", fmt.Errorf("unknown find type %q", raw)
	}
}

func normalizeFSRemotePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "." {
		return "/"
	}
	return normalizeAFSGrepPath(raw)
}

func fsDisplayType(raw string) string {
	switch raw {
	case "dir":
		return "dir"
	case "symlink":
		return "link"
	case "file":
		return "file"
	default:
		return raw
	}
}

func fsDisplaySize(entry controlplane.TreeItem) string {
	if entry.Kind == "dir" {
		return "-"
	}
	return formatBytes(entry.Size)
}

func formatFSMtime(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "unknown"
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	return parsed.Local().Format("2006-01-02 15:04")
}

func fsListUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs [workspace] ls [path] [--json|--files]

List files in a workspace directory.
`, bin)
}

func fsCatUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs [workspace] cat <path>
  %s fs [workspace] cat <path> --version <version-id>

Print a workspace file to stdout. With --version or --file-id/--ordinal, print
an exact historical file version.
`, bin, bin)
}

func fsGetUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs [workspace] get <path>[:line] [--from <line>] [-l <lines>] [--line-numbers]

Print a workspace text file, optionally sliced to a line range.
`, bin)
}

func fsMultiGetUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs [workspace] multi-get <pattern> [-l <lines>] [--max-bytes <bytes>] [--json|--csv|--md|--xml|--files]

Fetch multiple workspace files by glob or comma-separated path list.
`, bin)
}

func fsFindUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s fs [workspace] find [path] [-name <pattern>] [-type f|d|l] [-print]

Find workspace paths by basename pattern.
`, bin)
}
