---
name: team-prd
description: Read and update the team's shared PRD in the team-prd AFS workspace. Trigger when the user asks about current/inflight/done work, wants to start a new work item, mark something complete, or add an open question to the team's spec. Enforces per-dev subdir isolation to avoid write conflicts between multiple developers' agents.
---

# Team PRD Workflow

The team's shared product requirements doc lives in the `team-prd` MCP workspace.
Multiple developers' Claude Code instances write here concurrently, so the
layout is structured to **avoid write conflicts without needing a locking
protocol**.

## Layout convention

```
/prd.md                          # canonical spec (shared — append-only edits)
/inflight/<dev-handle>/*.md      # per-dev work-in-progress (single-writer subdir)
/done/<YYYY-MM-DD>-<slug>.md     # completed items (append-only log)
/questions.md                    # open questions (shared — append-only)
/CONVENTIONS.md                  # this convention doc (read before first use)
```

## Rules for avoiding conflicts

1. **You only ever write under `/inflight/<your-handle>/`.** No exceptions for
   your own drafts. Never touch another developer's subdir.
2. **Shared files (`/prd.md`, `/questions.md`) are append-only.** If you must
   revise an existing section, ask the user first, then use `file_replace` on
   a uniquely-identified block and add a `<!-- revised by @<handle>
   YYYY-MM-DD -->` marker.
3. **`/done/` is append-only.** New files only — never rewrite existing entries.
4. Each shared section should carry a trailing owner marker:
   `<!-- @<owner-handle> last-updated-YYYY-MM-DD -->`.

## Establishing identity

Before any write, know the current developer's handle. Ask the user once:

> "What handle should I use for you in the team PRD workspace?"

Remember it for the session. Use the handle as the subdir under `/inflight/`.

## Starting a new work item

1. Read `/prd.md` — find the section the item belongs to.
2. Read existing files under `/inflight/` to make sure nobody else is already
   working on it.
3. Create `/inflight/<handle>/<slug>.md` with:
   - A one-sentence description
   - A link/excerpt from `/prd.md`
   - A "Plan" section with concrete steps
   - A timestamp and owner marker

## Marking an item done

1. Read `/inflight/<handle>/<slug>.md`.
2. Write `/done/<YYYY-MM-DD>-<slug>.md` with the original content plus a
   "Completed" section summarizing what shipped.
3. Delete the inflight file.

## Adding an open question

Append to `/questions.md` — do not rewrite. Format:

```markdown
## <question title>
<!-- @<handle> YYYY-MM-DD STATUS: open -->

<question body>
```

When a question is answered, `file_replace` only the `STATUS: open` marker
with `STATUS: answered (<answer-slug>)`.

## Read-heavy reflexes

Before any write, re-read the relevant shared file. The workspace is live —
state may have changed since you last looked.
