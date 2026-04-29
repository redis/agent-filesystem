---
description: Ingest one source from the shared LLM wiki workspace.
---

Ingest the source path the user provides into the shared LLM wiki.

1. Read `AGENTS.md` and `wiki/index.md`.
2. Read the source from `raw/`.
3. Create or update the matching source note under `wiki/sources/`.
4. Update affected topic/entity pages.
5. Update `wiki/index.md`.
6. Append to `wiki/log.md` with an `ingest` heading.

Source: $ARGUMENTS
