import { SectionCard, SectionGrid, SectionHeader, SectionTitle } from "../../components/afs-kit";
import { ActivityTable } from "../../foundation/tables/activity-table";
import type { AFSActivityEvent } from "../../foundation/types/afs";

type StudioTab = "overview" | "files" | "checkpoints" | "activity";

type Props = {
  activity: AFSActivityEvent[];
  onTabChange: (tab: StudioTab) => void;
};

function activityDestinationTab(event: AFSActivityEvent): StudioTab {
  if (event.scope === "savepoint") {
    return "checkpoints";
  }
  if (event.scope === "file") {
    return "files";
  }
  if (event.scope === "workspace") {
    return "overview";
  }
  return "activity";
}

export function ActivityTab({ activity, onTabChange }: Props) {
  return (
    <SectionGrid>
      <SectionCard $span={12}>
        <SectionHeader>
          <SectionTitle title="Workspace activity" />
        </SectionHeader>
        <ActivityTable
          rows={activity}
          onOpenActivity={(event) => onTabChange(activityDestinationTab(event))}
        />
      </SectionCard>
    </SectionGrid>
  );
}
