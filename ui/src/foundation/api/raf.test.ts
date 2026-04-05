import { describe, expect, test, beforeEach } from "vitest";
import { rafApi } from "./raf";

describe("rafApi", () => {
  beforeEach(() => {
    window.localStorage.clear();
    rafApi.resetDemo();
  });

  test("creates a workspace with a default main session", async () => {
    const workspace = await rafApi.createWorkspace({
      name: "demo-space",
      description: "Testing workspace creation",
      cloudAccount: "Redis Cloud / Tests",
      databaseName: "agentfs-tests-us-test-1",
      region: "us-test-1",
      source: "blank",
    });

    expect(workspace.name).toBe("demo-space");
    expect(workspace.databaseName).toBe("agentfs-tests-us-test-1");
    expect(workspace.sessions).toHaveLength(1);
    expect(workspace.sessions[0]?.name).toBe("main");
  });

  test("updates a file and checkpoints it into a new savepoint", async () => {
    const workspace = await rafApi.getWorkspace("payments-portal");
    expect(workspace).not.toBeNull();

    const sessionId = workspace?.sessions[0]?.id;
    expect(sessionId).toBeTruthy();

    await rafApi.updateSessionFile({
      workspaceId: workspace.id,
      sessionId: sessionId ?? "",
      path: "README.md",
      content: "# Updated",
    });

    const dirtyWorkspace = await rafApi.getWorkspace("payments-portal");
    expect(dirtyWorkspace?.sessions[0]?.status).toBe("dirty");

    await rafApi.createSavepoint({
      workspaceId: dirtyWorkspace?.id ?? "",
      sessionId: sessionId ?? "",
      name: "after-update",
      note: "Checkpoint after editing",
    });

    const savedWorkspace = await rafApi.getWorkspace("payments-portal");
    expect(savedWorkspace?.sessions[0]?.status).toBe("clean");
    expect(savedWorkspace?.sessions[0]?.savepoints[0]?.name).toBe("after-update");
  });
});
