# Agent Filesystem Repo Walkthrough

This guide is the "what lives where" map for the current `/Users/rowantrollope/git/agent-filesystem` working tree as of 2026-04-07.

## Scope

- This guide treats the current worktree as the source of truth.
- It covers project-owned source, tests, docs, examples, and build/config files.
- It does not try to inventory generated/runtime-only checkout state such as `.git/`, `.claude/`, `ui/node_modules/`, `ui/dist/`, `tests/__pycache__/`, the built `afs` and `afs-control-plane` binaries, or compiled module/mount artifacts.
- It calls out tracked files that are already deleted from the worktree in a separate legacy-cleanup section instead of treating them as active code.
- It also calls out local-only files that exist in the worktree but are not tracked yet.

## The Big Picture

AFS currently has three main implementation layers:

1. `module/`: the original Redis module that exposes `FS.*` commands against one Redis key per filesystem.
2. `mount/`: the newer Go client and mount layer that stores filesystem state in an inode-oriented Redis schema and exposes it through FUSE or NFS.
3. `cmd/` + `internal/` + `ui/`: the newer workspace/checkpoint/control-plane product surface, where Redis stores manifests, blobs, savepoints, and audit data while agents work against local materialized trees.

Supporting areas:

- `tests/`: Python integration tests for the Redis module, plus benchmark helpers.
- `sandbox/`: a process runner service for isolated execution.
- `redisclaw/`: a Python agent experiment built on top of the sandbox plus AFS-backed storage.
- `skills/`: agent-facing skill docs.
- `third_party/go-nfs/`: vendored upstream NFS library used by the NFS server binary.

## Top-Level Folder Map

- `cmd/`: Go command binaries for `afs` and `afs-control-plane`.
- `docs/`: current architecture and API notes.
- `examples/`: example configs and migration guides.
- `internal/`: shared workspace/control-plane packages used by the top-level commands.
- `module/`: Redis module implementation.
- `mount/`: Go mount and NFS serving stack.
- `redisclaw/`: Python coding-agent side project.
- `sandbox/`: HTTP and MCP process sandbox.
- `scripts/`: helper scripts for web-dev and grep benchmarking.
- `skills/`: installable agent skill docs and assets.
- `tasks/`: task/design notes.
- `tests/`: Redis-module integration suite and benchmarks.
- `third_party/`: vendored dependencies, currently `go-nfs`.
- `ui/`: React/TanStack control-plane UI.

## Root Files

- `.gitignore`: ignores built binaries, module objects, local config, Python artifacts, and common editor/test junk.
- `AGENTS.md`: repo guidance for coding agents; now best read together with this walkthrough.
- `BACKLOG.md`: milestone backlog, mostly tracking module hardening, mount correctness, and future data-model work.
- `Dockerfile.redis`: minimal Redis image that builds `module/fs.so` and boots Redis with the module loaded.
- `LICENSE`: AGPLv3 project license.
- `Makefile`: top-level orchestration for module, mount, command binaries, install/uninstall, web-dev, and skill installation.
- `README.md`: current product story and user-facing usage docs for the workspace-first AFS model.
- `SKILL.md`: root skill doc that teaches agents how to use AFS.
- `example.afs.config.json`: sample CLI config for the current workspace-first flow.
- `go.mod`: root Go module definition for the workspace/control-plane app.
- `go.sum`: root dependency lockfile for that Go module.

## `cmd/` and `internal/`

### `cmd/afs/`

