# RAF Control Plane API

Date: 2026-04-03

## Goal

Define one shared control-plane contract for:

- the RAF Web UI,
- the `raf` CLI in cloud-connected mode,
- and the `raf` CLI in direct local mode.

The key requirement is:

- the CLI must continue to work fully without Redis Cloud,
- but when the user authenticates to cloud, the CLI and UI must operate on the same canonical workspace/session model.

## Control Plane vs Data Plane

RAF cloud mode should use a split architecture.

### Control plane

The cloud service is the control plane.

It stores and manages:

- database registrations,
- workspace inventory,
- workspace-to-database mappings,
- user-visible workspace metadata,
- access control,
- capability flags,
- sync and health state,
- cached summary statistics for fast table rendering.

### Data plane

The backing Redis database is the data plane.

It stores the canonical RAF workspace content:

- files,
- sessions,
- checkpoints,
- manifests and blobs,
- audit streams,
- other RAF Redis structures.

### Recommendation

Cloud mode should not work by storing only database connections and discovering all workspaces live on every request.

Instead:

- the cloud service should store real workspace records,
- each workspace record should point at one backing database and RAF namespace,
- the cloud service may verify and refresh against Redis,
- but the UI workspace list should come from the cloud inventory first.

This gives:

- stable workspace IDs,
- fast list views,
- cleaner ownership and access control,
- predictable create/delete flows,
- and consistent behavior between UI and CLI.

## Operating Modes

RAF should support two backends behind the same command surface.

### Direct mode

Direct mode talks to Redis and local RAF state directly.

Characteristics:

- no cloud account required,
- works against local Redis or any reachable Redis,
- current RAF Redis keyspace remains authoritative,
- local materialization under `~/.raf/workspaces` remains authoritative for dirty state.

### Cloud mode

Cloud mode talks to a cloud-hosted RAF API.

Characteristics:

- enabled after `raf cloud auth`,
- the API is authoritative for workspaces, sessions, checkpoints, and file operations,
- the Web UI and CLI share the same resource IDs and semantics,
- the CLI must not mutate Redis directly behind the API in this mode.

## Design Rule

The CLI command language does not fork by mode.

These commands remain the public surface:

- `raf workspaces`
- `raf inspect`
- `raf session list`
- `raf session fork`
- `raf session save`
- `raf session rollback`
- `raf session run`

Only the transport changes:

- direct mode uses a Redis-backed service implementation,
- cloud mode uses an HTTP API-backed service implementation.

## Shared Resource Model

The shared nouns are:

- `database`
- `workspace`
- `session`
- `checkpoint`
- `file`
- `job`

### Database

A Redis database connection target.

In direct mode this may map to the CLI's configured Redis endpoint.

In cloud mode this maps to a Redis Cloud database record.

### Workspace

One Agent Filesystem.

This is the top-level object shown in the main UI table.

Each workspace has:

- one backing database,
- one control-plane workspace record,
- one RAF root key or namespace,
- zero or more sessions,
- zero or more checkpoints through its sessions.

### Session

A branch-like line of work within a workspace.

### Checkpoint

An immutable saved state within a session.

This corresponds to today's savepoint concept.

### File

A path inside a workspace/session view, readable and editable by the browser/editor and CLI.

### Job

A long-running operation such as:

- import,
- export,
- materialize,
- rollback,
- sync.

## Workspace Table Contract

The UI main page should be a table of workspaces.

Each row must be fully renderable from one summary response without per-row fanout.

The source of that table in cloud mode is the control-plane workspace inventory, not raw Redis discovery.

Required workspace summary fields:

- `id`
- `name`
- `database_id`
- `database_name`
- `redis_key`
- `status`
- `file_count`
- `folder_count`
- `total_bytes`
- `session_count`
- `fork_count`
- `checkpoint_count`
- `default_session_id`
- `last_checkpoint_at`
- `updated_at`

Optional helpful fields:

- `region`
- `owner`
- `created_at`
- `last_actor`
- `dirty_session_count`

## HTTP API

These endpoints are required for cloud mode.

All paths below are rooted at `/v1`.

### Databases

- `GET /databases`

Returns databases available to the authenticated user for RAF operations.

### Workspaces

- `GET /workspaces`
- `GET /workspaces/{workspace_id}`
- `POST /workspaces`
- `DELETE /workspaces/{workspace_id}`

`GET /workspaces` returns workspace summary rows for the main table.

Those rows are backed by control-plane workspace records, with Redis-backed stats attached or refreshed by the service.

`GET /workspaces/{workspace_id}` returns:

