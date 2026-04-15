import { Button } from "@redis-ui/components";
import styled from "styled-components";
import { SectionCard, SectionGrid, SectionHeader, SectionTitle } from "../../components/afs-kit";
import { formatBytes } from "../../foundation/api/afs";
import type { AFSWorkspaceDetail } from "../../foundation/types/afs";
import { useNavigate } from "@tanstack/react-router";

type Props = {
  workspace: AFSWorkspaceDetail;
};

export function OverviewTab({ workspace }: Props) {
  const navigate = useNavigate();
  const latestActivity = workspace.activity[0] ?? null;
  const connectedAgents = workspace.agents.length;

  return (
    <SectionGrid>
      <SectionCard $span={12}>
        <SectionHeader>
          <SectionTitle title="Status" />
        </SectionHeader>
        <StatusTable>
          <tbody>
            <StatusRow>
              <StatusLabel>Files</StatusLabel>
              <StatusValue>{workspace.fileCount.toLocaleString()}</StatusValue>
            </StatusRow>
            <StatusRow>
              <StatusLabel>Folders</StatusLabel>
              <StatusValue>{workspace.folderCount.toLocaleString()}</StatusValue>
            </StatusRow>
            <StatusRow>
              <StatusLabel>Checkpoints</StatusLabel>
              <StatusValue>{workspace.checkpointCount.toLocaleString()}</StatusValue>
            </StatusRow>
            <StatusRow>
              <StatusLabel>Connected agents</StatusLabel>
              <StatusValue>
                <AgentsCell>
                  <span>{connectedAgents.toLocaleString()}</span>
                  <Button
                    size="small"
                    variant="secondary-fill"
                    onClick={() => {
                      void navigate({
                        to: "/agents",
                        search: {
                          workspaceId: workspace.id,
                        },
                      });
                    }}
                  >
                    Show Agents
                  </Button>
                </AgentsCell>
              </StatusValue>
            </StatusRow>
            <StatusRow>
              <StatusLabel>Size</StatusLabel>
              <StatusValue>{formatBytes(workspace.totalBytes)}</StatusValue>
            </StatusRow>
            <StatusRow>
              <StatusLabel>Last updated</StatusLabel>
              <StatusValue>{new Date(workspace.updatedAt).toLocaleString()}</StatusValue>
            </StatusRow>
            <StatusRow>
              <StatusLabel>Latest activity</StatusLabel>
              <StatusValue>
                {latestActivity == null
                  ? "No activity yet"
                  : `${latestActivity.title} · ${new Date(latestActivity.createdAt).toLocaleString()}`}
              </StatusValue>
            </StatusRow>
            <StatusRow>
              <StatusLabel>Database</StatusLabel>
              <StatusValue>{workspace.databaseName}</StatusValue>
            </StatusRow>
            <StatusRow>
              <StatusLabel>Redis key</StatusLabel>
              <StatusValue>{workspace.redisKey}</StatusValue>
            </StatusRow>
            {workspace.mountedPath ? (
              <StatusRow>
                <StatusLabel>Mounted path</StatusLabel>
                <StatusValue>{workspace.mountedPath}</StatusValue>
              </StatusRow>
            ) : null}
          </tbody>
        </StatusTable>
      </SectionCard>
    </SectionGrid>
  );
}

const StatusTable = styled.table`
  width: 100%;
  border-collapse: collapse;
`;

const StatusRow = styled.tr`
  border-top: 1px solid var(--afs-line);

  &:first-child {
    border-top: none;
  }
`;

const StatusLabel = styled.th`
  width: 220px;
  padding: 14px 0;
  color: var(--afs-muted);
  font-size: 13px;
  font-weight: 600;
  text-align: left;
  vertical-align: top;
`;

const StatusValue = styled.td`
  padding: 14px 0;
  color: var(--afs-ink);
  font-size: 14px;
  line-height: 1.5;
  text-align: left;
`;

const AgentsCell = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
`;

