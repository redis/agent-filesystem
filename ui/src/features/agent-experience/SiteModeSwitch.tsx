import styled from "styled-components";
import { useSiteMode } from "./site-mode-context";

type SiteModeSwitchProps = {
  compact?: boolean;
  shortLabels?: boolean;
  className?: string;
};

export function SiteModeSwitch({ compact = false, shortLabels = false, className }: SiteModeSwitchProps) {
  const { mode, setMode } = useSiteMode();
  const showAgent = mode === "agent";

  return (
    <SwitchWrap className={className} $compact={compact} aria-label="Switch site mode">
      <SwitchButton
        type="button"
        $active={!showAgent}
        $compact={compact}
        onClick={() => setMode("human")}
      >
        {shortLabels ? "H" : "Human"}
      </SwitchButton>
      <SwitchButton
        type="button"
        $active={showAgent}
        $compact={compact}
        onClick={() => setMode("agent")}
      >
        {shortLabels ? "A" : "Agent"}
      </SwitchButton>
    </SwitchWrap>
  );
}

const SwitchWrap = styled.div<{ $compact: boolean }>`
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: ${({ $compact }) => ($compact ? "3px" : "5px")};
  border: 1px solid color-mix(in srgb, var(--afs-line-strong) 78%, transparent);
  border-radius: 999px;
  overflow: hidden;
  background:
    linear-gradient(
      180deg,
      color-mix(in srgb, var(--afs-panel-strong) 88%, transparent),
      color-mix(in srgb, var(--afs-bg-1, var(--afs-bg)) 92%, transparent)
    );
  backdrop-filter: blur(14px);
  box-shadow:
    0 12px 28px rgba(7, 9, 14, 0.12),
    inset 0 1px 0 rgba(255, 255, 255, 0.16);
`;

const SwitchButton = styled.button<{
  $active: boolean;
  $compact: boolean;
}>`
  min-width: ${({ $compact }) => ($compact ? "24px" : "88px")};
  min-height: ${({ $compact }) => ($compact ? "30px" : "38px")};
  border: 1px solid ${({ $active }) =>
    $active
      ? "color-mix(in srgb, var(--afs-redis-red, #ff4438) 48%, transparent)"
      : "transparent"};
  border-radius: 999px;
  padding: 0 ${({ $compact }) => ($compact ? "8px" : "14px")};
  background: ${({ $active }) => {
    if (!$active) return "transparent";
    return "linear-gradient(135deg, color-mix(in srgb, var(--afs-redis-red, #ff4438) 78%, white) 0%, var(--afs-redis-red, #ff4438) 100%)";
  }};
  color: ${({ $active }) =>
    !$active
      ? "var(--afs-ink)"
      : "#fffaf8"};
  font-family: var(--afs-font-mono);
  font-size: ${({ $compact }) => ($compact ? "10px" : "12px")};
  font-weight: 600;
  line-height: 1;
  cursor: pointer;
  transition:
    background var(--afs-dur-fast) var(--afs-ease),
    color var(--afs-dur-fast) var(--afs-ease),
    border-color var(--afs-dur-fast) var(--afs-ease),
    transform var(--afs-dur-fast) var(--afs-ease),
    box-shadow var(--afs-dur-fast) var(--afs-ease);

  &:hover {
    transform: translateY(-1px);
    border-color: ${({ $active }) =>
      $active
        ? "color-mix(in srgb, var(--afs-redis-red, #ff4438) 58%, transparent)"
        : "var(--afs-line-strong)"};
    background: ${({ $active }) => {
      if ($active) {
        return "linear-gradient(135deg, color-mix(in srgb, var(--afs-redis-red, #ff4438) 72%, white) 0%, color-mix(in srgb, var(--afs-redis-red, #ff4438) 92%, black) 100%)";
      }
      return "color-mix(in srgb, var(--afs-redis-red, #ff4438) 10%, transparent)";
    }};
    box-shadow: ${({ $active }) =>
      $active ? "inset 0 1px 0 rgba(255, 255, 255, 0.18)" : "none"};
  }

  &:focus-visible {
    outline: 2px solid var(--afs-focus);
    outline-offset: 2px;
  }
`;
