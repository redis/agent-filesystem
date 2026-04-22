package controlplane

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/redis/agent-filesystem/internal/mcpproto"
	afsclient "github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

const hostedMCPProtocolVersion = "2024-11-05"

type hostedMCPProvider struct {
	manager    *DatabaseManager
	databaseID string
	workspace  string
	readonly   bool
}

type mcpFileListItem struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modified_at,omitempty"`
	Target     string `json:"target,omitempty"`
}

type mcpGrepMatch struct {
	Path   string `json:"path"`
	Line   int64  `json:"line,omitempty"`
	Text   string `json:"text"`
	Binary bool   `json:"binary,omitempty"`
}

func authWrappedMCPHandler(manager *DatabaseManager, auth *AuthHandler) http.Handler {
	server := &mcpproto.Server{
		ProtocolVersion: hostedMCPProtocolVersion,
		Name:            "afs-cloud",
		Version:         "0.1.0",
		Instructions:    "Workspace-scoped hosted Agent Filesystem MCP server.",
		Provider: mcpproto.ProviderFunc{
			ToolsFn: func(ctx context.Context) []mcpproto.Tool {
				provider, ok := hostedMCPProviderFromContext(ctx, manager)
				if !ok {
					return nil
				}
				return provider.Tools(ctx)
			},
			CallToolFn: func(ctx context.Context, name string, args map[string]any) mcpproto.ToolResult {
				provider, ok := hostedMCPProviderFromContext(ctx, manager)
				if !ok {
					return mcpErrorResult(ErrUnauthorized)
				}
				return provider.CallTool(ctx, name, args)
			},
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider, sessionInput, sessionID, err := hostedMCPProviderForRequest(r.Context(), manager, r)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}
		if _, err := manager.UpsertWorkspaceSession(r.Context(), provider.databaseID, provider.workspace, sessionID, sessionInput); err != nil {
			writeError(w, err)
			return
		}
		ctx := context.WithValue(r.Context(), hostedMCPProviderContextKey{}, provider)
		server.ServeHTTP(w, r.WithContext(ctx))
	})

	if auth == nil {
		auth = NewNoAuthHandler()
	}
	return auth.Middleware(handler)
}

type hostedMCPProviderContextKey struct{}

func hostedMCPProviderFromContext(ctx context.Context, manager *DatabaseManager) (*hostedMCPProvider, bool) {
	provider, ok := ctx.Value(hostedMCPProviderContextKey{}).(*hostedMCPProvider)
	if !ok || provider == nil || manager == nil {
		return nil, false
	}
	return provider, true
}

func hostedMCPProviderForRequest(ctx context.Context, manager *DatabaseManager, r *http.Request) (*hostedMCPProvider, createWorkspaceSessionRequest, string, error) {
	identity, ok := AuthIdentityFromContext(ctx)
	if !ok || strings.TrimSpace(identity.TokenID) == "" || strings.TrimSpace(identity.ScopedDatabaseID) == "" || strings.TrimSpace(identity.ScopedWorkspace) == "" {
		return nil, createWorkspaceSessionRequest{}, "", ErrUnauthorized
	}
	sessionInput := createWorkspaceSessionRequest{
		AgentID:         strings.TrimSpace(r.Header.Get(AgentIDHeader)),
		ClientKind:      firstNonEmpty(strings.TrimSpace(r.Header.Get("X-AFS-Client-Kind")), "mcp"),
		AFSVersion:      strings.TrimSpace(r.Header.Get("X-AFS-AFS-Version")),
		Hostname:        strings.TrimSpace(r.Header.Get("X-AFS-Hostname")),
		OperatingSystem: strings.TrimSpace(r.Header.Get("X-AFS-OS")),
		LocalPath:       strings.TrimSpace(r.Header.Get("X-AFS-Local-Path")),
		Readonly:        identity.Readonly,
	}
	sessionID := buildHostedMCPSessionID(identity.TokenID, sessionInput.Hostname, sessionInput.LocalPath)
	return &hostedMCPProvider{
		manager:    manager,
		databaseID: strings.TrimSpace(identity.ScopedDatabaseID),
		workspace:  strings.TrimSpace(identity.ScopedWorkspace),
		readonly:   identity.Readonly,
	}, sessionInput, sessionID, nil
}

func buildHostedMCPSessionID(tokenID, hostname, localPath string) string {
	base := strings.TrimSpace(tokenID)
	if host := strings.TrimSpace(hostname); host != "" {
		base += ":" + host
	}
	if lp := strings.TrimSpace(localPath); lp != "" {
		base += ":" + lp
	}
	if len(base) > 96 {
		base = base[:96]
	}
	return "mcp-" + strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(base)
}

