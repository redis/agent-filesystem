# Protocol for this workspace

This workspace coordinates a team. Multiple agents and humans write
here. Follow these rules so you do not step on each other.

## Your handle

On first interaction, ask the user for a short stable handle (for
example `alice` or `bob`). Remember it for the session and use it in
every task file you create or update.

## Claiming a task

1. Look in `tasks/backlog.md` for a line that matches the user's goal.
2. `file_write` `tasks/in-progress/<handle>-<slug>.md` with:

       ---
       owner: <handle>
       started: YYYY-MM-DD
       progress: 0%
       status: active
       ---

       # <Task title>

       **Goal.** One paragraph.

       **Plan.** Bulleted steps.

       **Log.** Append dated bullets as you work.

3. Remove the matching line from `tasks/backlog.md` (edit the file,
   delete only that line).

## Making progress

Append a dated bullet to the **Log** section of your task file after
each meaningful step. Update the `progress:` front-matter field.

## Finishing a task

1. Move the content to `tasks/done/<slug>.md`.
2. Delete the in-progress file with `file_delete_lines` + `file_write`
   replacement, or simply `file_write` to an empty string.
3. If `checkpoint_create` is available to you, snapshot the workspace
   with a name like `milestone-<slug>`.

## Recording a question

`file_write` `questions/<slug>.md` with:

    ---
    asked-by: <handle>
    date: YYYY-MM-DD
    status: open
    ---

    # <Question>

    Answer here once resolved.

## Never

- Never edit another owner's in-progress task file.
- Never rewrite `spec.md` or `roadmap.md` — append new sections.
