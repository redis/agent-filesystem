# AFS Implementation Plan

This document consolidates a set of related work items for the Agent Filesystem (AFS) codebase. The stages are ordered so that each one delivers standalone value and does not depend on later work. The array-type migration — the largest and most disruptive change — is deliberately sequenced last.

## Framing: AFS has three deployment modes, not two

Before the stages, it's worth making a framing point that sharpens the product positioning and reorders the priority of the MCP work.

AFS has historically been described as having two local surfaces:

- **Sync mode** — a real local directory kept reconciled with Redis by a background daemon
- **Mount mode** — FUSE (Linux) or NFS (macOS) exposing Redis as a live filesystem

There is a third, already-working surface that has been under-framed:

- **Library mode (via MCP)** — the `afs mcp` binary opens a Redis connection directly and serves MCP over stdio. No mount, no sync daemon, no local filesystem. Agents interact through MCP tool calls; the server translates them into Redis operations using the same `mount/client` package the FUSE and NFS drivers use. Identical key schema, identical semantics, same pub/sub invalidation channel.

The significance: library mode works in every environment where sync and mount cannot — serverless functions, privileged-container-denied pods, in-process agent frameworks, CI runners without FUSE support, managed agent runtimes where the developer has no kernel control. The only requirements are a Redis connection and the ability to run a process.

This reframing changes the priority and framing of the MCP-related stages below. Stage 2 is no longer "fill some missing tool wrappers" — it is **completing MCP as a first-class deployment target** for environments where the mount cannot reach. Stage 4 is no longer an ergonomic nice-to-have — it is **the primary ergonomic surface for library-mode agents**, who have no real shell to fall back to.

Sync and mount are the deployment modes for environments that permit them. Library mode via MCP is the portable default for everything else.

---

## Stage 1 — Delete the dead trigram search index

**Status:** Ready to ship. Zero runtime risk.

**Problem.** The mount driver maintains per-file case-insensitive trigram indexes in the inode hash (`grep_grams_ci`, `search_state`). These fields are populated on every file write but are never read. The `Grep` implementation in `internal/client/native_walk.go` walks the directory tree, loads each file's full content, and scans in Go — no index lookup, no prefilter, no short-circuit.

This is dead code on the read path. It burns write-path CPU (hashing every 3-byte window on every write), wastes network bandwidth (every inode hash carries a trigram blob), and wastes Redis memory (trigrams can be large for non-trivial files).

**Scope.**

Delete from `internal/client/native_helpers.go`:

- Constants: `fileSearchGramSize`, `fileSearchMaxIndexedBytes`, `fileSearchMaxUniqueGrams`, `fileSearchStateReady`, `fileSearchStateBinary`, `fileSearchStateLarge`
- Struct: `fileSearchFields`, including the `GrepGramsCI` and `SearchState` fields
- Functions: `buildFileSearchFields`, `fileSearchIndexFields`, `fileSearchIsBinaryPrefix`, `fileSearchGramTerms`

Remove `fileSearchIndexFields(...)` merges from every call site:

- `native_helpers.go`: lines ~482, ~557, ~643, ~744
- `native_core.go`: lines around 1062-1072
- `native_range.go`: lines around 126-143 and 220-247

Remove the `search_state` and `grep_grams_ci` hash fields from every `HSet` call.

**What stays.**

- `chunk_size` and `chunk_hashes` inode-hash fields — these feed sync reconciliation (rolling-hash chunk diffing in the sync daemon), not search
- `WriteChunks`, `ReadChunks`, `ChunkMeta` — all still used
- The binary-file detection in `grepFile` (NUL-byte check in the first 8 KB) — this is part of the grep correctness path and will stay until `ARGREP` absorbs it

**Data migration.**

None required. Existing workspaces will have stale `search_state` and `grep_grams_ci` fields on old inode hashes. These become inert — no code reads them, `HGETALL` just returns them and the driver ignores. No explicit cleanup is needed. If desired, a one-time `HDEL` pass can be added later, but it is not blocking.

**Tests.**

- Remove any assertions in `native_test.go` that check for `search_state` or `grep_grams_ci` hash fields being written
- Remove any tests that directly exercise the trigram computation
- Existing `Grep` correctness tests stay — the `grepWalk` path is unchanged

