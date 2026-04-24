import type { IconType } from "@redis-ui/icons";
import {
  BotIcon,
  BookOpenIcon,
  FoldersIcon,
  SparklesIcon,
} from "../../components/lucide-icons";
import type { AFSMCPProfile } from "../../foundation/types/afs";

export type TemplateSeedFile = {
  path: string;
  content: string;
};

/**
 * Client-neutral agent skill. The install UI renders this as a Claude Code
 * skill, a Codex skill, a Claude plugin bundle, or plain generic instructions.
 * `{{serverName}}` is substituted with the generated MCP server name.
 */
export type TemplateAgentSkill = {
  skillDescription: string;
  skillBody: string;
  commands?: ReadonlyArray<{
    name: string;
    body: string;
  }>;
};

export type Template = {
  id: string;
  slug: string;
  title: string;
  tagline: string;
  icon: IconType;
  accent: string;
  profile: AFSMCPProfile;
  profileLabel: string;
  summary: readonly string[];
  whyItMatters: string;
  seedFiles: readonly TemplateSeedFile[];
  firstPrompt: string;
  agentSkill?: TemplateAgentSkill;
};

/* -------------------------------------------------------------------------- */
/* Shared Agent Memory                                                        */
/* -------------------------------------------------------------------------- */

const sharedMemoryFiles: TemplateSeedFile[] = [
  {
    path: "README.md",
    content: `# Shared Agent Memory

A shared long-term memory for agents across your team. Every agent that
connects (Claude Code, Codex, Cursor, or any MCP client) reads from and
writes to the same memory, backed by Redis through Agent Filesystem.

## Layout

- \`shared-memory/index.md\` — curated rollup of all learnings, newest first.
- \`shared-memory/entries/YYYY-MM-DD-<slug>.md\` — one file per learning.
- \`AGENTS.md\` — the protocol every agent should follow when using this workspace.

## Why it's interesting

Redis sits behind the workspace, so reads are sub-millisecond and every
agent connected to this workspace sees writes immediately. Nothing to
sync, nothing to pull — just a filesystem that happens to be shared.

## Getting started

See \`AGENTS.md\` for the read/write protocol agents should follow.
`,
  },
  {
    path: "AGENTS.md",
    content: `# Protocol for this workspace

This is a shared memory workspace. Multiple agents, potentially across
multiple machines and users, read and write the same files here.

## Before answering a non-trivial question

1. \`file_grep\` the \`shared-memory/\` tree for any of the key nouns in
   the user's question. If you find relevant entries, cite them in your
   answer and build on them rather than re-deriving the answer.
2. If nothing relevant exists, proceed normally.

## After discovering something durable

A "durable" learning is a non-obvious fact about this codebase, team,
or domain that will still be true next week and that another agent
could reuse. Debugging breadcrumbs and session-specific context do
**not** qualify.

When you find one:

1. Create \`shared-memory/entries/YYYY-MM-DD-<short-slug>.md\` using
   the template below.
2. Append a one-line entry to \`shared-memory/index.md\` under the most
   recent date heading. Keep it under 140 characters.

## Entry template

    ---
    date: YYYY-MM-DD
    agent: <your client, e.g. claude-code or codex>
    ---

    # <Title — a clear, concrete statement>

    **Context.** When or where does this apply?

    **Finding.** What is the durable fact?

    **Sources.** Files, links, or conversations the finding came from.

## Rules that keep concurrent writes safe

- **Never overwrite another agent's entry.** Always write new files,
  never edit existing entries.
- **\`index.md\` is append-only.** Add new lines under the newest date
  heading. Do not rewrite older lines.
- **Slugs must be unique.** Include a short random suffix if in doubt:
  \`2026-04-22-redis-ttl-3f9a.md\`.
`,
  },
  {
    path: "shared-memory/index.md",
    content: `# Memory index

Append new entries below the newest date heading. Keep each line under
140 characters — title plus a one-line summary and a link to the full
entry file.

## 2026-04-22

- [Example learning](entries/2026-04-22-example.md) — the first entry that shipped with this template; explains the expected shape.
`,
  },
  {
    path: "shared-memory/entries/2026-04-22-example.md",
    content: `---
date: 2026-04-22
agent: template
---

# Example memory entry

**Context.** This entry shipped with the Shared Agent Memory template
so the layout is visible to the next agent that connects.

**Finding.** Useful memory entries describe a durable fact plus where
the fact came from. They are not debugging trails or session recaps.
See \`AGENTS.md\` in the workspace root for the full protocol.

**Sources.** The Shared Agent Memory template in the AFS web UI.
`,
  },
];

/* -------------------------------------------------------------------------- */
/* Org Coding Standards                                                       */
/* -------------------------------------------------------------------------- */

