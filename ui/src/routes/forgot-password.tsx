import { createFileRoute } from "@tanstack/react-router";
import { SignIn } from "@clerk/react";
import { AuthShell } from "../features/auth/AuthShell";
import { AuthSlotState } from "../features/auth/AuthSlotState";
import { redisClerkAppearance } from "../features/auth/clerk-appearance";
import { useAuthSession } from "../foundation/auth-context";
import { useColorMode } from "../foundation/theme-context";

export const Route = createFileRoute("/forgot-password")({
  component: ForgotPasswordRoute,
});

function ForgotPasswordRoute() {
  const auth = useAuthSession();
  const { colorMode } = useColorMode();

  return (
    <AuthShell
      title="Reset your password"
      subtitle="Enter the email on your account and we'll send a secure link to set a new password."
    >
      {auth.isLoading ? (
        <AuthSlotState kind="loading" />
      ) : !auth.supportsAccountAuth ? (
        <AuthSlotState kind="unsupported" />
      ) : (
        <SignIn
          routing="path"
          path="/forgot-password"
          signUpUrl="/signup"
          appearance={redisClerkAppearance(colorMode)}
        />
      )}
    </AuthShell>
  );
}
