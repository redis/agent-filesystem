import { ChevronRight, Lightbulb } from "lucide-react";
import { useEffect, useState } from "react";
import styled, { keyframes } from "styled-components";
import { SurfaceCard } from "../../components/card-shell";

export const QUICKSTART_TIP_DISMISSED_KEY = "afs_home_quickstart_tip_dismissed";

function readDismissedState() {
  return window.localStorage.getItem(QUICKSTART_TIP_DISMISSED_KEY) === "1";
}

export function QuickstartTipCard({ onOpen }: { onOpen: () => void }) {
  const [dismissed, setDismissed] = useState(() => readDismissedState());

  useEffect(() => {
    if (dismissed) {
      window.localStorage.setItem(QUICKSTART_TIP_DISMISSED_KEY, "1");
      return;
    }

    window.localStorage.removeItem(QUICKSTART_TIP_DISMISSED_KEY);
  }, [dismissed]);

  if (dismissed) {
    return null;
  }

  return (
    <TipCard role="note" aria-label="Getting Started tip">
      <TipCopy>
        <TipIcon aria-hidden="true">
          <Lightbulb size={14} strokeWidth={2.1} />
        </TipIcon>
        <TipText>
          Need the quick start again? Open <strong>Getting Started</strong>{" "}
          from the top-right button.
        </TipText>
      </TipCopy>

      <TipLaunchButton
        type="button"
        onClick={onOpen}
        aria-label="Open Getting Started"
      >
        <ButtonVisual aria-hidden="true">
          <TerminalCursor>_</TerminalCursor>
          <ChevronRight size={12} strokeWidth={2.4} />
        </ButtonVisual>
        <LaunchLabel>
          <LaunchTitle>Getting Started</LaunchTitle>
          <LaunchMeta>top right</LaunchMeta>
        </LaunchLabel>
      </TipLaunchButton>

      <DismissButton
        type="button"
        onClick={() => setDismissed(true)}
        aria-label="Dismiss Getting Started tip"
      >
        ×
      </DismissButton>
    </TipCard>
  );
}

const cursorBlink = keyframes`
  0%, 49% { opacity: 1; }
  50%, 100% { opacity: 0.28; }
`;

const TipCard = styled(SurfaceCard).attrs({ as: "section" })`
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 10px 12px 10px 14px;

  @media (max-width: 720px) {
    flex-wrap: wrap;
    align-items: flex-start;
  }
`;

const TipCopy = styled.div`
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1 1 280px;

  @media (max-width: 640px) {
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
  }
`;

const TipIcon = styled.span`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  flex: 0 0 auto;
  border-radius: 999px;
  background: color-mix(in srgb, var(--afs-accent, #2563eb) 12%, transparent);
  color: var(--afs-accent, #2563eb);
`;

const TipText = styled.p`
  margin: 0;
  color: var(--afs-ink);
  font-size: 13px;
  line-height: 1.45;
`;

const TipLaunchButton = styled.button`
  display: inline-flex;
  align-items: center;
  gap: 10px;
  flex: 0 0 auto;
  border: 1px solid var(--afs-line);
  border-radius: 11px;
  background: color-mix(in srgb, var(--afs-panel) 84%, white);
  padding: 7px 10px 7px 8px;
  color: inherit;
  cursor: pointer;
  transition: border-color 120ms ease, background 120ms ease, transform 120ms ease;

  &:hover {
    border-color: var(--afs-accent, #2563eb);
    background: var(--afs-panel);
    transform: translateY(-1px);
  }

  &:focus-visible {
    outline: 2px solid var(--afs-focus);
    outline-offset: 2px;
  }
`;

const ButtonVisual = styled.span`
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 5px 7px 5px 8px;
  border-radius: 8px;
  border: 1px solid #1f2937;
  background: #0d1117;
  color: #4ade80;
  box-shadow: inset 0 0 0 1px rgba(74, 222, 128, 0.03);
`;

const TerminalCursor = styled.span`
  display: inline-flex;
  align-items: flex-end;
  width: 9px;
  height: 12px;
  color: #4ade80;
  font-family: var(--afs-mono, "SF Mono", "Fira Code", monospace);
  font-size: 14px;
  font-weight: 700;
  line-height: 1;
  animation: ${cursorBlink} 1.1s steps(1) infinite;
`;

const LaunchLabel = styled.span`
  display: grid;
  gap: 1px;
  text-align: left;
`;

const LaunchTitle = styled.span`
  color: var(--afs-ink);
  font-size: 12px;
  font-weight: 700;
  line-height: 1.1;
`;

const LaunchMeta = styled.span`
  color: var(--afs-muted);
  font-size: 10px;
  line-height: 1.1;
  text-transform: uppercase;
  letter-spacing: 0.06em;
`;

const DismissButton = styled.button`
  width: 26px;
  height: 26px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  border: 1px solid var(--afs-line);
  border-radius: 8px;
  background: transparent;
  color: var(--afs-muted);
  font-size: 16px;
  line-height: 1;
  cursor: pointer;
  transition: border-color 120ms ease, color 120ms ease, background 120ms ease;

  &:hover {
    border-color: var(--afs-line-strong);
    color: var(--afs-ink);
    background: color-mix(in srgb, var(--afs-panel) 70%, white);
  }

  &:focus-visible {
    outline: 2px solid var(--afs-focus);
    outline-offset: 2px;
  }
`;
