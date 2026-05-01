import { Moon, Sun } from "lucide-react";
import styled from "styled-components";
import { useColorMode } from "../foundation/theme-context";

type ThemeModeToggleProps = {
  compact?: boolean;
  className?: string;
};

export function ThemeModeToggle({ compact = false, className }: ThemeModeToggleProps) {
  const { colorMode, toggleColorMode } = useColorMode();
  const isDark = colorMode === "dark";
  const title = isDark ? "Switch to light mode" : "Switch to dark mode";

  if (compact) {
    return (
      <IconButton
        type="button"
        role="switch"
        aria-checked={isDark}
        aria-label="Dark mode"
        title={title}
        className={className}
        onClick={toggleColorMode}
      >
        {isDark ? (
          <Moon size={15} strokeWidth={2.1} aria-hidden="true" />
        ) : (
          <Sun size={15} strokeWidth={2.1} aria-hidden="true" />
        )}
      </IconButton>
    );
  }

  return (
    <SwitchButton
      type="button"
      role="switch"
      aria-checked={isDark}
      aria-label="Dark mode"
      title={title}
      className={className}
      onClick={toggleColorMode}
    >
      <SwitchTrack $on={isDark}>
        <SwitchIcon $active={!isDark}>
          <Sun size={12} strokeWidth={2.1} aria-hidden="true" />
        </SwitchIcon>
        <SwitchIcon $active={isDark}>
          <Moon size={12} strokeWidth={2.1} aria-hidden="true" />
        </SwitchIcon>
        <SwitchThumb $on={isDark} />
      </SwitchTrack>
    </SwitchButton>
  );
}

const SwitchButton = styled.button`
  border: none;
  background: none;
  padding: 0;
  cursor: pointer;
  line-height: 0;

  &:focus-visible {
    outline: 2px solid var(--afs-focus);
    outline-offset: 3px;
    border-radius: 999px;
  }
`;

const SwitchTrack = styled.span<{ $on: boolean }>`
  position: relative;
  width: 46px;
  height: 24px;
  border-radius: 999px;
  background: var(--afs-panel);
  border: 1px solid var(--afs-line-strong);
  transition: background 0.2s ease, border-color 0.2s ease;
  display: flex;
  align-items: center;
  padding: 0 6px;
  justify-content: space-between;

  ${SwitchButton}:hover & {
    background: var(--afs-panel-strong);
  }

  [data-theme="dark"] & {
    background: var(--afs-bg-soft);
    border-color: var(--afs-line-strong);
  }
`;

const SwitchIcon = styled.span<{ $active: boolean }>`
  position: relative;
  z-index: 1;
  display: inline-flex;
  color: ${({ $active }) => ($active ? "var(--afs-ink)" : "var(--afs-muted)")};
  opacity: ${({ $active }) => ($active ? 1 : 0.78)};
  transition: color 0.2s ease, opacity 0.2s ease;

  [data-theme="dark"] & {
    color: ${({ $active }) => ($active ? "var(--afs-ink)" : "var(--afs-ink-soft)")};
    opacity: ${({ $active }) => ($active ? 1 : 0.86)};
  }
`;

const SwitchThumb = styled.span<{ $on: boolean }>`
  position: absolute;
  top: 2px;
  left: ${({ $on }) => ($on ? "24px" : "2px")};
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--afs-panel-strong);
  box-shadow: 0 1px 4px rgba(8, 6, 13, 0.22);
  transition: left 0.2s ease;

  [data-theme="dark"] & {
    background: var(--afs-panel);
    box-shadow: 0 1px 5px rgba(0, 0, 0, 0.35);
  }
`;

const IconButton = styled.button`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: 1px solid var(--afs-line);
  border-radius: 9px;
  background: transparent;
  color: var(--afs-ink-soft);
  cursor: pointer;
  transition: background 0.15s ease, border-color 0.15s ease, color 0.15s ease;

  &:hover {
    background: var(--afs-panel);
    border-color: var(--afs-line-strong);
    color: var(--afs-ink);
  }

  &:focus-visible {
    outline: 2px solid var(--afs-focus);
    outline-offset: 2px;
  }

  [data-theme="dark"] & {
    border-color: var(--afs-line-strong);
    color: var(--afs-ink);
  }
`;
