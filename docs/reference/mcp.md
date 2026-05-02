# AFS MCP Tool Reference

This reference covers the MCP surfaces exposed by Agent Filesystem.

AFS exposes two MCP shapes:

- Local stdio MCP through `afs mcp`.
- Hosted/control-plane HTTP MCP at `/mcp`.

Both use the same workspace model: file tools operate on live workspace state,
and checkpoints are explicit.

## Local Stdio Setup

```bash
afs mcp [--workspace <name>] [--profile <profile>]
```

Example client config:

```json
{
  "mcpServers": {
    "agent-filesystem": {
      "command": "/absolute/path/to/afs",
      "args": ["mcp", "--workspace", "my-workspace", "--profile", "workspace-rw-checkpoint"]
    }
  }
}
```

Use an absolute path for `command`.

## Hosted HTTP Setup

Use the hosted `/mcp` endpoint with a bearer token:

```json
{
  "mcpServers": {
    "agent-filesystem": {
      "url": "https://afs.cloud/mcp",
      "headers": {
        "Authorization": "Bearer ${AFS_TOKEN}"
      }
    }
  }
}
```

For a Self-managed control plane, replace the URL with your control-plane
origin plus `/mcp`.

## Profiles

| Profile | Tools allowed |
| --- | --- |
| `workspace-ro` | Read-only file tools. |
| `workspace-rw` | Read-only file tools plus write tools. Default. |
| `workspace-rw-checkpoint` | Read/write file tools plus checkpoint tools. |
| `admin-ro` | Admin tools, read-only file tools, and `checkpoint_list`. |
| `admin-rw` | Admin tools, read/write file tools, and checkpoint tools. |

Workspace-bound profiles require a workspace. Pass `--workspace` locally, or
use a workspace-scoped hosted token.

## Tool Families

| Family | Tools |
| --- | --- |
| Status/admin | `afs_status`, `workspace_list`, `workspace_create`, `workspace_fork` |
| Checkpoints | `checkpoint_list`, `checkpoint_create`, `checkpoint_restore` |
| File reads | `file_read`, `file_lines`, `file_list`, `file_glob`, `file_grep` |
| File writes | `file_write`, `file_create_exclusive`, `file_replace`, `file_insert`, `file_delete_lines`, `file_patch` |
| Hosted token administration | `mcp_token_issue`, `mcp_token_revoke` |

Hosted workspace-scoped MCP exposes the workspace file and checkpoint tools.
Hosted control-plane-scoped MCP exposes workspace lifecycle, checkpoint, and
token management tools.

## Status And Workspace Tools

### `afs_status`

Local stdio MCP only.

Shows local AFS configuration and mount status.

Arguments: none.

### `workspace_list`

Lists available workspaces.

Arguments: none for local MCP. Hosted control-plane MCP may scope results by
the authenticated identity.

### `workspace_create`

Creates an empty workspace.

Local MCP arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | Yes | Workspace name. |
| `description` | No | Optional description. |
| `set_current` | No | Also select the workspace locally. |

Hosted control-plane arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `name` | Yes | Workspace name. |
| `description` | No | Optional description. |
| `template_slug` | No | Optional template slug. |

### `workspace_get`

Hosted control-plane MCP only.

Gets one workspace.

Arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | Yes | Workspace name or ID. |

### `workspace_fork`

Forks a workspace into a second line of work.

Local MCP arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | Yes | Source workspace. |
| `new_workspace` | Yes | New workspace name. |

Hosted control-plane arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `source` | Yes | Source workspace. |
| `name` | Yes | New workspace name. |

### `workspace_delete`

Hosted control-plane MCP only.

Deletes a workspace.

Arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | Yes | Workspace name or ID. |

## Checkpoint Tools

### `checkpoint_list`

Lists checkpoints for a workspace.

Arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | No | Workspace name. Local MCP defaults to current; hosted workspace MCP uses the token workspace. |

### `checkpoint_create`

Creates a checkpoint from the current live workspace state.

Arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | No | Workspace name. Local MCP defaults to current; hosted workspace MCP uses the token workspace. |
| `checkpoint` | No | Optional checkpoint name. |

### `checkpoint_restore`

Restores live workspace state to a checkpoint.

Arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | No | Workspace name. Local MCP defaults to current; hosted workspace MCP uses the token workspace. |
| `checkpoint` | Yes | Checkpoint name. |

Restoring overwrites live workspace state. Create a fresh checkpoint first if
the current state may matter.

## File Read And Search Tools

All file paths are absolute inside the workspace, for example `/src/main.go`.

### `file_read`

Reads a file or symlink.

Arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | No | Local MCP workspace override. |
| `path` | Yes | Absolute file or symlink path. |

Use this for whole-file reads. Use `file_lines` for partial text reads,
`file_list` or `file_glob` for discovery, and `file_grep` for content search.

### `file_lines`

Reads a specific line range from a text file.

Arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | No | Local MCP workspace override. |
| `path` | Yes | Absolute text file path. |
| `start` | Yes | Start line, 1-indexed. |
| `end` | No | End line, inclusive. Use `-1` for EOF. |

### `file_list`

Lists files and directories under a path.

Arguments:

