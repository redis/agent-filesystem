# Shared LLM Wiki (Karpathy style)

A shared knowledge wiki maintained by agents. Humans curate sources and ask
questions; agents read sources, update the wiki, preserve provenance, and keep
the structure healthy over time.

This template is inspired by Andrej Karpathy's LLM wiki idea:
https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f

## Layout

- `AGENTS.md` - the protocol every connected agent follows.
- `raw/` - immutable source material waiting to be processed.
- `wiki/index.md` - content-oriented map of the maintained wiki.
- `wiki/log.md` - append-only timeline of ingests, queries, and lint passes.
- `wiki/topics/` - concept and theme pages.
- `wiki/entities/` - people, orgs, places, products, projects, or objects.
- `wiki/syntheses/` - durable answers, comparisons, and higher-level analysis.
- `wiki/questions/` - active research questions and follow-ups.
- `tools/` - optional helper scripts or search notes.

## First workflow

1. Drop one source into `raw/inbox/`.
2. Ask an agent to ingest it.
3. Review the summary and the touched wiki pages.
4. Keep the resulting pages as durable knowledge for the next agent.

The wiki is meant to compound. Every useful answer can become a page, every
new source can refine existing pages, and every agent sees the same current
state through Agent Filesystem.
