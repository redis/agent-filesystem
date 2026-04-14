import { Card, Typography } from "@redislabsdev/redis-ui-components";
import styled, { css } from "styled-components";
import type {
  AFSActivityEvent,
  AFSWorkspaceSource,
} from "../foundation/types/afs";

const panelSurface = css`
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  background: var(--afs-panel-strong);
  box-shadow: none;
`;

const insetSurface = css`
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  background: var(--afs-panel-strong);
  box-shadow: none;
`;

export const PageStack = styled.div`
  display: flex;
  flex-direction: column;
  gap: 24px;
  width: min(100%, 1480px);
  margin: 0 auto;
  padding: 28px 32px 44px;

  @media (max-width: 900px) {
    padding: 20px 18px 36px;
  }
`;

export const MetaItem = styled.div`
  ${insetSurface}
  display: grid;
  gap: 8px;
  padding: 16px 18px;
`;

export const StatGrid = styled.div`
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(4, minmax(0, 1fr));

  @media (max-width: 1080px) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  @media (max-width: 700px) {
    grid-template-columns: 1fr;
  }
`;

export const StatCard = styled(Card)`
  ${panelSurface}
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 10px;
  min-height: 100px;
  padding: 16px 18px 14px;
`;

export const StatLabel = styled.span`
  color: var(--afs-muted);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
`;

export const StatValue = styled.span`
  display: block;
  color: var(--afs-ink);
  font-size: clamp(1.5rem, 2.5vw, 2rem);
  font-weight: 700;
  line-height: 0.95;
  letter-spacing: -0.04em;
`;

export const StatDetail = styled.span`
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.5;
`;

export const SectionGrid = styled.div`
  display: grid;
  gap: 18px;
  grid-template-columns: repeat(12, minmax(0, 1fr));
`;

export const SectionCard = styled(Card)<{ $span?: number }>`
  ${panelSurface}
  grid-column: span ${({ $span = 6 }) => $span};
  padding: 24px;

  @media (max-width: 1080px) {
    grid-column: 1 / -1;
    padding: 20px;
  }
`;

export const SectionHeader = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  margin-bottom: 18px;

  @media (max-width: 720px) {
    flex-direction: column;
  }
`;

export const InlineActions = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
`;

export const WorkspaceGrid = styled.div`
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(2, minmax(0, 1fr));

  @media (max-width: 1100px) {
    grid-template-columns: 1fr;
  }
`;

export const WorkspaceCard = styled(Card)`
  ${panelSurface}
  padding: 22px;
  transition: border-color 180ms ease;

  &:hover {
    border-color: var(--afs-line-strong);
  }
`;

export const CardHeader = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  margin-bottom: 14px;

  @media (max-width: 640px) {
    flex-direction: column;
  }
`;

export const MetaRow = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
`;

export const Tag = styled.span`
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border-radius: 999px;
  padding: 7px 11px;
  border: 1px solid var(--afs-line);
  background: var(--afs-panel);
  color: var(--afs-ink-soft);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.01em;
`;

const toneStyles = {
  blank: css`
    background: var(--afs-panel);
    color: var(--afs-ink-soft);
  `,
  "git-import": css`
    background: var(--afs-accent-soft);
    color: var(--afs-accent);
  `,
  "cloud-import": css`
    background: var(--afs-bg-soft);
    color: var(--afs-ink-soft);
  `,
} as const;

export const ToneChip = styled.span<{
  $tone: AFSWorkspaceSource;
}>`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 7px 11px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  ${({ $tone }) => toneStyles[$tone]}
`;

export const FormGrid = styled.form`
  display: grid;
  gap: 14px;
`;

export const TwoColumnFields = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(2, minmax(0, 1fr));

  @media (max-width: 760px) {
    grid-template-columns: 1fr;
  }
`;

export const Field = styled.label`
  display: flex;
  flex-direction: column;
  gap: 8px;
  color: var(--afs-ink-soft);
  font-size: 13px;
  font-weight: 700;
