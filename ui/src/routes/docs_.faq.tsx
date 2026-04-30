import { createFileRoute } from "@tanstack/react-router";
import { DocsTopicPage, docsTopicById } from "../features/docs/docs-topics";

export const Route = createFileRoute("/docs_/faq")({
  component: FaqDocsPage,
});

function FaqDocsPage() {
  return <DocsTopicPage topic={docsTopicById.faq} />;
}
