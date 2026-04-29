---
description: Search shared memory for entries matching a query. Use when you want an explicit lookup without asking a question.
---

Search the shared memory workspace for entries matching the user's query.

1. Call `{{toolPrefix}}file_grep` with pattern = the user's query,
   path = `/shared-memory`.
2. For each promising hit, read the full entry with
   `{{toolPrefix}}file_read`.
3. Report the findings: for each entry, show title + context + finding +
   the file path. If nothing matched, say so explicitly.

Query: $ARGUMENTS
