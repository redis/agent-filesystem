package controlplane

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/agent-filesystem/internal/mcpproto"
)

// controlPlaneTools is the tool catalog surfaced to control-plane-scoped
// tokens. No file tools — file operations require a workspace-scoped token,
// which the agent can mint on demand via `mcp_token_issue`.
func (p *hostedMCPProvider) controlPlaneTools() []mcpproto.Tool {
	stringType := map[string]any{"type": "string"}
	return []mcpproto.Tool{
		{
			Name:        "workspace_list",
			Description: "List all workspaces visible to the caller. Returns an array of {name, database_id, description, template_slug}.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "workspace_get",
			Description: "Fetch a single workspace by name. Returns details including database, template, and owner.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": withDesc(stringType, "Workspace name"),
				},
				"required": []string{"workspace"},
			},
		},
		{
			Name:        "workspace_create",
			Description: "Create a new workspace in the caller's default database.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":          withDesc(stringType, "Workspace name (lowercase, hyphens)"),
					"description":   withDesc(stringType, "Optional human description"),
					"template_slug": withDesc(stringType, "Optional template to seed the workspace with"),
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "workspace_fork",
			Description: "Fork an existing workspace into a new one. Destructive-adjacent — the new workspace starts as a snapshot of the source.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source": withDesc(stringType, "Source workspace name to fork"),
					"name":   withDesc(stringType, "New workspace name"),
				},
				"required": []string{"source", "name"},
			},
		},
		{
			Name:        "workspace_delete",
			Description: "Delete a workspace and all its data. This cannot be undone.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": withDesc(stringType, "Workspace name to delete"),
				},
				"required": []string{"workspace"},
			},
		},
		{
			Name:        "checkpoint_list",
			Description: "List checkpoints for a specific workspace.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": withDesc(stringType, "Workspace name"),
				},
				"required": []string{"workspace"},
			},
		},
		{
			Name:        "checkpoint_create",
			Description: "Create a checkpoint of a workspace's current live state.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace":  withDesc(stringType, "Workspace name"),
					"checkpoint": withDesc(stringType, "Optional checkpoint name; auto-generated if omitted"),
				},
				"required": []string{"workspace"},
			},
		},
		{
			Name:        "checkpoint_restore",
			Description: "Restore a workspace to a named checkpoint. Destructive — overwrites the live workspace.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace":  withDesc(stringType, "Workspace name"),
					"checkpoint": withDesc(stringType, "Checkpoint name to restore"),
				},
				"required": []string{"workspace", "checkpoint"},
			},
		},
		{
			Name:        "mcp_token_issue",
			Description: "Issue a new workspace-scoped MCP access token. Returns {url, token, server_name, workspace, scope, profile} ready to paste into an MCP client config.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": withDesc(stringType, "Workspace the token will be bound to"),
					"name":      withDesc(stringType, "Token label (e.g. \"shared-memory agent\")"),
					"profile": map[string]any{
						"type":        "string",
						"description": "Permission profile: workspace-ro, workspace-rw (default), or workspace-rw-checkpoint",
						"enum":        []string{"workspace-ro", "workspace-rw", "workspace-rw-checkpoint"},
					},
					"expires_at": withDesc(stringType, "Optional RFC3339 expiry timestamp"),
				},
				"required": []string{"workspace"},
			},
		},
		{
			Name:        "mcp_token_revoke",
			Description: "Revoke one of the caller's MCP access tokens by id. Works for both control-plane and workspace tokens the caller owns.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"token_id": withDesc(stringType, "Token id (the first segment after the prefix, not the full token)"),
				},
				"required": []string{"token_id"},
			},
		},
	}
}

func (p *hostedMCPProvider) callControlPlaneTool(ctx context.Context, name string, args map[string]any) mcpproto.ToolResult {
	value, err := p.dispatchControlPlaneTool(ctx, name, args)
	if err != nil {
		return mcpErrorResult(err)
	}
	return mcpproto.StructuredResult(value)
}

