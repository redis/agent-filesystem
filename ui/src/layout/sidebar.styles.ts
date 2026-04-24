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

export const ProfileMenuContainer = styled.div<{ $isExpanded: boolean }>`
  position: relative;
  padding: ${({ $isExpanded }) => ($isExpanded ? "12px 12px 8px" : "12px 8px 8px")};
`;

export const ProfileButton = styled.button<{ $isExpanded: boolean }>`
  width: 100%;
  display: flex;
  align-items: center;
  gap: ${({ $isExpanded }) => ($isExpanded ? "10px" : "0")};
  justify-content: ${({ $isExpanded }) => ($isExpanded ? "flex-start" : "center")};
  border: 1px solid var(--afs-line);
  border-radius: 10px;
  background: var(--afs-panel);
  padding: ${({ $isExpanded }) => ($isExpanded ? "8px 10px" : "8px")};
  color: var(--afs-ink);
  cursor: pointer;
  text-align: left;
  transition: border-color 0.15s ease, background 0.15s ease;

  &:hover {
    border-color: var(--afs-line-strong);
    background: var(--afs-panel-strong);
  }

  &:focus-visible {
    outline: 2px solid var(--afs-focus);
    outline-offset: 2px;
  }
`;

export const ProfileAvatar = styled.div`
  width: 28px;
  height: 28px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  background: var(--afs-accent-soft);
  color: var(--afs-accent);
  font-size: 12px;
  font-weight: 700;
`;

export const ProfileTextGroup = styled.div`
  min-width: 0;
  display: grid;
  gap: 1px;
  flex: 1;
`;

export const ProfileName = styled.div`
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  font-weight: 600;
  color: var(--afs-ink);
`;

export const ProfileMeta = styled.div`
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  color: var(--afs-muted);
`;

export const ProfileChevron = styled.div<{ $isOpen: boolean; $isExpanded: boolean }>`
  margin-left: auto;
  display: ${({ $isExpanded }) => ($isExpanded ? "inline-flex" : "none")};
  align-items: center;
  justify-content: center;
  color: var(--afs-muted);
  transform: rotate(${({ $isOpen }) => ($isOpen ? "180deg" : "0deg")});
  transition: transform 0.18s ease;
`;

export const ProfileDropdown = styled.div<{ $isExpanded: boolean }>`
  position: absolute;
  left: ${({ $isExpanded }) => ($isExpanded ? "12px" : "calc(100% + 8px)")};
  right: ${({ $isExpanded }) => ($isExpanded ? "12px" : "auto")};
  bottom: calc(100% - 4px);
  min-width: 180px;
  border: 1px solid var(--afs-line);
  border-radius: 10px;
  background: var(--afs-panel-strong);
  box-shadow: var(--afs-shadow);
  padding: 4px;
  z-index: 20;
`;

export const ProfileMenuItem = styled.button`
  width: 100%;
  border: none;
  background: transparent;
  border-radius: 6px;
  padding: 8px 10px;
  text-align: left;
  font-size: 13px;
  font-weight: 500;
  color: var(--afs-ink);
  cursor: pointer;

  &:hover {
    background: var(--afs-accent-soft);
    color: var(--afs-accent);
  }

  &:disabled {
    color: var(--afs-muted);
    cursor: not-allowed;
  }
`;

export const SignInButtonWrapper = styled.div<{ $isExpanded: boolean }>`
  display: flex;
  justify-content: ${({ $isExpanded }) => ($isExpanded ? "flex-start" : "center")};
  padding: ${({ $isExpanded }) => ($isExpanded ? "12px 12px 8px" : "12px 8px 8px")};

  > button {
    width: ${({ $isExpanded }) => ($isExpanded ? "100%" : "auto")};
  }
`;

export const DarkModeRow = styled.div<{ $isExpanded: boolean }>`
  display: flex;
  justify-content: center;
  padding: ${({ $isExpanded }) => ($isExpanded ? "10px 12px 4px" : "10px 8px 4px")};
`;

export const DarkModeToggle = styled.button`
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

export const ToggleTrack = styled.span<{ $on: boolean }>`
  position: relative;
  width: 52px;
  height: 28px;
  border-radius: 999px;
  background: ${({ $on }) => ($on ? "var(--afs-ink-soft)" : "var(--afs-panel-strong)")};
  border: 1px solid ${({ $on }) => ($on ? "transparent" : "var(--afs-line)")};
  transition: background 0.2s ease, border-color 0.2s ease;
  display: flex;
  align-items: center;
  padding: 0 7px;
  justify-content: space-between;
`;

export const ToggleIcon = styled.span<{ $active: boolean }>`
  position: relative;
  z-index: 1;
  display: inline-flex;
  color: ${({ $active }) => ($active ? "var(--afs-ink)" : "var(--afs-muted)")};
  opacity: ${({ $active }) => ($active ? 1 : 0.54)};
  transition: color 0.2s ease, opacity 0.2s ease;
`;

export const ToggleThumb = styled.span<{ $on: boolean }>`
  position: absolute;
  top: 3px;
  left: ${({ $on }) => ($on ? "25px" : "3px")};
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--afs-bg-soft);
  box-shadow: 0 1px 4px rgba(8, 6, 13, 0.22);
  transition: left 0.2s ease;
`;
