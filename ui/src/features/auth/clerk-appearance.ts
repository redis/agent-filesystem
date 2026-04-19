import type { Appearance } from "@clerk/react";

const REDIS_RED = "#DC382C";
const REDIS_RED_HOVER = "#B72217";

export function redisClerkAppearance(colorMode: "light" | "dark"): Appearance {
  const isDark = colorMode === "dark";
  return {
    variables: {
      colorPrimary: REDIS_RED,
      colorText: isDark ? "#F2F5F4" : "#0B1620",
      colorTextSecondary: isDark ? "#98AAB1" : "#556270",
      colorBackground: "transparent",
      colorInputBackground: isDark ? "#0B1B24" : "#FFFFFF",
      colorInputText: isDark ? "#F2F5F4" : "#0B1620",
      colorDanger: "#E55442",
      borderRadius: "10px",
      fontFamily: `inherit`,
      fontSize: "14px",
    },
    elements: {
      rootBox: {
        width: "100%",
      },
      cardBox: {
        width: "100%",
        maxWidth: "100%",
        boxShadow: "none",
        border: "none",
        background: "transparent",
      },
      card: {
        width: "100%",
        maxWidth: "100%",
        boxShadow: "none",
        border: "none",
        background: "transparent",
        padding: 0,
      },
      header: { display: "none" },
      main: {
        gap: "14px",
      },
      formFieldInput: {
        height: "44px",
      },
      formButtonPrimary: {
        background: REDIS_RED,
        textTransform: "none",
        fontWeight: 600,
        height: "44px",
        "&:hover": { background: REDIS_RED_HOVER },
      },
      footer: {
        background: "transparent",
        boxShadow: "none",
        borderTop: "none",
        padding: "12px 0 0",
      },
      footerAction: {
        background: "transparent",
      },
      footerActionLink: {
        color: REDIS_RED,
        "&:hover": { color: REDIS_RED_HOVER },
      },
      formFieldAction: {
        color: REDIS_RED,
        "&:hover": { color: REDIS_RED_HOVER },
      },
      identityPreviewEditButtonIcon: { color: REDIS_RED },
      formResendCodeLink: { color: REDIS_RED },
    },
  };
}