**Risk.** None. The code being deleted has no readers.

**Estimate.** ~150 lines deleted, one PR, half a day including tests.

---

## Stage 2 — Complete MCP as library-mode deployment target

**Status:** Ready to scope. No dependencies.

**Problem.** The MCP server is AFS's library-mode surface — the deployment path for every environment where FUSE and sync can't run. But its tool coverage is incomplete for that role. An agent cannot delete a file, rename a file, create an empty directory, or remove an empty directory through MCP. In a Lambda function or privileged-container-denied pod where the agent has no shell and no mount, these gaps aren't ergonomic annoyances — they're blockers. An agent doing normal coding work (creating a scratch file, testing, deleting it) hits a wall.

The underlying native client already supports all of these operations. Only the MCP tool wrappers are missing. Filling them completes library mode as a full filesystem surface.

**Tools to add (in priority order).**

Tier 1 — blocking gaps for realistic agent workflows:

| Tool | Description | Native operation |
|------|-------------|------------------|
| `file_delete` | Delete a file or symlink at a path | existing remove path |
| `file_move` | Rename or move a file/directory, creating parent dirs as needed | existing rename path |
| `directory_create` | Create an empty directory; fail if exists unless `exist_ok` | existing mkdir path |
| `directory_delete` | Delete a directory; `recursive` flag for non-empty | existing rmdir path |

Tier 2 — quality-of-life, ship after Tier 1 is stable:

| Tool | Description |
|------|-------------|
| `file_copy` | Copy a file to a new path within the workspace |
| `file_stat` | Return metadata (type, size, mode, mtime) for a single path without reading content |
| `symlink_create` | Create a symlink; the inode model already supports this |
| `workspace_delete` | Delete a workspace and its checkpoint history (with confirmation) |
| `checkpoint_delete` | Delete a specific checkpoint; decrements blob refcounts |

Tier 3 — optional, evaluate based on usage:

- `file_append` (bandwidth optimization for log-like files)
- `file_chmod` (for executable scripts the agent generates)

**Schema conventions.**

All new tools follow the existing pattern:

- `workspace` optional, defaults to the current workspace selection
- `path` always absolute within the workspace
- Structured JSON return, with `error` field on failure carrying a stable error code
- Destructive operations leave the workspace dirty until `checkpoint_create` — consistent with existing write tools

**Tests.**

Add MCP-level tests that exercise each new tool through the MCP server's callTool dispatch, asserting both success and failure modes. The underlying native operations already have coverage.

**Risk.** Low. Each tool is a thin wrapper over an existing native operation.

**Estimate.** ~4 tools × ~40 lines of tool-definition + handler each = ~160 lines for Tier 1. Plus tests. Two or three days.

---

## Stage 3 — Auto-checkpointing

**Status:** Design complete, ready to implement. Depends on Stage 2 only insofar as auto-checkpoint retention needs `checkpoint_delete` to work cleanly.

**Problem.** Today, AFS only creates checkpoints when the user explicitly runs `afs checkpoint create`. This leaves real gaps:

- Agent session starts, makes changes, crashes → no restore point exists from before the session
- Agent imports a workspace but forgets to create an initial checkpoint → restore and fork are impossible
- Long-running agent session accumulates hours of work → no recovery points if something breaks

The invariant we want: **the moment any session starts, a checkpoint exists representing the pre-session state.** Everything else is optimization.

**Four triggers.**

1. **Pre-session (most important).** On `CreateWorkspaceSession`: if `root_dirty` is set (previous session left changes unsaved), create a checkpoint named `auto/pre-session-{sessionID}-{timestamp}` before returning the session. If the workspace is clean, do nothing — HEAD already represents the pre-session state.

2. **Initial-on-import.** `afs workspace import` already has the concept of an `initial` checkpoint (the `afsInitialCheckpointName` constant exists). Make sure this is always created, unconditionally, before the import completes. Without it, freshly imported workspaces have no HEAD and fork/restore don't work.

3. **Session-end.** On clean session disconnect and on session lease expiry (crash path): if dirty, create `auto/session-end-{sessionID}-{timestamp}`. This captures the final state of the agent's work, including after crashes.

