# QMD-Inspired Workspace Query

Status: active
Owner: rowan / Codex
Created: 2026-05-06
Updated: 2026-05-07

## Goal

Add QMD-inspired workspace query surfaces that let humans and agents search for
exact text, semantic meaning, and hybrid conceptual matches across AFS
workspaces.

`file_grep` remains the deterministic text-search tool for exact strings,
regex, glob matching, counts, and line-oriented matches. `file_query` becomes
the ranked retrieval tool for lexical search, local vector similarity, typed
query documents, and hybrid search. The CLI keeps this as two commands:
`grep` for exact text and `query` for ranked retrieval. Narrow query modes use
flags instead of separate verbs.

The design is inspired by Tobi Lutke's QMD command split:

- `qmd search` -> lexical ranked search.
- `qmd vsearch` -> vector-only semantic search.
- `qmd embed` -> generate or refresh local embeddings.
- `qmd query` -> recommended hybrid search.
- QMD typed query documents: `lex:`, `vec:`, `hyde:`, and `intent:`.
- QMD plain `query` text is an implicit `expand:` query; explicit `expand:`
  cannot be mixed with typed lines.

AFS keeps the public model crisp:

- `grep` -> exact deterministic evidence.
- `query` -> recommended hybrid retrieval.
- `query --keyword` -> BM25 keyword-only retrieval.
- `query --semantic` -> vector-only semantic retrieval.

## Scope

In scope:

- Add a new MCP `file_query` tool for local and hosted MCP.
- Add CLI commands:
  - `afs grep`
  - `afs query`
  - `afs query --keyword`
  - `afs query --semantic`
  - `afs fs <workspace> query`
  - `afs fs <workspace> query --keyword`
  - `afs fs <workspace> query --semantic`
- Add a workspace config surface, modeled after top-level `afs config`, so
  workspace settings use one key-based API instead of one subcommand family per
  feature.
- Add vector-index operations under `query index` for status, rebuild, and
  cleanup.
- Use Redis Search / `FT.SEARCH` as the canonical search backend, with
  `FT.HYBRID` as an optimization where available.
- Store embeddings as chunk-level Redis HASH documents indexed by RediSearch
  vector fields.
- Use a local GGUF embedding model by default, matching QMD's local-first
  posture.
- Let users enable or disable vector embeddings per workspace.
- Enqueue embedding work from control-plane materialization and sync/mount write
  paths without blocking file writes.
- Provide status, rebuild, stale-index, and explain/profiling surfaces.

Out of scope:

- Replacing or weakening `afs fs grep` / MCP `file_grep`.
- Redis VectorSets as the primary backend.
- Hosted cloud embeddings as the default.
- Full LLM reranking in the first production slice.
- UI search surfaces. This plan is CLI/MCP/backend-first.

## User Interface

### CLI

Keep existing deterministic grep unchanged:

```bash
afs fs repo grep "DirtyHint"
afs fs repo grep -E "error|warning" --path /internal
afs fs repo grep -l -i "disk full"
```

Add QMD-style query commands:

```bash
afs grep "DirtyHint"
afs query "how does auth attach tenant scope to a workspace?"
afs query --keyword "auth subject workspace"
afs query --semantic "how does the UI know a workspace has unsaved changes?"
afs query "expand: how does auth attach tenant scope to a workspace?"
afs fs repo query "how does auth attach tenant scope to a workspace?"
afs fs repo query --semantic "how does the UI know a workspace has unsaved changes?"
```

Support typed query documents for the recommended hybrid command:

```bash
afs fs repo query $'lex: "dirty marker" workspace\nvec: how does the UI detect unsaved workspace changes?'
afs fs repo query $'intent: AFS live mount setup\nlex: "mount backend"\nvec: where does setup choose between NFS and FUSE?'
afs fs repo query $'hyde: The setup command stores a selected live mount backend in local configuration.'
```

Shared query options:

