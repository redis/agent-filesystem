# AFS Control Plane API

Date: 2026-04-05
Last reviewed: 2026-04-30

## Goal

Define one shared control-plane contract for:

- the AFS Web UI
- the `afs` CLI in cloud-connected mode
- future hosted AFS services

The current shared model is workspace-first:

- one workspace owns one active filesystem state
- one workspace owns one checkpoint timeline
- files are read and written directly against the workspace view
- aggregate workspace listing is served through the control-plane catalog
- database-scoped routes still exist for admin/debug flows and disambiguation

## Resource Model

The shared nouns are:

- `database`
- `workspace`
- `checkpoint`
- `file`
- `agent session`
- `activity event`
- `MCP access token`
- `job`

### Workspace

A top-level Agent Filesystem record.

Each workspace has:

- one backing database
- one AFS key namespace
- one active filesystem state
- zero or more immutable checkpoints
- recent audit activity

### Checkpoint

An immutable saved state within a workspace.

### File

A path inside the active workspace view, readable and editable by the browser/editor and CLI.

## Workspace Summary Contract

The main table response should fully render a workspace row without per-row fanout.

Required summary fields:

- `id`
- `name`
- `database_id`
- `database_name`
- `redis_key`
- `status`
- `file_count`
- `folder_count`
- `total_bytes`
- `checkpoint_count`
- `draft_state`
- `last_checkpoint_at`
- `updated_at`

Optional helpful fields:

- `region`
- `source`
- `owner`
- `created_at`

## HTTP API

Unless noted otherwise, paths below are rooted at `/v1`.

### Runtime And Auth

- `GET /auth/config`
- `POST /auth/exchange`
- `GET /version`
- `GET /account`
- `DELETE /account`
- `POST /account/developer/reset`
- `GET /cli`
- `GET /install.sh` (not under `/v1`; root-level installer script)
- `POST /quickstart`

### Databases

- `GET /databases`
- `POST /databases`
- `PUT /databases/{database_id}`
- `DELETE /databases/{database_id}`
- `POST /databases/{database_id}/default`
- `GET /databases/{database_id}/activity`
- `GET /databases/{database_id}/events`
- `GET /databases/{database_id}/agents`

### Workspaces

- `GET /workspaces`
- `GET /workspaces/{workspace_id}`
- `POST /workspaces`
- `PUT /workspaces/{workspace_id}`
- `DELETE /workspaces/{workspace_id}`
- `POST /workspaces:import`
- `POST /workspaces:import-local`

Database-scoped equivalents remain available:

- `GET /databases/{database_id}/workspaces`
- `POST /databases/{database_id}/workspaces`
- `POST /databases/{database_id}/workspaces:import`
- `POST /databases/{database_id}/workspaces:import-local`
- `GET|PUT|DELETE /databases/{database_id}/workspaces/{workspace_id}`

`GET /workspaces/{workspace_id}` should return:

- workspace metadata
- checkpoint summaries
- file inventory or tree summary
- recent activity
- connected-agent/session summaries
- capability flags

### Checkpoints

- `GET /workspaces/{workspace_id}/checkpoints`
- `GET /workspaces/{workspace_id}/checkpoints/{checkpoint_id}`
- `POST /workspaces/{workspace_id}/checkpoints`
- `GET /workspaces/{workspace_id}/diff`
- `POST /workspaces/{workspace_id}:restore`
- `POST /workspaces/{workspace_id}:save-from-live`
- `POST /workspaces/{workspace_id}:fork`

`GET /workspaces/{workspace_id}/checkpoints/{checkpoint_id}` returns richer
checkpoint detail:

- workspace id/name
- checkpoint id/name
- description/note
- kind/source/author/actor/session metadata
- parent checkpoint summary
- manifest hash, file count, folder count, byte total
- change summary from parent to this checkpoint

`GET /workspaces/{workspace_id}/diff` accepts `base` and `head` view refs. View
refs may be `checkpoint:{checkpoint_id}`, `head`, or `working-copy`. The
response includes file-level entries and, for UTF-8 text files under the server
limit, bounded `text_diff` hunks. Binary and oversized files report a skipped
reason instead of blocking the request.

`POST /workspaces/{workspace_id}:restore` accepts:

- `checkpoint_id`

It returns a restore result:

- `restored`
- `checkpoint_id`
- `workspace_id`
- `workspace_name`
- `safety_checkpoint_created`
- `safety_checkpoint_id` when AFS preserved unsaved active state before restore

### Files and Browser

- `GET /workspaces/{workspace_id}/tree`
- `GET /workspaces/{workspace_id}/files/content`
- `GET /workspaces/{workspace_id}/path-last`

The browser-facing read paths accept view/path/depth query parameters where
applicable. File mutation still happens through checkpoint/save/import flows
rather than a standalone `PUT /files/content` route.

### Activity, Changes, And Agents

- `GET /activity`
- `GET /events`
- `GET /agents`
- `GET /monitor/stream` opens a Server-Sent Events stream. The stream emits
  `monitor` events when workspace, activity, file-change, MCP-token, or
  agent-session state changes, so browser views can refresh live data without
  polling.
- `GET /workspaces/{workspace_id}/activity`
- `GET /workspaces/{workspace_id}/events`
- `GET /workspaces/{workspace_id}/changes`
- `GET /workspaces/{workspace_id}/sessions`
- `GET /workspaces/{workspace_id}/sessions/{session_id}/summary`

Database-scoped equivalents are available under
`/databases/{database_id}/workspaces/{workspace_id}/...`.

### MCP Tokens

- `GET /mcp-tokens`
- `GET /mcp-tokens?scope=control-plane`
- `POST /mcp-tokens`
- `DELETE /mcp-tokens/{token_id}`
- `GET /workspaces/{workspace_id}/mcp-tokens`
- `POST /workspaces/{workspace_id}/mcp-tokens`
- `DELETE /workspaces/{workspace_id}/mcp-tokens/{token_id}`

### Client Session Surface

The client/daemon surface is mounted under `/v1/client`.

- `POST /v1/client/workspaces/{workspace_id}/sessions`
- `POST /v1/client/workspaces/{workspace_id}/sessions/{session_id}/heartbeat`
- `DELETE /v1/client/workspaces/{workspace_id}/sessions/{session_id}`

Database-scoped equivalents are available under
`/v1/client/databases/{database_id}/workspaces/{workspace_id}/sessions`.

### Catalog Health

- `GET /catalog/health`
- `POST /catalog/reconcile`

## Example `GET /workspaces`

```json
{
  "items": [
    {
      "id": "payments-portal",
      "name": "payments-portal",
      "database_id": "db-payments-portal",
      "database_name": "payments-portal-us-east-1",
      "redis_key": "afs:payments-portal",
      "status": "healthy",
      "file_count": 3,
      "folder_count": 2,
      "total_bytes": 894,
      "checkpoint_count": 2,
      "draft_state": "dirty",
      "last_checkpoint_at": "2026-04-03T10:36:00Z",
      "updated_at": "2026-04-03T10:48:00Z",
      "region": "us-east-1",
      "source": "git-import"
    }
  ]
}
```

## Local Mode vs Cloud Mode

The command language should stay the same across transports.

- Local mode talks to Redis and local AFS state directly.
- Cloud mode talks to the HTTP API and must not bypass it.

The shared contract should preserve workspace IDs, checkpoint IDs, and file semantics across both modes.
