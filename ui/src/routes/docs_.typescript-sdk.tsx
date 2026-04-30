import { createFileRoute } from "@tanstack/react-router";
import { DocsTopicPage, docsTopicById } from "../features/docs/docs-topics";

export const Route = createFileRoute("/docs_/typescript-sdk")({
  component: TypeScriptSdkDocsPage,
});

function TypeScriptSdkDocsPage() {
  return <DocsTopicPage topic={docsTopicById["typescript-sdk"]} />;
}
