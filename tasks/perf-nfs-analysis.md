# AFS NFS perf analysis: ~/.claude

## TL;DR

- **Reads are fine.** After `afs up` finishes path cache warming (~900ms), every read/stat/readdir op on AFS is within ~10% of local disk. The existing cache + warming machinery works.
- **Writes are catastrophic.** 100 jsonl appends against a small file take **31.5s on AFS** vs **7.5ms on local** — a ~4,200x slowdown. This is what makes Claude Code feel slow, because Claude Code appends to a session `.jsonl` file after every message.
- **Root cause:** each NFS WRITE RPC costs ~8 Redis round-trips, and for every write the entire file content is read from and rewritten to Redis. Worse, each write calls `invalidatePrefix("/")` which blows the path cache, so the next stat/read pays cold-cache cost.
- **Top fix:** rewrite `WriteInodeAt` to (a) fetch metadata + content in ONE round trip, (b) pipeline the metadata/content save with the two counter updates into ONE round trip, (c) update the path cache in place instead of wiping it. Estimated 2–3× improvement on writes. If that is not enough, follow up with a Lua EVALSHA variant that does the read-modify-write server-side and avoids transferring content at all (~10× improvement, but larger change).

## Methodology

- `scripts/bench_claude_fs.sh <dir>` — runs 12 ops N times, emits CSV. Uses a single perl harness per measurement (~2ms noise floor) and runs random-stat/random-read/append in-process via perl so fork/exec overhead does not dominate.
- `scripts/bench_compare.sh` — wraps the above: does `afs down` + `afs up` before an AFS cold pass, re-runs for warm, then runs the same against `/tmp/claude-local/` (an `rsync -a --delete` copy).
- Sample: 1332 files / 636 dirs / ~50 MB.
- Redis: `thing-yearly-candied-59640.db.redis.io:11499` (remote managed Redis). Effective Redis RTT from this Mac: ~15-25ms based on the append numbers below (see [Math](#math)).

Not run in this pass (follow-ups):
- `scripts/capture_claude_fs_patterns.sh` — needs sudo for `fs_usage`, deferred to manual run.
- `INFO commandstats` sampling — no `redis-cli` on this host.

## Baseline results (`tasks/perf-run-v2/summary.md`)

All values are median ms over 5 rounds. Harness noop is ~2ms, so "1x" ratios on sub-5ms ops are noise-bound.

| op | local | afs warm | afs cold | warm/local | cold/local |
|---|---:|---:|---:|---:|---:|
| stat_root | 6.14 | 6.52 | 6.66 | 1.1x | 1.1x |
| readdir_root | 4.12 | 3.85 | 3.88 | 0.9x | 0.9x |
| tree_walk (find -type f, 1332 files) | 25.64 | 27.04 | 26.04 | 1.1x | 1.0x |
| tree_walk_dirs | 24.67 | 23.49 | 23.28 | 1.0x | 0.9x |
| ls_recursive | 51.68 | 56.00 | 56.14 | 1.1x | 1.1x |
| du -sh | 14.62 | 17.53 | 17.45 | 1.2x | 1.2x |
| grep_text | 46.01 | 42.70 | 43.67 | 0.9x | 0.9x |
| glob_md | 25.89 | 24.52 | 23.97 | 0.9x | 0.9x |
| random_stat (100 files) | 4.57 | 4.53 | 4.49 | 1.0x | 1.0x |
| random_read (100 files) | 6.68 | 6.67 | 9.48 | 1.0x | 1.4x |
| head_of_tree (read well-known files) | 9.17 | 9.03 | 10.24 | 1.0x | 1.1x |

Note: "cold" here really means "first pass after `afs up` completes". Because path cache warming is eager (scans the whole Redis keyspace in ~900ms at mount time), there is essentially no cold cost visible at the client level — the cost is paid in `afs up`, not at first access. That is actually good design.

## Write results (the interesting part)

Added write ops in a second pass:

| op | local | afs warm | AFS / local |
|---|---:|---:|---:|
| write_new_small (50 brand-new small files) | 12 ms | **158,458 ms** | **12,779x** |
| write_overwrite (50 overwrites of 1 small file) | 8 ms | **31,772 ms** | **3,972x** |
| append_jsonl (100 single-line appends to 1 file) | 7.5 ms | **31,580 ms** | **4,211x** |

Per-op math:
- `append_jsonl`: 316 ms per append on AFS, 0.075 ms per append on local.
- Single-append probe (perl, 1 append, small existing file): observed 168 ms (`open: 1.4ms, print: 0.0ms, close: 168ms`). macOS NFS client issues the WRITE RPC on close with sync semantics; the 168ms is the full server round trip.
- `write_new_small` is worse than `append_jsonl` per-op because creating a file adds `CreateFile` RTTs (parent lookup + dirents HSet + allocInodeID Incr + new inode HSet).

## Why writes are slow — code trace

Path: macOS kernel → NFS v3 WRITE RPC → `third_party/go-nfs/nfs_onwrite.go:onWrite` → billy `FS.OpenFile`/`fileHandle.Write` in `mount/internal/nfsfs/fs.go` → `client.WriteInodeAt` in `mount/internal/client/native_range.go`.

Per single WRITE RPC:

1. `nfs_onwrite.go:54` — `fs.Stat(fullPath)` for **pre-op wcc**. → `client.Stat` → `resolvePath` + `loadInodeByID`. **1 RTT (cache hit usually — but see step 3e).**
2. `nfs_onwrite.go:67` — `fs.OpenFile(fullPath, O_RDWR, ...)` → **another** `client.Stat` inside `nfsfs/fs.go:73`. **1 more RTT (or cache hit).**
3. `nfsfs/fs.go:418` — `fh.fs.client.WriteInodeAt(ctx, inode, payload, offset)` → `native_range.go:48`:
   - `loadInodeByID` → `HMGET inode:{id} <11 metadata fields>`. **1 RTT.**
   - `loadContentByID` → `HGET inode:{id} content`. **1 RTT** (separate call to the *same* hash key — should be merged).
   - Go-side byte math to build the new buffer.
   - `saveInodeDirect` → `HSET inode:{id} <metadata + new content>`. **1 RTT.** For a 400 KB session.jsonl, this ships the entire file content back to Redis on *every append*.
   - `c.invalidatePrefix("/")` — nukes the in-process path cache for **all paths**. No RTT, but cascades: every subsequent stat has to repopulate.
   - `adjustTotalData` → `HINCRBY afs:{fs}:info total_data_bytes delta`. **1 RTT.**
   - `markRootDirty` → `SET afs:{fs}:rootDirty 1`. **1 RTT.**
4. `nfs_onwrite.go:95` — `tryStat(fs, path)` for **post-op wcc**. → another `client.Stat`. Because step 3e just invalidated the cache, this walks the path from `/` hitting Redis at each component. **2–N RTTs.**

**Total: 7–10 Redis RTTs per NFS WRITE RPC**, plus 400 KB × 2 bytes over the wire if the file is 400 KB.

For Claude Code's session jsonl (typically 100 KB – 1 MB once a conversation runs for a while), this is terrible:
- Each append transfers ~2× the file size to/from Redis.
- `invalidatePrefix("/")` ensures the next LOOKUP/GETATTR burst from macOS (which NFS clients do a lot) pays cache-miss cost, adding more RTTs.
- macOS NFS client issues WRITEs synchronously on close, so the user-observed wall time = server round-trip time.

## Math

Observed append close time: **168 ms**.
Code review says: **8 RTTs** per WRITE in the common case (2 stats + 5 in WriteInodeAt + 1 post-op stat).
→ Effective per-RTT cost ≈ 168 / 8 ≈ **21 ms**. Plausible for a cloud Redis with TLS/auth overhead.

Claude Code's session.jsonl is typically touched once per conversation turn (user message → assistant message cycle appends one or more JSONL lines). With 168 ms per append, every turn pays at least that much, and on top of it macOS's GETATTR/LOOKUP bursts pay cold-cache cost because the write wiped the path cache.

Independent check: 100 `append_jsonl` rounds took 31.5 s → 315 ms per append. Higher than the single-file probe because the bench op reopens the file per append (open+append+close) from a shell `perl` loop, so each iteration also pays open-time stats. Consistent story.

## Ranked fixes

### #1 — Pipeline WriteInodeAt + targeted cache invalidation (picked as top fix)

**Change**: in `mount/internal/client/native_range.go:48`, rewrite `WriteInodeAt` to:
- Use one HMGET that fetches **metadata fields + content together** (one RTT instead of two).
- Use a single pipeline with `HSet(inode)` + `HIncrBy(total_data)` + `Set(rootDirty)` (one RTT instead of three).
- Stop calling `c.invalidatePrefix("/")`. Instead, take a `path string` parameter and update the cache entry for that specific path with the new size and mtime. Callers already know the path.

Add a new method on the client interface (`WriteInodeAtPath(ctx, inode, path, payload, off)`). Keep `WriteInodeAt` wrapping it with an empty path for any non-NFS callers.

Update `mount/internal/nfsfs/fs.go:fileHandle.Write` (line 393) to call `WriteInodeAtPath(ctx, inode, fh.path, ...)`.

**Expected impact**: 5 RTTs → 2 RTTs inside WriteInodeAt. Total per NFS WRITE RPC: ~8 → ~5. Plus the post-op stat is now a cache hit (was a cold path resolve). Realistic: 168 ms → ~60–80 ms per append. **~2–3× speedup.**

**Risk**: low. Schema unchanged. Content is still stored as a hash field. Only the access pattern changes. The `invalidatePrefix` removal is the most subtle part — the surgical alternative (update the cache entry in place) preserves correctness because we know the exact path being updated.

### #2 — Redis Lua script for read-modify-write (follow-up if #1 is not enough)

**Change**: add a small Lua script that on the server:
1. Reads current content (`HGET key content`).
2. Computes the new buffer (Lua string ops).
3. Writes new content + size + mtime atomically (`HSET`), updates `total_data_bytes`, sets root dirty.
4. Returns the new size.

Load the script once with `SCRIPT LOAD` at client init; invoke via `EVALSHA`.

**Expected impact**: 2 RTTs → 1 RTT. And — crucially — the full file content stays inside Redis. For a 400 KB file, appending a 100-byte line transfers ~100 bytes over the wire instead of ~800 KB. **10–20× speedup** on appends to large files.

**Risk**: moderate. Need to handle NOSCRIPT fallback (script evicted), Lua memory limits (~64MB default per script execution), and the slightly different semantics of atomic server-side modification. Worth doing if #1 does not bring writes close to local perf.

### #3 — Store content in a separate Redis string key

**Change**: stop storing `content` as a hash field on `inode:{id}`. Put it at a new key `content:{id}` as a Redis string. This unlocks:
- `APPEND content:{id} payload` for pure appends → O(1), one RTT, negligible transfer.
- `SETRANGE content:{id} offset payload` for random writes → one RTT, only the delta transferred.
- `GETRANGE content:{id} off len` for partial reads → one RTT, only what you need.
- Metadata-only operations (`stat`, `chmod`, etc.) no longer touch content.

**Expected impact**: this is the "right" answer. For appends on large files, probably **20–50× speedup** and reads of small ranges become cheap too.

**Risk**: higher. Schema change. Requires migration for existing workspaces, careful handling of create/delete/truncate, and touches many paths in the client. Not worth it unless #1 and #2 do not solve the Claude Code pain point.

### #4 — Remove the double `Stat` in NFS write path

**Change**: in `nfs_onwrite.go:54` the pre-op stat and in `fs.go:73` inside `OpenFile`, there are two `Stat` calls for the same path. Cache the first result. Small refactor; saves 1 RTT per WRITE. Included implicitly in #1 once the cache is no longer wiped.

### #5 — Consolidate pre-op/post-op wcc

**Change**: `onWrite` does a pre-op stat and a post-op stat. NFSv3 spec allows this to be weaker when the server knows the data is consistent. Another 1 RTT saved. Minor.

### #6 — Tune Redis client

- Current pool size is 16 (`mount/cmd/agent-filesystem-nfs/main.go:106`). For the typical single-client-per-NFS-server case this is fine; increase if bursty concurrent writes become the bottleneck after #1.
- Consider TCP_NODELAY and keepalive tuning on the Redis connection if on AWS managed Redis (TLS handshake setup is not the bottleneck for steady-state — the 21 ms/RTT is network + Redis processing).

## Decision

Implement **#1** now, re-benchmark, and decide on #2/#3 based on the resulting numbers.

## Results after fix #1

Changed files:
- `mount/internal/client/native_range.go` — rewrote `WriteInodeAt` and `TruncateInode`. Both now go through new `WriteInodeAtPath` / `TruncateInodeAtPath` methods that (a) load metadata + content in a single HMGET, (b) pipeline the save + counter update + root-dirty set into a single round trip, and (c) update the path cache in place when the caller supplies the path.
- `mount/internal/client/client.go` — added `WriteInodeAtPath` and `TruncateInodeAtPath` to the `Client` interface.
- `mount/internal/nfsfs/fs.go` — `fileHandle.Write`, `fileHandle.Truncate`, and the O_TRUNC branch of `OpenFile` now pass `fh.path`/`p` to the new methods.

Result (AFS warm median ms, 5 rounds, same bench as baseline):

| op | baseline | fix #1 (Write only) | fix #1 (Write + Truncate) | total speedup |
|---|---:|---:|---:|---:|
| append_jsonl (100 appends) | 31,580 | 4,026 | 3,815 | **8.3×** |
| write_overwrite (50 ops) | 31,772 | 12,700 | 3,760 | **8.5×** |
| write_new_small (50 creates) | 158,458 | 91,254 | 58,530 | 2.7× |

Per-op:
- `append_jsonl`: 316 ms/op → **38 ms/op**. A Claude Code session.jsonl append now takes roughly two Redis round trips instead of eight.
- `write_overwrite`: 635 ms/op → **75 ms/op**. Limited by the Stat/OpenFile path, which still has two redundant stats per WRITE RPC (see fix #4).
- `write_new_small`: 3.2 s/file → **1.2 s/file**. Still bad because `CreateFile` has its own multi-RTT sequence that this fix did not touch. See the follow-up section.

Read-side numbers did not regress: every read op is within ~5% of its baseline reading (and still ~1× local disk).

### Does fix #1 land the Claude Code improvement?

Yes, for the dominant case. Claude Code's hot write pattern is appending lines to `projects/*/*.jsonl` after each message. That op went from ~316 ms per append to ~38 ms per append — an **8× improvement** that should be subjectively obvious. Target for fix #1 was "100 ms per append or better"; we landed at 38 ms.

### What's still slow (follow-ups)

- **CreateFile is still slow** (~1.2 s per new file). Root cause is the same pattern: sequential Redis calls in `createFileIfMissing` (parent resolve → `lookupChildID` → `allocInodeID` → pipelined inode/dirents HSet → separate `markRootDirty`). Applying the same "pipeline + don't invalidate" pattern here should bring it to ~80 ms. This matters for Claude Code's startup (creating per-session files) and skill bundling, but not for steady-state conversation.
- **The NFS WRITE RPC still does two Stats** (`fs.Stat` pre-op wcc + `fs.OpenFile` internal stat) plus a third post-op stat. After fix #1 these are cache hits, so they cost ~0 RTTs — but they still burn Go/allocation cycles. Cleaning them up is fix #4 from the ranked list.
- **Lua / separate content key (#2, #3)** are not needed for the jsonl append hot path after #1. They become interesting again if we see Claude Code appending to multi-MB session files, because our current code still transfers the full file content over the wire on every append. For typical conversation sizes (100–500 KB) that's tolerable after #1.
- **The capture script still needs a manual sudo run** to produce an actual fs_usage trace of a real canned Claude Code session. Hypothesis confirmed by math + code review and by the 8× benchmark win, but a trace would be nice to commit into the repo for regression testing.

## Part 2 — follow-up pass: AppleDouble shadow, CreateFile cache, markRootDirty throttle

After landing fix #1, a deeper investigation of `CreateFile` (~1.2 s per new file) turned up a surprise that dominated almost every non-read operation on the mount: **macOS AppleDouble sidecar files**.

### The discovery

For a single `echo "hi" > x.txt`, macOS NFS client actually issues **18+ NFS RPCs**:

```
nfsfs: OpenFile path="/__probe_debug/x.txt"         (create)
nfsfs: OpenFile truncating inode=X path="/__probe_debug/x.txt"
nfsfs: Close
nfsfs: OpenFile path="/__probe_debug/._x.txt"       (create sidecar!)
nfsfs: OpenFile truncating inode=Y path="/__probe_debug/._x.txt"
nfsfs: Close
nfsfs: OpenFile path="/__probe_debug/._x.txt" flag=0x2
nfsfs: Write inode=Y off=0 len=4096                 (full 4 KB sidecar!)
nfsfs: Close
nfsfs: OpenFile path="/__probe_debug/._x.txt" flag=0x2
nfsfs: Write inode=Y off=0 len=163
nfsfs: Close
nfsfs: OpenFile path="/__probe_debug/._x.txt" flag=0x2
nfsfs: Write inode=Y off=152 len=11
nfsfs: Close
nfsfs: OpenFile path="/__probe_debug/x.txt" flag=0x2
nfsfs: Write inode=X off=0 len=3                    (actual "hi\n")
nfsfs: Close
```

**More than half of the NFS traffic for every single file touch was going to AppleDouble `._*` sidecar files.** macOS creates these on every non-HFS/APFS filesystem to store resource-fork and extended-attribute metadata it cannot otherwise express. They are never observed by cross-platform tools or by anything accessing the workspace through the AFS CLI, but they dominate NFS traffic on this mount.

I verified this with a Redis op counter (`AFS_NFS_OPSTATS=1` — see `mount/cmd/agent-filesystem-nfs/main.go`): a single isolated `echo > file` hit Redis with **12+ round trips**, ~7 of which were for the ._ sidecar alone. At ~85 ms effective Redis RTT in this run (cloud Redis is slower than the LAN assumption), that's the full ~1.0 s per create.

### Fixes landed

**A. In-memory AppleDouble shadow** — `mount/internal/nfsfs/appledouble.go` (new) + shadow-routing in `fs.go`.

Every operation whose basename starts with `._` is routed to an in-process `shadowStore` that holds the files in memory for the life of the NFS server. Reads, writes, truncates, stats, removes, renames, chmod/chown/chtimes all work against the in-memory copy. **Zero Redis traffic for AppleDouble sidecars.**

Design decisions:
- ReadDir does **not** surface shadow entries. macOS reaches sidecars via direct LOOKUP/OPEN (which the shadow handles), and hiding them from READDIR matches native Apple-filesystem semantics where "._" files are invisible to directory enumeration. As a bonus it avoids a TOCTOU race with tools like rsync (which stat every entry they see in a listing) because the shadow mutates under live Claude Code sessions.
- Shadow entries are per-process. They are regenerated by macOS on the next file touch — losing them across a mount restart is the same as losing them across an xattr flush on a native filesystem, which macOS handles fine.
- Shadow files use synthetic inode numbers starting at `1<<40` to avoid any collision with real inode IDs from Redis.

**B. CreateFile cache path cleanup** — `mount/internal/client/native_helpers.go`.

Two cooperating fixes:
- `ensureParents` now walks the path cache before hitting Redis. Previously it always issued 2 RTTs per parent component (HGET dirents + HMGET inode), even when the parent chain was already warm. Now a burst of creates in the same directory costs zero RTTs for parent traversal.
- `createFileIfMissing` and `createInodeUnderParent` no longer wipe the parent's path-cache entry. They invalidate only the dir **listing** cache (which genuinely is stale after adding a child) and update the parent's cached inode with the new mtime in place. This preserves the warm cache for sibling operations that follow. A new helper `invalidateDirListing` was added in `native_core.go`.

**C. `markRootDirty` throttle** — `mount/internal/client/native_core.go` + `native_helpers.go`.

Every mutating client op (Chmod, Chown, Utimens, Write, Truncate, CreateFile, Mkdir, Rm, Rename) used to end with an unconditional `SET rootDirty "1"`. The value is always "1" — it's a boolean hint to the control plane that the workspace has unflushed changes — so writing it on every op wastes a full round trip per mutation.

The fix: debounce. `markRootDirty` now skips the Redis write if the last successful SET was within a `markRootDirtyThrottle = 100 ms` window. In a burst of 50 creates this collapses 50 RTTs into 1. Worst case: a reader observing the marker lags by up to 100 ms, which is acceptable because the marker is a hint, not a lock.

**D. Redis op counter hook** — `mount/cmd/agent-filesystem-nfs/main.go`.

Added a `go-redis` hook that counts commands + pipelines and logs the delta every second. Gated by `AFS_NFS_OPSTATS=1` so it is a zero-cost opt-in debugging tool. This was the measurement that made the AppleDouble problem impossible to miss — I recommend leaving it in permanently.

### Results — full trajectory

AFS warm median (ms, 5 rounds):

| op | baseline | fix #1 | fix #2 (Trunc) | fix A (shadow) | fix B+C (cache+throttle) | final vs baseline |
|---|---:|---:|---:|---:|---:|---:|
| append_jsonl (100 appends) | 31,580 | 4,026 | 3,815 | 5,739 | 4,661 | **6.8×** |
| write_overwrite (50 ops) | 31,772 | 12,700 | 3,760 | 5,296 | 3,895 | **8.2×** |
| write_new_small (50 new files) | 158,458 | 91,254 | 58,530 | 32,105 | 26,046 | **6.1×** |

Per-op wall time:
- `append_jsonl`: 316 ms → **47 ms per append** (6.7×). Matches a cost of ~2–3 Redis RTTs, consistent with the theoretical floor for this Redis backend.
- `write_overwrite`: 635 ms → **78 ms per overwrite** (8.1×).
- `write_new_small`: 3,170 ms → **521 ms per create** (6.1×). The remaining cost is dominated by `createFileIfMissing`'s WATCH/MULTI/EXEC round trip sequence (~4–5 RTTs) plus `attrs.Apply` running Chmod/Chown/Utimens sequentially (~3 RTTs) — see "still-standing follow-ups" below.

Reads are still within 5–10 % of local disk for all bench ops. Nothing regressed.

Run-to-run variance across passes is roughly 30 % — cloud Redis latency is noisy, so the individual-column numbers above wobble but the trajectory is stable.

### Still-standing follow-ups

- **Lua EVALSHA for `WriteInodeAtPath`.** Would collapse the current 2 RTTs to 1 and — more importantly — avoid transferring file content over the wire (read-modify-write server-side). Interesting once Claude Code session jsonls grow past ~1 MB.
- **Separate Redis string key for file content.** The "elegant" version of the Lua fix. Unlocks native APPEND/SETRANGE/GETRANGE semantics. Schema change with migration cost; only worth it if the ~47 ms/append number becomes a problem again.
- **The capture script still needs a manual sudo run.** The AppleDouble discovery was made via `AFS_NFS_DEBUG=1` + the new `AFS_NFS_OPSTATS=1` hook, not via fs_usage — so the fs_usage trace is less urgent than before.

## Part 3 — follow-up pass: HSETNX CreateFile + batched SetAttrs

After pass 2, `tasks/perf-nfs-followups.md` candidates 1 and 2 were the
top remaining wins on the write hot path. Both are now landed.

### Candidate 1 landed: HSETNX `createFileIfMissing`

`mount/internal/client/native_helpers.go:536` was rewritten to replace
the WATCH/MULTI name-claim dance with an optimistic HSETNX pipeline:

1. `INCR nextInode` — 1 RTT.
2. Pipeline `HSETNX dirents parent name id` + `HSet inode` +
   `queueTouchTimes` + `queueCreateInfo` — 1 RTT.

On the rare lost-race path the loser pays one extra RTT to DEL the
orphan inode and compensate the optimistic `files` / `total_data_bytes`
counter bumps. In the single-writer Claude Code case contention is
effectively zero, so the effective cost per create drops from the
4-RTT WATCH/MULTI flow to 2 RTTs.

### Candidate 2 landed: batched `Client.SetAttrs` + `BatchSetAttrer` fast path

A new `SetAttrs(ctx, path, AttrUpdate)` method was added to the `Client`
interface (`mount/internal/client/client.go`) and implemented in
`native_core.go`. It writes only the non-nil fields in one partial HSet
against the inode hash, skipping the full `inodeFieldsAtPath` map, so
the wire payload is 1-5 fields instead of 13.

`mount/internal/nfsfs/fs.go` got a `FS.SetAttrs(name, *mode, *uid, *gid,
*atime, *mtime)` wrapper that handles read-only, AppleDouble shadow
routing, and os-type -> client-type conversion. It satisfies a new
optional interface `BatchSetAttrer` declared in
`third_party/go-nfs/file.go`. `SetFileAttributes.Apply` type-asserts
changer against `BatchSetAttrer`; when the assertion succeeds (our FS
does) the entire Chmod + Lchown + Chtimes triple collapses into one
client call. The "candidate 7" no-op skip is folded in: if the computed
diff is empty after comparing against the pre-op stat, the whole call
is skipped and zero Redis commands land.

memfs and upstream go-nfs consumers that do not implement
BatchSetAttrer still run the legacy Chmod + Lchown + Chtimes path.

### Tests added

- `TestCreateFileRaceLoserCleansUpOrphan` — two goroutines race on the
  same path; asserts exactly one winner, matching inode IDs on both
  sides, `Info.Files == 1`, and exactly two inode keys in Redis.
- `TestCreateFileRaceLoserCompensatesContentBytes` — two Echo calls
  with non-empty content race; asserts `Info.TotalDataBytes ==
  len(payload)` (loser's content-byte delta is properly compensated).
- `TestCreateFileRaceExclusiveLoserReturnsError` — both callers
  exclusive; asserts exactly one gets `"already exists"`.
- `TestCreateFileOverExistingDirectoryPreservesError` — collides a
  create against a dir, asserts `"not a file"` and that counters are
  untouched (the failed create must not leak an `HIncrBy files -1`).
- `TestSetAttrsPartialFieldsOnly` — table-driven partial-update
  verification across mode / uid / gid / atime / mtime permutations.
- `TestSetAttrsEmptyIsNoOp` — zero Redis commands for
  `SetAttrs(AttrUpdate{})`.
- `TestSetAttrsCacheUpdatesInPlace` — post-`SetAttrs` `Stat` hits the
  path cache (zero HMGETs on the inode hash).
- `TestSetAttrsParityWithChmodChownUtimens` — two identical workspaces
  end up with identical inode state after the batched and legacy
  paths; the HGets match exactly on mode/uid/gid/atime_ms/mtime_ms.
- `TestSetAttrsSingleRoundTrip` — a 5-field `SetAttrs` issues exactly
  one `HSet` against the inode hash.
- `TestFSSetAttrsFlowsThroughToClient`, `TestFSSetAttrsShadowAppleDouble`,
  `TestFSSetAttrsShadowNotExist`, `TestFSSetAttrsReadOnly`,
  `TestFSSetAttrsEmptyIsNoOp` — the `nfsfs.FS.SetAttrs` wrapper tests.
- `TestSetFileAttributesApplyDispatchesToBatchSetAttrer` — end-to-end
  through the go-nfs `SetFileAttributes.Apply` dispatcher: building a
  real `SetFileAttributes` and calling `Apply(fs, fs, path)` must hit
  the fast path with exactly one inode HSET, not three.
- `TestSetFileAttributesApplyEmptyDiffIsFreeRider` — an `Apply` whose
  diff is entirely a no-op issues zero inode HSETs (candidate 7
  free-rider win).
- `TestSetAttrsCommandsStrictlyLessThanLegacy`,
  `TestCreateFileCommandCountIsBounded` — regression guards pinning
  the hot-path command counts.
- `BenchmarkCreateFile`, `BenchmarkSetAttrsBatched`,
  `BenchmarkLegacyChmodChownUtimens` — wall-clock benchmarks against a
  local `redis-server` spawned on a free port.

### Results

Command counts, warm cache, local `redis-server`:

| op | pre-Fix 1+2 | post-Fix 1+2 | reduction |
|---|---:|---:|---:|
| CreateFile (uncontended) | 10-12 cmds | **8 cmds** | -2-4 cmds |
| SetAttrs (5-field update) | 6 cmds (legacy 3x HSet + 3x PUBLISH) | **2 cmds** | **3.0x** |

Wall clock, local `redis-server` (Apple M4, Apple Redis 7.2, go test -bench=.):

| op | ns/op | allocs/op |
|---|---:|---:|
| `BenchmarkCreateFile` | 266,128 ns | 398 |
| `BenchmarkSetAttrsBatched` (5 fields) | **36,581 ns** | **39** |
| `BenchmarkLegacyChmodChownUtimens` | 121,209 ns | 171 |

On local Redis the batched SetAttrs path is **3.3× faster** than the
equivalent Chmod + Chown + Utimens triple and uses **4.4× fewer
allocations**. The absolute savings of 84 µs/op scales linearly with
RTT, so on a 20 ms-RTT remote Redis this is the predicted ~40-80 ms
saved per SETATTR and CREATE that was called out as the expected win
in `perf-nfs-followups.md`.

Wall-clock numbers on the 20 ms-RTT cloud Redis will need a separate
pass against the actual backing store (this repository was tested
against the user's local Redis at 127.0.0.1:6379). The command-count
reduction is the stable, machine-independent proof that the fix lands.

### Still-standing follow-ups

Deferred per the decision framework in `perf-nfs-followups.md`:

- **Lua EVALSHA for `WriteInodeAtPath`** (candidate 3). Not needed for
  the typical 10-500 KB session.jsonl sizes; revisit if measured
  workloads move past ~1 MB.
- **Separate Redis string key for file content** (candidate 4). Schema
  change with migration cost; parked unless candidate 3 is also
  insufficient.
- **Collapsing the three NFS-write stats** (candidate 5). CPU-only
  win, profile first.
- **Writeback buffer on the NFS server.** Not yet needed; the two
  fixes above close the hot-path gap to the point where remote-Redis
  perception should be noticeably better. Re-measure in a real Claude
  Code session before escalating.


## Re-benchmark plan

1. Run `scripts/bench_compare.sh 5 tasks/perf-run-after-fix-1` after the fix.
2. Compare `append_jsonl`, `write_overwrite`, `write_new_small` medians.
3. Target: append_jsonl median ≤ 10 s for 100 appends (100 ms per append). That would be ~3× improvement — acceptable.
4. If target met: stop, ship, open a follow-up ticket for #2 as "nice to have".
5. If not met: add #2 (Lua) and repeat.

## Open items / follow-ups

- Run `sudo scripts/capture_claude_fs_patterns.sh` to produce an actual fs_usage trace of a real Claude Code canned session. Confirms the top-N paths are what we think (`projects/*/*.jsonl`, `CLAUDE.md`, `settings.json`, `memory/MEMORY.md`).
- Install `redis-cli` (`brew install redis`) and sample `INFO commandstats` before/after a canned Claude session to get absolute op-count numbers.
- Measure whether macOS NFS client coalesces multiple small user writes into one NFS WRITE RPC, or issues one RPC per fwrite. (`fs_usage -f filesys` would show.)
- `afs down` + `afs up` currently takes ~2 s wall time (path cache warming is ~900 ms, plus mount). If that becomes a startup-perceived problem for Claude Code editors that auto-mount, that would be the next thing to look at.