- `cmd/afs/afs_commands.go`: workspace-facing command handlers such as import and manifest/save helpers.
- `cmd/afs/afs_commands_test.go`: tests command-surface helpers in `afs_commands.go`.
- `cmd/afs/afs_diff.go`: lightweight manifest diff formatting helpers.
- `cmd/afs/afs_grep.go`: `afs grep` implementation, with fast Redis-side and richer client-side search modes.
- `cmd/afs/afs_grep_test.go`: tests grep argument parsing and behavior.
- `cmd/afs/afs_hash.go`: manifest/blob hashing helpers shared by the checkpoint logic.
- `cmd/afs/afs_local.go`: local materialization and workspace-state helpers.
- `cmd/afs/afs_materialize.go`: wrappers that materialize workspace manifests into local directories.
- `cmd/afs/afs_materialize_test.go`: tests materialization helpers.
- `cmd/afs/afs_session_reset.go`: resets a local session by archiving and rematerializing from a checkpoint head.
- `cmd/afs/afs_setup_test.go`: tests setup/config-related flows.
- `cmd/afs/afs_store.go`: thin wrapper around the control-plane Redis store.
- `cmd/afs/afs_store_test.go`: tests store helper behavior.
- `cmd/afs/afs_surface_test.go`: broader CLI-surface tests.
- `cmd/afs/afs_types.go`: type aliases that let the command depend on shared control-plane/worktree types without re-declaring them.
- `cmd/afs/config_commands.go`: `afs config show/set/path` subcommands.
- `cmd/afs/config_commands_test.go`: tests config command behavior.
- `cmd/afs/controlplane_client.go`: adapters that wire CLI config/store values into the control-plane service layer.
- `cmd/afs/main.go`: the main `afs` entrypoint, setup wizard, lifecycle control, mount management, and subcommand routing.
- `cmd/afs/migration_ignore.go`: `.afsignore` and legacy ignore-file loading.
- `cmd/afs/migration_ignore_test.go`: tests ignore-file parsing.
- `cmd/afs/migration_pipeline.go`: older import pipeline that writes directly into the inode-keyed Redis namespace.
- `cmd/afs/migration_pipeline_test.go`: tests that older import pipeline.
- `cmd/afs/mount_backend.go`: chooses and validates the mount backend (`none`, `fuse`, `nfs`, `auto`).
- `cmd/afs/mount_backend_test.go`: tests mount-backend selection.
- `cmd/afs/redis_options.go`: Redis connection option helpers for the CLI.
- `cmd/afs/redis_options_test.go`: tests Redis option handling.
- `cmd/afs/stat_darwin.go`: macOS-specific stat handling.
- `cmd/afs/stat_linux.go`: Linux-specific stat handling.
- `cmd/afs/ui.go`: terminal UI helpers such as spinners, banners, and boxes.
- `cmd/afs/workspace_checkpoint_commands.go`: `workspace` and `checkpoint` subcommand implementations.
- `cmd/afs/workspace_checkpoint_commands_test.go`: tests those subcommands.
- `cmd/afs/workspace_mount_bridge.go`: bridge between control-plane workspace metadata and the canonical live Redis workspace root used by mounts and checkpoint capture.
- `cmd/afs/workspace_mount_bridge_test.go`: tests mount-bridge behavior.
- `cmd/afs/afs_mcp.go`: local-only, untracked stdio MCP server implementation built directly into `afs`.
- `cmd/afs/afs_mcp_test.go`: local-only, untracked tests for the new Go MCP surface.

### `cmd/afs-control-plane/`

- `cmd/afs-control-plane/main.go`: HTTP control-plane server binary that exposes the workspace/checkpoint API used by the web UI.

### `internal/controlplane/`

- `internal/controlplane/config.go`: control-plane-specific config loading and Redis client setup.
- `internal/controlplane/http.go`: HTTP handler for `/healthz` and `/v1/...` workspace, tree, file, restore, and activity routes.
- `internal/controlplane/http_test.go`: tests the HTTP handler.
- `internal/controlplane/service.go`: main domain/service layer for workspaces, checkpoints, audit, file browsing, and restores.
- `internal/controlplane/store.go`: Redis persistence layer for workspace metadata, savepoints, manifests, blobs, and audit records.

### `internal/worktree/`

- `internal/worktree/manifest.go`: scans local trees into manifests and blobs, and materializes manifests back to disk.
- `internal/worktree/materialize.go`: ensures or recreates local working-copy directories.
- `internal/worktree/worktree.go`: local workspace-state file layout under `~/.afs/workspaces/<workspace>/`.

## `docs/`

- `docs/afs-control-plane-api.md`: current shared HTTP contract for CLI cloud mode and the web UI.
- `docs/afs-hybrid-architecture.md`: high-level explanation of the current Redis-canonical-plus-local-working-copy model.
- `docs/afs-phase1-execution-plan.md`: scoped plan for the first workspace-first AFS release.
- `docs/repo-walkthrough.md`: this file.

## `examples/`

