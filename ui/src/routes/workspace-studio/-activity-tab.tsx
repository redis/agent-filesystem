import styled from "styled-components";
import { SectionCard, SectionGrid, SectionHeader, SectionTitle } from "../../components/afs-kit";
import { ActivityTable } from "../../foundation/tables/activity-table";
import type { AFSActivityEvent } from "../../foundation/types/afs";

type StudioTab = "browse" | "checkpoints" | "activity" | "settings";

type Props = {
  activity: AFSActivityEvent[];
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

export function ActivityTab({ activity, updatedAt, onTabChange }: Props) {
  return (
    <SectionGrid>
      <SectionCard $span={12}>
        <SectionHeader>
          <SectionTitle title="Workspace activity" />
          <LastUpdated>Last updated {new Date(updatedAt).toLocaleString()}</LastUpdated>
        </SectionHeader>
        <ActivityTable
          rows={activity}
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
