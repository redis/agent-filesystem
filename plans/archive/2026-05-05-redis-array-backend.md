# Redis Array Backend

Status: archived
Owner: codex
Created: 2026-05-05
Updated: 2026-05-05
Archived: 2026-05-05

## Goal

Implement Redis Array as the preferred live-root file-content backend when the
connected Redis server supports the Array command family, while preserving the
existing external string-key backend everywhere else.

## Scope

- Detect Redis Array support safely and cache the result.
- Prefer `content_ref=array` for new and rewritten files when Arrays are
  available.
- Preserve mixed `ext` and `array` workspaces on Array-capable Redis.
- Support full reads/writes, range reads/writes, truncation, and sync chunk IO
  for array-backed files.
- Keep manifest materialization and extraction storage-agnostic.
- Add `ARGREP` prefiltering where it preserves existing grep semantics.
- Surface Array capability, workspace content backend mix, and search-index
  state in the workspace settings UI; keep the databases table focused on
  database-level Redis version and Array capability.
- Add tests for fallback behavior and Array-backed integration.

## Checklist

- [x] Add shared Redis content backend helpers for capability detection, array
      IO, and `ARGREP` prefiltering.
- [x] Update the native mount client to prefer Arrays and preserve mixed
      backends.
- [x] Update control-plane workspace-root materialization and manifest reads to
      handle both backends.
- [x] Update grep and search-index content loading to handle array-backed
      content.
- [x] Expose Redis Array support, workspace storage profile, and search-index
      status in the Workspace Details settings card.
- [x] Show Redis database version and simplified Array capability in the
      databases table.
- [x] Add targeted tests and manual verification against the local Array Redis
      instance on port 6382.
- [x] Update current docs if the landed behavior changes repo truth.

## In Flight

- Shared `internal/rediscontent` now owns Array capability detection, chunked
  reads / writes / truncation, and `ARGREP` literal prefiltering.
- The mount client now prefers `content_ref=array` when Arrays are available
  and preserves mixed `ext` / `array` workspaces on Array-capable Redis.
- Workspace-root materialization, manifest extraction, grep indexing, and grep
  content loading all read both backends through the shared helper.
- Workspace detail responses now include database Array capability, content
  storage profile, and RediSearch index status for UI display.
- Database records now expose Redis server version and Search capability in the
  sampled/status blocks; the databases table shows only version plus Array and
  Search capabilities, not workspace storage inventory.
- Workspace settings no longer show Redis keyspace or database-level Array
  support. Array-backed file storage is shown as a green workspace-level
  storage state, with Redis Insight launch links on both workspace settings and
  the databases table.

## Decisions / Blockers

- Array-backed files will use the existing `afs:{fs}:content:{inode}` key name.
  The key type changes from `STRING` to `ARRAY`; the inode hash remains the
  source of file metadata and logical size.
- The initial Array encoding is fixed-size chunked byte storage rather than a
  section-aware or line-aware semantic format.
- `ARGREP` is a candidate filter only. Exact grep output still comes from the
  existing line matcher after a file is loaded.
- Follow-up note: revisit grep acceleration with a hybrid plan that uses
  `FT.SEARCH` to narrow wildcard candidate files, then `ARGREP` to search
  within array-backed files with regex support. Validate semantics carefully
  against the chunked Array storage model before promoting it from plan to
  implementation.

## Verification

- [x] `go test ./internal/controlplane ./cmd/afs`
- [x] `cd ui && npm run test -- workspace-studio/-settings-tab.test.tsx`
- [x] `cd ui && npm run build`
- [x] `cd mount && go test ./...`
- [x] Array integration tests against
      `AFS_TEST_ARRAY_REDIS_ADDR=127.0.0.1:6382`
- [x] Manual smoke checks for local Array commands on port 6382 before wiring
      AFS to the backend

## Result

Redis Array is now wired in as the preferred live-root content backend when the
connected Redis supports the Array command family. The remaining follow-up is
benchmarking, not core enablement.
