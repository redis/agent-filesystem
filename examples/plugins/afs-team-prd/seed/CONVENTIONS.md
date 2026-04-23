# Team PRD Workspace Conventions

This workspace is a **live, shared PRD** accessed by multiple developers and
their Claude Code agents. To avoid write conflicts, follow the layout below.

## Layout

| Path | Owner | Write pattern |
|------|-------|--------------|
| `/prd.md` | PRD owners | Append-only; existing sections are edited only by the owner with a revised-by marker |
| `/inflight/<handle>/*.md` | That handle only | Free-form within your subdir |
| `/done/<YYYY-MM-DD>-<slug>.md` | Anyone | Append-only; new files only |
| `/questions.md` | Anyone | Append-only |

## Identity

Each developer picks a short, stable `<handle>` — lowercase, kebab-case, no
spaces. Their Claude Code agent uses it as the subdir under `/inflight/`.

## Section markers

Every shared section ends with a marker:

```markdown
<!-- @<handle> last-updated-YYYY-MM-DD -->
```

For revisions on a shared section:

```markdown
<!-- revised by @<handle> YYYY-MM-DD -->
```

## Question status

Questions in `/questions.md` use a status marker:

```markdown
<!-- @<handle> YYYY-MM-DD STATUS: open -->
```

Status transitions: `open` → `answered (<answer-slug>)` → optional
`wontfix (<reason>)`.
