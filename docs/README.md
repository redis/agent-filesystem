# Documentation

Last reviewed: 2026-05-02.

Docs are split into three simple buckets:

- [guides/](guides/) - how to use AFS.
- [reference/](reference/) - CLI, API, SDK, and MCP contracts.
- [internals/](internals/) - current architecture, repo map, and performance notes.

`docs/` describes the current state of the app and repo. Active plans, future
work, and archived planning notes live under root [../plans/](../plans/).

## Start Here

- [guides/agent-filesystem.md](guides/agent-filesystem.md) - agent-facing guide.
- [reference/cli.md](reference/cli.md) - CLI reference.
- [reference/mcp.md](reference/mcp.md) - MCP tool reference.
- [reference/control-plane-api.md](reference/control-plane-api.md) - HTTP API.
- [internals/cloud.md](internals/cloud.md) - cloud/control-plane architecture.
- [internals/versioned-filesystem.md](internals/versioned-filesystem.md) -
  checkpoint and history model.
- [internals/repo-walkthrough.md](internals/repo-walkthrough.md) - repo map.

## Rules

- Current behavior goes in `guides/`, `reference/`, or `internals/`.
- Future work and active implementation plans go in root `plans/`, not `docs/`.
- Accepted decisions that should not be relitigated go in
  `internals/decisions/` as short ADRs.
- Do not create dated trackers, backlog files, proposal folders, or plan folders
  under `docs/`.
- Raw benchmark output belongs outside the repo, usually under `/tmp`; summarize
  durable conclusions in [internals/performance.md](internals/performance.md).
