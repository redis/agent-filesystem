# AFS NFS perf — remaining improvements to evaluate

This is the actionable follow-up list after the two-pass perf sweep documented
in `tasks/perf-nfs-analysis.md`. Everything below is a candidate improvement,
not yet committed. Each item has an estimated win, a risk/scope call, and a
"decide before you code" section so we don't start implementing something
whose value we have not yet justified.

> **Status update (post Pass 3):** candidates **1** and **2** (plus the
> candidate **7** "free rider") have **landed**. See "Part 3" of
> `tasks/perf-nfs-analysis.md` for the trajectory and benchmark results.
> Remaining live items: candidates **3**, **4**, **5**, **6**.

## Current state (after pass 2)

AFS warm-median, ~/.claude on remote Redis:

| op | current | local | remaining gap |
|---|---:|---:|---:|
| append_jsonl (100 appends, 47 ms/op) | 4,661 ms | 7 ms | 666× |
| write_overwrite (50 ops, 78 ms/op) | 3,895 ms | 6 ms | 649× |
| write_new_small (50 creates, 521 ms/op) | 26,046 ms | 10 ms | 2,570× |

Read-side ops are already within 5–10 % of local disk and are not in scope
here. The three write ops above are the candidates everything below targets.

Effective Redis RTT observed in this environment: ~15–25 ms per round trip
(cloud Redis, TLS on, ~30 % run-to-run variance).

## Decision framework

Before implementing any item: answer these three questions.

1. **Does Claude Code actually hit this path often?** The jsonl append on an
   existing file is hot (every turn). File creation is warm (new sessions,
   new plans, etc.). Large-file content writes are rare. Rank wins by hot-path
   impact, not by raw speedup factor.
2. **What does the wall-clock feel like after the fix?** Not "how many RTTs"
   — actual milliseconds the user perceives. 47 ms → 20 ms per append is
   imperceptible; 500 ms → 80 ms per create is not.
3. **Is there a lower-risk option that gets us 80 % of the win?** Prefer
   those. The team's CLAUDE.md is explicit about simplicity-first.

---

## Candidate 1 — replace `createFileIfMissing` WATCH/MULTI with HSETNX claim — **LANDED**

The implementation lives at `mount/internal/client/native_helpers.go:536`
(`createFileIfMissing`). The lost-race path also compensates the
optimistic `files` and `total_data_bytes` counter bumps in a single
cleanup pipeline. Race tests:
`TestCreateFileRaceLoserCleansUpOrphan`,
`TestCreateFileRaceLoserCompensatesContentBytes`,
`TestCreateFileRaceExclusiveLoserReturnsError`,
`TestCreateFileOverExistingDirectoryPreservesError`. Command-count
guard: `TestCreateFileCommandCountIsBounded` (8 cmds against a warm
root cache, down from 10-12 pre-fix). Wall-clock micro-bench:
`BenchmarkCreateFile`.

The original design notes are kept below for archival.

**Where:** `mount/internal/client/native_helpers.go` — `createFileIfMissing`
currently does:

1. `c.retryWatch(ctx, []string{direntsKey}, fn)` — WATCH the parent's
   dirents set (1 RTT)
2. Inside the tx, `tx.HGet(ctx, direntsKey, name)` — check if the name is
   taken (1 RTT)
3. `c.allocInodeID(ctx)` via the main rdb (1 RTT — Incr on `nextInode`)
4. `tx.TxPipelined(... HSet inode + HSet dirents + touch + createInfo ...)` —
   atomic commit (1 RTT, MULTI+EXEC)

Total: **4 RTTs** in the hot path, before any post-work (markRootDirty,
attrs.Apply, etc.).

**Proposed change:** drop the WATCH dance entirely. Use `HSETNX` on the
dirents set to atomically claim the name, then pipeline the rest:

