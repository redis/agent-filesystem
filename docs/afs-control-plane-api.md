# AFS Control Plane API

Date: 2026-04-05

## Goal

Define one shared control-plane contract for:

- the AFS Web UI
- the `afs` CLI in cloud-connected mode
- future hosted AFS services

The current shared model is workspace-first:

- one workspace owns one working copy state
- one workspace owns one checkpoint timeline
- files are read and written directly against the workspace view

## Resource Model

The shared nouns are:

- `database`
- `workspace`
- `checkpoint`
- `file`
- `job`

### Workspace

A top-level Agent Filesystem record.

Each workspace has:

- one backing database
- one AFS key namespace
- one current working-copy state
- zero or more immutable checkpoints
- recent audit activity

### Checkpoint

An immutable saved state within a workspace.

### File

A path inside the current workspace view, readable and editable by the browser/editor and CLI.

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

All paths below are rooted at `/v1`.

### Databases

- `GET /databases`

### Workspaces

- `GET /workspaces`
- `GET /workspaces/{workspace_id}`
- `POST /workspaces`
- `DELETE /workspaces/{workspace_id}`

`GET /workspaces/{workspace_id}` should return:

- workspace metadata
- checkpoint summaries
- file inventory or tree summary
- recent activity
- capability flags

### Checkpoints

- `GET /workspaces/{workspace_id}/checkpoints`
- `POST /workspaces/{workspace_id}/checkpoints`
- `POST /workspaces/{workspace_id}:restore`

`POST /workspaces/{workspace_id}:restore` accepts:

- `checkpoint_id`

### Files and Browser

- `GET /workspaces/{workspace_id}/tree`
- `GET /workspaces/{workspace_id}/files/content`
- `PUT /workspaces/{workspace_id}/files/content`

`PUT /workspaces/{workspace_id}/files/content` accepts:

- `path`
- `content`
- `expected_revision` (optional)

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

## Direct Mode vs Cloud Mode

The command language should stay the same across transports.

- Direct mode talks to Redis and local AFS state directly.
- Cloud mode talks to the HTTP API and must not bypass it.

The shared contract should preserve workspace IDs, checkpoint IDs, and file semantics across both modes.
