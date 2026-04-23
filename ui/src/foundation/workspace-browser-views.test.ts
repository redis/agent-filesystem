import { describe, expect, test } from "vitest";
import { cloneInitialAFSState } from "./mocks/afs";
import {
  getDefaultWorkspaceBrowserView,
  getWorkspaceBrowserViewOptions,
} from "./workspace-browser-views";

describe("workspace browser views", () => {
  test("shows only head for a clean workspace whose head is the initial checkpoint", () => {
    const workspace = cloneInitialAFSState().workspaces.find((item) => item.id === "support-sandbox");
    expect(workspace).toBeDefined();

    expect(getWorkspaceBrowserViewOptions(workspace!)).toEqual([
      { value: "head", label: "head" },
    ]);
    expect(getDefaultWorkspaceBrowserView(workspace!)).toBe("head");
  });

  test("keeps a dirty working copy while hiding the duplicate head checkpoint entry", () => {
    const workspace = cloneInitialAFSState().workspaces.find((item) => item.id === "payments-portal");
    expect(workspace).toBeDefined();

    expect(getWorkspaceBrowserViewOptions(workspace!)).toEqual([
      { value: "working-copy", label: "working-copy" },
      { value: "head", label: "head" },
      { value: "checkpoint:sp-payments-baseline-ui", label: "baseline-ui" },
    ]);
    expect(getDefaultWorkspaceBrowserView(workspace!)).toBe("working-copy");
  });
});
