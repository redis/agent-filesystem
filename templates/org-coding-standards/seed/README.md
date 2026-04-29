# Org Coding Standards

A **read-only** source of truth for your team's coding standards.
Every developer's agents mount this workspace and consult it before
writing code. Updates flow through a small set of maintainers.

## Layout

- `AGENTS.md` — the protocol agents follow.
- `standards/languages/<lang>.md` — per-language rules.
- `standards/review-checklist.md` — what to check in every PR.
- `standards/security.md` — security rules.
- `standards/architecture-principles.md` — org-wide architecture defaults.

## Sharing this workspace

The MCP access token created alongside this workspace has the
`workspace-ro` profile. Agents can read the standards but cannot
modify them.

Distribute the token through your team password manager. Every
developer drops it into their MCP client config (see the dialog that
opened when you created this workspace) and their agent immediately
sees the same rules.

## Updating the standards

Edit the files through the AFS web UI, or create a second
`workspace-rw` token scoped to maintainers only. The next read from
any developer's agent picks up the change — no redeploy, no cache.
