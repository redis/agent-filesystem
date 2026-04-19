import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import { SignUp } from "@clerk/react";
import { AuthShell } from "../features/auth/AuthShell";
import { AuthSlotState } from "../features/auth/AuthSlotState";
import { redisClerkAppearance } from "../features/auth/clerk-appearance";
import { useAuthSession } from "../foundation/auth-context";
import { useColorMode } from "../foundation/theme-context";

type SignupSearch = {
  redirect?: string;
};

export function validateSignupSearch(search: Record<string, unknown>): SignupSearch {
  return {
    redirect: typeof search.redirect === "string" ? search.redirect : undefined,
  };
}

export const Route = createFileRoute("/signup")({
  validateSearch: validateSignupSearch,
  component: SignupRouteView,
});

export function SignupRouteView() {
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
      title="Create your AFS Cloud account"
      subtitle="Spin up a durable workspace for your agents in under a minute."
    >
      {auth.isLoading ? (
        <AuthSlotState kind="loading" />
      ) : !auth.supportsAccountAuth ? (
        <AuthSlotState kind="unsupported" />
      ) : (
        <SignUp
          routing="path"
          path="/signup"
          signInUrl="/login"
          forceRedirectUrl={afterUrl}
          fallbackRedirectUrl={afterUrl}
          appearance={redisClerkAppearance(colorMode)}
        />
      )}
    </AuthShell>
  );
}
