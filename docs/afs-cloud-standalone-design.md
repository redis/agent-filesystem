# AFS Cloud + Standalone Design

Date: 2026-04-06

## Goal

Adopt the useful parts of the `cloud-context-engine` design for AFS:

- a cloud-managed CLI
- a hosted control plane
- profile-based endpoint selection
- secure local credential storage
- human auth for interactive use and token auth for automation

But keep one stronger guarantee than that repo currently assumes:

- the AFS CLI must remain fully usable with no cloud account at all

Standalone mode is not a degraded fallback. It is a first-class operating mode.

## What We Can Reuse From `cloud-context-engine`

The other repo has a clean pattern worth copying:

1. The CLI separates profile configuration from secrets.
2. The CLI supports human login for interactive use and stored API keys for automation.
3. The server accepts more than one auth shape depending on route and deployment mode.
4. The deployment can switch auth behavior by mode instead of forcing one global assumption.

The concrete ideas worth borrowing are:

- multi-profile CLI config
- secure credential storage outside the plain config file
- unified auth resolution for commands
- an HTTP control plane that becomes the only path in managed mode
- server-side auth mode gates for cloud vs self-hosted behavior

## What AFS Already Has

AFS already has several pieces that line up well with that model:

- a Redis-backed canonical store for workspaces, checkpoints, manifests, blobs, and audit
- a local hybrid execution model where real work happens in a local directory
- a local HTTP control plane and UI contract
- workspace metadata that already includes cloud-shaped fields such as `database_id`, `database_name`, `cloud_account`, and `region`

That means the product model is already compatible with a hosted control plane.

## Main Gaps In AFS Today

AFS is still wired as a direct Redis CLI:

- commands call the store/service layer directly
- the current HTTP server is local-only and unauthenticated
- the HTTP API is good enough for browsing, but not yet complete enough for a remote CLI write path
- config is a single local JSON file with Redis and mount settings only
- there is no profile system, secure credential store, or login flow

The biggest architectural issue is coupling:

- the CLI currently assumes it can always open Redis directly

That assumption must become optional.

## Proposed Operating Modes

AFS should support three explicit modes.

### 1. `direct`

This is the default and preserves current AFS behavior.

- no cloud account
- no hosted control plane required
- CLI talks directly to Redis
- local Redis management remains available
- current mount behavior remains available

This mode should continue to work on a laptop with only Redis and the AFS binaries.

### 2. `self-hosted`

This is for a privately deployed AFS API without a public cloud account dependency.

- CLI talks to an HTTP control plane
- storage is still managed by that deployment
- auth can be bootstrap admin key, local token auth, or localhost-only for single-user installs
- no Redis direct access from the CLI in normal operation

This gives us the same operational shape as cloud mode without requiring AFS Cloud.

### 3. `cloud`

This is the managed mode modeled after `cloud-context-engine`.

- CLI talks only to the hosted AFS API
- human users authenticate with session-based login
- automation uses stored API keys or workspace-scoped tokens
- Redis placement, quotas, and workspace policy are controlled by the service
- the CLI must not depend on long-lived Redis credentials

## Core Design Rule

Keep one command language and swap only the backend transport.

That means commands like these stay the same:

- `afs workspace list`
- `afs workspace create`
- `afs workspace import`
- `afs workspace run`
- `afs checkpoint create`
- `afs checkpoint restore`

What changes is how those commands are fulfilled:

- `direct` uses Redis/store calls
- `self-hosted` and `cloud` use HTTP

## Proposed Backend Split

Introduce a transport boundary inside the CLI.

### New CLI abstraction

Add a backend interface that covers the workspace lifecycle instead of exposing Redis-specific store calls everywhere.

Suggested shape:

```go
type Backend interface {
    ListWorkspaces(ctx context.Context) (WorkspaceListResponse, error)
    GetWorkspace(ctx context.Context, workspace string) (WorkspaceDetail, error)
    CreateWorkspace(ctx context.Context, req CreateWorkspaceRequest) (WorkspaceDetail, error)
    DeleteWorkspace(ctx context.Context, workspace string) error

    ListCheckpoints(ctx context.Context, workspace string, limit int) ([]CheckpointSummary, error)
    CreateCheckpoint(ctx context.Context, req SaveCheckpointRequest) (bool, error)
    RestoreCheckpoint(ctx context.Context, workspace, checkpointID string) error

    GetManifest(ctx context.Context, workspace, view string) (Manifest, error)
    GetBlob(ctx context.Context, workspace, blobID string) ([]byte, error)
    EnsureBlobs(ctx context.Context, workspace string, blobs map[string][]byte) error

    GetTree(ctx context.Context, workspace, view, path string, depth int) (TreeResponse, error)
    GetFileContent(ctx context.Context, workspace, view, path string) (FileContentResponse, error)
    ListActivity(ctx context.Context, workspace string, limit int) (ActivityListResponse, error)
}
```

