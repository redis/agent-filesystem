# Volume + Workspace Data Model

Status: draft
Owner: rowan
Created: 2026-05-08
Updated: 2026-05-08

Pair plan: [`volume-workspace-surfaces.md`](volume-workspace-surfaces.md) (CLI + UI rollout).

## Goal

Introduce a two-layer information model:

- **Volume** — one named, checkpointable, forkable file tree. Atomic unit of
  persistence and content history. Today's `WorkspaceMeta` renamed.
- **Workspace** — a manifest composing one or more mounted volumes with
  per-volume permissions. Has no file content of its own. The natural unit
  of agent configuration.

This is the substrate the surfaces plan builds on.

## Scope

**In scope:**

- Go data model: rename `Workspace*` types to `Volume*`; introduce
  `Workspace*` (composition) types
- Redis schema for volumes and workspace manifests
- HTTP API: `/v2/volumes/...` and `/v2/workspaces/...`
- Access token model: (scope, capability) two-axis design
- Daemon: singleton-per-user-per-machine, multiplexes mount sessions
- Workspace bookmarks (composition-level checkpoint references)
- MCP backwards-compat layer for tool names and capability profile strings
- Migration script + cutover plan for existing data and tokens

**Out of scope:**

- All user-facing surfaces (CLI, UI, copy, docs) — see surfaces plan
- MCP wire identifier rename (`workspace_*` tools, `workspace-*` profile
  strings) — separate future plan
- SDK rewrites beyond the minimum needed to back the new endpoints
- Workspace templates (composition templates) — future

## Concepts

### Volume

- Same shape as today's `WorkspaceMeta` (`internal/controlplane/store.go:51`)
- Owns its checkpoint timeline, manifest, blobs, fork lineage
- Addressable by `(database_id, volume_id)`
- Volume IDs prefixed `vol_` going forward

### Workspace

- A manifest of mount entries, ordered for deterministic application
- Owned by a database; addressable by `(database_id, workspace_id)`
- Addressable IDs prefixed `ws_` going forward
- Carries no file content; all content flows through mounted volumes

### Mount entry

```
{
  volume_id: string,        // which volume to mount
  mount_path: string,       // relative path under workspace root, e.g. "/skills"
  readonly: bool,           // forces ro even if token would allow rw
  volume_token_id?: string  // optional, for cross-owner volume access
}
```

Mount paths are *relative*. The absolute root is supplied by the client at
mount time, so the same workspace works across machines.

### Mount conflict policy

- Same `(volume_id, mount_path)` mounted twice → idempotent no-op
- Same volume at different paths → two independent sessions, allowed
- Two volumes at same `mount_path` within one workspace → reject at API write
- Overlapping paths within one workspace (`/a` and `/a/b` from different
  volumes) → reject at API write

## Access tokens

Two orthogonal axes.

### Scope

| Scope | Reaches |
|---|---|
| `volume:<id>` | Exactly one volume |
| `workspace:<id>` | All volumes mounted by that workspace, with manifest readonly applied |
| `database:<id>` | All volumes and workspaces in the database |
| `control-plane` | Everything in the user's control plane |

### Capability

| Capability | Allows |
|---|---|
| `ro` | Read files, read checkpoints |
| `rw` | `ro` + write/edit files |
| `rw-checkpoint` | `rw` + create/restore volume checkpoints, create/restore workspace bookmarks |
| `admin` | `rw-checkpoint` + manage volumes, workspaces, tokens |

### Rules

- A token is `(scope, capability, optional expiry)`
- Capability is uniform across the scope
- Per-volume `readonly` in a manifest is enforced *in addition to* the
  token capability — a workspace `rw` token cannot write to a volume mounted
  readonly
- Cross-owner mounts: when a workspace mounts a volume not owned by the
  workspace owner, the manifest entry must reference a `volume_token_id`
  granting access. Removing/expiring that volume token invalidates the
  mount entry but leaves the workspace intact
- Multi-token agents: agents needing mixed access across multiple workspaces
  hold multiple tokens. No compound policy in a single token

### Mapping from current MCP profiles

