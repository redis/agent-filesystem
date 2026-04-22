import { describe, expect, test } from "vitest";
import { studioTabSchema } from "./workspace-tabs";

describe("workspace tabs", () => {
  test("schema rejects removed legacy tab names", () => {
    expect(() => studioTabSchema.parse("files")).toThrow();
  });

  test("schema preserves current tab names", () => {
    expect(studioTabSchema.parse("checkpoints")).toBe("checkpoints");
  });
});
