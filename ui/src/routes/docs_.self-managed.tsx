import { createFileRoute } from "@tanstack/react-router";
import { DocsTopicPage, docsTopicById } from "../features/docs/docs-topics";

export const Route = createFileRoute("/docs_/self-managed")({
  component: SelfManagedDocsPage,
});

function SelfManagedDocsPage() {
  return <DocsTopicPage topic={docsTopicById["self-managed"]} />;
}
