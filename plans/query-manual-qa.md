# AFS Query Manual QA

Status: complete
Owner: Codex
Created: 2026-05-07
Updated: 2026-05-08

## Goal

Manually QA the new AFS `query` feature across every exposed user surface so we
can answer three questions with evidence:

1. Does the shipped keyword and hybrid-fallback behavior work end to end?
2. Are intentionally unavailable semantic paths reported clearly and
   consistently?
3. Do CLI, control-plane HTTP, MCP, and UI surfaces agree on the same contract,
   defaults, warnings, and recovery guidance?

## Scope

In scope:

- Top-level and workspace-scoped CLI query commands:
  - `afs query`
  - `afs fs <workspace> query`
  - `afs query --keyword`
  - `afs query --semantic`
  - `afs query index <status|rebuild|clean>`
- Query argument parsing and typed query document handling:
  - plain text
  - `expand:`
  - `lex:`
  - `vec:`
  - `hyde:`
  - `intent:`
- Query output formats:
  - plain
  - `--json`
  - `--files`
  - `--paths`
  - `--md`
  - `--csv`
  - `--xml`
- Workspace query config under `afs ws config`, especially
  `query.embeddings.enabled`, `query.embeddings.model`, and
  `query.embeddings.chunkStrategy`
- Query index status and rebuild behavior
- Control-plane HTTP workspace query routes
- Local and hosted MCP `file_query`
- Workspace Studio Search tab behavior in the UI
- Contract/documentation alignment for query-specific docs

Out of scope:

- Tuning ranking relevance beyond obvious correctness failures
- Performance benchmarking beyond basic responsiveness notes
- Implementing fixes
- Broader grep/manual-QA coverage unrelated to query behavior

## Current Observations

Grounded in `main` at `834f311` plus targeted passing tests:

- `go test ./cmd/afs -run Query`
- `go test ./internal/controlplane -run Query`
- `go test ./internal/mcptools -run FileQuery`

Observed implementation shape:

- Hybrid and keyword requests are live and route through control-plane query
  handlers.
- Semantic-only mode is intentionally unavailable today.
- When embeddings are disabled, hybrid plain queries fall back to keyword
  retrieval.
- When embeddings are disabled, `vec:` and `hyde:` typed clauses on hybrid
  queries are expected to warn and degrade to keyword text.
- Query index status and rebuild are implemented; `query index clean` currently
  reports a no-op status rather than removing data.
- The UI now exposes query in Workspace Studio even though the early feature
  plan described UI as out of scope.

These are the expected baselines the QA run must validate.

## Environments

Use the lightest environment that can prove each surface:

- E1 local standalone CLI
  - Redis 8
  - local config
  - at least one seeded workspace
- E2 self-managed control plane
  - control plane running against a disposable database
  - browser-accessible UI
  - ability to hit `/v1/...` routes directly
- E3 MCP clients
  - local stdio MCP against a seeded workspace
  - hosted/control-plane MCP token if hosted MCP parity is in scope for the run

## Fixtures

Seed one workspace with content crafted to expose ranking, scoping, and
typed-clause behavior:

- `/docs/checkpoints.md`
  - checkpoint, savepoint, restore, snapshot language
- `/docs/index.md`
  - text beginning with `Index` to test `query index ...` disambiguation
- `/notes/auth.md`
  - auth, token, tenant scope language
- `/mount/setup.md`
  - NFS, FUSE, mount backend language
- `/archive/checkpoints.md`
  - checkpoint language outside `/docs` to verify path scoping
- one binary or non-text file
  - validate it does not break indexing or query responses

## Scenario Matrix

### QRY-ENV: Environment And Fixture Readiness

- Confirm Redis Search is available in the chosen Redis 8 environment.
- Confirm the seeded workspace is reachable from CLI, HTTP, MCP, and UI.
- Record workspace id, database id, and seed file inventory in the run log.

### QRY-CLI-01: Help And Command Discovery

- Verify `afs query --help` and `afs fs <workspace> query --help`.
- Verify usage mentions:
  - hybrid default
  - `--keyword`
  - `--semantic`
  - typed query clauses
  - supported output formats
  - index subcommands
- Verify natural queries beginning with `index` are not treated as index
  subcommands.

Expected:

- Help matches the documented command family.
- No stale `search` or `vsearch` terminology appears.
- `query index` is only entered when `index` is the actual subcommand.

### QRY-CLI-02: Plain And Typed Query Parsing

