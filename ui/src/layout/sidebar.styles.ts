import { SideBar } from "@redislabsdev/redis-ui-components";
import styled from "styled-components";

export const SidebarContainer = styled.div`
  position: relative;
  z-index: 6;
  height: 100vh;

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
  color: ${({ theme }) => theme.semantic.color.text.neutral500};
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
  color: ${({ theme }) => theme.semantic.color.text.neutral700};
  padding: 4px 10px 8px;
`;