- `examples/claude_desktop_config.json`: minimal MCP client config example pointing Claude Desktop at `afs mcp`.
- `examples/codex-settings-migration.md`: step-by-step guide for moving `~/.codex` into a shared AFS workspace.

## `module/`

- `module/Makefile`: builds `fs.so` for the Redis module.
- `module/fs.c`: the Redis module itself, including type registration, RDB persistence, and all `FS.*` command handlers.
- `module/fs.h`: `fsObject` and `fsInode` struct definitions plus constants and limits.
- `module/path.c`: path normalization, basename/parent helpers, joins, and glob matching.
- `module/path.h`: declarations for the path utilities.
- `module/redismodule.h`: vendored Redis Module API header.

## `mount/`

### `mount/` root

- `mount/Makefile`: builds the FUSE and NFS binaries.
- `mount/go.mod`: Go module definition for the mount stack.
- `mount/go.sum`: dependency lockfile for the mount stack.

### `mount/client/`

- `mount/client/client.go`: public re-export of the internal client package for callers outside `mount/internal`.

### `mount/cmd/agent-filesystem-mount/`

- `mount/cmd/agent-filesystem-mount/main.go`: FUSE mount daemon entrypoint.

### `mount/cmd/agent-filesystem-nfs/`

- `mount/cmd/agent-filesystem-nfs/main.go`: NFS server entrypoint that exports one Redis-backed filesystem key.
- `mount/cmd/agent-filesystem-nfs/main_test.go`: tests NFS handle behavior, rename invalidation, and open-handle survival.

### `mount/internal/afsfs/`

- `mount/internal/afsfs/attr.go`: converts client stat data into FUSE attrs.
- `mount/internal/afsfs/dir.go`: FUSE directory operations such as lookup, readdir, mkdir, rename, and rmdir.
- `mount/internal/afsfs/errors.go`: maps client/storage errors to FUSE errno values.
- `mount/internal/afsfs/errors_test.go`: tests errno mapping.
- `mount/internal/afsfs/file.go`: file create/open/read/write/unlink behavior for the FUSE layer.
- `mount/internal/afsfs/fs.go`: root filesystem object, mount options, statfs, getattr, and setattr.
- `mount/internal/afsfs/fs_cache_test.go`: tests FUSE cache invalidation behavior.
- `mount/internal/afsfs/handle.go`: inode-based file-handle IO and lock operations.
- `mount/internal/afsfs/locks_test.go`: tests FUSE record-lock cleanup and behavior.
- `mount/internal/afsfs/symlink.go`: symlink create/read support for the FUSE layer.

### `mount/internal/cache/`

- `mount/internal/cache/cache.go`: small TTL cache used by the client layer.

### `mount/internal/client/`

- `mount/internal/client/client.go`: main filesystem client interface plus constructors.
- `mount/internal/client/glob.go`: glob matching used by the client-side find/grep walk code.
- `mount/internal/client/glob_test.go`: tests the glob matcher.
- `mount/internal/client/keys.go`: Redis key naming for the inode-keyed namespace.
- `mount/internal/client/native_core.go`: core inode/directory/symlink implementation against Redis hashes and dirent sets.
- `mount/internal/client/native_helpers.go`: helper utilities for the native client.
- `mount/internal/client/native_locks.go`: Redis-backed advisory locking implementation.
- `mount/internal/client/native_range.go`: ranged inode read/write/truncate support for open file handles.
- `mount/internal/client/native_test.go`: large behavior suite for the native client.
- `mount/internal/client/native_text.go`: head/tail/lines/wc/insert/replace/delete-lines text operations.
- `mount/internal/client/native_walk.go`: tree/find/grep/copy traversal logic.
- `mount/internal/client/parser.go`: response parsing helpers and compatibility structs.

### `mount/internal/nfsfs/`

- `mount/internal/nfsfs/fs.go`: `go-billy` filesystem adapter over the native client for the NFS server.
- `mount/internal/nfsfs/fs_test.go`: NFS filesystem adapter tests.

### `mount/internal/redisconn/`

- `mount/internal/redisconn/redisconn.go`: shared Redis connection option/TLS helper.
- `mount/internal/redisconn/redisconn_test.go`: tests TLS and option wiring.

## `redisclaw/`

### `redisclaw/` root

