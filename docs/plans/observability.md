# Observability Plan — Agents & Workspaces

Last reviewed: 2026-04-24.
Status: research context; superseded where it conflicts with
`docs/plans/event-history-merge.md`.

Research doc. What's worth observing, for whom, and where the signal lives in
code. The first changelog milestone has partially landed; use
`docs/plans/event-history-merge.md` for the current next step that merges
workspace lifecycle events and file changes into one history stream.

Current implementation note: lifecycle activity and file changes live in Redis
Streams today. There is no SQLite activity table to remove, and the durable
next step is the Redis-stream merge described in `event-history-merge.md`.

---

## 0. First milestone — per-session file-change log

**Ship this before anything else.** It's the highest-value observability signal AFS can offer and it's unique to this system: every other observability concern (HTTP RED, Redis, auth) you can get from any server. Per-agent-session diffs are the thing operators and users can't get anywhere else.

### Goal
For every agent session, capture every file **added / modified / deleted / renamed / chmod'd / symlinked** with enough metadata to reconstruct "what did this agent do?" Surface that in the UI as a session-scoped timeline and in the CLI as `afs log <id>`.

### Data model
**Store the changelog in Redis**, not SQLite. The workspace is the unit of portability — its history must travel with it across forks, imports, and control planes. Redis is already the source of truth for manifests and blobs; the changelog belongs in the same place, reachable identically from cloud and local-dev modes.

Primary structure: **Redis Stream per workspace.**

```
Key:      afs:{workspace_id}:workspace:changelog
Type:     Stream (XADD / XRANGE / XREVRANGE / XTRIM)
Entry ID: auto (ms-seq)
Fields:
  session_id      string
  user            string   (optional; authenticated principal when present)
  label           string   (optional; session label)
  op              string   put | delete | mkdir | rmdir | symlink | chmod | rename
  path            string
  prev_path       string   (renames only)
  size_bytes      int      (final size; omitted on delete)
  delta_bytes     int      (signed)
  content_hash    string   (post-op; omitted on delete)
  prev_hash       string   (pre-op; omitted on create)
  mode            int      (final mode bits)
  checkpoint_id   string   (when the op landed as part of a checkpoint save)
  source          string   agent_sync | mount_fuse | server_restore | import
```

Companion keys:
- `afs:{id}:sess:{sid}:summary` — hash. `HINCRBY` on count/bytes per op alongside each `XADD`. Cheap "what did this session do" rollup.
- `afs:{id}:path:last:{path}` — hash with `last_entry_id`, `last_session_id`, `last_hash`. Updated on every write. Powers "who last touched this path" without scanning.

**Queries**
| Need | How |
|---|---|
| Session timeline | `XRANGE afs:{id}:workspace:changelog - +` filtered by `session_id` (V1); small enough for short sessions |
| Workspace recent feed | `XREVRANGE afs:{id}:workspace:changelog + - COUNT N` |
| Path history | deep: scan stream; V1 ship the "last writer" from `path:last` only |
| Session summary totals | `HGETALL afs:{id}:sess:{sid}:summary` |

If session-filter scans get expensive later, add `afs:{id}:sess:{sid}:entries` (list of entry IDs) as a secondary index, or a second stream scoped to the session.

**Retention.** `XADD ... MAXLEN ~ N` per workspace at write time for soft cap; a periodic `XTRIM MINID` by age for hard cap. Per-workspace configurable.

**SQLite role.** Optional projection/cache for the control plane's UI queries, lazily rebuilt from the stream. Not the source of truth, not required for correctness. Skippable entirely in local mode.

### Where the events come from
Control-plane-side emission, not agent-side — the server is the authority on what actually landed. Single code path, no trust issues.

| Hook | Emits |
|---|---|
| [service.go:1030](../../internal/controlplane/service.go) `SaveCheckpoint` manifest diff vs parent | one row per changed path, tagged with `checkpoint_id` |
| sync upload receive path (server receives manifest delta from agent) | one row per op as the server applies it |
| [service.go](../../internal/controlplane/service.go) `RestoreCheckpoint` | rows with `source=server_restore` for paths the restore mutated |
| [service.go](../../internal/controlplane/service.go) `ImportWorkspace` | rows with `source=import` |

