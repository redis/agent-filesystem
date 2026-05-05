import {
  createRootRoute,
  Outlet,
  useLocation,
  useNavigate,
  useRouter,
} from "@tanstack/react-router";
import { lazy, Suspense, useEffect } from "react";
import styled from "styled-components";
import { SiteModeFrame } from "../features/agent-experience/SiteModeFrame";
import { RouteErrorBoundary } from "../error-boundaries/route-error-boundary";
import { isCloudAdminConfig, useAuthSession } from "../foundation/auth-context";
import { BackgroundPatternProvider } from "../foundation/background-pattern";
import { AppBar } from "../layout/app-bar";
import { isPublicMarketingPath } from "../layout/public-routes";
import { PublicShell } from "../layout/public-shell";
import { BgFx } from "../layout/situation-room-chrome";
import {
  adminNavigationItem,
  bottomNavigationItems,
  navigationItems,
} from "../layout/navigation-items";
import { FlexRow, FlexColItem, MainContainer } from "../layout/layout.styles";
import { DrawerProvider } from "../foundation/drawer-context";
import { GlobalDrawer } from "../components/global-drawer";

const AppSidebar = lazy(() =>
  import("../layout/sidebar").then((module) => ({
    default: module.AppSidebar,
  })),
);

const AUTH_PATH_PREFIXES = [
  "/login",
  "/signup",
  "/forgot-password",
  "/sso-callback",
];
const PUBLIC_APP_PATHS = new Set(["/connect-cli"]);

function isAuthPath(pathname: string) {
  return AUTH_PATH_PREFIXES.some(
    (prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`),
  );
}

function RouteWarmup() {
  const router = useRouter();
  const location = useLocation();
  const auth = useAuthSession();

  useEffect(() => {
    const targets = [
      ...navigationItems,
      adminNavigationItem,
      ...bottomNavigationItems,
    ]
      .filter((item) => !item.adminOnly || isCloudAdminConfig(auth.config))
      .map((item) => item.path)
      .filter(
        (path, index, values) =>
          path !== location.pathname && values.indexOf(path) === index,
      );

    const preload = () => {
      for (const target of targets) {
        void router.preloadRoute({ to: target });
      }
    };

    if ("requestIdleCallback" in window) {
      const handle = window.requestIdleCallback(preload, { timeout: 1200 });
      return () => window.cancelIdleCallback(handle);
    }

    const timeout = window.setTimeout(preload, 250);
    return () => window.clearTimeout(timeout);
  }, [auth.config, location.pathname, router]);

  return null;
}

function RootLayout() {
  const location = useLocation();
  const auth = useAuthSession();
  const navigate = useNavigate();

  const onAuthRoute = isAuthPath(location.pathname);
  const isPublicAppPath = PUBLIC_APP_PATHS.has(location.pathname);
  const isMarketingPath = isPublicMarketingPath(location.pathname);
  const forcePublicShell = location.pathname === "/home";
  const showPublicMarketingShell =
    forcePublicShell ||
    (isMarketingPath && (auth.isLoading || auth.isSignedOut));

  useEffect(() => {
    if (auth.isLoading) return;
    if (onAuthRoute) return;
    if (isPublicAppPath) return;
    if (isMarketingPath) return;
    if (!auth.isSignedOut) return;
    const target = location.pathname + location.searchStr;
    void navigate({
      to: "/login",
      search: target && target !== "/" ? { redirect: target } : undefined,
      replace: true,
    });
  }, [
    auth.isLoading,
    auth.isSignedOut,
    isMarketingPath,
    isPublicAppPath,
    location.pathname,
    location.searchStr,
    navigate,
    onAuthRoute,
  ]);

  const humanView = onAuthRoute ? (
    <Outlet />
  ) : showPublicMarketingShell ? (
    <PublicShell>
      <Outlet />
    </PublicShell>
  ) : (
    <>
      <RouteWarmup />
      <FlexRow>
        <Suspense fallback={<SidebarPlaceholder aria-hidden="true" />}>
          <AppSidebar />
        </Suspense>
        <FlexColItem>
          <AppBar />
          <MainContainer>
            {auth.isLoading && !isPublicAppPath ? (
              <CenteredState>
                <LoadingSpinner data-testid="loader--spinner" />
              </CenteredState>
            ) : auth.isSignedOut && !isPublicAppPath ? (
              <CenteredState>
                <LoadingSpinner data-testid="loader--spinner" />
              </CenteredState>
            ) : (
              <Outlet />
            )}
          </MainContainer>
        </FlexColItem>
      </FlexRow>
    </>
  );

  return (
    <BackgroundPatternProvider>
      <DrawerProvider>
        <BgFx />
        <SiteModeFrame>{humanView}</SiteModeFrame>
        <GlobalDrawer />
      </DrawerProvider>
    </BackgroundPatternProvider>
  );
}

function RootErrorBoundary(props: Parameters<typeof RouteErrorBoundary>[0]) {
  return <RouteErrorBoundary {...props} fullPage />;
}

export const Route = createRootRoute({
  component: RootLayout,
  errorComponent: RootErrorBoundary,
});

const CenteredState = styled.div`
  min-height: calc(100vh - 140px);
  display: grid;
  place-items: center;
  padding: 32px;
`;

const LoadingSpinner = styled.div`
  width: 28px;
  height: 28px;
  border: 3px solid var(--afs-line-strong);
  border-top-color: var(--afs-accent);
  border-radius: 999px;
  animation: afs-spin 0.8s linear infinite;

  @keyframes afs-spin {
    to {
      transform: rotate(360deg);
    }
  }
`;

const SidebarPlaceholder = styled.aside`
  flex: 0 0 252px;
  height: 100%;
  border-right: 1px solid var(--afs-line);
  background: var(--afs-bg-1);

  @media (max-width: 1279px) {
    flex-basis: 72px;
  }
`;
