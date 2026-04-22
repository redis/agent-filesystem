package main

import (
	"bufio"
	"context"
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
	cfg     config
	store   *afsStore
	service *controlplane.Service
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

func cmdMCP(args []string) error {
	bin := filepath.Base(os.Args[0])
	if len(args) > 1 && isHelpArg(args[1]) {
		fmt.Fprint(os.Stderr, mcpUsageText(bin))
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("%s", mcpUsageText(bin))
	}

	cfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		return err
	}
	defer closeStore()

	server := &afsMCPServer{
		cfg:     cfg,
		store:   store,
		service: controlPlaneServiceFromStore(cfg, store),
	}
	return server.protocolServer().Serve(context.Background(), os.Stdin, os.Stdout)
}

func mcpUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s mcp

Start the Agent Filesystem MCP server over stdio.

This command is meant to be launched by an MCP client, for example:

  {
    "mcpServers": {
      "afs": {
        "command": "/absolute/path/to/%s",
        "args": ["mcp"]
      }
    }
  }
`, bin, bin)
}

func (s *afsMCPServer) protocolServer() *mcpproto.Server {
	return &mcpproto.Server{
		ProtocolVersion: afsMCPProtocolVersion,
		Name:            "afs",
		Version:         "0.1.0",
		Instructions:    "Workspace-first Agent Filesystem MCP server. Use workspace/file/checkpoint tools instead of raw Redis FS commands.",
		Provider:        s,
	}
}

func (s *afsMCPServer) serve(ctx context.Context, r io.Reader, w io.Writer) error {
	return s.protocolServer().Serve(ctx, r, w)
}

func (s *afsMCPServer) Tools(_ context.Context) []mcpproto.Tool {
	return []mcpproto.Tool{
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
			Name:        "workspace_current",
			Description: "Show the current workspace selection from the local AFS config",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "workspace_use",
			Description: "Set the current workspace in the local AFS config",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]string{"type": "string", "description": "Workspace name"},
				},
				"required": []string{"workspace"},
			},
		},
		{
			Name:        "workspace_create",
			Description: "Create a new empty workspace",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace":   map[string]string{"type": "string", "description": "Workspace name"},
					"description": map[string]string{"type": "string", "description": "Optional description"},
					"set_current": map[string]string{"type": "boolean", "description": "Also set it as the current workspace"},
				},
				"required": []string{"workspace"},
			},
		},
		{
			Name:        "workspace_fork",
			Description: "Fork a workspace at its current head into a new workspace",
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
					"workspace": map[string]string{"type": "string", "description": "Workspace name (defaults to current workspace)"},
				},
			},
		},
		{
			Name:        "checkpoint_create",
			Description: "Create a new checkpoint from the current live workspace state",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace":  map[string]string{"type": "string", "description": "Workspace name (defaults to current workspace)"},
					"checkpoint": map[string]string{"type": "string", "description": "Optional checkpoint name"},
				},
			},
		},
		{
			Name:        "checkpoint_restore",
			Description: "Restore a workspace to a checkpoint in the live workspace root",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace":  map[string]string{"type": "string", "description": "Workspace name (defaults to current workspace)"},
					"checkpoint": map[string]string{"type": "string", "description": "Checkpoint name"},
				},
				"required": []string{"checkpoint"},
			},
		},
		{
			Name:        "file_read",
			Description: "Read a file or symlink from a workspace",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]string{"type": "string", "description": "Workspace name (defaults to current workspace)"},
					"path":      map[string]string{"type": "string", "description": "Absolute path inside the workspace"},
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
					"workspace": map[string]string{"type": "string", "description": "Workspace name (defaults to current workspace)"},
					"path":      map[string]string{"type": "string", "description": "Absolute path inside the workspace"},
					"start":     map[string]string{"type": "integer", "description": "Start line (1-indexed)"},
					"end":       map[string]string{"type": "integer", "description": "End line (inclusive, -1 for EOF)"},
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
					"workspace": map[string]string{"type": "string", "description": "Workspace name (defaults to current workspace)"},
					"path":      map[string]string{"type": "string", "description": "Absolute directory path", "default": "/"},
					"depth":     map[string]string{"type": "integer", "description": "Depth relative to the requested path", "default": "1"},
				},
			},
		},
		{
			Name:        "file_write",
			Description: "Write a file in a workspace, creating parent directories as needed; leaves the workspace dirty until checkpoint_create is called",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]string{"type": "string", "description": "Workspace name (defaults to current workspace)"},
					"path":      map[string]string{"type": "string", "description": "Absolute file path"},
					"content":   map[string]string{"type": "string", "description": "Full file contents"},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			Name:        "file_replace",
			Description: "Replace text in a file and leave the workspace dirty until checkpoint_create is called",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]string{"type": "string", "description": "Workspace name (defaults to current workspace)"},
					"path":      map[string]string{"type": "string", "description": "Absolute file path"},
					"old":       map[string]string{"type": "string", "description": "Text to find"},
					"new":       map[string]string{"type": "string", "description": "Replacement text"},
					"all":       map[string]string{"type": "boolean", "description": "Replace all occurrences"},
				},
				"required": []string{"path", "old", "new"},
			},
		},
		{
			Name:        "file_insert",
			Description: "Insert content after a specific line and leave the workspace dirty until checkpoint_create is called",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]string{"type": "string", "description": "Workspace name (defaults to current workspace)"},
					"path":      map[string]string{"type": "string", "description": "Absolute file path"},
					"line":      map[string]string{"type": "integer", "description": "Insert after this line; 0=beginning, -1=end"},
					"content":   map[string]string{"type": "string", "description": "Content to insert"},
				},
				"required": []string{"path", "line", "content"},
			},
		},
		{
			Name:        "file_delete_lines",
			Description: "Delete a line range from a file and leave the workspace dirty until checkpoint_create is called",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]string{"type": "string", "description": "Workspace name (defaults to current workspace)"},
					"path":      map[string]string{"type": "string", "description": "Absolute file path"},
					"start":     map[string]string{"type": "integer", "description": "Start line (1-indexed)"},
					"end":       map[string]string{"type": "integer", "description": "End line (inclusive)"},
				},
				"required": []string{"path", "start", "end"},
			},
		},
		{
			Name:        "file_grep",
			Description: "Search a workspace directly in Redis using the same engine as `afs grep`",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace":          map[string]string{"type": "string", "description": "Workspace name (defaults to current workspace)"},
					"path":               map[string]string{"type": "string", "description": "Limit the search to a path", "default": "/"},
					"pattern":            map[string]string{"type": "string", "description": "Pattern to search for"},
					"ignore_case":        map[string]string{"type": "boolean", "description": "Case-insensitive search"},
					"glob":               map[string]string{"type": "boolean", "description": "Use AFS glob matching semantics"},
					"fixed_strings":      map[string]string{"type": "boolean", "description": "Treat the pattern as a fixed string"},
					"regexp":             map[string]string{"type": "boolean", "description": "Use regex mode (RE2 syntax)"},
					"word_regexp":        map[string]string{"type": "boolean", "description": "Match whole words"},
					"line_regexp":        map[string]string{"type": "boolean", "description": "Match entire lines"},
					"invert_match":       map[string]string{"type": "boolean", "description": "Return non-matching lines"},
					"files_with_matches": map[string]string{"type": "boolean", "description": "Return only matching file paths"},
					"count":              map[string]string{"type": "boolean", "description": "Return match counts per file"},
					"max_count":          map[string]string{"type": "integer", "description": "Maximum selected lines per file"},
				},
				"required": []string{"pattern"},
			},
		},
	}
}

func (s *afsMCPServer) CallTool(ctx context.Context, name string, args map[string]any) mcpproto.ToolResult {
	var (
		value any
		err   error
	)

	switch name {
	case "afs_status":
		value, err = s.toolAFSStatus()
	case "workspace_list":
		value, err = s.toolWorkspaceList(ctx)
	case "workspace_current":
		value, err = s.toolWorkspaceCurrent(ctx)
	case "workspace_use":
		value, err = s.toolWorkspaceUse(ctx, args)
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
	case "file_write":
		value, err = s.toolFileWrite(ctx, args)
	case "file_replace":
		value, err = s.toolFileReplace(ctx, args)
	case "file_insert":
		value, err = s.toolFileInsert(ctx, args)
	case "file_delete_lines":
		value, err = s.toolFileDeleteLines(ctx, args)
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
		"mount_backend":     s.cfg.MountBackend,
		"local_path":        s.cfg.LocalPath,
	}, nil
}

func (s *afsMCPServer) toolWorkspaceList(ctx context.Context) (any, error) {
	return s.service.ListWorkspaceSummaries(ctx)
}

func (s *afsMCPServer) toolWorkspaceCurrent(ctx context.Context) (any, error) {
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
	setCurrent, err := mcpBool(args, "set_current", false)
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
	if setCurrent {
		s.cfg.CurrentWorkspace = workspace
		if err := prepareConfigForSave(&s.cfg); err != nil {
			return nil, err
		}
		if err := saveConfig(s.cfg); err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"workspace":   detail,
		"set_current": setCurrent,
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
	saved, err := saveAFSWorkspaceOrLiveRoot(ctx, s.cfg, s.store, workspace, checkpointID, false)
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
	_, err = resetAFSWorkspaceHead(ctx, s.service, workspace, checkpointID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace":  workspace,
		"checkpoint": checkpointID,
		"mode":       "live-workspace",
	}, nil
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
	return s.mutateWorkspaceFile(ctx, args, func(ctx context.Context, fsClient client.Client, normalizedPath string, stat *client.StatResult) (map[string]any, error) {
		if stat == nil {
			return nil, os.ErrNotExist
		}
		if stat.Type != "file" {
			return nil, fmt.Errorf("path %q is not a regular file", normalizedPath)
		}
		count, err := fsClient.Replace(ctx, normalizedPath, oldValue, newValue, replaceAll)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"operation":    "replace",
			"replacements": int(count),
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
	requested, err := mcpOptionalString(args, field)
	if err != nil {
		return "", err
	}
	workspace, err := resolveWorkspaceName(ctx, s.cfg, s.store, requested)
	if err != nil {
		return "", err
	}
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
	payload["workspace"] = workspace
	payload["path"] = normalizedPath
	payload["dirty"] = dirty
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
