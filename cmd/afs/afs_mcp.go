package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/redis/agent-filesystem/internal/controlplane"
	"github.com/redis/agent-filesystem/internal/mcpproto"
	"github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

const afsMCPProtocolVersion = "2024-11-05"

type afsMCPServer struct {
	cfg             config
	store           *afsStore
	service         *controlplane.Service
	profile         string
	workspaceLocked string
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

type mcpGrepCount struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

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
	Workspace      string           `json:"workspace,omitempty"`
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

func (s *afsMCPServer) effectiveProfile() string {
	profile, err := controlplane.NormalizeMCPProfile(s.profile)
	if err != nil {
		return controlplane.MCPProfileWorkspaceRW
	}
	return profile
}

func (s *afsMCPServer) effectiveWorkspaceLock() string {
	if strings.TrimSpace(s.workspaceLocked) != "" {
		return strings.TrimSpace(s.workspaceLocked)
	}
	if controlplane.MCPProfileIsWorkspaceBound(s.effectiveProfile()) {
		return selectedWorkspaceName(s.cfg)
	}
	return ""
}

func cmdMCP(args []string) error {
	bin := filepath.Base(os.Args[0])
	if len(args) > 1 && isHelpArg(args[1]) {
		fmt.Fprint(os.Stderr, mcpUsageText(bin))
		return nil
	}
	workspaceFlag := ""
	profileFlag := ""
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--workspace":
			if i+1 >= len(args) {
				return fmt.Errorf("missing value for --workspace\n\n%s", mcpUsageText(bin))
			}
			workspaceFlag = strings.TrimSpace(args[i+1])
			i++
		case "--profile":
			if i+1 >= len(args) {
				return fmt.Errorf("missing value for --profile\n\n%s", mcpUsageText(bin))
			}
			profileFlag = strings.TrimSpace(args[i+1])
			i++
		default:
			return fmt.Errorf("unknown mcp flag %q\n\n%s", args[i], mcpUsageText(bin))
		}
	}

	cfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()
	profile, err := controlplane.NormalizeMCPProfile(profileFlag)
	if err != nil {
		return err
	}
	workspaceLocked := strings.TrimSpace(workspaceFlag)
	if workspaceLocked == "" && controlplane.MCPProfileIsWorkspaceBound(profile) {
		workspaceLocked, err = resolveWorkspaceName(context.Background(), cfg, store, "")
		if err != nil {
			return fmt.Errorf("workspace-bound mcp profile %q requires a selected workspace: %w", profile, err)
		}
	}

	server := &afsMCPServer{
		cfg:             cfg,
		store:           store,
		service:         controlPlaneServiceFromStore(cfg, store),
		profile:         profile,
		workspaceLocked: workspaceLocked,
	}
	return server.protocolServer().Serve(context.Background(), os.Stdin, os.Stdout)
}

func mcpUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s mcp [--workspace <name>] [--profile <profile>]

Start the Agent Filesystem MCP server over stdio.

Profiles:
  workspace-ro              Workspace-bound read-only file tools
  workspace-rw              Workspace-bound read/write file tools (default)
  workspace-rw-checkpoint   Workspace-bound file tools plus checkpoints
  admin-ro                  Broad read-only MCP surface
  admin-rw                  Broad read/write MCP surface

This command is meant to be launched by an MCP client, for example:

  {
    "mcpServers": {
      "afs": {
        "command": "/absolute/path/to/%s",
        "args": ["mcp", "--workspace", "my-workspace", "--profile", "workspace-rw"]
      }
    }
  }