```bash
--path <workspace-path>
-n, --limit <num>
--all
--min-score <num>
--json
--files
--md
--full
--line-numbers
--explain
--candidate-limit <n>
--no-rerank
--keyword
--semantic
--intent <text>
--chunk-strategy <regex|auto>
```

Workspace config:

```bash
afs ws config repo list
afs ws config repo get query.embeddings.enabled
afs ws config repo set query.embeddings.enabled true
afs ws config repo set query.embeddings.model embeddinggemma
afs ws config repo set query.embeddings.chunkStrategy auto
afs ws config repo unset query.embeddings.model
```

Vector index operations:

```bash
afs query index status
afs query index rebuild --wait
afs query index rebuild --force
afs query index rebuild --path /cmd/afs
afs query index clean
afs fs repo query index status
```

### MCP

Keep `file_grep` exact and deterministic.

Add `file_query`:

```json
{
  "workspace": "repo",
  "path": "/",
  "query": "how does auth attach tenant scope?",
  "searches": [
    { "type": "lex", "query": "\"auth subject\" workspace" },
    { "type": "vec", "query": "how bearer tokens map to tenant scoped workspaces" }
  ],
  "intent": "AFS control-plane auth",
  "limit": 10,
  "min_score": 0.2,
  "rerank": "auto",
  "explain": false
}
```

Result shape:

```json
{
  "matches": [
    {
      "path": "/internal/controlplane/http.go",
      "start_line": 120,
      "end_line": 168,
      "score": 0.82,
      "source": "hybrid",
      "snippet": "...",
      "explain": {
        "lex_rank": 2,
        "vector_rank": 5,
        "rrf_score": 0.031
      }
    }
  ],
  "index": {
    "model": "embeddinggemma",
    "embedding_state": "ready",
    "pending_files": 0,
    "stale_files": 0
  }
}
```

Add a lightweight status tool only if it proves necessary after the first MCP
implementation. Prefer including enough index status in `file_query` errors and
responses first.

## Architecture

### Packages

Add focused shared packages so CLI, local MCP, and hosted MCP use the same
logic:

- `internal/querysearch`: public query orchestration, typed query parsing,
  result shaping, RRF fusion, explain traces.
- `internal/vectorindex`: Redis key builders, RediSearch vector index creation,
  chunk upserts/deletes, vector KNN search.
- `internal/embedding`: local embedding engine interface and model config.
- `internal/chunking`: text/code chunking with line spans and chunk previews.

Keep existing `internal/searchindex` for deterministic grep candidate indexing.
Add lexical ranked search support there only if it can be done without coupling
grep behavior to semantic query behavior.

### Redis Data Model

AFS now has two derived search projections beside canonical file content:

- `grep` projection on inode HASHes for exact literal candidate discovery.
  Key pattern: `afs:{fsKey}:inode:<inodeID>`, indexed by
  `afs:idx:{fsKey}:v1`, with `grep_grams_ci` and file metadata fields.
- `query` keyword projection on chunk HASHes for BM25 ranked retrieval.
  Key pattern:
  `afs:{fsKey}:qchunk:<inodeID>:<contentHash>:<seq>`, indexed by
  `afs:qidx:{fsKey}:v1`, with `text`, `preview`, path filters, line spans, and
  content identity.

Canonical file bytes remain in the filesystem content backend (`ARRAY` when
available, `STRING` fallback). RediSearch indexes the HASH projection because
the search module indexes HASH/JSON documents, not Redis Array payloads. The
projection is derived and rebuildable.

Do not store vectors on inode HASHes. Store one HASH per embedded chunk:

```text
afs:{fsKey}:vchunk:<modelDigest>:<inodeID>:<contentHash>:<seq>
  type=chunk
  path=/cmd/afs/afs_grep.go
  path_ancestors=/cmd,/cmd/afs
  inode_id=<inode>
  content_hash=<sha256>
  model=<model id>
  seq=<chunk seq>
  pos=<byte offset>
  start_line=<line>
  end_line=<line>
  preview=<short text>
  embedding=<float32 bytes>
```