### Implementations

- `DirectBackend`
  - wraps the current `controlplane.Service` and `Store`
  - preserves current behavior

- `HTTPBackend`
  - talks to an AFS API server
  - is used by both `self-hosted` and `cloud`

This is the single most important codebase change because it decouples the CLI product from direct Redis access.

## Control Plane API Direction

The existing AFS `/v1` contract is a good base and should be preserved.

The hosted API should keep the current workspace-first model, but add the missing write/sync operations needed by a remote CLI.

### Keep the existing browse-oriented endpoints

- `GET /v1/workspaces`
- `GET /v1/workspaces/{workspace_id}`
- `POST /v1/workspaces`
- `DELETE /v1/workspaces/{workspace_id}`
- `GET /v1/workspaces/{workspace_id}/tree`
- `GET /v1/workspaces/{workspace_id}/files/content`
- `GET /v1/activity`
- `GET /v1/workspaces/{workspace_id}/activity`

### Add the missing remote-workflow endpoints

- `GET /v1/workspaces/{workspace_id}/checkpoints`
- `POST /v1/workspaces/{workspace_id}/checkpoints`
- `POST /v1/workspaces/{workspace_id}:restore`
- `GET /v1/workspaces/{workspace_id}/manifest?view=head|checkpoint:<id>`
- `POST /v1/workspaces/{workspace_id}/blobs:missing`
- `PUT /v1/workspaces/{workspace_id}/blobs/{blob_id}`
- `GET /v1/workspaces/{workspace_id}/blobs/{blob_id}`
- `POST /v1/workspaces/{workspace_id}/sessions`
- `DELETE /v1/workspaces/{workspace_id}/sessions/{session_id}`

### Why split manifest/blob sync from file APIs

The browser/editor APIs are fine for UI use.

The CLI needs a higher-throughput path for:

- materializing a workspace
- uploading changed blobs
- creating a checkpoint with optimistic concurrency

Using manifest/blob endpoints lets us reuse AFS’s current saved-state model directly instead of rebuilding it around per-file REST calls.

## Authentication Model

AFS should copy the shape of the other repo’s auth system, but not its assumptions.

### Cloud mode

Support two auth styles:

- session auth for human CLI use
- API keys or workspace tokens for automation

Suggested commands:

- `afs auth login`
- `afs auth logout`
- `afs auth status`

The CLI stores:

- profile metadata in config
- secrets in keychain/keyring with encrypted fallback

The server accepts:

- session auth for interactive management routes
- token auth for automation and headless tools

### Self-hosted mode

Support one of these server-side choices:

- bootstrap admin token written once on first boot
- local JWT auth for multi-user installs
- localhost-only no-auth mode for single-user development

The important rule is:

- self-hosted mode must not require an AFS Cloud account

### Direct mode

No account and no fake local account.

Direct mode should remain:

- config + Redis connection + local filesystem behavior

That is simpler and better than forcing a local login story onto single-user standalone AFS.

## Profile and Config Model

AFS should move from one implicit config to named profiles.

Suggested shape:

```json
{
  "default_profile": "local",
  "profiles": {
    "local": {
      "mode": "direct",
      "redis_addr": "localhost:6379",
      "redis_username": "",
      "redis_db": 0,
      "redis_tls": false,
      "work_root": "~/.afs/workspaces",
      "mount_backend": "auto",
      "mountpoint": "~/afs",
      "current_workspace": ""
    },
    "prod": {
      "mode": "cloud",
      "api_url": "https://api.afs.example.com",
      "mcp_url": "https://mcp.afs.example.com",
      "account_id": "acct_123",
      "work_root": "~/.afs/workspaces",
      "current_workspace": ""
    }
  }
}
```

### Secret handling

Do not store these in plain config:

- session refresh token
- session cookie/token material
- API keys
- bootstrap admin token copies

Use:

- OS keyring when available
- encrypted local fallback when it is not

### Migration

Preserve the existing experience by migrating the current `afs.config.json` into a generated `local` profile.

Compatibility rule:

- if no multi-profile config exists, read the old config and treat it as `local`

## CLI Surface Changes

### Existing commands remain

Keep all existing workspace/checkpoint commands.

### Add global profile selection