```go
id, err := c.allocInodeID(ctx)            // 1 RTT (Incr)
if err != nil { return err }

// Atomic claim; returns 0 if the name already exists.
pipe := c.rdb.Pipeline()
claim := pipe.HSetNX(ctx, direntsKey, name, id)
pipe.HSet(ctx, c.keys.inode(id), c.inodeFieldsAtPath(inode, p, true))
c.queueTouchTimes(pipe, parentInode.ID, now)
c.queueCreateInfo(pipe, inode)
if _, err := pipe.Exec(ctx); err != nil { return err } // 1 RTT (pipeline)

if !claim.Val() {
    // Lost the race — someone else created the same path. Delete the
    // orphan inode we just wrote and fall back to the existing-file path.
    c.rdb.Del(ctx, c.keys.inode(id))     // 1 RTT on contended path only
    ...existing handling...
}
```

**Estimated win:** 4 RTTs → 2 RTTs on the common (uncontended) path.
At 20 ms/RTT, ~40 ms per create saved. `write_new_small` currently shows
521 ms per create → roughly **~480 ms per create** after this fix.

**Risks:**
- **Orphan inode on lost-race:** we write the inode key *before* we know the
  claim succeeded. If the claim loses, we have a dangling `inode:{id}` entry.
  Mitigation: `DEL` it on the loss path (costs 1 RTT, but only on contention,
  which in practice is near-zero for the AFS workload — a single user session
  is the only writer).
- **ID gap on lost-race:** the `nextInode` counter advances even for lost
  claims. That is fine; inode IDs do not need to be contiguous.
- **Test coverage:** `TestCreateFileExclusive` exercises the O_EXCL path;
  make sure it still passes. The existing retryWatch retry loop exists
  because two concurrent clients could both see `HGET dirents name = nil`
  and both try to HSet. HSETNX is the exact primitive that collapses that
  check-and-set, so it is strictly safer.

**Decide before coding:**
- Is there a production scenario with *multiple* AFS NFS servers against the
  same Redis workspace? If yes, HSETNX still wins (it is atomic), but we need
  to think about the dirents vs inode write ordering carefully under crashes.
  For the single-client Claude Code case this does not matter.

**Scope estimate:** ~40 lines changed, plus a regression test that replays a
simulated "two creators hit the same name" race. Low.

---

## Candidate 2 — batched `SetAttrs` client method — **LANDED**

The implementation lives at:
- `mount/internal/client/client.go` (`AttrUpdate` struct + `SetAttrs`
  on the `Client` interface).
- `mount/internal/client/native_core.go` (`nativeClient.SetAttrs`,
  partial-field HSet — does not reuse `saveInodeMeta` so the wire
  payload stays at 1-5 fields).
- `mount/internal/nfsfs/fs.go` (`FS.SetAttrs` wrapper handling readOnly
  + AppleDouble shadow + os/client type conversion).
- `third_party/go-nfs/file.go` (`BatchSetAttrer` optional interface +
  `SetFileAttributes.Apply` rewrite that builds the diff once and
  dispatches via type assertion). The legacy fallback path is
  preserved for memfs and any non-AFS billy.Change consumer.

The "candidate 7" no-op skip is folded in: when the computed diff is
empty, `Apply` returns nil without touching Redis. End-to-end tests
exercising both paths:
`TestSetFileAttributesApplyDispatchesToBatchSetAttrer`,
`TestSetFileAttributesApplyEmptyDiffIsFreeRider`. Plus the per-layer
suite (`TestSetAttrsPartialFieldsOnly`, `TestSetAttrsParityWith*`,
`TestFSSetAttrs*`, etc.).

Measured impact (warm cache, local Redis, see `perf-nfs-analysis.md`
Part 3): **3.0× fewer Redis commands** (2 vs 6) and **3.3× faster
wall clock** (37 µs vs 121 µs) than the legacy three-method sequence.

The original design notes are kept below for archival.

