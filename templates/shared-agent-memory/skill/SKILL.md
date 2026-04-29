---
name: {{skillName}}
description: "Use when Codex has access to the {{serverName}} AFS shared-memory MCP server, or when the user asks to check shared memory, prior cross-agent context, durable learnings, memory entries, workspace memory, saved commands, acronyms, aliases, shorthand such as CPP/CCP, /memory-search, or /memory-record. Before any non-trivial answer in a connected project, fresh-search /shared-memory for the task's key nouns, read relevant entries, and build on them instead of re-deriving. After discovering a durable reusable fact, write a new dated memory entry and append a pointer to /shared-memory/index.md. Skip trivial, purely conversational, or session-only tasks."
---

# AFS Shared Memory Protocol

This skill tells Codex how to use a shared Agent Filesystem workspace as
long-term memory across agents, machines, and sessions.

The MCP server provides the tools. This skill provides the behavior.

## Tool Discovery

Expected MCP server name: `{{serverName}}`.

In Codex, MCP tool namespaces may normalize hyphens to underscores. The
expected tool prefix for this workspace is `{{toolPrefix}}`, but always use
the live tool namespace exposed in the current session.

If the tools are unavailable, say that shared memory is not connected and
continue without pretending to have checked it.

## Read Before Answering

For every non-trivial task in a connected project:

1. Extract 3 to 7 key nouns from the user's request: repo names, commands,
   files, product terms, errors, feature names, or short aliases.
2. Fresh-search `/shared-memory` with `{{toolPrefix}}file_grep`. Do this even
   if prior context suggests a result, because other agents may have updated
   the workspace.
3. For very short prompts such as acronyms or saved commands, search the exact
   token first before interpreting it.
4. Read relevant hits with `{{toolPrefix}}file_read` or
   `{{toolPrefix}}file_lines`.
5. Use memory as prior context, not unquestioned truth. Verify drift-prone
   facts when cheap.
6. Briefly mention memory-derived context when it materially affects the
   answer.

Skip the search for trivial requests, quick rewrites, or questions already
answered in the active conversation.

## Write After Learning

Write a memory only for a durable fact.

A durable fact is:

- non-obvious;
- likely still true next week;
- useful to another agent;
- grounded in a source, file, command, user preference, or decision.

Do not write memories for:

- temporary debug breadcrumbs;
- one-off command output;
- broad session summaries;
- guesses;
- facts that should instead live in repo docs.

## Entry Format

Create a new file:

`/shared-memory/entries/YYYY-MM-DD-short-slug.md`

Use this shape:

```markdown
---
date: YYYY-MM-DD
agent: codex
---

# Clear Concrete Title

**Context.** When this applies.

**Finding.** The durable reusable fact.

**Sources.** Files, commands, links, or conversation context that support it.

**Keywords.** Search terms future agents are likely to use.
```

For saved user commands or acronyms, include the exact token in the title,
filename, finding, and `Keywords` field.

## Index Update

After writing an entry:

1. Read `/shared-memory/index.md`.
2. If today's date heading exists, insert the pointer under that heading.
3. If today's date heading does not exist, insert a new date heading after the
   intro paragraph.
4. Never rewrite older entries.
5. Never use a hardcoded line number without first reading the current index.

Pointer format:

```markdown
- [Clear title](entries/YYYY-MM-DD-short-slug.md) - short searchable summary.
```

## Concurrency Rules

- Never overwrite another agent's entry.
- Always create a new entry file.
- Treat `/shared-memory/index.md` as append-only except to add today's heading.
- Use unique slugs; add a short random suffix if unsure.
- Prefer exact replace or insert tools over full-file rewrites.

## User Commands

If the user says:

- "check shared memory" - search first, then answer.
- "record this" - write a durable memory entry if it qualifies.
- "what do we know about X?" - search `/shared-memory` for X and summarize
  relevant entries.
