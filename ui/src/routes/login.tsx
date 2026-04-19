import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import { SignIn } from "@clerk/react";
import { AuthShell } from "../features/auth/AuthShell";
import { AuthSlotState } from "../features/auth/AuthSlotState";
import { redisClerkAppearance } from "../features/auth/clerk-appearance";
import { useAuthSession } from "../foundation/auth-context";
import { useColorMode } from "../foundation/theme-context";

type LoginSearch = {
  redirect?: string;
};

export function validateLoginSearch(search: Record<string, unknown>): LoginSearch {
  return {
    redirect: typeof search.redirect === "string" ? search.redirect : undefined,
  };
}

export const Route = createFileRoute("/login")({
  validateSearch: validateLoginSearch,
  component: LoginRouteView,
});

export function LoginRouteView() {
  const auth = useAuthSession();
  const { colorMode } = useColorMode();
  const navigate = useNavigate();
  const { redirect } = Route.useSearch();

  useEffect(() => {
    if (auth.isAuthenticated && auth.supportsAccountAuth) {
      void navigate({ to: redirect ?? "/", replace: true });
    }
  }, [auth.isAuthenticated, auth.supportsAccountAuth, navigate, redirect]);

  const afterUrl = redirect && redirect.startsWith("/") ? redirect : "/";

  return (
    <AuthShell
      title="Log in to AFS Cloud"
      subtitle="Welcome back. Sign in to manage your workspaces."
    >
      {auth.isLoading ? (
        <AuthSlotState kind="loading" />
      ) : !auth.supportsAccountAuth ? (
        <AuthSlotState kind="unsupported" />
      ) : (
        <SignIn
          routing="path"
          path="/login"
          signUpUrl="/signup"
          forceRedirectUrl={afterUrl}
          fallbackRedirectUrl={afterUrl}
          appearance={redisClerkAppearance(colorMode)}
        />
      )}
    </AuthShell>
  );
}
