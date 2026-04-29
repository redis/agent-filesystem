# Protocol for this workspace

This workspace contains your organization's coding standards. It is
**read-only** for agents. Your role is to consult these files before
writing code and to cite them in reviews.

## Before writing or modifying code

1. Identify the language and surface you're about to touch.
2. `file_read` the matching file under `standards/languages/`.
3. `file_read` `standards/architecture-principles.md` and
   `standards/security.md` when relevant.
4. Apply the rules you read. If any rule conflicts with the user's
   request, surface the conflict and ask how to proceed — do not
   silently override the standard.

## When reviewing a PR or diff

1. `file_read` `standards/review-checklist.md`.
2. Walk each item against the diff. Cite the specific standard file
   and section when you flag something.

## If a standard seems wrong or missing

Do not edit it — this workspace is read-only. Instead, raise it with
the user and point at the specific file and line.