const standardsFiles: TemplateSeedFile[] = [
  {
    path: "README.md",
    content: `# Org Coding Standards

A **read-only** source of truth for your team's coding standards.
Every developer's agents mount this workspace and consult it before
writing code. Updates flow through a small set of maintainers.

## Layout

- \`AGENTS.md\` — the protocol agents follow.
- \`standards/languages/<lang>.md\` — per-language rules.
- \`standards/review-checklist.md\` — what to check in every PR.
- \`standards/security.md\` — security rules.
- \`standards/architecture-principles.md\` — org-wide architecture defaults.

## Sharing this workspace

The MCP access token created alongside this workspace has the
\`workspace-ro\` profile. Agents can read the standards but cannot
modify them.

Distribute the token through your team password manager. Every
developer drops it into their MCP client config (see the dialog that
opened when you created this workspace) and their agent immediately
sees the same rules.

## Updating the standards

Edit the files through the AFS web UI, or create a second
\`workspace-rw\` token scoped to maintainers only. The next read from
any developer's agent picks up the change — no redeploy, no cache.
`,
  },
  {
    path: "AGENTS.md",
    content: `# Protocol for this workspace

This workspace contains your organization's coding standards. It is
**read-only** for agents. Your role is to consult these files before
writing code and to cite them in reviews.

## Before writing or modifying code

1. Identify the language and surface you're about to touch.
2. \`file_read\` the matching file under \`standards/languages/\`.
3. \`file_read\` \`standards/architecture-principles.md\` and
   \`standards/security.md\` when relevant.
4. Apply the rules you read. If any rule conflicts with the user's
   request, surface the conflict and ask how to proceed — do not
   silently override the standard.

## When reviewing a PR or diff

1. \`file_read\` \`standards/review-checklist.md\`.
2. Walk each item against the diff. Cite the specific standard file
   and section when you flag something.

## If a standard seems wrong or missing

Do not edit it — this workspace is read-only. Instead, raise it with
the user and point at the specific file and line.
`,
  },
  {
    path: "standards/languages/go.md",
    content: `# Go

Replace this file with your team's Go standards. The template seeds a
minimal starting point.

## Structure

- Follow \`gofmt\` and \`goimports\` output. Do not hand-wrap.
- One concept per package. Resist the urge to create a \`util\` package.

## Error handling

- Wrap errors with \`fmt.Errorf("context: %w", err)\` when crossing a
  layer boundary.
- Never ignore errors without an explicit comment.

## Tests

- Table-driven tests where there is more than one input shape.
- Prefer \`t.TempDir()\` over manual temp-file cleanup.
`,
  },
  {
    path: "standards/languages/typescript.md",
    content: `# TypeScript

Replace this file with your team's TypeScript standards.

## Types

- Prefer \`type\` aliases for unions and discriminated unions, \`interface\`
  for public object contracts that might be extended.
- Never use \`any\`. Use \`unknown\` at boundaries and narrow.

## React components

- Components are functions. No classes.
- Keep prop types colocated with the component file.
- State flows down, events flow up. Avoid shared mutable singletons.

## Async

- Prefer \`async/await\` over raw promise chains.
- Always handle rejection — either \`try/catch\` or route through a
  boundary that does.
`,
  },
  {
    path: "standards/review-checklist.md",
    content: `# Review checklist

For every PR, check each item and cite the specific standard when you
flag something.

- [ ] Scope matches the PR description; no drive-by changes.
- [ ] New code follows the relevant \`standards/languages/*.md\`.
- [ ] No new \`any\`, \`interface{}\`, \`// @ts-ignore\`, or \`//nolint\`
      without a comment explaining why.
- [ ] Tests cover the new behavior and at least one failure mode.
- [ ] Error messages are specific and actionable.
- [ ] No secrets, tokens, or internal URLs in code or comments.
- [ ] Public API changes are documented.
- [ ] Dependencies added are justified; no drive-by adds.
`,
  },
  {
    path: "standards/security.md",
    content: `# Security standards

Replace with your team's rules. Defaults below.

## Secrets

- Never commit secrets. Use the org secret manager.
- Tokens in logs must be redacted.

## Input validation

- Validate all inputs at trust boundaries (HTTP handlers, CLI args,
  external API responses).
- Parameterize all SQL; never string-concatenate queries.

## Authorization

- Default deny. Explicitly allow per endpoint and per action.
- Never trust client-supplied user identity.
`,
  },
  {
    path: "standards/architecture-principles.md",
    content: `# Architecture principles

Replace with your team's principles. Defaults below.

## Simplicity first

- Prefer the simplest thing that works. Abstractions earn their way in
  by being demanded a second time, not predicted.
- Delete unused code. Do not keep it "just in case".

## Observability

- Log at boundaries with structured fields. Log IDs, not payloads.
- Metrics count things that matter to a human, not every function call.

## Failure

- Make failures explicit at boundaries. Don't swallow errors inside
  helpers that can't recover.
- Retries and timeouts are features. Design them in, don't add them after.
`,
  },
];

/* -------------------------------------------------------------------------- */
/* Team Planning Board                                                        */
/* -------------------------------------------------------------------------- */

