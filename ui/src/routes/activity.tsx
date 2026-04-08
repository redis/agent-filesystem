import { createFileRoute } from "@tanstack/react-router";
import { Loader } from "@redislabsdev/redis-ui-components";
import { EventList, PageStack, SectionCard, SectionGrid, SectionTitle } from "../components/afs-kit";
import { useDatabaseScope, useScopedActivity } from "../foundation/database-scope";

export const Route = createFileRoute("/activity")({
  component: ActivityPage,
});

function ActivityPage() {
  const activityQuery = useScopedActivity(50);
  const { selectedDatabase } = useDatabaseScope();

  if (activityQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const events = activityQuery.data.map((event) => ({
    ...event,
    title: event.workspaceName ? `${event.workspaceName}: ${event.title}` : event.title,
  }));

  return (
    <PageStack>
      <SectionGrid>
        <SectionCard $span={12}>
          <SectionTitle
            eyebrow="Audit"
            title="Workspace activity"
            body={
              selectedDatabase == null
                ? "Open a Redis database to scope the activity feed."
                : `Activity across workspaces in ${selectedDatabase.displayName}.`
            }
          />
          <div style={{ marginTop: 16 }}>
            <EventList events={events} />
          </div>
        </SectionCard>
      </SectionGrid>
    </PageStack>
  );
}
