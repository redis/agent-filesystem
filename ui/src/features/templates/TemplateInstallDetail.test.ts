import { afterEach, describe, expect, it, vi } from "vitest";

import {
  buildCodexSkillInstallCommand,
  buildCodexTomlConfig,
} from "./codex-install";
import { copyTextToClipboard } from "./clipboard";
import { findTemplate } from "./templates-data";
import { templateServerName, templateSkillName } from "./agent-install";

const originalClipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");
const originalExecCommand = Object.getOwnPropertyDescriptor(document, "execCommand");

afterEach(() => {
  vi.restoreAllMocks();
  if (originalClipboard) {
    Object.defineProperty(navigator, "clipboard", originalClipboard);
  } else {
    Reflect.deleteProperty(navigator, "clipboard");
  }
  if (originalExecCommand) {
    Object.defineProperty(document, "execCommand", originalExecCommand);
  } else {
    Reflect.deleteProperty(document, "execCommand");
  }
});

describe("copyTextToClipboard", () => {
  it("uses the async clipboard API when it is available", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    const execCommand = vi.fn(() => true);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: execCommand,
    });

    await expect(copyTextToClipboard("displayed command")).resolves.toBe(true);

    expect(writeText).toHaveBeenCalledWith("displayed command");
    expect(execCommand).not.toHaveBeenCalled();
  });

  it("falls back when the async clipboard API is denied", async () => {
    const writeText = vi.fn().mockRejectedValue(new DOMException("denied"));
    const execCommand = vi.fn(() => true);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: execCommand,
    });

    await expect(copyTextToClipboard("displayed command")).resolves.toBe(true);

    expect(writeText).toHaveBeenCalledWith("displayed command");
    expect(execCommand).toHaveBeenCalledWith("copy");
  });
});

describe("buildCodexTomlConfig", () => {
  it("uses Codex's documented static header format for hosted MCP servers", () => {
    const body = buildCodexTomlConfig("team-memory", "afs_mcp_test_token");
    expect(body).toContain("[mcp_servers.afs-team-memory]");
    expect(body).toContain('http_headers = { Authorization = "Bearer afs_mcp_test_token" }');
    expect(body).not.toContain("bearer_token =");
  });

  it("normalizes workspace names before using them in server keys", () => {
    expect(templateServerName("Team Memory!")).toBe("afs-team-memory");
    const body = buildCodexTomlConfig("Team Memory!", "afs_mcp_test_token");
    expect(body).toContain("[mcp_servers.afs-team-memory]");
  });
});

describe("buildCodexSkillInstallCommand", () => {
  it("installs the shared-memory skill into Codex's user-scope discovery path", () => {
    const template = findTemplate("shared-agent-memory")!;
    const command = buildCodexSkillInstallCommand({
      workspaceName: "team-memory",
      template,
    });

    expect(command).toBeTruthy();
    expect(templateSkillName("team-memory")).toBe("afs-team-memory");
    expect(command).toContain("mkdir -p ~/.agents/skills/afs-team-memory");
    expect(command).toContain(
      "cat > ~/.agents/skills/afs-team-memory/SKILL.md",
    );
    expect(command).toContain("name: afs-team-memory");
    expect(command).toContain("mcp__afs-team-memory__file_grep");
    expect(command).not.toContain("{{serverName}}");
  });

  it("ships a unique skill for each non-blank template", () => {
    for (const id of [
      "shared-agent-memory",
      "shared-llm-wiki-karpathy",
      "org-coding-standards",
      "team-planning-board",
    ]) {
      const template = findTemplate(id)!;
      expect(template.agentSkill, id).toBeTruthy();
      expect(
        buildCodexSkillInstallCommand({
          workspaceName: `${template.slug}-workspace`,
          template,
        }),
        id,
      ).toContain(`mcp__afs-${template.slug}-workspace__`);
    }
  });

  it("returns null when the template does not ship a skill", () => {
    const template = findTemplate("blank")!;
    expect(
      buildCodexSkillInstallCommand({
        workspaceName: "blank",
        template,
      }),
    ).toBeNull();
  });
});
