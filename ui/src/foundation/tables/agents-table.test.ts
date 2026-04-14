import { describe, expect, test } from "vitest";
import type { AFSAgentSession } from "../types/afs";
import { filterAndSortAgents } from "./agents-table-utils";

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
});