Every write that reaches Redis also `XADD`s to the stream. Same code path that
mutates the manifest issues the `XADD` + `HINCRBY` + `path:last` update. Tiny
drift window between the FS mutation and the stream write is acceptable for V1;
if profiling shows the overhead matters, batch the Redis writes in the Go
client path instead of reviving the retired Redis module.

**Diffing:** when an agent submits a manifest delta, we already know old-hash / new-hash per path (the manifest carries it). No extra hashing. For checkpoint save we walk parent→child manifest diff once.

### UI surfaces
1. **Session detail page** — new tab "Changes." Paginated timeline, filter by op, search by path. Header shows totals: `N added · M modified · K deleted · +X MB / -Y MB`.
2. **Workspace activity page** — existing page, enrich each row with actor (session + agent identity) and expose "view session changes" link.
3. **Path history** — click any path, see ordered list of sessions that touched it.
4. **Agent runs list** — for a given user, list their sessions with summary line "touched 47 files, +2.3 MB" so you can scan "what did each agent do today."

### CLI surface
- `afs log [<session-id>]` — default current session, tail-follows while session is live
- `afs log summary <id>` — totals + top-N paths by delta bytes
- `afs ws log --since 1h` — workspace-wide feed

### Exposing to the agent itself
Agents benefit from reading their own change log mid-run (e.g. "what have I done so far in this session?"). Expose via:
- MCP tool `afs_session_changes` returning recent ops
- HTTP `GET /v1/sessions/{id}/changes?since=seq`

### Retention
Stream is trimmed by a combination of `MAXLEN ~` at write time (soft cap, e.g.
100k entries per workspace) and a periodic `XTRIM MINID` by age (hard cap, e.g.
30 days). Session summary hashes (`afs:{id}:sess:{sid}:summary`) persist for the
life of the session + a configurable grace period, then drop. Per-workspace
`path:last` entries persist indefinitely — one hash per live path, cheap. All
thresholds configurable per workspace.

### Non-goals for this milestone
- Content diffs (line-level). Hash + size is enough for V1; content diff is a later layer on top.
- Cross-workspace correlation.
- Real-time websocket push. Poll every N seconds from the UI.

### Cut list (order of work)
1. Define stream key layout and entry schema; helper write path (`XADD` + `HINCRBY` + `path:last`)
2. Emit from the server-side apply path (sync receive + checkpoint save + restore + import)
3. `GET /v1/sessions/{id}/changes` and `GET /v1/workspaces/{id}/changes` endpoints (backed by `XRANGE`/`XREVRANGE`)
4. UI: Session detail → Changes tab
5. CLI: `afs log`
6. Workspace activity enrichment
7. "Last writer per path" view (from `path:last`)
8. Retention: `MAXLEN ~` at write + periodic `XTRIM MINID` sweep

### Restore, checkpoint, import semantics
- **Restore**: append-only — emit new entries with `source=server_restore` for every path the restore mutated. Stream is never rewritten.
- **Checkpoint save**: each modified path emits one entry tagged `checkpoint_id`.
- **Import**: seed the stream with one `source=import` entry per imported file so path history is populated from day one.
- **Fork** (backend-only today): if/when it becomes user-facing, add `parent_workspace_id` + `forked_from_entry_id` to the workspace record and keep child streams independent. Not in scope for V1.

### Write-path sketch
Pseudocode for the control-plane apply helper. Runs in the same path that mutates the manifest.

