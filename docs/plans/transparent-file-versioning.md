# Transparent File Versioning

Last reviewed: 2026-04-28.
Status: proposed design spec.

Add optional automatic per-file version history to AFS.

Agents, shells, editors, sync mode, live mount mode, MCP file reads, and the
browser should continue to see the latest workspace state by default. When
versioning is enabled for a workspace or matching path globs, every successful
mutation of a tracked path records an immutable file version that can later be
listed, read, diffed, restored, or undeleted through explicit `afs`, MCP, and
HTTP surfaces.

---

## Today

AFS already has two nearby primitives:

- explicit workspace checkpoints
- per-path changelog entries with path, op, hashes, source, and timestamp

What AFS does not have today is first-class automatic file history that lets a
user or agent say:

- show `/src/main.go` at an earlier version
- diff version A vs version B of one file
- restore only one file to an earlier version
- undelete a file from its own history

Current workspace browsing is workspace-scoped, not file-version-scoped:

- `head`
- `checkpoint:<id>`
- `working-copy`

That model should stay intact. File history should be a sidecar capability, not
an extra virtual tree exposed by default in the workspace browser.

---

## Goals

- Keep the normal filesystem and browser views latest-only.
- Make versioning optional per workspace.
- Allow path-scoped policy with glob include and exclude rules.
- Record immutable file versions for successful tracked mutations.
- Give every tracked file lineage a stable ordered version sequence.
- Reuse existing blob identities when content already exists.
- Support file-scoped list, show, diff, restore, and undelete flows.
- Preserve file lineage across rename.
- Make history portable with the workspace across import, clone, and fork.
- Keep checkpoints as the workspace-wide restore primitive.

## Non-goals

- Replacing checkpoints with automatic versioning.
- Exposing old versions as normal writable filesystem paths.
- Supporting in-place deletion of arbitrary historical versions in normal UX.
- Building line-level or semantic diff storage into the version record itself.
- Recording versions for untracked paths when versioning is off for that path.
- Solving retention for every workload in the first slice.

---

## Product Model

Transparent file versioning is a third history layer:

1. Working copy: the latest mutable workspace state.
2. Checkpoints: explicit immutable workspace-wide snapshots.
3. File versions: automatic immutable per-file history for tracked paths.

The three layers serve different jobs:

- working copy is for normal tools
- checkpoints are for whole-workspace save and restore
- file versions are for path-level history and recovery

Versioning must not make the workspace browser or live mount ambiguous. When an
agent opens `/src/main.go`, it should still get the latest file. Older versions
must be accessed explicitly through commands or APIs.

Important concurrency assumption:

- AFS should not be modeled as globally serialized across all writers.
- Some write paths are atomic, and some per-client workers are serial, but
  multiple sessions and clients can still race on the same workspace.
- File version ordering must therefore come from per-lineage sequencing and
  atomic lineage updates, not from timestamps or a belief that all writes land
  one-at-a-time globally.

---

## User Experience

### Workspace settings

Versioning lives in workspace settings and is off by default.

Modes:

- `off` - no automatic file versions
- `all` - track every path except excluded globs
- `paths` - track only included globs, then subtract excluded globs

Policy fields:

- `mode`
- `include_globs`
- `exclude_globs`
- `max_versions_per_file` optional
- `max_age_days` optional
- `max_total_bytes` optional workspace budget
- `large_file_cutoff_bytes` optional guardrail

Implemented retention semantics:

- `max_versions_per_file` trims oldest non-head versions within one lineage.
- `max_age_days` trims non-head versions older than the policy window.
- `max_total_bytes` trims oldest non-head stored-content versions across the
  workspace until the byte budget is met or only lineage heads remain.
- `large_file_cutoff_bytes` keeps recording ordered version metadata for large
  files but omits historical blob bytes once the cutoff is exceeded. Those
  versions remain listable and addressable, but content read/diff/restore
  behaves like an unavailable binary payload.

Recommended default excludes:

- editor swap and temporary files
- common cache/build output directories

### CLI

Follow the existing current-workspace convention: when `[workspace]` is
omitted, use the selected workspace.

Recommended commands:

