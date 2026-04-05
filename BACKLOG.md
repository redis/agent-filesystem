# Redis-FS Engineering Backlog

This backlog translates the review into an implementation plan with clear tickets and acceptance criteria.

## Milestone P0: Correctness Hardening (Immediate)

### P0-1 Block invalid subtree moves
- Problem: `FS.MV` currently allows moving a directory into its own subtree.
- Scope:
  - Reject `dst` if `dst == src` or `dst` is under `src/`.
  - Add regression tests.
- Acceptance:
  - `FS.MV key /a /a/b` returns error.
  - Existing tree remains intact after rejected move.

### P0-2 Strict path-depth validation
- Problem: deep paths could be silently truncated during normalization.
- Scope:
  - Make normalization fail when depth exceeds `FS_MAX_PATH_DEPTH`.
  - Return deterministic user error.
  - Add tests that verify no truncated alias is created.
- Acceptance:
  - Too-deep path returns `ERR path depth exceeds limit`.
  - No inode is created for either full or truncated path.

### P0-3 Strict metadata argument validation
- Problem: `CHMOD` and `CHOWN` currently accept malformed or out-of-range input.
- Scope:
  - `CHMOD`: require strict octal parse in `[0000, 07777]`.
  - `CHOWN`: reject negative and `>UINT32_MAX` values.
  - Add regression tests.
- Acceptance:
  - Invalid mode strings fail without mutating inode mode.
  - Invalid uid/gid fail without mutating ownership.

### P0-4 Preserve metadata in `FS.CP`
- Problem: `CP` did not preserve full metadata.
- Scope:
  - Preserve `uid/gid/mode/ctime/mtime/atime` for files, dirs, and symlinks.
  - Propagate copy failures to caller.
  - Add regression tests.
- Acceptance:
  - `FS.STAT` fields match on source and copied destination (except path).
  - Recursive copy failure returns error, not silent success.

## Milestone P1: Test and Reliability Expansion

### P1-1 Coverage for new APIs
- Add tests for `FS.TRUNCATE` and `FS.UTIMENS` edge cases.
- Acceptance:
  - Shrink/extend/zero semantics verified.
  - `-1` UTIME_OMIT semantics verified.

### P1-2 Invariant verification helper
- Add test utility to verify:
  - each child entry resolves to inode,
  - each non-root inode is referenced by exactly one parent child list.
- Acceptance:
  - Utility runs after `MV/CP/RM RECURSIVE` tests.

### P1-3 CI integration
- Automate:
  - module build,
  - Redis boot with module,
  - Python integration suite,
  - Go mount unit tests.
- Acceptance:
  - CI fails on any regression.

## Milestone P2: FUSE Correctness and UX

### P2-1 Wire CLI flags end-to-end
- Implement `--allow-other` and `--foreground` behavior.
- Acceptance:
  - Effective mount options reflect CLI flags.

### P2-2 Robust cache invalidation
- Invalidate subtree caches on rename/remove recursive operations.
- Acceptance:
  - No stale lookups after subtree moves/deletes.

### P2-3 Range-based I/O path
- Introduce ranged read/write server APIs and use them in FUSE handle path.
- Acceptance:
  - No full-file rewrite for small writes.
  - Better concurrent writer behavior.

## Milestone P3: Data-Model Evolution

### P3-1 Inode-ID based namespace
- Move from path-keyed canonical storage to inode-id + dir-entry relations.
- Acceptance:
  - Simpler rename semantics and lower duplication.
  - Foundation for hard links/xattrs.

### P3-2 Online migration tooling
- Add migrator/compat mode for old `redis-fs0` encoding versions.
- Acceptance:
  - Existing RDB data loads and migrates without data loss.

## Milestone P4: Operability and Scale

### P4-1 Integrity and repair commands
- Add `FS.CHECK` and `FS.REPAIR`.
- Acceptance:
  - Detects and repairs reference inconsistencies.

### P4-2 Large-workload behavior
- Add chunked file payload mode and optional batched recursive operations.
- Acceptance:
  - Predictable latency under large files/subtrees.
