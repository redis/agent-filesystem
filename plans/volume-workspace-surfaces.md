# Volume + Workspace User Surfaces

Status: implementation pass landed
Owner: rowan
Created: 2026-05-08
Updated: 2026-05-09

Pair plan: [`volume-workspace-model.md`](volume-workspace-model.md) (data
model substrate). This plan covers the user-facing rollout: CLI commands,
UI navigation, route changes, copy, and docs.

## Goal

Roll out the Volume + Workspace information architecture to users:

- Rename today's "Workspace" surfaces to **Volume**
- Introduce **Workspace** as a first-class composition concept in CLI and UI
- Reshuffle UI navigation so Monitor hosts live runtime state and the
  Workspace/Volume pair separates configuration from content

## Scope

**In scope:**

- CLI: `afs vol`, `afs ws`, `afs daemon` command trees; flag rename
- UI: navigation, routes, page content, redirects, banner copy
- Concept-level docs: `README.md`, `AGENTS.md`, `SKILL.md`,
  `docs/guides/agent-filesystem.md`, CLI/MCP/SDK reference docs
- Templates: copy fields and profile mapping
- Migration messaging: in-CLI errors, in-UI banner, dedicated migration guide

**Out of scope:**

- The data model — see model plan
- MCP wire identifier rename
- Workspace templates (composition templates)

## Information architecture

### Navigation order and labels

| Position | Today | New | Notes |
|---|---|---|---|
| 1 | Monitor | Monitor | Now hosts Live Topology + Connection History |
| 2 | Workspaces | **Workspaces** | New page — list of compositions (was Agents) |
| 3 | Agents | **Volumes** | Renamed from Workspaces |
| 4 | MCP | MCP | Token UI updated for (scope, capability) |
| 5 | Databases | Databases | Unchanged |
| 6 | History | History | Unchanged |
| 7 | Admin | Admin | Unchanged |

Implementation: edit `ui/src/layout/navigation-items.ts`.

### Per-page summary

#### Monitor (`/`)
- Existing dashboard
- New: **Live Topology** (move `live-topology-card.tsx` here)
- New: **Connection History** (move from Agents)
- Rule: anything answering "what is happening *now*" lives on Monitor

#### Workspaces (`/workspaces`) — NEW PAGE LOGIC
- List of workspace compositions
- Per row: name, mounted volumes (count + truncated names), connected agent
  status, last activity
- Detail (`/workspaces/{id}`):
  - Mount manifest editor: ordered list of `{volume, path, readonly}`
  - Add/remove volume; reorder; toggle readonly
  - Cross-owner mounts: paste a volume token to mount someone else's volume
  - Connected agents (live)
  - Bookmarks tab (composition-level snapshot list)
  - Issue workspace token action

