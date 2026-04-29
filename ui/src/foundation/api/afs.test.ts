import { beforeEach, describe, expect, test } from "vitest";
import { getDemoAFSClientForTesting } from "./afs";

describe("afsApi", () => {
  const paymentsDatabaseId = "db-payments-portal";
  const paymentsDatabaseName = "payments-portal-us-east-1";
  const afsApi = getDemoAFSClientForTesting();

  beforeEach(() => {
    window.localStorage.clear();
    afsApi.resetDemo();
  });

  test("creates a workspace with an initial checkpoint", async () => {
    const workspace = await afsApi.createWorkspace({
      name: "demo-space",
      description: "Testing workspace creation",
      databaseId: paymentsDatabaseId,
      cloudAccount: "Redis Cloud / Tests",
      databaseName: paymentsDatabaseName,
      region: "us-east-1",
      source: "blank",
    });

    expect(workspace.name).toBe("demo-space");
    expect(workspace.databaseId).toBe(paymentsDatabaseId);
    expect(workspace.databaseName).toBe(paymentsDatabaseName);
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

  test("compares a checkpoint with the live workspace", async () => {
    const diff = await afsApi.getWorkspaceDiff({
      databaseId: paymentsDatabaseId,
      workspaceId: "payments-portal",
      base: "checkpoint:sp-payments-before-refactor",
      head: "working-copy",
    });

    expect(diff.summary.total).toBeGreaterThan(0);
    expect(diff.summary.updated).toBeGreaterThan(0);
    expect(diff.entries.some((entry) => entry.path === "/README.md")).toBe(true);
    expect(diff.entries.some((entry) => entry.path === "/src/routes/editor.tsx")).toBe(true);
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

  test("persists workspace versioning policy in demo mode", async () => {
    await afsApi.updateWorkspaceVersioningPolicy({
      databaseId: paymentsDatabaseId,
      workspaceId: "payments-portal",
      policy: {
        mode: "paths",
        includeGlobs: ["src/**"],
        excludeGlobs: ["**/*.log"],
        maxVersionsPerFile: 10,
        maxAgeDays: 14,
        maxTotalBytes: 8192,
        largeFileCutoffBytes: 2048,
      },
    });

    const policy = await afsApi.getWorkspaceVersioningPolicy({
      databaseId: paymentsDatabaseId,
      workspaceId: "payments-portal",
    });

    expect(policy).toEqual({
      mode: "paths",
      includeGlobs: ["src/**"],
      excludeGlobs: ["**/*.log"],
      maxVersionsPerFile: 10,
      maxAgeDays: 14,
      maxTotalBytes: 8192,
      largeFileCutoffBytes: 2048,
    });
  });
});