Create one RediSearch vector index per workspace/model digest:

```text
FT.CREATE afs:vidx:{fsKey}:<modelDigest>:v1
ON HASH PREFIX 1 afs:{fsKey}:vchunk:<modelDigest>:
SCHEMA
  type TAG
  path TAG
  path_ancestors TAG SEPARATOR ","
  inode_id TAG
  content_hash TAG
  model TAG
  seq NUMERIC
  pos NUMERIC
  start_line NUMERIC
  end_line NUMERIC
  preview TEXT NOSTEM
  embedding VECTOR HNSW 6 TYPE FLOAT32 DIM <dim> DISTANCE_METRIC COSINE
```

Use the `{fsKey}` hash tag on all vector keys so Redis Cluster colocates a
workspace's vector data.

### Embedding Engine

Use a local model by default, matching QMD's approach:

- Default model: `embeddinggemma-300M-Q8_0.gguf`.
- Default cache directory: `~/.cache/afs/models`.
- Environment override:

```bash
AFS_EMBED_MODEL="hf:ggml-org/embeddinggemma-300M-GGUF/embeddinggemma-300M-Q8_0.gguf"
AFS_EMBED_PROVIDER=local
```

Initial interface:

```go
type Engine interface {
    Name() string
    ModelID() ModelID
    EmbedDocuments(ctx context.Context, chunks []ChunkForEmbedding) ([][]float32, error)
    EmbedQuery(ctx context.Context, query string) ([]float32, error)
}
```

Testing uses a deterministic fake embedder. If the local model is unavailable,
default `query` falls back to keyword-only results with a clear warning, and
`query --semantic` returns a structured "embeddings unavailable" error.

### Chunking

Start with a deterministic regex/line-aware chunker:

- Target around 900 tokens or a conservative byte approximation.
- 15% overlap.
- Preserve `start_line`, `end_line`, and byte offsets.
- Avoid splitting inside fenced code blocks where practical.
- Include title/path context in the embedding input, e.g. `path | heading |
  chunk`.

Add `--chunk-strategy auto` after the first slice to use AST-aware boundaries
for Go, TypeScript, JavaScript, Python, and Rust, inspired by QMD's tree-sitter
mode.

### Write Path

File writes and imports must not synchronously chunk or embed.

Current keyword projection flow:

1. Write canonical content and inode metadata.
2. Update the grep projection fields in the inode HASH.
3. Mark the file stale with `query_state=stale`.
4. Add the inode id to `afs:{fsKey}:query_index:dirty`.
5. Return to the caller.

The async query worker claims dirty inode ids, re-reads current file content,
skips unsupported/binary/oversized files, writes `qchunk` HASH projection
documents, deletes stale chunk documents, and marks the inode `query_state`
`ready`, `skipped`, or `error`. Empty file creation does not enqueue query
work; later writes/truncates mark the file dirty.

Future vector flow on control-plane materialization/import and mount/sync
writes:

1. Write file content and existing search metadata.
2. Compute content hash.
3. Mark inode vector state as `pending` for the active model.
4. Enqueue a deduplicated embedding job.
5. Return to the caller.

Suggested queue keys:

```text
afs:{fsKey}:vector_pending:<modelDigest>
afs:{fsKey}:vector_events
afs:{fsKey}:vector_meta:<modelDigest>
```

The worker:

1. Claims pending inode/path.
2. Re-reads current inode metadata and content.
3. Skips binary files and files over the configured vector indexing cap.
4. Chunks content.
5. Embeds chunks in batches.
6. Deletes stale chunks for the inode/model.
7. Upserts chunk HASHes with vector bytes.
8. Marks the inode `vector_state=ready`, `skipped`, or `error`.

Renames can update chunk path metadata without re-embedding. Deletes should
delete chunks by inode id. Full workspace restore/root replacement can clear the
vector namespace and enqueue a rebuild.

