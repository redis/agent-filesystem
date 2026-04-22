# Event History Merge

Merge `workspace:audit` (lifecycle) + `workspace:changelog` (file ops) → single unified stream. One source of truth for "what happened to this workspace," one UI surface, one CLI command.

---

## Scope correction

Original framing said "drop SQLite activity table + Postgres." No such table exists — lifecycle events already live in Redis Stream `afs:{ws}:workspace:audit`, surfaced by `ListAudit` → `activityFromAudit` → /activity endpoints. SQLite/Postgres catalog holds workspace metadata, sessions, tokens, access tokens — untouched by this plan.

**Actual merge**: two Redis Streams → one. No SQL schema changes.

---

## Today

Two parallel streams. Duplicate effort, drift risk, two UI tabs.

| Stream | Key | Writers | Emits |
|---|---|---|---|
| Audit | `afs:{ws}:workspace:audit` | `Store.Audit()` at high-level ops | workspace_create, import, save, checkpoint_restore, workspace_fork, session_start/close/stale, run_start/exit |
| Changelog | `afs:{ws}:workspace:changelog` | `enqueueChangeEntries` / `writeChangeEntries` | per-path put/delete/mkdir/rmdir/symlink/chmod + source tag |

Readers today:
- `GET /v1/…/activity` → audit stream, via `activityFromAudit` text synthesis.
- `GET /v1/…/changes` → changelog stream.
- UI tabs: `Events` (activity), `File Changes` (changelog). Each own query.
- CLI `afs session log` → changelog.

Overlap: every `save` in audit = N `put` rows in changelog with matching `checkpoint_id`. Every `session_start` has no counterpart in changelog but is the anchor for a block of agent_sync rows.

---

## Target

Single stream: `afs:{ws}:workspace:events`. One row per event, wide schema, op discriminates.

### Entry schema

```
id             auto (ms-seq)
ts_ms          int
kind           workspace | session | checkpoint | process | file
op             create | import | fork | delete             (workspace)
               start | close | stale | heartbeat           (session)
               save | restore                              (checkpoint)
               start | exit                                (process)
               put | delete | mkdir | rmdir | symlink | chmod | rename (file)
source         agent_sync | checkpoint | server_restore | import | server
actor          "afs" | user subject | session id
session_id     string
user           string
label          string
agent_version  string
hostname       string

# file-op-specific
path           string
prev_path      string   (renames)
size_bytes     int
delta_bytes    int
content_hash   string
prev_hash      string
mode           int
checkpoint_id  string

# lifecycle-specific (extras folded into a single JSON blob to avoid
# field sprawl — schema stays flat for file ops, bag for the rest)
extras         json     (client_kind, argv, exit_code, source_workspace, etc.)
```

Empty fields elided at write time, same as today's changelog.

### Companion hashes (unchanged shape)

- `afs:{ws}:sess:{sid}:summary` — per-session op-count + bytes rollup. Already exists.
- `afs:{ws}:path:last:{path}` — last-writer pointer. Already exists; only file ops update it.

### Retention

One policy, one place. `MAXLEN ~ 200000` per workspace at write time + periodic `XTRIM MINID` sweep (e.g. 30 days). Budget absorbs both streams' current volume (lifecycle ~dozens, file ops 10³–10⁵).

---

## Migration

Three phases, ship between.

### Phase A — dual-write

1. `Store.Audit()` → additionally XADD to `workspace:events` with `kind=workspace|session|checkpoint|process` and `extras` populated.
2. `enqueueChangeEntries` → additionally XADD to `workspace:events` with `kind=file`.
3. Both legacy streams keep writing. Readers unchanged.

Ships to prod. Validate entry shape in `redis-cli` for a few days. Zero user impact.

### Phase B — switch readers

1. Replace `ListAudit` + `ListChangelog` with `ListEvents(kind=…, session_id=…, since=…, limit=…)` against the unified stream. Old funcs become thin wrappers for back-compat.
2. `activityFromAudit` retargets the events stream (filtering `kind != file` by default).
3. New endpoint `GET /v1/…/events` (canonical). Keep `/activity` + `/changes` returning the same data filtered until UI migrates.
4. UI: collapse `Events` + `File Changes` tabs into one `History` tab with a filter pill set (lifecycle on, file ops off by default). CLI `afs session log` keeps its current filter (file ops only).

Ships. Users still see two tabs if we delay the UI collapse, but the backend is unified.

### Phase C — drop legacy

1. Stop writing `workspace:audit` + `workspace:changelog`.
2. `XRENAME` / one-time migration: for workspaces with non-empty legacy streams, `XRANGE` read + re-emit into `events` if gap detected. Alternative: delete legacy streams after N days once no readers remain.
3. Remove `workspace:audit` / `workspace:changelog` key helpers + reader methods.
4. Remove `/activity` + `/changes` HTTP routes (or keep as permanent aliases).

---

## UI shape (post-merge)

Single **History** tab. Filter pill row:
- `Lifecycle` (workspace, session, checkpoint, process) — default on.
- `File changes` — default off.
- `Source`: agent_sync / checkpoint / restore / import.
- Session-scoped deep link: `?session=…` preselects a session and turns on all pills.

Checkpoint-save rows expand to reveal the `put`/`delete` rows carrying the same `checkpoint_id`. Session rows expand to the span of `file` rows with matching `session_id`.

CLI:
- `afs events [--since 1h] [--session <id>] [--kind file,checkpoint]` → new.
- `afs session log` → unchanged behavior, now backed by `ListEvents(kind=file)`.

---

## Cut list

1. Define schema + key (`internal/controlplane/events.go`); shared `EventEntry` struct.
2. Dual-write: extend `Store.Audit()` + `enqueueChangeEntries` to XADD into `events` with mapped kind/op.
3. `ListEvents()` reader + kind/session/path filters (in-memory post-filter initially; upgrade to `XRANGE` + tag streams if scans get slow).
4. `GET /v1/…/events` endpoint.
5. UI History tab. Collapse logic for checkpoint-expand.
6. CLI `afs events`.
7. Retarget old endpoints to `events` (aliases).
8. Telemetry: log volumes for both streams side-by-side for one week.
9. Stop writing legacy streams.
10. Backfill + drop old keys.

---

## Non-goals

- Cross-workspace correlation.
- Search/grep over event bodies.
- Changing SQLite/Postgres catalog (workspaces, sessions, tokens) — out of scope.
- Websocket push — polling stays.

---

## Unresolved questions

1. **Include `agent_sync` file ops in default view or hide?** Risk: agent sessions swamp the feed. Bet: hide by default, expand-on-demand under session anchor rows.
2. **`extras` as JSON blob vs flat fields for lifecycle?** JSON keeps schema clean but hurts `XRANGE` filter-by-field. Flat fields balloon the schema. Bet: JSON; filtering by extras is rare.
3. **Backfill legacy streams into the new one during Phase C, or declare a cutover epoch?** Backfill is cleaner for history continuity; cutover is simpler. Bet: cutover + keep old streams read-only for a deprecation window.
4. **Retain `activityFromAudit` title/detail synthesis or move to UI?** Synthesis in the server keeps clients dumb; in the UI keeps the stream pure. Bet: move to UI — server ships raw fields.
5. **Path-history query at Phase B or later?** Plan §0 already promised "who last touched X" via `path:last`; real path history needs a per-path index. Bet: defer to its own iteration.
