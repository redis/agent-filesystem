import { createFileRoute } from "@tanstack/react-router";
import { DocsDocumentPage } from "../features/docs/docs-document-page";

export const Route = createFileRoute("/docs")({
  component: DocsDocumentPage,
});