### Query Flow

Lexical retrieval inside `query`:

1. Parse lexical query.
2. Query RediSearch text fields.
3. Return ranked chunk/file results with snippets.

`query --semantic`:

1. Embed query locally.
2. Run RediSearch vector KNN with path filters.
3. Return ranked chunk results.

Default `query`:

1. Parse query document, explicit `expand:`, or single natural-language query.
2. If single query, use it as the original search with optional local expansion
   in a later phase.
3. Run lexical and vector retrieval in parallel.
4. Fuse results using reciprocal rank fusion, with extra weight for the first
   query clause and exact lexical hits.
5. Return top chunks/files with snippets and explain traces.
6. Later: add local reranking behind `--no-rerank` / `rerank:auto`.

Prefer `FT.HYBRID` when the deployed Redis supports it. Keep a manual fallback
using separate `FT.SEARCH` lexical and vector queries plus Go-side RRF.

## Checklist

### Phase 1 - Contracts and shared query core

- [x] Add this plan to `plans/`.
- [x] Add workspace config request/response structs and key validation for
      `afs ws config <workspace> get|set|list|unset`.
- [x] Add query embedding config keys:
      `query.embeddings.enabled`, `query.embeddings.model`, and
      `query.embeddings.chunkStrategy`.
- [x] Define `file_query` MCP request/response structs in a shared package.
- [x] Define CLI option structs for `query` and its `--keyword` / `--semantic`
      modes.
- [x] Define CLI option structs for vector-index management.
- [x] Implement QMD-style query parsing for `expand:`, `lex:`, `vec:`,
      `hyde:`, and `intent:`.
- [x] Add unit tests for query parsing, invalid mixed query documents, balanced
      quote checks, and line constraints.
- [x] Add result structs with stable JSON field names.

### Phase 2 - Redis vector index

- [x] Add `internal/queryindex` key builders for keyword chunk projection.
- [x] Add RediSearch BM25 chunk index creation and capability detection.
- [x] Add chunk HASH upsert/delete operations for keyword projection.
- [x] Add dirty-set processing for text files, binary skips, unsupported
      document/container skips, and deleted inode cleanup.
- [x] Route CLI, HTTP, local MCP, and hosted MCP through the shared query
      service.
- [ ] Add `internal/vectorindex` key builders.
- [ ] Add RediSearch vector index creation and capability detection.
- [ ] Add chunk HASH upsert/delete operations.
- [ ] Add vector KNN query with path/path ancestor filtering.
- [ ] Add manual lexical/vector RRF fusion helpers.
- [ ] Add tests using a fake Redis or integration Redis where RediSearch vector
      support is available.

### Phase 3 - Local embedding engine

- [ ] Add `internal/embedding` engine interface.
- [ ] Add deterministic fake embedder for tests.
- [ ] Add local GGUF model config and model identity/dimension handling.
- [ ] Add model cache path and environment overrides.
- [ ] Add clear errors for missing model/runtime.
- [ ] Add model-change detection that requires `query index rebuild --force`.

### Phase 4 - Chunking

- [ ] Add line-aware chunker with overlap and fenced-code handling.
- [ ] Include path/title context in embedding input.
- [ ] Add chunk preview extraction.
- [ ] Add chunk tests for Markdown, Go, empty files, large files, and binary
      detection.
- [ ] Add `--chunk-strategy auto` as a no-op alias or feature flag until
      AST-aware chunking lands.

### Phase 5 - Embedding worker and write hooks

- [ ] Add embedding pending queue and metadata keys.
- [ ] Gate enqueue/rebuild behavior on workspace config
      `query.embeddings.enabled`.
- [x] Hook control-plane workspace materialization/import to enqueue keyword
      query projection work.
- [x] Hook mount/sync write paths to enqueue keyword query projection work after
      content changes.
