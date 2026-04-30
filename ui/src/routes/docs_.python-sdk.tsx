import { createFileRoute } from "@tanstack/react-router";
import { DocsTopicPage, docsTopicById } from "../features/docs/docs-topics";

export const Route = createFileRoute("/docs_/python-sdk")({
  component: PythonSdkDocsPage,
});

function PythonSdkDocsPage() {
  return <DocsTopicPage topic={docsTopicById["python-sdk"]} />;
}
