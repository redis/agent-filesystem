import type { ReactNode } from "react";
import styled, { css, keyframes } from "styled-components";

type InactiveTone = "neutral" | "danger";

type StatusNameCellProps = {
  active: boolean;
  children: ReactNode;
  icon: ReactNode;
  inactiveTone?: InactiveTone;
  statusLabel?: string;
  statusTitle?: string;
};

export function StatusNameCell({
  active,
  children,
  icon,
  inactiveTone = "neutral",
  statusLabel,
  statusTitle,
}: StatusNameCellProps) {
  return (
    <Cell>
      <StatusDot
        $active={active}
        $inactiveTone={inactiveTone}
        aria-label={statusLabel ?? statusTitle}
        title={statusTitle}
      />
      <IconBox>{icon}</IconBox>
      <StatusNameStack>{children}</StatusNameStack>
    </Cell>
  );
}

export const StatusNameLine = styled.div`
  display: inline-flex;
  align-items: center;
  width: 100%;
  gap: 8px;
  min-width: 0;
`;

const Cell = styled.div`
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
`;

const IconBox = styled.span`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 18px;
  height: 18px;
  color: var(--afs-muted, #71717a);
`;

const StatusNameStack = styled.div`
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
`;

const pulse = keyframes`
  0%, 100% { opacity: 1; }
  50% { opacity: 0.45; }
`;

const inactiveColor = (tone: InactiveTone) => (tone === "danger" ? "#dc2626" : "#d1d5db");
const inactiveGlow = (tone: InactiveTone) =>
  tone === "danger" ? "rgba(220, 38, 38, 0.55)" : "transparent";

const StatusDot = styled.span<{ $active: boolean; $inactiveTone: InactiveTone }>`
  display: inline-block;
  flex: 0 0 auto;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: ${({ $active, $inactiveTone }) =>
    $active ? "#22c55e" : inactiveColor($inactiveTone)};
  box-shadow: ${({ $active, $inactiveTone }) =>
    $active ? "0 0 6px rgba(34, 197, 94, 0.55)" : `0 0 6px ${inactiveGlow($inactiveTone)}`};
  ${({ $active }) =>
    $active
      ? css`
          animation: ${pulse} 2s ease-in-out infinite;
        `
      : ""}
`;
