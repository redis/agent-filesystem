import { createFileRoute } from "@tanstack/react-router";
import { LoginView, validateAuthRedirectSearch } from "../features/auth/auth-route-views";

export const Route = createFileRoute("/login")({
  validateSearch: validateAuthRedirectSearch,
  component: LoginRoute,
});

function LoginRoute() {
  const { redirect } = Route.useSearch();
  return <LoginView redirect={redirect} />;
}
