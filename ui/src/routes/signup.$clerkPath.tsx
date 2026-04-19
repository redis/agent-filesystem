import { createFileRoute } from "@tanstack/react-router";
import { SignupRouteView, validateSignupSearch } from "./signup";

export const Route = createFileRoute("/signup/$clerkPath")({
  validateSearch: validateSignupSearch,
  component: SignupRouteView,
});
