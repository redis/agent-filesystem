import { createFileRoute } from "@tanstack/react-router";
import { Loader } from "@redislabsdev/redis-ui-components";
import { EventList, PageStack, SectionCard, SectionGrid, SectionTitle } from "../components/afs-kit";
import { useActivity } from "../foundation/hooks/use-afs";

export const Route = createFileRoute("/activity")({
  component: ActivityPage,
});

function ActivityPage() {
  const activityQuery = useActivity(50);

  if (activityQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const events = (activityQuery.data ?? []).map((event) => ({
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
            body="This page now reads from the control-plane audit feed instead of fanning out through every workspace payload."
          />
          <div style={{ marginTop: 16 }}>
            <EventList events={events} />
          </div>
        </SectionCard>
      </SectionGrid>
    </PageStack>
  );
}