`;

const fieldBase = css`
  width: 100%;
  border-radius: 16px;
  border: 1px solid var(--afs-line);
  background: var(--afs-panel);
  color: var(--afs-ink);
  padding: 12px 14px;
  outline: none;
  transition:
    border-color 160ms ease,
    box-shadow 160ms ease,
    transform 160ms ease;

  &::placeholder {
    color: rgba(64, 56, 77, 0.6);
  }

  &:focus {
    border-color: var(--afs-accent);
    box-shadow: 0 0 0 3px var(--afs-accent-soft);
    transform: translateY(-1px);
  }
`;

export const TextInput = styled.input`
  ${fieldBase}
`;

export const TextArea = styled.textarea`
  ${fieldBase}
  min-height: 110px;
  resize: vertical;
`;

export const Select = styled.select`
  ${fieldBase}
`;

export const EmptyState = styled.div`
  ${insetSurface}
  padding: 18px;
`;

export const NoticeCard = styled.div<{ $tone?: "warning" | "danger" | "neutral" }>`
  ${insetSurface}
  padding: 16px 18px;
  border-color: ${({ $tone = "neutral" }) =>
    $tone === "warning"
      ? "rgba(217, 119, 6, 0.28)"
      : $tone === "danger"
        ? "rgba(220, 38, 38, 0.26)"
        : "var(--afs-line)"};
  background: ${({ $tone = "neutral" }) =>
    $tone === "warning"
      ? "rgba(245, 158, 11, 0.08)"
      : $tone === "danger"
        ? "rgba(239, 68, 68, 0.08)"
        : "var(--afs-panel-strong)"};
`;

export const NoticeTitle = styled.div`
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 700;
`;

export const NoticeBody = styled.div`
  margin-top: 6px;
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.5;
`;

export const Tabs = styled.div`
  display: inline-flex;
  gap: 8px;
  padding: 7px;
  border-radius: 999px;
  border: 1px solid var(--afs-line-strong);
  background: var(--afs-bg-soft);
`;

export const TabButton = styled.button<{ $active?: boolean }>`
  border: 1px solid ${({ $active }) => ($active ? "var(--afs-accent)" : "transparent")};
  border-radius: 999px;
  padding: 10px 15px;
  cursor: pointer;
  font-weight: 700;
  color: ${({ $active }) => ($active ? "var(--afs-ink)" : "var(--afs-muted)")};
  background: ${({ $active }) => ($active ? "var(--afs-accent-soft)" : "transparent")};
  box-shadow: ${({ $active }) =>
    $active ? "0 4px 10px rgba(8, 6, 13, 0.08)" : "none"};
  transition:
    background 160ms ease,
    color 160ms ease,
    transform 160ms ease;

  &:hover {
    background: ${({ $active }) => ($active ? "var(--afs-accent-soft)" : "var(--afs-panel)")};
    transform: translateY(-1px);
  }
`;

export const FileStudio = styled.div`
  display: grid;
  gap: 18px;
  grid-template-columns: minmax(280px, 320px) minmax(0, 1fr);

  @media (max-width: 1100px) {
    grid-template-columns: 1fr;
  }
`;

export const FileList = styled.div`
  display: flex;
  flex-direction: column;
  gap: 10px;
`;

export const FileButton = styled.button<{ $active?: boolean }>`
  display: grid;
  gap: 6px;
  width: 100%;
  border: 1px solid
    ${({ $active }) => ($active ? "var(--afs-accent)" : "var(--afs-line)")};
  background: ${({ $active }) =>
    $active ? "var(--afs-accent-soft)" : "var(--afs-panel-strong)"};
  border-radius: 18px;
  padding: 13px 14px;
  text-align: left;
  cursor: pointer;
  transition:
    transform 160ms ease,
    border-color 160ms ease,
    background 160ms ease;

  &:hover {
    transform: translateY(-1px);
    border-color: var(--afs-line-strong);
  }
`;

export const EditorPanel = styled(Card)`
  ${panelSurface}
  min-height: 520px;
  padding: 20px;
