# AFS Usage Feedback Tracker

Last reviewed: 2026-04-30.
Status: working tracker.
Source: external AFS usage log and feedback from 2026-04-30.

## Purpose

This document tracks the bugs, product feedback, and suggested implementation
plan from the 2026-04-30 AFS usage feedback batch.

Use this as the working board for deciding what to do, what not to do, and what
order to do it in. Once an item graduates into a detailed implementation plan,
link the plan from the relevant row instead of duplicating all details here.

## Triage Legend

- `P0`: security, tenant isolation, or data-loss risk.
- `P1`: blocks successful first real use or breaks trust in the product.
- `P2`: important UX/product polish after core trust is restored.
- `P3`: nice-to-have, packaging, or later-scale improvement.

Status values:

- `Needs repro`: verify the report against current local and hosted behavior.
- `Planned`: accepted, not yet implemented.
- `In progress`: actively being implemented.
- `Done`: implemented and verified.
- `Deferred`: accepted as useful, but not in the first implementation slice.
- `Won't do`: intentionally rejected.

## Executive Summary

The feedback points to one product correction:

AFS should feel like a workspace-first remote filesystem that can be attached
to, detached from, inspected, and checkpointed. The current surface still leaks
too much implementation vocabulary: `up`, `down`, sync, mount, database,
session, and ambiguous workspace/database IDs.

The most urgent risks are:

1. Tenant isolation and cross-tenant error leakage.
2. Local data-loss fear from `afs ws detach` deleting a directory by default.
3. Import appearing to do nothing.
4. A confusing command model for the real workflow: create a workspace from an
   existing directory and keep working in that directory.

## Current Product Direction

Accepted direction:

- Prefer `attach` and `detach` for local lifecycle.
- Preserve local files by default.
- Keep `workspace` as the primary noun.
- Keep databases mostly invisible in normal use.
- Keep CLI output boring and scriptable for operational commands.
- Add remote filesystem inspection so a workspace can be explored without
  mounting or syncing it locally.

Detailed command-surface plan:

- [CLI Attach/Detach Command Surface Plan](cli-attach-detach-command-surface.md)

## Implementation Waves

### Wave 0: Security And Trust

Goal: remove tenant/data-loss risk before expanding the UX.

1. Fix tenant isolation and cross-tenant error leakage.
2. Make `detach` semantics safe by default.
3. Stop deleting local sync directories unless `--delete` is explicit.
4. Ensure errors never print other tenants' workspace IDs, database IDs, or
   workspace names.

Validation:

- Add duplicate `getting-started` fixtures across tenants/databases.
- Verify `afs ws detach`/session cleanup cannot resolve across unauthorized
  workspace routes.
- Verify local directories survive normal detach/down.

### Wave 1: First Real Local Workflow

Goal: make the desired workflow obvious and working.

1. Add `afs ws attach <workspace> <directory>`.
2. Add `afs ws detach <directory> [--delete]`.
3. Keep `afs ws` as the first-class documented workspace command group.
4. Keep hidden root shortcuts out of docs/help.
5. Make import noisy and trustworthy: scan summary, progress, result summary,
   skipped paths, and actionable errors.
6. Rework docs/help/install next steps around attach/detach.

Validation:

- Clean HOME CLI tests for attach/detach parsing and behavior.
- End-to-end import existing directory, attach, edit file, detach, reattach.
- Non-TTY output snapshots for scriptability.

### Wave 2: Inspect Before Attach

Goal: let users inspect a remote workspace without a local attachment.

1. Add or consolidate a remote filesystem command group.
2. Support listing, cat, find, and grep against workspace state.
3. Support checkpoint-qualified references such as `workspace@checkpoint`.
4. Keep this distinct from local attach/detach lifecycle.

Proposed shape:

```bash
afs fs -w <workspace> ls
afs fs -w <workspace> cat memories/entities/people.md
afs fs -w <workspace> find . -name '*.md' -print
afs fs -w <workspace@checkpoint> grep Redis .
```

Decision still needed: whether `afs fs` is the right name, or whether these
belong under `afs ws files` or `afs fs`.

### Wave 3: History, Tags, And Logs

Goal: make workspace state auditable and recoverable.

1. Add a user-facing log command that shows workspace operations and file
   operations together.
2. Add checkpoint labels if the tag model is still wanted after checkpoint UX
   review.
3. Keep tags as labels for checkpoints, not Git-style branches or commits.

Proposed shapes:

```bash
afs log -w <workspace>
afs tag create <workspace>@<checkpoint-or-timestamp> <name>
afs tag list <workspace>
afs tag delete <workspace> <name>
```

### Wave 4: Packaging And Scale

Goal: reduce setup friction and support larger local setups.

