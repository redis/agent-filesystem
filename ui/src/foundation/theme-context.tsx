import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";

export type ColorMode = "light" | "dark";

interface ThemeContextValue {
  colorMode: ColorMode;
  setColorMode: (colorMode: ColorMode) => void;
  toggleColorMode: () => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

const STORAGE_KEY = "afs_color_mode";
const VALID_COLOR_MODES: ReadonlyArray<ColorMode> = ["light", "dark"];

function isColorMode(value: string | null): value is ColorMode {
  return VALID_COLOR_MODES.includes(value as ColorMode);
}

function readStoredMode(): ColorMode {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (isColorMode(stored)) return stored;
  } catch {
    // ignore
  }
  return "light";
}

export function ColorModeProvider({ children }: { children: (colorMode: ColorMode) => ReactNode }) {
  const [colorMode, setColorMode] = useState<ColorMode>(readStoredMode);

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, colorMode);
    } catch {
      // ignore
    }
    document.documentElement.setAttribute("data-theme", colorMode);
  }, [colorMode]);

  useEffect(() => {
    function handleStorage(event: StorageEvent) {
      if (event.key !== STORAGE_KEY) return;
      if (isColorMode(event.newValue)) {
        setColorMode(event.newValue);
      }
    }

    window.addEventListener("storage", handleStorage);
    return () => window.removeEventListener("storage", handleStorage);
  }, []);

  const toggleColorMode = useCallback(() => {
    setColorMode((prev) => (prev === "light" ? "dark" : "light"));
  }, []);

  const value = useMemo<ThemeContextValue>(
    () => ({ colorMode, setColorMode, toggleColorMode }),
    [colorMode, toggleColorMode],
  );

  return (
    <ThemeContext.Provider value={value}>
      {children(colorMode)}
    </ThemeContext.Provider>
  );
}

export function useColorMode() {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error("useColorMode must be used inside ColorModeProvider");
  return ctx;
}
