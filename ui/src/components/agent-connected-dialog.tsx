import { Button } from "@redis-ui/components";
import styled from "styled-components";
import type { AFSAgentSession } from "../foundation/types/afs";
import {
  DialogOverlay,
  DialogCard,
  DialogHeader,
  DialogTitle,
  DialogCloseButton,
  DialogFooter,
} from "./afs-kit";

type Props = {
  agent: AFSAgentSession;
  onClose: () => void;
};

export function AgentConnectedDialog({ agent, onClose }: Props) {
  return (
    <DialogOverlay onClick={(e) => e.target === e.currentTarget && onClose()}>
      <CompactDialogCard>
        <DialogHeader>
          <TitleRow>
            <SuccessIcon>
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
                <polyline points="22 4 12 14.01 9 11.01" />
              </svg>
            </SuccessIcon>
            <DialogTitle>Agent Connected</DialogTitle>
          </TitleRow>
          <DialogCloseButton type="button" onClick={onClose} aria-label="Close">
            &times;
          </DialogCloseButton>
        </DialogHeader>

        <DetailGrid>
          <DetailLabel>Workspace</DetailLabel>
          <DetailValue>{agent.workspaceName}</DetailValue>

          <DetailLabel>Client</DetailLabel>
          <DetailValue>{agent.clientKind || "sync"}</DetailValue>

          {agent.hostname ? (
            <>
              <DetailLabel>Host</DetailLabel>
              <DetailValue>{agent.hostname}</DetailValue>
            </>
          ) : null}

          {agent.localPath ? (
            <>
              <DetailLabel>Local path</DetailLabel>
              <DetailValue><code>{agent.localPath}</code></DetailValue>
            </>
          ) : null}

          {agent.operatingSystem ? (
            <>
              <DetailLabel>OS</DetailLabel>
              <DetailValue>{agent.operatingSystem}</DetailValue>
            </>
          ) : null}

          {agent.afsVersion ? (
            <>
              <DetailLabel>AFS version</DetailLabel>
              <DetailValue>{agent.afsVersion}</DetailValue>
            </>
          ) : null}
        </DetailGrid>

        <DialogFooter>
          <GotItButton size="large" onClick={onClose}>
            Got it
          </GotItButton>
        </DialogFooter>
      </CompactDialogCard>
    </DialogOverlay>
  );
}

const CompactDialogCard = styled(DialogCard)`
  width: min(480px, 100%);
  max-height: min(70vh, 500px);
`;

const TitleRow = styled.div`
  display: flex;
  align-items: center;
  gap: 12px;
`;

const SuccessIcon = styled.div`
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: #ecfdf5;
  color: #059669;
`;

const DetailGrid = styled.div`
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 8px 16px;
  margin-top: 4px;

  code {
    font-family: var(--afs-mono);
    font-size: 13px;
  }
`;

const DetailLabel = styled.span`
  color: var(--afs-muted);
  font-size: 13px;
  font-weight: 600;
`;

const DetailValue = styled.span`
  color: var(--afs-ink);
  font-size: 13px;
`;

const GotItButton = styled(Button)`
  && {
    box-shadow: none;
  }
`;