- Run a plain natural-language query.
- Run an explicit `expand:` query.
- Run a typed document with `lex:` + `vec:`.
- Run a typed document with `intent:` + `lex:` + `hyde:`.
- Verify trailing flags after query text still parse correctly.
- Verify `--intent` conflicts with `intent:` typed clauses.
- Verify `--keyword` and `--semantic` reject typed documents.
- Verify `--keyword` and `--semantic` together fail cleanly.
- Verify invalid multi-line plain text and malformed typed docs return useful
  parse errors.

Expected:

- Parsed behavior matches the typed-document rules from the tests and MCP parser.
- Failures are actionable, not generic flag parser noise.

### QRY-CLI-03: Result Quality, Path Scoping, And Warnings

- Run hybrid plain-text queries expected to hit `/docs/checkpoints.md`.
- Run scoped queries with `--path /docs`.
- Verify results do not leak from `/archive` when scope is `/docs`.
- Run typed hybrid queries with `vec:` or `hyde:` while embeddings are off.
- Capture stderr warnings for:
  - plain hybrid fallback with embeddings disabled
  - typed semantic-clause degradation to keyword text
- Verify `--min-score`, `--limit`, `--all`, `--candidate-limit`, `--full`, and
  `--explain`.

Expected:

- Relevant files rank first or near first for the seeded dataset.
- Path scoping is enforced.
- Warning semantics differ correctly between plain fallback and typed semantic
  clause fallback.
- `--explain` includes backend/fallback details when requested.

### QRY-CLI-04: Output Formats

- Verify plain output block formatting and line-number rendering.
- Verify empty-result behavior for:
  - plain
  - `--md`
  - `--files`
  - `--paths`
  - `--csv`
  - `--xml`
- Verify `--files` emits QMD-style `#id,score,afs://...`.
- Verify `--paths` deduplicates file paths and omits snippets.
- Verify markdown, CSV, and XML are structurally valid and consistent with the
  same result set.
- Verify overlapping chunks coalesce instead of duplicating adjacent results.

Expected:

- Every format matches the CLI contract encoded by the tests.
- Empty outputs are format-appropriate and stable.

### QRY-CLI-05: Semantic-Only Unavailable Paths

- With embeddings disabled, run `afs query --semantic ...`.
- Enable embeddings in workspace config, then run `afs query --semantic ...`
  again.
- Repeat with `afs fs <workspace> query --semantic ...`.
- Verify both human-readable and JSON behaviors.

Expected:

- With embeddings disabled: the user gets explicit enablement guidance.
- With embeddings enabled: the user gets a clear “not ready yet” or
  unavailable response instead of a silent fallback to keyword.
- JSON mode returns `status: "unavailable"` with empty results and warnings.

### QRY-IDX-01: Query Index Status

- Run `query index status` before any query.
- Run a first query and re-check status.
- Verify status fields:
  - state
  - files
  - ready
  - pending
  - stale
  - unindexed
  - skipped
  - errors
  - chunks
  - embeddings/model/strategy
- Verify path-scoped status checks.

Expected:

- First-run indexing/backfill behavior is visible.
- Status messaging changes appropriately for ready/indexing/needs_rebuild/
  unavailable/error.

### QRY-IDX-02: Query Index Rebuild And Clean

- Run `query index rebuild` without `--wait`.
- Run `query index rebuild --wait`.
- Run path-scoped rebuild.
- Run `query index rebuild --force`.
- Run `query index clean` in plain and JSON modes.

Expected:

- Rebuild returns enqueue/process details consistent with status.
- `--wait` visibly drains pending work.
- `clean` behavior is documented as current no-op behavior if no deletion occurs.
- Any mismatch between help text and actual destructive behavior is recorded.

### QRY-CONFIG-01: Workspace Query Config

- Read existing config with `afs ws config <workspace> list`.
- Toggle `query.embeddings.enabled`.
- Set and unset `query.embeddings.model`.
- Set `query.embeddings.chunkStrategy` to `auto` and `regex`.
- Verify invalid config values fail cleanly.
- Re-run relevant query and index commands after config changes.

Expected:

- Config round-trips through CLI and reflected API/UI surfaces.
- Config changes affect warning and unavailable messaging consistently.

### QRY-API-01: Control-Plane HTTP Contract

- Exercise:
  - `GET /.../query/index/status`
  - `POST /.../query/index/rebuild`
  - `POST /.../query`
- Cover both top-level workspace routes and database-scoped equivalents if both
  are intended to work.
- Verify request/response JSON matches the CLI/MCP contract:
  - defaults
  - warnings
  - explain entries
  - unavailable semantic status
- Verify bad methods and malformed JSON return useful errors.

