import { createRootRoute, Outlet } from "@tanstack/react-router";
import { RouteErrorBoundary } from "../error-boundaries/route-error-boundary";
import { AppSidebar } from "../layout/sidebar";
import { AppBar } from "../layout/app-bar";
import {
  FlexRow,
  FlexColItem,
  MainContainer,
} from "../layout/layout.styles";

function RootLayout() {
  return (
    <FlexRow>
      <AppSidebar />
      <FlexColItem>
        <AppBar />
        <MainContainer>
          <Outlet />
        </MainContainer>
      </FlexColItem>
    </FlexRow>
  );
}

function RootErrorBoundary(props: Parameters<typeof RouteErrorBoundary>[0]) {
  return <RouteErrorBoundary {...props} fullPage />;
}

export const Route = createRootRoute({
  component: RootLayout,
  errorComponent: RootErrorBoundary,
});
