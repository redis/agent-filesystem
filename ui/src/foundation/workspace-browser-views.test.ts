import { describe, expect, test } from "vitest";
import { cloneInitialAFSState } from "./mocks/afs";
import {
  getActiveWorkspaceView,
  getDefaultWorkspaceBrowserView,
  getWorkspaceBrowserViewOptions,
} from "./workspace-browser-views";

describe("workspace browser views", () => {
  test("shows the active workspace for a clean workspace whose checkpoint matches active state", () => {
    const workspace = cloneInitialAFSState().workspaces.find((item) => item.id === "support-sandbox");
    expect(workspace).toBeDefined();

    expect(getWorkspaceBrowserViewOptions(workspace!)).toEqual([
      { value: "head", label: "Active workspace" },
    ]);
    expect(getActiveWorkspaceView(workspace!)).toBe("head");
    expect(getDefaultWorkspaceBrowserView(workspace!)).toBe("head");
  });

  test("shows dirty active state separately from every saved checkpoint", () => {
    const workspace = cloneInitialAFSState().workspaces.find((item) => item.id === "payments-portal");
    expect(workspace).toBeDefined();

    expect(getWorkspaceBrowserViewOptions(workspace!)).toEqual([
      { value: "working-copy", label: "Active workspace" },
      { value: "checkpoint:sp-payments-before-refactor", label: "before-refactor" },
      { value: "checkpoint:sp-payments-baseline-ui", label: "baseline-ui" },
    ]);
    expect(getActiveWorkspaceView(workspace!)).toBe("working-copy");
    expect(getDefaultWorkspaceBrowserView(workspace!)).toBe("working-copy");
  });
});
