import { useEffect, useState } from "react";

// Generic over the set of valid view modes a caller cares about.
// Existing callers used "table" | "cards"; the agents page uses "table" | "map".
// The hook just persists whatever string the caller hands it; readers
// validate by passing a fallback.

export type ViewMode = string;

function readStored<T extends ViewMode>(
  key: string,
  fallback: T,
  isValid: (v: string) => v is T,
): T {
  try {
    const stored = localStorage.getItem(key);
    if (stored != null && isValid(stored)) return stored;
  } catch {
    // ignore
  }
  return fallback;
}

export function useStoredViewMode<T extends ViewMode = "table" | "cards">(
  key: string,
  fallback: T = "cards" as T,
  validModes: readonly T[] = ["table" as T, "cards" as T],
): [T, (mode: T) => void] {
  const isValid = (v: string): v is T => (validModes as readonly string[]).includes(v);
  const [viewMode, setViewMode] = useState<T>(() => readStored(key, fallback, isValid));

  useEffect(() => {
    try {
      localStorage.setItem(key, viewMode);
    } catch {
      // ignore
    }
  }, [key, viewMode]);

  return [viewMode, setViewMode];
}