**Where:** `third_party/go-nfs/nfs_oncreate.go` + `third_party/go-nfs/file.go`
(`SetFileAttributes.Apply`) calls `changer.Chmod`, then `changer.Lchown`,
then `changer.Chtimes` sequentially whenever the NFS CREATE RPC carries mode
/ uid / gid / atime / mtime (which macOS usually does).

Each of those goes through the client layer as a separate
`resolvePath → saveInodeMeta → markRootDirty` chain. After pass 2 the cache
is warm and `markRootDirty` is throttled, so each call is roughly **1 RTT
(HSet metadata)** on the hot path. Three sequential Chmod/Chown/Chtimes =
**3 RTTs**.

**Proposed change:** add a batched setter on the client interface:

```go
type AttrUpdate struct {
    Mode    *uint32   // pointer means "not set"
    UID     *uint32
    GID     *uint32
    AtimeMs *int64
    MtimeMs *int64
}

func (c Client) SetAttrs(ctx context.Context, path string, upd AttrUpdate) error
```

Implementation: one `resolvePath` (cache hit), one HSet with however many
fields actually changed, one (throttled) markRootDirty. Then modify the
vendored `SetFileAttributes.Apply` to build an `AttrUpdate` once and make
**one** client call.

**Estimated win:** 3 RTTs → 1 RTT per CREATE (and per SETATTR RPC). At
20 ms/RTT that is ~40 ms saved per create. Combined with candidate 1,
`write_new_small` per-op goes from 521 ms → **~440 ms**. Not huge on its own
but composable with everything else.

**Risks:**
- We have to touch the vendored `third_party/go-nfs` fork. That fork already
  contains AFS-specific changes, so this is in-scope, but the more we touch
  it the harder the upstream-rebase story gets.
- `SetFileAttributes.Apply` also has a `SetSize` path that reopens the file
  with O_WRONLY|O_EXCL and calls Truncate. That must stay separate — size
  changes go through TruncateInodeAtPath (which we already fixed in pass 1).

**Decide before coding:**
- Is the marginal win worth touching the fork? Probably yes, because this
  is the only easy way to shave another round-trip off every SETATTR RPC,
  which macOS issues on almost every file mutation.

**Scope estimate:** ~80 lines in the client, ~30 lines in the go-nfs fork.
Low-to-medium.

---

## Candidate 3 — Lua EVALSHA for `WriteInodeAtPath`

**Where:** `mount/internal/client/native_range.go` — `WriteInodeAtPath` is
currently 2 RTTs: one HMGET to load metadata + content, one pipeline to
write them back. That's the per-append floor as long as we continue to
read-modify-write on the client.

**Proposed change:** move the read-modify-write to the Redis server via a
Lua script loaded once at client startup and invoked with EVALSHA:

```lua
-- KEYS[1] = inode hash key
-- KEYS[2] = info hash key
-- KEYS[3] = root dirty key
-- ARGV[1] = offset
-- ARGV[2] = payload
-- ARGV[3] = mtime_ms
-- ARGV[4] = path (optional, for inodeFieldsAtPath path field)
local content = redis.call('HGET', KEYS[1], 'content') or ''
local off = tonumber(ARGV[1])
local payload = ARGV[2]
-- splice/grow and write back
...
redis.call('HSET', KEYS[1], 'content', new_content, 'size', new_size,
           'mtime_ms', ARGV[3], 'atime_ms', ARGV[3])
redis.call('HINCRBY', KEYS[2], 'total_data_bytes', delta)
redis.call('SET', KEYS[3], '1')
return new_size
```

Go side: load script with `SCRIPT LOAD` at init, cache the SHA, call with
`EVALSHA`, handle NOSCRIPT fallback by re-loading.

**Estimated win:** 2 RTTs → 1 RTT per write. At 20 ms/RTT, ~20 ms saved per
append. More importantly — and this is the real motivation — the full file
content stays on the Redis server. For a 400 KB session jsonl, that's ~800 KB
of bytes *not* transferred per append. Today that is a secondary cost (the
RTT dominates for small files on this backend), but for multi-MB session
files it becomes the primary cost.

