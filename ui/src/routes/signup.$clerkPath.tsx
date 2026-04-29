import { createFileRoute } from "@tanstack/react-router";
import { SignupView, validateAuthRedirectSearch } from "../features/auth/auth-route-views";

export const Route = createFileRoute("/signup/$clerkPath")({
  validateSearch: validateAuthRedirectSearch,
  component: SignupClerkPathRoute,
});

function SignupClerkPathRoute() {
  const { redirect } = Route.useSearch();
  return <SignupView redirect={redirect} />;
}