`, bin, bin)
}

func (s *afsMCPServer) protocolServer() *mcpproto.Server {
	instructions := "Workspace-first Agent Filesystem MCP server."
	if controlplane.MCPProfileIsWorkspaceBound(s.effectiveProfile()) {
		instructions = fmt.Sprintf("Workspace-bound Agent Filesystem MCP server for %s with profile %s. Use file tools only within the locked workspace.", s.effectiveWorkspaceLock(), s.effectiveProfile())
	} else {
		instructions = fmt.Sprintf("Agent Filesystem admin MCP server with profile %s.", s.effectiveProfile())
	}
	return &mcpproto.Server{
		ProtocolVersion: afsMCPProtocolVersion,
		Name:            "afs",
		Version:         "0.1.0",
		Instructions:    instructions,
		Provider:        s,
	}
}

func (s *afsMCPServer) serve(ctx context.Context, r io.Reader, w io.Writer) error {
	return s.protocolServer().Serve(ctx, r, w)
}

func (s *afsMCPServer) Tools(_ context.Context) []mcpproto.Tool {
	tools := []mcpproto.Tool{
		{
			Name:        "afs_status",
			Description: "Show the current AFS configuration and selected workspace",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "workspace_list",
			Description: "List AFS workspaces stored in Redis",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "workspace_create",
			Description: "Create a new empty workspace",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace":   map[string]string{"type": "string", "description": "Workspace name"},
					"description": map[string]string{"type": "string", "description": "Optional description"},
				},
				"required": []string{"workspace"},
			},
		},
		{
			Name:        "workspace_fork",
			Description: "Fork a workspace from its current checkpoint into a new workspace",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace":     map[string]string{"type": "string", "description": "Source workspace"},
					"new_workspace": map[string]string{"type": "string", "description": "New workspace name"},
				},
				"required": []string{"workspace", "new_workspace"},
			},
		},
		{
			Name:        "checkpoint_list",
			Description: "List checkpoints for a workspace",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]string{"type": "string", "description": "Workspace name"},
				},
			},
		},
		{
			Name:        "checkpoint_create",
			Description: "Create a new checkpoint from workspace state",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace":  map[string]string{"type": "string", "description": "Workspace name"},
					"checkpoint": map[string]string{"type": "string", "description": "Optional checkpoint name"},
				},
			},
		},
		{
			Name:        "checkpoint_restore",
			Description: "Restore workspace state to a checkpoint",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace":  map[string]string{"type": "string", "description": "Workspace name"},
					"checkpoint": map[string]string{"type": "string", "description": "Checkpoint name"},
				},
				"required": []string{"checkpoint"},
			},
		},
		{
			Name:        "file_read",
			Description: "Read a file or symlink from a workspace. Use this for whole-file reads when you need the complete current contents. Do not use this for partial text reads (use file_lines), directory discovery (use file_list), or content search across files (use file_grep). Paths must be absolute inside the workspace, for example /src/main.go.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]string{"type": "string", "description": "Workspace name"},
					"path":      map[string]string{"type": "string", "description": "Absolute workspace path to a file or symlink, for example /src/main.go"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "file_lines",
			Description: "Read a specific line range from a text file. Use this instead of file_read when the file is large or you only need a slice. This is for text files only. Do not use it for directory listing or cross-file search. Paths must be absolute inside the workspace, for example /src/main.go.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]string{"type": "string", "description": "Workspace name"},
					"path":      map[string]string{"type": "string", "description": "Absolute workspace path to a text file, for example /src/main.go"},
					"start":     map[string]string{"type": "integer", "description": "Start line, 1-indexed"},
					"end":       map[string]string{"type": "integer", "description": "End line, inclusive. Use -1 to read through EOF"},
				},
				"required": []string{"path", "start"},
			},
		},
		{
			Name:        "file_list",
			Description: "List files and directories under a workspace path. Use this for structure discovery and navigation. Do not use it for filename pattern matching or content search; use a dedicated glob-style tool or file_grep instead. Paths must be absolute inside the workspace, for example / or /src.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]string{"type": "string", "description": "Workspace name"},
					"path":      map[string]string{"type": "string", "description": "Absolute workspace directory path, for example / or /src", "default": "/"},
					"depth":     map[string]string{"type": "integer", "description": "Depth relative to the requested path. Use 1 for immediate children", "default": "1"},
				},
			},
		},
		{
			Name:        "file_glob",
			Description: "Find files or directories under a workspace path by basename glob pattern. Use this for filename discovery before reading or editing. Do not use it for content search; use file_grep instead. The search path must be an absolute directory path inside the workspace, for example / or /src.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]string{"type": "string", "description": "Workspace name"},
					"path":      map[string]string{"type": "string", "description": "Absolute workspace directory path to search within, for example / or /src", "default": "/"},
					"pattern":   map[string]string{"type": "string", "description": "Basename glob pattern, for example *.go or [Mm]akefile"},
					"kind":      map[string]string{"type": "string", "description": "Optional kind filter: file, dir, symlink, or any", "default": "file"},
					"limit":     map[string]string{"type": "integer", "description": "Maximum number of results to return", "default": "100"},
				},
				"required": []string{"pattern"},
			},
		},
		{
			Name:        "file_write",
			Description: "Write a full file in a workspace, creating parent directories as needed. Use this for new files or full overwrites. Do not use it for small localized edits; prefer file_replace, file_insert, or file_delete_lines for that. File edits update the workspace immediately and leave it dirty until checkpoint_create is called. Paths must be absolute inside the workspace, for example /src/main.go.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]string{"type": "string", "description": "Workspace name"},
					"path":      map[string]string{"type": "string", "description": "Absolute workspace file path, for example /src/main.go"},
					"content":   map[string]string{"type": "string", "description": "Complete file contents to write"},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			Name:        "file_create_exclusive",
			Description: "Atomically create a file only if it does not already exist; fails if the path is already taken. Useful for distributed locking and coordination between agents. Creates parent directories as needed. Leaves the workspace dirty until checkpoint_create is called.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]string{"type": "string", "description": "Workspace name"},
					"path":      map[string]string{"type": "string", "description": "Absolute file path"},
					"content":   map[string]string{"type": "string", "description": "File contents to write on creation"},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			Name:        "file_replace",
			Description: "Replace text in a file. Use this for small exact substitutions after you have inspected the file. Do not use it for full rewrites; use file_write instead. If the target text may occur more than once, callers should be explicit about whether all occurrences are intended. File edits update the workspace immediately and leave it dirty until checkpoint_create is called.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace":            map[string]string{"type": "string", "description": "Workspace name"},
					"path":                 map[string]string{"type": "string", "description": "Absolute workspace file path, for example /src/main.go"},
					"old":                  map[string]string{"type": "string", "description": "Exact text to find"},
					"new":                  map[string]string{"type": "string", "description": "Replacement text"},
					"all":                  map[string]string{"type": "boolean", "description": "Replace all occurrences instead of a single occurrence"},
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
			Description: "Insert content at a line boundary in a text file. Use this for additive edits where an exact insertion point is known. Do not use it for broad rewrites or ambiguous structural edits. File edits update the workspace immediately and leave it dirty until checkpoint_create is called.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]string{"type": "string", "description": "Workspace name"},
					"path":      map[string]string{"type": "string", "description": "Absolute workspace file path, for example /src/main.go"},
					"line":      map[string]string{"type": "integer", "description": "Insert after this line. Use 0 for the beginning of the file and -1 for the end"},
					"content":   map[string]string{"type": "string", "description": "Content to insert"},
				},
				"required": []string{"path", "line", "content"},
			},
		},
		{
			Name:        "file_delete_lines",
			Description: "Delete a line range from a text file. Use this for precise removals when line numbers are known. Do not use it for semantic search-and-replace; use file_replace instead. File edits update the workspace immediately and leave it dirty until checkpoint_create is called.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]string{"type": "string", "description": "Workspace name"},
					"path":      map[string]string{"type": "string", "description": "Absolute workspace file path, for example /src/main.go"},
					"start":     map[string]string{"type": "integer", "description": "Start line to delete, 1-indexed"},
					"end":       map[string]string{"type": "integer", "description": "End line to delete, inclusive"},
				},
				"required": []string{"path", "start", "end"},
			},
		},
		{
			Name:        "file_patch",
			Description: "Apply one or more structured text patches to a file. Use this for precise multi-step edits where exact context matters. This tool supports replace, insert, and delete operations with optional line anchors, surrounding context checks, and a file hash precondition. File edits update the workspace immediately and leave it dirty until checkpoint_create is called.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace":       map[string]string{"type": "string", "description": "Workspace name"},
					"path":            map[string]string{"type": "string", "description": "Absolute workspace file path, for example /src/main.go"},
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
		{
			Name:        "file_grep",
			Description: "Search file contents in a workspace using the same engine as afs fs grep. Use this for content search across one file or many files. Do not use it for directory discovery or filename-only matching. The search path must be absolute inside the workspace, for example / or /src. Choose only one search mode among glob, fixed_strings, or regexp.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace":          map[string]string{"type": "string", "description": "Workspace name"},
					"path":               map[string]string{"type": "string", "description": "Absolute workspace path to search within, for example / or /src", "default": "/"},
					"pattern":            map[string]string{"type": "string", "description": "Pattern to search for"},
					"ignore_case":        map[string]string{"type": "boolean", "description": "Case-insensitive search"},
					"glob":               map[string]string{"type": "boolean", "description": "Use AFS glob matching semantics for the pattern"},
					"fixed_strings":      map[string]string{"type": "boolean", "description": "Treat the pattern as a fixed string"},
					"regexp":             map[string]string{"type": "boolean", "description": "Use regex mode with RE2 syntax"},
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
	}
	filtered := make([]mcpproto.Tool, 0, len(tools))
	for _, tool := range tools {
		if controlplane.MCPProfileAllowsTool(s.effectiveProfile(), tool.Name) {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

func (s *afsMCPServer) CallTool(ctx context.Context, name string, args map[string]any) mcpproto.ToolResult {
	var (
		value any
		err   error
	)
	if !controlplane.MCPProfileAllowsTool(s.effectiveProfile(), name) {
		return mcpproto.ToolResult{
			Content: []mcpproto.TextContent{{
				Type: "text",
				Text: fmt.Sprintf("tool %q is not available for mcp profile %q", name, s.effectiveProfile()),
			}},
			IsError: true,
		}
	}

	switch name {
	case "afs_status":
		value, err = s.toolAFSStatus()
	case "workspace_list":
		value, err = s.toolWorkspaceList(ctx)
	case "workspace_create":
		value, err = s.toolWorkspaceCreate(ctx, args)
	case "workspace_fork":
		value, err = s.toolWorkspaceFork(ctx, args)
	case "checkpoint_list":
		value, err = s.toolCheckpointList(ctx, args)
	case "checkpoint_create":
		value, err = s.toolCheckpointCreate(ctx, args)
	case "checkpoint_restore":
		value, err = s.toolCheckpointRestore(ctx, args)
	case "file_read":
		value, err = s.toolFileRead(ctx, args)
	case "file_lines":
		value, err = s.toolFileLines(ctx, args)
	case "file_list":
		value, err = s.toolFileList(ctx, args)
	case "file_glob":
		value, err = s.toolFileGlob(ctx, args)
	case "file_write":
		value, err = s.toolFileWrite(ctx, args)
	case "file_create_exclusive":
		value, err = s.toolFileCreateExclusive(ctx, args)
	case "file_replace":
		value, err = s.toolFileReplace(ctx, args)
	case "file_insert":
		value, err = s.toolFileInsert(ctx, args)
	case "file_delete_lines":
		value, err = s.toolFileDeleteLines(ctx, args)
	case "file_patch":
		value, err = s.toolFilePatch(ctx, args)
	case "file_grep":
		value, err = s.toolFileGrep(ctx, args)
	default:
		err = fmt.Errorf("unknown tool %q", name)
	}

	if err != nil {
		return mcpproto.ToolResult{
			Content: []mcpproto.TextContent{{
				Type: "text",
				Text: err.Error(),
			}},
			IsError: true,
		}
	}
	return mcpproto.StructuredResult(value)
}

func (s *afsMCPServer) callTool(ctx context.Context, name string, args map[string]any) mcpproto.ToolResult {
	return s.CallTool(ctx, name, args)
}

func readMCPFrame(r *bufio.Reader) ([]byte, error) {
	return mcpproto.ReadFrame(r)
}

func (s *afsMCPServer) toolAFSStatus() (any, error) {
	return map[string]any{
		"redis_addr":        s.cfg.RedisAddr,
		"redis_db":          s.cfg.RedisDB,
		"current_workspace": selectedWorkspaceName(s.cfg),
		"workspace_locked":  s.effectiveWorkspaceLock(),
		"profile":           s.effectiveProfile(),
		"mount_backend":     s.cfg.MountBackend,
		"local_path":        s.cfg.LocalPath,
	}, nil
}

func (s *afsMCPServer) toolWorkspaceList(ctx context.Context) (any, error) {
	return s.service.ListWorkspaceSummaries(ctx)
}

func (s *afsMCPServer) toolWorkspaceCurrent(ctx context.Context) (any, error) {
	if s.effectiveWorkspaceLock() != "" {
		return map[string]any{
			"workspace": s.effectiveWorkspaceLock(),
			"exists":    true,
			"locked":    true,
			"profile":   s.effectiveProfile(),
		}, nil
	}
	workspace := selectedWorkspaceName(s.cfg)
	exists := false
	if workspace != "" {
		var err error
		exists, err = s.store.workspaceExists(ctx, workspace)
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"workspace": workspace,
		"exists":    exists,
	}, nil
}

func (s *afsMCPServer) toolWorkspaceUse(ctx context.Context, args map[string]any) (any, error) {
	workspace, err := mcpRequiredString(args, "workspace")
	if err != nil {
		return nil, err
	}
	if err := validateAFSName("workspace", workspace); err != nil {
		return nil, err
	}
	exists, err := s.store.workspaceExists(ctx, workspace)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("workspace %q does not exist", workspace)
	}
	s.cfg.CurrentWorkspace = workspace
	if err := prepareConfigForSave(&s.cfg); err != nil {
		return nil, err
	}
	if err := saveConfig(s.cfg); err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace": workspace,
		"config":    compactDisplayPath(configPath()),
	}, nil
}

func (s *afsMCPServer) toolWorkspaceCreate(ctx context.Context, args map[string]any) (any, error) {
	workspace, err := mcpRequiredString(args, "workspace")
	if err != nil {
		return nil, err
	}
	description, err := mcpOptionalString(args, "description")
	if err != nil {
		return nil, err
	}
	detail, err := s.service.CreateWorkspace(ctx, controlplane.CreateWorkspaceRequest{
		Name:        workspace,
		Description: description,
		Source: controlplane.SourceRef{
			Kind: controlplane.SourceBlank,
		},
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace": detail,
	}, nil
}

func (s *afsMCPServer) toolWorkspaceFork(ctx context.Context, args map[string]any) (any, error) {
	workspace, err := s.resolveWorkspaceArg(ctx, args, "workspace")
	if err != nil {
		return nil, err
	}
	newWorkspace, err := mcpRequiredString(args, "new_workspace")
	if err != nil {
		return nil, err
	}
	if err := s.service.ForkWorkspace(ctx, workspace, newWorkspace); err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace":     workspace,
		"new_workspace": newWorkspace,
	}, nil
}

func (s *afsMCPServer) toolCheckpointList(ctx context.Context, args map[string]any) (any, error) {
	workspace, err := s.resolveWorkspaceArg(ctx, args, "workspace")
	if err != nil {
		return nil, err
	}
	checkpoints, err := s.service.ListCheckpoints(ctx, workspace, 100)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace":   workspace,
		"checkpoints": checkpoints,
	}, nil
}

func (s *afsMCPServer) toolCheckpointCreate(ctx context.Context, args map[string]any) (any, error) {
	workspace, err := s.resolveWorkspaceArg(ctx, args, "workspace")
	if err != nil {
		return nil, err
	}
	checkpointID, err := mcpOptionalString(args, "checkpoint")
	if err != nil {
		return nil, err
	}
	if checkpointID == "" {
		checkpointID = generatedSavepointName()
	}
	if err := validateAFSName("checkpoint", checkpointID); err != nil {
		return nil, err
	}
	saved, err := saveAFSWorkspaceOrLiveRoot(ctx, s.cfg, s.store, workspace, checkpointID, false, controlplane.SaveCheckpointFromLiveOptions{
		Kind:   controlplane.CheckpointKindManual,
		Source: controlplane.CheckpointSourceMCP,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace":   workspace,
		"checkpoint":  checkpointID,
		"created":     saved,
		"description": ternaryString(saved, "checkpoint created", "no changes to checkpoint"),
	}, nil
}

func (s *afsMCPServer) toolCheckpointRestore(ctx context.Context, args map[string]any) (any, error) {
	workspace, err := s.resolveWorkspaceArg(ctx, args, "workspace")
	if err != nil {
		return nil, err
	}
	checkpointID, err := mcpRequiredString(args, "checkpoint")
	if err != nil {
		return nil, err
	}
	result, err := resetAFSWorkspaceHead(ctx, s.service, workspace, checkpointID)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"workspace":  workspace,
		"checkpoint": checkpointID,
		"mode":       "live-workspace",
	}
	if result.SafetyCheckpointCreated {
		payload["safety_checkpoint"] = result.SafetyCheckpointID
		payload["safety_checkpoint_created"] = true
	}
	return payload, nil
}

func (s *afsMCPServer) toolFileRead(ctx context.Context, args map[string]any) (any, error) {
	workspace, normalizedPath, fsClient, stat, err := s.resolveWorkspaceFSPath(ctx, args, true)
	if err != nil {
		return nil, err
	}
	return readWorkspaceFSEntry(ctx, workspace, normalizedPath, fsClient, stat)
}

func (s *afsMCPServer) toolFileLines(ctx context.Context, args map[string]any) (any, error) {
	workspace, normalizedPath, fsClient, stat, err := s.resolveWorkspaceFSPath(ctx, args, true)
	if err != nil {
		return nil, err
	}
	if stat.Type != "file" {
		return nil, fmt.Errorf("path %q is not a regular file", normalizedPath)
	}
	start, err := mcpInt(args, "start", 0)
	if err != nil {
		return nil, err
	}
	if start <= 0 {
		return nil, errors.New("start must be >= 1")
	}
	end, err := mcpInt(args, "end", -1)
	if err != nil {
		return nil, err
	}
	content, err := fsClient.Cat(ctx, normalizedPath)
	if err != nil {
		return nil, err
	}
	if grepBinaryPrefix(content) {
		return nil, fmt.Errorf("path %q is binary", normalizedPath)
	}
	lines := splitTextLines(string(content))
	if start > len(lines) {
		return map[string]any{
			"workspace": workspace,
			"path":      normalizedPath,
			"start":     start,
			"end":       end,
			"content":   "",
		}, nil
	}
	if end < 0 || end > len(lines) {
		end = len(lines)
	}
	if end < start {
		end = start - 1
	}
	segment := ""
	if end >= start {
		segment = strings.Join(lines[start-1:end], "")
	}
	return map[string]any{
		"workspace": workspace,
		"path":      normalizedPath,
		"start":     start,
		"end":       end,
		"content":   segment,
	}, nil
}

func (s *afsMCPServer) toolFileList(ctx context.Context, args map[string]any) (any, error) {
	workspace, normalizedPath, fsClient, stat, err := s.resolveWorkspaceFSPath(ctx, args, false)
	if err != nil {
		return nil, err
	}
	if stat.Type != "dir" {
		return nil, fmt.Errorf("path %q is not a directory", normalizedPath)
	}
	depth, err := mcpInt(args, "depth", 1)
	if err != nil {
		return nil, err
	}
	if depth <= 0 {
		depth = 1
	}
	items, err := listWorkspaceFSEntries(ctx, fsClient, normalizedPath, depth)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace": workspace,
		"path":      normalizedPath,
		"depth":     depth,
		"items":     items,
	}, nil
}

func (s *afsMCPServer) toolFileGlob(ctx context.Context, args map[string]any) (any, error) {
	workspace, err := s.resolveWorkspaceArg(ctx, args, "workspace")
	if err != nil {
		return nil, err
	}
	pattern, err := mcpRequiredText(args, "pattern", false)
	if err != nil {
		return nil, err
	}
	rawPath, err := mcpStringDefault(args, "path", "/")
	if err != nil {
		return nil, err
	}
	normalizedPath := normalizeAFSGrepPath(rawPath)
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

	fsKey, _, _, err := s.store.ensureWorkspaceRoot(ctx, workspace)
	if err != nil {
		return nil, err
	}
	fsClient := client.New(s.store.rdb, fsKey)
	stat, err := fsClient.Stat(ctx, normalizedPath)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	if stat == nil {
		return nil, os.ErrNotExist
	}
	if stat.Type != "dir" {
		return nil, fmt.Errorf("path %q is not a directory", normalizedPath)
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
		item, err := workspaceFileListItem(ctx, fsClient, matchPath)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			if items[i].Kind == "dir" {
				return true
			}
			if items[j].Kind == "dir" {
				return false
			}
		}
		return items[i].Path < items[j].Path
	})
	return map[string]any{
		"workspace": workspace,
		"path":      normalizedPath,
		"pattern":   pattern,
		"kind":      ternaryString(kind == "", "any", kind),
		"count":     len(items),
		"truncated": truncated,
		"items":     items,
	}, nil
}

func (s *afsMCPServer) toolFileWrite(ctx context.Context, args map[string]any) (any, error) {
	content, err := mcpRequiredText(args, "content", true)
	if err != nil {
		return nil, err
	}
	return s.mutateWorkspaceFile(ctx, args, func(ctx context.Context, fsClient client.Client, normalizedPath string, stat *client.StatResult) (map[string]any, error) {
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
			if err := ensureWorkspaceParentDirs(ctx, fsClient, normalizedPath); err != nil {
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

func (s *afsMCPServer) toolFileCreateExclusive(ctx context.Context, args map[string]any) (any, error) {
	content, err := mcpRequiredText(args, "content", true)
	if err != nil {
		return nil, err
	}
	workspace, err := s.resolveWorkspaceArg(ctx, args, "workspace")
	if err != nil {
		return nil, err
	}
	rawPath, err := mcpRequiredString(args, "path")
	if err != nil {
		return nil, err
	}
	normalizedPath := normalizeAFSGrepPath(rawPath)

	fsKey, _, _, err := s.store.ensureWorkspaceRoot(ctx, workspace)
	if err != nil {
		return nil, err
	}
	fsClient := client.New(s.store.rdb, fsKey)

	// Check if path is already a directory.
	if stat, statErr := fsClient.Stat(ctx, normalizedPath); statErr == nil && stat != nil {
		if stat.Type == "dir" {
			return nil, fmt.Errorf("path %q is a directory", normalizedPath)
		}
		return nil, fmt.Errorf("path %q already exists", normalizedPath)
	}

	if err := ensureWorkspaceParentDirs(ctx, fsClient, normalizedPath); err != nil {
		return nil, err
	}

	// Atomic exclusive create via CreateFile (backed by HSETNX in Redis).
	// When exclusive=true, CreateFile returns an error if the file already
	// exists (via the HSETNX race), so we only need the error check.
	_, _, err = fsClient.CreateFile(ctx, normalizedPath, 0o644, true)
	if err != nil {
		return nil, err
	}

	// Write the content into the newly created file.
	if err := fsClient.Echo(ctx, normalizedPath, []byte(content)); err != nil {
		return nil, err
	}

	dirty, err := s.refreshWorkspaceLiveState(ctx, workspace)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"operation": "create_exclusive",
		"created":   true,
		"workspace": workspace,
		"path":      normalizedPath,
		"bytes":     len(content),
		"dirty":     dirty,
	}, nil
}

func (s *afsMCPServer) toolFileReplace(ctx context.Context, args map[string]any) (any, error) {
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
	return s.mutateWorkspaceFile(ctx, args, func(ctx context.Context, fsClient client.Client, normalizedPath string, stat *client.StatResult) (map[string]any, error) {
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

func (s *afsMCPServer) toolFileInsert(ctx context.Context, args map[string]any) (any, error) {
	insertAfter, err := mcpInt(args, "line", 0)
	if err != nil {
		return nil, err
	}
	content, err := mcpRequiredText(args, "content", true)
	if err != nil {
		return nil, err
	}
	return s.mutateWorkspaceFile(ctx, args, func(ctx context.Context, fsClient client.Client, normalizedPath string, stat *client.StatResult) (map[string]any, error) {
		if stat == nil {
			return nil, os.ErrNotExist
		}
		if stat.Type != "file" {
			return nil, fmt.Errorf("path %q is not a regular file", normalizedPath)
		}
		if err := fsClient.Insert(ctx, normalizedPath, insertAfter, content); err != nil {
			return nil, err
		}
		return map[string]any{
			"operation": "insert",
			"line":      insertAfter,
		}, nil
	})
}

func (s *afsMCPServer) toolFileDeleteLines(ctx context.Context, args map[string]any) (any, error) {
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
	return s.mutateWorkspaceFile(ctx, args, func(ctx context.Context, fsClient client.Client, normalizedPath string, stat *client.StatResult) (map[string]any, error) {
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

func (s *afsMCPServer) toolFilePatch(ctx context.Context, args map[string]any) (any, error) {
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

	return s.mutateWorkspaceFile(ctx, args, func(ctx context.Context, fsClient client.Client, normalizedPath string, stat *client.StatResult) (map[string]any, error) {
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

func (s *afsMCPServer) toolFileGrep(ctx context.Context, args map[string]any) (any, error) {
	workspace, err := s.resolveWorkspaceArg(ctx, args, "workspace")
	if err != nil {
		return nil, err
	}
	pattern, err := mcpRequiredText(args, "pattern", false)
	if err != nil {
		return nil, err
	}

	opts := grepOptions{
		workspace:       workspace,
		path:            "/",
		showLineNumbers: true,
		patterns:        []string{pattern},
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
	regexpMode, err := mcpBool(args, "regexp", false)
	if err != nil {
		return nil, err
	}
	opts.extendedRegexp = regexpMode
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
	if opts.extendedRegexp {
		modeFlags++
	}
	if modeFlags > 1 {
		return nil, errors.New("choose only one of glob, fixed_strings, or regexp")
	}

	searchPath := normalizeAFSGrepPath(opts.path)
	fsKey, exists, err := resolveWorkspaceFSKey(ctx, s.cfg, s.store, workspace)
	if err != nil {
		return nil, err
	}
	if !exists {
		return s.grepLocalWorkspace(ctx, workspace, searchPath, opts)
	}

	fsClient := client.New(s.store.rdb, fsKey)
	if useFastGrepBackend(opts) {
		matches, err := fsClient.Grep(ctx, searchPath, literalGlobPattern(pattern), opts.ignoreCase)
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return nil, fmt.Errorf("path %q does not exist in workspace %q", searchPath, workspace)
			}
			return nil, err
		}
		out := make([]mcpGrepMatch, 0, len(matches))
		for _, match := range matches {
			out = append(out, mcpGrepMatch{
				Path: match.Path,
				Line: match.LineNum,
				Text: match.Line,
			})
		}
		return map[string]any{
			"workspace": workspace,
			"path":      searchPath,
			"mode":      "matches",
			"matches":   out,
		}, nil
	}

	matcher, err := compileGrepMatcher(opts)
	if err != nil {
		return nil, err
	}
	targets, err := collectGrepTargets(ctx, fsClient, searchPath)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, fmt.Errorf("path %q does not exist in workspace %q", searchPath, workspace)
		}
		return nil, err
	}

	result := map[string]any{
		"workspace": workspace,
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
			if grepFileHasMatch(content, opts, matcher) {
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
				Count: grepFileMatchCount(content, opts, matcher),
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
			matches = append(matches, grepCollectMatches(target.path, content, opts, matcher)...)
		}
		result["mode"] = "matches"
		result["matches"] = matches
	}

	return result, nil
}

func (s *afsMCPServer) grepLocalWorkspace(ctx context.Context, workspace, searchPath string, opts grepOptions) (any, error) {
	workspaceMeta, err := s.store.getWorkspaceMeta(ctx, workspace)
	if err != nil {
		return nil, err
	}
	manifestValue, err := s.store.getManifest(ctx, workspace, workspaceMeta.HeadSavepoint)
	if err != nil {
		return nil, err
	}
	targets, err := collectManifestGrepTargets(ctx, s.store, workspace, manifestValue, searchPath)
	if err != nil {
		return nil, err
	}
	matcher, err := compileGrepMatcher(opts)
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"workspace": workspace,
		"path":      searchPath,
	}
	switch {
	case opts.filesWithMatches:
		files := make([]string, 0)
		for _, target := range targets {
			if grepFileHasMatch(target.content, opts, matcher) {
				files = append(files, target.path)
			}
		}
		result["mode"] = "files"
		result["files"] = files
	case opts.countOnly:
		counts := make([]mcpGrepCount, 0, len(targets))
		for _, target := range targets {
			counts = append(counts, mcpGrepCount{
				Path:  target.path,
				Count: grepFileMatchCount(target.content, opts, matcher),
			})
		}
		result["mode"] = "counts"
		result["counts"] = counts
	default:
		matches := make([]mcpGrepMatch, 0)
		for _, target := range targets {
			matches = append(matches, grepCollectMatches(target.path, target.content, opts, matcher)...)
		}
		result["mode"] = "matches"
		result["matches"] = matches
	}
	return result, nil
}

func collectManifestGrepTargets(ctx context.Context, store *afsStore, workspace string, manifestValue manifest, searchPath string) ([]grepFileTarget, error) {
	entry, ok := manifestValue.Entries[searchPath]
	if !ok {
		return nil, os.ErrNotExist
	}

	targets := make([]grepFileTarget, 0)
	for manifestPath, child := range manifestValue.Entries {
		switch {
		case manifestPath == searchPath && child.Type == "file":
		case entry.Type == "dir" && strings.HasPrefix(manifestPath, manifestPathPrefix(searchPath)) && child.Type == "file":
		default:
			continue
		}

		data, err := manifestEntryData(child, func(blobID string) ([]byte, error) {
			return store.getBlob(ctx, workspace, blobID)
		})
		if err != nil {
			return nil, err
		}
		targets = append(targets, grepFileTarget{
			path:    manifestPath,
			content: data,
			loaded:  true,
		})
	}
	return targets, nil
}

func manifestPathPrefix(path string) string {
	if path == "/" {
		return "/"
	}
	return path + "/"
}

func (s *afsMCPServer) resolveWorkspaceArg(ctx context.Context, args map[string]any, field string) (string, error) {
	if s.effectiveWorkspaceLock() != "" {
		requested, err := mcpOptionalString(args, field)
		if err != nil {
			return "", err
		}
		if requested != "" && strings.TrimSpace(requested) != s.effectiveWorkspaceLock() {
			return "", fmt.Errorf("workspace is locked to %q for mcp profile %q", s.effectiveWorkspaceLock(), s.effectiveProfile())
		}
		return s.effectiveWorkspaceLock(), nil
	}
	requested, err := mcpOptionalString(args, field)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(requested) == "" {
		return "", errors.New("workspace is required")
	}
	workspace := requested
	if err := validateAFSName("workspace", workspace); err != nil {
		return "", err
	}
	return workspace, nil
}

func (s *afsMCPServer) resolveWorkspaceFSPath(ctx context.Context, args map[string]any, requireFile bool) (string, string, client.Client, *client.StatResult, error) {
	workspace, err := s.resolveWorkspaceArg(ctx, args, "workspace")
	if err != nil {
		return "", "", nil, nil, err
	}
	rawPath, err := mcpRequiredString(args, "path")
	if err != nil {
		return "", "", nil, nil, err
	}
	normalizedPath := normalizeAFSGrepPath(rawPath)

	fsKey, _, _, err := s.store.ensureWorkspaceRoot(ctx, workspace)
	if err != nil {
		return "", "", nil, nil, err
	}
	fsClient := client.New(s.store.rdb, fsKey)
	stat, err := fsClient.Stat(ctx, normalizedPath)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", "", nil, nil, os.ErrNotExist
		}
		return "", "", nil, nil, err
	}
	if stat == nil {
		return "", "", nil, nil, os.ErrNotExist
	}
	if requireFile && stat.Type == "dir" {
		return "", "", nil, nil, fmt.Errorf("path %q is a directory", normalizedPath)
	}
	return workspace, normalizedPath, fsClient, stat, nil
}

func (s *afsMCPServer) mutateWorkspaceFile(ctx context.Context, args map[string]any, mutate func(context.Context, client.Client, string, *client.StatResult) (map[string]any, error)) (any, error) {
	workspace, err := s.resolveWorkspaceArg(ctx, args, "workspace")
	if err != nil {
		return nil, err
	}
	rawPath, err := mcpRequiredString(args, "path")
	if err != nil {
		return nil, err
	}
	normalizedPath := normalizeAFSGrepPath(rawPath)

	fsKey, _, _, err := s.store.ensureWorkspaceRoot(ctx, workspace)
	if err != nil {
		return nil, err
	}
	fsClient := client.New(s.store.rdb, fsKey)
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

	dirty, err := s.refreshWorkspaceLiveState(ctx, workspace)
	if err != nil {
		return nil, err
	}
	updatedStat, statErr := fsClient.Stat(ctx, normalizedPath)
	if statErr != nil && !errors.Is(statErr, redis.Nil) {
		return nil, statErr
	}
	payload["workspace"] = workspace
	payload["path"] = normalizedPath
	payload["dirty"] = dirty
	if updatedStat != nil {
		payload["kind"] = updatedStat.Type
		payload["size"] = updatedStat.Size
		payload["modified_at"] = mcpFileModifiedAt(updatedStat.Mtime)
	}
	return payload, nil
}

func (s *afsMCPServer) refreshWorkspaceLiveState(ctx context.Context, workspace string) (bool, error) {
	meta, err := s.store.getWorkspaceMeta(ctx, workspace)
	if err != nil {
		return false, err
	}
	liveManifest, _, err := liveWorkspaceManifest(ctx, s.store, workspace, meta.HeadSavepoint)
	if err != nil {
		return false, err
	}
	dirty, err := workspaceManifestIsDirty(ctx, s.store, workspace, meta.HeadSavepoint, liveManifest)
	if err != nil {
		return false, err
	}
	if dirty {
		if err := s.store.markWorkspaceRootDirty(ctx, workspace); err != nil {
			return false, err
		}
	} else {
		if err := s.store.markWorkspaceRootClean(ctx, workspace, meta.HeadSavepoint); err != nil {
			return false, err
		}
	}
	meta.DirtyHint = dirty
	return dirty, s.store.putWorkspaceMeta(ctx, meta)
}

func ensureWorkspaceParentDirs(ctx context.Context, fsClient client.Client, normalizedPath string) error {
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

func readWorkspaceFSEntry(ctx context.Context, workspace, normalizedPath string, fsClient client.Client, stat *client.StatResult) (any, error) {
	switch stat.Type {
	case "dir":
		return map[string]any{
			"workspace": workspace,
			"path":      normalizedPath,
			"kind":      "dir",
			"size":      stat.Size,
		}, nil
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
			"size":      stat.Size,
		}, nil
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

func workspaceFileListItem(ctx context.Context, fsClient client.Client, filePath string) (mcpFileListItem, error) {
	stat, err := fsClient.Stat(ctx, filePath)
	if err != nil {
		return mcpFileListItem{}, err
	}
	if stat == nil {
		return mcpFileListItem{}, os.ErrNotExist
	}
	item := mcpFileListItem{
		Path:       filePath,
		Name:       filepath.Base(filePath),
		Kind:       stat.Type,
		Size:       stat.Size,
		ModifiedAt: mcpFileModifiedAt(stat.Mtime),
	}
	if stat.Type == "symlink" {
		target, err := fsClient.Readlink(ctx, filePath)
		if err != nil {
			return mcpFileListItem{}, err
		}
		item.Target = target
	}
	return item, nil
}

func listWorkspaceFSEntries(ctx context.Context, fsClient client.Client, manifestPath string, depth int) ([]mcpFileListItem, error) {
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
		item, err := workspaceFileListItem(ctx, fsClient, node.Path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
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

func readWorkspaceTextContent(ctx context.Context, fsClient client.Client, normalizedPath string, stat *client.StatResult) (string, error) {
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

func readLocalWorkspaceEntry(workspace, normalizedPath, localPath string, info os.FileInfo) (any, error) {
	response := map[string]any{
		"workspace": workspace,
		"path":      normalizedPath,
		"kind":      "file",
		"size":      info.Size(),
	}
	if !info.ModTime().IsZero() {
		response["modified_at"] = info.ModTime().UTC().Format(time.RFC3339)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(localPath)
		if err != nil {
			return nil, err
		}
		response["kind"] = "symlink"
		response["target"] = target
		response["content"] = target
		return response, nil
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path %q is a directory", normalizedPath)
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return nil, err
	}
	if grepBinaryPrefix(data) {
		response["binary"] = true
		return response, nil
	}
	response["content"] = string(data)
	return response, nil
}

func readTextWorkspaceFile(localPath, normalizedPath string) (string, os.FileInfo, error) {
	info, err := os.Lstat(localPath)
	if err != nil {
		return "", nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("path %q is a symlink; edit the target explicitly", normalizedPath)
	}
	if info.IsDir() {
		return "", nil, fmt.Errorf("path %q is a directory", normalizedPath)
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", nil, err
	}
	if grepBinaryPrefix(data) {
		return "", nil, fmt.Errorf("path %q is binary", normalizedPath)
	}
	return string(data), info, nil
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

func listLocalWorkspaceEntries(treePath, localPath, manifestPath string, depth int) ([]mcpFileListItem, error) {
	entries := make([]mcpFileListItem, 0)
	err := listLocalWorkspaceEntriesRecursive(treePath, localPath, manifestPath, depth, &entries)
	if err != nil {
		return nil, err
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

func collectLocalGrepTargets(treePath, searchPath string) ([]grepFileTarget, error) {
	localPath := afsMaterializedPath(treePath, searchPath)
	info, err := os.Lstat(localPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		data, err := os.ReadFile(localPath)
		if err != nil {
			return nil, err
		}
		return []grepFileTarget{{
			path:    searchPath,
			content: data,
			loaded:  true,
		}}, nil
	}

	targets := make([]grepFileTarget, 0)
	err = filepath.WalkDir(localPath, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			targetInfo, err := os.Stat(current)
			if err != nil {
				return nil
			}
			if targetInfo.IsDir() {
				return nil
			}
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(treePath, current)
		if err != nil {
			return err
		}
		manifestPath := "/"
		if rel != "." {
			manifestPath = "/" + filepath.ToSlash(rel)
		}
		targets = append(targets, grepFileTarget{
			path:    manifestPath,
			content: data,
			loaded:  true,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return targets, nil
}

func listLocalWorkspaceEntriesRecursive(treePath, localPath, manifestPath string, depth int, out *[]mcpFileListItem) error {
	dirEntries, err := os.ReadDir(localPath)
	if err != nil {
		return err
	}
	for _, entry := range dirEntries {
		fullPath := filepath.Join(localPath, entry.Name())
		info, err := os.Lstat(fullPath)
		if err != nil {
			return err
		}
		item, err := buildLocalWorkspaceEntry(treePath, fullPath, info)
		if err != nil {
			return err
		}
		if manifestParent(item.Path) != manifestPath {
			continue
		}
		*out = append(*out, item)
		if depth > 1 && item.Kind == "dir" {
			if err := listLocalWorkspaceEntriesRecursive(treePath, fullPath, item.Path, depth-1, out); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildLocalWorkspaceEntry(treePath, fullPath string, info os.FileInfo) (mcpFileListItem, error) {
	rel, err := filepath.Rel(treePath, fullPath)
	if err != nil {
		return mcpFileListItem{}, err
	}
	manifestPath := "/"
	if rel != "." {
		manifestPath = "/" + filepath.ToSlash(rel)
	}
	item := mcpFileListItem{
		Path: manifestPath,
		Name: filepath.Base(fullPath),
		Kind: "file",
		Size: info.Size(),
	}
	if !info.ModTime().IsZero() {
		item.ModifiedAt = info.ModTime().UTC().Format(time.RFC3339)
	}
	if info.IsDir() {
		item.Kind = "dir"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		item.Kind = "symlink"
		target, err := os.Readlink(fullPath)
		if err != nil {
			return mcpFileListItem{}, err
		}
		item.Target = target
	}
	return item, nil
}

func manifestParent(p string) string {
	if p == "/" {
		return "/"
	}
	idx := strings.LastIndex(p, "/")
	if idx <= 0 {
		return "/"
	}
	return p[:idx]
}

func grepFileHasMatch(content []byte, opts grepOptions, matcher *grepMatcher) bool {
	if grepBinaryPrefix(content) {
		selected := matcher.matchBytes(content)
		if opts.invertMatch {
			return false
		}
		return selected
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		selected := matcher.matchLine(line)
		if opts.invertMatch {
			selected = !selected
		}
		if selected {
			return true
		}
	}
	return false
}

func grepFileMatchCount(content []byte, opts grepOptions, matcher *grepMatcher) int {
	if grepBinaryPrefix(content) {
		if grepFileHasMatch(content, opts, matcher) {
			return 1
		}
		return 0
	}
	count := 0
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		selected := matcher.matchLine(line)
		if opts.invertMatch {
			selected = !selected
		}
		if selected {
			count++
			if opts.maxCount > 0 && count >= opts.maxCount {
				break
			}
		}
	}
	return count
}

func grepCollectMatches(filePath string, content []byte, opts grepOptions, matcher *grepMatcher) []mcpGrepMatch {
	if grepBinaryPrefix(content) {
		if !grepFileHasMatch(content, opts, matcher) {
			return nil
		}
		return []mcpGrepMatch{{
			Path:   filePath,
			Text:   "Binary file matches",
			Binary: true,
		}}
	}

	lines := strings.Split(string(content), "\n")
	matches := make([]mcpGrepMatch, 0)
	for i, line := range lines {
		selected := matcher.matchLine(line)
		if opts.invertMatch {
			selected = !selected
		}
		if !selected {
			continue
		}
		matches = append(matches, mcpGrepMatch{
			Path: filePath,
			Line: int64(i + 1),
			Text: line,
		})
		if opts.maxCount > 0 && len(matches) >= opts.maxCount {
			break
		}
	}
	return matches
}

func ternaryString(condition bool, yes, no string) string {
	if condition {
		return yes
	}
	return no
}