```go
func applyFileOp(ctx, ws, sess, op FileOp) error {
    // 1. Existing FS mutation.
    prevHash, prevSize := readPathHeader(ws, op.Path)
    if err := fsApply(ws, op); err != nil { return err }

    // 2. Changelog append.
    streamKey := fmt.Sprintf("ws:%s:changelog", ws.ID)
    fields := map[string]any{
        "session_id":    sess.ID,
        "user":          sess.User,
        "label":         sess.Label,
        "op":            op.Kind,
        "path":          op.Path,
        "size_bytes":    op.Size,
        "delta_bytes":   op.Size - prevSize,
        "content_hash":  op.Hash,
        "prev_hash":     prevHash,
        "mode":          op.Mode,
        "source":        op.Source,
        "checkpoint_id": op.CheckpointID,
    }
    if op.PrevPath != "" { fields["prev_path"] = op.PrevPath }

    entryID, err := rdb.XAdd(streamKey, "MAXLEN", "~", maxStreamLen, "*", fields)
    if err != nil {
        metrics.ChangelogDrift.Inc()
        slog.Warn("changelog write failed", "ws", ws.ID, "err", err)
        return nil // observability must not block correctness
    }

    // 3. Session rollup.
    summaryKey := fmt.Sprintf("ws:%s:sess:%s:summary", ws.ID, sess.ID)
    rdb.HIncrBy(summaryKey, "op_"+op.Kind, 1)
    rdb.HIncrBy(summaryKey, "delta_bytes", op.Size - prevSize)

    // 4. Last-writer index.
    rdb.HSet(
        fmt.Sprintf("ws:%s:path:%s:last", ws.ID, op.Path),
        "entry_id", entryID,
        "session_id", sess.ID,
        "hash", op.Hash,
        "occurred_at", timeNow(),
    )
    return nil
}
```

Three Redis writes per FS op (stream append, summary hash, last-writer hash). Acceptable; migrate to a single module command if profiling flags the overhead.

### Session identity (minimal)
Keep existing machine-derived fields (hostname, OS, agent version). Add two things:

- **Label** — optional user-supplied string, set at `afs ws mount --label "<string>"` and editable via `afs log rename <id> "<string>"`. Display field only, not an identity key. Session ID remains the opaque primary key.
- **Authenticated user** — when the request carries a Clerk token, record the user on the session row. In local/CLI-token mode, leave null or record the token owner, whichever is already present.

No agent-kind auto-detect, no run-vs-session split. Revisit if users ask.

---

## 1. Audiences

Two distinct consumers with different questions. We should design the data model once, then surface different cuts.

### A. Operator (you, running the control plane)
Cares about **fleet health, cost, and SLOs across all tenants**. Questions:
- Is anything broken right now? (error rates, stale sessions, Redis health)
- Is the system growing the way I expect? (workspace count, blob storage, unique agents)
- Who is using it hardest? (top workspaces by churn, top users by bytes)
- Are agents behaving? (abandoned sessions, auth failure spikes, manifest rejection rate)
- What's my Redis bill going to look like? (memory growth, blob dedupe ratio)

### B. Agent / end-user (developer running an agent against a workspace)
Cares about **their own session and workspace**. Questions:
- Is my sync keeping up? (upload lag, queued ops, conflicts)
- What has my agent actually done? (files changed, bytes moved, last activity)
- Who else is in this workspace? (other active sessions, last writer per file)
- Is the workspace healthy? (checkpoint cadence, divergence from base)
- Why did that op fail? (conflict, auth, Redis timeout)

---

## 2. Signal taxonomy

Four signal types. Each has a different storage/query pattern.

| Signal | Examples | Backing store |
|---|---|---|
| Counters | files_uploaded_total, auth_failures_total | Prom or SQLite rollup |
| Gauges | active_sessions, redis_memory_used | Prom scrape or polled |
| Histograms | sync_upload_latency, checkpoint_save_bytes | Prom or bucket-in-SQLite |
| Events (activity) | "agent X modified /foo", "session Y went stale" | Redis streams |

Activity events are already partially there — [catalog_health.go](../../internal/controlplane/catalog_health.go) and the `/v1/activity` endpoint. Lean into it.

---

## 3. Operator-facing metrics

### 3.1 Session fleet
- `afs_sessions_active{database,workspace,mode}` gauge — mode = sync|mount
- `afs_sessions_opened_total{database,outcome}` counter — outcome = ok|auth_fail|quota|error
- `afs_sessions_closed_total{reason}` counter — reason = user_down|stale|server_shutdown|error
- `afs_session_duration_seconds` histogram (observed at close)
- `afs_session_heartbeats_total{state_transition}` — active→stale transitions are the signal

Instrument at [service.go:405-593](../../internal/controlplane/service.go) (`CreateWorkspaceSession`, `HeartbeatWorkspaceSession`, `CloseWorkspaceSession`, `reapExpiredWorkspaceSessions`).

