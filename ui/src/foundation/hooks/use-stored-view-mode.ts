import { useEffect, useState } from "react";

export type ViewMode = "table" | "cards";

function readStored(key: string, fallback: ViewMode): ViewMode {
  try {
    const stored = localStorage.getItem(key);
    if (stored === "table" || stored === "cards") return stored;
  } catch {
    // ignore
  }
  return fallback;
}

export function useStoredViewMode(
  key: string,
  fallback: ViewMode = "cards",
): [ViewMode, (mode: ViewMode) => void] {
  const [viewMode, setViewMode] = useState<ViewMode>(() => readStored(key, fallback));

  useEffect(() => {
    try {
      localStorage.setItem(key, viewMode);
    } catch {
      // ignore
    }
  }, [key, viewMode]);

  return [viewMode, setViewMode];
}
