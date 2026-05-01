# afs-team-rules

A Claude Code plugin that gives every developer on your team **live access to
a shared, read-only library of coding rules** backed by an Agent Filesystem
workspace.

Update the rules in one place (the `team-rules` AFS workspace); every
teammate's Claude Code sees the new version on the next tool call.

## What's in the box

- **Skill** `team-rules` — auto-triggers before code edits; reads applicable
  rule files from the shared workspace and applies them.
- **Slash command** `/rules` — list and summarize current rules on demand.
- **MCP server config** — wires `afs mcp --workspace team-rules --profile
  workspace-ro` as a read-only MCP server.

## Prerequisites

1. An AFS workspace named `team-rules` with rule files under `/rules/*.md`.
2. The `afs` CLI installed and logged in. See the **Connect an Agent** flow in
   the AFS web UI for installation commands.
3. A read-only token scoped to the `team-rules` workspace with profile
   `workspace-ro`, shared with your team.

## Install

```bash
# From the team-rules AFS workspace owner, once:
#   1. Create workspace: afs ws create team-rules
#   2. Upload rule files to /rules/*.md
#   3. Create a workspace-ro MCP token and share with the team

# On each developer's machine:
afs auth login   # enter the shared team token if prompted

# Add this plugin to Claude Code (from repo root):
claude plugin install ./examples/plugins/afs-team-rules
```

## Seeding the workspace

Drop files under `/rules/` — one topic per file. See
[`sample-rules/`](./sample-rules) for starter content you can copy into the
workspace.

## Customizing

- Edit `skills/team-rules/SKILL.md` to change the trigger phrasing or the
  rule-to-situation mapping.
- Edit `.mcp.json` if you renamed the workspace or use a different profile.
