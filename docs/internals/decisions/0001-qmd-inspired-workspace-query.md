# 0001 QMD-Inspired Workspace Query

Status: accepted
Date: 2026-05-07

## Context

AFS already has deterministic content search through `grep` and MCP
`file_grep`. That is the right interface when an agent knows the exact text,
glob, or regex to find. It is not the right interface for conceptual questions
such as "how do checkpoints work?" or QMD-style typed retrieval using lexical
and semantic clauses.

The candidate command surfaces were separate `search`, `vsearch`, and `query`
verbs, or one broader `query` command with narrower flags. Four public search
verbs made the CLI harder to explain.

## Decision

AFS exposes two public search verbs:

- `grep` for exact deterministic evidence.
- `query` for ranked retrieval.

`query` is the recommended hybrid surface. `query --keyword` selects
keyword-ranked retrieval and `query --semantic` selects vector-only semantic
retrieval. QMD-style typed documents use `lex:`, `vec:`, `hyde:`, and
`intent:` on the default `query` mode.

Workspace embedding settings live under `afs ws config`, for example
`query.embeddings.enabled`. Operational vector-index commands live under
`afs query index`.

MCP mirrors this split with `file_grep` and `file_query`.

## Consequences

Agents have one exact-search tool and one ranked-search tool, which keeps the
choice teachable. Plain `query` can fall back to keyword-ranked results when
embeddings are disabled or unavailable. Semantic-only retrieval remains gated
by the workspace embedding config and vector backend availability.

The vector backend should use Redis Search chunk documents with path filters
and explainable hybrid ranking. VectorSets are not the primary backend because
AFS needs rich metadata filters, lexical search, hybrid ranking, and result
explanation in the same retrieval model.

Keyword query uses the same projection pattern first: one derived HASH per text
chunk, indexed by RediSearch BM25 when available. Canonical file bytes remain
in the AFS content backend, including Redis Array storage when supported; the
projection is rebuildable and maintained asynchronously so file writes stay
fast.