#### Volumes (`/volumes`) — RENAMED FROM `/workspaces`
- Same content as today's Workspaces page
- Per row: name, file count, last checkpoint, active sessions
- Detail (`/volumes/{id}`):
  - File tree, checkpoints, forks, sessions (today's tabs)
  - Issue volume token action
  - "Used by N workspaces" — links to compositions mounting this volume

#### MCP (`/mcp`)
- Token management with new (scope, capability) creation flow
- Filter by scope: volume / workspace / database / control-plane
- Filter by capability ladder

## Routes and redirects

```
/workspaces            serves NEW Workspaces page (compositions)
/workspaces/{id}       if {id} starts with `vol_`, redirect to /volumes/{id}
                       if {id} starts with `ws_`, serve composition page
/agents                301 → /workspaces
/agents/{id}           301 → /workspaces/{id}
/volumes               NEW route, serves volumes list
/volumes/{id}          NEW route, serves volume detail
```

ID prefixes (`vol_`, `ws_`) make the redirect deterministic and
collision-free.

`/workspaces` (no id) is intentionally NOT redirected — it serves the new
page. Users with old bookmarks land on the new page; a one-time dismissible
banner ("Looking for what used to be Workspaces? They are now Volumes — see
the Volumes tab.") helps them re-orient. Banner shown for 30 days
post-launch, then removed.

## CLI

### Command vocab

```
afs vol create <name>
afs vol import <name> <directory>
afs vol mount <volume> <path>
afs vol unmount <volume-or-path>
afs vol list
afs vol fork <source> <target>
afs vol show <volume>

afs ws create <name>
afs ws add <workspace> <directory> [--name <volume>] [--at <path>] [--readonly]
afs ws attach <workspace> [volume] [--at <path>] [--readonly] [--token <volume-token>]
afs ws detach <workspace> <volume>
afs ws mount <workspace> <root>
afs ws unmount <workspace-or-root>
afs ws list
afs ws show <workspace>
afs ws bookmark create <workspace> <name>
afs ws bookmark list <workspace>
afs ws bookmark restore <workspace> <name>

afs cp create <volume> <name>
afs cp list <volume>
afs cp restore <volume> <name>

afs daemon status
afs daemon stop

afs mcp --workspace <name> --capability <ro|rw|rw-checkpoint>
afs mcp --volume <name> --capability <ro|rw|rw-checkpoint>
```

### Breaking changes

- `afs ws ...` semantics change. Today operates on what becomes a volume;
  tomorrow operates on workspaces (compositions).
- `afs ws create <name>` returns an empty composition, not a tree
- `afs cp ...` retargets to volumes only (`afs cp create <volume>`)
- `--workspace` flag → `--volume` on commands acting on a single tree
  (cp, fs, mcp). `--workspace` is reused for compositions on `afs ws`
  commands.

### Migration UX

No silent compat aliases — semantics differ enough that aliasing would
mislead. Old commands print a clear error pointing at the new shape:

```
$ afs ws create my-data
Error: 'afs ws' now manages workspaces (compositions of mounted volumes).
       To create a volume (formerly known as a workspace), use:
         afs vol create my-data
       See https://afs.cloud/docs/migration/volume-rename for details.
```

Detection heuristic: if `afs ws create` is invoked with the legacy
single-positional-name shape AND the user has no existing compositions,
print the migration error. Otherwise proceed with the new semantics. After
the deprecation horizon (90 days), the heuristic is removed and `afs ws
create` is unconditionally the new behavior.

## Documentation

- `README.md` — concept-level rewrite, comparison table, new diagram
- `AGENTS.md`, `SKILL.md` — Core Model section
- `docs/guides/agent-filesystem.md` — full rewrite of Core Model + Agent
  Operating Loop + MCP setup examples
- `docs/reference/cli.md` — new command reference (regenerate or rewrite)
- `docs/reference/mcp.md` — token (scope, capability) section + profile
  mapping table
- `docs/reference/typescript.md`, `python.md` — concept references; SDK
  method names track API and stay
- New: `docs/guides/migration-volume-rename.md` — authoritative migration
  guide; cited from CLI errors and UI banner

## Templates

- Templates remain volume installers
- `templates/*/manifest.json`: update user-facing copy fields (`tagline`,
  `whyItMatters`, `firstPrompt`, `summary[]`)
- `profile` field in template manifests stays as a wire identifier
  (mapped server-side to (scope=volume, capability=...))
- Add "workspace templates" to future plan list — not in this pass

## Component-level UI changes

Non-exhaustive; final list driven by build failures during the change.

- `ui/src/routes/workspaces.tsx` → `volumes.tsx`
- `ui/src/routes/workspaces.$workspaceId.tsx` → `volumes.$volumeId.tsx`
- `ui/src/routes/workspace-studio/` → `volume-studio/`
- `ui/src/routes/templates.installed.$workspaceId.tsx` →
  `templates.installed.$volumeId.tsx`
- `ui/src/routes/agents.tsx` → `workspaces.tsx` (page logic replaced, not
  pure rename — now lists compositions)
- `ui/src/components/live-topology-card.tsx` — relocate import into Monitor
  route
- `ui/src/components/connect-agent-banner.tsx` — copy update
- `ui/src/components/getting-started-onboarding-dialog.tsx` — copy + flow
  update (now: create a workspace, add a volume, mount)
- `ui/src/components/onboarding-drawer.tsx` — copy update
- `ui/src/components/mcp-connection-panel.tsx` — token creation UI for new
  (scope, capability)
- `ui/src/foundation/types/afs.ts` — display labels (wire types track API
  and update with the model plan)
- `ui/src/features/agents/CreateMCPAccessDialog.tsx`,
  `LocalMCPAccessDialog.tsx` — token creation flow with scope/capability
- `ui/src/features/templates/templates-data.ts` — copy
- `ui/src/foundation/tables/access-tokens-table.tsx` — show new scope and
  capability columns

## Phases

### Phase 1 — Volume rename (3–4 days)

- [x] Rename `/workspaces` route to `/volumes`; rename component files
- [x] Update sidebar nav label "Workspaces" → "Volumes" (temporarily —
      Phase 2 reorders)
- [x] Update copy on volume detail and list pages
- [x] CLI: `afs vol` command tree (parallel to existing `afs ws` until
      Phase 3 cuts over)
- [x] `--volume` flag added to `cp`, `fs`, `mcp`; `--workspace` retained
      temporarily

### Phase 2 — Workspace composition pages (5–7 days)

- [x] New `/workspaces` route serving the composition list (replaces old
      `/agents` content as repurposed page)
- [ ] Workspace detail with manifest editor (add/remove/reorder volumes,
      readonly toggle)
- [x] Workspace detail manifest viewer
- [x] Bookmarks section on workspace detail
- [ ] Cross-owner mount UI (paste volume token)
- [x] Token creation flow updated for (scope, capability) in MCP page
- [x] Sidebar nav reorder: Monitor → Workspaces → Volumes → MCP →
      Databases → History → Admin
- [x] Reorientation notice on `/workspaces` for migrating users

### Phase 3 — Monitor refactor (2–3 days)

- [x] Move Live Topology card to Monitor
- [ ] Move connection history to Monitor
- [x] Remove `/agents` page content; add app-level redirect from `/agents`
      to Monitor

### Phase 4 — CLI cutover (3–4 days)

- [x] `afs ws` command tree replaced with composition semantics
- [x] Migration-error path on legacy `afs ws` file-tree subcommands
- [x] `afs ws` composition manifest subcommands exposed
- [x] `afs daemon status`, `afs daemon stop` exposed
- [ ] Deprecate `--workspace` flag on volume-targeting commands; warn-once
- [x] Update CLI integration tests

### Phase 5 — Docs and templates (3 days)

- [ ] Rewrite concept-level pages (README, AGENTS, SKILL,
      `docs/guides/agent-filesystem.md`)
- [ ] Regenerate `docs/reference/cli.md`
- [ ] Update MCP/TS/Python references for new vocabulary; SDK method names
      stay
- [ ] Author `docs/guides/migration-volume-rename.md`
- [ ] Update template manifests' user-facing copy

### Phase 6 — Verification (2 days)

- [ ] `make build` and `make lint` pass
- [x] `cd ui && npm run build` passes
- [x] `go test ./cmd/afs ./internal/controlplane` passes
- [x] UI dev server starts; renamed routes render; redirect smoke test passes
- [ ] Round-trip: create workspace → add ro volume + rw volume → mount via
      CLI → confirm read/write enforcement → bookmark → restore
- [ ] Token round-trip: workspace `rw-checkpoint` token → mount, write,
      checkpoint, restore — capability ladder enforced
- [ ] Existing CLI scripts produce useful migration errors
- [ ] Migration banner appears on `/workspaces` for users with legacy
      bookmarks

## Acceptance criteria

- A user can create a workspace, add two volumes (one ro, one rw), mount
  via CLI, and observe write enforcement matching the manifest
- A user with a 30-day-old `/workspaces/vol_xyz` bookmark lands on
  `/volumes/vol_xyz` with no error
- A user invoking the old `afs ws create my-data` shape receives a
  migration error pointing at `afs vol create` and the migration guide
- 100-volume mount stress test completes in 1 daemon process
- All MCP clients using legacy profile strings continue to function

## Open questions

- **Sidebar icon for Workspaces.** `BotIcon` (current Agents) feels off for
  compositions. Candidates: `LayersIcon`, `BoxIcon`, `PackageIcon`. Decide
  before Phase 2.
- **Banner duration.** 30 days proposed. Adjust based on analytics on
  `/workspaces` direct hits vs nav-driven traffic.
- **Onboarding flow.** New users today land on a "create your first
  workspace" path. Should new flow be "create your first volume" or
  "create your first workspace (composition)"? Recommend the former —
  volume is the simpler atomic unit, composition can come later in their
  journey.

## Implementation Review — 2026-05-09

Landed a compatibility-first user-surface pass:

- Added `afs vol` as the explicit content-tree command family and added
  `--volume` to `afs fs`, `afs cp`, and `afs mcp`.
- Added `afs daemon status` and `afs daemon stop`.
- Added workspace-manifest CLI subcommands. The current user-facing verbs are
  `create`, `list`, `show`, `add`, `attach`, `detach`, `mount`, `unmount`,
  `bookmark`, and `restore-bookmark`.
- Split the UI into `/workspaces` for composition manifests and `/volumes`
  for the existing file studio, with generated route tree updates.
- Moved Live Topology onto Monitor and turned `/agents` into a redirect.
- Updated MCP token creation/display to show volume scope and capability.

Still remaining:

- Detail-page manifest editing UI is read-only in this pass; CLI/API can add
  and remove mounts.
- Full docs/template vocabulary migration remains open.

## Implementation Review — 2026-05-10

Finished the CLI cutover:

- `afs ws create/list/show/add/attach/detach/mount/unmount/bookmark` now
  targets Agent Workspace composition manifests.
- Legacy file-tree operations moved behind `afs vol`, and root shortcuts such
  as `afs mount`, `afs list`, and `afs import` now dispatch through `afs vol`.
- `afs vol list` lists volumes; `afs ws list` lists Agent Workspaces only.
- Mount/setup/status/auth/config copy now points users at `afs vol mount` for
  direct content-tree mounts.
- Added CLI coverage proving `afs ws list` does not show raw volume rows.
- Fixed `afs ws mount <workspace>` so it resolves the Agent Workspace manifest
  before prompting for a local folder and accepts the default `~/workspace`
  root for mounted volumes.
- Corrected root `afs mount` and `afs unmount` so they target Agent Workspace
  manifests. Direct single-volume lifecycle remains under `afs vol mount` and
  `afs vol unmount`.
- Added mount-registry metadata for Agent Workspace mounts so no-arg
  `afs unmount` can list mounted Agent Workspaces instead of raw volume rows.
- Follow-up fix: `afs status` now aggregates child volume mount records into a
  single Mounted workspaces row and keeps direct volume mounts in a separate
  Mounted volumes section. Agent Workspace unmount now removes all grouped child
  mounts with one workspace-level result instead of printing per-volume
  unmounts.

Verification:

- `go test ./cmd/afs`
- `go test ./cmd/afs -run 'TestCmdStatusAggregates|TestCmdStatusVerboseKeepsSingleVolumeWorkspaceGrouped|TestCmdStatusPrintsAlignedMountTable|TestCmdStatusDoesNotListStoppedRecordsAsMounted|TestCmdStatusVerboseIncludesConnectionDetails|TestRootUnmountByAgentWorkspace|TestRootUnmountPrompts|TestRootMountPrompts'`
- `go test ./cmd/afs -run 'TestWorkspace(Mount|CommandsManage)'`
- `go test ./cmd/afs -run 'Test(Root|VolumeRootShortcuts|WorkspaceMount|WorkspaceCommandsManage)'`
- `go test ./internal/controlplane -run TestHTTPV2VolumesAndWorkspaceCompositions`
- `make commands`

## Implementation Review — 2026-05-11

Applied the CLI ergonomics pass from developer feedback:

- `afs ws add` now imports a local folder into a new volume and attaches it to
  the Agent Workspace, with default volume and mount-path names derived from
  the folder basename.
- `afs ws attach` is the existing-volume path, defaults the in-workspace path
  from the volume name, and prompts for a volume when one is not supplied.
- Removed the legacy `afs ws remove` alias; detach is the workspace manifest
  verb.
- Updated root, `ws`, and `vol` help copy so `vol` is clearly the direct
  underlying volume management surface.
- Read-only sync unmount now releases local filesystem write protections before
  preserving or deleting the local tree.

Verification:

- `go test ./cmd/afs`
- `make commands`
