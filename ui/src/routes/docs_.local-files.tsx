import { createFileRoute } from "@tanstack/react-router";
import { DocsTopicPage, docsTopicById } from "../features/docs/docs-topics";

export const Route = createFileRoute("/docs_/local-files")({
  component: LocalFilesDocsPage,
});

function LocalFilesDocsPage() {
  return <DocsTopicPage topic={docsTopicById["local-files"]} />;
}