const teamBoardFiles: TemplateSeedFile[] = [
  {
    path: "README.md",
    content: `# Team Planning Board

A shared whiteboard for a team of humans and agents coordinating on a
project. One spec, one roadmap, a live view of who is working on what,
a place to record completed work, and a place to surface open questions.

## Layout

- \`plan/spec.md\` — the overall goal, scope, non-goals.
- \`plan/roadmap.md\` — phases and milestones.
- \`tasks/backlog.md\` — unstarted work.
- \`tasks/in-progress/<owner>-<slug>.md\` — one file per active task.
- \`tasks/done/<slug>.md\` — completed tasks, appended after finish.
- \`questions/<slug>.md\` — open decisions awaiting an answer.

## How it stays sane with many writers

- **Per-owner subdirs for drafts.** Each person writes under a file
  named for their handle, so two agents never edit the same file.
- **Append-only for shared docs.** \`spec.md\`, \`roadmap.md\`, and
  \`backlog.md\` grow by addition. Use owner markers
  (\`<!-- @handle 2026-04-22 -->\`) when amending.
- **Checkpoints per milestone.** This workspace's MCP profile includes
  \`checkpoint_create\`. Snapshot the board at each milestone so the
  timeline is recoverable.

See \`AGENTS.md\` for the full protocol.
`,
  },
  {
    path: "AGENTS.md",
    content: `# Protocol for this workspace

This workspace coordinates a team. Multiple agents and humans write
here. Follow these rules so you do not step on each other.

## Your handle

On first interaction, ask the user for a short stable handle (for
example \`alice\` or \`bob\`). Remember it for the session and use it in
every task file you create or update.

## Claiming a task

1. Look in \`tasks/backlog.md\` for a line that matches the user's goal.
2. \`file_write\` \`tasks/in-progress/<handle>-<slug>.md\` with:

       ---
       owner: <handle>
       started: YYYY-MM-DD
       progress: 0%
       status: active
       ---

       # <Task title>

       **Goal.** One paragraph.

       **Plan.** Bulleted steps.

       **Log.** Append dated bullets as you work.

3. Remove the matching line from \`tasks/backlog.md\` (edit the file,
   delete only that line).

## Making progress

Append a dated bullet to the **Log** section of your task file after
each meaningful step. Update the \`progress:\` front-matter field.

## Finishing a task

1. Move the content to \`tasks/done/<slug>.md\`.
2. Delete the in-progress file with \`file_delete_lines\` + \`file_write\`
   replacement, or simply \`file_write\` to an empty string.
3. If \`checkpoint_create\` is available to you, snapshot the workspace
   with a name like \`milestone-<slug>\`.

## Recording a question

\`file_write\` \`questions/<slug>.md\` with:

    ---
    asked-by: <handle>
    date: YYYY-MM-DD
    status: open
    ---

    # <Question>

    Answer here once resolved.

## Never

- Never edit another owner's in-progress task file.
- Never rewrite \`spec.md\` or \`roadmap.md\` — append new sections.
`,
  },
  {
    path: "plan/spec.md",
    content: `# Project spec

Replace the sample text with your real spec. Keep this file
append-only — add new sections rather than rewriting existing ones,
and use owner markers when amending.

## Goal

<What are we building, in one sentence?>

## Scope

- <In-scope item 1>
- <In-scope item 2>

## Non-goals

- <Out-of-scope item 1>

## Success criteria

- <Measurable outcome 1>
- <Measurable outcome 2>

<!-- @handle YYYY-MM-DD - example amend marker -->
`,
  },
  {
    path: "plan/roadmap.md",
    content: `# Roadmap

Phases and milestones. Snapshot the workspace (\`checkpoint_create\`)
each time you finish a milestone so the timeline is recoverable.

## Phase 1 — <name>

- **Milestone 1.1** <description> — target YYYY-MM-DD
- **Milestone 1.2** <description> — target YYYY-MM-DD

## Phase 2 — <name>

- **Milestone 2.1** <description> — target YYYY-MM-DD
`,
  },
  {
    path: "tasks/backlog.md",
    content: `# Backlog

Unstarted work, one line per task. When you claim a task, delete the
line and create \`tasks/in-progress/<handle>-<slug>.md\`.

- Example task — short description of the work to do.
- Another example task — different shape of work.
`,
  },
  {
    path: "tasks/in-progress/README.md",
    content: `# In-progress tasks

One file per active task, named \`<owner-handle>-<slug>.md\`.
See \`AGENTS.md\` in the workspace root for the task file format.
`,
  },
  {
    path: "tasks/done/README.md",
    content: `# Completed tasks

One file per completed task. Keep the history — do not delete files
from this directory. Past tasks are the easiest way for agents to
learn how your team approaches work.
`,
  },
  {
    path: "questions/README.md",
    content: `# Open questions

One file per question awaiting a decision. Close a question by
appending the answer and flipping \`status: open\` to \`status: resolved\`
in the front matter. Do not delete resolved questions — the history is
useful.
`,
  },
];

/* -------------------------------------------------------------------------- */
/* LLM Wiki                                                                   */
/* -------------------------------------------------------------------------- */

