import { describe, expect, it } from "vitest";

import { buildClaudePlugin, installCommands } from "./claude-plugin";
import { findTemplate } from "./templates-data";

describe("buildClaudePlugin", () => {
  const template = findTemplate("shared-agent-memory")!;
  const args = {
    template,
    workspaceName: "team-memory",
    controlPlaneUrl: "https://afs.cloud",
    token: "afs_mcp_test_token",
  };

  it("emits the expected marketplace + plugin layout under a single top-level dir", () => {
    const files = buildClaudePlugin(args);
    const paths = files.map((f) => f.path).sort();
    expect(paths).toEqual([
      "afs-team-memory/.claude-plugin/marketplace.json",
      "afs-team-memory/README.md",
      "afs-team-memory/install.sh",
      "afs-team-memory/plugins/afs-team-memory/.claude-plugin/plugin.json",
      "afs-team-memory/plugins/afs-team-memory/.mcp.json",
      "afs-team-memory/plugins/afs-team-memory/commands/memory-record.md",
      "afs-team-memory/plugins/afs-team-memory/commands/memory-search.md",
      "afs-team-memory/plugins/afs-team-memory/skills/afs-team-memory/SKILL.md",
    ]);
  });

  it("install.sh contains the expected copy + mcp-add steps", () => {
    const files = buildClaudePlugin(args);
    const installScript = files.find((f) => f.path.endsWith("install.sh"));
    expect(installScript).toBeDefined();
    const body = installScript!.content;
    expect(body.startsWith("#!/usr/bin/env bash")).toBe(true);
    expect(body).toContain(
      'cp -R "$PLUGIN_DIR/skills/afs-team-memory" "$HOME/.claude/skills/afs-team-memory"',
    );
    expect(body).toContain(
      'cp "$PLUGIN_DIR/commands/memory-search.md" "$HOME/.claude/commands/memory-search.md"',
    );
    expect(body).toContain("claude mcp add --scope user --transport http");
    expect(body).toContain("afs-team-memory");
    expect(body).toContain("https://afs.cloud/mcp");
    expect(body).toContain("Bearer afs_mcp_test_token");
  });

  it("marketplace.json references the plugin at the correct relative path", () => {
    const files = buildClaudePlugin(args);
    const marketplace = files.find((f) =>
      f.path.endsWith("marketplace.json"),
    );
    expect(marketplace).toBeDefined();
    const parsed = JSON.parse(marketplace!.content);
    expect(parsed.name).toBe("afs-team-memory");
    expect(parsed.plugins).toHaveLength(1);
    expect(parsed.plugins[0]).toMatchObject({
      name: "afs-team-memory",
      source: "./plugins/afs-team-memory",
    });
    expect(parsed.owner.name).toBe("Agent Filesystem");
  });

  it("names the plugin after the workspace", () => {
    const files = buildClaudePlugin(args);
    const manifest = files.find((f) => f.path.endsWith("plugin.json"));
    expect(manifest).toBeDefined();
    const parsed = JSON.parse(manifest!.content);
    expect(parsed.name).toBe("afs-team-memory");
    expect(parsed.version).toBe("0.1.0");
  });

  it("embeds the MCP URL and bearer token in .mcp.json", () => {
    const files = buildClaudePlugin(args);
    const mcp = files.find((f) => f.path.endsWith(".mcp.json"));
    expect(mcp).toBeDefined();
    const parsed = JSON.parse(mcp!.content);
    const server = parsed.mcpServers["afs-team-memory"];
    expect(server.url).toBe("https://afs.cloud/mcp");
    expect(server.headers.Authorization).toBe("Bearer afs_mcp_test_token");
  });

  it("normalizes a trailing slash on the control-plane URL", () => {
    const files = buildClaudePlugin({
      ...args,
      controlPlaneUrl: "https://afs.cloud/",
    });
    const parsed = JSON.parse(
      files.find((f) => f.path.endsWith(".mcp.json"))!.content,
    );
    expect(parsed.mcpServers["afs-team-memory"].url).toBe(
      "https://afs.cloud/mcp",
    );
  });

  it("substitutes {{serverName}} in the skill description and body", () => {
    const files = buildClaudePlugin(args);
    const skill = files.find((f) => f.path.endsWith("/SKILL.md"));
    expect(skill).toBeDefined();
    expect(skill!.content).toContain("afs-team-memory");
    expect(skill!.content).not.toContain("{{serverName}}");
    // Frontmatter present.
    expect(skill!.content.startsWith("---\nname: afs-team-memory\n")).toBe(
      true,
    );
  });

  it("substitutes {{serverName}} in commands", () => {
    const files = buildClaudePlugin(args);
    const search = files.find((f) => f.path.endsWith("/memory-search.md"));
    expect(search).toBeDefined();
    expect(search!.content).toContain("mcp__afs-team-memory__file_grep");
    expect(search!.content).not.toContain("{{serverName}}");
  });

  it("throws for a template without an agentSkill spec", () => {
    const blank = findTemplate("blank")!;
    expect(() =>
      buildClaudePlugin({
        template: blank,
        workspaceName: "x",
        controlPlaneUrl: "https://afs.cloud",
        token: "t",
      }),
    ).toThrow(/no agentSkill spec/);
  });
});

describe("installCommands", () => {
  it("builds the marketplace-add, plugin-install, and fallback strings", () => {
    const cmds = installCommands({
      workspaceName: "team-memory",
      extractPath: "~/Downloads/afs-team-memory",
    });
    expect(cmds.marketplaceAdd).toBe(
      "/plugin marketplace add ~/Downloads/afs-team-memory",
    );
    expect(cmds.pluginInstall).toBe(
      "/plugin install afs-team-memory@afs-team-memory",
    );
    expect(cmds.fallbackInstall).toBe(
      "bash ~/Downloads/afs-team-memory/install.sh",
    );
  });
});