```bash
afs ws versioning get [workspace]
afs ws versioning set [workspace] --mode off|all|paths \
  [--include 'src/**']... \
  [--exclude '**/*.log']... \
  [--max-versions-per-file 100] \
  [--max-age-days 30]

afs fs history [-w workspace] <path>
afs fs cat [-w workspace] <path> --version <version-id>
afs fs diff [-w workspace] <path> --from <version-id|head> --to <version-id|head>
afs fs restore [-w workspace] <path> --version <version-id>
afs fs undelete [-w workspace] <path> --version <version-id>
```

Notes:

- `history` should show stable `version_id`, display ordinal, timestamp, size,
  op, source, session, and checkpoint annotations when available.
- `history` should support both oldest-first and newest-first output.
- `cat --version` should print file content or a binary notice.
- `restore` should create a new latest version; it must not rewrite history.
- `undelete` should materialize the selected historical content back into the
  working copy and create a new latest version.

### MCP

Expose explicit tools rather than a filesystem-like history path:

- `file_history`
- `file_read_version`
- `file_diff_versions`
- `file_restore_version`
- `workspace_get_versioning_policy`
- `workspace_set_versioning_policy`

This keeps agent behavior predictable. A normal `read_file` still reads the
latest file.

### UI

Add three UI surfaces:

1. Workspace Settings tab:
   - versioning toggle
   - mode picker
   - include/exclude glob editors
   - retention controls
2. File browser:
   - `History` action on files
   - side panel or drawer listing versions for that path
   - actions: view, diff against head, restore, undelete
3. Changes table:
   - when a changelog row has a version link, let users jump directly into the
     version drawer for that path

Do not add a top-level workspace browser view like `version:<id>`. File
versions are file-local, not workspace-wide.

---

## Data Model

### Versioning policy

```text
WorkspaceVersioningPolicy
  mode                 off | all | paths
  include_globs        []string
  exclude_globs        []string
  max_versions_per_file int
  max_age_days         int
  max_total_bytes      int64
  large_file_cutoff_bytes int64
```

### File lineage

Every tracked file needs a stable lineage identifier.

```text
FileLineage
  file_id              string
  workspace_id         string
  current_path         string
  state                live | deleted
  created_at           timestamp
  deleted_at           timestamp optional
```

`file_id` must survive rename. Recreating a deleted path creates a new
`file_id`.

### File version

```text
FileVersion
  version_id           string
  file_id              string
  ordinal              int64
  path                 string
  prev_path            string optional
  op                   put | delete | symlink | chmod | rename | restore
  kind                 file | symlink | tombstone
  blob_id              string optional
  content_hash         string optional
  prev_hash            string optional
  size_bytes           int64
  delta_bytes          int64
  mode                 uint32
  target               string optional
  source               agent_sync | mcp | checkpoint_restore | import | version_restore
  session_id           string optional
  agent_id             string optional
  user                 string optional
  checkpoint_ids       []string optional
  created_at           timestamp
```

`checkpoint_ids` can start as optional or deferred. The important invariant is
that version retrieval does not depend on checkpoints.

`ordinal` is required, not cosmetic. It is a monotonic per-lineage sequence:

- first tracked version in a lineage is ordinal `1`
- every later version increments by `1`
- ordinals are never reused inside a lineage
- deleting a historical version out of the middle is unsupported in normal UX

Users and agents can refer to versions by either stable `version_id` or
lineage-local ordinal, but APIs should always return both.

---

## Redis Key Shape

Exact naming can change, but the layout should look like:

```text
afs:{<workspace>}:workspace:versioning:policy
afs:{<workspace>}:workspace:path_file_ids
afs:{<workspace>}:workspace:path_history:{<normalized-path>}
afs:{<workspace>}:workspace:version_file_ids
afs:{<workspace>}:file:{<file-id>}:meta
afs:{<workspace>}:file:{<file-id>}:versions
afs:{<workspace>}:file:{<file-id>}:version:{<version-id>}
```

Behavior:

- `path_file_ids` maps current live path to `file_id`
- `path_history:{path}` indexes all version ids ever observed at that path,
  across rename and delete boundaries
- `version_file_ids` maps stable `version_id` to owning `file_id`, so exact
  version retrieval does not require scanning every lineage
- `file:{id}:versions` is the canonical per-lineage ordered history
- version records reference blob ids already stored in the existing content
  store when possible

Keep the changelog as an event feed. Do not force it to become the canonical
version retrieval store.

---

## Write Semantics

### Concurrency And Ordering

