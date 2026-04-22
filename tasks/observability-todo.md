# TODO — Per-session file-change log (Phase 1a, backend MVP)

Scope: backend only. UI + CLI in next iteration.

## Key discoveries from code exploration
- Every FS mutation funnels through `SaveCheckpoint`. No separate sync receive path.
- `Audit()` at store.go:617 already writes stream `afs:{storageID}:workspace:audit` — high-level ops only ("save", "restore", "import"). Prior art for XADD patterns.
- Session metadata NOT currently in `SaveCheckpointRequest` — must plumb.
- Parent-manifest already fetched inside `saveCheckpoint` at service.go:1043 for equivalence check — reuse for per-path diff.

## Design decisions
- **New dedicated stream** `ws:{storageID}:changelog` (separate from existing `workspace:audit`) — row-per-path vs row-per-op, different read patterns.
- Companion hashes: `ws:{id}:sess:{sid}:summary`, `ws:{id}:path:{path}:last`.
- Session ID/user/label plumbed via optional fields on `SaveCheckpointRequest` + `RestoreCheckpoint` + `Import*` call sites.
- Changelog write failure logged but does NOT fail the FS op.
- Instrument inside existing `saveCheckpoint` transactional pipe (service.go:1075–1137) for atomicity with manifest write.

## Tasks

### 1. Foundation
- [ ] New file `internal/controlplane/changelog.go`
  - `type ChangeEntry struct` matching plan §0 data model
  - `enqueueChangeEntries(pipe redis.Pipeliner, storageID string, entries []ChangeEntry) error`
  - Key builders: `changelogStreamKey`, `sessionSummaryKey`, `pathLastKey`
  - Default MAXLEN constant (100000)
- [ ] Unit tests for key builders + entry field serialization

### 2. Manifest diff helper
- [ ] `manifestDiff(parent, child *Manifest) []ChangeEntry` in changelog.go
  - Emits one entry per added/modified/deleted/chmod'd path
  - Handles missing parent (import) and missing child (delete)
- [ ] Unit tests: add, delete, modify, chmod-only, no-op

### 3. Plumb session metadata
- [ ] Add optional fields to `SaveCheckpointRequest`: `SessionID`, `User`, `Label`, `AgentVersion`, `Source` (enum: agent_sync | checkpoint_save | server_restore | import)
- [ ] Same for `RestoreCheckpoint` args
- [ ] Same for `importWorkspace` + `importLocal`
- [ ] HTTP handler at http.go:829 parses session ID from `X-AFS-Session-Id` header (ligthtest-touch, no request-body churn)
- [ ] Server looks up session record by ID to get user/label/agent version

### 4. Wire into apply paths
- [ ] `saveCheckpoint` (service.go:1024): call `manifestDiff(parentManifest, input.Manifest)`, append `enqueueChangeEntries` calls to the existing `pipe` before `pipe.Exec()`. Tag entries with checkpoint_id + source.
- [ ] `restoreCheckpoint`: same, with `source=server_restore`.
- [ ] `importWorkspace` + `importLocal`: iterate manifest entries, emit with `source=import`.
- [ ] Log + metric on XADD failure; do NOT error the parent op.

### 5. Read API
- [ ] `GET /v1/databases/{db}/workspaces/{ws}/changes?session_id=&cursor=&limit=&direction=` → `XRANGE` / `XREVRANGE`
- [ ] `GET /v1/databases/{db}/workspaces/{ws}/sessions/{sid}/summary` → `HGETALL`
- [ ] `GET /v1/databases/{db}/workspaces/{ws}/paths/{path}/last` → `HGETALL`
- [ ] Response: `{ entries: [...], next_cursor: "..." }`; cursor = last Redis entry ID

### 6. Session label
- [ ] Add `label` to session record (SQLite catalog column + Redis session JSON)
- [ ] Accept `label` on session create payload
- [ ] `PATCH` endpoint to rename mid-session

### 7. Tests
- [ ] Helper unit tests (§1, §2)
- [ ] Service: SaveCheckpoint with 3 changed paths emits 3 entries + session summary hincr + path:last updates
- [ ] Service: RestoreCheckpoint emits reverse entries
- [ ] Service: Import emits N entries
- [ ] HTTP: GET /changes paginates via cursor
- [ ] Failure: mocked XADD error does NOT fail SaveCheckpoint

### 8. Manual verification
- [ ] Local: start control plane + Redis, `afs up` + edit a file + `afs checkpoint save`
- [ ] `redis-cli XRANGE ws:{id}:changelog - +` shows entry
- [ ] `curl /v1/databases/{db}/workspaces/{ws}/changes` returns same
- [ ] No regressions in existing `go test ./...`

## Out of scope (next iteration)
- UI "Changes" tab
- `afs session log` / `afs session summary` CLI
- Workspace activity page enrichment
- Retention sweeper (MAXLEN ~ covers V1)
- Moving writes into AFS Redis module for atomic single-command apply

## Unresolved questions
1. **Stream naming**: confirm OK to coexist with existing `workspace:audit` — new stream `workspace:changelog`. Different semantics justify separation, but two streams means two writes. OK?
2. **Session ID plumbing**: confirm header (`X-AFS-Session-Id`) over request-body field. Header keeps `SaveCheckpointRequest` schema unchanged.
3. **Source enum**: for sync-mode implicit checkpoints, do we want to distinguish them from explicit user checkpoints? Values I'm proposing: `agent_sync | checkpoint_save | server_restore | import`. Client needs a flag to set this. Easier: single `agent_sync` value, drop `checkpoint_save` distinction.
4. **Session label capture timing**: collect at `afs up --label` only, or also allow `afs session rename`? Rename is trivial to add; V1 could be create-time only.
5. **Path normalization**: paths can be long / contain unicode. Store raw, or normalize? Raw for V1; revisit if queries need it.
