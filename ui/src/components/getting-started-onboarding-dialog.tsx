import { Button } from "@redis-ui/components";
import { useEffect, useState } from "react";
import styled from "styled-components";
import { DialogCloseButton, DialogOverlay } from "./afs-kit";
import { ConnectAgentBanner } from "./connect-agent-banner";

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
  databaseName,
  fileCount,
  folderCount,
  initialStage = "success",
  onClose,
}: Props) {
  const [stage, setStage] = useState<OnboardingStage>(initialStage);

  useEffect(() => {
    if (open) {
      setStage(initialStage);
    }
  }, [initialStage, open, workspaceId]);

  if (!open) {
    return null;
  }

  const workspaceLabel = displayWorkspaceLabel(workspaceName);

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
            onDismiss={onClose}
          />
        </WorkflowShell>
      ) : (
        <SuccessShell onClick={(event) => event.stopPropagation()}>
          <SuccessCard>
            <SuccessCloseButton type="button" aria-label="Close" onClick={onClose}>
              ×
            </SuccessCloseButton>
            <SuccessHeader>
              <SuccessIconWrap aria-hidden>
                <SuccessSpark>✦</SuccessSpark>
              </SuccessIconWrap>
              <SuccessHeaderCopy>
                <SuccessEyebrow>Step 1 of 2 • AFS Cloud</SuccessEyebrow>
                <SuccessTitle>{workspaceLabel} is ready</SuccessTitle>
              </SuccessHeaderCopy>
            </SuccessHeader>
            <SuccessBody>
              Congrats. We created your first workspace, <strong>{workspaceLabel}</strong>, and loaded it with sample files so you can explore AFS right away.
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
              <SuccessStat>
                <SuccessStatValue>{databaseName?.trim() || "AFS Cloud"}</SuccessStatValue>
                <SuccessStatLabel>current database</SuccessStatLabel>
              </SuccessStat>
            </SuccessStats>
            <SuccessBody>
              Next, connect your first agent. Once it is linked, it can sync this workspace locally or access it through MCP.
            </SuccessBody>
            <SuccessActions>
              <Button size="large" onClick={() => setStage("connect")}>
                Connect my first agent
              </Button>
              <Button size="large" kind="ghost" onClick={onClose}>
                I&apos;ll do this later
              </Button>
            </SuccessActions>
          </SuccessCard>
        </SuccessShell>
      )}
    </DialogOverlay>
  );
}

function displayWorkspaceLabel(workspaceName: string) {
  const trimmed = workspaceName.trim().toLowerCase();
  if (trimmed === "getting-started" || trimmed.startsWith("getting-started-")) {
    return "Getting-started";
  }
  return workspaceName;
}

const WorkflowShell = styled.div`
  width: min(960px, 100%);
  max-height: calc(100vh - 48px);
  overflow: auto;
`;

const SuccessShell = styled.div`
  width: min(760px, 100%);
  max-height: calc(100vh - 48px);
  overflow: auto;
`;

const SuccessCard = styled.div`
  position: relative;
  border-radius: 28px;
  padding: 30px;
  background:
    radial-gradient(circle at top right, color-mix(in srgb, var(--afs-accent) 16%, transparent), transparent 28%),
    radial-gradient(circle at bottom left, rgba(255, 209, 102, 0.18), transparent 34%),
    linear-gradient(180deg, var(--afs-panel-strong), color-mix(in srgb, var(--afs-bg-soft) 58%, white));
  border: 1px solid color-mix(in srgb, var(--afs-accent) 16%, var(--afs-line));
  box-shadow: 0 24px 60px rgba(79, 51, 24, 0.10);

  @media (max-width: 720px) {
    padding: 22px;
  }
`;

const SuccessCloseButton = styled(DialogCloseButton)`
  position: absolute;
  top: 20px;
  right: 20px;
`;

const SuccessHeader = styled.div`
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 12px;
  padding-right: 48px;

  @media (max-width: 640px) {
    align-items: flex-start;
  }
`;

const SuccessHeaderCopy = styled.div`
  min-width: 0;
`;

const SuccessIconWrap = styled.div`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 52px;
  height: 52px;
  border-radius: 16px;
  flex: 0 0 auto;
  background: linear-gradient(
    135deg,
    color-mix(in srgb, var(--afs-accent) 18%, white),
    color-mix(in srgb, #ffd98f 72%, white)
  );
  color: var(--afs-accent);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--afs-accent) 14%, transparent);
`;

const SuccessSpark = styled.span`
  font-size: 22px;
  line-height: 1;
`;

const SuccessEyebrow = styled.div`
  color: var(--afs-accent);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
`;

const SuccessTitle = styled.h2`
  margin: 10px 0 0;
  color: var(--afs-ink);
  font-size: clamp(28px, 4vw, 40px);
  line-height: 1.05;
`;

const SuccessBody = styled.p`
  margin: 0;
  max-width: 66ch;
  color: var(--afs-muted);
  font-size: 16px;
  line-height: 1.6;

  & + & {
    margin-top: 10px;
  }
`;

const SuccessStats = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin: 18px 0;

  @media (max-width: 820px) {
    grid-template-columns: 1fr;
  }
`;

const SuccessStat = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 18px;
  padding: 14px 16px;
  background: color-mix(in srgb, var(--afs-panel) 72%, white);
`;

const SuccessStatValue = styled.div`
  color: var(--afs-ink);
  font-size: 18px;
  font-weight: 700;
  line-height: 1.2;
  letter-spacing: -0.02em;
  word-break: break-word;
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
  gap: 12px;
  margin-top: 24px;
  flex-wrap: wrap;
`;
