import styled from "styled-components";
import { SectionCard, SectionGrid, SectionHeader, SectionTitle } from "../../components/afs-kit";
import { useActivityPage } from "../../foundation/hooks/use-afs";
import { ActivityTable } from "../../foundation/tables/activity-table";
import type { AFSActivityEvent } from "../../foundation/types/afs";

type StudioTab = "browse" | "checkpoints" | "activity" | "settings";

type Props = {
  databaseId?: string;
  workspaceId: string;
  updatedAt: string;
  onTabChange: (tab: StudioTab) => void;
};

function activityDestinationTab(event: AFSActivityEvent): StudioTab {
  if (event.scope === "savepoint") {
    return "checkpoints";
  }
  if (event.scope === "file") {
    return "browse";
  }
  if (event.scope === "workspace") {
    return "browse";
  }
  return "activity";
}

export function ActivityTab({ databaseId, workspaceId, updatedAt, onTabChange }: Props) {
  const activityQuery = useActivityPage({
    databaseId,
    workspaceId,
    limit: 50,
  });
  const activity = activityQuery.data?.items ?? [];

  return (
    <SectionGrid>
      <SectionCard $span={12}>
        <SectionHeader>
          <SectionTitle title="Workspace history" />
          <LastUpdated>Last updated {new Date(updatedAt).toLocaleString()}</LastUpdated>
        </SectionHeader>
        <ActivityTable
          rows={activity}
          loading={activityQuery.isLoading}
          error={activityQuery.isError}
          errorMessage={activityQuery.error instanceof Error ? activityQuery.error.message : undefined}
          onOpenActivity={(event) => onTabChange(activityDestinationTab(event))}
        />
      </SectionCard>
    </SectionGrid>
  );
}

const LastUpdated = styled.span`
  color: var(--afs-muted);
  font-size: 13px;
  white-space: nowrap;
`;