const llmWikiFiles: TemplateSeedFile[] = [
  {
    path: "README.md",
    content: `# Shared LLM Wiki (Karpathy style)

A shared knowledge wiki maintained by agents. Humans curate sources and ask
questions; agents read sources, update the wiki, preserve provenance, and keep
the structure healthy over time.

This template is inspired by Andrej Karpathy's LLM wiki idea:
https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f

## Layout

- \`AGENTS.md\` - the protocol every connected agent follows.
- \`raw/\` - immutable source material waiting to be processed.
- \`wiki/index.md\` - content-oriented map of the maintained wiki.
- \`wiki/log.md\` - append-only timeline of ingests, queries, and lint passes.
- \`wiki/topics/\` - concept and theme pages.
- \`wiki/entities/\` - people, orgs, places, products, projects, or objects.
- \`wiki/syntheses/\` - durable answers, comparisons, and higher-level analysis.
- \`wiki/questions/\` - active research questions and follow-ups.
- \`tools/\` - optional helper scripts or search notes.

## First workflow

1. Drop one source into \`raw/inbox/\`.
2. Ask an agent to ingest it.
3. Review the summary and the touched wiki pages.
4. Keep the resulting pages as durable knowledge for the next agent.

The wiki is meant to compound. Every useful answer can become a page, every
new source can refine existing pages, and every agent sees the same current
state through Agent Filesystem.
`,
  },
  {
    path: "AGENTS.md",
    content: `# Protocol for this workspace

This is an LLM-maintained knowledge wiki. Multiple agents may read and write
the same workspace, so keep updates explicit, cited, and easy to review.

## Operating model

- \`raw/\` is source of truth. Do not rewrite source files.
- \`wiki/\` is agent-maintained synthesis. Update it when sources or user
  questions create durable knowledge.
- \`wiki/index.md\` is content-oriented. Update it whenever you create or
  materially change a wiki page.
- \`wiki/log.md\` is chronological and append-only. Add one entry for every
  ingest, filed answer, or lint pass.

## Before answering a non-trivial question

1. Read \`wiki/index.md\`.
2. Search \`wiki/\` for the key nouns in the user's question.
3. Read the most relevant pages and cite their paths in the answer.
4. If the answer is durable, ask whether to file it under \`wiki/syntheses/\`
   or \`wiki/questions/\`.

## Ingesting a source

1. Confirm the source path under \`raw/inbox/\`, \`raw/sources/\`, or
   \`raw/assets/\`.
2. Read the source and identify its core claims, entities, dates, and open
   questions.
3. Create or update a page under \`wiki/sources/\` with the summary and
   provenance.
4. Update relevant pages under \`wiki/topics/\` and \`wiki/entities/\`.
5. Note contradictions or stale claims in the affected page's
   \`Open questions\` or \`Contradictions\` section.
6. Move processed text sources from \`raw/inbox/\` to \`raw/sources/\` only
   when the user explicitly asks you to organize the source folder.
7. Update \`wiki/index.md\`.
8. Append an entry to \`wiki/log.md\` using the log format below.

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

Prefer wiki links such as \`[Retrieval](../topics/retrieval.md)\` over vague
references. Keep claims tied to source paths or source summary pages.

## Log format

Append entries to \`wiki/log.md\` with a parseable heading:

    ## [YYYY-MM-DD] ingest | <Source title>

Use one of these actions: \`ingest\`, \`query\`, \`lint\`, \`maintenance\`.

## Lint pass

When asked to lint the wiki, check for:

- pages missing sources;
- stale claims superseded by newer sources;
- contradictions between pages;
- orphan pages missing inbound links from \`wiki/index.md\`;
- important concepts mentioned repeatedly without a page;
- open questions that need a source search.

Report suggested fixes first, then make edits only with user approval unless
the requested lint explicitly includes cleanup.

## Concurrent editing rules

- Prefer focused edits over rewriting whole pages.
- Never delete another agent's work without explaining why in \`wiki/log.md\`.
- If a page may be contested, add a dated note under \`Open questions\` rather
  than silently replacing the old claim.
`,
  },
  {
    path: "wiki/index.md",
    content: `# Wiki index

Start every non-trivial query here, then drill into the relevant pages.
Update this file whenever pages are created or materially changed.

## Overview

- [Wiki operating model](topics/wiki-operating-model.md) - how this workspace
  turns raw sources into maintained markdown knowledge.

## Sources

- [Karpathy LLM wiki idea](sources/karpathy-llm-wiki.md) - seed source note
  for the pattern this template instantiates.

## Topics

- [Wiki operating model](topics/wiki-operating-model.md) - raw sources,
  maintained wiki pages, and agent protocol.

## Entities

No entity pages yet.

## Syntheses

No synthesis pages yet.

## Open questions

- [Search and scale](questions/search-and-scale.md) - when should this wiki
  add a search layer beyond \`wiki/index.md\`?
`,
  },
  {
    path: "wiki/log.md",
    content: `# Wiki log

Append-only timeline of ingests, queries, lint passes, and maintenance.

## [2026-04-23] maintenance | Template initialized

- Created the starter LLM wiki structure.
- Seeded the first source note, operating model topic, and scale question.
- Next step: add one source to \`raw/inbox/\` and ask an agent to ingest it.
`,
  },
  {
    path: "wiki/sources/karpathy-llm-wiki.md",
    content: `---
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
- Read \`wiki/index.md\` first, then drill into relevant pages.
- Keep \`wiki/log.md\` as an append-only history of ingests and queries.

## Connections

- [Wiki operating model](../topics/wiki-operating-model.md)
- [Search and scale](../questions/search-and-scale.md)

## Open questions

- What source types will this workspace ingest first?
- At what size should this wiki add a dedicated markdown search tool?

## Sources

- Karpathy gist: https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f
`,
  },
  {
    path: "wiki/topics/wiki-operating-model.md",
    content: `---
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

- \`raw/\` contains source material. It should preserve provenance and remain
  stable after ingestion.
- \`wiki/\` contains agent-written markdown pages that can evolve as new
  sources arrive.
- \`AGENTS.md\` defines the rules for ingestion, query answering, logging, and
  maintenance.
- \`wiki/index.md\` is the navigation map.
- \`wiki/log.md\` is the chronological record.

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
`,
  },
  {
    path: "wiki/questions/search-and-scale.md",
    content: `---
type: question
status: draft
updated: 2026-04-23
sources:
  - wiki/sources/karpathy-llm-wiki.md
---

# Search and scale

## Question

When should this wiki add search tooling beyond \`wiki/index.md\`?

## Current answer

Use \`wiki/index.md\` while the wiki is small enough for an agent to read and
navigate directly. Add a markdown search tool once the index becomes too large,
when source volume grows faster than manual curation, or when agents repeatedly
miss relevant pages during query answering.

## Signals to watch

- \`wiki/index.md\` becomes too long to scan comfortably.
- Agents answer from partial context and miss obvious related pages.
- Source documents are too large to read in one pass during ingestion.
- Lint passes regularly find orphan pages or duplicated concepts.

## Sources

- [Karpathy LLM wiki idea](../sources/karpathy-llm-wiki.md)
`,
  },
  {
    path: "raw/inbox/README.md",
    content: `# Inbox

Drop new source files here before asking an agent to ingest them.

Examples:

- clipped articles as markdown;
- paper notes;
- meeting transcripts;
- exported chat logs;
- links collected into a markdown note.

Agents should treat files here as source material, not as wiki pages.
`,
  },
  {
    path: "raw/sources/README.md",
    content: `# Sources

Processed source files can live here once the user asks to organize the raw
folder. Keep filenames stable so wiki pages can cite them.
`,
  },
  {
    path: "raw/assets/README.md",
    content: `# Assets

Images, PDFs, and other non-markdown source assets can live here. When an
agent uses an asset, it should cite the path from the relevant wiki page.
`,
  },
  {
    path: "wiki/entities/README.md",
    content: `# Entities

Create one page per important person, organization, product, project, place,
or object. Keep links back to source summaries and related topic pages.
`,
  },
  {
    path: "wiki/syntheses/README.md",
    content: `# Syntheses

File durable answers here: comparisons, briefs, argument maps, timelines, and
other outputs that should outlive the chat where they were created.
`,
  },
  {
    path: "tools/README.md",
    content: `# Tools

Optional helper scripts and notes for operating the wiki. Start with plain
markdown and \`wiki/index.md\`; add tools only when the wiki outgrows them.
`,
  },
];