| Today | Tomorrow |
|---|---|
| `workspace-ro` | scope `volume:<id>`, capability `ro` |
| `workspace-rw` | scope `volume:<id>`, capability `rw` |
| `workspace-rw-checkpoint` | scope `volume:<id>`, capability `rw-checkpoint` |
| `admin-ro` | scope `control-plane`, capability `ro` |
| `admin-rw` | scope `control-plane`, capability `admin` |

The wire-level `workspace-*` profile strings remain valid identifiers in
MCP for backwards compat. Internal Go switches to (scope, capability)
tuples; a translation layer maps incoming legacy strings.

## Checkpoints

- **Volumes own checkpoint timelines.** No content history at the workspace
  layer.
- A **workspace bookmark** is a named tuple
  `(workspace_id, name, [{volume_id: checkpoint_id}, ...])` capturing one
  head per mounted volume at bookmark time.
- "Restore workspace bookmark X" iterates the recorded list and restores
  each volume to its referenced checkpoint. Atomic intent at the workspace
  level; canonical truth still at the volume.
- Bookmarks soft-pin referenced checkpoints (volumes record back-references
  so checkpoint GC honors active bookmarks).

## Daemon model

- One daemon process per user per machine. Singleton; ref-counted lifecycle.
- Daemon holds N mount sessions across any number of volumes and workspaces.
- `afs vol mount` and `afs ws mount` register sessions with the same daemon.
  `ws mount` expands its manifest into N session registrations.
- Per-session lease (existing model). On revocation/expiry, the affected
  session unmounts; other sessions continue.
- Scaling: 100 mounts → 100 sessions in 1 process. Real cliffs are Redis
  pubsub fan-out and FUSE handle limits, not OS process count.
- New ops surface: `afs daemon status`, `afs daemon stop` (see surfaces plan).

## API surface

New `/v2/` namespace. `/v1/workspaces` (current) keeps working through the
deprecation window and reads identical data to `/v2/volumes` (same backing
record).

```
POST   /v2/volumes
GET    /v2/volumes
GET    /v2/volumes/{id}
DELETE /v2/volumes/{id}
POST   /v2/volumes/{id}:fork
POST   /v2/volumes/{id}:restore
GET    /v2/volumes/{id}/checkpoints
POST   /v2/volumes/{id}/checkpoints
POST   /v2/volumes/{id}/checkpoints/{name}:restore

POST   /v2/workspaces
GET    /v2/workspaces
GET    /v2/workspaces/{id}
DELETE /v2/workspaces/{id}
PUT    /v2/workspaces/{id}/mounts                # bulk replace manifest
POST   /v2/workspaces/{id}/mounts                # add one mount entry
DELETE /v2/workspaces/{id}/mounts/{volume_id}
GET    /v2/workspaces/{id}/bookmarks
POST   /v2/workspaces/{id}/bookmarks
POST   /v2/workspaces/{id}/bookmarks/{name}:restore
```

Token operations gain `scope` and `capability` fields; the legacy `profile`
field is read-only and computed from (scope, capability) for compat.

## Redis schema

Today:

```
afs:database:{db}:workspace:{id}
afs:database:{db}:workspace:{id}:savepoints
```

After:

```
afs:database:{db}:volume:{id}                    # was workspace:{id}
afs:database:{db}:volume:{id}:checkpoints
afs:database:{db}:workspace:{id}                 # NEW shape: composition
afs:database:{db}:workspace:{id}:bookmarks
```

The `workspace:{id}` namespace is reused for the *new* concept. Migration
moves existing data out of that key first.

## Migration

Breaking change at API and Redis-key level. Mitigated via:

1. **Versioned API.** `/v1/workspaces` continues during the deprecation
   window, served by a compat layer that reads from the new volume keys.
   `/v2` is authoritative.
2. **One-shot Redis migration script.** For each existing workspace:
   - Copy `WorkspaceMeta` payload to a new `volume:{id}` key
   - Rewrite token records from `workspace:<id>` scope to `volume:<id>` scope
   - Leave the old `workspace:{id}` key in place for the duration of the
     deprecation window, then delete after cutover
