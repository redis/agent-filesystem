import { describe, expect, test } from "vitest";
import { normalizeStudioTab, studioTabSchema } from "./workspace-tabs";

describe("workspace tabs", () => {
  test("normalizes the legacy files tab to browse", () => {
    expect(normalizeStudioTab("files")).toBe("browse");
  });

  test("schema accepts the legacy files tab", () => {
    expect(studioTabSchema.parse("files")).toBe("browse");
  });

  test("schema preserves current tab names", () => {
    expect(studioTabSchema.parse("checkpoints")).toBe("checkpoints");
  });
});
