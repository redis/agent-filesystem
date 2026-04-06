import { useEffect, useState } from "react";
import { useLocation, useNavigate } from "@tanstack/react-router";
import { SideBar } from "@redislabsdev/redis-ui-components";
import {
  ChevronLeftIcon,
  ChevronRightIcon,
  DocumentationIcon,
  DoubleChevronLeftIcon,
  DoubleChevronRightIcon,
  SupportIcon,
} from "@redislabsdev/redis-ui-icons";
import {
  RedisLogoDarkFullIcon,
  RedisLogoDarkMinIcon,
} from "@redislabsdev/redis-ui-icons/multicolor";
import * as S from "./sidebar.styles";
import {
  getNavigationPanel,
  getSidebarPanelForPath,
  isNavigationItemActive,
  navigationItems,
} from "./navigation-items";
import type {
  NavigationItem,
  NavigationRouteItem,
  SidebarPanelId,
} from "./navigation-items";

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
  const [activePanel, setActivePanel] = useState<SidebarPanelId>(() =>
    getSidebarPanelForPath(location.pathname),
  );
  const subPanels = navigationItems.filter(
    (item): item is Extract<NavigationItem, { kind: "panel" }> => item.kind === "panel",
  );
  const visiblePanel = isExpanded ? activePanel : "root";
  const activePanelIndex = ["root", ...subPanels.map((item) => item.panelId)].indexOf(visiblePanel);

  useEffect(() => {
    localStorage.setItem(SIDEBAR_LOCALSTORAGE_KEY, JSON.stringify(isExpanded));
  }, [isExpanded]);

  useEffect(() => {
    setActivePanel(getSidebarPanelForPath(location.pathname));
  }, [location.pathname]);

  useEffect(() => {
    const handleResize = () => {
      if (window.innerWidth < 1280) {
        setIsExpanded(false);
      }
    };

    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, []);

  const handleNavigate = (path: string) => void navigate({ to: path });

  const handlePanelOpen = (panelId: Exclude<SidebarPanelId, "root">) => {
    if (!isExpanded) {
      setIsExpanded(true);
    }

    setActivePanel(panelId);

    const panel = getNavigationPanel(panelId);
    const defaultRoute = panel?.children[0];
    if (defaultRoute) {
      handleNavigate(defaultRoute.path);
    }
  };

  const renderRouteItem = (item: NavigationRouteItem) => (
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

  const renderRootItem = (item: NavigationItem) => {
    if (item.kind === "route") {
      return renderRouteItem(item);
    }

    return (
      <SideBar.Item
        key={item.panelId}
        isActive={isNavigationItemActive(item, location.pathname)}
        tooltipProps={{ text: item.label, placement: "right" }}
        onClick={() => handlePanelOpen(item.panelId)}
      >
        <SideBar.Item.Icon icon={item.icon} aria-label={item.label} />
        <SideBar.Item.Text>{item.label}</SideBar.Item.Text>
        {isExpanded ? (
          <S.ItemChevron aria-hidden="true">
            <ChevronRightIcon customSize="16px" />
          </S.ItemChevron>
        ) : null}
      </SideBar.Item>
    );
  };

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
          <S.NavigationViewport>
            <S.NavigationTrack
              $activeIndex={Math.max(activePanelIndex, 0)}
              $panelCount={subPanels.length + 1}
            >
              <S.NavigationPanel>
                <SideBar.ItemsContainer>{navigationItems.map(renderRootItem)}</SideBar.ItemsContainer>
              </S.NavigationPanel>

              {subPanels.map((panel) => (
                <S.NavigationPanel key={panel.panelId}>
                  <S.SubmenuHeader
                    type="button"
                    aria-label={`Back to main navigation from ${panel.label}`}
                    onClick={() => setActivePanel("root")}
                  >
                    <S.BackButton>
                      <ChevronLeftIcon customSize="16px" />
                    </S.BackButton>
                    <S.SubmenuTitle>{panel.label}</S.SubmenuTitle>
                  </S.SubmenuHeader>
                  <SideBar.ItemsContainer>{panel.children.map(renderRouteItem)}</SideBar.ItemsContainer>
                </S.NavigationPanel>
              ))}
            </S.NavigationTrack>
          </S.NavigationViewport>

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
