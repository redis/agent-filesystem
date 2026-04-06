import { Card, Typography } from "@redislabsdev/redis-ui-components";
import styled, { css } from "styled-components";
import type {
  AFSActivityEvent,
  AFSDraftState,
  AFSWorkspaceSource,
  AFSWorkspaceStatus,
} from "../foundation/types/afs";

const panelSurface = css`
  position: relative;
  overflow: hidden;
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(249, 251, 255, 0.92)),
    var(--afs-panel);
  box-shadow: var(--afs-shadow);

  &::before {
    content: "";
    position: absolute;
    inset: 0;
    pointer-events: none;
    background:
      linear-gradient(140deg, rgba(170, 59, 255, 0.08), transparent 36%),
      radial-gradient(circle at top right, rgba(71, 191, 255, 0.12), transparent 30%);
    opacity: 0.9;
  }

  > * {
    position: relative;
    z-index: 1;
  }
`;

const insetSurface = css`
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.74), rgba(243, 246, 251, 0.9)),
    rgba(255, 255, 255, 0.9);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.7);
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

export const HeroCard = styled(Card)`
  ${panelSurface}
  background:
    linear-gradient(135deg, rgba(170, 59, 255, 0.1), rgba(71, 191, 255, 0.12)),
    var(--afs-panel);

  &::after {
    content: "";
    position: absolute;
    inset: 0;
    pointer-events: none;
    background-image:
      linear-gradient(rgba(170, 59, 255, 0.05) 1px, transparent 1px),
      linear-gradient(90deg, rgba(170, 59, 255, 0.05) 1px, transparent 1px);
    background-size: 28px 28px;
    mask-image: linear-gradient(135deg, rgba(0, 0, 0, 0.45), transparent 78%);
  }
`;

export const HeroLayout = styled.div`
  display: grid;
  gap: 22px;
  grid-template-columns: minmax(0, 1.25fr) minmax(320px, 0.95fr);
  padding: 32px;

  @media (max-width: 1080px) {
    grid-template-columns: 1fr;
    padding: 24px;
  }
`;

export const Eyebrow = styled.span`
  display: inline-flex;
  align-items: center;
  gap: 8px;
  width: fit-content;
  padding: 7px 12px;
  border-radius: 999px;
  background: var(--afs-accent-soft);
  color: var(--afs-ink-soft);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

export const HeroBody = styled.div`
  display: flex;
  flex-direction: column;
  gap: 16px;
`;

export const HeroMetaGrid = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(2, minmax(0, 1fr));

  @media (max-width: 640px) {
    grid-template-columns: 1fr;
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
  gap: 18px;
  min-height: 152px;
  padding: 22px 22px 20px;
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
  font-size: clamp(2rem, 3vw, 2.8rem);
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
  transition:
    transform 180ms ease,
    border-color 180ms ease,
    box-shadow 180ms ease;

  &:hover {
    transform: translateY(-2px);
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
  background: rgba(255, 255, 255, 0.78);
  color: var(--afs-ink-soft);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.01em;
`;

const toneStyles = {
  healthy: css`
    background: rgba(26, 172, 79, 0.12);
    color: #12703d;
  `,
  syncing: css`
    background: rgba(71, 191, 255, 0.14);
    color: #095b8a;
  `,
  attention: css`
    background: rgba(255, 174, 36, 0.16);
    color: #7c5005;
  `,
  clean: css`
    background: rgba(26, 172, 79, 0.12);
    color: #12703d;
  `,
  dirty: css`
    background: rgba(255, 174, 36, 0.16);
    color: #7c5005;
  `,
  blank: css`
    background: rgba(8, 6, 13, 0.08);
    color: #40384d;
  `,
  "git-import": css`
    background: rgba(170, 59, 255, 0.12);
    color: #7d14ff;
  `,
  "cloud-import": css`
    background: rgba(71, 191, 255, 0.14);
    color: #095b8a;
  `,
} as const;

export const ToneChip = styled.span<{
  $tone: AFSWorkspaceStatus | AFSDraftState | AFSWorkspaceSource;
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
  background: rgba(255, 255, 255, 0.94);
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

export const Tabs = styled.div`
  display: inline-flex;
  gap: 8px;
  padding: 7px;
  border-radius: 999px;
  border: 1px solid rgba(8, 6, 13, 0.06);
  background: rgba(8, 6, 13, 0.05);
`;

export const TabButton = styled.button<{ $active?: boolean }>`
  border: none;
  border-radius: 999px;
  padding: 10px 15px;
  cursor: pointer;
  font-weight: 700;
  color: ${({ $active }) => ($active ? "var(--afs-ink)" : "var(--afs-muted)")};
  background: ${({ $active }) => ($active ? "#fff" : "transparent")};
  box-shadow: ${({ $active }) =>
    $active ? "0 4px 10px rgba(8, 6, 13, 0.08)" : "none"};
  transition:
    background 160ms ease,
    color 160ms ease,
    transform 160ms ease;

  &:hover {
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
    ${({ $active }) => ($active ? "#47bfff" : "var(--afs-line)")};
  background: ${({ $active }) =>
    $active ? "rgba(71, 191, 255, 0.08)" : "#fff"};
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
    border-color: rgba(170, 59, 255, 0.18);
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
  background: rgba(255, 255, 255, 0.96);
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

export function SectionTitle(props: { eyebrow?: string; title: string; body: string }) {
  return (
    <TitleCopy>
      {props.eyebrow ? <Eyebrow>{props.eyebrow}</Eyebrow> : null}
      <Typography.Heading component="h2" size="S" style={{ margin: 0 }}>
        {props.title}
      </Typography.Heading>
      <TitleBody>{props.body}</TitleBody>
    </TitleCopy>
  );
}

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
