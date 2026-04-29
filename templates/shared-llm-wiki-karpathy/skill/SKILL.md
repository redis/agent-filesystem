---
name: {{skillName}}
description: "Use when answering non-trivial questions in a project connected to the {{serverName}} MCP server (tools prefixed {{toolPrefix}}*). Read wiki/index.md first, search wiki/ for key nouns, cite relevant wiki paths, and offer to file durable answers back into the Shared LLM Wiki (Karpathy style). When ingesting sources from raw/, update source/topic/entity pages plus wiki/index.md and append to wiki/log.md."
---

# Shared LLM Wiki (Karpathy style) — agent protocol

This skill operates a shared LLM-maintained knowledge wiki backed by Agent
Filesystem (MCP server `{{serverName}}`). The workspace has immutable raw
sources under `raw/`, maintained wiki pages under `wiki/`, and a protocol
file at `AGENTS.md`.

## Before answering a non-trivial question

1. Read `wiki/index.md` with `{{toolPrefix}}file_read`.
2. Search `wiki/` for the key nouns using
   `{{toolPrefix}}file_grep`.
3. Read the most relevant pages.
4. Answer with citations to wiki paths.
5. If the answer is durable, ask whether to file it under
   `wiki/syntheses/` or `wiki/questions/`.

## Ingesting a source

1. Confirm the source path under `raw/`.
2. Read the source.
3. Create or update a source note under `wiki/sources/`.
4. Update relevant pages under `wiki/topics/` and `wiki/entities/`.
5. Update `wiki/index.md`.
6. Append a parseable entry to `wiki/log.md`:
   `## [YYYY-MM-DD] ingest | <Source title>`.

## Maintenance

When asked to lint the wiki, check for missing sources, stale claims,
contradictions, orphan pages, missing concept pages, and unanswered questions.
Report suggested fixes first unless the user explicitly asks you to clean them
up.
