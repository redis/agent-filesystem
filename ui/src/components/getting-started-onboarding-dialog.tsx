import { Button } from "@redis-ui/components";
import { useEffect, useState } from "react";
import styled from "styled-components";
import { DialogCloseButton, DialogOverlay } from "./afs-kit";
import { ConnectAgentBanner } from "./connect-agent-banner";
import { useScopedAgents } from "../foundation/database-scope";
import { displayWorkspaceName } from "../foundation/workspace-display";

type OnboardingStage = "success" | "connect";

type Props = {
  open: boolean;
  workspaceId: string;
  workspaceName: string;
  databaseName?: string;
  fileCount?: number;
  folderCount?: number;
  initialStage?: OnboardingStage;
  onClose: () => void;
};

export function GettingStartedOnboardingDialog({
  open,
  workspaceId,
  workspaceName,
  fileCount,
  folderCount,
  initialStage = "success",
  onClose,
}: Props) {
  const [stage, setStage] = useState<OnboardingStage>(initialStage);
  const agentsQuery = useScopedAgents();

  useEffect(() => {
    if (open) {
      setStage(initialStage);
    }
  }, [initialStage, open, workspaceId]);

  // Poll for agent connections while the connect stage is active.
  useEffect(() => {
    if (!open || stage !== "connect") return;
    const interval = setInterval(() => {
      void agentsQuery.refetch();
    }, 5000);
    return () => clearInterval(interval);
  }, [open, stage, agentsQuery]);

  if (!open) {
    return null;
  }

  const workspaceLabel = displayWorkspaceName(workspaceName);
  const agentConnected = (agentsQuery.data ?? []).some(
    (agent) => agent.workspaceId === workspaceId,
  );

  return (
    <DialogOverlay
      role="dialog"
      aria-modal="true"
      onClick={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      {stage === "connect" ? (
        <WorkflowShell onClick={(event) => event.stopPropagation()}>
          <ConnectAgentBanner
            workspaceId={workspaceId}
            workspaceName={workspaceName}
            workspaceLabel={workspaceLabel}
            agentConnected={agentConnected}
            onDismiss={onClose}
          />
        </WorkflowShell>
      ) : (
        <SuccessShell onClick={(event) => event.stopPropagation()}>
          <SuccessCard>
            <SuccessCloseButton type="button" aria-label="Close" onClick={onClose}>
              ×
            </SuccessCloseButton>
            <SuccessEyebrow>Step 1 of 2</SuccessEyebrow>
            <SuccessTitle>Workspace Created!</SuccessTitle>
            <WorkspaceChip>
              <ChipDot />
              <ChipName>{workspaceLabel}</ChipName>
            </WorkspaceChip>
            <SuccessBody>
              We created your first workspace and loaded it with sample files
              so you can explore AFS right away.
            </SuccessBody>
            <SuccessStats>
              <SuccessStat>
                <SuccessStatValue>{fileCount ?? 0}</SuccessStatValue>
                <SuccessStatLabel>sample files</SuccessStatLabel>
              </SuccessStat>
              <SuccessStat>
                <SuccessStatValue>{folderCount ?? 0}</SuccessStatValue>
                <SuccessStatLabel>folders ready</SuccessStatLabel>
              </SuccessStat>
            </SuccessStats>
            <SuccessBody>
              Next, connect your first agent. Once linked, it can sync this
              workspace locally or access it through MCP.
            </SuccessBody>
            <SuccessActions>
              <Button
                size="large"
                variant="secondary-fill"
                onClick={onClose}
              >
                I&apos;ll do this later
              </Button>
              <Button size="large" onClick={() => setStage("connect")}>
                Connect my first agent &rarr;
              </Button>
            </SuccessActions>
          </SuccessCard>
        </SuccessShell>
      )}
    </DialogOverlay>
  );
}

const WorkflowShell = styled.div`
  width: min(960px, 100%);
  max-height: calc(100vh - 48px);
  overflow: auto;
`;

const SuccessShell = styled.div`
  width: min(620px, 100%);
  max-height: calc(100vh - 48px);
  overflow: auto;
`;

const SuccessCard = styled.div`
  position: relative;
  border-radius: 24px;
  padding: 40px 36px 32px;
  background:
    radial-gradient(circle at top right, color-mix(in srgb, var(--afs-accent) 14%, transparent), transparent 32%),
    linear-gradient(180deg, var(--afs-panel-strong, var(--afs-panel)), var(--afs-panel));
  border: 1px solid color-mix(in srgb, var(--afs-accent) 16%, var(--afs-line));
  box-shadow: 0 24px 60px rgba(8, 6, 13, 0.12);

  @media (max-width: 720px) {
    padding: 28px 22px 24px;
  }
`;

const SuccessCloseButton = styled(DialogCloseButton)`
  position: absolute;
  top: 20px;
  right: 20px;
`;

const SuccessEyebrow = styled.div`
  color: var(--afs-accent);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
`;

const SuccessTitle = styled.h2`
  margin: 10px 0 16px;
  color: var(--afs-ink);
  font-size: clamp(28px, 4vw, 38px);
  line-height: 1.08;
  letter-spacing: -0.02em;
`;

const WorkspaceChip = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 14px 6px 10px;
  border-radius: 999px;
  background: #ecfdf5;
  color: #047857;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 18px;
`;

const ChipDot = styled.span`
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #10b981;
  box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.18);
`;

const ChipName = styled.span`
  color: #065f46;
`;

const SuccessBody = styled.p`
  margin: 0;
  max-width: 58ch;
  color: var(--afs-muted);
  font-size: 15px;
  line-height: 1.6;

  & + & {
    margin-top: 10px;
  }
`;

const SuccessStats = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  margin: 20px 0;

  @media (max-width: 520px) {
    grid-template-columns: 1fr;
  }
`;

const SuccessStat = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 14px;
  padding: 14px 16px;
  background: color-mix(in srgb, var(--afs-panel) 72%, white);
`;

const SuccessStatValue = styled.div`
  color: var(--afs-ink);
  font-size: 20px;
  font-weight: 700;
  line-height: 1.2;
  letter-spacing: -0.02em;
`;

const SuccessStatLabel = styled.div`
  margin-top: 4px;
  color: var(--afs-muted);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
`;

const SuccessActions = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 28px;
  flex-wrap: wrap;
`;
