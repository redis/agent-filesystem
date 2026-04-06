import { beforeEach, describe, expect, test } from "vitest";
import { afsApi } from "./afs";

describe("afsApi", () => {
  beforeEach(() => {
    window.localStorage.clear();
    afsApi.resetDemo();
  });

  test("creates a workspace with an initial checkpoint", async () => {
    const workspace = await afsApi.createWorkspace({
      name: "demo-space",
      description: "Testing workspace creation",
      cloudAccount: "Redis Cloud / Tests",
      databaseName: "agentfs-tests-us-test-1",
      region: "us-test-1",
      source: "blank",
    });

    expect(workspace.name).toBe("demo-space");
    expect(workspace.databaseName).toBe("agentfs-tests-us-test-1");
    expect(workspace.savepoints).toHaveLength(1);
    expect(workspace.savepoints[0]?.name).toBe("initial");
  });

  test("updates a file and checkpoints it into a new savepoint", async () => {
    const workspace = await afsApi.getWorkspace("payments-portal");
    expect(workspace).not.toBeNull();

    await afsApi.updateWorkspaceFile({
      workspaceId: workspace?.id ?? "",
      path: "README.md",
      content: "# Updated",
    });

    const dirtyWorkspace = await afsApi.getWorkspace("payments-portal");
    expect(dirtyWorkspace?.draftState).toBe("dirty");

    await afsApi.createSavepoint({
      workspaceId: dirtyWorkspace?.id ?? "",
      name: "after-update",
      note: "Checkpoint after editing",
    });

    const savedWorkspace = await afsApi.getWorkspace("payments-portal");
    expect(savedWorkspace?.draftState).toBe("clean");
    expect(savedWorkspace?.savepoints[0]?.name).toBe("after-update");
  });
});
