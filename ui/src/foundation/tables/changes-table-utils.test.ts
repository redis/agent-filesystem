import { describe, expect, test } from "vitest";
import { displayPath } from "./changes-table-utils";

describe("displayPath", () => {
  test("keeps short paths unchanged", () => {
    expect(displayPath("/skills/script/deploy.sh")).toBe("/skills/script/deploy.sh");
    expect(displayPath("README.md")).toBe("README.md");
  });

  test("keeps long paths unchanged", () => {
    expect(displayPath("/skills/vercel-deploy/script/deploy.sh")).toBe(
      "/skills/vercel-deploy/script/deploy.sh",
    );
    expect(displayPath("skills/vercel-deploy/script/deploy.sh")).toBe(
      "skills/vercel-deploy/script/deploy.sh",
    );
  });

  test("preserves full path in edge-ish cases", () => {
    expect(displayPath("")).toBe("");
    expect(displayPath("/a/b/c/")).toBe("/a/b/c/");
  });
});
