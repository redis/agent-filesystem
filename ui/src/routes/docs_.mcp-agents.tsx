import { createFileRoute } from "@tanstack/react-router";
import { DocsTopicPage, docsTopicById } from "../features/docs/docs-topics";

export const Route = createFileRoute("/docs_/mcp-agents")({
  component: McpAgentsDocsPage,
});

function McpAgentsDocsPage() {
  return <DocsTopicPage topic={docsTopicById["mcp-agents"]} />;
}