const llmWikiAgentSkill: TemplateAgentSkill = {
  skillDescription:
    "Use when answering non-trivial questions in a project connected to the {{serverName}} MCP server (tools prefixed mcp__{{serverName}}__*). Read wiki/index.md first, search wiki/ for key nouns, cite relevant wiki paths, and offer to file durable answers back into the Shared LLM Wiki (Karpathy style). When ingesting sources from raw/, update source/topic/entity pages plus wiki/index.md and append to wiki/log.md.",
  skillBody: `# Shared LLM Wiki (Karpathy style) — agent protocol

This skill operates a shared LLM-maintained knowledge wiki backed by Agent
Filesystem (MCP server \`{{serverName}}\`). The workspace has immutable raw
sources under \`raw/\`, maintained wiki pages under \`wiki/\`, and a protocol
file at \`AGENTS.md\`.

## Before answering a non-trivial question

1. Read \`wiki/index.md\` with \`mcp__{{serverName}}__file_read\`.
2. Search \`wiki/\` for the key nouns using
   \`mcp__{{serverName}}__file_grep\`.
3. Read the most relevant pages.
4. Answer with citations to wiki paths.
5. If the answer is durable, ask whether to file it under
   \`wiki/syntheses/\` or \`wiki/questions/\`.

## Ingesting a source

1. Confirm the source path under \`raw/\`.
2. Read the source.
3. Create or update a source note under \`wiki/sources/\`.
4. Update relevant pages under \`wiki/topics/\` and \`wiki/entities/\`.
5. Update \`wiki/index.md\`.
6. Append a parseable entry to \`wiki/log.md\`:
   \`## [YYYY-MM-DD] ingest | <Source title>\`.

## Maintenance

When asked to lint the wiki, check for missing sources, stale claims,
contradictions, orphan pages, missing concept pages, and unanswered questions.
Report suggested fixes first unless the user explicitly asks you to clean them
up.
`,
  commands: [
    {
      name: "wiki-ingest",
      body: `---
description: Ingest one source from the shared LLM wiki workspace.
---

Ingest the source path the user provides into the shared LLM wiki.

1. Read \`AGENTS.md\` and \`wiki/index.md\`.
2. Read the source from \`raw/\`.
3. Create or update the matching source note under \`wiki/sources/\`.
4. Update affected topic/entity pages.
5. Update \`wiki/index.md\`.
6. Append to \`wiki/log.md\` with an \`ingest\` heading.

Source: $ARGUMENTS
`,
    },
    {
      name: "wiki-lint",
      body: `---
description: Health-check the shared LLM wiki.
---

Run a lint pass over the shared LLM wiki.

Check for pages missing sources, stale claims, contradictions, orphan pages,
important concepts without pages, and open questions that need a source search.
Report findings first. Make edits only if the user explicitly asks for cleanup.

Scope: $ARGUMENTS
`,
    },
  ],
};