- workspace metadata,
- session summaries,
- recent checkpoints,
- tree summary,
- recent activity,
- capability flags.

### Sessions

- `GET /workspaces/{workspace_id}/sessions`
- `GET /workspaces/{workspace_id}/sessions/{session_id}`
- `POST /workspaces/{workspace_id}/sessions`
- `DELETE /workspaces/{workspace_id}/sessions/{session_id}`

Session creation supports:

- new branch session,
- imported session,
- empty session if allowed later.

### Checkpoints

- `GET /workspaces/{workspace_id}/sessions/{session_id}/checkpoints`
- `POST /workspaces/{workspace_id}/sessions/{session_id}/checkpoints`
- `POST /workspaces/{workspace_id}/sessions/{session_id}:rollback`

### Files and Browser

- `GET /workspaces/{workspace_id}/tree`
- `GET /workspaces/{workspace_id}/files/content`
- `PUT /workspaces/{workspace_id}/files/content`
- `POST /workspaces/{workspace_id}/files/delete`
- `POST /workspaces/{workspace_id}/files/mkdir`

Query parameters for tree and content reads:

- `session_id`
- `path`

### Activity

- `GET /workspaces/{workspace_id}/activity`

### Jobs

- `POST /workspaces/{workspace_id}:import`
- `GET /jobs/{job_id}`

## HTTP Request and Response Shapes

### `GET /workspaces`

Response:

```json
{
  "items": [
    {
      "id": "ws_123",
      "name": "payments-portal",
      "database_id": "db_9",
      "database_name": "prod-us-east-1",
      "redis_key": "raf:payments-portal",
      "status": "healthy",
      "file_count": 1842,
      "folder_count": 217,
      "total_bytes": 18439221,
      "session_count": 6,
      "fork_count": 4,
      "checkpoint_count": 22,
      "default_session_id": "ses_main",
      "last_checkpoint_at": "2026-04-03T19:10:00Z",
      "updated_at": "2026-04-03T19:12:00Z"
    }
  ]
}
```

### `POST /workspaces`

Request:

```json
{
  "name": "payments-portal",
  "database_id": "db_9",
  "redis_key": "raf:payments-portal",
  "description": "Checkout debugging workspace",
  "source": {
    "kind": "import",
    "uri": "git://example/repo"
  }
}
```

### `POST /workspaces/{workspace_id}/sessions`

Request:

```json
{
  "name": "fix-login",
  "description": "Investigate login regressions",
  "mode": "fork",
  "source_session_id": "ses_main",
  "source_checkpoint_id": "cp_123"
}
```

### `PUT /workspaces/{workspace_id}/files/content`

Request:

```json
{
  "session_id": "ses_fix_login",
  "path": "/src/app.tsx",
  "content": "export function App() {}",
  "expected_revision": "rev_456"
}
```

The `expected_revision` field is required for optimistic concurrency between:

- UI edits,
- CLI edits,
- and future collaborative sessions.

## Shared Domain Types

These structs should exist in a shared RAF domain package, independent of transport.

### Go

```go
type WorkspaceSummary struct {
    ID               string
    Name             string
    DatabaseID       string
    DatabaseName     string
    RedisKey         string
    Status           string
    FileCount        int64
    FolderCount      int64
    TotalBytes       int64
    SessionCount     int64
    ForkCount        int64
    CheckpointCount  int64
    DefaultSessionID string
    LastCheckpointAt time.Time
    UpdatedAt        time.Time
}

type WorkspaceDetail struct {
    Summary          WorkspaceSummary
    Description      string
    Sessions         []SessionSummary
    RecentCheckpoints []Checkpoint
    RecentActivity   []ActivityEvent
    Capabilities     WorkspaceCapabilities
}

type SessionSummary struct {
    ID               string
    Name             string
    Description      string
    Mode             string
    Status           string
    HeadCheckpointID string
    UpdatedAt        time.Time
}

type Checkpoint struct {
    ID          string
    Name        string
    SessionID   string
    FileCount   int64
    TotalBytes  int64
    CreatedAt   time.Time
    CreatedBy   string
    Description string
}

type TreeEntry struct {
    Path      string
    Name      string
    Kind      string
    Size      int64
    Revision  string
    UpdatedAt time.Time
}

type FileContent struct {
    Path      string
    Content   string
    Revision  string
    Size      int64
    UpdatedAt time.Time
}
```

### TypeScript

The UI should mirror the same logical shapes:

