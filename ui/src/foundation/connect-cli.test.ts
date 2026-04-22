import { describe, expect, test } from "vitest";
import { shouldEnableConnectCLIQueries } from "./connect-cli";

describe("shouldEnableConnectCLIQueries", () => {
  test("returns false while auth is still loading", () => {
    expect(shouldEnableConnectCLIQueries({ isLoading: true, isAuthenticated: false })).toBe(false);
  });

  test("returns false when the browser session is signed out", () => {
    expect(shouldEnableConnectCLIQueries({ isLoading: false, isAuthenticated: false })).toBe(false);
  });

  test("returns true once the browser session is authenticated", () => {
    expect(shouldEnableConnectCLIQueries({ isLoading: false, isAuthenticated: true })).toBe(true);
  });
});