4. **Idle recovery.** In the sync daemon's background loop: when `root_dirty` has been set for ≥ 30 seconds with no writes in the last 30 seconds, and it's been ≥ 2 minutes since the last checkpoint, create `auto/idle-{timestamp}`. Idle-triggered rather than interval-triggered to avoid capturing mid-refactor states.

**Naming and retention.**

All auto-checkpoints live under the `auto/` namespace prefix. This is detectable and filterable.

Default `afs checkpoint list` shows only manual checkpoints. New flag `--all` or `--auto` exposes the auto ones.

Retention policy for `auto/` checkpoints:

- All auto-checkpoints from the last 24 hours: keep
- Hourly resolution for the last 7 days: keep one per hour
- Daily resolution for the last 30 days: keep one per day
- Older: delete

Retention runs as a background task in the control plane, probably triggered on session start (at most once per hour globally). Requires working `checkpoint_delete` with proper blob refcount decrement and blob GC — see Stage 2 Tier 2.

**Concurrency.**

The BFS manifest builder does not freeze the workspace. Auto-checkpoints taken during active writes can capture mixed pre-write/post-write states across files. This is best-effort: an inconsistent auto-checkpoint is still a vastly better restore point than none.

If strict snapshot consistency is ever needed, it requires either a read-lock-during-BFS protocol or a CoW snapshot layer. Not in scope here.

**Multi-session policy.**

Current assumption: one active session per workspace. If concurrent sessions become supported, the pre-session trigger needs rethinking — a second session's pre-session checkpoint would capture the first session's in-progress state. Note in the implementation comments; revisit if/when multi-session lands.

**Tests.**

- Session start on dirty workspace creates `auto/pre-session-*`
- Session start on clean workspace creates nothing
- Import creates `initial` checkpoint even without explicit call
- Crashed session (lease expiry) creates `auto/session-end-*`
- Idle trigger fires only after write quiescence period
- Retention policy correctly prunes old auto checkpoints and decrements blob refs

**Risk.** Medium. Incorrect retention logic could delete checkpoints users care about. Mitigate: auto-checkpoints are always namespaced under `auto/`, so manual checkpoints are never touched by the retention logic.

**Estimate.** 1–2 weeks. The triggers are straightforward; the retention logic and blob GC is the substantive part.

---

## Stage 4 — Bash-shaped MCP tool

**Status:** Ready to design, independent of other stages.

**Problem.** In library mode — the deployment path for environments without a mount or sync — MCP is the agent's *only* filesystem surface. There is no shell to fall back to, no `bash` binary on the host, no way for the agent to shell out. Every filesystem interaction must happen through MCP tool calls.

The current semantic surface (17 structured tools) has two costs in this context. First, it requires the agent to learn an AFS-specific API — every conversation turn carries the full tool list in context, and the agent has to map its shell-shaped thinking ("grep -rn TODO src/") onto a set of structured JSON tool calls. Second, for read-shaped operations the agent already knows how to express in bash (`ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`, `sort`, `uniq`, `diff`), the mapping is token-inefficient and ergonomically awkward. Agents are trained on bash; they fight the semantic API.

Meanwhile, destructive write operations benefit from structured tools: `file_replace` is safer than `sed -i`, `file_write` is safer than `echo > file`. Errors are legible, arguments are validated, behavior is predictable.

A bash-shaped tool gives library-mode agents the shell they otherwise lack, while structured tools preserve safety for destructive operations.

**Design.** Hybrid surface.

**Keep as structured semantic tools:**

- All destructive writes: `file_write`, `file_replace`, `file_insert`, `file_delete_lines`, plus the new Stage 2 destructive tools (`file_delete`, `file_move`, `directory_delete`)
- All stateful operations: `checkpoint_create`, `checkpoint_restore`, `workspace_fork`, `workspace_create`, `workspace_use`

**Add a new `bash` tool** that handles everything read-shaped. Use Option C from the earlier analysis: curated command subset, no real bash interpreter. Parse the command string, recognize known patterns, dispatch to native client operations, return output formatted to look like the real command.

**Commands to support in v1 of the `bash` tool:**

