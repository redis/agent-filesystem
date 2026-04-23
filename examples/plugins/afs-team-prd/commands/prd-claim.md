---
description: Claim a new PRD work item — creates /inflight/<handle>/<slug>.md
argument-hint: <handle> <slug>
---

Claim a new work item in the team-prd MCP workspace.

Arguments: $1 = developer handle, $2 = task slug (kebab-case).

1. Check whether `/inflight/$1/$2.md` already exists (use `file_list` on
   `/inflight/$1`). If it does, abort and tell the user.
2. Read `/prd.md` and `/inflight/` briefly to confirm no other dev is already
   working on this slug under a different subdir.
3. Ask the user for:
   - A one-sentence description of the work
   - The section of `/prd.md` it relates to
4. Write `/inflight/$1/$2.md` with this template:

```markdown
# $2

<one-sentence description>

## Spec reference
<link/excerpt from /prd.md>

## Plan
- [ ] step 1
- [ ] step 2

<!-- @$1 claimed YYYY-MM-DD -->
```

5. Confirm with the path and a reminder: "Only edit files under
   `/inflight/$1/`."
