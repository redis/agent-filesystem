---
description: Force-record a durable learning into shared memory, bypassing the auto-skill's judgment on durability.
---

Record a new entry in shared memory. Use the title the user provides.

1. Ask the user for Context, Finding, and Sources if they didn't provide
   them in the command arguments.
2. Pick a slug from the title: kebab-case, prefixed with today's date
   (YYYY-MM-DD). If unsure about collisions, append a 4-char random suffix.
3. Write `/shared-memory/entries/<slug>.md` via
   `{{toolPrefix}}file_write` using the standard entry template
   (frontmatter: date, agent; sections: Context, Finding, Sources, Keywords).
4. Append a one-line pointer to `/shared-memory/index.md` under the most
   recent date heading via `{{toolPrefix}}file_insert`.

Title: $ARGUMENTS