- `ls [-la] [path]` — directory listing with optional long format
- `cat path [path ...]` — concatenate files
- `head [-n N] path` / `tail [-n N] path` — line-oriented reads
- `grep [-rnilwE] pattern [path]` — wraps the existing `client.Grep`
- `find path [-name glob] [-type f|d]` — tree walk with filters
- `wc [-l|-w|-c] path` — line/word/byte counts
- `stat path` — metadata
- `pwd`, `echo text` — trivial, no filesystem interaction

**Compositions to support:**

- `&&` sequential with exit-on-fail
- `;` sequential unconditional
- Output redirect `> path` and `>> path` — these dispatch to `file_write` / `file_append` with the captured stdout

**Not supported in v1:**

- Pipes (`|`) — significant implementation cost for limited win. Revisit only if agents demand it.
- Subshells, command substitution
- Loops, conditionals, functions
- Environment variable expansion beyond a minimal set
- `sed`, `awk` — if needed, add as explicit commands later; do not try to be a real shell

**Output discipline.**

Each command has a max output size cap (e.g., 256 KB). When exceeded, truncate and append a line indicating truncation and how much was omitted. Agents that blow past this are expected to narrow their search. This is the same pattern just-bash uses.

**State between calls.**

Stateless. Each `bash` tool call is isolated. Agents must use absolute paths or pass `cwd` as a structured argument to the tool. No persistent environment variables, no directory stack, no background jobs.

**Deprecation path for existing read tools.**

`file_read`, `file_lines`, `file_list`, `file_grep` can all be expressed as `bash` invocations. Once `bash` has shipped and agent traffic has moved over:

- Mark the old tools deprecated in their descriptions
- Keep them working for two releases
- Remove in a subsequent release

**Tests.**

- Each supported command with its common flag combinations
- Redirect and append operators correctly route through `file_write` / `file_append`
- Output truncation at the size cap
- Unsupported commands return a clear error with guidance (e.g., "pipes are not supported; use sequential commands with `&&`")
- `&&` short-circuits on failure
- Absolute-path requirement is enforced when no `cwd` is given

**Risk.** Medium. The main risk is scope creep — "just one more flag" or "just one more command" can turn a bounded implementation into a half-finished shell. Ship a minimal command set, observe real usage, expand deliberately.

**Estimate.** 2–3 weeks for v1. Command dispatch layer plus ~10 command implementations, each calling into existing native client operations. Plus tests.

---

## Stage 5 — Replace `grepWalk` with `ARGREP`

**Status:** Gated on Redis `ARGREP` command availability and on Stage 6 (array-backed content) landing.

**Problem.** Today, `Grep` on a directory pulls every file body over the network and scans in Go. For large workspaces this is O(workspace size) of network traffic per grep invocation, even when the pattern matches zero files.

`ARGREP` (forthcoming Redis command) runs regex matching server-side against an array key and returns only the matches. This collapses grep traffic from O(workspace) to O(matches).

**Dependencies.**

- Stage 6 must be complete (external content stored as arrays, not strings)
- `ARGREP` must be in the Redis baseline version AFS supports
- The `ARGREP` command spec must be finalized — in particular, whether it returns line numbers directly or only byte offsets

**Design.**

Replace `grepFile` in `native_walk.go`. For each file visited by `grepWalk`:

1. If `content_ref == "inline"` (tiny files in the inode hash), keep the existing in-Go scan. Content is already in hand from the prior `HGETALL`.
2. If `content_ref == "array"`, issue `ARGREP afs:{ws}:content:{id} pattern [flags]` and translate the matches.

For directory-level grep, pipeline `ARGREP` calls across files. One round trip, many files searched, matches returned.

**Line number translation.**

Three options depending on what `ARGREP` returns:

1. If `ARGREP` returns line numbers directly: trivial. Use them.
2. If only byte offsets: maintain a parallel array `afs:{ws}:nlcount:{id}` with one int per chunk holding that chunk's newline count. On match, sum prefix + count in matched chunk. Write-time cost, read-time savings.
3. Otherwise: on match, fetch only the matched chunk via `ARGET`, count lines in Go. Still much cheaper than the current "fetch every file" behavior.

Start with option 3. Move to option 2 if benchmarking shows line-number computation is a real bottleneck.

**Binary file handling.**

