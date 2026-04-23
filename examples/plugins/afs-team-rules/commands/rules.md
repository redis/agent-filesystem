---
description: List and summarize the team's current coding rules from the shared AFS workspace
---

List every rule file in the shared `team-rules` MCP workspace and produce a concise summary.

1. Call `mcp__team-rules__file_list` with `path: "/rules"` and `depth: 2`.
2. For each file returned, read it with `mcp__team-rules__file_read`.
3. Produce a one-line summary per rule, grouped by subdirectory if present.
4. Flag any rule whose `modified_at` is within the last 14 days as **NEW/CHANGED**.
5. End with a reminder that the workspace is read-only and rules are authoritative.
