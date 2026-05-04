// Global drawer context — one slide-over drawer rendered at root, opened
// from anywhere. Two content modes:
//   - "onboarding" — agent/CLI prompts + workspace creation status
//   - "commands"   — per-page command reference (Create / Mount / etc.)
//
// Pages that have contextual commands call `useDrawerCommands(config)` to
// register what the global Help button opens. The Help button falls back to
// the onboarding drawer when no page has registered a config.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import type { ReactNode } from "react";
import type { OnboardingPath } from "../components/onboarding-drawer";

export type DrawerCommandSection = {
  title: string;
  description?: string;
  command: string;
};

export type CommandsDrawerConfig = {
  title: string;
  subline?: string;
  sections: DrawerCommandSection[];
};

export type DrawerState =
  | null
  | { kind: "onboarding"; path: OnboardingPath }
  | ({ kind: "commands" } & CommandsDrawerConfig);

type DrawerContextValue = {
  state: DrawerState;
  open: (s: NonNullable<DrawerState>) => void;
  close: () => void;
  pageHelp: CommandsDrawerConfig | null;
  setPageHelp: (s: CommandsDrawerConfig | null) => void;
};

const DrawerContext = createContext<DrawerContextValue | null>(null);

export function DrawerProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<DrawerState>(null);
  const [pageHelp, setPageHelp] = useState<CommandsDrawerConfig | null>(null);

  const open = useCallback((s: NonNullable<DrawerState>) => setState(s), []);
  const close = useCallback(() => setState(null), []);

  const value = useMemo<DrawerContextValue>(
    () => ({ state, open, close, pageHelp, setPageHelp }),
    [state, open, close, pageHelp],
  );

  return <DrawerContext.Provider value={value}>{children}</DrawerContext.Provider>;
}

export function useDrawer() {
  const ctx = useContext(DrawerContext);
  if (!ctx) throw new Error("useDrawer must be used within DrawerProvider");
  return ctx;
}

// Pages call this to register their contextual commands. The config is
// cleared on unmount so each page only contributes while mounted.
export function useDrawerCommands(config: CommandsDrawerConfig | null) {
  const { setPageHelp } = useDrawer();
  useEffect(() => {
    setPageHelp(config);
    return () => setPageHelp(null);
  }, [config, setPageHelp]);
}