Expected:

- HTTP behavior is a faithful transport of the shared control-plane logic.
- Query routes are reachable wherever the UI hooks expect them.

### QRY-MCP-01: Local MCP `file_query`

- Call `file_query` with:
  - plain query
  - typed query document in `query`
  - explicit `searches`
  - `mode=keyword`
  - `mode=semantic`
- Verify parser errors and typed-mode restrictions match CLI behavior.
- Compare results and warnings against equivalent CLI commands.

Expected:

- Local MCP and CLI share the same request validation rules.
- Semantic-unavailable behavior is consistent.

### QRY-MCP-02: Hosted MCP `file_query`

- If a hosted token is available, repeat the local MCP checks through hosted
  MCP.
- Verify workspace scoping from the token.
- Verify no control-plane-only data leaks in workspace-scoped responses.

Expected:

- Hosted MCP matches local MCP for the same workspace state.

### QRY-UI-01: Workspace Studio Search Tab

- Load the Search tab for a seeded workspace.
- Verify index status card states:
  - loading
  - ready
  - indexing
  - needs rebuild
  - unavailable
  - error
- Run hybrid, keyword, and semantic-only searches from the UI.
- Verify warning pills, explain badges, empty state, score display, and line
  range labels.
- Verify index rebuild dialog:
  - default path
  - path normalization
  - force toggle
- Verify semantic settings:
  - top-level toggle
  - model field
  - chunk strategy
  - saved notice and error states

Expected:

- UI uses the same query contract as CLI/MCP.
- Semantic-only failures are clearly surfaced.
- UI state refreshes after rebuild/config changes.

### QRY-DOC-01: Contract And Documentation Alignment

- Compare the shipped behavior with:
  - `docs/reference/cli.md`
  - `docs/reference/mcp.md`
  - `docs/reference/control-plane-api.md`
  - `docs/internals/decisions/0001-qmd-inspired-workspace-query.md`
- Record any mismatch between documentation and observed behavior.

Expected:

- Docs either match the product or generate concrete follow-up items.

## Evidence Capture

For every scenario, record:

- environment used
- command/request/tool invocation
- response or screenshot
- whether behavior matches current expected contract
- follow-up issue if not

Store command transcripts and JSON payloads under `.ai/query-qa/` during the
run. Do not commit raw evidence.

## Results

- Executed the CLI, index/config, HTTP, MCP, and UI scenarios against a
  disposable Redis 8 + self-managed control-plane environment.
- Verified that the shipped keyword and hybrid-fallback paths work across every
  exposed surface.
- Verified that semantic-only retrieval remains intentionally unavailable and is
  surfaced in each transport.
- Captured evidence and a summarized run log in `.ai/query-qa/run-log.md`.

Key follow-ups discovered:

- `docs/reference/control-plane-api.md` does not yet document the shipped query
  HTTP routes.
- `query index clean` is described like a cleanup operation but is currently a
  no-op implementation.
- Semantic-unavailable warnings differ across CLI, HTTP, MCP, and UI.
- `docs/reference/cli.md` says existing workspaces backfill on first query, but
  `query index status` can also warm the index before any search command runs.

## Decisions / Blockers

- Treated semantic-only retrieval as intentionally unavailable unless a newer
  build changed the contract.
- Treated `query index clean` as behavior-under-test because the current code
  advertises cleanup language while returning a no-op status.
- Treated UI query coverage as required because the feature is already exposed
  in `workspace-studio/-search-tab.tsx`.

## Verification

Research and grounding completed with:

- `git switch main && git pull --ff-only origin main`
- `go test ./cmd/afs -run Query`
- `go test ./internal/controlplane -run Query`
- `go test ./internal/mcptools -run FileQuery`
- headless Chrome interaction against
  `http://127.0.0.1:8091/workspaces/ws_2de43f4165de4ad0?databaseId=local-development&tab=search`
- local stdio MCP against `query-qa` and `query-fresh`
- hosted `/mcp` token-backed tool calls against `query-qa` and `query-fresh`
- design/docs/code review across:
  - `plans/file-query.md`
  - `docs/internals/decisions/0001-qmd-inspired-workspace-query.md`
  - `docs/reference/cli.md`
  - `docs/reference/mcp.md`
  - `docs/reference/control-plane-api.md`
  - `cmd/afs/afs_query_commands*.go`
  - `cmd/afs/afs_query_index_commands.go`
  - `internal/controlplane/workspace_query.go`
  - `internal/mcptools/query.go`
  - `ui/src/routes/workspace-studio/-search-tab.tsx`