/* -------------------------------------------------------------------------- */
/* Agent skill specs                                                          */
/* -------------------------------------------------------------------------- */

const sharedMemoryAgentSkill: TemplateAgentSkill = {
  skillDescription:
    "Use when answering any non-trivial question in a project connected to the {{serverName}} MCP server (tools prefixed mcp__{{serverName}}__*). Before answering, grep shared-memory/ for prior learnings on the question's key nouns — cite and build on any hits rather than re-derive. After discovering a durable fact (non-obvious, still true next week, reusable by another agent — NOT debug breadcrumbs or session recaps), record it as a new dated entry under shared-memory/entries/ and append a one-line pointer to shared-memory/index.md. Skip for trivial or session-specific questions.",
  skillBody: `# Shared Agent Memory — read/write protocol

This skill reads and writes a shared long-term memory workspace backed by
Agent Filesystem (MCP server \`{{serverName}}\`). Multiple agents across
machines and users read and write the same files. Follow the protocol below
so concurrent writes stay safe.

## Before answering a non-trivial question

1. Call \`mcp__{{serverName}}__file_grep\` against \`shared-memory/\` for the
   key nouns in the user's question.
2. If hits look relevant, read the entry with \`mcp__{{serverName}}__file_read\`,
   cite it in your answer, and build on it instead of re-deriving.
3. If nothing relevant, proceed normally.

Skip this entirely for trivial questions, quick follow-ups, or anything
already answered in the active conversation.

## After discovering a durable fact

A **durable** fact is non-obvious, still true next week, and reusable by
another agent. Debugging breadcrumbs, session recaps, and one-off context
do **not** qualify.

When you find one:

1. Pick a slug. Format: \`YYYY-MM-DD-<short-slug>\`. If unsure about
   collisions, append a random 4-char suffix (e.g.
   \`2026-04-22-redis-ttl-3f9a\`).
2. Write \`shared-memory/entries/<slug>.md\` with
   \`mcp__{{serverName}}__file_write\` using this template:

   \`\`\`
   ---
   date: YYYY-MM-DD
   agent: <client-name>
   ---

   # <Title — a clear concrete statement>

   **Context.** When or where does this apply?

   **Finding.** What is the durable fact?

   **Sources.** Files, links, or conversations the finding came from.
   \`\`\`

3. Append a one-line pointer to \`shared-memory/index.md\` under the most
   recent date heading. Use \`mcp__{{serverName}}__file_insert\` — never
   rewrite existing lines. Keep it under 140 chars:
   \`- [Title](entries/<slug>.md) — one-line summary.\`

## Concurrency rules — do not break these

- **Never overwrite another agent's entry.** Always write new files.
- **\`index.md\` is append-only.** Add new lines under the newest date
  heading. Do not rewrite older lines.
- **Slugs must be unique.** Add a random suffix if in doubt.

## Escape hatches

When slash commands are available, the user can explicitly invoke
\`/memory-search <query>\` or \`/memory-record <title>\` to bypass
auto-behavior.
`,
  commands: [
    {
      name: "memory-search",
      body: `---
description: Search shared memory for entries matching a query. Use when you want an explicit lookup without asking a question.
---

Search the shared memory workspace for entries matching the user's query.

1. Call \`mcp__{{serverName}}__file_grep\` with pattern = the user's query,
   path = \`shared-memory/\`.
2. For each promising hit, read the full entry with
   \`mcp__{{serverName}}__file_read\`.
3. Report the findings: for each entry, show title + context + finding +
   the file path. If nothing matched, say so explicitly.

Query: $ARGUMENTS
`,
    },
    {
      name: "memory-record",
      body: `---
description: Force-record a durable learning into shared memory, bypassing the auto-skill's judgment on durability.
---

Record a new entry in shared memory. Use the title the user provides.

1. Ask the user for Context, Finding, and Sources if they didn't provide
   them in the command arguments.
2. Pick a slug from the title: kebab-case, prefixed with today's date
   (YYYY-MM-DD). If unsure about collisions, append a 4-char random suffix.
3. Write \`shared-memory/entries/<slug>.md\` via
   \`mcp__{{serverName}}__file_write\` using the standard entry template
   (frontmatter: date, agent; sections: Context, Finding, Sources).
4. Append a one-line pointer to \`shared-memory/index.md\` under the most
   recent date heading via \`mcp__{{serverName}}__file_insert\`.

Title: $ARGUMENTS
`,
    },
  ],
};

