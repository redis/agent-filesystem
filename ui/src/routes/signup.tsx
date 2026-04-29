import { createFileRoute } from "@tanstack/react-router";
import { SignupView, validateAuthRedirectSearch } from "../features/auth/auth-route-views";

export const Route = createFileRoute("/signup")({
  validateSearch: validateAuthRedirectSearch,
  component: SignupRoute,
});

function SignupRoute() {
  const { redirect } = Route.useSearch();
  return <SignupView redirect={redirect} />;
}
