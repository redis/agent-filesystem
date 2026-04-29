import { createFileRoute } from "@tanstack/react-router";
import { DocsTopicPage, docsTopicById } from "../features/docs/docs-topics";

export const Route = createFileRoute("/docs_/workspaces")({
  component: WorkspacesDocsPage,
});

function WorkspacesDocsPage() {
  return <DocsTopicPage topic={docsTopicById.workspaces} />;
}