func (p *hostedMCPProvider) Tools(context.Context) []mcpproto.Tool {
	return []mcpproto.Tool{
		{
			Name:        "workspace_current",
			Description: "Show the current hosted AFS workspace available to this MCP token",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "checkpoint_list",
			Description: "List checkpoints for the current workspace",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "checkpoint_create",
			Description: "Create a new checkpoint from the current live workspace state",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"checkpoint": map[string]string{"type": "string", "description": "Optional checkpoint name"},
				},
			},
		},
		{
			Name:        "checkpoint_restore",
			Description: "Restore the current workspace to a checkpoint",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"checkpoint": map[string]string{"type": "string", "description": "Checkpoint name"},
				},
				"required": []string{"checkpoint"},
			},
		},
		{
			Name:        "file_read",
			Description: "Read a file or symlink from the current workspace",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]string{"type": "string", "description": "Absolute path inside the workspace"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "file_lines",
			Description: "Read a specific line range from a text file",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]string{"type": "string", "description": "Absolute path inside the workspace"},
					"start": map[string]string{"type": "integer", "description": "Start line (1-indexed)"},
					"end":   map[string]string{"type": "integer", "description": "End line (inclusive, -1 for EOF)"},
				},
				"required": []string{"path", "start"},
			},
		},
		{
			Name:        "file_list",
			Description: "List files and directories under a workspace path",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]string{"type": "string", "description": "Absolute directory path", "default": "/"},
					"depth": map[string]string{"type": "integer", "description": "Depth relative to the requested path", "default": "1"},
				},
			},
		},
		{
			Name:        "file_write",
			Description: "Write a file in the current workspace, creating parent directories as needed",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]string{"type": "string", "description": "Absolute file path"},
					"content": map[string]string{"type": "string", "description": "Full file contents"},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			Name:        "file_replace",
			Description: "Replace text in a file",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]string{"type": "string", "description": "Absolute file path"},
					"old":  map[string]string{"type": "string", "description": "Text to find"},
					"new":  map[string]string{"type": "string", "description": "Replacement text"},
					"all":  map[string]string{"type": "boolean", "description": "Replace all occurrences"},
				},
				"required": []string{"path", "old", "new"},
			},
		},
		{
			Name:        "file_insert",
			Description: "Insert content after a specific line",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]string{"type": "string", "description": "Absolute file path"},
					"line":    map[string]string{"type": "integer", "description": "Insert after this line; 0=beginning, -1=end"},
					"content": map[string]string{"type": "string", "description": "Content to insert"},
				},
				"required": []string{"path", "line", "content"},
			},
		},
		{
			Name:        "file_delete_lines",
			Description: "Delete a line range from a file",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]string{"type": "string", "description": "Absolute file path"},
					"start": map[string]string{"type": "integer", "description": "Start line (1-indexed)"},
					"end":   map[string]string{"type": "integer", "description": "End line (inclusive)"},
				},
				"required": []string{"path", "start", "end"},
			},
		},
	}
}

