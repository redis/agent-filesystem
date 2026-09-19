# Atomic file commits

Status: completed
Owner: Codex / AFS maintainers
Created: 2026-09-19
Updated: 2026-09-19

## Goal and result

Prevent stale sync uploads from overwriting simultaneous edits in a shared Redis
workspace, independent of which control plane each client uses. The verified
snapshot now supplies the conditional publication token, mode is committed with
content, and sync records the exact commit result. Local candidates survive
conflicts.

## Completed scope

- [x] Stable reads and conditional sync upload, save, and recovery writes.
- [x] Complete staged String/Array publication with selected-database COPY.
- [x] Exact commit metadata and frozen history bytes.
- [x] Receipt-based acknowledgment of transport retries after later peer edits.
- [x] Async retired-content cleanup and one durable journal event per mutation.
- [x] Revision-aware persisted baselines and recovery comparisons.
- [x] Concurrency, lost-response, malformed-bookkeeping, cache, and cleanup tests.
- [x] Current protocol/rollout docs and benchmark comparison.

## Decisions and adaptation

The initial design was made against an older checkout. Current main (`1ff1fa0`)
already has revisioned staged Lua publication, parent-chain WATCH validation,
workspace-generation fencing, and range-write retry semantics. This work reuses
those mechanisms instead of introducing a second mutation-state schema or new
directory namespace revisions. Existing data layout and client interfaces remain
compatible through optional context helpers.

Receipts protect within-call transport retries. Persisted pending-operation IDs
and cross-process replay were not added; restart recovery verifies the observed
remote state. File history remains on the existing API, supplied with frozen
bytes. Multi-file transactions and automatic edit merging are outside scope.
All writers must be upgraded for the sync precondition guarantee.

## Verification

- Core: `go test ./cmd/... ./deploy/... ./internal/...`.
- Mount: `go test ./...` with Redis 7.4.2, plus Redis 8.10.0 Array tests.
- Repeated targeted atomic/sync tests with `-race -count=3`.
- `make commands mount`; no running control plane restarted.
- Three-sample before/after publication benchmarks; results in
  `docs/internals/performance.md`.

No implementation remains in flight. GitHub branch/PR delivery follows final
verification; deployment is not part of this change.