### 3.2 File-sync throughput (server view)
- `afs_sync_ops_total{op,outcome}` — op = file_put|file_del|mkdir|symlink|chmod|chunk
- `afs_sync_bytes_total{direction}` — direction = up|down
- `afs_sync_conflicts_total{resolution}` — resolution = local_win|remote_win|abort
- `afs_manifest_rejections_total{reason}` — reason = hash_mismatch|race|schema

Instrument at [sync_uploader.go:91-267](../../cmd/afs/sync_uploader.go) (agent-side emit) and the server-side receive path.

### 3.3 Checkpoints
- `afs_checkpoint_ops_total{op,outcome}` — op = save|restore|fork|delete
- `afs_checkpoint_save_bytes` histogram
- `afs_checkpoint_save_duration_seconds` histogram
- `afs_checkpoint_file_count` histogram (how big are the trees?)

[service.go:1030-1157](../../internal/controlplane/service.go) `SaveCheckpoint`.

### 3.4 HTTP control plane
Standard RED metrics per route. Wrap the handler registered in [http.go](../../internal/controlplane/http.go):
- `afs_http_requests_total{route,method,status}`
- `afs_http_request_duration_seconds{route}`
- `afs_http_requests_in_flight`

### 3.5 Redis / data plane
Already collected by [redis_stats.go](../../internal/controlplane/redis_stats.go). Expose as gauges:
- `afs_redis_memory_bytes{database}`
- `afs_redis_memory_fragmentation_ratio{database}`
- `afs_redis_ops_per_sec{database}`
- `afs_redis_cache_hit_ratio{database}`
- `afs_redis_connected_clients{database}`
- `afs_redis_dbsize{database}`

Add: `afs_redis_probe_duration_seconds{database}` — our own latency to Redis, separate from Redis's internal numbers. Early-warning for network issues.

### 3.6 Auth
- `afs_auth_attempts_total{kind,outcome}` — kind = cli|mcp|clerk; outcome = ok|expired|invalid|revoked
- `afs_auth_tokens_active{kind}` gauge
- `afs_auth_token_exchanges_total`

[auth.go](../../internal/controlplane/auth.go), esp. `verifyClerkSessionToken`, `MountCLITokenAuthenticator`.

### 3.7 Storage cost (derived)
- `afs_workspace_blob_bytes{workspace}` — sum of unique blob sizes
- `afs_workspace_manifest_bytes{workspace}`
- `afs_workspace_dedupe_ratio{workspace}` — logical / physical
- `afs_checkpoints_per_workspace` gauge
- `afs_orphan_blob_bytes` gauge — GC candidates

Not cheap to compute live. Scheduled rollup job, emit on interval.

---

## 4. Agent-facing metrics

Exposed via CLI (`afs status --verbose`), MCP, and UI. Same underlying data, scoped to one workspace/session.

### 4.1 My session
- Uptime, mode, local path, last heartbeat ack
- Queued upload ops / download ops
- Upload lag (time from local mtime → server ack), p50/p99
- Recent errors (last N with reason)

### 4.2 My workspace activity (live feed)
Already have the activity event model. Add structured fields per event:
- actor (session_id, agent_version, hostname, user)
- op (put|del|mkdir|symlink|chmod|checkpoint_save|checkpoint_restore|fork)
- path (for file ops)
- bytes, duration
- outcome

Serves three UIs: live tail in CLI, workspace activity panel in UI, audit log.

### 4.3 Co-tenancy
- Other active sessions in my workspace (who is editing concurrently)
- Last writer per path (useful before overwriting)
- Recent conflicts and how they were resolved

Pulled from session catalog + manifest history. No new storage needed — query differently.

### 4.4 Per-file "exhaust" (agent activity summary)
For the end user: "in this session my agent touched N files, added X KB, deleted Y files." Summarize the uploader result stream at [sync_uploader.go](../../cmd/afs/sync_uploader.go).

---

## 5. Cardinality & cost

Labels worth keeping:
- `database` (dozens, OK)
- `workspace` (hundreds–thousands — risky on Prom, fine in SQLite)
- `op`, `outcome`, `reason` (low cardinality)
- `agent_version`, `os` (low-medium)