**Risks:**
- Lua script memory: Redis default Lua memory limit is small. Need to verify
  it handles our typical session file sizes comfortably.
- NOSCRIPT handling: the script can be evicted from Redis's script cache
  (rare, but possible). Client must catch `NOSCRIPT` and re-`SCRIPT LOAD`.
- Correctness of byte-slicing in Lua: edge cases around `off > len(content)`
  (holes are zero-filled in POSIX) and 1-based Lua indexing.
- Schema assumption: this still stores content as a hash field, so it does
  not compose with candidate 4 (separate string key). Do candidate 3 only if
  candidate 4 is not going to happen.

**Decide before coding:**
- **Is there measured demand?** Claude Code session files in `~/.claude/
  projects/**/*.jsonl` are typically 10–500 KB. At those sizes the RTT
  dominates, not the content transfer, and the existing 47 ms/append is
  fine. Measure the actual distribution of session jsonl sizes before doing
  this. If most appends are under 200 KB, skip and move on.

**Scope estimate:** ~60 lines of Lua + Go. Medium risk, high payoff **only**
for large files.

---

## Candidate 4 — split file content into a separate Redis string key

**Where:** the entire client layer. Schema change with migration.

**Current layout:** `afs:{fs}:inode:{id}` is a hash with fields `type, mode,
uid, gid, size, ctime_ms, mtime_ms, atime_ms, target, parent, name, path,
content`. Content is stored inside the same hash.

**Proposed layout:** move `content` to its own Redis string key
`afs:{fs}:content:{id}`. The inode hash becomes metadata-only.

This unlocks Redis's native byte-level primitives:

- `APPEND content:{id} payload` — O(1) append, no read, no full-content
  transfer. Perfect for jsonl appends.
- `SETRANGE content:{id} offset payload` — O(1) random write, only the delta
  is transferred.
- `GETRANGE content:{id} off len` — partial reads.
- Metadata operations (stat, chmod, chown, chtimes) no longer touch content
  at all and become faster even on the read side.

**Estimated win:** this is the "right answer" for the hot path. For append:
~2 RTTs → 1 RTT, and the content does not move across the wire at all.
Large-file append goes from ~100 ms + 2×filesize transfer to ~25 ms + 0
transfer. For small files the win is modest (~20 ms); for MB-scale files
it's 10–50×.

Also makes the following cheaper:
- Truncate → `SET content:{id}` with the truncated buffer, or `DEL` for
  truncate-to-zero.
- Random writes (unusual for Claude Code but common for other tools).
- Partial reads (first-N-lines, tail, grep with early-out).

**Risks:**
- **Schema migration.** Existing workspaces have content embedded in the
  inode hash. Need an online migration step that reads `HGET inode:{id}
  content` and writes it to the new key during first access, then deletes
  the hash field. Or a mandatory offline migration via the control plane.
- **Atomicity.** Metadata updates and content updates used to be one HSet.
  Now they are two separate keys. A crash between them can leave size out
  of sync with actual content length. Mitigation: pipeline them together
  (not atomic, but crash-window is small) or use MULTI/EXEC (adds RTTs).
  Or: make `size` derivable from `STRLEN content:{id}` on read — then you
  can skip the explicit size write and there is only one source of truth.
- **Touch points.** Every method that reads or writes content has to change:
  `loadContentByID`, `saveInodeDirect`, `saveInodeAtPath`,
  `createFileIfMissing`, `WriteInodeAtPath`, `TruncateInodeAtPath`, `Cat`,
  `ReadInodeAt`, `Head`, `Tail`, `Lines`, `Wc`, `Insert`, `Replace`,
  `DeleteLines`, `Cp`, the warming path, and all the parsers. High blast
  radius.
