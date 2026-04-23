# afs-team-prd

A Claude Code plugin that turns an Agent Filesystem workspace into a **shared,
live PRD** that a whole team of developers (and their Claude Code agents) can
collaborate on concurrently.

## How multi-writer works without locks

The workspace enforces a layout convention that eliminates write conflicts:

| Path | Owner | Write pattern |
|------|-------|--------------|
| `/prd.md` | PRD owners | Append-only, owner marker required |
| `/inflight/<handle>/` | That handle only | Single-writer — no conflicts |
| `/done/` | Anyone | Append-only (new files only) |
| `/questions.md` | Anyone | Append-only |

Per-dev subdirs mean two devs' Claude Code instances never race to write the
same file. Append-only shared docs mean edits compose instead of overwriting.

## What's in the box

- **Skill** `team-prd` — reads the PRD, respects the layout, avoids conflicts.
- **Slash commands**
  - `/prd-status` — current inflight/done/questions summary
  - `/prd-claim <handle> <slug>` — start a new work item
  - `/prd-done <handle> <slug>` — mark an item shipped
  - `/prd-question <handle>` — append an open question
- **MCP server config** — wires `afs mcp --workspace team-prd --profile
  workspace-rw`.

## Prerequisites

1. An AFS workspace named `team-prd`.
2. `afs` CLI installed; each dev logged in with a `workspace-rw` team token.
3. The workspace seeded with `/prd.md`, `/CONVENTIONS.md`, and empty
   `/inflight/`, `/done/`, `/questions.md`.

## Install

```bash
# Workspace owner (once):
afs workspace create team-prd
# Seed /prd.md, /CONVENTIONS.md, /questions.md via the web UI or `afs up`
# Create a workspace-rw MCP token, share with the team

# Each developer:
afs login
claude plugin install ./examples/plugins/afs-team-prd
```

## Customizing

- Rename the workspace: update `.mcp.json` and every reference in
  `skills/team-prd/SKILL.md`.
- Add fields to the claim template: edit `commands/prd-claim.md`.
- Enforce stricter ownership (e.g. only PRD owners touch `/prd.md`): extend
  the skill with a check against an `/OWNERS` file in the workspace.
