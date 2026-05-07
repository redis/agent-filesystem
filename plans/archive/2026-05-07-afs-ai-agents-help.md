# AFS AI Agents Help

Status: completed
Owner: Codex
Created: 2026-05-07
Updated: 2026-05-07

## Goal

Make AFS advertise agent integrations like QMD and let users show/install the
packaged AFS skill from the CLI.

## Scope

- Top-level `AI Agents:` help section.
- `afs skill show` and `afs skill install`.
- `afs --skill` alias for `afs skill show`.
- Local install target: `./.agents/skills/afs`.
- Global install target: `~/.agents/skills/afs`.
- Optional Claude symlink: `.claude/skills/afs`.

## Checklist

- [x] Add embedded AFS skill payload.
- [x] Add `skill` command routing and install helpers.
- [x] Update top-level help copy.
- [x] Add CLI tests for help, show, install, and alias behavior.
- [x] Update current CLI/docs references.
- [x] Run targeted validation.

## In Flight

None.

## Decisions

- Use the repo's canonical `skills/agent-filesystem/SKILL.md` content as the
  packaged skill content, installed under the user-facing `afs` skill folder.
- Keep the AFS-specific advanced MCP copy to `--workspace` and `--profile`;
  the local Go CLI does not expose QMD's HTTP MCP transport flags.

## Verification

- `go test ./cmd/afs -run 'TestTopLevelHelpDocumentsAIAgents|TestSkill'`
- `go test ./cmd/afs`
- `make afs`
- `./afs --help`
- `./afs skill --help`
- `./afs --skill`
- `/Users/rowantrollope/git/agent-filesystem/afs skill install` from a temp
  directory, then verified `./.agents/skills/afs/SKILL.md` exists.

## Review

AFS now has a QMD-shaped AI-agent help block, a packaged skill command, local
and global install targets, optional Claude symlink support, and current docs
for the new CLI surface.
