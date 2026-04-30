import { ClerkProvider, useAuth, useUser } from "@clerk/react";
import { useQuery } from "@tanstack/react-query";
import { createContext, useContext, useMemo } from "react";
import type { PropsWithChildren } from "react";
import { afsApi } from "./api/afs";
import type { AFSAuthConfig } from "./types/afs";

const AFS_CLOUD_HOST = "afs.cloud";

type AuthContextValue = {
  config: AFSAuthConfig;
  isLoading: boolean;
  isAuthenticated: boolean;
  isSignedOut: boolean;
  supportsAccountAuth: boolean;
  displayName: string;
  secondaryLabel?: string;
};

const defaultAuthConfig: AFSAuthConfig = {
  mode: "none",
  enabled: false,
  provider: "none",
  signInRequired: false,
  authenticated: true,
  productMode: "self-hosted",
};

const AuthContext = createContext<AuthContextValue | null>(null);

function resolveDisplayName(config: AFSAuthConfig) {
  if (config.user?.name?.trim()) {
    return config.user.name.trim();
  }
  if (config.user?.email?.trim()) {
    return config.user.email.trim();
  }
  if (config.user?.subject?.trim()) {
    return config.user.subject.trim();
  }
  if (config.mode === "clerk") {
    return "AFS Cloud";
  }
  if (config.mode !== "none") {
    return "Authenticated user";
  }
  return "Browser session";
}

function resolveSecondaryLabel(config: AFSAuthConfig) {
  if (config.user?.email?.trim() && config.user.email.trim() !== resolveDisplayName(config)) {
    return config.user.email.trim();
  }
  if (config.mode === "clerk") {
    return "AFS Cloud account";
  }
  if (config.mode !== "none") {
    return config.provider || config.mode;
  }
  return "AFS Cloud";
}

function AuthContextProvider(props: PropsWithChildren<{ value: AuthContextValue }>) {
  return (
    <AuthContext.Provider value={props.value}>
      {props.children}
    </AuthContext.Provider>
  );
}

function ClerkAuthBridge(props: PropsWithChildren<{ config: AFSAuthConfig }>) {
  const { isLoaded, isSignedIn } = useAuth();
  const { user } = useUser();

  const effectiveConfig = useMemo<AFSAuthConfig>(() => {
    const email = user?.primaryEmailAddress?.emailAddress ?? props.config.user?.email;
    const name = user?.fullName ?? user?.firstName ?? props.config.user?.name;
    return {
      ...props.config,
      authenticated: !!isSignedIn,
      user: isSignedIn ? {
        subject: user?.id ?? props.config.user?.subject ?? "",
        name: name ?? undefined,
        email: email ?? undefined,
        groups: props.config.user?.groups,
        isAdmin: props.config.user?.isAdmin,
      } : undefined,
    };
  }, [isSignedIn, props.config, user]);

  const value = useMemo<AuthContextValue>(() => ({
    config: effectiveConfig,
    isLoading: !isLoaded,
    isAuthenticated: !!isSignedIn,
    isSignedOut: isLoaded && !isSignedIn,
    supportsAccountAuth: true,
    displayName: resolveDisplayName(effectiveConfig),
    secondaryLabel: resolveSecondaryLabel(effectiveConfig),
  }), [effectiveConfig, isLoaded, isSignedIn]);

  return (
    <AuthContextProvider value={value}>
      {props.children}
    </AuthContextProvider>
  );
}

function decodeClerkFrontendHost(publishableKey?: string): string | null {
  const key = publishableKey?.trim();
  if (!key) {
    return null;
  }
  const parts = key.split("_", 3);
  if (parts.length < 3) {
    return null;
  }
  try {
    const normalized = parts[2].replace(/-/g, "+").replace(/_/g, "/");
    const padded = normalized + "=".repeat((4 - (normalized.length % 4)) % 4);
    const decoded = atob(padded);
    return decoded.replace(/\$$/, "").trim() || null;
  } catch {
    return null;
  }
}

export function AuthProvider(props: PropsWithChildren) {
  const authQuery = useQuery({
    queryKey: ["afs", "auth", "config"],
    queryFn: () => afsApi.getAuthConfig(),
    staleTime: 15_000,
    gcTime: 10 * 60 * 1000,
    retry: 1,
  });

  const config = authQuery.data ?? defaultAuthConfig;
  const baseValue = useMemo<AuthContextValue>(() => ({
    config,
    isLoading: authQuery.isLoading,
    isAuthenticated: !authQuery.isLoading && !!config.authenticated,
    isSignedOut: !authQuery.isLoading && config.signInRequired && !config.authenticated,
    supportsAccountAuth: config.mode === "clerk",
    displayName: resolveDisplayName(config),
    secondaryLabel: resolveSecondaryLabel(config),
  }), [authQuery.isLoading, config]);

  if (config.mode === "clerk" && config.clerkPublishableKey?.trim()) {
    return (
      <ClerkProvider publishableKey={config.clerkPublishableKey}>
        <ClerkAuthBridge config={config}>
          {props.children}
        </ClerkAuthBridge>
      </ClerkProvider>
    );
  }

  return (
    <AuthContextProvider value={baseValue}>
      {props.children}
    </AuthContextProvider>
  );
}

export function useAuthSession() {
  const context = useContext(AuthContext);
  if (context == null) {
    throw new Error("useAuthSession must be used inside AuthProvider.");
  }
  return context;
}

export function isCloudAdminConfig(config: AFSAuthConfig) {
  return config.productMode === "cloud" && config.user?.isAdmin === true;
}
