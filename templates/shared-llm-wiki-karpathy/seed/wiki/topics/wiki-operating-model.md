---
type: topic
status: current
updated: 2026-04-23
sources:
  - wiki/sources/karpathy-llm-wiki.md
---

# Wiki operating model

## Summary

This workspace has three layers: immutable raw sources, maintained wiki pages,
and an agent protocol. The value comes from compiling knowledge into the wiki
as sources are added, so future agents and future questions start from the
existing synthesis instead of rediscovering everything.

## Key facts

- `raw/` contains source material. It should preserve provenance and remain
  stable after ingestion.
- `wiki/` contains agent-written markdown pages that can evolve as new
  sources arrive.
- `AGENTS.md` defines the rules for ingestion, query answering, logging, and
  maintenance.
- `wiki/index.md` is the navigation map.
- `wiki/log.md` is the chronological record.

## Connections

- [Karpathy LLM wiki idea](../sources/karpathy-llm-wiki.md)
- [Search and scale](../questions/search-and-scale.md)

## Open questions

- Which categories matter for this domain: entities, concepts, projects,
  claims, timelines, or decisions?
- Should answers created during chat be filed automatically or only after
  explicit user approval?

## Sources

- [Karpathy LLM wiki idea](../sources/karpathy-llm-wiki.md)
