# Post-Review Cleanup Follow-ups

Status: active
Owner: rowan
Created: 2026-05-05
Updated: 2026-05-05

## Goal

Track the cleanup items identified in the pre-review pass on PR
[#13](https://github.com/redis/agent-filesystem/pull/13) but deliberately
deferred from that PR to keep individual commits scoped and reviewable.
Each item below has a concrete file:line anchor and a recommended
approach so future work can pick up cold.

## Scope

Code quality, structural refactors, and test-coverage gaps. Product
roadmap items (cloud, benchmarks, versioned-fs) belong in
[future-work.md](future-work.md) and are out of scope here.

## What landed in PR #13

Listed for context so this plan stays additive.

- Phase 1 — Doc/config paper cuts (`.dockerignore`, `.gitignore`,
  `Makefile`, `example.afs.config.json`, plans archived/moved,
  `cli-first-ui.md` dedupe, `repo-walkthrough.md` refresh).
- Phase 2 — Dead code (-480 LOC): `afs-situation-room-kit.tsx`,
  `background-pattern.tsx`, `file_versions.go` trampolines,
  `parentPath`/`baseName` in afsfs, `Capabilities` type alias,
  `examples/codex-settings-migration.md`.
- Phase 3 — UI deps: jszip dynamic import, `lucide-icons.tsx` canonical
  import, lint --fix.
- Phase 4 — UI helpers: `foundation/sort-compare.ts`,
  `foundation/clipboard-icons.tsx`.
- Phase 5 — Reconciler renames: `sync_reconcile.go` →
  `sync_full_reconciler.go`, `sync_reconciler.go` →
  `sync_event_reconciler.go`.
- Phase 6 — `.github/workflows/ci.yml` (root, mount, sandbox, ui, ui-lint).
- Phase 7 — Sandbox bind default `127.0.0.1` + threat-model doc + WARNING
  log on external bind.
- Phase 8 — FUSE write cache flush bug fix (afsfs handle/file route
  through `*AtPath` variants).
- Phase 9 — UI lint baseline cleared (27 → 0); ui-lint job now blocking.
- Phase 10 — Sandbox HTTP timeouts, output buffer cap, process-map GC,
  first sandbox tests.
- Phase 11 — Mount client typed error sentinels in
  `mount/internal/client/errors.go`; `mapError` now uses `errors.Is`
  with substring fallback for externally-formatted messages.
- Phase 12 — Extract MCP helpers to `internal/mcptools/` (text-patch,
  args parsing, shared shapes); ~800 LOC of duplication consolidated to
  one canonical implementation; aliases left in place transitionally.

## Checklist

### Tier A — Quick wins (under 1 hour each)

- [ ] **Controlplane writeError sentinels.**
      `internal/controlplane/http.go:2246-2255` still does
      `strings.Contains(strings.ToLower(err.Error()), ...)` for status
      mapping. Define sentinels mirroring the mount side
      (`ErrAlreadyExists`, `ErrInvalidArgument`, `ErrUnsupported`,
      `ErrNotAllowed`, `ErrIsDirectory`, `ErrNotDirectory`), wrap returns
      from `service.go` and `database_manager.go`, switch
      `writeError` to `errors.Is`.
- [ ] **Mount cache LRU bound.**
      `mount/internal/cache/cache.go:25` is an unbounded
      `map[string]AttrEntry`. With the 1-hour TTL and per-inode +
      per-listing population, a million-file workspace OOMs the mount
      daemon. Add an LRU cap (configurable, default ~100k entries) or a
      periodic sweep loop.
- [ ] **Drop mcptools aliases + inline rename.**
      `cmd/afs/afs_mcp.go` and
      `internal/controlplane/mcp_hosted_helpers.go` retain
      `type mcpFooBar = mcptools.FooBar` and
      `var mcpFooBar = mcptools.FooBar` aliases from the Phase 12
      extraction. Replace ~150 call sites of `mcpRequiredString(...)`
      with `mcptools.RequiredString(...)`. Mechanical, rg/sed-friendly.
- [ ] **Pre-existing redis-go deprecations.**
      `internal/controlplane/store.go:420,444,629,656`,
      `file_versions.go:871,893`, `import_lock.go:77` use
      `ZRevRange`, `SetNX`, `ZRangeByScore` which the redis-go client
      flags as deprecated. Migrate to `ZRangeArgs` (with `Rev` /
      `ByScore` options) and `Set` with the `NX` option.
- [ ] **Per-database error swallowing.**
      `internal/controlplane/database_manager.go:2057-2074`,
      `:1444`, `:1488`, `:1538` silently `continue` on per-database
      Redis errors. Accumulate via `errors.Join` and return; at
      minimum log so a Redis blip is visible.
- [ ] **Two `panic(err)` calls on configuration paths.**
      `internal/controlplane/auth.go:226` `NewNoAuthHandler` panics if
      `DefaultAuthConfig()` ever fails to validate;
      `internal/controlplane/blob_writer.go:150` `BlobWriter.newPipeline`
      panics on an unsupported `redis.Cmdable`. Return errors so callers
      can decide.

### Tier B — Medium refactors (1–3 hours each)

- [ ] **MCP `mcpDiffOperand*` extraction.**
      Currently in both `cmd/afs/afs_mcp.go:1058-1126` and
      `internal/controlplane/mcp_hosted.go`. Cannot move to `mcptools`
      because the helpers take `controlplane.FileVersionDiffOperand` —
      circular dep. Solution: move the operand type to a small shared
      package (e.g. `internal/fileversionops/`) or pass the type as a
      generic. ~150 lines consolidated.
- [ ] **Redis-value coercion duplicated three places.**
      `cmd/afs/workspace_mount_bridge.go:420-448`,
      `internal/controlplane/workspace_root_manifest.go:237-254`,
      `cmd/afs/afs_search_index.go:118` reimplement
      `coerceString`/`coerceInt64`. Extract to
      `internal/rediscontent/`.
- [ ] **`use-afs.ts` mutation hook repetition.**
      `ui/src/foundation/hooks/use-afs.ts:851-985` defines 13
      `useCreate*`/`useDelete*`/`useUpdate*` hooks that each repeat
      the same `useWorkspaceInvalidation` + `useMutation` shape.
      A `makeWorkspaceMutation()` factory shrinks this by ~120 lines.
- [ ] **Sandbox HTTP authentication.**
      `--bind 127.0.0.1` is the only protection today. For callers
      that need to bind externally, add a token check (env or flag-
      sourced). Document the threat model addition in the sandbox
      package comment that already exists.
- [ ] **Sandbox tests for pre-existing code.**
      Phase 10 covered the new buffer + GC. The original
      `Launch`/`Read`/`Wait`/`Kill` flow still has zero direct tests —
      add lifecycle and timeout cases.
- [ ] **23 `type Foo = foo` aliases in `internal/controlplane`.**
      `service.go:378-396` and `database_manager.go:112-114`. Either
      export the underlying structs in place (and delete the aliases)
      or document why the package wants the lowercase-internal /
      Uppercase-API split.
- [ ] **Boolean-flag parameter sweep.**
      `saveAFSManifest(..., bool)`,
      `parseAFSArgs(args, allowForce, allowReadonly bool)`,
      `cmdImportSelfHosted(..., replaceExisting bool, ..., mountAtSource bool)`,
      `newReconciler(...)` 14 positional args. Replace with options
      structs to make call sites readable.

### Tier C — Big surgical refactors (half-day+ each)

- [ ] **`Resolved` / scoped collapse in
      `internal/controlplane/database_manager.go` + `http.go`.**
      ~30 method pairs (`SaveCheckpoint` /
      `SaveResolvedCheckpoint`, `RestoreCheckpoint` /
      `RestoreResolvedCheckpoint`, etc.) differ only in whether they
      call `resolveScopedWorkspace` or `resolveWorkspace`. ~1500
      lines of avoidable duplication. Plan: introduce a `route`
      struct and one `resolve(ctx, databaseID, workspace) (*Service,
      route, error)` helper that handles both cases via a default
      empty `databaseID`. Independently: replace the 40-branch
      `switch strings.HasSuffix(...)` URL dispatch in
      `handleWorkspaceRoute` / `handleResolvedWorkspaceRoute` with a
      real router (chi, gorilla, or `http.ServeMux` with `{workspace}`
      placeholders if Go 1.22+).
- [ ] **File splits for the 2000+ LOC files.**
      Each is a careful 1–2 hour exercise on its own.
      - `ui/src/foundation/api/afs.ts` (3641 lines): split
        `client.ts`, `http.ts`, `http-types.ts`, `mappers.ts`,
        `demo/`. Hand-rolled and well-typed already; mostly
        mechanical.
      - `internal/controlplane/database_manager.go` (2916 lines):
        split into `_registry.go`, `_workspace.go`, `_aggregate.go`.
        After the `Resolved`-collapse the file shrinks first.
      - `cmd/afs/afs_mcp.go` (2490 after Phase 12): split per tool
        family (file-read, file-write, grep, workspace, checkpoint).
      - `internal/controlplane/service.go` (2843 lines): split per
        concern (`_workspace.go`, `_checkpoint.go`, `_session.go`,
        `_tree.go`, `_helpers.go`).
      - `internal/controlplane/http.go` (2271 lines): split into
        `http_admin.go`, `http_client.go`, `http_workspace.go`,
        `http_resolved.go`, `http_helpers.go`. Kill the 627-line
        `newAdminMux` with route helpers.
      - `cmd/afs/workspace_checkpoint_commands.go` (2105 lines).
      - `internal/controlplane/mcp_hosted.go` (still 1753 after
        Phase 12).
- [ ] **Big test-file unit / integration split.**
      `internal/controlplane/http_test.go` (2507 lines),
      `internal/controlplane/file_versions_test.go` (1944 lines),
      `cmd/afs/sync_integration_test.go` (1887 lines). Tag the
      integration-side with `//go:build integration` so plain
      `go test ./...` stays fast.
- [ ] **Migrate mount tests off real `redis-server` to `miniredis`.**
      `mount/internal/client/native_test.go` forks a real
      `redis-server` per test, 46x in parallel. CI installs the
      binary today (Phase 6) but the cost is real on smaller
      runners. Use `miniredis` for the bulk; keep one or two
      against a real server for behaviors miniredis can't model.
- [ ] **`mount/client/` shim duplication.**
      Every new exported type in `mount/internal/client/` has to be
      re-aliased in `mount/client/` because of the nested-module
      boundary. Either auto-generate the shim from a list of
      symbols, or merge the modules.
- [ ] **Drop the vestigial `native_*` prefix in mount client.**
      `mount/internal/client/native_core.go`, `native_helpers.go`,
      `native_text.go`, `native_walk.go`, `native_range.go`,
      `native_locks.go`. There's only one client implementation;
      the prefix carries no information. Rename and split
      `native_helpers.go` (1418 lines) into `paths.go`, `search.go`,
      `inode.go`.
- [ ] **Drop the `afs_*` prefix in `cmd/afs/`.**
      `afs_commands.go`, `afs_types.go`, `afs_local.go`,
      `afs_diff.go`, `afs_grep.go`, `afs_hash.go`, `afs_mcp.go` —
      everything in `cmd/afs/` is already "afs". Drop.
- [ ] **`MountedFS.bash()` syncs the entire workspace per call.**
      `sdk/python/src/redis_afs/client.py:374`,
      `sdk/typescript/src/index.ts:538`. For a workspace with 10k
      files that's hundreds of MCP round-trips per shell call.
      Document loudly as O(workspace), or replace with a real
      mount on the SDK side.

## Decisions / Blockers

- **`format-toggle.tsx` + `lib/snippets.ts` are explicitly held back
  from any cleanup pass.** They are listed as Phase 1 deliverables of
  the active [cli-first-ui.md](cli-first-ui.md) plan (harvested from
  the now-removed `agent-site/`). If `cli-first-ui.md` is paused or
  superseded, add a checkbox here to delete them as confirmed dead
  code.
- **Sandbox tests for the existing `Launch`/`Read`/`Wait`/`Kill`
  flow** are listed in Tier B, but the sandbox itself is the rare Go
  surface that is also a security boundary. Tests should include
  hostile-input cases, not just happy paths — budget for that when
  scheduling.

## Verification

Each item should:
- Land as its own commit (or atomic series for the big surgical ones).
- Run `make test` plus the targeted module's tests after the change.
- For UI items: run `cd ui && npm run build && npm run test &&
  npm run lint` (lint is now blocking in CI).
- For mount/sandbox items: run `cd mount && go test ./...` /
  `cd sandbox && go test ./...` respectively.

## Result

Fill in before archiving.