- [x] Add query projection delete and rename handling.
- [ ] Hook control-plane workspace materialization/import to enqueue vector work.
- [ ] Hook mount/sync write paths to enqueue vector work after content changes.
- [ ] Add vector projection delete and rename handling.
- [x] Add `afs fs <workspace> query index rebuild` to process pending keyword
      projection work.
- [x] Add `afs fs <workspace> query index status`, `rebuild`, and `clean`.
- [x] Ensure writes never block on chunking, model download, or embedding
      inference.

### Phase 6 - CLI query commands

- [x] Add top-level `afs grep` as the active-workspace shorthand for existing
      deterministic grep behavior.
- [x] Add `afs fs <workspace> query`.
- [x] Add `afs query --keyword` and `afs query --semantic`.
- [x] Add top-level `afs query` active-workspace shorthand.
- [x] Accept shared output options: `--json`, `--files`, `--md`, `--full`,
      `--line-numbers`, `--explain`, `--limit`, `--all`, and `--min-score`.
- [ ] Add profile timings similar to `AFS_GREP_PROFILE`.
- [x] Add user-friendly fallback messages for missing/stale embeddings.

### Phase 7 - MCP tools

- [x] Add local MCP `file_query`.
- [x] Add hosted MCP `file_query`.
- [x] Keep `file_grep` documentation pointed at deterministic text search.
- [x] Update MCP tool descriptions so agents choose `file_query` for concepts
      and `file_grep` for exact occurrences.
- [x] Add MCP tests for lexical, vector-unavailable fallback, typed queries,
      path scoping, and JSON result shape.

### Phase 8 - Hybrid and explain quality

- [ ] Add RRF explain traces.
- [ ] Weight the first typed search clause higher, matching QMD's pattern.
- [ ] Boost exact lexical hits when they rank highly.
- [ ] Add `--candidate-limit`.
- [ ] Add `--no-rerank` as a forward-compatible option.
- [ ] Evaluate whether local reranking is worth enabling by default after the
      retrieval-only version is stable.

### Phase 9 - Documentation and cleanup

- [x] Update `README.md` with the new query command family.
- [ ] Update `docs/reference/control-plane-api.md` if hosted MCP/API contracts
      change.
- [x] Update MCP setup docs with `file_query` examples.
- [x] Add a short ADR under `docs/internals/decisions/` for QMD-inspired
      hybrid search, local embeddings, and RediSearch over VectorSets.
- [ ] Remove any stale entries from `plans/future-work.md` if this work lands.

## In Flight

- First implementation slice landed: `afs ws config <workspace>
  get|set|unset|list` with JSON output, versioning keys, and query embedding
  keys.
- Retired the old `afs ws versioning` CLI path so versioning now follows the
  workspace config API shape.
- Second implementation slice landed: shared `file_query` contract structs,
  typed QMD-style query parsing, and `afs query` CLI skeletons with `--keyword`
  / `--semantic` modes, structured unavailable JSON, and clear
  embedding-disabled messages.
- Third implementation slice landed: plain `afs query` and
  `afs query --keyword` now return keyword-ranked workspace results, with typed
  `lex:`, `vec:`, and `hyde:` clauses falling back to keyword text when vector
  retrieval is unavailable.
- Fourth implementation slice landed: `afs query index status|rebuild|clean`
  and `afs fs <workspace> query index ...` exist as the vector-index operations
  surface. Status reflects workspace embedding config; rebuild reports the
  vector worker as unavailable until the embedding backend lands.
- Fifth implementation slice landed: local and hosted MCP expose `file_query`
  beside deterministic `file_grep`, using the shared query request/response and
  keyword-ranking fallback.
- Sixth implementation slice landed: `internal/queryindex` maintains a
  chunk-level HASH projection for BM25 keyword query. Control-plane
  materialization and mount/sync writes mark changed files dirty; local sync,
  FUSE, and NFS daemons run the async worker; CLI, HTTP, local MCP, and hosted
  MCP all use `Service.QueryWorkspace`. If RediSearch is unavailable, query
  falls back to direct keyword ranking without requiring embeddings.
