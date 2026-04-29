---
name: {{skillName}}
description: "Use when coordinating project work through the {{serverName}} MCP server: reading the spec, claiming tasks, updating in-progress work, recording done work, or writing open questions. Preserve per-owner task files and append-only shared docs."
---

# Team Planning Board — coordination protocol

This skill coordinates a shared project board stored in Agent Filesystem and
exposed as MCP server `{{serverName}}`. Multiple humans and agents may write
to this workspace, so keep ownership clear and avoid broad rewrites.

## First step in a session

If you do not already know the user's handle, ask for a short stable handle
such as `alice` or `frontend`. Use that handle in every task file you
create or update.

## Before planning work

1. Read `plan/spec.md` and `plan/roadmap.md`.
2. Read `tasks/backlog.md`.
3. List `tasks/in-progress/` with
   `{{toolPrefix}}file_list` at depth 2 so you do not duplicate active
   work.

## Claiming a task

1. Pick the matching backlog line with the user.
2. Create `tasks/in-progress/<handle>-<slug>.md` with owner, date, progress,
   goal, plan, and log sections.
3. Remove only the claimed line from `tasks/backlog.md`.

## Making progress

Append dated notes to your own in-progress file. Do not edit another owner's
in-progress file. Shared files such as `plan/spec.md`,
`plan/roadmap.md`, and `questions/*.md` should grow by focused additions
instead of rewrites.

## Finishing work

1. Move the completed task content to `tasks/done/<slug>.md`.
2. Clear the in-progress file after verifying the done file exists.
3. If `checkpoint_create` is available, create a milestone checkpoint with a
   short descriptive name.