- `redisclaw/Makefile`: install/dev/test/e2e helpers for the RedisClaw package.
- `redisclaw/README.md`: explains RedisClaw's OpenClaw-style agent loop, memory model, and sandbox integration.
- `redisclaw/pyproject.toml`: package metadata and dependencies for the RedisClaw Python package.

### `redisclaw/redisclaw/`

- `redisclaw/redisclaw/__init__.py`: package marker and version string.
- `redisclaw/redisclaw/agent.py`: long-running agent loop, session persistence, Anthropic integration, and memory bootstrapping.
- `redisclaw/redisclaw/cli.py`: interactive shell and slash-command UI.
- `redisclaw/redisclaw/memory.py`: markdown-memory manager stored in Agent Filesystem.
- `redisclaw/redisclaw/tools.py`: minimal tool set that talks to the sandbox and AFS.

### `redisclaw/tests/`

- `redisclaw/tests/__init__.py`: test package marker.
- `redisclaw/tests/test_e2e.py`: end-to-end tests across RedisClaw, the sandbox service, and Agent Filesystem.

## `sandbox/`

### `sandbox/` root

- `sandbox/Dockerfile`: builds a container with the mount binary plus sandbox server.
- `sandbox/Makefile`: build/test/clean/docker helpers for the sandbox.
- `sandbox/docker-compose.yml`: local Redis-plus-sandbox composition for end-to-end work.
- `sandbox/entrypoint.sh`: container entrypoint that waits for Redis, mounts AFS, and launches the sandbox server.
- `sandbox/go.mod`: sandbox Go module definition.
- `sandbox/go.sum`: dependency lockfile for the sandbox module.

### `sandbox/cmd/sandbox/`

- `sandbox/cmd/sandbox/main.go`: HTTP or stdio-MCP sandbox server entrypoint.

### `sandbox/cmd/sandbox-cli/`

- `sandbox/cmd/sandbox-cli/main.go`: human CLI for interacting with the sandbox HTTP server.

### `sandbox/internal/api/`

- `sandbox/internal/api/mcp.go`: stdio MCP framing and dispatch for the sandbox.
- `sandbox/internal/api/mcp_tools.go`: MCP tool handlers for launch/read/write/kill/list.
- `sandbox/internal/api/server.go`: HTTP API server for process lifecycle operations.

### `sandbox/internal/executor/`

- `sandbox/internal/executor/monitor.go`: process wait/timeout/read/write/kill/list logic.
- `sandbox/internal/executor/process.go`: process object definitions plus launch/manager setup.

## `scripts/`

- `scripts/compare_grep_times.py`: benchmark helper comparing recursive search speed across two directory trees.
- `scripts/web-dev.sh`: runs `afs-control-plane`, waits for `/healthz`, then launches the UI dev server with the right env vars.

## `skills/`

### `skills/agent-filesystem/`

- `skills/agent-filesystem/SKILL.md`: installable skill for using AFS through MCP, mounts, or direct `redis-cli`.

### `skills/codex-settings-sync/`

- `skills/codex-settings-sync/SKILL.md`: skill for migrating `~/.codex` into AFS and sharing it across machines.
- `skills/codex-settings-sync/assets/.afsignore`: starter ignore file used by the Codex settings sync workflow.

## `tasks/`

- `tasks/001_AGENT_SKILL.md`: planning doc for the agent skill work; still useful as history, but mostly superseded by the current skill files.

## `tests/`

