import type { Appearance } from "@clerk/react";

const REDIS_RED = "#DC382C";

export function redisClerkAppearance(_colorMode: "light" | "dark"): Appearance {
  return {
    variables: {
      colorPrimary: REDIS_RED,
      fontFamily: "inherit",
    },
  };
}
