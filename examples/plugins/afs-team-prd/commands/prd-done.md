---
description: Move a completed PRD work item from /inflight/ to /done/
argument-hint: <handle> <slug>
---

Mark a claimed work item as completed.

Arguments: $1 = developer handle, $2 = task slug.

1. Read `/inflight/$1/$2.md`. Abort if missing.
2. Ask the user for a one-paragraph "Completed" summary of what actually
   shipped (may differ from the original plan).
3. Compute today's date in `YYYY-MM-DD`.
4. Write `/done/<DATE>-$2.md` containing the original file's contents plus:

```markdown

## Completed
<!-- @$1 completed <DATE> -->

<completion summary from the user>
```

5. Delete `/inflight/$1/$2.md` with `file_delete_lines` across all lines, then
   verify with `file_list` that the file is gone.
