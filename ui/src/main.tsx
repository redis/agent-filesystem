import { StrictMode } from "react";
import ReactDOM from "react-dom/client";
import { RouterProvider, createRouter } from "@tanstack/react-router";
import { RouteErrorBoundary } from "./error-boundaries/route-error-boundary";
import { ThemeProvider } from "styled-components";
import { themesRebrand, CommonStyles } from "@redislabsdev/redis-ui-styles";
import "modern-normalize/modern-normalize.css";
import "@redislabsdev/redis-ui-styles/normalized-styles.css";
import "@redislabsdev/redis-ui-styles/fonts.css";
import "./index.css";

// Import the generated route tree
import { routeTree } from "./routeTree.gen";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AppErrorBoundary } from "./error-boundaries/app-error-boundary";
import { DatabaseScopeProvider } from "./foundation/database-scope";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      refetchIntervalInBackground: true,
    },
  },
});

// Create a new router instance
const router = createRouter({
  routeTree,
  defaultErrorComponent: RouteErrorBoundary,
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
      <ThemeProvider theme={themesRebrand.light}>
        <CommonStyles />
        <AppErrorBoundary>
          <QueryClientProvider client={queryClient}>
            <DatabaseScopeProvider>
              <RouterProvider router={router} />
            </DatabaseScopeProvider>
          </QueryClientProvider>
        </AppErrorBoundary>
      </ThemeProvider>
    </StrictMode>
  );
}
