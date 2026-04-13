import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Loader } from "@redislabsdev/redis-ui-components";
import {
  PageStack,
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
} from "../components/afs-kit";
import { useScopedActivity } from "../foundation/database-scope";
import { ActivityTable } from "../foundation/tables/activity-table";
import type { AFSActivityEvent } from "../foundation/types/afs";

export const Route = createFileRoute("/activity")({
  component: ActivityPage,
});

function ActivityPage() {
  const navigate = useNavigate();
  const activityQuery = useScopedActivity(50);

  if (activityQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const events = activityQuery.data.map((event) => ({
    ...event,
    title: event.workspaceName ? `${event.workspaceName}: ${event.title}` : event.title,
  }));

  function openActivity(event: AFSActivityEvent) {
    if (event.workspaceId == null) {
      return;
    }

    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: event.workspaceId },
      search:
        event.scope === "savepoint"
          ? { tab: "checkpoints" }
          : event.scope === "file"
            ? { tab: "files" }
            : event.scope === "workspace"
              ? {}
              : { tab: "activity" },
    });
  }

  return (
    <PageStack>
      <SectionGrid>
        <SectionCard $span={12}>
          <SectionHeader>
            <SectionTitle title="Recent activity" />
          </SectionHeader>
          <ActivityTable rows={events} onOpenActivity={openActivity} />
        </SectionCard>
      </SectionGrid>
    </PageStack>
  );
}