- **Memory / serialization in Redis:** a large string uses Redis's native
  string encoding, which is more memory-efficient than a hash field of the
  same size. Actually a win on the Redis side.

**Decide before coding:**
- This is a big project. Only do it if candidate 3 + the existing fixes are
  *not enough* for a real large-file workload that matters. Right now that
  workload does not exist.
- If you do it, do it as its own plan-mode task with a migration design doc.

**Scope estimate:** 300–600 lines across the client layer, plus a migration.
Multi-session.

---

## Candidate 5 — collapse the NFS WRITE RPC's three stats

**Where:** `third_party/go-nfs/nfs_onwrite.go` does:

1. Pre-op `fs.Stat(fullPath)` for wcc.
2. `fs.OpenFile(..., O_RDWR, info.Mode().Perm())` which internally calls
   `client.Stat` *again* inside `nfsfs/fs.go`.
3. `fs.Write(...)` (the actual work).
4. Post-op `tryStat(fs, path)` for wcc.

After pass 1 all three stats are cache hits, so they do not cost Redis round
trips. But they still burn Go allocations and path walks: `resolvePath`
walks the path component by component, cloning the inode, etc. For a deep
path like `/projects/-Users-.../subagents/foo.jsonl` that is 5–6 path-cache
lookups per stat × 3 stats = ~15–20 map lookups per WRITE RPC.

**Proposed change:** pass the pre-op stat result through to OpenFile so it
does not re-stat; optionally cache the pre-op result on the billy file
handle so the post-op stat can read it from there.

Requires:
- A new method on the nfsfs `FS` like `OpenFileWithInfo(path, flag, perm,
  preStat)` or a thread-local stash of the last stat result.
- Changes in the go-nfs fork to pass the pre-op stat through.

**Estimated win:** zero RTTs (already cache hits) — CPU only. Maybe 1–2 ms
per WRITE on a deep path. **Probably not worth doing** unless a CPU profile
shows path-cache lookups as hot.

**Decide before coding:**
- Take a CPU profile of the NFS server under a Claude Code workload. If
  `resolvePath` / `cache.Get` are hot, do this. Otherwise skip.

**Scope estimate:** ~40 lines in the fork + ~20 lines in the client. Low
blast radius but the gain is marginal.

---

## Candidate 6 — fs_usage trace for regression detection

**Where:** `scripts/capture_claude_fs_patterns.sh` exists but needs a manual
`sudo` run (fs_usage requires root on macOS).

**Why it is still on the list:** we have no automated way to detect a
regression where a new change causes macOS to start issuing more RPCs. The
pass-2 discovery (AppleDouble dominating the cost) was a surprise precisely
because we were flying blind on actual NFS traffic. `AFS_NFS_DEBUG=1`
+ `AFS_NFS_OPSTATS=1` solved that reactively, but a pre-commit regression
harness would catch it proactively.

**Proposed work:**
1. Run `sudo scripts/capture_claude_fs_patterns.sh` once during a canned
   Claude Code session and commit the resulting trace as a reference.
2. Write a post-processor that:
   - Parses the fs_usage output.
   - Computes an op-mix histogram (CREATE / WRITE / GETATTR / LOOKUP / etc.).
   - Computes a "bytes transferred to Redis" estimate based on the hook
     counter delta logged by `AFS_NFS_OPSTATS=1`.
3. Add to CI / pre-commit: re-run the capture, compare histograms, fail
   the check if any bucket drifts by more than some threshold (e.g. 25 %).

**Estimated win:** zero runtime improvement. Pure regression insurance.
Value is "we catch the next AppleDouble-sized surprise in minutes instead
of weeks".

**Decide before coding:**
- Do we actually want this in CI, or is it enough to keep the scripts in
  the repo for ad-hoc use? Full CI integration is a separate multi-hour
  project; a committed reference trace + a README pointer is a 30-minute
  deliverable.

