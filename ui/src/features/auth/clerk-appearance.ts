import type { Appearance } from "@clerk/react";

const REDIS_RED = "#DC382C";

export function redisClerkAppearance(_colorMode: "light" | "dark"): Appearance {
  return {
    variables: {
      colorPrimary: REDIS_RED,
      fontFamily: "inherit",
      fontSize: "16px",
    },
    elements: {
      rootBox: { width: "100%" },
      cardBox: { width: "100%", maxWidth: "100%" },
      card: { width: "100%" },
      header: { display: "none" },
    },
  };
}
