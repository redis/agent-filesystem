import { createFileRoute } from "@tanstack/react-router";
import { LoginRouteView, validateLoginSearch } from "./login";

export const Route = createFileRoute("/login/$clerkPath")({
  validateSearch: validateLoginSearch,
  component: LoginRouteView,
});
