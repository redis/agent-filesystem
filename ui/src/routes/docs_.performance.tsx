import { createFileRoute } from "@tanstack/react-router";
import { DocsTopicPage, docsTopicById } from "../features/docs/docs-topics";

export const Route = createFileRoute("/docs_/performance")({
  component: PerformanceDocsPage,
});

function PerformanceDocsPage() {
  return <DocsTopicPage topic={docsTopicById.performance} />;
}
