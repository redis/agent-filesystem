import { SideBar } from "@redis-ui/components";
import styled from "styled-components";

export const SidebarContainer = styled.div`
  position: sticky;
  top: 0;
  z-index: 6;
  height: 100vh;
  flex-shrink: 0;

  [data-role="nav-bar"] {
    height: 100vh !important;
  }
`;

export const Spacer = styled.div`
  flex: 1;
`;

export const CenterSidebarHeader = styled(SideBar.Header)`
  box-shadow: none !important;
  height: auto !important;
  margin: 0 !important;

  > div {
    justify-content: flex-start;
    height: auto !important;
  }

  > button {
    color: ${({ theme }) => theme.semantic.color.text.neutral400} !important;
  }

  > button > svg {
    display: none;
  }
`;

export const HeaderToggleIcon = styled.div<{ $isExpanded: boolean }>`
  position: absolute;
  top: 50%;
  right: ${({ $isExpanded }) => ($isExpanded ? "1.6rem" : "calc(2.2rem * -0.45)")};
  transform: translateY(-50%);
  z-index: 7;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2.2rem;
  height: 2.2rem;
  color: var(--afs-muted);
  pointer-events: none;
`;

export const LogoWithName = styled.div`
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0px;
  padding: 10px 0px 2px 0px;
`;

export const LogoWrapper = styled.div`
  display: flex;
  cursor: pointer;
  overflow: hidden;
  margin-left: 10px;

  svg {
    width: 100px;
    height: 35px;
    display: block;
  }
`;

export const CollapsedLogoWrapper = styled.div`
  display: flex;
  justify-content: center;
  align-items: center;
  width: 100%;
  padding: 10px 0 8px;
`;

export const ProductName = styled.div`
  font-size: 14px;
  font-weight: 400;
  color: var(--afs-ink-soft);
  padding: 4px 10px 8px;
`;

export const NavItemWrapper = styled.div<{ $disabled?: boolean }>`
  ${({ $disabled }) =>
    $disabled
      ? `
    opacity: 0.35;
    pointer-events: none;
    user-select: none;
  `
      : ""}
`;

/* ── Dark-mode toggle switch ── */

export const DarkModeRow = styled.div`
  display: flex;
  justify-content: center;
  padding: 8px 0 4px;
`;

export const DarkModeToggle = styled.button`
  border: none;
  background: none;
  padding: 0;
  cursor: pointer;
  line-height: 0;
`;

export const ToggleTrack = styled.div<{ $on: boolean }>`
  position: relative;
  width: 44px;
  height: 24px;
  border-radius: 12px;
  background: ${({ $on }) => ($on ? "var(--afs-ink-soft)" : "var(--afs-panel-strong)")};
  border: 1px solid ${({ $on }) => ($on ? "transparent" : "var(--afs-line)")};
  transition: background 0.2s ease;
  display: flex;
  align-items: center;
  padding: 0 4px;
  justify-content: space-between;
`;

export const ToggleThumb = styled.div<{ $on: boolean }>`
  position: absolute;
  top: 2px;
  left: ${({ $on }) => ($on ? "22px" : "2px")};
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: ${({ $on }) => ($on ? "#0b1b24" : "var(--afs-ink)")};
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.2);
  transition: left 0.2s ease;
`;

export const ToggleSun = styled.span<{ $on: boolean }>`
  font-size: 12px;
  line-height: 1;
  opacity: ${({ $on }) => ($on ? 0.4 : 1)};
  transition: opacity 0.2s ease;
  user-select: none;
  z-index: 1;
`;

export const ToggleMoon = styled.span<{ $on: boolean }>`
  font-size: 12px;
  line-height: 1;
  opacity: ${({ $on }) => ($on ? 1 : 0.4)};
  transition: opacity 0.2s ease;
  user-select: none;
  z-index: 1;
  color: ${({ $on }) => ($on ? "#0b1b24" : "var(--afs-muted)")};
`;
