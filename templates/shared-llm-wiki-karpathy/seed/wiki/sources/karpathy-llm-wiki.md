---
type: source
status: current
updated: 2026-04-23
sources:
  - https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f
---

# Karpathy LLM wiki idea

## Summary

The source describes a pattern for personal or team knowledge bases where an
agent incrementally maintains a structured markdown wiki. Instead of relying
only on query-time retrieval from raw documents, the agent reads new sources
once, extracts durable knowledge, updates relevant pages, and records what
changed.

## Key ideas

- Keep raw sources immutable and separate from the maintained wiki.
- Let agents own the bookkeeping: summaries, links, entity pages, topic pages,
  contradictions, and logs.
- Use an agent-facing protocol file to define the workspace conventions.
- Read `wiki/index.md` first, then drill into relevant pages.
- Keep `wiki/log.md` as an append-only history of ingests and queries.

## Connections

- [Wiki operating model](../topics/wiki-operating-model.md)
- [Search and scale](../questions/search-and-scale.md)

## Open questions

- What source types will this workspace ingest first?
- At what size should this wiki add a dedicated markdown search tool?

## Sources

- Karpathy gist: https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f
