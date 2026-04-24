# Agent Filesystem Codex Plugin

Codex plugin for managing Agent Filesystem through a control-plane MCP endpoint.

This plugin bundles:

- `.codex-plugin/plugin.json` — Codex plugin manifest
- `.mcp.json` — MCP config for the control-plane server
- `skills/agent-filesystem/SKILL.md` — operating guidance for Codex

The plugin is intentionally secret-free. It references the bearer token through
the `AFS_CONTROL_PLANE_TOKEN` environment variable.

## Endpoint Configuration

Choose the target control plane by editing the `url` value in `.mcp.json`.

For AFS Cloud:

```json
"url": "https://agent-filesystem.vercel.app/mcp"
```

For a local or Self-managed control plane:

```json
"url": "http://127.0.0.1:8091/mcp"
```

Keep the plugin name and MCP server name as `agent-filesystem`; only the URL
changes.

## Token Setup

1. Open the AFS UI MCP page.
2. Create a `Control plane` token.
3. Store it in the environment before starting Codex:

```bash
export AFS_CONTROL_PLANE_TOKEN='<paste-token-here>'
```

Codex reads the token through `bearer_token_env_var` in `.mcp.json`; the token
does not belong in the plugin files or in Git.

## Install

For repo-local testing, keep this plugin at:

```text
plugins/agent-filesystem
```

and keep the marketplace entry in:

```text
.agents/plugins/marketplace.json
```

Then restart Codex, open the plugin manager, add this repo as a marketplace
source, and install the `agent-filesystem` plugin from that marketplace.

After changing `.mcp.json`, reinstall or refresh the local marketplace install
and restart Codex so the cached plugin copy picks up the new endpoint.
