import { describe, expect, test } from "vitest";
import { truncateMiddlePath } from "./changes-table-utils";

describe("truncateMiddlePath", () => {
  test("keeps short paths unchanged", () => {
    expect(truncateMiddlePath("/skills/script/deploy.sh")).toBe("/skills/script/deploy.sh");
    expect(truncateMiddlePath("README.md")).toBe("README.md");
  });

  test("keeps the leading directory and final two segments", () => {
    expect(truncateMiddlePath("/skills/vercel-deploy/script/deploy.sh")).toBe("/skills/.../script/deploy.sh");
    expect(truncateMiddlePath("skills/vercel-deploy/script/deploy.sh")).toBe("skills/.../script/deploy.sh");
  });

  test("preserves full path in edge-ish cases", () => {
    expect(truncateMiddlePath("")).toBe("");
    expect(truncateMiddlePath("/a/b/c/")).toBe("/a/b/c/");
  });
});
