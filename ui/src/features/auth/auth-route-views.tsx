import { SignIn, SignUp } from "@clerk/react";
import { useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import { useAuthSession } from "../../foundation/auth-context";
import { useColorMode } from "../../foundation/theme-context";
import { AuthShell } from "./AuthShell";
import { AuthSlotState } from "./AuthSlotState";
import { redisClerkAppearance } from "./clerk-appearance";

export type AuthRedirectSearch = {
  redirect?: string;
};

export function validateAuthRedirectSearch(search: Record<string, unknown>): AuthRedirectSearch {
  return {
    redirect: typeof search.redirect === "string" ? search.redirect : undefined,
  };
}

type AuthRouteViewProps = {
  redirect?: string;
};

export function LoginView({ redirect }: AuthRouteViewProps) {
  const auth = useAuthSession();
  const { colorMode } = useColorMode();
  const navigate = useNavigate();

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

export function SignupView({ redirect }: AuthRouteViewProps) {
  const auth = useAuthSession();
  const { colorMode } = useColorMode();
  const navigate = useNavigate();

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