`ARGREP` may or may not handle binary detection server-side. If not: pre-check by fetching only chunk 0 (`ARGETRANGE contentKey 0 0`), scan first 8 KB for NUL bytes. If binary, report "Binary file matches" or skip, per current behavior.

**Regex semantics change.**

Current `grepFile` uses shell-glob matching via `globMatch`. `ARGREP` with RE2 regex is a user-visible behavior change. Document in release notes. The new behavior matches user expectations of `grep` better than the glob-based matcher does.

**Inline file exception.**

Tiny files (under the inline threshold) stay in the inode hash, not in arrays. `ARGREP` doesn't apply to them. Keep the Go-side scan for inline files. They're small; it doesn't matter.

**Tests.**

- Regex patterns that would fail against the old glob matcher
- Large-file grep with one match near the end, verify no full-body fetch
- Binary file detection
- Sparse file grep (holes must not match anything)
- Multi-file directory grep where most files don't match, verify network traffic proportional to matches only
- Inline-file grep still works unchanged

**Risk.** Medium. Regex semantics change is user-visible. `ARGREP` spec details may force design changes. Line-number edge cases need careful testing.

**Estimate.** 1 week once `ARGREP` is available.

---

## Stage 6 — Migrate external file content from strings to arrays

**Status:** Biggest single change. Gated on Redis array type being in the AFS-supported baseline Redis version.

**Problem.** Today, file bodies above the inline threshold are stored as Redis strings at `afs:{ws}:content:{id}`. This has real limits:

- **512 MB string size cap** — hard ceiling on file size
- **No native sparse semantics** — `truncate -s 10G` either allocates 10 GB or requires a side-channel hole bitmap
- **Whole-file rewrites for truncate** — current path is `GETRANGE 0 newSize-1` + `SET`, which is O(file size) of network traffic just to shrink a file
- **Partial-chunk writes force the string to grow** to `offset+len` even when writing at a large offset past current content

Arrays solve all four. An array indexed by chunk number has no size cap, treats untouched slots as genuine holes, supports O(slices-touched) range delete, and handles scattered writes without materializing the gaps.

**Schema change.**

The inode hash gains a new `content_ref` value:

- `content_ref: "inline"` — body in the inode hash's `content` field (unchanged, tiny-file path)
- `content_ref: "ext"` — body in a Redis string at `content:{id}` (legacy, read-only after migration)
- `content_ref: "array"` — body in a Redis Array at `content:{id}` (new, default for all new writes above inline threshold)

The key path `afs:{ws}:content:{id}` stays the same. Only the Redis type of the value changes. Cluster co-location via the `{ws}` hash tag continues to work.

**Chunk size.**

Store one chunk per array slot. Default chunk size **64 KiB**:

- Matches common NFS `rsize`/`wsize` and FUSE `max_read`/`max_write` values
- A 256 MiB file fits in exactly one slice at default `AR_SLICE_SIZE=4096`
- Files up to ~1 TiB fit in default array config without tuning

`chunk_size` is already in the inode hash (from the sync path). Reuse it — single source of truth.

**I/O path rewrites.**

`ReadChunks`:
```
Current:  pipeline of GetRange(contentKey, idx*chunkSize, (idx+1)*chunkSize-1) per index
New:      ARGETRANGE contentKey first last    (contiguous case)
          ARMGET contentKey idx1 idx2 ...     (scattered case)
```

`WriteChunks`:
```
Current:  pipeline of SetRange(contentKey, idx*chunkSize, data) per index
New:      ARSET contentKey startIdx data1 data2 ...    (contiguous case)
          ARMSET contentKey idx1 data1 idx2 data2 ...  (scattered case)
```

`Truncate` (shrink):
```
Current:  GETRANGE 0 newSize-1 over the wire + SET back
New:      ARDELRANGE contentKey lastKeptChunk+1 MAX  (server-side, O(slices touched))
          If truncation lands mid-chunk:
            ARGET contentKey lastKeptChunk → truncate to remainder → ARSET
```

`Extend` (grow past current):
```
Current:  SETRANGE to fill, requires materializing intermediate bytes
New:      ARSET contentKey newLastChunkIdx <data>
          Intermediate slots stay NULL; read path returns zeros
```

