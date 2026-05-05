import { createContext, useContext } from "react";

export type SiteMode = "human" | "agent";

type SiteModeContextValue = {
  mode: SiteMode;
  setMode: (mode: SiteMode) => void;
};

export const SiteModeContext = createContext<SiteModeContextValue | null>(null);

export function useSiteMode() {
  const ctx = useContext(SiteModeContext);
  if (ctx == null) {
    throw new Error("useSiteMode must be used inside SiteModeContext.Provider");
  }
  return ctx;
}
