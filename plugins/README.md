# Plugins

This directory holds installable plugin packages for client apps.

## Current Baseline

- `agent-filesystem/` — the baseline Codex plugin for AFS

That baseline plugin is intentionally generic. It wires the
`agent-filesystem` MCP server to a cloud or Self-managed control-plane endpoint,
loads the generic AFS skill, and keeps bearer tokens outside the package.

## Use-Case Packages

Workspace-specific behavior can still come from the template install flow: the
UI creates a workspace, seeds files, mints a scoped token, and renders
client-specific MCP + skill install steps. The control-plane plugin can also
mint those workspace-scoped tokens on demand through `mcp_token_issue`.

Reusable packages can still live above the baseline plugin:

- put polished first-party packages under `templates/<use-case>/codex/`
- keep smaller reference implementations under `examples/plugins/`

Those higher-level packages should layer on top of the generic AFS control-plane
model whenever possible rather than duplicating baseline guidance or embedding a
single workspace token.
