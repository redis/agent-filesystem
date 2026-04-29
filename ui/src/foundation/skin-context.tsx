import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

export type Skin = "classic" | "situation-room";

interface SkinContextValue {
  skin: Skin;
  setSkin: (skin: Skin) => void;
  toggleSkin: () => void;
  /** Whether the skin switcher should be exposed in the UI. Dev only for now. */
  isSwitcherEnabled: boolean;
}

const SkinContext = createContext<SkinContextValue | null>(null);

const STORAGE_KEY = "afs_skin";
const VALID_SKINS: ReadonlyArray<Skin> = ["classic", "situation-room"];

// New users default to situation-room. Existing users keep whatever they had.
const DEFAULT_SKIN: Skin = "situation-room";

function readStoredSkin(): Skin {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored && VALID_SKINS.includes(stored as Skin)) {
      return stored as Skin;
    }
  } catch {
    // ignore — likely SSR or a locked-down browser context
  }
  return DEFAULT_SKIN;
}

export function SkinProvider({ children }: { children: ReactNode }) {
  const [skin, setSkinState] = useState<Skin>(readStoredSkin);

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, skin);
    } catch {
      // ignore
    }
    document.documentElement.setAttribute("data-skin", skin);
  }, [skin]);

  const setSkin = useCallback((next: Skin) => setSkinState(next), []);
  const toggleSkin = useCallback(
    () => setSkinState((prev) => (prev === "classic" ? "situation-room" : "classic")),
    [],
  );

  const value = useMemo<SkinContextValue>(
    () => ({
      skin,
      setSkin,
      toggleSkin,
      isSwitcherEnabled: import.meta.env.DEV,
    }),
    [skin, setSkin, toggleSkin],
  );

  return <SkinContext.Provider value={value}>{children}</SkinContext.Provider>;
}

export function useSkin() {
  const ctx = useContext(SkinContext);
  if (!ctx) throw new Error("useSkin must be used inside SkinProvider");
  return ctx;
}