- Seventh implementation slice landed: `afs query index status --json` reports
  keyword projection health (`files`, `ready`, `pending`, `stale`, `unindexed`,
  `skipped`, `errors`, `chunks`, and RediSearch availability). `afs query index
  rebuild --wait` enqueues existing files for backfill. First query also
  opportunistically backfills unindexed existing files once per workspace.

## Decisions / Blockers

- **Separate tool boundary.** `file_query` is a new ranked retrieval surface.
  `file_grep` remains deterministic and line-oriented.
- **Two CLI verbs.** Use `grep` and `query`: exact evidence and powerful
  ranked retrieval. `query` defaults to hybrid retrieval; `--keyword` and
  `--semantic` select narrower modes.
- **Vector management is split by responsibility.** Enable, disable, and
  model/chunk settings live under `afs ws config`; rebuild, status, and clean
  commands live under `query index`.
- **Workspace settings use one key-based API.** New query/embedding settings
  should go through `afs ws config <workspace> get|set|list|unset`, not a dedicated
  `afs ws query` command.
- **Redis Search first.** Use chunk HASHes plus RediSearch vector fields.
  VectorSets are not the primary backend because AFS needs rich path filters,
  lexical search, hybrid ranking, and explainability.
- **Derived projections.** Canonical file bytes stay in the filesystem content
  backend. Grep and query maintain rebuildable HASH projections because
  RediSearch cannot index Redis Array values directly.
- **Local-first embeddings.** Default to a local GGUF embedding model,
  conceptually matching QMD's `embeddinggemma-300M-Q8_0` setup. Cloud/provider
  embeddings can be added later behind explicit configuration.
- **Async indexing.** File writes enqueue embedding work and return. Search
  status must make staleness visible.
- **Reranking later.** Preserve `--no-rerank` / `rerank:auto` API space, but
  avoid making local reranking a first-slice dependency.
- **Redis capability detection.** Need runtime detection for RediSearch vector
  support and `FT.HYBRID`. Manual `FT.SEARCH` + Go RRF is the compatibility
  path.

## Verification

- [x] `make commands`
- [x] `make test`
- [x] Targeted CLI tests under `./cmd/afs/...`
- [x] Targeted control-plane tests under `./internal/controlplane/...`
- [x] Targeted vector/query package unit tests.
- [x] `go test ./internal/queryindex`
- [x] `go test ./internal/controlplane ./internal/mcptools ./internal/queryindex ./internal/querysearch ./internal/searchindex`
- [x] `go test ./cmd/afs`
- [x] `cd mount && go test ./internal/client ./cmd/agent-filesystem-mount ./cmd/agent-filesystem-nfs`
- [x] `cd mount && go test ./...` after write-hook changes.
- [x] MCP local and hosted tool tests pass.
- [ ] Manual smoke:

```bash
afs ws config getting-started set query.embeddings.enabled true
afs fs getting-started query index rebuild --force --wait
afs fs getting-started query --semantic "how do I save a snapshot?"
afs fs getting-started query "how do checkpoints work?"
afs fs getting-started query $'lex: checkpoint\nvec: how do I save a snapshot?'
```

Current smoke evidence from the implementation workspace:

```bash
./afs query daemon
./afs query --keyword daemon
./afs query --semantic daemon
./afs query index status --json
./afs fs testgrep query --json daemon
./afs fs testgrep query index status --json
```

- [x] Confirm `afs fs grep` and MCP `file_grep` behavior is unchanged.
- [x] Confirm vector search degrades cleanly when embeddings or RediSearch
      vector support are unavailable.

## Result

The usable first version is implemented: CLI and MCP now have the final
two-verb search model (`grep` for exact evidence, `query` for ranked
retrieval), workspace config owns embedding settings, and vector-index
operations have a stable command surface. Keyword-ranked query works today;
semantic/vector retrieval remains explicitly gated on the future embedding
worker and Redis vector index implementation.
