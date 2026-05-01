import { createFileRoute } from "@tanstack/react-router";
import { PublicLandingPage } from "../features/landing/PublicLandingPage";

export const Route = createFileRoute("/home")({
  component: PublicLandingPage,
});