| Field | Required | Default | Meaning |
| --- | --- | --- | --- |
| `workspace` | No | Current/token workspace | Local MCP workspace override. |
| `path` | No | `/` | Absolute directory path. |
| `depth` | No | `1` | Depth relative to requested path. |

### `file_glob`

Finds files or directories by basename glob pattern.

Arguments:

| Field | Required | Default | Meaning |
| --- | --- | --- | --- |
| `workspace` | No | Current/token workspace | Local MCP workspace override. |
| `path` | No | `/` | Absolute directory path to search within. |
| `pattern` | Yes | - | Basename glob pattern such as `*.go`. |
| `kind` | No | `file` | `file`, `dir`, `symlink`, or `any`. |
| `limit` | No | `100` | Maximum results. |

Use this for filename discovery. Do not use it for content search.

### `file_grep`

Searches file contents.

Arguments:

| Field | Required | Default | Meaning |
| --- | --- | --- | --- |
| `workspace` | No | Current/token workspace | Local MCP workspace override. |
| `path` | No | `/` | Absolute file or directory to search within. |
| `pattern` | Yes | - | Search pattern. |
| `ignore_case` | No | `false` | Case-insensitive search. |
| `glob` | No | `false` | Use glob matching semantics. |
| `fixed_strings` | No | `false` | Treat pattern as fixed string. |
| `regexp` | No | `false` | Use RE2 regex mode. |
| `word_regexp` | No | `false` | Match whole words. |
| `line_regexp` | No | `false` | Match whole lines. |
| `invert_match` | No | `false` | Return non-matching lines. |
| `files_with_matches` | No | `false` | Return only matching file paths. |
| `count` | No | `false` | Return match counts per file. |
| `max_count` | No | unset | Maximum selected lines per file. |

Choose only one search mode among `glob`, `fixed_strings`, and `regexp`.

## File Write Tools

File edits update live workspace state immediately. They leave the workspace
dirty until `checkpoint_create` is called.

### `file_write`

Writes a full file, creating parent directories as needed.

Arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | No | Local MCP workspace override. |
| `path` | Yes | Absolute file path. |
| `content` | Yes | Complete file contents. |

Use for new files or full overwrites. Prefer smaller edit tools for localized
changes.

### `file_create_exclusive`

Atomically creates a file only if the path does not already exist.

Arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | No | Local MCP workspace override. |
| `path` | Yes | Absolute file path. |
| `content` | Yes | File contents to write on creation. |

Useful for distributed locks and claims between agents.

### `file_replace`

Replaces exact text in a file.

Arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | No | Local MCP workspace override. |
| `path` | Yes | Absolute file path. |
| `old` | Yes | Exact text to find. |
| `new` | Yes | Replacement text. |
| `all` | No | Replace all occurrences. |
| `expected_occurrences` | No | Fail unless this many occurrences match. |
| `start_line` | No | Exact 1-indexed line where the match must begin. |
| `context_before` | No | Exact text that must appear before the match. |
| `context_after` | No | Exact text that must appear after the match. |

### `file_insert`

Inserts content at a line boundary.

Arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | No | Local MCP workspace override. |
| `path` | Yes | Absolute file path. |
| `line` | Yes | Insert after this line. Use `0` for beginning or `-1` for EOF. |
| `content` | Yes | Content to insert. |

### `file_delete_lines`

Deletes a known line range.

Arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | No | Local MCP workspace override. |
| `path` | Yes | Absolute file path. |
| `start` | Yes | Start line, 1-indexed. |
| `end` | Yes | End line, inclusive. |

### `file_patch`

Applies one or more structured text patches.

Arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | No | Local MCP workspace override. |
| `path` | Yes | Absolute file path. |
| `expected_sha256` | No | Optional hash precondition. |
| `patches` | Yes | Ordered list of patch operations. |

Patch operation fields:

| Field | Required | Meaning |
| --- | --- | --- |
| `op` | Yes | `replace`, `insert`, or `delete`. |
| `start_line` | No | 1-indexed starting line. For insert, `0` means file beginning and `-1` means EOF. |
| `end_line` | No | Inclusive end line for delete operations. |
| `old` | No | Exact expected text for replace or delete. |
| `new` | No | Replacement or inserted text. |
| `context_before` | No | Exact text immediately before the patch. |
| `context_after` | No | Exact text immediately after the patch. |

Prefer `file_patch` for precise multi-step edits where exact context matters.

## Hosted Token Tools

Hosted control-plane MCP exposes token tools so agents can mint
workspace-scoped MCP access.

### `mcp_token_issue`

Issues a workspace-scoped MCP token.

Arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `workspace` | Yes | Workspace name or ID. |
| `name` | No | Token display name. |
| `profile` | No | `workspace-ro`, `workspace-rw`, or `workspace-rw-checkpoint`. |
| `expires_at` | No | Optional expiry timestamp. |

### `mcp_token_revoke`

Revokes an MCP token.

Arguments:

| Field | Required | Meaning |
| --- | --- | --- |
| `token_id` | Yes | Token ID to revoke. |

## Agent Usage Rules

- Use `file_glob` for filename discovery.
- Use `file_grep` for content search.
- Read before editing.
- Prefer precise edit tools over full-file rewrites.
- Create checkpoints before risky changes and after useful results.
- Do not restore a checkpoint unless the user explicitly wants live state
  overwritten.
