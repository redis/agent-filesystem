# Plans

Agent-created plans live here while work is active. These are workflow
artifacts, not product docs. Keep `docs/` focused on the current state of the
app.

## Layout

- `plans/<slug>.md` - active implementation plans.
- `plans/future-work.md` - known future work that is not currently active.
- `plans/archive/` - completed, cancelled, or superseded plans.

## Lifecycle

1. Create a short, named plan in `plans/<slug>.md` before starting non-trivial
   work.
2. Track current status, what is in flight, what is left, decisions, blockers,
   and verification evidence in that file as work progresses.
3. When the work is complete, add the final result and verification summary,
   then move the file to `plans/archive/YYYY-MM-DD-<slug>.md`.
4. If the work changes current behavior, update `docs/` separately. Do not
   leave product truth only in a plan.
5. If a plan becomes obsolete before completion, move it to `plans/archive/`
   with a short note explaining why.

## Active Plan Template

```markdown
# <Plan Title>

Status: active
Owner: <agent/person>
Created: YYYY-MM-DD
Updated: YYYY-MM-DD

## Goal

## Scope

## Checklist

- [ ] ...

## In Flight

- ...

## Decisions / Blockers

- ...

## Verification

- [ ] ...

## Result

Fill this in before archiving.
```
