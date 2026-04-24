import type { Template } from "./templates-data";
import {
  buildTemplateSkillMarkdown,
  substituteTemplateSkillPlaceholders,
  templateMCPUrl,
  templateServerName,
  templateSkillName,
} from "./agent-install";

/* -------------------------------------------------------------------------- */
/* Claude Code plugin generator                                               */
/*                                                                            */
/* Builds an in-memory file tree for a Claude Code *marketplace* bundle       */
/* containing a single plugin wired to a specific AFS workspace.              */
/*                                                                            */
/* `claude plugin install <zip>` does NOT accept a bare plugin zip — it only  */
/* installs from marketplaces. So we ship the plugin inside a minimal local   */
/* marketplace and instruct the user to:                                      */
/*                                                                            */
/*   /plugin marketplace add ~/Downloads/afs-<workspace>                      */
/*   /plugin install afs-<workspace>@afs-<workspace>                          */
/*                                                                            */
/* The bundle uses a single top-level directory so macOS Safari's auto-unzip  */
/* yields a clean `~/Downloads/afs-<workspace>/` folder.                      */
/*                                                                            */
/* The plugin pre-embeds the workspace's bearer token in `.mcp.json`. Token   */
/* rotation is the user's responsibility — the README explains the flow.     */
/* -------------------------------------------------------------------------- */

export type PluginFile = {
  path: string;
  content: string;
};

/**
 * Build the Claude Code marketplace bundle for a given workspace.
 *
 * Returns an array of `{path, content}` pairs ready to zip. All paths sit
 * under a single top-level directory (`afs-<workspaceName>/`) so Safari's
 * auto-unzip produces that folder at the download location.
 */
export function buildClaudePlugin(args: {
  template: Template;
  workspaceName: string;
  controlPlaneUrl: string;
  token: string;
}): PluginFile[] {
  const { template, workspaceName, controlPlaneUrl, token } = args;
  const spec = template.agentSkill;
  if (!spec) {
    throw new Error(
      `Template "${template.id}" has no agentSkill spec — cannot build plugin.`,
    );
  }

  const serverName = templateServerName(workspaceName);
  const pluginName = serverName;
  // Marketplace and plugin share the same name; Claude Code's `@` separator
  // disambiguates them in `/plugin install <plugin>@<marketplace>`.
  const marketplaceName = pluginName;
  const bundleRoot = pluginName; // top-level directory inside the zip
  const pluginDir = `${bundleRoot}/plugins/${pluginName}`;
  const skillDir = templateSkillName(workspaceName);
  const mcpUrl = templateMCPUrl(controlPlaneUrl);

  const files: PluginFile[] = [];

  // .claude-plugin/marketplace.json (at bundle root)
  files.push({
    path: `${bundleRoot}/.claude-plugin/marketplace.json`,
    content:
      JSON.stringify(
        {
          name: marketplaceName,
          owner: {
            name: "Agent Filesystem",
          },
          plugins: [
            {
              name: pluginName,
              source: `./plugins/${pluginName}`,
              description: `${template.title} — ${template.tagline}`,
            },
          ],
        },
        null,
        2,
      ) + "\n",
  });

  // plugins/<name>/.claude-plugin/plugin.json
  files.push({
    path: `${pluginDir}/.claude-plugin/plugin.json`,
    content:
      JSON.stringify(
        {
          name: pluginName,
          version: "0.1.0",
          description: `${template.title} — ${template.tagline}`,
          author: "Agent Filesystem",
        },
        null,
        2,
      ) + "\n",
  });

  // plugins/<name>/.mcp.json — HTTP + inline bearer token
  files.push({
    path: `${pluginDir}/.mcp.json`,
    content:
      JSON.stringify(
        {
          mcpServers: {
            [serverName]: {
              url: mcpUrl,
              headers: {
                Authorization: `Bearer ${token}`,
              },
            },
          },
        },
        null,
        2,
      ) + "\n",
  });

  // plugins/<name>/skills/<workspace-skill>/SKILL.md
  const skillMarkdown = buildTemplateSkillMarkdown({
    workspaceName,
    template,
  });
  files.push({
    path: `${pluginDir}/skills/${skillDir}/SKILL.md`,
    content: skillMarkdown ?? "",
  });

  // plugins/<name>/commands/<name>.md
  for (const command of spec.commands ?? []) {
    files.push({
      path: `${pluginDir}/commands/${command.name}.md`,
      content:
        substituteTemplateSkillPlaceholders({
          text: command.body,
          workspaceName,
          template,
        }).trimEnd() + "\n",
    });
  }

  // README.md (at bundle root — user-facing install instructions)
  files.push({
    path: `${bundleRoot}/README.md`,
    content: buildReadme({
      template,
      workspaceName,
      serverName,
      pluginName,
      marketplaceName,
    }),
  });

  // install.sh (at bundle root — fallback installer for when `/plugin` is
  // unavailable, gated, or the user prefers plain-file install).
  files.push({
    path: `${bundleRoot}/install.sh`,
    content: buildInstallScript({
      serverName,
      pluginName,
      mcpUrl,
      token,
      skillDir,
      commandNames: (spec.commands ?? []).map((c) => c.name),
    }),
  });

  return files;
}

/**
 * Commands the user runs after the bundle is extracted to `<extractPath>`
 * (e.g. `~/Downloads/afs-shared-memory`). `marketplaceAdd` + `pluginInstall`
 * are slash commands for Claude Code's plugin system. `fallbackInstall` is a
 * shell command that runs the bundled `install.sh` — use when `/plugin` is
 * unavailable or gated.
 */
