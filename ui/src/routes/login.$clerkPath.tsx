import { createFileRoute } from "@tanstack/react-router";
import { LoginView, validateAuthRedirectSearch } from "../features/auth/auth-route-views";

export const Route = createFileRoute("/login/$clerkPath")({
  validateSearch: validateAuthRedirectSearch,
  component: LoginClerkPathRoute,
});

function LoginClerkPathRoute() {
  const { redirect } = Route.useSearch();
  return <LoginView redirect={redirect} />;
}
