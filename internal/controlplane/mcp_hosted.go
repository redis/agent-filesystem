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
	scope      string // "workspace:<id>" or "control-plane"
	databaseID string // empty for control-plane scope
	workspace  string // empty for control-plane scope
	profile    string // empty for control-plane scope
	readonly   bool
	baseURL    string // absolute URL of /mcp (used by mcp_token_issue)
}

func (p *hostedMCPProvider) isControlPlane() bool {
	return isControlPlaneScope(p.scope)
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
		// Session tracking is per-workspace; control-plane tokens have no
		// workspace binding, so skip the upsert for that path.
		if !provider.isControlPlane() {
			if _, err := manager.UpsertWorkspaceSession(r.Context(), provider.databaseID, provider.workspace, sessionID, sessionInput); err != nil {
				writeError(w, err)
				return
			}
		}
		ctx := context.WithValue(r.Context(), hostedMCPProviderContextKey{}, provider)
		ctx = WithChangeSessionContext(ctx, ChangeSessionContext{SessionID: sessionID})
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
	if !ok || strings.TrimSpace(identity.TokenID) == "" {
		return nil, createWorkspaceSessionRequest{}, "", ErrUnauthorized
	}
	scope := strings.TrimSpace(identity.Scope)
	if scope == "" && strings.TrimSpace(identity.ScopedWorkspaceID) != "" {
		scope = workspaceScope(identity.ScopedWorkspaceID)
	}
	isControlPlane := isControlPlaneScope(scope)
	// Workspace-scoped tokens require a workspace binding; control-plane tokens must not.
	if !isControlPlane && (strings.TrimSpace(identity.ScopedDatabaseID) == "" || strings.TrimSpace(identity.ScopedWorkspace) == "") {
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
	provider := &hostedMCPProvider{
		manager: manager,
		scope:   scope,
		baseURL: deriveMCPBaseURL(r),
	}
	if !isControlPlane {
		provider.databaseID = strings.TrimSpace(identity.ScopedDatabaseID)
		provider.workspace = strings.TrimSpace(identity.ScopedWorkspace)
		provider.profile = firstNonEmpty(strings.TrimSpace(identity.MCPProfile), MCPProfileWorkspaceRW)
		provider.readonly = identity.Readonly
	}
	return provider, sessionInput, sessionID, nil
}

// deriveMCPBaseURL reconstructs the absolute URL the caller reached /mcp at,
// honoring X-Forwarded-Proto/Host when running behind a reverse proxy.
func deriveMCPBaseURL(r *http.Request) string {
	if r == nil {
		return ""
	}
	scheme := "http"
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host + "/mcp"
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

func (p *hostedMCPProvider) Tools(ctx context.Context) []mcpproto.Tool {
	if p.isControlPlane() {
		return p.controlPlaneTools()
	}
	return p.workspaceTools()
}

func (p *hostedMCPProvider) workspaceTools() []mcpproto.Tool {
	tools := []mcpproto.Tool{
		{
			Name:        "workspace_current",
			Description: "Show the current hosted AFS workspace available to this MCP token",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "workspace_get_versioning_policy",
			Description: "Fetch the versioning policy for the current workspace",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "workspace_set_versioning_policy",
			Description: "Update the versioning policy for the current workspace. Omitted fields keep their current values; array fields can be cleared with an empty array.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"mode":                    map[string]any{"type": "string", "enum": []string{WorkspaceVersioningModeOff, WorkspaceVersioningModeAll, WorkspaceVersioningModePaths}},
					"include_globs":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"exclude_globs":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"max_versions_per_file":   map[string]any{"type": "integer"},
					"max_age_days":            map[string]any{"type": "integer"},
					"max_total_bytes":         map[string]any{"type": "integer"},
					"large_file_cutoff_bytes": map[string]any{"type": "integer"},
				},
			},
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
			Name:        "file_history",
			Description: "List ordered file history for a path in the current workspace",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]string{"type": "string", "description": "Absolute file path inside the workspace"},
					"direction": map[string]string{"type": "string", "description": "History order: desc (default) or asc"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "file_read_version",
			Description: "Read the exact historical content for a file version by version_id",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"version_id": map[string]string{"type": "string", "description": "Stable file version identifier"},
					"file_id":    map[string]string{"type": "string", "description": "Stable file lineage identifier used with ordinal"},
					"ordinal":    map[string]string{"type": "integer", "description": "Per-lineage version ordinal used with file_id"},
				},
			},
		},
		{
			Name:        "file_diff_versions",
			Description: "Diff one file version selector against another selector such as head or working-copy",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":            map[string]string{"type": "string", "description": "Absolute file path inside the workspace"},
					"from_ref":        map[string]string{"type": "string", "description": "Source selector ref: head or working-copy"},
					"from_version_id": map[string]string{"type": "string", "description": "Source stable file version identifier"},
					"from_file_id":    map[string]string{"type": "string", "description": "Source stable file lineage identifier used with from_ordinal"},
					"from_ordinal":    map[string]string{"type": "integer", "description": "Source per-lineage version ordinal used with from_file_id"},
					"to_ref":          map[string]string{"type": "string", "description": "Destination selector ref: head or working-copy"},
					"to_version_id":   map[string]string{"type": "string", "description": "Destination stable file version identifier"},
					"to_file_id":      map[string]string{"type": "string", "description": "Destination stable file lineage identifier used with to_ordinal"},
					"to_ordinal":      map[string]string{"type": "integer", "description": "Destination per-lineage version ordinal used with to_file_id"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "file_restore_version",
			Description: "Restore historical file content into the live workspace and create a new latest version",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":       map[string]string{"type": "string", "description": "Absolute file path inside the workspace"},
					"version_id": map[string]string{"type": "string", "description": "Stable file version identifier"},
					"file_id":    map[string]string{"type": "string", "description": "Stable file lineage identifier used with ordinal"},
					"ordinal":    map[string]string{"type": "integer", "description": "Per-lineage version ordinal used with file_id"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "file_undelete",
			Description: "Revive the latest deleted lineage at a path or restore a selected historical version from a deleted lineage",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":       map[string]string{"type": "string", "description": "Absolute file path inside the workspace"},
					"version_id": map[string]string{"type": "string", "description": "Stable file version identifier"},
					"file_id":    map[string]string{"type": "string", "description": "Stable file lineage identifier used with ordinal"},
					"ordinal":    map[string]string{"type": "integer", "description": "Per-lineage version ordinal used with file_id"},
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
			Name:        "file_glob",
			Description: "Find files or directories under a workspace path by basename glob pattern. Use this for filename discovery before reading or editing. Do not use it for content search; use file_grep instead.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]string{"type": "string", "description": "Absolute directory path", "default": "/"},
					"pattern": map[string]string{"type": "string", "description": "Basename glob pattern"},
					"kind":    map[string]string{"type": "string", "description": "Optional kind filter: file, dir, symlink, or any", "default": "file"},
					"limit":   map[string]string{"type": "integer", "description": "Maximum number of results to return", "default": "100"},
				},
				"required": []string{"pattern"},
			},
		},
		{
			Name:        "file_grep",
			Description: "Search file contents in the current workspace. Use this for content search across one file or many files. Do not use it for directory discovery or filename-only matching. Choose only one search mode among glob, fixed_strings, or regexp.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":               map[string]string{"type": "string", "description": "Absolute workspace path to search within", "default": "/"},
					"pattern":            map[string]string{"type": "string", "description": "Pattern to search for"},
					"ignore_case":        map[string]string{"type": "boolean", "description": "Case-insensitive search"},
					"glob":               map[string]string{"type": "boolean", "description": "Use glob matching semantics for the pattern"},
					"fixed_strings":      map[string]string{"type": "boolean", "description": "Treat the pattern as a fixed string"},
					"regexp":             map[string]string{"type": "boolean", "description": "Use regex mode with RE2-style syntax"},
					"word_regexp":        map[string]string{"type": "boolean", "description": "Match whole words"},
					"line_regexp":        map[string]string{"type": "boolean", "description": "Match entire lines"},
					"invert_match":       map[string]string{"type": "boolean", "description": "Return non-matching lines"},
					"files_with_matches": map[string]string{"type": "boolean", "description": "Return only matching file paths"},
					"count":              map[string]string{"type": "boolean", "description": "Return match counts per file instead of line matches"},
					"max_count":          map[string]string{"type": "integer", "description": "Maximum selected lines per file"},
				},
				"required": []string{"pattern"},
			},
		},
		{
			Name:        "file_write",
			Description: "Write a full file in the current workspace, creating parent directories as needed. Use this for new files or full overwrites. Do not use it for small localized edits; prefer file_replace, file_insert, file_delete_lines, or file_patch for that.",
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
			Name:        "file_create_exclusive",
			Description: "Atomically create a file only if it does not already exist; fails if the path is already taken. Useful for distributed locking and coordination between agents. Creates parent directories as needed.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]string{"type": "string", "description": "Absolute file path"},
					"content": map[string]string{"type": "string", "description": "File contents to write on creation"},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			Name:        "file_replace",
			Description: "Replace text in a file. Use this for small exact substitutions after you have inspected the file. Do not use it for full rewrites; use file_write instead. If the target text may occur more than once, be explicit about whether all occurrences are intended.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":                 map[string]string{"type": "string", "description": "Absolute file path"},
					"old":                  map[string]string{"type": "string", "description": "Text to find"},
					"new":                  map[string]string{"type": "string", "description": "Replacement text"},
					"all":                  map[string]string{"type": "boolean", "description": "Replace all occurrences"},
					"expected_occurrences": map[string]string{"type": "integer", "description": "Optional expected number of matching occurrences before replacing"},
					"start_line":           map[string]string{"type": "integer", "description": "Optional exact 1-indexed line where the match must begin"},
					"context_before":       map[string]string{"type": "string", "description": "Optional exact text that must appear immediately before the match"},
					"context_after":        map[string]string{"type": "string", "description": "Optional exact text that must appear immediately after the match"},
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
			Description: "Delete a line range from a file. Use this for precise removals when line numbers are known. Do not use it for semantic search-and-replace; use file_replace instead.",
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
		{
			Name:        "file_patch",
			Description: "Apply one or more structured text patches to a file. Use this for precise multi-step edits where exact context matters.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":            map[string]string{"type": "string", "description": "Absolute workspace file path"},
					"expected_sha256": map[string]string{"type": "string", "description": "Optional SHA-256 hash of the file before patching; fail if the file changed"},
					"patches": map[string]any{
						"type":        "array",
						"description": "Ordered list of structured patches to apply",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"op":             map[string]string{"type": "string", "description": "Patch operation: replace, insert, or delete"},
								"start_line":     map[string]string{"type": "integer", "description": "1-indexed starting line for the patch. For insert, use 0 for the file beginning or -1 for EOF"},
								"end_line":       map[string]string{"type": "integer", "description": "Optional inclusive end line for delete operations"},
								"old":            map[string]string{"type": "string", "description": "Exact expected text for replace or delete"},
								"new":            map[string]string{"type": "string", "description": "Replacement or inserted text"},
								"context_before": map[string]string{"type": "string", "description": "Optional exact text that must appear immediately before the patch"},
								"context_after":  map[string]string{"type": "string", "description": "Optional exact text that must appear immediately after the patch"},
							},
							"required": []string{"op"},
						},
					},
				},
				"required": []string{"path", "patches"},
			},
		},
	}
	filtered := make([]mcpproto.Tool, 0, len(tools))
	for _, tool := range tools {
		if MCPProfileAllowsTool(p.profile, tool.Name) {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

func (p *hostedMCPProvider) CallTool(ctx context.Context, name string, args map[string]any) mcpproto.ToolResult {
	if p.isControlPlane() {
		return p.callControlPlaneTool(ctx, name, args)
	}
	return p.callWorkspaceTool(ctx, name, args)
}

func (p *hostedMCPProvider) callWorkspaceTool(ctx context.Context, name string, args map[string]any) mcpproto.ToolResult {
	var (
		value any
		err   error
	)
	switch name {
	case "workspace_current":
		value = map[string]any{
			"workspace": p.workspace,
			"database":  p.databaseID,
			"profile":   p.profile,
			"readonly":  p.readonly,
		}
	case "workspace_get_versioning_policy":
		value, err = p.manager.GetWorkspaceVersioningPolicy(ctx, p.databaseID, p.workspace)
		if err == nil {
			value = map[string]any{"workspace": p.workspace, "policy": value}
		}
	case "workspace_set_versioning_policy":
		err = p.ensureWritable()
		if err == nil {
			var current WorkspaceVersioningPolicy
			current, err = p.manager.GetWorkspaceVersioningPolicy(ctx, p.databaseID, p.workspace)
			if err == nil {
				var patch mcpWorkspaceVersioningPolicyPatch
				patch, err = mcpWorkspaceVersioningPolicyPatchFromArgs(args)
				if err == nil {
					value, err = p.manager.UpdateWorkspaceVersioningPolicy(ctx, p.databaseID, p.workspace, applyMCPWorkspaceVersioningPolicyPatch(current, patch))
					if err == nil {
						value = map[string]any{"workspace": p.workspace, "policy": value}
					}
				}
			}
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
					saved, err = p.manager.SaveCheckpointFromLiveWithOptions(ctx, p.databaseID, p.workspace, checkpointID, SaveCheckpointFromLiveOptions{
						Kind:   CheckpointKindManual,
						Source: CheckpointSourceMCP,
					})
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
				result, restoreErr := p.manager.RestoreCheckpointWithResult(ctx, p.databaseID, p.workspace, checkpointID)
				err = restoreErr
				payload := map[string]any{
					"workspace":  p.workspace,
					"checkpoint": checkpointID,
					"mode":       "live-workspace",
				}
				if restoreErr == nil && result.SafetyCheckpointCreated {
					payload["safety_checkpoint"] = result.SafetyCheckpointID
					payload["safety_checkpoint_created"] = true
				}
				value = payload
			}
		}
	case "file_read":
		value, err = p.toolFileRead(ctx, args)
	case "file_history":
		value, err = p.toolFileHistory(ctx, args)
	case "file_read_version":
		value, err = p.toolFileReadVersion(ctx, args)
	case "file_diff_versions":
		value, err = p.toolFileDiffVersions(ctx, args)
	case "file_restore_version":
		err = p.ensureWritable()
		if err == nil {
			value, err = p.toolFileRestoreVersion(ctx, args)
		}
	case "file_undelete":
		err = p.ensureWritable()
		if err == nil {
			value, err = p.toolFileUndelete(ctx, args)
		}
	case "file_lines":
		value, err = p.toolFileLines(ctx, args)
	case "file_list":
		value, err = p.toolFileList(ctx, args)
	case "file_glob":
		value, err = p.toolFileGlob(ctx, args)
	case "file_grep":
		value, err = p.toolFileGrep(ctx, args)
	case "file_write":
		err = p.ensureWritable()
		if err == nil {
			value, err = p.toolFileWrite(ctx, args)
		}
	case "file_create_exclusive":
		err = p.ensureWritable()
		if err == nil {
			value, err = p.toolFileCreateExclusive(ctx, args)
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
	case "file_patch":
		err = p.ensureWritable()
		if err == nil {
			value, err = p.toolFilePatch(ctx, args)
		}
	default:
		err = fmt.Errorf("unknown tool %q", name)
	}
	if err == nil && !MCPProfileAllowsTool(p.profile, name) {
		err = fmt.Errorf("tool %q is not available for mcp profile %q", name, p.profile)
	}
	if err != nil {
		return mcpErrorResult(err)
	}
	return mcpproto.StructuredResult(value)
}

func (p *hostedMCPProvider) ensureWritable() error {
	if p.readonly || MCPProfileIsReadonly(p.profile) {
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

func (p *hostedMCPProvider) toolFileHistory(ctx context.Context, args map[string]any) (any, error) {
	rawPath, err := mcpRequiredString(args, "path")
	if err != nil {
		return nil, err
	}
	direction, err := mcpStringDefault(args, "direction", "desc")
	if err != nil {
		return nil, err
	}
	newestFirst, err := mcpHistoryDirection(direction)
	if err != nil {
		return nil, err
	}
	limit, err := mcpOptionalInt(args, "limit")
	if err != nil {
		return nil, err
	}
	cursor, err := mcpOptionalString(args, "cursor")
	if err != nil {
		return nil, err
	}
	limitValue := 0
	if limit != nil {
		limitValue = *limit
	}
	history, err := p.manager.GetResolvedFileHistoryPage(ctx, p.workspace, FileHistoryRequest{
		Path:        rawPath,
		NewestFirst: newestFirst,
		Limit:       limitValue,
		Cursor:      cursor,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace": p.workspace,
		"history":   history,
	}, nil
}

func (p *hostedMCPProvider) toolFileReadVersion(ctx context.Context, args map[string]any) (any, error) {
	versionID, err := mcpOptionalString(args, "version_id")
	if err != nil {
		return nil, err
	}
	fileID, err := mcpOptionalString(args, "file_id")
	if err != nil {
		return nil, err
	}
	ordinal, err := mcpOptionalInt(args, "ordinal")
	if err != nil {
		return nil, err
	}
	var version FileVersionContentResponse
	switch {
	case versionID != "":
		version, err = p.manager.GetResolvedFileVersionContent(ctx, p.workspace, versionID)
	case fileID != "" && ordinal != nil:
		version, err = p.manager.GetResolvedFileVersionContentAtOrdinal(ctx, p.workspace, fileID, int64(*ordinal))
	default:
		err = fmt.Errorf("version_id or file_id+ordinal is required")
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace": p.workspace,
		"version":   version,
	}, nil
}

func (p *hostedMCPProvider) toolFileDiffVersions(ctx context.Context, args map[string]any) (any, error) {
	rawPath, err := mcpRequiredString(args, "path")
	if err != nil {
		return nil, err
	}
	from, err := hostedMCPDiffOperand(args, "from")
	if err != nil {
		return nil, err
	}
	to, err := hostedMCPDiffOperandWithDefault(args, "to", FileVersionDiffOperand{Ref: "head"})
	if err != nil {
		return nil, err
	}
	diff, err := p.manager.DiffResolvedFileVersions(ctx, p.workspace, rawPath, from, to)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace": p.workspace,
		"diff":      diff,
	}, nil
}

func (p *hostedMCPProvider) toolFileRestoreVersion(ctx context.Context, args map[string]any) (any, error) {
	rawPath, err := mcpRequiredString(args, "path")
	if err != nil {
		return nil, err
	}
	versionID, err := mcpOptionalString(args, "version_id")
	if err != nil {
		return nil, err
	}
	fileID, err := mcpOptionalString(args, "file_id")
	if err != nil {
		return nil, err
	}
	ordinal, err := mcpOptionalInt(args, "ordinal")
	if err != nil {
		return nil, err
	}
	selector := FileVersionSelector{
		VersionID: versionID,
		FileID:    fileID,
	}
	if ordinal != nil {
		selector.Ordinal = int64(*ordinal)
	}
	response, err := p.manager.RestoreResolvedFileVersion(ctx, p.workspace, rawPath, selector)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace": p.workspace,
		"restore":   response,
	}, nil
}

func (p *hostedMCPProvider) toolFileUndelete(ctx context.Context, args map[string]any) (any, error) {
	rawPath, err := mcpRequiredString(args, "path")
	if err != nil {
		return nil, err
	}
	versionID, err := mcpOptionalString(args, "version_id")
	if err != nil {
		return nil, err
	}
	fileID, err := mcpOptionalString(args, "file_id")
	if err != nil {
		return nil, err
	}
	ordinal, err := mcpOptionalInt(args, "ordinal")
	if err != nil {
		return nil, err
	}
	selector := FileVersionSelector{
		VersionID: versionID,
		FileID:    fileID,
	}
	if ordinal != nil {
		selector.Ordinal = int64(*ordinal)
	}
	response, err := p.manager.UndeleteResolvedFileVersion(ctx, p.workspace, rawPath, selector)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace": p.workspace,
		"undelete":  response,
	}, nil
}

func hostedMCPDiffOperand(args map[string]any, prefix string) (FileVersionDiffOperand, error) {
	return hostedMCPDiffOperandWithDefault(args, prefix, FileVersionDiffOperand{})
}

func hostedMCPDiffOperandWithDefault(args map[string]any, prefix string, fallback FileVersionDiffOperand) (FileVersionDiffOperand, error) {
	ref, err := mcpOptionalString(args, prefix+"_ref")
	if err != nil {
		return FileVersionDiffOperand{}, err
	}
	ref = strings.ToLower(strings.TrimSpace(ref))
	versionID, err := mcpOptionalString(args, prefix+"_version_id")
	if err != nil {
		return FileVersionDiffOperand{}, err
	}
	fileID, err := mcpOptionalString(args, prefix+"_file_id")
	if err != nil {
		return FileVersionDiffOperand{}, err
	}
	ordinal, err := mcpOptionalInt(args, prefix+"_ordinal")
	if err != nil {
		return FileVersionDiffOperand{}, err
	}
	operand := FileVersionDiffOperand{
		Ref:       ref,
		VersionID: versionID,
		FileID:    fileID,
	}
	if ordinal != nil {
		operand.Ordinal = int64(*ordinal)
	}
	if !hostedMCPDiffOperandProvided(operand) {
		if !hostedMCPDiffOperandProvided(fallback) {
			return FileVersionDiffOperand{}, fmt.Errorf("%s_ref, %s_version_id, or %s_file_id+%s_ordinal is required", prefix, prefix, prefix, prefix)
		}
		return fallback, nil
	}
	if err := validateHostedMCPDiffOperand(prefix, operand); err != nil {
		return FileVersionDiffOperand{}, err
	}
	return operand, nil
}

func hostedMCPDiffOperandProvided(operand FileVersionDiffOperand) bool {
	return strings.TrimSpace(operand.Ref) != "" || strings.TrimSpace(operand.VersionID) != "" || strings.TrimSpace(operand.FileID) != "" || operand.Ordinal > 0
}

func validateHostedMCPDiffOperand(prefix string, operand FileVersionDiffOperand) error {
	selectors := 0
	if strings.TrimSpace(operand.Ref) != "" {
		selectors++
		if operand.Ref != "head" && operand.Ref != "working-copy" {
			return fmt.Errorf("%s_ref must be head or working-copy", prefix)
		}
	}
	if strings.TrimSpace(operand.VersionID) != "" {
		selectors++
	}
	if strings.TrimSpace(operand.FileID) != "" || operand.Ordinal > 0 {
		if strings.TrimSpace(operand.FileID) == "" || operand.Ordinal <= 0 {
			return fmt.Errorf("%s_file_id and %s_ordinal must be used together", prefix, prefix)
		}
		selectors++
	}
	if selectors > 1 {
		return fmt.Errorf("choose exactly one %s selector", prefix)
	}
	return nil
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

func (p *hostedMCPProvider) toolFileGlob(ctx context.Context, args map[string]any) (any, error) {
	path, err := mcpStringDefault(args, "path", "/")
	if err != nil {
		return nil, err
	}
	pattern, err := mcpRequiredText(args, "pattern", false)
	if err != nil {
		return nil, err
	}
	kind, err := mcpStringDefault(args, "kind", "file")
	if err != nil {
		return nil, err
	}
	switch kind {
	case "", "any":
		kind = ""
	case "file", "dir", "symlink":
	default:
		return nil, fmt.Errorf("argument %q must be one of file, dir, symlink, or any", "kind")
	}
	limit, err := mcpInt(args, "limit", 100)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	normalizedPath := normalizeAFSGrepPath(path)
	fsClient, err := p.fsClient(ctx)
	if err != nil {
		return nil, err
	}
	matches, err := fsClient.Find(ctx, normalizedPath, pattern, kind)
	if err != nil {
		return nil, err
	}
	truncated := false
	if len(matches) > limit {
		truncated = true
		matches = matches[:limit]
	}
	items := make([]mcpFileListItem, 0, len(matches))
	for _, matchPath := range matches {
		stat, err := fsClient.Stat(ctx, matchPath)
		if err != nil {
			return nil, err
		}
		if stat == nil {
			continue
		}
		item := mcpFileListItem{
			Path:       matchPath,
			Name:       filepath.Base(matchPath),
			Kind:       stat.Type,
			Size:       stat.Size,
			ModifiedAt: mcpFileModifiedAt(stat.Mtime),
		}
		if stat.Type == "symlink" {
			target, err := fsClient.Readlink(ctx, matchPath)
			if err != nil {
				return nil, err
			}
			item.Target = target
		}
		items = append(items, item)
	}
	return map[string]any{
		"workspace": p.workspace,
		"path":      normalizedPath,
		"pattern":   pattern,
		"kind":      ternaryString(kind == "", "any", kind),
		"count":     len(items),
		"truncated": truncated,
		"items":     items,
	}, nil
}

func (p *hostedMCPProvider) toolFileWrite(ctx context.Context, args map[string]any) (any, error) {
	content, err := mcpRequiredText(args, "content", true)
	if err != nil {
		return nil, err
	}
	return p.mutateWorkspaceFile(ctx, args, func(ctx context.Context, fsClient afsclient.Client, normalizedPath string, stat *afsclient.StatResult) (map[string]any, error) {
		if stat != nil {
			if stat.Type == "dir" {
				return nil, fmt.Errorf("path %q is a directory", normalizedPath)
			}
			if stat.Type == "symlink" {
				return nil, fmt.Errorf("path %q is a symlink; write the target explicitly", normalizedPath)
			}
			if err := fsClient.Echo(ctx, normalizedPath, []byte(content)); err != nil {
				return nil, err
			}
		} else {
			if err := ensureHostedWorkspaceParentDirs(ctx, fsClient, normalizedPath); err != nil {
				return nil, err
			}
			if err := fsClient.EchoCreate(ctx, normalizedPath, []byte(content), 0o644); err != nil {
				return nil, err
			}
		}
		return map[string]any{
			"operation": "write",
			"bytes":     len(content),
		}, nil
	})
}

func (p *hostedMCPProvider) toolFileCreateExclusive(ctx context.Context, args map[string]any) (any, error) {
	content, err := mcpRequiredText(args, "content", true)
	if err != nil {
		return nil, err
	}
	return p.mutateWorkspaceFile(ctx, args, func(ctx context.Context, fsClient afsclient.Client, normalizedPath string, stat *afsclient.StatResult) (map[string]any, error) {
		if stat != nil {
			if stat.Type == "dir" {
				return nil, fmt.Errorf("path %q is a directory", normalizedPath)
			}
			return nil, fmt.Errorf("path %q already exists", normalizedPath)
		}
		if err := ensureHostedWorkspaceParentDirs(ctx, fsClient, normalizedPath); err != nil {
			return nil, err
		}
		if _, _, err := fsClient.CreateFile(ctx, normalizedPath, 0o644, true); err != nil {
			return nil, err
		}
		if err := fsClient.Echo(ctx, normalizedPath, []byte(content)); err != nil {
			return nil, err
		}
		return map[string]any{
			"operation": "create_exclusive",
			"created":   true,
			"bytes":     len(content),
		}, nil
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
	expectedOccurrences, err := mcpOptionalInt(args, "expected_occurrences")
	if err != nil {
		return nil, err
	}
	startLine, err := mcpOptionalInt(args, "start_line")
	if err != nil {
		return nil, err
	}
	contextBefore, err := mcpOptionalText(args, "context_before")
	if err != nil {
		return nil, err
	}
	contextAfter, err := mcpOptionalText(args, "context_after")
	if err != nil {
		return nil, err
	}
	return p.mutateWorkspaceFile(ctx, args, func(ctx context.Context, fsClient afsclient.Client, normalizedPath string, stat *afsclient.StatResult) (map[string]any, error) {
		if stat == nil {
			return nil, os.ErrNotExist
		}
		if stat.Type != "file" {
			return nil, fmt.Errorf("path %q is not a regular file", normalizedPath)
		}
		content, err := readWorkspaceTextContent(ctx, fsClient, normalizedPath, stat)
		if err != nil {
			return nil, err
		}
		matchCount := countTextMatches(content, oldValue, contextBefore, contextAfter, startLine, nil)
		if expectedOccurrences != nil && matchCount != *expectedOccurrences {
			return nil, fmt.Errorf("expected %d matching occurrences, found %d", *expectedOccurrences, matchCount)
		}
		var replaced int
		switch {
		case replaceAll:
			if startLine != nil || contextBefore != "" || contextAfter != "" {
				return nil, errors.New("all=true cannot be combined with start_line, context_before, or context_after")
			}
			if matchCount == 0 {
				return nil, errors.New("old text not found")
			}
			content = strings.ReplaceAll(content, oldValue, newValue)
			replaced = matchCount
		default:
			match, err := findSingleTextMatch(content, oldValue, contextBefore, contextAfter, startLine, nil)
			if err != nil {
				return nil, err
			}
			content = content[:match.Start] + newValue + content[match.End:]
			replaced = 1
		}
		if err := fsClient.Echo(ctx, normalizedPath, []byte(content)); err != nil {
			return nil, err
		}
		return map[string]any{
			"operation":    "replace",
			"replacements": replaced,
		}, nil
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
		if stat.Type != "file" {
			return nil, fmt.Errorf("path %q is not a regular file", normalizedPath)
		}
		if err := fsClient.Insert(ctx, normalizedPath, line, content); err != nil {
			return nil, err
		}
		return map[string]any{
			"operation": "insert",
			"line":      line,
			"bytes":     len(content),
		}, nil
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
	if start <= 0 || end <= 0 || end < start {
		return nil, errors.New("start and end must be >= 1 and end must be >= start")
	}
	return p.mutateWorkspaceFile(ctx, args, func(ctx context.Context, fsClient afsclient.Client, normalizedPath string, stat *afsclient.StatResult) (map[string]any, error) {
		if stat == nil {
			return nil, os.ErrNotExist
		}
		if stat.Type != "file" {
			return nil, fmt.Errorf("path %q is not a regular file", normalizedPath)
		}
		deleted, err := fsClient.DeleteLines(ctx, normalizedPath, start, end)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"operation":     "delete_lines",
			"deleted_lines": int(deleted),
		}, nil
	})
}

func (p *hostedMCPProvider) toolFilePatch(ctx context.Context, args map[string]any) (any, error) {
	var input mcpFilePatchInput
	if err := decodeMCPArgs(args, &input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Path) == "" {
		return nil, fmt.Errorf("missing required argument %q", "path")
	}
	if len(input.Patches) == 0 {
		return nil, errors.New("argument \"patches\" must not be empty")
	}
	return p.mutateWorkspaceFile(ctx, args, func(ctx context.Context, fsClient afsclient.Client, normalizedPath string, stat *afsclient.StatResult) (map[string]any, error) {
		if stat == nil {
			return nil, os.ErrNotExist
		}
		if stat.Type != "file" {
			return nil, fmt.Errorf("path %q is not a regular file", normalizedPath)
		}
		content, err := readWorkspaceTextContent(ctx, fsClient, normalizedPath, stat)
		if err != nil {
			return nil, err
		}
		if input.ExpectedSHA256 != "" {
			got := textSHA256(content)
			if !strings.EqualFold(got, input.ExpectedSHA256) {
				return nil, fmt.Errorf("expected_sha256 mismatch: got %s", got)
			}
		}
		applied := make([]map[string]any, 0, len(input.Patches))
		for i, patch := range input.Patches {
			var patchMeta map[string]any
			content, patchMeta, err = applyMCPTextPatch(content, patch)
			if err != nil {
				return nil, fmt.Errorf("patch %d: %w", i+1, err)
			}
			applied = append(applied, patchMeta)
		}
		if err := fsClient.Echo(ctx, normalizedPath, []byte(content)); err != nil {
			return nil, err
		}
		return map[string]any{
			"operation":       "patch",
			"patches_applied": len(applied),
			"applied":         applied,
			"sha256":          textSHA256(content),
		}, nil
	})
}

func (p *hostedMCPProvider) toolFileGrep(ctx context.Context, args map[string]any) (any, error) {
	pattern, err := mcpRequiredText(args, "pattern", false)
	if err != nil {
		return nil, err
	}
	opts := hostedMCPGrepOptions{
		path:    "/",
		pattern: pattern,
	}
	if opts.path, err = mcpStringDefault(args, "path", "/"); err != nil {
		return nil, err
	}
	if opts.ignoreCase, err = mcpBool(args, "ignore_case", false); err != nil {
		return nil, err
	}
	if opts.glob, err = mcpBool(args, "glob", false); err != nil {
		return nil, err
	}
	if opts.fixedStrings, err = mcpBool(args, "fixed_strings", false); err != nil {
		return nil, err
	}
	if opts.regexp, err = mcpBool(args, "regexp", false); err != nil {
		return nil, err
	}
	if opts.wordRegexp, err = mcpBool(args, "word_regexp", false); err != nil {
		return nil, err
	}
	if opts.lineRegexp, err = mcpBool(args, "line_regexp", false); err != nil {
		return nil, err
	}
	if opts.invertMatch, err = mcpBool(args, "invert_match", false); err != nil {
		return nil, err
	}
	if opts.filesWithMatches, err = mcpBool(args, "files_with_matches", false); err != nil {
		return nil, err
	}
	if opts.countOnly, err = mcpBool(args, "count", false); err != nil {
		return nil, err
	}
	if opts.maxCount, err = mcpInt(args, "max_count", 0); err != nil {
		return nil, err
	}
	modeFlags := 0
	if opts.glob {
		modeFlags++
	}
	if opts.fixedStrings {
		modeFlags++
	}
	if opts.regexp {
		modeFlags++
	}
	if modeFlags > 1 {
		return nil, errors.New("choose only one of glob, fixed_strings, or regexp")
	}
	searchPath := normalizeAFSGrepPath(opts.path)
	fsClient, err := p.fsClient(ctx)
	if err != nil {
		return nil, err
	}
	targets, err := collectHostedMCPGrepTargets(ctx, fsClient, searchPath)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, fmt.Errorf("path %q does not exist in workspace %q", searchPath, p.workspace)
		}
		return nil, err
	}
	matcher, err := compileHostedMCPGrepMatcher(opts)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"workspace": p.workspace,
		"path":      searchPath,
	}
	switch {
	case opts.filesWithMatches:
		files := make([]string, 0)
		for _, target := range targets {
			content := target.content
			if !target.loaded {
				content, err = fsClient.Cat(ctx, target.path)
				if err != nil {
					return nil, err
				}
			}
			if hostedMCPGrepFileHasMatch(content, opts, matcher) {
				files = append(files, target.path)
			}
		}
		result["mode"] = "files"
		result["files"] = files
	case opts.countOnly:
		counts := make([]mcpGrepCount, 0, len(targets))
		for _, target := range targets {
			content := target.content
			if !target.loaded {
				content, err = fsClient.Cat(ctx, target.path)
				if err != nil {
					return nil, err
				}
			}
			counts = append(counts, mcpGrepCount{
				Path:  target.path,
				Count: hostedMCPGrepFileMatchCount(content, opts, matcher),
			})
		}
		result["mode"] = "counts"
		result["counts"] = counts
	default:
		matches := make([]mcpGrepMatch, 0)
		for _, target := range targets {
			content := target.content
			if !target.loaded {
				content, err = fsClient.Cat(ctx, target.path)
				if err != nil {
					return nil, err
				}
			}
			matches = append(matches, hostedMCPCollectGrepMatches(target.path, content, opts, matcher)...)
			if opts.maxCount > 0 && len(matches) >= opts.maxCount {
				matches = matches[:opts.maxCount]
				break
			}
		}
		result["mode"] = "matches"
		result["matches"] = matches
	}
	return result, nil
}

type hostedWorkspaceContext struct {
	service   *Service
	route     workspaceCatalogRoute
	meta      WorkspaceMeta
	storageID string
	fsClient  afsclient.Client
}

func (p *hostedMCPProvider) resolveWorkspaceContext(ctx context.Context) (*hostedWorkspaceContext, error) {
	service, _, route, err := p.manager.resolveScopedWorkspace(ctx, p.databaseID, p.workspace)
	if err != nil {
		return nil, err
	}
	meta, err := service.store.GetWorkspaceMeta(ctx, route.Name)
	if err != nil {
		return nil, err
	}
	storageID := workspaceStorageID(meta)
	fsKey, _, _, err := EnsureWorkspaceRoot(ctx, service.store, route.Name)
	if err != nil {
		return nil, err
	}
	return &hostedWorkspaceContext{
		service:   service,
		route:     route,
		meta:      meta,
		storageID: storageID,
		fsClient:  afsclient.New(service.store.rdb, fsKey),
	}, nil
}

func (p *hostedMCPProvider) fsClient(ctx context.Context) (afsclient.Client, error) {
	resolved, err := p.resolveWorkspaceContext(ctx)
	if err != nil {
		return nil, err
	}
	return resolved.fsClient, nil
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
	resolved, err := p.resolveWorkspaceContext(ctx)
	if err != nil {
		return nil, err
	}
	fsClient := resolved.fsClient
	stat, err := fsClient.Stat(ctx, normalizedPath)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	if errors.Is(err, redis.Nil) {
		stat = nil
	}
	beforeSnapshot, err := hostedVersionedSnapshot(ctx, fsClient, normalizedPath, stat)
	if err != nil {
		return nil, err
	}
	payload, err := mutate(ctx, fsClient, normalizedPath, stat)
	if err != nil {
		return nil, err
	}
	if err := MarkWorkspaceRootDirty(ctx, resolved.service.store, resolved.storageID); err != nil {
		return nil, err
	}
	resolved.meta.DirtyHint = true
	if err := resolved.service.store.PutWorkspaceMeta(ctx, resolved.meta); err != nil {
		return nil, err
	}
	updatedStat, statErr := fsClient.Stat(ctx, normalizedPath)
	if statErr != nil && !errors.Is(statErr, redis.Nil) {
		return nil, statErr
	}
	payload["workspace"] = p.workspace
	payload["path"] = normalizedPath
	payload["dirty"] = true
	afterSnapshot, err := hostedVersionedSnapshot(ctx, fsClient, normalizedPath, updatedStat)
	if err != nil {
		return nil, err
	}
	if updatedStat != nil {
		payload["kind"] = updatedStat.Type
		payload["size"] = updatedStat.Size
		payload["modified_at"] = mcpFileModifiedAt(updatedStat.Mtime)
	}
	template := resolved.service.buildChangelogTemplate(ctx, resolved.storageID, strings.TrimSpace(resolved.meta.HeadSavepoint), ChangeSourceMCP)
	version, err := resolved.service.store.RecordFileVersionMutation(ctx, resolved.storageID, beforeSnapshot, afterSnapshot, FileVersionMutationMetadata{
		Source:       ChangeSourceMCP,
		SessionID:    template.SessionID,
		AgentID:      template.AgentID,
		User:         template.User,
		CheckpointID: strings.TrimSpace(resolved.meta.HeadSavepoint),
	})
	if err != nil {
		return nil, err
	}
	entry := template
	entry.Path = normalizedPath
	entry.Op = ChangeOpPut
	entry.PrevHash = beforeSnapshot.ContentHash
	entry.DeltaBytes = -beforeSnapshot.SizeBytes
	if afterSnapshot.Exists {
		entry.ContentHash = afterSnapshot.ContentHash
		entry.SizeBytes = afterSnapshot.SizeBytes
		entry.DeltaBytes = afterSnapshot.SizeBytes - beforeSnapshot.SizeBytes
		entry.Mode = afterSnapshot.Mode
	}
	if version != nil {
		payload["file_id"] = version.FileID
		payload["version_id"] = version.VersionID
		entry.FileID = version.FileID
		entry.VersionID = version.VersionID
	}
	WriteChangeEntries(ctx, resolved.service.store.rdb, resolved.storageID, []ChangeEntry{entry})
	return payload, nil
}

func ensureHostedWorkspaceParentDirs(ctx context.Context, fsClient afsclient.Client, normalizedPath string) error {
	trimmed := strings.Trim(normalizedPath, "/")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) <= 1 {
		return nil
	}
	current := ""
	for _, part := range parts[:len(parts)-1] {
		current += "/" + part
		if stat, err := fsClient.Stat(ctx, current); err == nil && stat != nil {
			continue
		} else if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		if err := fsClient.Mkdir(ctx, current); err != nil {
			return err
		}
	}
	return nil
}

func hostedVersionedSnapshot(ctx context.Context, fsClient afsclient.Client, normalizedPath string, stat *afsclient.StatResult) (VersionedFileSnapshot, error) {
	snapshot := VersionedFileSnapshot{Path: normalizedPath}
	if stat == nil {
		return snapshot, nil
	}
	snapshot.Exists = true
	snapshot.Kind = stat.Type
	snapshot.Mode = stat.Mode
	switch stat.Type {
	case "file":
		content, err := fsClient.Cat(ctx, normalizedPath)
		if err != nil {
			return VersionedFileSnapshot{}, err
		}
		snapshot.Content = content
		snapshot.SizeBytes = int64(len(content))
	case "symlink":
		target, err := fsClient.Readlink(ctx, normalizedPath)
		if err != nil {
			return VersionedFileSnapshot{}, err
		}
		snapshot.Target = target
	default:
		snapshot.Exists = false
	}
	return completeVersionedSnapshot(snapshot), nil
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

func mcpHistoryDirection(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "desc":
		return true, nil
	case "asc":
		return false, nil
	default:
		return false, fmt.Errorf("direction must be asc or desc")
	}
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