- `tests/__init__.py`: package marker for the Python test suite.
- `tests/append.py`: module test for `FS.APPEND`.
- `tests/bench_afs_grep.py`: benchmark comparing `afs grep` against mounted GNU `grep`.
- `tests/chmod.py`: module test for `FS.CHMOD`.
- `tests/chown.py`: module test for `FS.CHOWN`.
- `tests/cp.py`: module test for `FS.CP`.
- `tests/create-test-memories`: shell helper that generates a directory of realistic markdown "memory" files.
- `tests/deletelines.py`: module test for `FS.DELETELINES`.
- `tests/echo_cat.py`: module tests for `FS.ECHO` and `FS.CAT`.
- `tests/error_handling.py`: edge-case and invalid-input tests.
- `tests/find.py`: module test for `FS.FIND`.
- `tests/glob_patterns.py`: glob matcher coverage for `FS.FIND`.
- `tests/grep.py`: module test for `FS.GREP`.
- `tests/hardening_phase0.py`: regression tests for the hardening items in `BACKLOG.md`.
- `tests/head.py`: module test for `FS.HEAD`.
- `tests/info.py`: module test for `FS.INFO`.
- `tests/insert.py`: module test for `FS.INSERT`.
- `tests/invariants.py`: helper that checks directory listings and parent-child references stay consistent.
- `tests/lifecycle.py`: tests auto-create and auto-delete key lifecycle behavior.
- `tests/lines.py`: module test for `FS.LINES`.
- `tests/ls.py`: module test for `FS.LS`.
- `tests/mkdir.py`: module test for `FS.MKDIR`.
- `tests/mv.py`: module test for `FS.MV`.
- `tests/path_normalize.py`: tests path normalization edge cases.
- `tests/rdb_persistence.py`: module persistence tests across save/reload.
- `tests/replace.py`: module test for `FS.REPLACE`.
- `tests/rm.py`: module test for `FS.RM`.
- `tests/stat.py`: module test for `FS.STAT`.
- `tests/symlinks.py`: module tests for symlink creation and reading.
- `tests/tail.py`: module test for `FS.TAIL`.
- `tests/test.json`: sample AFS config used by some manual/local tests.
- `tests/test_cmd.py`: module test for `FS.TEST`.
- `tests/test_runner.py`: custom Python test harness for the Redis module suite.
- `tests/touch.py`: module test for `FS.TOUCH`.
- `tests/tree.py`: module test for `FS.TREE`.
- `tests/truncate_utimens.py`: module tests for `FS.TRUNCATE` and `FS.UTIMENS`.
- `tests/wc.py`: module test for `FS.WC`.
- `tests/wrong_key_type.py`: verifies `FS.*` commands reject non-filesystem Redis keys.
- `tests/bench/go.mod`: Go module file for the synthetic filesystem benchmark tool.
- `tests/bench/main.go`: benchmark program that compares local filesystem operations with mounted AFS operations.

## `third_party/go-nfs/`

This subtree is vendored upstream code from `github.com/willscott/go-nfs`, used by `mount/cmd/agent-filesystem-nfs`. It is not AFS-authored business logic, but it is part of the checked-in tree.

### Upstream repo metadata

- `third_party/go-nfs/.github/dependabot.yml`: upstream Dependabot config.
- `third_party/go-nfs/.github/workflows/codeql-analysis.yml`: upstream CodeQL workflow.
- `third_party/go-nfs/.github/workflows/go.yml`: upstream Go CI workflow.
- `third_party/go-nfs/CONTRIBUTING.md`: upstream contribution guide.
- `third_party/go-nfs/LICENSE`: upstream license.
- `third_party/go-nfs/README.md`: upstream project README.
- `third_party/go-nfs/SECURITY.md`: upstream security policy.
- `third_party/go-nfs/go.mod`: upstream module file.
- `third_party/go-nfs/go.sum`: upstream dependency lockfile.

### Core library files