func (p *hostedMCPProvider) CallTool(ctx context.Context, name string, args map[string]any) mcpproto.ToolResult {
	var (
		value any
		err   error
	)
	switch name {
	case "workspace_current":
		value = map[string]any{
			"workspace": p.workspace,
			"database":  p.databaseID,
			"readonly":  p.readonly,
		}
	case "checkpoint_list":
		value, err = p.manager.ListCheckpoints(ctx, p.databaseID, p.workspace, 100)
		if err == nil {
			value = map[string]any{"workspace": p.workspace, "checkpoints": value}
		}
	case "checkpoint_create":
		err = p.ensureWritable()
		if err == nil {
			var checkpointID string
			checkpointID, err = mcpOptionalString(args, "checkpoint")
			if err == nil {
				if checkpointID == "" {
					checkpointID = generatedSavepointName()
				}
				if err = validateHostedMCPName("checkpoint", checkpointID); err == nil {
					var saved bool
					saved, err = p.manager.SaveCheckpointFromLive(ctx, p.databaseID, p.workspace, checkpointID)
					value = map[string]any{
						"workspace":   p.workspace,
						"checkpoint":  checkpointID,
						"created":     saved,
						"description": ternaryString(saved, "checkpoint created", "no changes to checkpoint"),
					}
				}
			}
		}
	case "checkpoint_restore":
		err = p.ensureWritable()
		if err == nil {
			var checkpointID string
			checkpointID, err = mcpRequiredString(args, "checkpoint")
			if err == nil {
				err = p.manager.RestoreCheckpoint(ctx, p.databaseID, p.workspace, checkpointID)
				value = map[string]any{
					"workspace":  p.workspace,
					"checkpoint": checkpointID,
					"mode":       "live-workspace",
				}
			}
		}
	case "file_read":
		value, err = p.toolFileRead(ctx, args)
	case "file_lines":
		value, err = p.toolFileLines(ctx, args)
	case "file_list":
		value, err = p.toolFileList(ctx, args)
	case "file_write":
		err = p.ensureWritable()
		if err == nil {
			value, err = p.toolFileWrite(ctx, args)
		}
	case "file_replace":
		err = p.ensureWritable()
		if err == nil {
			value, err = p.toolFileReplace(ctx, args)
		}
	case "file_insert":
		err = p.ensureWritable()
		if err == nil {
			value, err = p.toolFileInsert(ctx, args)
		}
	case "file_delete_lines":
		err = p.ensureWritable()
		if err == nil {
			value, err = p.toolFileDeleteLines(ctx, args)
		}
	default:
		err = fmt.Errorf("unknown tool %q", name)
	}
	if err != nil {
		return mcpErrorResult(err)
	}
	return mcpproto.StructuredResult(value)
}

func (p *hostedMCPProvider) ensureWritable() error {
	if p.readonly {
		return fmt.Errorf("this mcp token is read-only")
	}
	return nil
}

func (p *hostedMCPProvider) toolFileRead(ctx context.Context, args map[string]any) (any, error) {
	normalizedPath, fsClient, stat, err := p.resolveWorkspaceFSPath(ctx, args, true)
	if err != nil {
		return nil, err
	}
	return readWorkspaceFSEntry(ctx, p.workspace, normalizedPath, fsClient, stat)
}

func (p *hostedMCPProvider) toolFileLines(ctx context.Context, args map[string]any) (any, error) {
	normalizedPath, fsClient, stat, err := p.resolveWorkspaceFSPath(ctx, args, true)
	if err != nil {
		return nil, err
	}
	start, err := mcpInt(args, "start", 0)
	if err != nil {
		return nil, err
	}
	end, err := mcpInt(args, "end", -1)
	if err != nil {
		return nil, err
	}
	if stat.Type == "dir" {
		return nil, fmt.Errorf("path %q is a directory", normalizedPath)
	}
	content, err := fsClient.Lines(ctx, normalizedPath, start, end)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace": p.workspace,
		"path":      normalizedPath,
		"start":     start,
		"end":       end,
		"content":   content,
	}, nil
}

func (p *hostedMCPProvider) toolFileList(ctx context.Context, args map[string]any) (any, error) {
	path, err := mcpStringDefault(args, "path", "/")
	if err != nil {
		return nil, err
	}
	depth, err := mcpInt(args, "depth", 1)
	if err != nil {
		return nil, err
	}
	normalizedPath := normalizeAFSGrepPath(path)
	fsClient, err := p.fsClient(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := listWorkspaceFSEntries(ctx, fsClient, normalizedPath, depth)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace": p.workspace,
		"path":      normalizedPath,
		"entries":   entries,
	}, nil
}

func (p *hostedMCPProvider) toolFileWrite(ctx context.Context, args map[string]any) (any, error) {
	content, err := mcpRequiredText(args, "content", true)
	if err != nil {
		return nil, err
	}
	return p.mutateWorkspaceFile(ctx, args, func(ctx context.Context, fsClient afsclient.Client, normalizedPath string, stat *afsclient.StatResult) (map[string]any, error) {
		if stat == nil {
			if err := fsClient.EchoCreate(ctx, normalizedPath, []byte(content), 0o644); err != nil {
				return nil, err
			}
			return map[string]any{"kind": "file", "created": true, "bytes": len(content)}, nil
		}
		if stat.Type == "dir" {
			return nil, fmt.Errorf("path %q is a directory", normalizedPath)
		}
		if err := fsClient.Echo(ctx, normalizedPath, []byte(content)); err != nil {
			return nil, err
		}
		return map[string]any{"kind": stat.Type, "created": false, "bytes": len(content)}, nil
	})
}