`;

export const EditorArea = styled.textarea`
  ${fieldBase}
  min-height: 420px;
  font-family: var(--afs-mono);
  font-size: 13px;
  line-height: 1.6;
  background: var(--afs-panel);
`;

export const SavepointGrid = styled.div`
  display: grid;
  gap: 12px;
`;

export const SavepointRow = styled.div`
  ${insetSurface}
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 18px;

  @media (max-width: 760px) {
    flex-direction: column;
  }
`;

export const ActivityList = styled.div`
  display: flex;
  flex-direction: column;
  gap: 12px;
`;

export const ActivityCard = styled.div`
  ${insetSurface}
  padding: 16px 18px;
`;

const TitleCopy = styled.div`
  display: grid;
  gap: 10px;
  max-width: 60rem;
`;

const TitleBody = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 15px;
  line-height: 1.65;
`;

export function SectionTitle(props: { title: string; body?: string }) {
  return (
    <TitleCopy>
      <Typography.Heading component="h2" size="S" style={{ margin: 0 }}>
        {props.title}
      </Typography.Heading>
      {props.body ? <TitleBody>{props.body}</TitleBody> : null}
    </TitleCopy>
  );
}

export const DialogOverlay = styled.div`
  position: fixed;
  inset: 0;
  z-index: 40;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(8, 6, 13, 0.36);
`;

export const DialogCard = styled.div`
  width: min(720px, 100%);
  max-height: min(88vh, 760px);
  overflow: auto;
  border: 1px solid var(--afs-line);
  border-radius: 24px;
  padding: 24px;
  background: var(--afs-panel-strong);
  box-shadow: 0 18px 40px rgba(8, 6, 13, 0.12);
`;

export const DialogHeader = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  margin-bottom: 18px;

  @media (max-width: 720px) {
    flex-direction: column;
  }
`;

export const DialogFooter = styled.div`
  position: sticky;
  bottom: 0;
  display: flex;
  justify-content: flex-end;
  gap: 16px;
  align-items: flex-end;
  margin: 20px -24px 0;
  padding: 18px 24px 24px;
  border-top: 1px solid var(--afs-line);
  background: var(--afs-panel-strong);

  @media (max-width: 720px) {
    flex-direction: column;
    align-items: stretch;
  }
`;

export const DialogTitle = styled.h2`
  margin: 0;
  color: var(--afs-ink);
  font-size: 18px;
  font-weight: 700;
  line-height: 1.3;
`;

export const DialogBody = styled.p`
  margin: 4px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.55;
`;

export const DialogCloseButton = styled.button`
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: 1px solid var(--afs-line);
  border-radius: 10px;
  background: transparent;
  color: var(--afs-muted);
  cursor: pointer;
  font-size: 18px;
  line-height: 1;
  transition: background 140ms ease, border-color 140ms ease;

  &:hover {
    background: rgba(8, 6, 13, 0.05);
    border-color: var(--afs-line-strong);
  }
`;

export const DialogActions = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
  justify-content: space-between;
`;

export const DialogError = styled.p`
  margin: 16px 0 0;
  color: #c2364a;
  font-size: 14px;
  line-height: 1.5;
`;

export function EventList(props: { events: AFSActivityEvent[] }) {
  if (props.events.length === 0) {
    return (
      <EmptyState>
        <Typography.Body component="p" color="secondary">
          No activity has been recorded yet.
        </Typography.Body>
      </EmptyState>
    );
  }

  return (
    <ActivityList>
      {props.events.map((event) => (
        <ActivityCard key={event.id}>
          <Typography.Body component="strong">{event.title}</Typography.Body>
          <Typography.Body color="secondary" component="p">
            {event.detail}
          </Typography.Body>
          <MetaRow>
            <Tag>{event.scope}</Tag>
            <Tag>{event.actor}</Tag>
            <Tag>{new Date(event.createdAt).toLocaleString()}</Tag>
          </MetaRow>
        </ActivityCard>
      ))}
    </ActivityList>
  );
}
