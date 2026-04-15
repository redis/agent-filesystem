import { useEffect, type ReactNode } from "react";

export function BackgroundPatternProvider({ children }: { children: ReactNode }) {
  useEffect(() => {
    document.documentElement.setAttribute("data-bg-pattern", "grid");
  }, []);

  return <>{children}</>;
}
