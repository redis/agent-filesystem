---
type: question
status: draft
updated: 2026-04-23
sources:
  - wiki/sources/karpathy-llm-wiki.md
---

# Search and scale

## Question

When should this wiki add search tooling beyond `wiki/index.md`?

## Current answer

Use `wiki/index.md` while the wiki is small enough for an agent to read and
navigate directly. Add a markdown search tool once the index becomes too large,
when source volume grows faster than manual curation, or when agents repeatedly
miss relevant pages during query answering.

## Signals to watch

- `wiki/index.md` becomes too long to scan comfortably.
- Agents answer from partial context and miss obvious related pages.
- Source documents are too large to read in one pass during ingestion.
- Lint passes regularly find orphan pages or duplicated concepts.

## Sources

- [Karpathy LLM wiki idea](../sources/karpathy-llm-wiki.md)