- `third_party/go-nfs/cache_helpers.go`: cache utilities used by the upstream server.
- `third_party/go-nfs/conn.go`: connection handling.
- `third_party/go-nfs/errors.go`: NFS protocol error helpers.
- `third_party/go-nfs/file.go`: common file helpers.
- `third_party/go-nfs/filesystem.go`: filesystem abstraction types.
- `third_party/go-nfs/handler.go`: request handler plumbing.
- `third_party/go-nfs/log.go`: logging helpers.
- `third_party/go-nfs/mount.go`: mount protocol support.
- `third_party/go-nfs/mountinterface.go`: mount-side interfaces.
- `third_party/go-nfs/nfs.go`: top-level NFS server/request definitions.
- `third_party/go-nfs/nfs_onaccess.go`: ACCESS handler.
- `third_party/go-nfs/nfs_oncommit.go`: COMMIT handler.
- `third_party/go-nfs/nfs_oncreate.go`: CREATE handler.
- `third_party/go-nfs/nfs_onfsinfo.go`: FSINFO handler.
- `third_party/go-nfs/nfs_onfsstat.go`: FSSTAT handler.
- `third_party/go-nfs/nfs_ongetattr.go`: GETATTR handler.
- `third_party/go-nfs/nfs_onlink.go`: LINK handler.
- `third_party/go-nfs/nfs_onlookup.go`: LOOKUP handler.
- `third_party/go-nfs/nfs_onmkdir.go`: MKDIR handler.
- `third_party/go-nfs/nfs_onmknod.go`: MKNOD handler.
- `third_party/go-nfs/nfs_onpathconf.go`: PATHCONF handler.
- `third_party/go-nfs/nfs_onread.go`: READ handler.
- `third_party/go-nfs/nfs_onreaddir.go`: READDIR handler.
- `third_party/go-nfs/nfs_onreaddirplus.go`: READDIRPLUS handler.
- `third_party/go-nfs/nfs_onreadlink.go`: READLINK handler.
- `third_party/go-nfs/nfs_onremove.go`: REMOVE handler.
- `third_party/go-nfs/nfs_onrename.go`: RENAME handler.
- `third_party/go-nfs/nfs_onrmdir.go`: RMDIR handler.
- `third_party/go-nfs/nfs_onsetattr.go`: SETATTR handler.
- `third_party/go-nfs/nfs_onsymlink.go`: SYMLINK handler.
- `third_party/go-nfs/nfs_onwrite.go`: WRITE handler.
- `third_party/go-nfs/nfs_test.go`: upstream test suite.
- `third_party/go-nfs/nfsinterface.go`: NFS-side interfaces.
- `third_party/go-nfs/server.go`: server loop.
- `third_party/go-nfs/time.go`: protocol time conversions.

### Platform/file helpers

- `third_party/go-nfs/file/file.go`: common file helpers.
- `third_party/go-nfs/file/file_other.go`: non-Unix/Windows fallbacks.
- `third_party/go-nfs/file/file_unix.go`: Unix-specific file behavior.
- `third_party/go-nfs/file/file_wasm.go`: WASM-specific file behavior.
- `third_party/go-nfs/file/file_windows.go`: Windows-specific file behavior.

### Upstream helpers

- `third_party/go-nfs/helpers/cachinghandler.go`: caching handler wrapper.
- `third_party/go-nfs/helpers/cachinghandler_test.go`: tests the caching handler.
- `third_party/go-nfs/helpers/nullauthhandler.go`: null-auth helper.
- `third_party/go-nfs/helpers/memfs/memfs.go`: in-memory test filesystem.
- `third_party/go-nfs/helpers/memfs/storage.go`: storage backing for the in-memory FS.

### Upstream examples

- `third_party/go-nfs/example/helloworld/main.go`: hello-world server example.
- `third_party/go-nfs/example/osnfs/changeos.go`: OS abstraction helper for example code.
- `third_party/go-nfs/example/osnfs/changeos_unix.go`: Unix-specific OS example helper.
- `third_party/go-nfs/example/osnfs/main.go`: example NFS server over the host OS filesystem.
- `third_party/go-nfs/example/osview/main.go`: example browser/viewer over NFS.

## `ui/`

### `ui/` root

- `ui/.gitignore`: ignores UI build output and local package-install state.
- `ui/.npmrc.example`: example npm registry/config override file.
- `ui/README.md`: quick overview for running the AFS UI.
- `ui/eslint.config.js`: ESLint config for the UI.
- `ui/index.html`: Vite HTML shell.
- `ui/package-lock.json`: npm lockfile.
- `ui/package.json`: package scripts and UI dependencies.
- `ui/tsconfig.app.json`: app TypeScript config.
- `ui/tsconfig.json`: base TypeScript config.
- `ui/tsconfig.node.json`: node/tooling TypeScript config.
- `ui/vite.config.ts`: Vite config with React and TanStack Router plugins.
- `ui/vitest.config.ts`: Vitest config.

### `ui/public/`

- `ui/public/favicon.svg`: UI favicon asset.

### `ui/src/components/`

- `ui/src/components/afs-kit.tsx`: the shared styled UI component kit for the AFS product surfaces.

### `ui/src/error-boundaries/`

- `ui/src/error-boundaries/app-error-boundary.tsx`: top-level app error boundary.
- `ui/src/error-boundaries/error-fallback.styles.ts`: shared styles for error fallbacks.
- `ui/src/error-boundaries/error-fallback.tsx`: reusable fallback component.
- `ui/src/error-boundaries/get-error-message.ts`: error-to-message helper.
- `ui/src/error-boundaries/route-error-boundary.tsx`: route-level error boundary.

