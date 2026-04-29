import { createFileRoute } from "@tanstack/react-router";
import { DocsTopicPage, docsTopicById } from "../features/docs/docs-topics";

export const Route = createFileRoute("/docs_/how-it-works")({
  component: HowItWorksDocsPage,
});

function HowItWorksDocsPage() {
  return <DocsTopicPage topic={docsTopicById["how-it-works"]} />;
}
