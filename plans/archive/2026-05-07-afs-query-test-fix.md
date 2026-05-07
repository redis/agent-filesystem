# afs query test/fix

Status: completed
Owner: Codex
Created: 2026-05-07
Updated: 2026-05-07

## Goal

Lock down `afs query` CLI behavior with a thorough regression, run it, report real bugs, then fix every failure.

## Scope

- `cmd/afs` query command contract and nearby test helpers.
- Targeted Go tests for `afs query`.
- Fix only bugs proven by the new regression.

## Checklist

- [x] Add broad `afs query` regression coverage.
- [x] Run targeted query tests.
- [x] Report failures as bugs.
- [x] Delegate independent fixes to subagents.
- [x] Integrate fixes.
- [x] Rerun targeted validation.

## In Flight

- Complete.

## Decisions / Blockers

- Used `plans/` instead of root `tasks/`; this repo explicitly retired top-level `tasks/`.
- Bug found: `afs query index files` routed to query-index management and errored instead of searching for `index files`.
- Fix: query-index routing now triggers only for `index`, `index <help>`, or `index <status|rebuild|clean>`.

## Verification

- [x] `go test ./cmd/afs -run Query` failed before fix: `cmdQuery(index files)` returned `unknown query index subcommand "files"`.
- [x] `go test ./cmd/afs -run Query -count=1` passed after fix.
- [x] `go test ./cmd/afs -count=1` passed after fix.

## Result

Added a broad `afs query` CLI regression covering request defaults, explicit query controls, typed hybrid fallback, path-scoped JSON results, semantic-disabled JSON output, file-only output, and natural queries beginning with `index`. Fixed query-index subcommand disambiguation so natural queries like `afs query index files` search normally while `afs query index status|rebuild|clean` remains reserved for index management.
