import styled from "styled-components";
import { SectionCard, SectionGrid, SectionHeader, SectionTitle } from "../../components/afs-kit";
import { useEvents } from "../../foundation/hooks/use-afs";
import { EventsTable } from "../../foundation/tables/events-table";
import type { AFSEventEntry } from "../../foundation/types/afs";
import type { StudioTab } from "../../foundation/workspace-tabs";

const WORKSPACE_HISTORY_LIMIT = 100;

type Props = {
  databaseId?: string;
  workspaceId: string;
  updatedAt: string;
  onTabChange: (tab: StudioTab) => void;
};

function eventDestinationTab(event: AFSEventEntry): StudioTab {
  if (event.kind === "checkpoint" || event.checkpointId) {
    return "checkpoints";
  }
  if (event.kind === "file" || event.path) {
    return "browse";
  }
  if (event.kind === "workspace") {
    return "browse";
  }
  return "activity";
}

export function ActivityTab({ databaseId, workspaceId, updatedAt, onTabChange }: Props) {
  const eventsQuery = useEvents({
    databaseId,
    workspaceId,
    limit: WORKSPACE_HISTORY_LIMIT,
    direction: "desc",
  });
  const events = eventsQuery.data?.items ?? [];

  return (
    <SectionGrid>
      <SectionCard $span={12}>
        <SectionHeader>
          <SectionTitle title="Workspace history" />
          <LastUpdated>Last updated {new Date(updatedAt).toLocaleString()}</LastUpdated>
        </SectionHeader>
        <EventsTable
          rows={events}
          loading={eventsQuery.isLoading}
          error={eventsQuery.isError}
          errorMessage={eventsQuery.error instanceof Error ? eventsQuery.error.message : undefined}
          emptyStateText="No workspace events have been recorded yet."
          hideWorkspaceColumn
          onOpenEvent={(event) => onTabChange(eventDestinationTab(event))}
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
