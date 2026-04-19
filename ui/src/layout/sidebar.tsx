import { useEffect, useRef, useState } from "react";
import { useLocation, useNavigate, useRouter } from "@tanstack/react-router";
import { SignOutButton } from "@clerk/react";
import { SideBar } from "@redis-ui/components";
import {
  ChevronDownIcon,
  DoubleChevronLeftIcon,
  DoubleChevronRightIcon,
} from "@redis-ui/icons";
import {
  RedisLogoDarkFullIcon,
  RedisLogoDarkMinIcon,
} from "@redis-ui/icons/multicolor";
import * as S from "./sidebar.styles";
import { bottomNavigationItems, isNavigationItemActive, navigationItems } from "./navigation-items";
import type { NavigationItem } from "./navigation-items";
import { useAuthSession } from "../foundation/auth-context";
import { useColorMode } from "../foundation/theme-context";
import { useDatabaseScope } from "../foundation/database-scope";

const SIDEBAR_LOCALSTORAGE_KEY = "afs_sidebar_open";

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

/** Routes that remain active even when no databases are configured. */
const ALWAYS_ENABLED_PATHS = new Set(["/", "/docs", "/agent-guide", "/downloads"]);

export function AppSidebar() {
  const location = useLocation();
  const navigate = useNavigate();
  const router = useRouter();
  const { colorMode, toggleColorMode } = useColorMode();
  const { databases, isLoading } = useDatabaseScope();
  const auth = useAuthSession();

  const [isExpanded, setIsExpanded] = useState(readInitialSidebarState);
  const [isProfileMenuOpen, setIsProfileMenuOpen] = useState(false);
  const profileMenuRef = useRef<HTMLDivElement | null>(null);

  const isEmpty = !isLoading && databases.length === 0;

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

  useEffect(() => {
    const handlePointerDown = (event: MouseEvent) => {
      if (profileMenuRef.current?.contains(event.target as Node)) {
        return;
      }
      setIsProfileMenuOpen(false);
    };

    if (!isProfileMenuOpen) {
      return;
    }

    window.addEventListener("mousedown", handlePointerDown);
    return () => window.removeEventListener("mousedown", handlePointerDown);
  }, [isProfileMenuOpen]);

  const handleNavigate = (path: string) => void navigate({ to: path });
  const handlePrefetch = (path: string) => {
    void router.preloadRoute({ to: path });
  };

  const renderRouteItem = (item: NavigationItem) => {
    const disabled = isEmpty && !ALWAYS_ENABLED_PATHS.has(item.path);
    return (
      <S.NavItemWrapper key={item.path} $disabled={disabled}>
        <SideBar.Item
          isActive={isNavigationItemActive(item, location.pathname)}
          tooltipProps={{
            text: disabled ? `${item.label} (add a database first)` : item.label,
            placement: "right",
          }}
          onMouseEnter={disabled ? undefined : () => handlePrefetch(item.path)}
          onFocus={disabled ? undefined : () => handlePrefetch(item.path)}
          onClick={disabled ? undefined : () => handleNavigate(item.path)}
        >
          <SideBar.Item.Icon icon={item.icon} aria-label={item.label} />
          <SideBar.Item.Text>{item.label}</SideBar.Item.Text>
        </SideBar.Item>
      </S.NavItemWrapper>
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
          <SideBar.ItemsContainer>{navigationItems.map(renderRouteItem)}</SideBar.ItemsContainer>

          <S.Spacer />
          <SideBar.Divider fullWidth />

          <SideBar.ItemsContainer>
            {bottomNavigationItems.map(renderRouteItem)}
          </SideBar.ItemsContainer>

        </SideBar.ScrollContainer>

        <SideBar.Footer>
          <>
            <SideBar.Divider fullWidth />
            <S.ProfileMenuContainer ref={profileMenuRef} $isExpanded={isExpanded}>
              <S.ProfileButton
                type="button"
                onClick={() => setIsProfileMenuOpen((open) => !open)}
                aria-expanded={isProfileMenuOpen}
                aria-haspopup="menu"
                title={auth.displayName}
                $isExpanded={isExpanded}
              >
                <S.ProfileAvatar>{auth.displayName.slice(0, 1).toUpperCase()}</S.ProfileAvatar>
                {isExpanded ? (
                  <S.ProfileTextGroup>
                    <S.ProfileName>{auth.displayName}</S.ProfileName>
                    {auth.secondaryLabel ? <S.ProfileMeta>{auth.secondaryLabel}</S.ProfileMeta> : null}
                  </S.ProfileTextGroup>
                ) : null}
                <S.ProfileChevron $isOpen={isProfileMenuOpen} $isExpanded={isExpanded}>
                  <ChevronDownIcon customSize="14px" />
                </S.ProfileChevron>
              </S.ProfileButton>
              {isProfileMenuOpen ? (
                <S.ProfileDropdown $isExpanded={isExpanded} role="menu">
                  {auth.supportsAccountAuth ? (
                    <SignOutButton redirectUrl="/login">
                      <S.ProfileMenuItem
                        type="button"
                        role="menuitem"
                        onClick={() => {
                          setIsProfileMenuOpen(false);
                        }}
                      >
                        Log out
                      </S.ProfileMenuItem>
                    </SignOutButton>
                  ) : (
                    <S.ProfileMenuItem type="button" role="menuitem" disabled>
                      Authentication managed externally
                    </S.ProfileMenuItem>
                  )}
                </S.ProfileDropdown>
              ) : null}
            </S.ProfileMenuContainer>
            <S.DarkModeRow>
              <S.DarkModeToggle
                role="switch"
                aria-checked={colorMode === "dark"}
                aria-label="Toggle dark mode"
                onClick={toggleColorMode}
              >
                <S.ToggleTrack $on={colorMode === "dark"}>
                  <S.ToggleSun $on={colorMode === "dark"}>☀</S.ToggleSun>
                  <S.ToggleMoon $on={colorMode === "dark"}>☾</S.ToggleMoon>
                  <S.ToggleThumb $on={colorMode === "dark"} />
                </S.ToggleTrack>
              </S.DarkModeToggle>
            </S.DarkModeRow>
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