`loadContentExternal` (used by `Head`, `Tail`, `Lines`, `Grep` on non-inline files):
```
Current:  GET content:{id}, returns full body
New:      ARGETRANGE content:{id} 0 lastChunk, concatenate (rendering NULLs as zero-filled chunks up to size)
```

**Sparse read rendering.**

When `ARGETRANGE` returns NULL for a slot, the driver materializes a chunk-sized zero buffer for that position, then truncates the final concatenation to the file's `size` (from the inode hash). The `size` field stays authoritative for EOF.

**Migration strategy.**

Write-through-on-touch. No bulk migration.

- New files with bodies above the inline threshold: always created as arrays (`content_ref: "array"`)
- Files with `content_ref: "ext"` (legacy strings): read via old code path
- First write to a legacy file triggers in-place migration: `GET` old string → `DEL` it → `ARSET` new array with chunks → update `content_ref` to `"array"`, all inside `MULTI/EXEC`
- After several releases, a background crawler migrates any remaining legacy strings; the old read path is then deleted

**Feature flag.**

New driver config: `native.content_backend` with values `"string"` (current) and `"array"` (new). Default `"string"` in the first release that ships array support. Flip default to `"array"` after bake-in. Users on older Redis versions can keep `"string"`.

**Minimum Redis version.**

Document the minimum Redis version that includes the array type. AFS continues to support older Redis with the legacy string backend indefinitely, or until a major version bump.

**Open questions to resolve before starting.**

1. Confirm the Redis version that ships the array type and when it will be GA
2. Confirm `COPY` on array keys works cleanly in cluster mode (hash-tag co-location should make it work, but verify — `workspace fork` depends on per-file `COPY`)
3. Confirm the exact `ARMSET` / `ARSET` semantics for partial-chunk writes (need to know if a short final chunk is stored as-is or requires padding)

**Interaction with other stages.**

- Enables Stage 5 (`ARGREP`) — that stage cannot ship until external content is array-backed
- Enables a future chunk-level dedup in the checkpoint store — out of scope here but a natural follow-on. Hashing each chunk independently in the manifest builder would let checkpoints dedup at chunk granularity across blobs, which is the fix for the "huge-file re-hashed on every checkpoint" weak spot

**Tests.**

Parameterize the existing `native_test.go` suite to run against both backends:

- Tiny file stays inline regardless of backend
- Crossover write (small file grows past inline threshold) goes to the active backend
- Truncate to larger, truncate to smaller, truncate to zero
- Sparse write (write at offset 10 MB on empty file) — read back intervening bytes, expect zeros
- Partial-chunk writes at every boundary permutation (start-of-chunk, end-of-chunk, spanning chunks)
- Concurrent read during write — verify the macOS NFS non-atomic-overwrite caveat from the README is resolved by `MULTI/EXEC` around the replace
- Fork/checkpoint with array-backed content

**Risk.** High. Storage format change, partial-chunk write edge cases, migration has to be correct. The feature flag contains the blast radius at rollout.

**Estimate.** 3–4 weeks including comprehensive testing. Concentrated in `native_core.go` and `native_range.go`.

---

## Ordering rationale

1. **Stage 1 (trigram cleanup)** is free money. Ship immediately.
2. **Stage 2 (MCP completion)** completes library mode as a real deployment target. Unlocks every environment that can't mount — serverless, sandboxed, managed agent runtimes. No dependencies.
3. **Stage 3 (auto-checkpointing)** is the biggest user-facing reliability improvement. Depends only lightly on Stage 2 (for checkpoint/workspace delete).
4. **Stage 4 (bash MCP)** is the ergonomic surface library-mode agents actually want. Strongly complements Stage 2 — after Stage 2 fills tool gaps, Stage 4 reshapes how agents interact with them.
5. **Stage 5 (`ARGREP`)** depends on Stage 6.
6. **Stage 6 (array migration)** is last because it's the largest change and has the most external dependencies (Redis version, spec stability).

Stages 1–4 can progress in parallel across different contributors. Stages 5 and 6 form a sequential tail.

The product narrative that falls out of this ordering: **after Stages 1–4, AFS has three fully-viable deployment modes (sync, mount, library-via-MCP) covering every agent runtime environment**. Stages 5 and 6 then turn the attention back to performance and scale on the mount/sync paths.