func (p *hostedMCPProvider) toolFileReplace(ctx context.Context, args map[string]any) (any, error) {
	oldValue, err := mcpRequiredText(args, "old", true)
	if err != nil {
		return nil, err
	}
	newValue, err := mcpRequiredText(args, "new", true)
	if err != nil {
		return nil, err
	}
	replaceAll, err := mcpBool(args, "all", false)
	if err != nil {
		return nil, err
	}
	return p.mutateWorkspaceFile(ctx, args, func(ctx context.Context, fsClient afsclient.Client, normalizedPath string, stat *afsclient.StatResult) (map[string]any, error) {
		if stat == nil {
			return nil, os.ErrNotExist
		}
		if stat.Type == "dir" {
			return nil, fmt.Errorf("path %q is a directory", normalizedPath)
		}
		replaced, err := fsClient.Replace(ctx, normalizedPath, oldValue, newValue, replaceAll)
		if err != nil {
			return nil, err
		}
		return map[string]any{"kind": stat.Type, "replacements": replaced}, nil
	})
}

func (p *hostedMCPProvider) toolFileInsert(ctx context.Context, args map[string]any) (any, error) {
	line, err := mcpInt(args, "line", 0)
	if err != nil {
		return nil, err
	}
	content, err := mcpRequiredText(args, "content", true)
	if err != nil {
		return nil, err
	}
	return p.mutateWorkspaceFile(ctx, args, func(ctx context.Context, fsClient afsclient.Client, normalizedPath string, stat *afsclient.StatResult) (map[string]any, error) {
		if stat == nil {
			return nil, os.ErrNotExist
		}
		if stat.Type == "dir" {
			return nil, fmt.Errorf("path %q is a directory", normalizedPath)
		}
		if err := fsClient.Insert(ctx, normalizedPath, line, content); err != nil {
			return nil, err
		}
		return map[string]any{"kind": stat.Type, "line": line, "bytes": len(content)}, nil
	})
}

func (p *hostedMCPProvider) toolFileDeleteLines(ctx context.Context, args map[string]any) (any, error) {
	start, err := mcpInt(args, "start", 0)
	if err != nil {
		return nil, err
	}
	end, err := mcpInt(args, "end", 0)
	if err != nil {
		return nil, err
	}
	return p.mutateWorkspaceFile(ctx, args, func(ctx context.Context, fsClient afsclient.Client, normalizedPath string, stat *afsclient.StatResult) (map[string]any, error) {
		if stat == nil {
			return nil, os.ErrNotExist
		}
		if stat.Type == "dir" {
			return nil, fmt.Errorf("path %q is a directory", normalizedPath)
		}
		deleted, err := fsClient.DeleteLines(ctx, normalizedPath, start, end)
		if err != nil {
			return nil, err
		}
		return map[string]any{"kind": stat.Type, "deleted": deleted}, nil
	})
}

func (p *hostedMCPProvider) fsClient(ctx context.Context) (afsclient.Client, error) {
	service, _, route, err := p.manager.resolveScopedWorkspace(ctx, p.databaseID, p.workspace)
	if err != nil {
		return nil, err
	}
	fsKey, _, _, err := EnsureWorkspaceRoot(ctx, service.store, route.Name)
	if err != nil {
		return nil, err
	}
	return afsclient.New(service.store.rdb, fsKey), nil
}

func (p *hostedMCPProvider) resolveWorkspaceFSPath(ctx context.Context, args map[string]any, requireFile bool) (string, afsclient.Client, *afsclient.StatResult, error) {
	rawPath, err := mcpRequiredString(args, "path")
	if err != nil {
		return "", nil, nil, err
	}
	normalizedPath := normalizeAFSGrepPath(rawPath)
	fsClient, err := p.fsClient(ctx)
	if err != nil {
		return "", nil, nil, err
	}
	stat, err := fsClient.Stat(ctx, normalizedPath)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil, nil, os.ErrNotExist
		}
		return "", nil, nil, err
	}
	if stat == nil {
		return "", nil, nil, os.ErrNotExist
	}
	if requireFile && stat.Type == "dir" {
		return "", nil, nil, fmt.Errorf("path %q is a directory", normalizedPath)
	}
	return normalizedPath, fsClient, stat, nil
}

func (p *hostedMCPProvider) mutateWorkspaceFile(ctx context.Context, args map[string]any, mutate func(context.Context, afsclient.Client, string, *afsclient.StatResult) (map[string]any, error)) (any, error) {
	rawPath, err := mcpRequiredString(args, "path")
	if err != nil {
		return nil, err
	}
	normalizedPath := normalizeAFSGrepPath(rawPath)
	fsClient, err := p.fsClient(ctx)
	if err != nil {
		return nil, err
	}
	stat, err := fsClient.Stat(ctx, normalizedPath)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	if errors.Is(err, redis.Nil) {
		stat = nil
	}
	payload, err := mutate(ctx, fsClient, normalizedPath, stat)
	if err != nil {
		return nil, err
	}
	payload["workspace"] = p.workspace
	payload["path"] = normalizedPath
	return payload, nil
}

