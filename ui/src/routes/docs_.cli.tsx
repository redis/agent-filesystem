import { createFileRoute } from "@tanstack/react-router";
import { DocsTopicPage, docsTopicById } from "../features/docs/docs-topics";

export const Route = createFileRoute("/docs_/cli")({
  component: CliDocsPage,
});

function CliDocsPage() {
  return <DocsTopicPage topic={docsTopicById.cli} />;
}