**Scope estimate:** 30 min for reference-trace-plus-README, 4–6 hours for
full CI integration.

---

## Candidate 7 — cut the Chmod/Chown/Utimens redundancy from the ATTR hot path — **LANDED**

Folded into candidate 2. The diff computed in `SetFileAttributes.Apply`
is the empty-diff check; when nothing changed the entire dispatch is
skipped (zero Redis commands), exercised by
`TestSetFileAttributesApplyEmptyDiffIsFreeRider`.

The original design notes are kept below for archival.

**Where:** even after candidate 2 (batched `SetAttrs`), the NFS server
receives a SETATTR RPC whose fields often duplicate what the file already
has. Example: macOS sends SETATTR with `mode=0o644` right after `fs.Create`
created the file with `mode=0o666`, so Chmod has real work. But for every
subsequent SETATTR in the same session, macOS is often sending back the
values we already stored.

The current code in `SetFileAttributes.Apply` does check for no-ops:
```go
if mode != curr.Mode().Perm() { Chmod(...) }
```

But the check happens *after* loading `curr` via `fs.Lstat`, which is
already a free cache hit in our case. The issue is: if any one of mode / uid
/ gid / atime / mtime differs, we still do N separate client calls.

**Proposed change:** after batching with candidate 2, also skip the RPC
entirely if the computed `AttrUpdate` has *no* actually-changed fields.

```go
var upd AttrUpdate
if s.SetMode != nil && os.FileMode(*s.SetMode)&os.ModePerm != curr.Mode().Perm() {
    upd.Mode = s.SetMode
}
// ... same for uid/gid/atime/mtime
if upd.IsEmpty() { return nil }  // no-op, skip the entire client call
changer.SetAttrs(file, upd)
```

**Estimated win:** a bunch of SETATTRs on the hot path go from 1 RTT to 0
RTTs. Hard to estimate without a trace — guess ~20–30 % of SETATTRs become
no-ops.

**Decide before coding:** ride on top of candidate 2. They should ship
together.

**Scope estimate:** 20 extra lines in the candidate-2 change.

---

## Suggested next-pass order of operations

Ranked by `expected user-perceived improvement` × `cheapness`:

1. **Candidate 1 + 2 together** (CreateFile HSETNX + batched SetAttrs +
   no-op skip). ~100 lines, low risk, lands ~80 ms per create improvement.
   Biggest-remaining-visible-win-for-smallest-change bucket.
2. **Candidate 6 (cheap variant):** commit one reference fs_usage trace and
   a README pointer. 30 minutes. Future regression insurance.
3. **Measure actual session jsonl size distribution from a real Claude
   session** (there is a snippet in `tasks/perf-nfs-analysis.md` explaining
   why). One-shot `find ~/.claude/projects -name '*.jsonl' -printf '%s\n'
   | sort -n` is enough. If the 95th percentile is > 1 MB, promote
   candidate 3 (Lua) or candidate 4 (schema split) to "do next". Otherwise
   shelve them.
4. **Candidate 5** only if a CPU profile says `resolvePath` is hot.
   Probably never.

Everything else waits for a measured reason.

## Not on this list (intentional)

- Increasing Redis connection pool size. Single NFS server, no concurrent
  writers. Current pool of 16 is fine.
- Client-side TCP tuning (TCP_NODELAY, keepalive). RTT is dominated by
  Redis processing + TLS handshake overhead at the network edge, not
  Nagle-induced buffering. Measured with `AFS_NFS_OPSTATS=1`.
- NFS v4 migration. Would enable native xattrs (killing the need for the
  AppleDouble shadow), but the shadow is already free after pass 2 and
  the migration is a multi-week project.
- Async writeback / write-behind cache. Would confuse Claude Code's
  crash-safety assumptions around session.jsonl.

## Status key

- **current:** what pass 2 shipped.
- **proposed:** described above, not yet implemented.
- **decide:** the question that has to be answered before coding.