Labels to avoid on hot metrics:
- `session_id` (unbounded)
- `path` (unbounded)
- `user_id` (medium, keep only on auth metrics)

Rule: per-workspace metrics live in Redis-derived rollups or optional catalog
projections, not Prom labels. Fleet-wide aggregates go to Prom.

---

## 6. Where to put it

Three transport options, use all three:

1. **Structured logs (slog)** — every event emits a log line with fields. Cheap, searchable, no infra. Start here.
2. **Catalog rollup projections** — optional derived tables can cache
   workspace/session aggregates for UI speed, but Redis Streams remain the
   source of truth.
3. **Prometheus `/metrics` endpoint** — operator-facing, Grafana-ready. Add when (2) stops scaling.

Phasing:
- **Phase 1a** (first): per-session file-change log (see §0). Schema + server-side emission + UI tab + CLI.
- **Phase 1b**: slog everywhere + extend activity log schema for non-file events (sessions, auth, checkpoints). Zero new infra.
- **Phase 2**: derived rollups + UI dashboards for workspace/session detail.
- **Phase 3**: Prometheus endpoint + public dashboard for operators.

---

## 7. Concrete instrumentation hit list

Minimum set for Phase 1:

| File | What to emit |
|---|---|
| [service.go:405](../../internal/controlplane/service.go) | session_created |
| [service.go:537](../../internal/controlplane/service.go) | session_heartbeat (sampled) |
| [service.go:561](../../internal/controlplane/service.go) | session_closed |
| [service.go:689](../../internal/controlplane/service.go) | session_stale |
| [service.go:1030](../../internal/controlplane/service.go) | checkpoint_saved |
| [http.go](../../internal/controlplane/http.go) middleware | http_request |
| [auth.go](../../internal/controlplane/auth.go) | auth_attempt |
| [sync_uploader.go:91](../../cmd/afs/sync_uploader.go) | upload_op |
| [sync_downloader.go:100](../../cmd/afs/sync_downloader.go) | download_op |
| [sync_reconcile.go:501](../../cmd/afs/sync_reconcile.go) | reconcile_cycle |
| [redis_stats.go:35](../../internal/controlplane/redis_stats.go) | redis_poll (add probe latency) |

---

## 8. Dashboards (what would we want to see)

### Operator dashboard
- Fleet: active sessions, workspaces, databases (now + 24h trend)
- Error rate: HTTP 5xx %, auth failures/min, stale-session rate
- Throughput: upload MB/s, download MB/s, ops/sec
- Redis: memory %, latency p99, clients per DB
- Top-N: noisiest workspaces, biggest workspaces, most-active users
- Storage: blob bytes trend, checkpoint count trend, dedupe ratio

### Workspace dashboard (agent-facing)
- Active sessions table with agent/host/uptime
- Activity timeline (file ops, checkpoints)
- File-churn heatmap (which paths change most)
- Conflict log
- Checkpoint timeline

### Session dashboard (agent-facing)
- Live stats: queued ops, upload lag, bytes moved
- Error tail
- "What my agent did" summary (files, bytes, checkpoints)

---

## 9. Non-goals (for now)

- Distributed tracing (OTEL) — overkill until we have multiple services.
- Per-path cardinality in Prom — use derived rollups.
- Real-time streaming to UI — polling is fine at this scale.
- Alerting rules — design after metrics exist.

---

## Unresolved questions

1. Do we want multi-tenant separation from day one (labels for `tenant_id`/`org_id`)? If the Vercel deployment is shared, this matters now, not later.
2. Retention policy for activity events? Currently unbounded in SQLite — will grow forever.
3. Should agent-side metrics ship to the control plane, or stay local? Shipping gives operators visibility into client behavior but adds a write path.
4. Is there appetite for a dependency on Prometheus client lib, or should we stay stdlib-only and expose JSON at `/v1/metrics`?
5. PII in activity events — paths can reveal secrets (e.g. `.env.production`). Hash, truncate, or store raw?
6. Do we want a synthetic canary agent continuously exercising the system, producing its own signal stream?
