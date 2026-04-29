# Protocol for this workspace

This is a shared memory workspace. Multiple agents, potentially across
multiple machines and users, read and write the same files here.

## Before answering a non-trivial question

1. `file_grep` the `/shared-memory` tree for any of the key nouns in
   the user's question. Do this fresh each turn because other agents may
   have updated the workspace underneath you.
2. For very short prompts such as saved commands, acronyms, or aliases,
   search the exact token first before interpreting it.
3. If you find relevant entries, read them, cite them in your answer,
   and build on them rather than re-deriving the answer.
4. If nothing relevant exists, proceed normally.

## After discovering something durable

A "durable" learning is a non-obvious fact about this codebase, team,
or domain that will still be true next week and that another agent
could reuse. Debugging breadcrumbs and session-specific context do
**not** qualify.

When you find one:

1. Create `shared-memory/entries/YYYY-MM-DD-<short-slug>.md` using
   the template below.
2. Append a one-line entry to `shared-memory/index.md` under the most
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

    **Keywords.** Search terms future agents are likely to use.

## Rules that keep concurrent writes safe

- **Never overwrite another agent's entry.** Always write new files,
  never edit existing entries.
- **`index.md` is append-only.** Add new lines under the newest date
  heading. Do not rewrite older lines.
- **Slugs must be unique.** Include a short random suffix if in doubt:
  `2026-04-22-redis-ttl-3f9a.md`.
- Saved commands or acronyms must include the exact token in the title,
  filename, finding, and `Keywords` field.