const codingStandardsAgentSkill: TemplateAgentSkill = {
  skillDescription:
    "Use before writing, modifying, or reviewing code when the {{serverName}} MCP server is connected. Read the relevant standards files live from standards/, cite the specific path you used, and treat the workspace as read-only.",
  skillBody: `# Org Coding Standards — read-only protocol

This skill reads your organization's coding standards from the Agent
Filesystem workspace exposed as MCP server \`{{serverName}}\`. The token is
read-only, so use this workspace as a live source of truth rather than a place
to edit rules.

## Before writing or modifying code

1. Identify the language and surface you are about to touch.
2. Read \`standards/languages/<language>.md\` with
   \`mcp__{{serverName}}__file_read\` when a matching file exists.
3. Read \`standards/architecture-principles.md\` and
   \`standards/security.md\` when the task crosses module boundaries, handles
   user input, touches secrets, or changes authorization behavior.
4. Apply the rules you read and cite the specific standard path when it affects
   the plan or implementation.

## When reviewing code

1. Read \`standards/review-checklist.md\`.
2. Check the diff against the applicable language, architecture, and security
   files.
3. Lead with findings, and cite the standard path behind each concern.

## If a standard seems wrong or missing

Do not edit this workspace through the MCP tools. Tell the user which file or
section is missing and ask whether a maintainer should update the standard.
`,
};

const teamBoardAgentSkill: TemplateAgentSkill = {
  skillDescription:
    "Use when coordinating project work through the {{serverName}} MCP server: reading the spec, claiming tasks, updating in-progress work, recording done work, or writing open questions. Preserve per-owner task files and append-only shared docs.",
  skillBody: `# Team Planning Board — coordination protocol

This skill coordinates a shared project board stored in Agent Filesystem and
exposed as MCP server \`{{serverName}}\`. Multiple humans and agents may write
to this workspace, so keep ownership clear and avoid broad rewrites.

## First step in a session

If you do not already know the user's handle, ask for a short stable handle
such as \`alice\` or \`frontend\`. Use that handle in every task file you
create or update.

## Before planning work

1. Read \`plan/spec.md\` and \`plan/roadmap.md\`.
2. Read \`tasks/backlog.md\`.
3. List \`tasks/in-progress/\` with
   \`mcp__{{serverName}}__file_list\` at depth 2 so you do not duplicate active
   work.

## Claiming a task

1. Pick the matching backlog line with the user.
2. Create \`tasks/in-progress/<handle>-<slug>.md\` with owner, date, progress,
   goal, plan, and log sections.
3. Remove only the claimed line from \`tasks/backlog.md\`.

## Making progress

Append dated notes to your own in-progress file. Do not edit another owner's
in-progress file. Shared files such as \`plan/spec.md\`,
\`plan/roadmap.md\`, and \`questions/*.md\` should grow by focused additions
instead of rewrites.

## Finishing work

1. Move the completed task content to \`tasks/done/<slug>.md\`.
2. Clear the in-progress file after verifying the done file exists.
3. If \`checkpoint_create\` is available, create a milestone checkpoint with a
   short descriptive name.
`,
};

/* -------------------------------------------------------------------------- */
/* Template registry                                                          */
/* -------------------------------------------------------------------------- */