export function installCommands(args: {
  workspaceName: string;
  extractPath: string;
}): {
  marketplaceAdd: string;
  pluginInstall: string;
  fallbackInstall: string;
} {
  const pluginName = templateServerName(args.workspaceName);
  return {
    marketplaceAdd: `/plugin marketplace add ${args.extractPath}`,
    pluginInstall: `/plugin install ${pluginName}@${pluginName}`,
    fallbackInstall: `bash ${args.extractPath}/install.sh`,
  };
}

/**
 * One-shot shell command that registers the MCP server with Claude Code at
 * user scope. This is the recommended primary path — no zip download, no
 * marketplace dance, just a single paste in the terminal. Requires the
 * `claude` CLI on PATH (ships with Claude Code).
 */
export function buildClaudeMcpAddCommand(args: {
  workspaceName: string;
  mcpUrl: string;
  token: string;
}): string {
  const serverName = templateServerName(args.workspaceName);
  // Use single quotes so URL fragments and bearer chars don't get interpreted
  // by the shell. Token secrets are hex — safe inside single quotes.
  return `claude mcp add --scope user --transport http ${serverName} '${args.mcpUrl}' --header 'Authorization: Bearer ${args.token}'`;
}

function buildInstallScript(args: {
  serverName: string;
  pluginName: string;
  mcpUrl: string;
  token: string;
  skillDir: string;
  commandNames: string[];
}): string {
  const { serverName, pluginName, mcpUrl, token, skillDir, commandNames } =
    args;
  const copyCommands = [
    `SCRIPT_DIR="$(cd "$(dirname "\${BASH_SOURCE[0]}")" && pwd)"`,
    `PLUGIN_DIR="$SCRIPT_DIR/plugins/${pluginName}"`,
    `mkdir -p "$HOME/.claude/skills" "$HOME/.claude/commands"`,
    `cp -R "$PLUGIN_DIR/skills/${skillDir}" "$HOME/.claude/skills/${skillDir}"`,
    ...commandNames.map(
      (name) =>
        `cp "$PLUGIN_DIR/commands/${name}.md" "$HOME/.claude/commands/${name}.md"`,
    ),
  ].join("\n");
  return `#!/usr/bin/env bash
# Manual installer for ${pluginName}.
#
# Use this when Claude Code's \`/plugin\` command is unavailable or gated in
# your environment. It copies the skill + slash commands into your user-scope
# Claude config and registers the MCP server via \`claude mcp add\`.
#
# Re-running is safe — files overwrite cleanly. The MCP server registration
# is idempotent.

set -euo pipefail

${copyCommands}

if command -v claude >/dev/null 2>&1; then
  # Idempotent: remove first if present, then add. \`claude mcp remove\` exits
  # 0 when the server is absent in recent versions; tolerate failure either way.
  claude mcp remove --scope user ${serverName} >/dev/null 2>&1 || true
  claude mcp add --scope user --transport http ${serverName} \\
    ${mcpUrl} \\
    --header "Authorization: Bearer ${token}"
  echo "Registered MCP server '${serverName}' at user scope."
else
  cat <<EOF
WARN: \`claude\` CLI not on PATH. Add the MCP server manually to ~/.claude.json:

  {
    "mcpServers": {
      "${serverName}": {
        "url": "${mcpUrl}",
        "headers": { "Authorization": "Bearer <TOKEN>" }
      }
    }
  }

The token is in .mcp.json inside this bundle.
EOF
fi

echo
echo "Done. Restart Claude Code so the new skill and commands are picked up."
`;
}

function buildReadme(args: {
  template: Template;
  workspaceName: string;
  serverName: string;
  pluginName: string;
  marketplaceName: string;
}): string {
  const { template, workspaceName, serverName, pluginName, marketplaceName } =
    args;
  return `# ${pluginName}

${template.title} — ${template.tagline}

Pre-configured Claude Code plugin for the AFS workspace \`${workspaceName}\`.
Installing it wires up the \`${serverName}\` MCP server (hosted) and a skill
that auto-triggers when relevant. No setup prompt required.

## Install

After downloading, Safari auto-extracts the zip into \`~/Downloads/${pluginName}/\`.

### Option A — Claude Code plugin marketplace (recommended)

Inside Claude Code:

\`\`\`
/plugin marketplace add ~/Downloads/${pluginName}
/plugin install ${pluginName}@${marketplaceName}
\`\`\`

### Option B — Plain-file install (fallback)

If \`/plugin\` is unavailable in your environment, run the bundled installer:

\`\`\`bash
bash ~/Downloads/${pluginName}/install.sh
\`\`\`

This copies the skill + slash commands into \`~/.claude/\` and registers the
MCP server via \`claude mcp add --scope user\`.

---

Either way, \`/mcp\` in Claude Code should then show \`${serverName}\`
connected. The skill fires automatically whenever it's relevant to what
you're asking.

## What's inside

- \`.claude-plugin/marketplace.json\` — minimal marketplace catalog so
  \`/plugin install\` can find the plugin
- \`plugins/${pluginName}/.mcp.json\` — MCP server config with your
  workspace's bearer token inlined
- \`plugins/${pluginName}/skills/${templateSkillName(workspaceName)}/SKILL.md\` — auto-triggering
  skill
${(template.agentSkill?.commands ?? [])
  .map(
    (c) =>
      `- \`plugins/${pluginName}/commands/${c.name}.md\` — \`/${c.name}\` slash command`,
  )
  .join("\n")}
- \`install.sh\` — plain-file fallback installer

## Token rotation

The bearer token in \`.mcp.json\` is embedded at download time and does not
expire automatically. If you rotate the token in the AFS UI, re-download
this plugin and reinstall.

## Uninstall

\`\`\`
/plugin uninstall ${pluginName}@${marketplaceName}
/plugin marketplace remove ${marketplaceName}
\`\`\`
`;
}
