import styled from "styled-components";
import { StatCard, StatLabel, StatValue } from "../../components/afs-kit";
import { formatBytes } from "../../foundation/api/afs";
import type { AFSWorkspaceDetail, AFSWorkspaceView } from "../../foundation/types/afs";
import { FilesTab } from "./-files-tab";

type Props = {
  workspace: AFSWorkspaceDetail;
  browserView: AFSWorkspaceView;
  onBrowserViewChange: (view: AFSWorkspaceView) => void;
};

export function BrowseTab({ workspace, browserView, onBrowserViewChange }: Props) {
  return (
    <div>
      <CompactStatGrid>
        <CompactStatCard>
          <CompactStatLabel>Files</CompactStatLabel>
          <CompactStatValue>{workspace.fileCount.toLocaleString()}</CompactStatValue>
        </CompactStatCard>
        <CompactStatCard>
          <CompactStatLabel>Folders</CompactStatLabel>
          <CompactStatValue>{workspace.folderCount.toLocaleString()}</CompactStatValue>
        </CompactStatCard>
        <CompactStatCard>
          <CompactStatLabel>Size</CompactStatLabel>
          <CompactStatValue>{formatBytes(workspace.totalBytes)}</CompactStatValue>
        </CompactStatCard>
        <CompactStatCard>
          <CompactStatLabel>Checkpoints</CompactStatLabel>
          <CompactStatValue>{workspace.checkpointCount.toLocaleString()}</CompactStatValue>
        </CompactStatCard>
        <CompactStatCard>
          <CompactStatLabel>Agents</CompactStatLabel>
          <CompactStatValue>{workspace.agents.length.toLocaleString()}</CompactStatValue>
        </CompactStatCard>
      </CompactStatGrid>

      <FilesTab
        workspace={workspace}
        browserView={browserView}
        onBrowserViewChange={onBrowserViewChange}
      />
    </div>
  );
}

/* ─── Styled components ─────────────────────────────────────────────── */

const CompactStatGrid = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  margin-bottom: 8px;

  @media (max-width: 900px) {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  @media (max-width: 560px) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
`;

const CompactStatCard = styled(StatCard)`
  && {
    min-height: 0;
    padding: 12px 14px 10px;
    gap: 4px;
  }
`;

const CompactStatLabel = styled(StatLabel)`
  font-size: 11px;
`;

const CompactStatValue = styled(StatValue)`
  font-size: clamp(1.1rem, 1.8vw, 1.4rem);
`;
