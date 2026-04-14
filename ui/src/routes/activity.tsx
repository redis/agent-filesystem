import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Loader } from "@redislabsdev/redis-ui-components";
import { PageStack } from "../components/afs-kit";
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

    const baseSearch = event.databaseId ? { databaseId: event.databaseId } : {};

    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: event.workspaceId },
      search:
        event.scope === "savepoint"
          ? { ...baseSearch, tab: "checkpoints" }
          : event.scope === "file"
            ? { ...baseSearch, tab: "files" }
            : event.scope === "workspace"
              ? baseSearch
              : { ...baseSearch, tab: "activity" },
    });
  }

  return (
    <PageStack>
      <ActivityTable rows={events} onOpenActivity={openActivity} />
    </PageStack>
  );
}
