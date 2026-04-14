import { beforeEach, describe, expect, test } from "vitest";
import { getDemoAFSClientForTesting } from "./afs";

describe("afsApi", () => {
  const paymentsDatabaseId = "db-payments-portal";
  const afsApi = getDemoAFSClientForTesting();

  beforeEach(() => {
    window.localStorage.clear();
    afsApi.resetDemo();
  });

  test("creates a workspace with an initial checkpoint", async () => {
    const workspace = await afsApi.createWorkspace({
      name: "demo-space",
      description: "Testing workspace creation",
      databaseId: "redis-agentfs-tests-us-test-1-0",
      cloudAccount: "Redis Cloud / Tests",
      databaseName: "agentfs-tests-us-test-1",
      region: "us-test-1",
      source: "blank",
    });

    expect(workspace.name).toBe("demo-space");
    expect(workspace.databaseId).toBe("redis-agentfs-tests-us-test-1-0");
    expect(workspace.databaseName).toBe("agentfs-tests-us-test-1");
    expect(workspace.savepoints).toHaveLength(1);
    expect(workspace.savepoints[0]?.name).toBe("initial");
  });

  test("updates a file and checkpoints it into a new savepoint", async () => {
    const workspace = await afsApi.getWorkspace(paymentsDatabaseId, "payments-portal");
    expect(workspace).not.toBeNull();

    await afsApi.updateWorkspaceFile({
      databaseId: paymentsDatabaseId,
      workspaceId: workspace?.id ?? "",
      path: "README.md",
      content: "# Updated",
    });

    await afsApi.createSavepoint({
      databaseId: paymentsDatabaseId,
      workspaceId: workspace?.id ?? "",
      name: "after-update",
      note: "Checkpoint after editing",
    });

    const savedWorkspace = await afsApi.getWorkspace(paymentsDatabaseId, "payments-portal");
    expect(savedWorkspace?.savepoints[0]?.name).toBe("after-update");
  });

  test("updates workspace metadata", async () => {
    const workspace = await afsApi.updateWorkspace({
      databaseId: paymentsDatabaseId,
      workspaceId: "payments-portal",
      description: "Updated description",
      cloudAccount: "Redis Cloud / Updated",
      databaseName: "payments-portal-prod-us-east-1",
      region: "us-east-2",
    });

    expect(workspace?.description).toBe("Updated description");
    expect(workspace?.cloudAccount).toBe("Redis Cloud / Updated");
    expect(workspace?.databaseName).toBe("payments-portal-prod-us-east-1");
    expect(workspace?.region).toBe("us-east-2");
  });
});
