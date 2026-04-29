---
name: {{skillName}}
description: "Use before writing, modifying, or reviewing code when the {{serverName}} MCP server is connected. Read the relevant standards files live from standards/, cite the specific path you used, and treat the workspace as read-only."
---

# Org Coding Standards — read-only protocol

This skill reads your organization's coding standards from the Agent
Filesystem workspace exposed as MCP server `{{serverName}}`. The token is
read-only, so use this workspace as a live source of truth rather than a place
to edit rules.

## Before writing or modifying code

1. Identify the language and surface you are about to touch.
2. Read `standards/languages/<language>.md` with
   `{{toolPrefix}}file_read` when a matching file exists.
3. Read `standards/architecture-principles.md` and
   `standards/security.md` when the task crosses module boundaries, handles
   user input, touches secrets, or changes authorization behavior.
4. Apply the rules you read and cite the specific standard path when it affects
   the plan or implementation.

## When reviewing code

1. Read `standards/review-checklist.md`.
2. Check the diff against the applicable language, architecture, and security
   files.
3. Lead with findings, and cite the standard path behind each concern.

## If a standard seems wrong or missing

Do not edit this workspace through the MCP tools. Tell the user which file or
section is missing and ask whether a maintainer should update the standard.
