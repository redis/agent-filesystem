import type { ReactNode } from "react";
import styled from "styled-components";
import { useStoredViewMode } from "../../foundation/hooks/use-stored-view-mode";
import { SiteAgentPane } from "./PublicAgentPane";
import { SiteModeContext, type SiteMode } from "./site-mode-context";
import { SiteModeSwitch } from "./SiteModeSwitch";

export function SiteModeFrame({ children }: { children: ReactNode }) {
  const [mode, setMode] = useStoredViewMode<SiteMode>(
    "afs_site_mode",
    "human",
    ["human", "agent"],
  );
  const showAgent = mode === "agent";

  return (
    <SiteModeContext.Provider value={{ mode, setMode }}>
      <Viewport>
        <Track $showAgent={showAgent}>
          <Panel aria-hidden={showAgent} $active={!showAgent}>
            {children}
          </Panel>
          <Panel aria-hidden={!showAgent} $active={showAgent}>
            <SiteAgentPane />
          </Panel>
        </Track>
        <ModeDock>
          <SiteModeSwitch compact />
        </ModeDock>
      </Viewport>
    </SiteModeContext.Provider>
  );
}

const Viewport = styled.div`
  position: relative;
  height: 100vh;
  min-height: 100vh;
  overflow: hidden;
`;

const Track = styled.div<{ $showAgent: boolean }>`
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  height: 100%;
  width: 200%;
  margin-left: ${({ $showAgent }) => ($showAgent ? "-100%" : "0%")};
  transition: margin-left 520ms cubic-bezier(0.22, 1, 0.36, 1);

  @media (prefers-reduced-motion: reduce) {
    transition: none;
  }
`;

const Panel = styled.section<{ $active: boolean }>`
  height: 100%;
  min-height: 0;
  overflow: hidden;
  min-width: 0;
  pointer-events: ${({ $active }) => ($active ? "auto" : "none")};
`;

const ModeDock = styled.div`
  position: fixed;
  left: 50%;
  bottom: 18px;
  z-index: 40;
  transform: translateX(-50%);

  @media (max-width: 640px) {
    bottom: 14px;
  }
`;
