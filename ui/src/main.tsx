import { StrictMode } from "react";
import ReactDOM from "react-dom/client";
import { RouterProvider, createRouter } from "@tanstack/react-router";
import { RouteErrorBoundary } from "./error-boundaries/route-error-boundary";
import { ThemeProvider } from "styled-components";
import { themesRebrand, CommonStyles } from "@redis-ui/styles";
import "modern-normalize/modern-normalize.css";
import "@redis-ui/styles/normalized-styles.css";
import "@redis-ui/styles/fonts.css";
import "./styles/skin-classic.css";
import "./styles/skin-situation-room.css";
import "./styles/skin-overrides.css";
import "./index.css";

// Import the generated route tree
import { routeTree } from "./routeTree.gen";
import { QueryClientProvider } from "@tanstack/react-query";
import { AppErrorBoundary } from "./error-boundaries/app-error-boundary";
import { AuthProvider } from "./foundation/auth-context";
import { DatabaseScopeProvider } from "./foundation/database-scope";
import { ColorModeProvider } from "./foundation/theme-context";
import { SkinProvider } from "./foundation/skin-context";
import { queryClient } from "./foundation/query-client";

// Create a new router instance
const router = createRouter({
  routeTree,
  defaultErrorComponent: RouteErrorBoundary,
  defaultPreload: "intent",
  defaultPreloadDelay: 35,
  defaultStaleTime: 15_000,
  defaultPreloadStaleTime: 30_000,
  defaultOnCatch: (error, errorInfo) => {
    console.error("Unhandled route error", error, errorInfo);
  },
});

// Register the router instance for type safety
declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

// Render the app
const rootElement = document.getElementById("root")!;
if (!rootElement.innerHTML) {
  const root = ReactDOM.createRoot(rootElement);
  root.render(
    <StrictMode>
      <SkinProvider>
        <ColorModeProvider>
          {(colorMode) => (
            <ThemeProvider theme={themesRebrand[colorMode]}>
              <CommonStyles />
              <AppErrorBoundary>
                <QueryClientProvider client={queryClient}>
                  <AuthProvider>
                    <DatabaseScopeProvider>
                      <RouterProvider router={router} />
                    </DatabaseScopeProvider>
                  </AuthProvider>
                </QueryClientProvider>
              </AppErrorBoundary>
            </ThemeProvider>
          )}
        </ColorModeProvider>
      </SkinProvider>
    </StrictMode>
  );
}