3. **No automatic workspace creation.** Migration does not auto-create a
   composition workspace per existing volume. Volumes work standalone via
   `afs vol mount` for users who don't need composition.
4. **Token compat.** Existing MCP tokens preserve effective access. Their
   wire `profile` string is unchanged; a server-side mapping populates
   (scope, capability) on read.

## Phases

### Phase 1 — Internal types and storage (3–4 days)

- [ ] Rename `WorkspaceMeta` → `VolumeMeta` in `internal/controlplane/`
- [ ] Rename `WorkspaceSessionRecord` → `VolumeSessionRecord` (sessions
      remain volume-bound; workspace sessions are aggregations)
- [ ] Add `WorkspaceMeta` (new) carrying the manifest and bookmarks
- [ ] Update Redis key constants; add new key shape for compositions
- [ ] Update `internal/controlplane/store.go`, `service.go` accordingly
- [ ] Backwards-compat read shim: `/v1/workspaces` reads from new volume
      keys without exposing the new shape

### Phase 2 — API endpoints (3–4 days)

- [ ] `/v2/volumes/...` endpoints (mostly direct rename of `/v1/workspaces`)
- [ ] `/v2/workspaces/...` endpoints (NEW)
- [ ] Manifest CRUD: PUT/POST/DELETE on `/v2/workspaces/{id}/mounts`
- [ ] Bookmarks: list, create, restore
- [ ] Validation: mount-path uniqueness, path overlap rejection,
      cross-owner volume_token_id enforcement
- [ ] HTTP integration tests

### Phase 3 — Token model (2–3 days)

- [ ] Add `Scope` and `Capability` fields on `mcpAccessTokenRecord`
- [ ] Compute `Profile` (legacy field) from (scope, capability) for compat
- [ ] Token issuance API accepts new fields; old `profile` accepted for compat
- [ ] Capability ladder enforcement in HTTP handlers and MCP tool gating
- [ ] Manifest readonly enforcement (workspace token + volume mount readonly
      = effective ro)

### Phase 4 — Daemon multiplexing (4–5 days)

- [ ] Daemon refactor to host N concurrent mount sessions
- [ ] Singleton lifecycle with reference counting
- [ ] `afs daemon status` and `afs daemon stop` support endpoints
- [ ] Lease watchdog per session; one session failure does not affect others
- [ ] Stress test at 50 and 100 concurrent mounts

### Phase 5 — Migration (2–3 days)

- [ ] Redis migration script (idempotent; safe to rerun)
- [ ] Token scope rewrite (`workspace:<id>` → `volume:<id>`)
- [ ] Compat read layer for `/v1/workspaces`
- [ ] Cutover runbook (staging → production)
- [ ] Deprecation window: 90 days from launch, then `/v1/workspaces` 410s

### Phase 6 — MCP compat (1–2 days)

- [ ] Add `volume_*` tool aliases for `workspace_*` tools; both route to the
      same handlers against volumes
- [ ] Add new `workspace_compose_*` tools (names tentative; review before
      merge): `_create`, `_mount_add`, `_mount_remove`, `_show`,
      `_bookmark_create`, `_bookmark_restore`
- [ ] Profile-string ↔ (scope, capability) translation in
      `mcp_profiles.go`

## Open questions

- **Bookmark retention semantics.** Hard-pin or soft-pin? Recommend soft-pin
  with explicit warning when GC would orphan a bookmark.
- **Workspace ownership transfer.** When a workspace is shared, are embedded
  volume_token_ids transferred? Recommend no — receiver supplies their own
  tokens for any volumes they don't already own.
- **Empty workspaces.** Allowed? Recommend yes.
- **Mount-path namespace policy.** Reject overlap (`/a` and `/a/b`)?
  Recommend yes; ship as a hard validation error.
- **`/v1/workspaces` deprecation horizon.** Recommend 90 days. Confirm with
  ops/comms.
- **Daemon location.** Per-user-per-machine vs per-machine. Recommend
  per-user — cleaner permission boundaries.