export const templates: readonly Template[] = [
  {
    id: "shared-agent-memory",
    slug: "shared-memory",
    title: "Shared Agent Memory",
    tagline:
      "A collective long-term memory your whole team of agents can read and write.",
    icon: BotIcon,
    accent: "#6366f1",
    profile: "workspace-rw",
    profileLabel: "Read / write",
    summary: [
      "One memory, many agents, any machine",
      "Append-only entries keep concurrent writes safe",
      "Works with Claude Code, Codex, Cursor, or any MCP client",
    ],
    whyItMatters:
      "Redis-backed filesystem means every agent sees learnings in real time, with sub-millisecond reads. Agent A's discovery from yesterday is Agent B's starting point today — no copy-paste, no syncing.",
    seedFiles: sharedMemoryFiles,
    firstPrompt:
      "What do you already know about this workspace? Grep shared-memory/ and summarize.",
    agentSkill: sharedMemoryAgentSkill,
  },
  {
    id: "shared-llm-wiki-karpathy",
    slug: "shared-llm-wiki",
    title: "Shared LLM Wiki (Karpathy style)",
    tagline:
      "A shared agent-maintained knowledge wiki inspired by Karpathy's LLM wiki: https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f",
    icon: BookOpenIcon,
    accent: "#f97316",
    profile: "workspace-rw",
    profileLabel: "Read / write",
    summary: [
      "Raw sources stay immutable while agents maintain the wiki",
      "Index and log files make knowledge compound across sessions",
      "Works as a shared workspace for multiple agents",
    ],
    whyItMatters:
      "Instead of rediscovering knowledge from raw documents on every question, agents compile useful findings into a durable markdown wiki. The next agent starts from the maintained synthesis, with source notes, links, and a chronological log already in place.",
    seedFiles: llmWikiFiles,
    firstPrompt:
      "Read AGENTS.md and wiki/index.md, then tell me how you would ingest the first source into this wiki.",
    agentSkill: llmWikiAgentSkill,
  },
  {
    id: "org-coding-standards",
    slug: "coding-standards",
    title: "Org Coding Standards",
    tagline:
      "Read-only source of truth for your team's coding standards and review rules.",
    icon: BookOpenIcon,
    accent: "#0ea5e9",
    profile: "workspace-ro",
    profileLabel: "Read-only",
    summary: [
      "Every agent reads the same canonical standards",
      "Update once — every developer's agent sees it immediately",
      "Read-only MCP token means agents can't clobber the rules",
    ],
    whyItMatters:
      "Coding standards go stale the moment they live in a Notion page nobody reads. Here they live in a workspace your agents consult automatically, gated by a read-only MCP profile so nothing can edit them by accident.",
    seedFiles: standardsFiles,
    firstPrompt:
      "Summarize every standard in this workspace in one paragraph each.",
    agentSkill: codingStandardsAgentSkill,
  },
  {
    id: "team-planning-board",
    slug: "team-board",
    title: "Team Planning Board",
    tagline:
      "Shared spec, in-flight tasks, completed work, and open questions — for humans and agents coordinating together.",
    icon: FoldersIcon,
    accent: "#22c55e",
    profile: "workspace-rw-checkpoint",
    profileLabel: "Read / write + checkpoints",
    summary: [
      "One workspace coordinates a whole team",
      "Per-owner subdirs avoid write conflicts",
      "Checkpoint at each milestone — full history recoverable",
    ],
    whyItMatters:
      "Kanban boards are great at state but terrible at context. This template keeps state (tasks, owners, progress) and context (spec, roadmap, open questions) in the same shared filesystem, navigable by every agent on your team.",
    seedFiles: teamBoardFiles,
    firstPrompt:
      "Read plan/spec.md and tasks/backlog.md, then help me claim the next task.",
    agentSkill: teamBoardAgentSkill,
  },
  {
    id: "blank",
    slug: "blank",
    title: "Blank Workspace",
    tagline: "Start empty and shape it as you go.",
    icon: SparklesIcon,
    accent: "#94a3b8",
    profile: "workspace-rw",
    profileLabel: "Read / write",
    summary: [
      "No seed files, no protocol — just a workspace",
      "Reach for this when your use case doesn't fit the others",
      "You can always add shape later",
    ],
    whyItMatters:
      "Sometimes you just want a fresh Redis-backed workspace to experiment in. Pick this, connect your agent, go.",
    seedFiles: [],
    firstPrompt: "What shall we build in this workspace?",
  },
] as const;

export function findTemplate(id: string): Template | undefined {
  return templates.find((template) => template.id === id);
}

/* -------------------------------------------------------------------------- */
/* Setup prompt generator                                                     */
/* -------------------------------------------------------------------------- */

export function buildSetupPrompt(template: Template, workspaceName: string) {
  if (template.seedFiles.length === 0) {
    return `You're connected to a fresh Agent Filesystem workspace named "${workspaceName}" via MCP. The file_read, file_write, file_list, file_grep, file_replace, file_insert, file_delete_lines, file_patch, file_lines, and file_glob tools reach its contents. The workspace is currently empty.

Suggest three ways we could use this workspace based on what I'm working on. Wait for me to pick one before creating any files.`;
  }

  const intro = `I've connected you to an Agent Filesystem workspace named "${workspaceName}" via MCP. The file_read, file_write, file_list, file_grep, file_replace, file_insert, file_delete_lines, file_patch, file_lines, and file_glob tools reach its contents.

Initialize this workspace as "${template.title}" — ${template.tagline}

Use file_write to create each of the files below exactly as shown. The content for each file is the block between the opening "<<<FILE: path>>>" marker and the matching "<<<END>>>" marker.`;

  const fileBlocks = template.seedFiles
    .map(
      (file) => `<<<FILE: ${file.path}>>>
${file.content.trimEnd()}
<<<END>>>`,
    )
    .join("\n\n");

  const outro = `Once every file is written, run file_list on the workspace root and on each subdirectory you created, then give me a one-paragraph summary of the layout.

From then on, follow the protocol in AGENTS.md for this and every future session pointed at this workspace. When the user is ready, suggest they try:

> ${template.firstPrompt}`;

  return `${intro}

${fileBlocks}

${outro}`;
}
