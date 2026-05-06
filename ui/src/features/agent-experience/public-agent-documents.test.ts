import { describe, expect, test } from "vitest";
import { getSiteAgentDocument } from "./public-agent-documents";

describe("getSiteAgentDocument", () => {
  test("builds a home page markdown view with quickstart commands", () => {
    const doc = getSiteAgentDocument("/", {
      controlPlaneUrl: "https://afs.example.com",
      siteOrigin: "https://ui.example.com",
    });

    expect(doc.title).toBe("Agent Filesystem");
    expect(doc.markdown).toContain("afs ws mount getting-started ~/getting-started");
    expect(doc.markdown).toContain("[Docs](https://ui.example.com/docs)");
  });

  test("builds downloads markdown with the current control plane URL", () => {
    const doc = getSiteAgentDocument("/downloads", {
      controlPlaneUrl: "https://afs.example.com",
      siteOrigin: "https://ui.example.com",
    });

    expect(doc.markdown).toContain(
      "curl -fsSL \"https://afs.example.com/v1/cli?os=$(uname -s)&arch=$(uname -m)\" -o afs && chmod +x afs",
    );
  });

  test("returns the agent guide asset for the guide route", () => {
    const doc = getSiteAgentDocument("/agent-guide", {
      controlPlaneUrl: "https://afs.example.com",
      siteOrigin: "https://ui.example.com",
    });

    expect(doc.assetPath).toBe("/agent-guide.md");
    expect(doc.markdown).toContain("The full markdown guide is appended below.");
  });

  test("summarizes docs topics with canonical references", () => {
    const doc = getSiteAgentDocument("/docs/cli", {
      controlPlaneUrl: "https://afs.example.com",
      siteOrigin: "https://ui.example.com",
    });

    expect(doc.title).toBe("AFS CLI Workflow");
    expect(doc.markdown).toContain("Reference: cli.md");
    expect(doc.markdown).toContain("Install the CLI");
  });

  test("summarizes authenticated app routes too", () => {
    const doc = getSiteAgentDocument("/agents", {
      controlPlaneUrl: "https://afs.example.com",
      siteOrigin: "https://ui.example.com",
    });

    expect(doc.title).toBe("Agents: Active Agents");
    expect(doc.markdown).toContain("Live sessions connected to AFS right now");
    expect(doc.markdown).toContain("[MCP](https://ui.example.com/mcp)");
  });

  test("uses workspace tab state for workspace studio routes", () => {
    const doc = getSiteAgentDocument("/workspaces/payments-portal", {
      controlPlaneUrl: "https://afs.example.com",
      siteOrigin: "https://ui.example.com",
      search: "?tab=checkpoints&databaseId=db-1",
    });

    expect(doc.title).toBe("Workspace Studio: Checkpoints");
    expect(doc.markdown).toContain("Active tab: Checkpoints");
    expect(doc.markdown).toContain("afs cp create payments-portal before-risky-change");
  });

  test("describes installed template pages directly", () => {
    const doc = getSiteAgentDocument("/templates/installed/payments-portal", {
      controlPlaneUrl: "https://afs.example.com",
      siteOrigin: "https://ui.example.com",
      search: "?databaseId=db-main",
    });

    expect(doc.title).toBe("Installed Template");
    expect(doc.markdown).toContain("repair missing template MCP tokens");
    expect(doc.markdown).toContain("[Workspace](https://ui.example.com/workspaces/payments-portal?databaseId=db-main)");
  });

  test("captures connect-cli success state", () => {
    const doc = getSiteAgentDocument("/connect-cli", {
      controlPlaneUrl: "https://afs.example.com",
      siteOrigin: "https://ui.example.com",
      search: "?connected=true&workspace_name=getting-started",
    });

    expect(doc.title).toBe("Connect CLI: Success");
    expect(doc.markdown).toContain("Browser handoff is complete");
    expect(doc.markdown).toContain("afs ws mount getting-started ~/afs/getting-started");
  });

  test("uses the dedicated MCP connect instructions", () => {
    const doc = getSiteAgentDocument("/mcp/connect", {
      controlPlaneUrl: "https://afs.example.com",
      siteOrigin: "https://ui.example.com",
    });

    expect(doc.title).toBe("MCP: Connect an Agent");
    expect(doc.markdown).toContain("codex mcp add agent-filesystem");
    expect(doc.markdown).toContain("claude mcp add --scope user");
  });
});