1. Fix installer PATH behavior.
2. Consider an install option into an existing writable PATH location.
3. Add Homebrew tap after release/signing/versioning is stable.
4. Support multiple attached workspaces per daemon or per machine.

## Feedback Tracker

| ID | Priority | Area | Feedback | Triage | Suggested plan | Status |
| --- | --- | --- | --- | --- | --- | --- |
| F001 | P2 | Pre-login web | A landing page before login would be welcome. | Good product ask. Current signed-out app mostly routes to auth and protected app chrome. | Add a public signed-out landing/getting-started page at `/`, with no protected workspace queries before auth. Keep post-login getting-started flow. | Done |
| F002 | P1 | Login | Validation code did not work, then multiple new codes arrived. Auth service felt flaky. | Needs repro with exact hosted auth flow and provider logs. | Instrument browser login/code exchange. Improve expired/invalid code handling. Prevent confusing multiple pending codes where possible. Show exact retry state. | Needs repro |
| F003 | P2 | Onboarding | Getting-started experience after login is very good. | Preserve this. | Keep the successful post-login flow intact while changing pre-login and CLI command copy. Add regression coverage for starter workspace naming. | Planned |
| F004 | P2 | Installer | Copy/paste curl installer is good. User manually read shell first. | Keep simple curl installer, make it readable and auditable. | Keep script concise. Consider checksum/signature copy once binary release path is stable. | Planned |
| F005 | P1 | Installer PATH | Installer edits `.zshrc`, but `hash -r` does not make `afs` available in current shell. | Current-shell availability is a first-run trust issue. | After install, continue using absolute binary path for next steps and print exact `export PATH=...` for current shell. Explore writable existing PATH install option. | Planned |
| F006 | P2 | Installer PATH | Installer writes absolute home path instead of `$HOME`. | Easy polish. | Write `export PATH="$HOME/.afs/bin:$PATH"` when install dir is default. Preserve explicit custom `AFS_INSTALL_DIR` literally. | Planned |
| F007 | P3 | Packaging | Homebrew tap would be even better. | Good later packaging path, not first slice. | Defer until release artifacts, signing/notarization, and versioning are stable. | Deferred |
| F008 | P1 | Import | `afs ws import` seemed to do nothing. Tried running/not running, new/initial workspace, no output, no files. | Must repro. Import cannot be silent. | Add clean repro matrix. Ensure import always emits scan, progress, success/no-op/error summary. Validate current source path, workspace route, and backend mode. | Needs repro |
| F009 | P2 | CLI output | CLI output is too busy. Prefer boring easily parsed output, no tables/ascii/color or optional. | Accepted for operational commands. | Add plain non-TTY defaults, `--json` where useful, `--no-color`, and compact fixed-column output. Keep rich boxes only for guided setup/onboarding. | Planned |
| F010 | P1 | Mental model | Workspace/database/directory/agent model is not immediately obvious. | Product copy leak. | Define: workspace = durable file tree, database = storage backing, attached directory = local working surface, agent = connected client/daemon. Hide databases by default. | Planned |
| F011 | P2 | Databases | User does not care about databases and liked that they mostly disappeared. | Preserve. | Keep database selection as advanced/admin. Do not solve ambiguity by exposing more database detail in happy-path commands. | Planned |
| F012 | P1 | Local workflow | User wanted to create a workspace from an existing directory and attach that same directory. Could not find clear path. | Core first-real-use gap. | Make `afs ws attach <workspace> <dir>` canonical. Rework import/attach flow so existing local directory can become the attached working directory safely. | Planned |
| F013 | P0 | Local data loss | `afs ws detach` deletes a directory on unmount. Scary. Should require `--delete`; default should detach. | Accepted as trust/data-loss issue. | Add `afs ws detach`. Make default preserve local files. Make `--delete` explicit and path-guarded. Transition `down` to detach behavior. | Planned |
| F014 | P1 | Vocabulary | `attach`/`detach` feels like a better metaphor than lifecycle start/stop language. | Accepted. | Make `afs ws attach` / `afs ws detach` the documented surface and update help/docs/install copy. | Planned |
| F015 | P1 | Large files | Large files appear ignored, probably safety/testing, with no logs. User thought it was broken. | Accepted. Silent skip is a bug. | Emit skipped-file rows with path, size, cap, and config key. Include skip count in attach/import/sync summaries and `afs log`. | Planned |
| F016 | P2 | Multi-attach | Allow daemon to support many attached/mounted workspaces. Current model appears to support one. | Useful but bigger state model change. | Defer until single attach/detach semantics are correct. Then replace single state file with attachment records keyed by local path and workspace ID. | Deferred |
| F017 | P1 | Remote inspection | Allow remote inspection of an unmounted workspace before attaching. | Accepted. | Add or consolidate `afs fs -w <workspace> ...` style commands for ls/cat/find/grep. Reuse existing workspace tree/content/search APIs. | Planned |
| F018 | P2 | Workspace list | Proposed `afs ws list` with size. | Good if `ws` is first-class. | Decide whether to make `ws` documented. Add sizes to ws list in a stable column if available cheaply. | Planned |
| F019 | P2 | Remote fs commands | Proposed `afs fs -w agent1 ls/find/grep/cat`. | Accepted direction, exact command name open. | Design separate remote-inspection surface. Avoid conflating with attach/detach. | Planned |
| F020 | P1 | Attach output | Proposed verbose attach operation table with import/download/upload/conflict/skipped codes. | Accepted. | Implement attach reconciliation summary and optional verbose per-path operation output. | Planned |
| F021 | P2 | Logs | Proposed `afs log -w agent1` showing workspace and file ops. | Accepted. | Build on unified event/changelog stream. Keep default concise, add `--json` for tooling. | Planned |
| F022 | P2 | Tags | Proposed `afs tag create agent1@<ts> pre-attach-v1`. | Useful, but should not become Git vocabulary. | Treat tags as checkpoint labels or aliases. Require checkpoint/timestamp resolution. Keep out of first attach/detach slice. | Deferred |
| F023 | P0 | Tenant isolation | `afs ws detach` error showed same workspace name across multiple databases with IDs. Looks like cross-tenant information leak. | Production blocker. | Enforce principal/org scoped workspace/session cleanup. Resolve by opaque workspace ID where possible. Scrub unauthorized IDs/names from errors. Add duplicate starter workspace tests. | Planned |
| F024 | P0 | Tenant model | Feedback says definitely no tenant isolation and to be careful. | Treat as security program, not copy tweak. | Audit service-layer authorization for workspace/session/database operations, especially cleanup and global list routes. | Planned |