The versioning layer must be correct under parallel writers.

Assumptions:

- one client may process its own queued writes serially
- multiple clients or sessions may still mutate the same workspace concurrently
- some write paths already detect conflict instead of enforcing a global lock

Required invariants:

- every lineage has one monotonic ordinal stream
- ordinals are allocated atomically per `file_id`
- no two committed versions in the same lineage share an ordinal
- rename, delete, restore, and recreate update lineage state and version history
  together
- version order is derived from `(file_id, ordinal)`, not `created_at`

Implementation requirement:

- the live mutation and the lineage/version metadata update must happen in one
  transactional unit where possible, or under one optimistic compare-and-swap
  loop if the backend path cannot bundle them directly

Recommended approach:

1. Resolve the live lineage for the path.
2. Read the current lineage head ordinal and expected live state hash.
3. Apply the file mutation only if the expected lineage head still matches.
4. Append the next version with `ordinal = head + 1`.
5. Update lineage head pointers and path mappings in the same transaction.
6. Retry or surface conflict if another writer won first.

The spec should assume per-lineage optimistic concurrency, not a workspace-wide
mutex.

### Create

- If a tracked path is created and has no live lineage, allocate a new `file_id`
- Write version `v1` with the current blob or symlink target

### Modify content

- If a tracked file already has a live lineage, append a new version
- Reuse existing blob id when the content hash already exists

### Mode-only change

- Append a new version with `op=chmod`
- Reuse the current blob id

### Symlink target change

- Append a new version with `kind=symlink`
- Store the target in the version row

### Rename

- Keep the same `file_id`
- Append a rename version with `prev_path`
- Update current-path mapping
- Update path history for both old and new paths
- If another writer changes either source or destination lineage first, fail and
  retry through the normal conflict path

### Delete

- Append a tombstone version
- Mark the lineage deleted
- Remove the live path mapping
- Delete must verify it is still deleting the expected live lineage head

### Recreate after delete

- Same path, new lineage
- Do not continue the deleted lineage automatically

### Restore file version

- `afs fs restore` or `file_restore_version` writes the selected historical
  content back to the working copy
- restore appends a new latest version with `source=version_restore`
- restore also emits normal changelog rows for observability
- restore must compare against the current live lineage head and either fail or
  require `--force` if the path moved since the user selected the old version

### Checkpoint create

- Creating a checkpoint does not need to duplicate file-version rows
- Checkpoint creation may annotate the latest matching versions, but file
  version retrieval must work even without that annotation

### Checkpoint restore

- Restoring a workspace checkpoint can create file versions for the paths that
  changed as a result of the restore
- those versions should use `source=checkpoint_restore`

### Import

- Imported tracked files get initial versions so history exists from day one

### Fork

- Forked workspaces inherit file history up to the fork point
- parent and child diverge after the fork

### Parallel writers on one path

If two writers race on the same tracked file:

- winner commits the next ordinal
- loser must not invent a parallel ordinal
- loser retries against the new lineage head or surfaces a conflict

This is especially important for:

- save-loop editor writes
- sync uploader vs MCP edit
- checkpoint restore vs live write
- delete or rename racing with content update

---

## Read And Query Semantics

### History

`afs fs history` should resolve the current or historical lineage for a path,
then return ordered versions newest-first by default.

Cases:

- live tracked file at current path
- deleted tracked file at historical path
- path that has had multiple lineages over time because it was deleted and later
  recreated

When multiple lineages exist for one path, history output should group them so a
user can see the breaks clearly.

Required ordering capabilities:

- newest-first listing for normal browsing
- oldest-first listing for replay and reconstruction
- address one version by ordinal within a lineage
- page through history without losing order

The ordering source of truth is `(file_id, ordinal)`, not timestamp alone.
Timestamps are informative but not sufficient for deterministic replay.

When a path has multiple historical lineages because it was deleted and later
recreated, ordering is:

1. deterministic within one lineage by ordinal
2. grouped by lineage in history output
3. secondarily ordered across lineages by creation or tombstone time

### Content

`afs fs cat --version` and `file_read_version` should fetch the exact stored version
content, not reconstruct it from checkpoints.

Both should accept either:

- `version_id`
- `file_id + ordinal`

### Diff

`afs fs diff` should compare:

- version vs version
- version vs head
- version vs working copy

