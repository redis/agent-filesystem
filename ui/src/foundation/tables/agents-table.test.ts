import { describe, expect, test } from "vitest";
import type { AFSAgentSession } from "../types/afs";
import {
  displayAgentIdentityLabel,
  filterAndSortAgents,
} from "./agents-table-utils";

const baseAgent: AFSAgentSession = {
  sessionId: "sess-1",
  workspaceId: "payments-portal",
  workspaceName: "Payments Portal",
  databaseId: "db-1",
  databaseName: "Primary Database",
  clientKind: "sync",
  afsVersion: "1.2.3",
  hostname: "maya-mbp",
  operatingSystem: "darwin",
  localPath: "/Users/maya/workspaces/payments-portal",
  readonly: false,
  state: "active",
  startedAt: "2026-04-03T10:18:00Z",
  lastSeenAt: "2026-04-03T10:48:00Z",
  leaseExpiresAt: "2026-04-03T10:49:00Z",
};

function buildAgent(overrides: Partial<AFSAgentSession>): AFSAgentSession {
  return { ...baseAgent, ...overrides };
}

describe("filterAndSortAgents", () => {
  const rows = [
    buildAgent({
      sessionId: "sess-payments",
      hostname: "maya-mbp",
      workspaceId: "payments-portal",
      workspaceName: "Payments Portal",
      localPath: "/Users/maya/workspaces/payments-portal",
      lastSeenAt: "2026-04-03T10:48:00Z",
    }),
    buildAgent({
      sessionId: "sess-memory",
      hostname: "support-gateway-01",
      workspaceId: "customer-memory",
      workspaceName: "Customer Memory",
      localPath: "/srv/agents/customer-memory",
      lastSeenAt: "2026-04-03T10:44:00Z",
    }),
  ];

  test("matches agent hostname", () => {
    const filtered = filterAndSortAgents(rows, "gateway", "lastSeenAt", "desc");

    expect(filtered).toHaveLength(1);
    expect(filtered[0]?.sessionId).toBe("sess-memory");
  });

  test("matches local path", () => {
    const filtered = filterAndSortAgents(rows, "srv/agents", "lastSeenAt", "desc");

    expect(filtered).toHaveLength(1);
    expect(filtered[0]?.sessionId).toBe("sess-memory");
  });

  test("matches workspace name", () => {
    const filtered = filterAndSortAgents(rows, "payments", "lastSeenAt", "desc");

    expect(filtered).toHaveLength(1);
    expect(filtered[0]?.sessionId).toBe("sess-payments");
  });

  test("matches readable agent and session names", () => {
    const namedRows = [
      buildAgent({
        sessionId: "sess-auth",
        agentName: "Rowan Codex",
        sessionName: "auth refactor",
      }),
      buildAgent({
        sessionId: "sess-orders",
        agentName: "CI Worker",
        sessionName: "orders import",
      }),
    ];

    expect(filterAndSortAgents(namedRows, "auth refactor", "lastSeenAt", "desc")[0]?.sessionId).toBe("sess-auth");
    expect(filterAndSortAgents(namedRows, "rowan codex - auth refactor", "lastSeenAt", "desc")[0]?.sessionId).toBe("sess-auth");
    expect(filterAndSortAgents(namedRows, "ci worker", "lastSeenAt", "desc")[0]?.sessionId).toBe("sess-orders");
  });

  test("sorts by session and agent names", () => {
    const namedRows = [
      buildAgent({
        sessionId: "sess-z",
        agentName: "Beta Agent",
        sessionName: "zeta cleanup",
      }),
      buildAgent({
        sessionId: "sess-a",
        agentName: "Alpha Agent",
        sessionName: "auth refactor",
      }),
    ];

    expect(
      filterAndSortAgents(namedRows, "", "sessionName", "asc").map(
        (agent) => agent.sessionId,
      ),
    ).toEqual(["sess-a", "sess-z"]);
    expect(
      filterAndSortAgents(namedRows, "", "agentName", "desc").map(
        (agent) => agent.sessionId,
      ),
    ).toEqual(["sess-z", "sess-a"]);
  });

  test("builds the first-column identity label from agent, session, and machine names", () => {
    expect(
      displayAgentIdentityLabel(
        buildAgent({
          agentName: "Codex",
          sessionName: "auth refactor",
          hostname: "rowan-mbp",
        }),
      ),
    ).toBe("Codex - auth refactor");

    expect(
      displayAgentIdentityLabel(
        buildAgent({
          agentName: undefined,
          sessionName: "docs cleanup",
          hostname: "ci-runner-01",
        }),
      ),
    ).toBe("docs cleanup - ci-runner-01");

    expect(
      displayAgentIdentityLabel(
        buildAgent({
          agentName: "Claude",
          sessionName: undefined,
          hostname: "maya-mbp",
        }),
      ),
    ).toBe("Claude - maya-mbp");

    expect(
      displayAgentIdentityLabel(
        buildAgent({
          agentName: undefined,
          sessionName: undefined,
          hostname: "support-gateway-01",
        }),
      ),
    ).toBe("support-gateway-01");
  });

  test("sorts the default name column by the displayed identity label", () => {
    const namedRows = [
      buildAgent({
        sessionId: "sess-zulu",
        agentName: "Zulu Agent",
        sessionName: "checkout",
        hostname: "machine-z",
      }),
      buildAgent({
        sessionId: "sess-alpha",
        agentName: undefined,
        sessionName: "alpha task",
        hostname: "machine-a",
      }),
      buildAgent({
        sessionId: "sess-bravo",
        agentName: "Bravo Agent",
        sessionName: undefined,
        hostname: "machine-b",
      }),
      buildAgent({
        sessionId: "sess-charlie",
        agentName: undefined,
        sessionName: undefined,
        hostname: "charlie-host",
      }),
    ];

    expect(
      filterAndSortAgents(namedRows, "", "agentName", "asc").map(
        (agent) => agent.sessionId,
      ),
    ).toEqual(["sess-alpha", "sess-bravo", "sess-charlie", "sess-zulu"]);
  });
});
