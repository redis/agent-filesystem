# Protocol for this workspace

This is an LLM-maintained knowledge wiki. Multiple agents may read and write
the same workspace, so keep updates explicit, cited, and easy to review.

## Operating model

- `raw/` is source of truth. Do not rewrite source files.
- `wiki/` is agent-maintained synthesis. Update it when sources or user
  questions create durable knowledge.
- `wiki/index.md` is content-oriented. Update it whenever you create or
  materially change a wiki page.
- `wiki/log.md` is chronological and append-only. Add one entry for every
  ingest, filed answer, or lint pass.

## Before answering a non-trivial question

1. Read `wiki/index.md`.
2. Search `wiki/` for the key nouns in the user's question.
3. Read the most relevant pages and cite their paths in the answer.
4. If the answer is durable, ask whether to file it under `wiki/syntheses/`
   or `wiki/questions/`.

## Ingesting a source

1. Confirm the source path under `raw/inbox/`, `raw/sources/`, or
   `raw/assets/`.
2. Read the source and identify its core claims, entities, dates, and open
   questions.
3. Create or update a page under `wiki/sources/` with the summary and
   provenance.
4. Update relevant pages under `wiki/topics/` and `wiki/entities/`.
5. Note contradictions or stale claims in the affected page's
   `Open questions` or `Contradictions` section.
6. Move processed text sources from `raw/inbox/` to `raw/sources/` only
   when the user explicitly asks you to organize the source folder.
7. Update `wiki/index.md`.
8. Append an entry to `wiki/log.md` using the log format below.

## Page conventions

Use this structure for new topic, entity, and synthesis pages:

    ---
    type: topic | entity | source | synthesis | question
    status: draft | current | stale
    updated: YYYY-MM-DD
    sources:
      - raw/sources/<file-or-url-note>
    ---

    # <Title>

    ## Summary

    ## Key facts

    ## Connections

    ## Open questions

    ## Sources

Prefer wiki links such as `[Retrieval](../topics/retrieval.md)` over vague
references. Keep claims tied to source paths or source summary pages.

## Log format

Append entries to `wiki/log.md` with a parseable heading:

    ## [YYYY-MM-DD] ingest | <Source title>

Use one of these actions: `ingest`, `query`, `lint`, `maintenance`.

## Lint pass

When asked to lint the wiki, check for:

- pages missing sources;
- stale claims superseded by newer sources;
- contradictions between pages;
- orphan pages missing inbound links from `wiki/index.md`;
- important concepts mentioned repeatedly without a page;
- open questions that need a source search.

Report suggested fixes first, then make edits only with user approval unless
the requested lint explicitly includes cleanup.

## Concurrent editing rules

- Prefer focused edits over rewriting whole pages.
- Never delete another agent's work without explaining why in `wiki/log.md`.
- If a page may be contested, add a dated note under `Open questions` rather
  than silently replacing the old claim.
