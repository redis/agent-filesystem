import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redislabsdev/redis-ui-components";
import { PageStack, SectionCard, SectionGrid, SectionHeader, SectionTitle } from "../components/afs-kit";
import { useActivity } from "../foundation/hooks/use-afs";
import { ActivityTable } from "../foundation/tables/activity-table";
import type { AFSActivityEvent } from "../foundation/types/afs";

export const Route = createFileRoute("/activity")({
  component: ActivityPage,
});

function ActivityPage() {
  const navigate = useNavigate();
  const activityQuery = useActivity(100);

  if (activityQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const events = activityQuery.data ?? [];

  function openActivity(event: AFSActivityEvent) {
    if (event.workspaceId == null) {
      return;
    }

    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: event.workspaceId },
      search: activityDestinationSearch(event),
    });
  }

  return (
    <PageStack>
      <ActivityTable rows={events} onOpenActivity={openActivity} />
    </PageStack>
  );
}

function activityDestinationSearch(event: AFSActivityEvent) {
  if (event.scope === "savepoint") {
    return { tab: "checkpoints" as const };
  }
  if (event.scope === "file") {
    return { tab: "files" as const };
  }
  return {};
}