Text diff can be computed on demand. Binary files should report that a binary
diff is unavailable.

### Undelete

`afs fs undelete` should list the latest tombstoned lineage at that path by
default and restore a selected version into the working copy.

---

## HTTP API

Add dedicated file-version routes instead of overloading `view=`.

### Versioning policy

- `GET /workspaces/{workspace_id}/versioning`
- `PUT /workspaces/{workspace_id}/versioning`

### File history

- `GET /workspaces/{workspace_id}/files/history?path=/src/main.go`
- `GET /workspaces/{workspace_id}/files/version-content?path=/src/main.go&version_id=fv_123`
- `POST /workspaces/{workspace_id}/files:restore-version`
- `POST /workspaces/{workspace_id}/files:undelete`

Recommended history query parameters:

- `path`
- `direction=asc|desc`
- `limit`
- `cursor`
- `lineage=current|all|<file-id>`

Recommended history response shape:

```json
{
  "path": "/src/main.go",
  "lineages": [
    {
      "file_id": "file_123",
      "state": "live",
      "current_path": "/src/main.go",
      "versions": [
        {
          "version_id": "fv_003",
          "ordinal": 3,
          "op": "put",
          "created_at": "2026-04-28T20:14:00Z"
        },
        {
          "version_id": "fv_002",
          "ordinal": 2,
          "op": "rename",
          "created_at": "2026-04-28T20:11:00Z"
        }
      ],
      "next_cursor": "file_123:1"
    }
  ]
}
```

Cursor semantics should preserve deterministic order within one lineage. A
simple shape is `<file_id>:<ordinal>`.

Version-content lookup should also accept:

- `file_id`
- `ordinal`

Recommended restore body:

```json
{
  "path": "/src/main.go",
  "version_id": "fv_123",
  "force": false
}
```

Do not extend `GET /tree` or `GET /files/content` with file-version-specific
virtual views in the first slice. The current `view=` contract is
workspace-scoped and should stay that way.

---

## Integration With Changelog

Changelog and file versioning should cooperate, but they are not the same
thing.

Recommended additions to changelog rows for tracked paths:

- `file_id`
- `version_id`

Benefits:

- the Changes tab can deep-link to exact file versions
- agents can move from "what changed?" to "show me that version" without a
  second path-resolution step

Changelog remains the lightweight event timeline. File-version records remain
the durable retrieval surface.

Changelog row order is useful for observability, but it must not be the sole
source of truth for file-version ordering.

---

## Retention And Cost

Automatic versioning can grow quickly, especially for editor save loops and
large generated files.

Start with:

- feature off by default
- path globs for selective rollout
- blob reuse by hash
- optional per-file, age-based, and workspace-budget retention
- large-file guardrails that preserve ordering while omitting oversized
  historical blobs

Follow-up work can add:

- version coalescing policies
- admin reporting for top history consumers

Normal users should not be able to delete arbitrary middle versions. Retention
must be policy-driven, not ad hoc.

If compliance-driven redaction is required later, add a separate admin-only
redaction flow using stable `version_id` handles.

---

## Testing

Coverage should include:

- sync-mode tracked writes
- mounted writes
- rename across tracked paths
- delete and undelete
- recreate after delete creates a new lineage
- checkpoint restore emits correct file versions
- import seeds initial versions
- fork preserves history to the fork point
- binary and large-file retrieval
- glob policy matching and exclusion
- retention trimming without corrupting live path mappings

---

## Rollout

### Phase 1

- workspace policy storage
- lineage/version Redis model
- write-path hooks for sync, MCP edits, import, and checkpoint restore
- CLI: `ws versioning get/set`, `fs history`, `fs cat --version`

### Phase 2

- CLI: `fs diff`, `fs restore`, `fs undelete`
- HTTP routes
- changelog links to version ids

### Phase 3

- UI settings controls
- file browser history drawer
- changes table deep links

### Phase 4

- retention enforcement
- checkpoint annotation polish
- cost reporting

---

## Open Questions

1. Should checkpoint creation annotate current version ids immediately, or can
   that remain a later optimization?
2. Should restore refuse when the current working-copy file is dirty relative to
   the latest recorded version, or should `--force` be enough?
3. Should AFS track mode-only changes for all files, or only when the path is
   already content-versioned?
4. What should the default `large_file_cutoff_bytes` be, if any?
