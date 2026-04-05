import { createFileRoute } from "@tanstack/react-router";
import { Loader } from "@redislabsdev/redis-ui-components";
import { EventList, PageStack, SectionCard, SectionGrid, SectionTitle } from "../components/raf-kit";
import { useWorkspaces } from "../foundation/hooks/use-raf";

export const Route = createFileRoute("/activity")({
  component: ActivityPage,
});

function ActivityPage() {
  const workspacesQuery = useWorkspaces();

  if (workspacesQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const events = (workspacesQuery.data ?? [])
    .flatMap((workspace) =>
      workspace.activity.map((event) => ({
        ...event,
        title: `${workspace.name}: ${event.title}`,
      })),
    )
    .sort((left, right) => right.createdAt.localeCompare(left.createdAt));

  return (
    <PageStack>
      <SectionGrid>
        <SectionCard $span={12}>
          <SectionTitle
            eyebrow="Audit"
            title="Workspace activity"
            body="Later this can read straight from AFS audit streams in Redis. For now it exercises the same UI patterns with the demo store."
          />
          <div style={{ marginTop: 16 }}>
            <EventList events={events} />
          </div>
        </SectionCard>
      </SectionGrid>
    </PageStack>
  );
}
