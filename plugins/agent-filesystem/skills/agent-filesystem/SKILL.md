---
name: agent-filesystem
description: Manage Agent Filesystem through the control-plane MCP server. Use when the `agent-filesystem` MCP server is available, or when the user asks Codex to list, create, fork, checkpoint, delete, or connect AFS workspaces using a control-plane token.
---

# Agent Filesystem

This plugin connects Codex to AFS as the `agent-filesystem` MCP server. It uses
a control-plane token, so the server exposes workspace management and
token-management tools rather than file tools for one specific workspace.

## Before Acting

- Prefer the `agent-filesystem` MCP tools for workspace management instead of
  shelling out to `afs` or editing local config by hand.
- Treat workspace deletion and checkpoint restore as destructive. Confirm the
  target workspace and checkpoint with the user before calling those tools.
- Do not print full bearer tokens back to the user. If a token tool returns a
  token, tell the user where it should be stored and avoid repeating it in later
  messages.
- Use `Self-managed` in user-facing copy for a user-run control plane.

## Control-Plane Tools

Use these tools when the `agent-filesystem` MCP server is connected with a
control-plane token:

- `workspace_list`: list visible workspaces.
- `workspace_get`: inspect a specific workspace.
- `workspace_create`: create a workspace, optionally from a template.
- `workspace_fork`: fork a workspace into a new workspace.
- `workspace_delete`: delete a workspace and its data. Confirm first.
- `checkpoint_list`: list checkpoints for a workspace.
- `checkpoint_create`: create a checkpoint from live workspace state.
- `checkpoint_restore`: restore live workspace state from a checkpoint. Confirm first.
- `mcp_token_issue`: mint a workspace-scoped MCP token for a specific workspace.
- `mcp_token_revoke`: revoke a control-plane or workspace MCP token by id.

## Common Flows

### Orient on Available Workspaces

Call `workspace_list`, then summarize the workspace names, databases, template
slugs, and anything that looks like a likely current target.

### Create a Workspace

Ask for a name only if the user has not given one. Prefer lowercase,
hyphen-delimited workspace names. If a template is relevant, pass
`template_slug`; otherwise create an empty workspace.

### Prepare a Workspace-Specific Agent Connection

1. Call `mcp_token_issue` with the workspace name and a clear token label.
2. Use `workspace-rw` for normal file editing, `workspace-ro` for inspection,
   and `workspace-rw-checkpoint` when the agent should manage checkpoints.
3. Tell the user to store the returned token in the target client's secret
   mechanism or environment variable, depending on that client's MCP config.
4. Use the `url` returned by `mcp_token_issue`; it will match the cloud or
   local/Self-managed control plane the agent is connected to.

For Codex, prefer an environment-backed token:

```toml
[mcp_servers.afs-workspace-name]
url = "<url-returned-by-mcp_token_issue>"
bearer_token_env_var = "AFS_WORKSPACE_TOKEN"
```

### Create a Checkpoint

Call `checkpoint_create` before risky changes or when the user explicitly asks
for a restore point. Use a short, meaningful checkpoint name when one is given;
otherwise let the server generate one.

## Endpoint Configuration

The plugin's MCP endpoint is configured in the plugin root `.mcp.json`.

Use the cloud endpoint for AFS Cloud:

```json
"url": "https://agent-filesystem.vercel.app/mcp"
```

Use localhost for a local or Self-managed control plane:

```json
"url": "http://127.0.0.1:8091/mcp"
```

## Key Points

- A control-plane token manages workspaces and can mint workspace tokens.
- A control-plane token does not expose file tools directly.
- File reads and writes require a workspace-scoped token issued for the target
  workspace.
- Checkpoints are explicit. Editing files does not automatically create a
  checkpoint.
- Redis remains the source of truth for workspaces, manifests, blobs,
  checkpoints, and activity.
