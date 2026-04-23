---
name: team-rules
description: Apply the team's shared coding rules before writing, modifying, or reviewing code. Trigger when the user asks to add a feature, refactor, fix a bug, write tests, or review a change. Reads rules live from the team-rules MCP workspace at /rules/*.md.
---

# Team Coding Rules

Your team keeps its canonical coding rules in a shared Agent Filesystem workspace
exposed as the `team-rules` MCP server. The workspace is **read-only** — nobody's
agent can write to it. It's the single source of truth for how this team writes
code.

## Before writing or editing code

1. Call `mcp__team-rules__file_list` with `path: "/rules"` and `depth: 2`.
2. For the task at hand, read every rule file that plausibly applies. Examples:
   - auth/secrets/PII work → `/rules/security.md`
   - any new file or identifier → `/rules/style.md`, `/rules/naming.md`
   - adding tests → `/rules/testing.md`
   - cross-module or new module work → `/rules/architecture.md`
3. Cite applicable rules in your plan by path (e.g. "per `/rules/security.md`…").
4. Apply the rules to the code you produce.

## Rules of engagement

- The workspace is read-only. Do not attempt `file_write`, `file_replace`,
  `file_insert`, or `file_delete_lines` against `team-rules` — they will fail.
- If a rule conflicts with what the user asked for, surface the conflict and
  ask before deciding.
- If no rule applies, say so — don't invent or extrapolate.
- Prefer reading the live rule over recalling it from memory. Rules change.

## Quick reference

| Situation | Rule file to read |
|-----------|------------------|
| Any code edit | `/rules/style.md` |
| New file / function / type | `/rules/naming.md` |
| Auth, input handling, secrets, PII | `/rules/security.md` |
| Adding or modifying tests | `/rules/testing.md` |
| New module or cross-boundary work | `/rules/architecture.md` |
| Commit messages, PR description | `/rules/git.md` |
