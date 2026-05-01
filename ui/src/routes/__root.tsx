import { createRootRoute, Outlet, useLocation, useNavigate, useRouter } from "@tanstack/react-router";
import { useEffect } from "react";
import styled from "styled-components";
import { Loader } from "@redis-ui/components";
import { RouteErrorBoundary } from "../error-boundaries/route-error-boundary";
import { isCloudAdminConfig, useAuthSession } from "../foundation/auth-context";
import { BackgroundPatternProvider } from "../foundation/background-pattern";
import { AppSidebar } from "../layout/sidebar";
import { AppBar } from "../layout/app-bar";
import { isPublicMarketingPath } from "../layout/public-routes";
import { PublicShell } from "../layout/public-shell";
import { BgFx } from "../layout/situation-room-chrome";
import { adminNavigationItem, bottomNavigationItems, navigationItems } from "../layout/navigation-items";
import {
  FlexRow,
  FlexColItem,
  MainContainer,
} from "../layout/layout.styles";

const AUTH_PATH_PREFIXES = ["/login", "/signup", "/forgot-password", "/sso-callback"];
const PUBLIC_APP_PATHS = new Set(["/connect-cli"]);

function isAuthPath(pathname: string) {
  return AUTH_PATH_PREFIXES.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`));
}

function RouteWarmup() {
  const router = useRouter();
  const location = useLocation();
  const auth = useAuthSession();

  useEffect(() => {
    const targets = [...navigationItems, adminNavigationItem, ...bottomNavigationItems]
      .filter((item) => !item.adminOnly || isCloudAdminConfig(auth.config))
      .map((item) => item.path)
      .filter((path, index, values) => path !== location.pathname && values.indexOf(path) === index);

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
  }, [auth.isLoading, auth.isSignedOut, isMarketingPath, isPublicAppPath, location.pathname, location.searchStr, navigate, onAuthRoute]);

  if (onAuthRoute) {
    return (
      <BackgroundPatternProvider>
        <BgFx />
        <Outlet />
      </BackgroundPatternProvider>
    );
  }

  if (isMarketingPath && (auth.isLoading || auth.isSignedOut)) {
    return (
      <BackgroundPatternProvider>
        <BgFx />
        <PublicShell>
          <Outlet />
        </PublicShell>
      </BackgroundPatternProvider>
    );
  }

  return (
    <BackgroundPatternProvider>
      <BgFx />
      <RouteWarmup />
      <FlexRow>
        <AppSidebar />
        <FlexColItem>
          <AppBar />
          <MainContainer>
            {auth.isLoading && !isPublicAppPath ? (
              <CenteredState>
                <Loader data-testid="loader--spinner" />
              </CenteredState>
            ) : auth.isSignedOut && !isPublicAppPath ? (
              <CenteredState>
                <Loader data-testid="loader--spinner" />
              </CenteredState>
            ) : (
              <Outlet />
            )}
          </MainContainer>
        </FlexColItem>
      </FlexRow>
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