```ts
export type WorkspaceSummary = {
  id: string;
  name: string;
  databaseId: string;
  databaseName: string;
  redisKey: string;
  status: string;
  fileCount: number;
  folderCount: number;
  totalBytes: number;
  sessionCount: number;
  forkCount: number;
  checkpointCount: number;
  defaultSessionId: string;
  lastCheckpointAt: string;
  updatedAt: string;
};
```

## CLI Service Boundary

The CLI should stop calling Redis directly from command handlers.

Instead, command handlers should depend on a service interface.

Suggested service:

```go
type WorkspaceService interface {
    ListWorkspaces(ctx context.Context) ([]WorkspaceSummary, error)
    GetWorkspace(ctx context.Context, workspaceID string) (WorkspaceDetail, error)
    CreateWorkspace(ctx context.Context, req CreateWorkspaceRequest) (WorkspaceDetail, error)
    DeleteWorkspace(ctx context.Context, workspaceID string) error

    ListSessions(ctx context.Context, workspaceID string) ([]SessionSummary, error)
    GetSession(ctx context.Context, workspaceID, sessionID string) (SessionDetail, error)
    CreateSession(ctx context.Context, workspaceID string, req CreateSessionRequest) (SessionDetail, error)
    DeleteSession(ctx context.Context, workspaceID, sessionID string) error

    ListCheckpoints(ctx context.Context, workspaceID, sessionID string) ([]Checkpoint, error)
    CreateCheckpoint(ctx context.Context, workspaceID, sessionID string, req CreateCheckpointRequest) (Checkpoint, error)
    RollbackSession(ctx context.Context, workspaceID, sessionID, checkpointID string) error

    ListTree(ctx context.Context, workspaceID, sessionID, path string) ([]TreeEntry, error)
    ReadFile(ctx context.Context, workspaceID, sessionID, path string) (FileContent, error)
    WriteFile(ctx context.Context, workspaceID, sessionID string, req WriteFileRequest) (FileContent, error)
}
```

Implementations:

- `DirectWorkspaceService`
- `CloudWorkspaceService`

## Mode Selection

The CLI should resolve backend mode in this order:

1. explicit `--backend=direct|cloud`
2. active saved context
3. fallback to direct

Recommended commands:

- `raf context show`
- `raf context use direct`
- `raf context use cloud`

## Config Model

Direct config should include:

- `redis_addr`
- `redis_db`
- `redis_username`
- `redis_password`
- `redis_tls`
- `work_root`

Cloud config should include:

- `api_base_url`
- `account_key` or auth token
- `selected_database_id`
- `selected_database_name`

Recommended top-level config shape:

```json
{
  "active_backend": "direct",
  "direct": {
    "redis_addr": "127.0.0.1:6379",
    "redis_db": 0,
    "redis_tls": false,
    "work_root": "~/.raf/workspaces"
  },
  "cloud": {
    "api_base_url": "https://agentfs.api.redis.example/v1",
    "selected_database_id": "db_9",
    "selected_database_name": "prod-us-east-1"
  }
}
```

## Command Mapping

The same CLI commands map to different transport implementations by backend.

### In direct mode

- `raf workspaces` reads Redis-backed RAF metadata
- `raf session save` writes savepoint metadata directly
- `raf session rollback` rematerializes locally from Redis-backed saved state

### In cloud mode

- `raf workspaces` calls `GET /workspaces`
- `raf inspect` calls `GET /workspaces/{id}`
- `raf session fork` calls `POST /sessions`
- `raf session save` calls `POST /checkpoints`
- `raf session rollback` calls `POST :rollback`
- file commands call file endpoints

## Important Authority Rule

The authoritative owner depends on mode.

### Direct mode authority

- local Redis and local RAF state

### Cloud mode authority

- RAF API

This rule prevents split-brain behavior between UI and CLI.

In cloud mode specifically:

- the control-plane API is authoritative for workspace inventory and routing,
- the backing Redis database is authoritative for workspace content,
- the CLI and UI must both go through the API for cloud-backed operations.

## Rollout Plan

### Phase 1

- define shared domain structs
- add `WorkspaceService` interface
- wrap current Redis logic in `DirectWorkspaceService`

### Phase 2

- implement `CloudWorkspaceService`
- add backend selection and context commands

### Phase 3

- move the Web UI to the HTTP client contract
- replace the demo workspace catalog with a table built from `WorkspaceSummary`

### Phase 4

- add browser/editor operations against real file endpoints
- keep CLI and UI behavior in lockstep through contract tests

## What This Enables

With this contract:

- the UI main page becomes a workspace table,
- clicking a workspace opens a browser/editor for that workspace,
- the CLI still works fully offline against local Redis,
- and cloud-connected CLI usage stays aligned with the Web UI.
