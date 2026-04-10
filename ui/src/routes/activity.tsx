import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Loader } from "@redislabsdev/redis-ui-components";
import { PageStack, SectionCard, SectionGrid, SectionTitle } from "../components/afs-kit";
import { useDatabaseScope, useScopedActivity } from "../foundation/database-scope";
import { ActivityTable } from "../foundation/tables/activity-table";
import type { AFSActivityEvent } from "../foundation/types/afs";

export const Route = createFileRoute("/activity")({
  component: ActivityPage,
});

function ActivityPage() {
  const navigate = useNavigate();
  const activityQuery = useScopedActivity(50);
  const { selectedDatabase } = useDatabaseScope();

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
          <div style={{ marginTop: 16 }}>
            <ActivityTable rows={events} onOpenActivity={openActivity} />
          </div>
    </PageStack>
  );
}