## Suggested Work Breakdown

### Security Fix Package

Scope:

- Session cleanup called from `afs ws detach`.
- Workspace resolution where names can exist in multiple databases.
- Global list/resolve paths used by CLI, UI, and hosted MCP.

Deliverables:

- Tenant-scoped workspace resolution helper.
- Sanitized ambiguity errors.
- Tests with duplicated `getting-started` workspaces across tenants/databases.
- Minimal prod deploy after verification.

### CLI Lifecycle Package

Scope:

- `afs ws attach`
- `afs ws detach`
- `afs ws detach --delete`
- Help/docs/install/onboarding copy.

Deliverables:

- Safe default detach behavior.
- Backward-compatible `up`/`down` routing during transition.
- No local deletion unless runtime state proves the path is attached and
  `--delete` is present.
- CLI tests for parse, preserve, delete, and help output.

### Import Reliability Package

Scope:

- `afs ws import`
- Existing directory attach path.
- Skipped/ignored/large file reporting.

Deliverables:

- Repro script or test fixture for the reported "no output, no files" case.
- Mandatory result summary.
- Verbose per-path operation table.
- Clear skip reasons.

### CLI Output Package

Scope:

- Operational command output formatting.
- Color/no-color behavior.
- JSON output for automation.

Deliverables:

- Non-TTY plain output defaults.
- `--json` on list/log/status-style commands where useful.
- `--no-color` global or command-level behavior.
- Snapshot tests for parseable output.

### Remote Inspection Package

Scope:

- Workspace filesystem commands without attach.
- Checkpoint-qualified references.

Deliverables:

- Command name decision.
- `ls`, `cat`, `find`, and `grep` MVP.
- Reuse browser/MCP/control-plane file APIs.
- Tests against active workspace and checkpoint state.

## What We Should Not Do In The First Pass

- Do not make databases more visible to paper over ambiguous workspace routing.
- Do not add hidden aliases.
- Do not implement multi-attach before single attach/detach is safe.
- Do not turn checkpoint tags into Git-like branch/commit semantics.
- Do not preserve delete-by-default behavior for compatibility.
- Do not redesign the entire UI before fixing CLI trust issues.

## Open Decisions

1. Should `afs ws detach` accept `--delete`, or should destructive behavior exist
   only as `afs ws detach <dir> --delete`?
2. Should remote inspection be named `afs fs`, or
   `afs ws files`?
5. Should `detach --delete` require an interactive confirmation in TTY mode?

## Next Review Checklist

Use this checklist when planning the next implementation pass:

- Confirm the P0 tenant leak path and write the failing test first.
- Confirm current `down` deletion behavior in sync and mount modes.
- Decide command names for `attach`, `detach`, and optional `ws`.
- Decide whether the first attach implementation targets sync only.
- Update this tracker with accepted/rejected decisions before coding broad
  command-surface changes.
