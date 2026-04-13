import { useEffect, useState } from "react";
import { useLocation, useNavigate } from "@tanstack/react-router";
import { SideBar } from "@redislabsdev/redis-ui-components";
import {
  DocumentationIcon,
  DoubleChevronLeftIcon,
  DoubleChevronRightIcon,
  SupportIcon,
} from "@redislabsdev/redis-ui-icons";
import {
  RedisLogoDarkFullIcon,
  RedisLogoDarkMinIcon,
} from "@redislabsdev/redis-ui-icons/multicolor";
import { useDatabaseScope } from "../foundation/database-scope";
import * as S from "./sidebar.styles";
import { isNavigationItemActive, navigationItems } from "./navigation-items";
import type { NavigationItem } from "./navigation-items";

const SIDEBAR_LOCALSTORAGE_KEY = "afs_sidebar_open";

const bottomItems = [
  { label: "Support", icon: SupportIcon },
  { label: "Documentation", icon: DocumentationIcon },
];

function readInitialSidebarState() {
  const stored = localStorage.getItem(SIDEBAR_LOCALSTORAGE_KEY);
  if (stored === null) return true;

  try {
    return JSON.parse(stored) as boolean;
  } catch {
    localStorage.removeItem(SIDEBAR_LOCALSTORAGE_KEY);
    return true;
  }
}

export function AppSidebar() {
  const location = useLocation();
  const navigate = useNavigate();

  const [isExpanded, setIsExpanded] = useState(readInitialSidebarState);

  useEffect(() => {
    localStorage.setItem(SIDEBAR_LOCALSTORAGE_KEY, JSON.stringify(isExpanded));
  }, [isExpanded]);

  useEffect(() => {
    const handleResize = () => {
      if (window.innerWidth < 1280) {
        setIsExpanded(false);
      }
    };

    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, []);

  const { selectedDatabase } = useDatabaseScope();

  const visibleItems = selectedDatabase == null
    ? navigationItems.filter((item) => item.kind !== "route" || (item.path !== "/workspaces" && item.path !== "/activity"))
    : navigationItems;

  const handleNavigate = (path: string) => void navigate({ to: path });

  const renderRouteItem = (item: NavigationItem) => (
    <SideBar.Item
      key={item.path}
      isActive={isNavigationItemActive(item, location.pathname)}
      tooltipProps={{ text: item.label, placement: "right" }}
      onClick={() => handleNavigate(item.path)}
    >
      <SideBar.Item.Icon icon={item.icon} aria-label={item.label} />
      <SideBar.Item.Text>{item.label}</SideBar.Item.Text>
    </SideBar.Item>
  );

  return (
    <S.SidebarContainer>
      <SideBar isExpanded={isExpanded}>
        <S.CenterSidebarHeader onToggle={() => setIsExpanded((prev) => !prev)}>
          {isExpanded ? (
            <S.LogoWithName>
              <S.LogoWrapper>
                <RedisLogoDarkFullIcon />
              </S.LogoWrapper>
              <S.ProductName>Agent Filesystem</S.ProductName>
            </S.LogoWithName>
          ) : (
            <S.CollapsedLogoWrapper>
              <RedisLogoDarkMinIcon customSize="28px" />
            </S.CollapsedLogoWrapper>
          )}
          <S.HeaderToggleIcon $isExpanded={isExpanded} aria-hidden="true">
            {isExpanded ? (
              <DoubleChevronLeftIcon customSize="14px" />
            ) : (
              <DoubleChevronRightIcon customSize="14px" />
            )}
          </S.HeaderToggleIcon>
        </S.CenterSidebarHeader>

        <SideBar.ScrollContainer>
          <SideBar.ItemsContainer>{visibleItems.map(renderRouteItem)}</SideBar.ItemsContainer>

          <S.Spacer />
          <SideBar.Divider fullWidth />

          <SideBar.ItemsContainer>
            {bottomItems.map((item) => (
              <SideBar.Item
                key={item.label}
                tooltipProps={{ text: item.label, placement: "right" }}
                onClick={() => {}}
              >
                <SideBar.Item.Icon icon={item.icon} aria-label={item.label} />
                <SideBar.Item.Text>{item.label}</SideBar.Item.Text>
              </SideBar.Item>
            ))}
          </SideBar.ItemsContainer>
        </SideBar.ScrollContainer>

        <SideBar.Footer>
          <>
            <SideBar.Divider fullWidth />
            <SideBar.Footer.MetaData>
              <SideBar.Footer.Link href="#" target="_blank" rel="noreferrer">
                Terms
              </SideBar.Footer.Link>
              <SideBar.Footer.Text>&copy; 2026 Redis</SideBar.Footer.Text>
            </SideBar.Footer.MetaData>
          </>
        </SideBar.Footer>
      </SideBar>
    </S.SidebarContainer>
  );
}
