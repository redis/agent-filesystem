---
description: Append a new open question to /questions.md in the team-prd workspace
argument-hint: <handle>
---

Append a new open question to the shared `/questions.md`.

Argument: $1 = developer handle.

1. Read `/questions.md` to understand existing format and avoid duplicates.
2. Ask the user for:
   - Question title
   - Question body (context, options considered)
3. Compute today's date in `YYYY-MM-DD`.
4. Append to `/questions.md` (do not rewrite existing content). Use
   `file_insert` at end of file with:

```markdown

## <title>
<!-- @$1 <DATE> STATUS: open -->

<body>
```

5. Confirm to the user that the question is posted and visible to the team.
