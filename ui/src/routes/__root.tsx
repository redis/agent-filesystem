import { createRootRoute, Outlet, useLocation, useRouter } from "@tanstack/react-router";
import { useEffect } from "react";
import { RouteErrorBoundary } from "../error-boundaries/route-error-boundary";
import { BackgroundPatternProvider } from "../foundation/background-pattern";
import { AppSidebar } from "../layout/sidebar";
import { AppBar } from "../layout/app-bar";
import { bottomNavigationItems, navigationItems } from "../layout/navigation-items";
import {
  FlexRow,
  FlexColItem,
  MainContainer,
} from "../layout/layout.styles";

function RouteWarmup() {
  const router = useRouter();
  const location = useLocation();

  useEffect(() => {
    const targets = [...navigationItems, ...bottomNavigationItems]
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
  }, [location.pathname, router]);

  return null;
}

function RootLayout() {
  return (
    <BackgroundPatternProvider>
      <RouteWarmup />
      <FlexRow>
        <AppSidebar />
        <FlexColItem>
          <AppBar />
          <MainContainer>
            <Outlet />
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