func (p *hostedMCPProvider) dispatchControlPlaneTool(ctx context.Context, name string, args map[string]any) (any, error) {
	switch name {
	case "workspace_list":
		response, err := p.manager.ListAllWorkspaceSummaries(ctx)
		if err != nil {
			return nil, err
		}
		return response, nil

	case "workspace_get":
		workspace, err := mcpRequiredString(args, "workspace")
		if err != nil {
			return nil, err
		}
		detail, err := p.manager.GetResolvedWorkspace(ctx, workspace)
		if err != nil {
			return nil, err
		}
		return detail, nil

	case "workspace_create":
		workspaceName, err := mcpRequiredString(args, "name")
		if err != nil {
			return nil, err
		}
		description, err := mcpOptionalString(args, "description")
		if err != nil {
			return nil, err
		}
		templateSlug, err := mcpOptionalString(args, "template_slug")
		if err != nil {
			return nil, err
		}
		input := createWorkspaceRequest{
			Name:         workspaceName,
			Description:  description,
			TemplateSlug: templateSlug,
		}
		detail, err := p.manager.CreateResolvedWorkspace(ctx, input)
		if err != nil {
			return nil, err
		}
		return detail, nil

	case "workspace_fork":
		source, err := mcpRequiredString(args, "source")
		if err != nil {
			return nil, err
		}
		target, err := mcpRequiredString(args, "name")
		if err != nil {
			return nil, err
		}
		if err := p.manager.ForkResolvedWorkspace(ctx, source, target); err != nil {
			return nil, err
		}
		return map[string]any{
			"source":    source,
			"workspace": target,
			"created":   true,
		}, nil

	case "workspace_delete":
		workspace, err := mcpRequiredString(args, "workspace")
		if err != nil {
			return nil, err
		}
		if err := p.manager.DeleteResolvedWorkspace(ctx, workspace); err != nil {
			return nil, err
		}
		return map[string]any{
			"workspace": workspace,
			"deleted":   true,
		}, nil

	case "checkpoint_list":
		workspace, err := mcpRequiredString(args, "workspace")
		if err != nil {
			return nil, err
		}
		items, err := p.manager.ListResolvedCheckpoints(ctx, workspace, 100)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"workspace":   workspace,
			"checkpoints": items,
		}, nil

	case "checkpoint_create":
		workspace, err := mcpRequiredString(args, "workspace")
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
		if err := validateHostedMCPName("checkpoint", checkpointID); err != nil {
			return nil, err
		}
		_, _, route, err := p.manager.resolveWorkspace(ctx, workspace)
		if err != nil {
			return nil, err
		}
		saved, err := p.manager.SaveCheckpointFromLiveWithOptions(ctx, route.DatabaseID, route.Name, checkpointID, SaveCheckpointFromLiveOptions{
			Kind:   CheckpointKindManual,
			Source: CheckpointSourceMCP,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"workspace":  workspace,
			"checkpoint": checkpointID,
			"created":    saved,
		}, nil

	case "checkpoint_restore":
		workspace, err := mcpRequiredString(args, "workspace")
		if err != nil {
			return nil, err
		}
		checkpointID, err := mcpRequiredString(args, "checkpoint")
		if err != nil {
			return nil, err
		}
		_, _, route, err := p.manager.resolveWorkspace(ctx, workspace)
		if err != nil {
			return nil, err
		}
		result, err := p.manager.RestoreCheckpointWithResult(ctx, route.DatabaseID, route.Name, checkpointID)
		if err != nil {
			return nil, err
		}
		payload := map[string]any{
			"workspace":  workspace,
			"checkpoint": checkpointID,
			"restored":   true,
		}
		if result.SafetyCheckpointCreated {
			payload["safety_checkpoint"] = result.SafetyCheckpointID
			payload["safety_checkpoint_created"] = true
		}
		return payload, nil

	case "mcp_token_issue":
		workspace, err := mcpRequiredString(args, "workspace")
		if err != nil {
			return nil, err
		}
		name, err := mcpOptionalString(args, "name")
		if err != nil {
			return nil, err
		}
		profile, err := mcpOptionalString(args, "profile")
		if err != nil {
			return nil, err
		}
		expiresAt, err := mcpOptionalString(args, "expires_at")
		if err != nil {
			return nil, err
		}
		if profile == "" {
			profile = MCPProfileWorkspaceRW
		}
		input := createMCPAccessTokenRequest{
			Name:      name,
			Profile:   profile,
			ExpiresAt: expiresAt,
		}
		response, err := p.manager.CreateResolvedMCPAccessToken(ctx, workspace, input)
		if err != nil {
			return nil, err
		}
		serverName := "afs-" + strings.TrimSpace(response.WorkspaceName)
		return map[string]any{
			"url":         strings.TrimSpace(p.baseURL),
			"token":       response.Token,
			"server_name": serverName,
			"workspace":   response.WorkspaceName,
			"scope":       response.Scope,
			"profile":     response.Profile,
			"expires_at":  response.ExpiresAt,
		}, nil

	case "mcp_token_revoke":
		tokenID, err := mcpRequiredString(args, "token_id")
		if err != nil {
			return nil, err
		}
		// Try control-plane revocation path first; if the token is
		// workspace-scoped, the routine returns os.ErrNotExist and we fall
		// back to the workspace-scoped path (which resolves the token record
		// and verifies the caller owns it).
		if err := p.manager.RevokeControlPlaneMCPAccessToken(ctx, tokenID); err == nil {
			return map[string]any{"token_id": tokenID, "revoked": true}, nil
		}
		if err := p.manager.revokeTokenByID(ctx, "", "", tokenID); err != nil {
			return nil, err
		}
		return map[string]any{"token_id": tokenID, "revoked": true}, nil

	default:
		return nil, fmt.Errorf("unknown tool %q for control-plane scope", name)
	}
}

// withDesc merges a description string into a JSON-schema fragment, preserving
// existing fields. Avoids the boilerplate of building a fresh map every time.
func withDesc(base map[string]any, description string) map[string]any {
	out := make(map[string]any, len(base)+1)
	for k, v := range base {
		out[k] = v
	}
	out["description"] = description
	return out
}