### `ui/src/foundation/api/`

- `ui/src/foundation/api/afs.test.ts`: tests the demo/API client behavior.
- `ui/src/foundation/api/afs.ts`: AFS client that can run in demo mode or against the HTTP control-plane API.

### `ui/src/foundation/hooks/`

- `ui/src/foundation/hooks/use-afs.ts`: React Query hooks and invalidation helpers for the AFS client.

### `ui/src/foundation/mocks/`

- `ui/src/foundation/mocks/afs.ts`: seeded demo data for the UI's no-backend mode.

### `ui/src/foundation/tables/`

- `ui/src/foundation/tables/workspace-table.styles.ts`: styling for the workspace table.
- `ui/src/foundation/tables/workspace-table.tsx`: workspace summary table component.

### `ui/src/foundation/types/`

- `ui/src/foundation/types/afs.ts`: shared UI types for workspaces, checkpoints, trees, files, and activity.

### `ui/src/layout/`

- `ui/src/layout/app-bar.styles.ts`: styles for the top app bar.
- `ui/src/layout/app-bar.tsx`: top bar component.
- `ui/src/layout/layout.styles.ts`: layout primitives used by the shell.
- `ui/src/layout/navigation-items.ts`: sidebar navigation model and title resolution.
- `ui/src/layout/sidebar.styles.ts`: sidebar styling.
- `ui/src/layout/sidebar.tsx`: collapsible product sidebar.

### `ui/src/routes/`

- `ui/src/routes/__root.tsx`: root TanStack Router layout with sidebar, app bar, and devtools.
- `ui/src/routes/activity.tsx`: global activity/audit page.
- `ui/src/routes/index.tsx`: product overview landing page.
- `ui/src/routes/workspaces.$workspaceId.tsx`: per-workspace studio with overview/files/activity tabs and savepoint controls.
- `ui/src/routes/workspaces.tsx`: workspace catalog plus create/import intake screen.

### `ui/src/`

- `ui/src/index.css`: app-level CSS variables and base styles.
- `ui/src/main.tsx`: React app bootstrap, providers, and router creation.
- `ui/src/routeTree.gen.ts`: generated TanStack Router route tree.

### `ui/src/test/`

- `ui/src/test/setup-tests.ts`: Vitest setup file.

## Current Local-Only Files

These files exist in the worktree but are not yet tracked by Git:

- `cmd/afs/afs_mcp.go`: Go-native MCP server for `afs`.
- `cmd/afs/afs_mcp_test.go`: tests for the Go-native MCP server.

## Legacy Paths Removed In This Worktree

These paths are already removed from the current checkout and are intended to stay removed. Until the deletion is committed, Git will still show them as tracked deletions.

### Old Python package and MCP server

- `agent_filesystem/__init__.py`
- `agent_filesystem/cli.py`
- `agent_filesystem/client.py`
- `agent_filesystem/exceptions.py`
- `agent_filesystem/py.typed`
- `mcp_server/__init__.py`
- `mcp_server/server.py`
- `pyproject.toml`

### Old container/orchestration files for the Python packaging path

- `Dockerfile`
- `Dockerfile.mcp`
- `docker-compose.yml`

### Old planning and test artifacts tied to the Python package / MCP server

- `tasks/002_PYTHON_LIBRARY.md`
- `tasks/003_MCP_SERVER.md`
- `tests/test_agent_filesystem.py`
- `tests/test_mcp_server.py`
- `RELEASE_NOTES.md`

## What I Would Read First To Understand The System

If you want the shortest path to the current architecture, read in this order:

1. `README.md`
2. `docs/afs-hybrid-architecture.md`
3. `cmd/afs/main.go`
4. `internal/controlplane/store.go`
5. `internal/controlplane/service.go`
6. `internal/worktree/manifest.go`
7. `mount/internal/client/native_core.go`
8. `mount/cmd/agent-filesystem-nfs/main.go`
9. `module/fs.c`
10. `ui/src/foundation/api/afs.ts`

That sequence explains the current product model first, then the persistence layers, then the UI and the older Redis-module substrate.
