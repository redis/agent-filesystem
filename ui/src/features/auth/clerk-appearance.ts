import type { Appearance } from "@clerk/react";

const REDIS_RED = "#DC382C";

export function redisClerkAppearance(colorMode: "light" | "dark"): Appearance {
  const isDark = colorMode === "dark";
  const colorText = isDark ? "#E8E6DF" : "#282828";
  const colorTextSecondary = isDark ? "#8A8B85" : "#6D6E71";
  const colorBackground = isDark ? "#0E2330" : "#FFFFFF";
  const colorInputBackground = isDark ? "#091A23" : "#FFFFFF";
  const colorBorder = isDark ? "#284A5E" : "#D1D3D4";

  return {
    variables: {
      colorPrimary: REDIS_RED,
      colorBackground,
      colorInputBackground,
      colorInputText: colorText,
      colorText,
      colorTextSecondary,
      fontFamily: "inherit",
      fontSize: "16px",
    },
    elements: {
      rootBox: { width: "100%" },
      cardBox: { width: "100%", maxWidth: "100%" },
      card: {
        width: "100%",
        backgroundColor: colorBackground,
        border: `1px solid ${colorBorder}`,
        boxShadow: "none",
      },
      header: { display: "none" },
      formFieldInput: {
        backgroundColor: colorInputBackground,
        borderColor: colorBorder,
        color: colorText,
      },
      footerActionText: { color: colorTextSecondary },
      footerActionLink: { color: REDIS_RED },
    },
  };
}
