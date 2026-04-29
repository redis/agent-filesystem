# Team Planning Board

A shared whiteboard for a team of humans and agents coordinating on a
project. One spec, one roadmap, a live view of who is working on what,
a place to record completed work, and a place to surface open questions.

## Layout

- `plan/spec.md` — the overall goal, scope, non-goals.
- `plan/roadmap.md` — phases and milestones.
- `tasks/backlog.md` — unstarted work.
- `tasks/in-progress/<owner>-<slug>.md` — one file per active task.
- `tasks/done/<slug>.md` — completed tasks, appended after finish.
- `questions/<slug>.md` — open decisions awaiting an answer.

## How it stays sane with many writers

- **Per-owner subdirs for drafts.** Each person writes under a file
  named for their handle, so two agents never edit the same file.
- **Append-only for shared docs.** `spec.md`, `roadmap.md`, and
  `backlog.md` grow by addition. Use owner markers
  (`<!-- @handle 2026-04-22 -->`) when amending.
- **Checkpoints per milestone.** This workspace's MCP profile includes
  `checkpoint_create`. Snapshot the board at each milestone so the
  timeline is recoverable.

See `AGENTS.md` for the full protocol.