- `afs --profile prod workspace list`
- `AFS_PROFILE=prod afs workspace run my-repo -- make test`

### Add auth commands

- `afs auth login`
- `afs auth logout`
- `afs auth status`

### Add profile helpers

- `afs profile list`
- `afs profile use <name>`
- `afs profile show`

### Keep `afs setup` focused on standalone

`afs setup` should continue to optimize for local/direct mode.

For cloud mode, setup becomes:

1. create or select a cloud profile
2. set `api_url`
3. run `afs auth login`

## Workspace Session Model

To make the CLI “managed by the cloud” instead of merely “able to hit an API”, add workspace sessions.

Each active CLI session should register:

- workspace id
- user or token principal
- hostname
- local path
- CLI version
- started at / last heartbeat
- current head seen by the client

This enables:

- active-session visibility in the UI
- better audit trails
- safer concurrency messaging
- future lease/presence behavior for multi-client workspaces

## Materialization and Save Flow

### `workspace run` in direct mode

Keep the current flow.

### `workspace run` in cloud/self-hosted mode

Use this flow:

1. Resolve auth and backend from the active profile.
2. Create a workspace session.
3. Fetch the current head manifest.
4. Download only the blobs needed to materialize the working copy.
5. Run the command locally in `~/.afs/workspaces/<workspace>/tree`.
6. Build a new manifest on exit.
7. Negotiate which blobs the server is missing.
8. Upload only missing blobs.
9. Create a checkpoint with `expected_head`.
10. Close the workspace session.

This preserves the existing AFS hybrid model:

- canonical saved state is remote
- real execution happens in a normal local directory

## `afs up` / Mounted Filesystem Strategy

This is the trickiest part because the current mount path assumes direct Redis access.

### Recommendation

Treat mounted filesystem support as two separate implementations:

#### Direct mode

Keep the current Redis-backed FUSE/NFS behavior.

#### Cloud/self-hosted mode

Do not require direct access to managed Redis.

Instead, use a local managed working copy:

- materialize the workspace into the local tree
- optionally expose the selected mountpoint as a symlink or bind-style view to that tree
- checkpoint explicitly or via a local background watcher/daemon

This avoids leaking long-lived Redis credentials to the client and keeps the “cloud mode uses HTTP, not direct Redis” rule intact.

### Practical rollout

Phase 1:

- full support for `workspace run`, `clone`, `checkpoint`, browse APIs, and UI in cloud mode
- keep live mounted filesystem behavior as direct-mode only

Phase 2:

- add a local workspace daemon for cloud/self-hosted profiles
- let `afs up` expose a managed local working tree with background save/refresh behavior

## Server Structure

AFS should split “local control plane implementation” from “transport exposure”.

### Suggested structure

- move the current reusable workspace/control-plane logic into a package that both CLI direct mode and servers can call
- keep the current local server command as the development/local API binary
- add auth middleware only at the server layer
- add an HTTP client package for the CLI remote backend

The key principle is:

- business logic should not know whether the caller is local or remote

## Rollout Plan

### Phase 1: Backend boundary

- introduce `Backend` interface in the CLI
- refactor current commands to use `DirectBackend`
- no product change yet

### Phase 2: Complete the HTTP API

- add checkpoint creation/listing endpoints
- add manifest/blob sync endpoints
- add an HTTP backend client

### Phase 3: Profiles and credentials

- add multi-profile config
- add secure credential storage
- add `afs auth ...` commands
- migrate legacy config to `local`

### Phase 4: Hosted/self-hosted auth

- add server auth middleware
- support session auth + API keys for cloud
- support bootstrap token and/or local JWT auth for self-hosted

### Phase 5: Cloud-managed sessions

- add workspace session records
- expose them in UI/activity
- wire optimistic concurrency and better conflict reporting through the API

### Phase 6: Cloud-mode `afs up`

- add local workspace daemon and optional background save/refresh
- keep direct-mode live mount untouched

## Recommended Product Positioning

AFS should describe itself as:

- standalone by default
- cloud-manageable when you want it

Not:

- cloud-first with a local fallback

That positioning matches the current codebase and keeps the design honest.

## Bottom Line

The best way to replicate the `cloud-context-engine` design in AFS is:

- copy the profile/auth/control-plane transport pattern
- do not copy the assumption that every serious deployment starts from a cloud account
- make `direct` a first-class backend, not a compatibility mode
- make `cloud` and `self-hosted` HTTP backends that sit beside it

If we do that, AFS gets:

- a managed cloud story
- a self-hosted story
- a zero-account standalone story

without fragmenting the command surface or the workspace model.
