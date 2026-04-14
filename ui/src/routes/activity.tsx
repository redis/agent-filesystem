import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Loader } from "@redislabsdev/redis-ui-components";
import {
  NoticeBody,
  NoticeCard,
  NoticeTitle,
  PageStack,
} from "../components/afs-kit";
import { useDatabaseScope, useScopedActivity } from "../foundation/database-scope";
import { ActivityTable } from "../foundation/tables/activity-table";
import type { AFSActivityEvent } from "../foundation/types/afs";

export const Route = createFileRoute("/activity")({
  component: ActivityPage,
});

function ActivityPage() {
  const navigate = useNavigate();
  const { unavailableDatabases } = useDatabaseScope();
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
      {unavailableDatabases.length > 0 ? (
        <NoticeCard $tone="warning" role="status">
          <NoticeTitle>Some databases are unavailable</NoticeTitle>
          <NoticeBody>
            Activity below is partial while these databases are disconnected:{" "}
            {unavailableDatabases.map((database) => database.displayName || database.databaseName).join(", ")}.
          </NoticeBody>
        </NoticeCard>
      ) : null}
      <ActivityTable
        rows={events}
        loading={activityQuery.isLoading}
        error={activityQuery.isError}
        errorMessage={activityQuery.error instanceof Error ? activityQuery.error.message : undefined}
        onOpenActivity={openActivity}
      />
    </PageStack>
  );
}