func mcpErrorResult(err error) mcpproto.ToolResult {
	return mcpproto.ToolResult{
		Content: []mcpproto.TextContent{{
			Type: "text",
			Text: err.Error(),
		}},
		IsError: true,
	}
}

func readWorkspaceFSEntry(ctx context.Context, workspace, normalizedPath string, fsClient afsclient.Client, stat *afsclient.StatResult) (any, error) {
	switch stat.Type {
	case "symlink":
		target, err := fsClient.Readlink(ctx, normalizedPath)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"workspace": workspace,
			"path":      normalizedPath,
			"kind":      "symlink",
			"target":    target,
			"content":   target,
		}, nil
	case "dir":
		return nil, fmt.Errorf("path %q is a directory", normalizedPath)
	default:
		content, err := fsClient.Cat(ctx, normalizedPath)
		if err != nil {
			return nil, err
		}
		if grepBinaryPrefix(content) {
			return map[string]any{
				"workspace": workspace,
				"path":      normalizedPath,
				"kind":      "file",
				"size":      stat.Size,
				"binary":    true,
			}, nil
		}
		return map[string]any{
			"workspace": workspace,
			"path":      normalizedPath,
			"kind":      "file",
			"content":   string(content),
			"size":      stat.Size,
		}, nil
	}
}

func listWorkspaceFSEntries(ctx context.Context, fsClient afsclient.Client, manifestPath string, depth int) ([]mcpFileListItem, error) {
	tree, err := fsClient.Tree(ctx, manifestPath, depth)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	entries := make([]mcpFileListItem, 0, len(tree))
	for _, node := range tree {
		if node.Path == manifestPath {
			continue
		}
		stat, err := fsClient.Stat(ctx, node.Path)
		if err != nil {
			return nil, err
		}
		if stat == nil {
			continue
		}
		item := mcpFileListItem{
			Path:       node.Path,
			Name:       filepath.Base(node.Path),
			Kind:       stat.Type,
			Size:       stat.Size,
			ModifiedAt: mcpFileModifiedAt(stat.Mtime),
		}
		if stat.Type == "symlink" {
			target, err := fsClient.Readlink(ctx, node.Path)
			if err != nil {
				return nil, err
			}
			item.Target = target
		}
		entries = append(entries, item)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			if entries[i].Kind == "dir" {
				return true
			}
			if entries[j].Kind == "dir" {
				return false
			}
		}
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func mcpFileModifiedAt(mtimeMs int64) string {
	if mtimeMs == 0 {
		return ""
	}
	return time.UnixMilli(mtimeMs).UTC().Format(time.RFC3339)
}

func mcpRequiredString(args map[string]any, key string) (string, error) {
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

func mcpRequiredText(args map[string]any, key string, allowEmpty bool) (string, error) {
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

func mcpOptionalString(args map[string]any, key string) (string, error) {
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

func mcpStringDefault(args map[string]any, key, fallback string) (string, error) {
	value, err := mcpOptionalString(args, key)
	if err != nil {
		return "", err
	}
	if value == "" {
		return fallback, nil
	}
	return value, nil
}

func mcpBool(args map[string]any, key string, fallback bool) (bool, error) {
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

func mcpInt(args map[string]any, key string, fallback int) (int, error) {
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

func grepBinaryPrefix(data []byte) bool {
	limit := len(data)
	if limit > 8192 {
		limit = 8192
	}
	for _, b := range data[:limit] {
		if b == 0 {
			return true
		}
	}
	return false
}

func normalizeAFSGrepPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "." {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	cleaned := filepath.ToSlash(filepath.Clean(trimmed))
	if cleaned == "." || cleaned == "" {
		return "/"
	}
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	return cleaned
}

func ternaryString(condition bool, whenTrue, whenFalse string) string {
	if condition {
		return whenTrue
	}
	return whenFalse
}

func generatedSavepointName() string {
	return "cp-" + time.Now().UTC().Format("20060102-150405")
}

func validateHostedMCPName(kind, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", kind)
	}
	if !namePattern.MatchString(value) {
		return fmt.Errorf("invalid %s %q", kind, value)
	}
	return nil
}
