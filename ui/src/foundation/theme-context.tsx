import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";

type ColorMode = "light" | "dark";

interface ThemeContextValue {
  colorMode: ColorMode;
  toggleColorMode: () => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

const STORAGE_KEY = "afs_color_mode";

function readStoredMode(): ColorMode {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === "dark" || stored === "light") return stored;
  } catch {
    // ignore
  }
  return "light";
}

export function ColorModeProvider({ children }: { children: (colorMode: ColorMode) => ReactNode }) {
  const [colorMode, setColorMode] = useState<ColorMode>(readStoredMode);

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, colorMode);
    document.documentElement.setAttribute("data-theme", colorMode);
  }, [colorMode]);

  const toggleColorMode = useCallback(() => {
    setColorMode((prev) => (prev === "light" ? "dark" : "light"));
  }, []);

  return (
    <ThemeContext.Provider value={{ colorMode, toggleColorMode }}>
      {children(colorMode)}
    </ThemeContext.Provider>
  );
}

export function useColorMode() {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error("useColorMode must be used inside ColorModeProvider");
  return ctx;
}
