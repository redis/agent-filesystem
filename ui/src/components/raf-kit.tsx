import { Card, Typography } from "@redislabsdev/redis-ui-components";
import styled, { css } from "styled-components";
import type {
  RAFActivityEvent,
  RAFSessionStatus,
  RAFWorkspaceSource,
  RAFWorkspaceStatus,
} from "../foundation/types/raf";

export const PageStack = styled.div`
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding: 24px 32px 40px;

  @media (max-width: 900px) {
    padding: 20px;
  }
`;

export const HeroCard = styled(Card)`
  overflow: hidden;
  position: relative;
  background:
    linear-gradient(135deg, rgba(125, 20, 255, 0.1), rgba(71, 191, 255, 0.12)),
    ${({ theme }) => theme.semantic.color.background.neutral0};
  border: 1px solid ${({ theme }) => theme.semantic.color.border.neutral200};
`;

export const HeroLayout = styled.div`
  display: grid;
  gap: 18px;
  grid-template-columns: 1.4fr 1fr;
  padding: 28px;

  @media (max-width: 1080px) {
    grid-template-columns: 1fr;
  }
`;

export const Eyebrow = styled.span`
  display: inline-flex;
  align-items: center;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(125, 20, 255, 0.12);
  color: ${({ theme }) => theme.semantic.color.text.neutral900};
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
`;

export const HeroBody = styled.div`
  display: flex;
  flex-direction: column;
  gap: 14px;
`;

export const HeroMetaGrid = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
`;

export const MetaItem = styled.div`
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.72);
  padding: 14px 16px;
  border: 1px solid rgba(125, 20, 255, 0.08);
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
  padding: 18px 20px;
`;

export const SectionGrid = styled.div`
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(12, minmax(0, 1fr));
`;

export const SectionCard = styled(Card)<{ $span?: number }>`
  grid-column: span ${({ $span = 6 }) => $span};
  padding: 20px;

  @media (max-width: 1080px) {
    grid-column: 1 / -1;
  }
`;

export const SectionHeader = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  margin-bottom: 16px;
`;

export const InlineActions = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
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
  padding: 20px;
`;

export const CardHeader = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  margin-bottom: 14px;
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
  border-radius: 999px;
  padding: 6px 10px;
  background: ${({ theme }) => theme.semantic.color.background.neutral100};
  color: ${({ theme }) => theme.semantic.color.text.neutral700};
  font-size: 12px;
  font-weight: 600;
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
    background: rgba(125, 20, 255, 0.12);
    color: #5f0ec1;
  `,
  "cloud-import": css`
    background: rgba(71, 191, 255, 0.14);
    color: #095b8a;
  `,
} as const;

export const ToneChip = styled.span<{
  $tone: RAFWorkspaceStatus | RAFSessionStatus | RAFWorkspaceSource;
}>`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 6px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  text-transform: capitalize;
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
  color: ${({ theme }) => theme.semantic.color.text.neutral700};
  font-size: 13px;
  font-weight: 600;
`;

const fieldBase = css`
  width: 100%;
  border-radius: 12px;
  border: 1px solid ${({ theme }) => theme.semantic.color.border.neutral200};
  background: ${({ theme }) => theme.semantic.color.background.neutral0};
  color: ${({ theme }) => theme.semantic.color.text.neutral900};
  padding: 12px 14px;
  outline: none;

  &:focus {
    border-color: #7d14ff;
    box-shadow: 0 0 0 3px rgba(125, 20, 255, 0.12);
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
  border: 1px dashed ${({ theme }) => theme.semantic.color.border.neutral200};
  border-radius: 16px;
  padding: 18px;
  background: ${({ theme }) => theme.semantic.color.background.neutral0};
`;

export const SessionLayout = styled.div`
  display: grid;
  gap: 16px;
  grid-template-columns: 280px 1fr;

  @media (max-width: 1080px) {
    grid-template-columns: 1fr;
  }
`;

export const SessionRail = styled.div`
  display: flex;
  flex-direction: column;
  gap: 10px;
`;

export const SessionButton = styled.button<{ $active?: boolean }>`
  width: 100%;
  border-radius: 16px;
  border: 1px solid
    ${({ theme, $active }) =>
      $active ? "#7d14ff" : theme.semantic.color.border.neutral200};
  background:
    ${({ $active }) =>
      $active
        ? "linear-gradient(135deg, rgba(125, 20, 255, 0.08), rgba(71, 191, 255, 0.08))"
        : "#fff"};
  padding: 14px 16px;
  text-align: left;
  cursor: pointer;
`;

export const Tabs = styled.div`
  display: inline-flex;
  gap: 8px;
  padding: 6px;
  border-radius: 999px;
  background: rgba(8, 6, 13, 0.05);
`;

export const TabButton = styled.button<{ $active?: boolean }>`
  border: none;
  border-radius: 999px;
  padding: 10px 14px;
  cursor: pointer;
  font-weight: 700;
  color: ${({ theme, $active }) =>
    $active ? theme.semantic.color.text.neutral900 : theme.semantic.color.text.neutral600};
  background: ${({ $active }) => ($active ? "#fff" : "transparent")};
  box-shadow: ${({ $active }) => ($active ? "0 4px 10px rgba(8, 6, 13, 0.08)" : "none")};
`;

export const FileStudio = styled.div`
  display: grid;
  gap: 16px;
  grid-template-columns: 280px 1fr;

  @media (max-width: 1100px) {
    grid-template-columns: 1fr;
  }
`;

export const FileList = styled.div`
  display: flex;
  flex-direction: column;
  gap: 8px;
`;

export const FileButton = styled.button<{ $active?: boolean }>`
  border: 1px solid
    ${({ theme, $active }) =>
      $active ? "#47bfff" : theme.semantic.color.border.neutral200};
  background: ${({ $active }) => ($active ? "rgba(71, 191, 255, 0.08)" : "#fff")};
  border-radius: 14px;
  padding: 12px 14px;
  text-align: left;
  cursor: pointer;
`;

export const EditorPanel = styled(Card)`
  padding: 18px;
`;

export const EditorArea = styled.textarea`
  ${fieldBase}
  min-height: 420px;
  font-family: "Monaco", "Menlo", monospace;
  font-size: 13px;
  line-height: 1.5;
`;

export const SavepointGrid = styled.div`
  display: grid;
  gap: 12px;
`;

export const SavepointRow = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 0;
  border-top: 1px solid ${({ theme }) => theme.semantic.color.border.neutral200};

  &:first-child {
    border-top: none;
    padding-top: 0;
  }

  @media (max-width: 760px) {
    flex-direction: column;
  }
`;

export const ActivityList = styled.div`
  display: flex;
  flex-direction: column;
  gap: 10px;
`;

export const ActivityCard = styled.div`
  padding: 14px 16px;
  border-radius: 16px;
  background: ${({ theme }) => theme.semantic.color.background.neutral0};
  border: 1px solid ${({ theme }) => theme.semantic.color.border.neutral200};
`;

export function SectionTitle(props: { eyebrow?: string; title: string; body: string }) {
  return (
    <div>
      {props.eyebrow ? <Eyebrow>{props.eyebrow}</Eyebrow> : null}
      <Typography.Heading component="h2" size="S" style={{ marginTop: props.eyebrow ? 12 : 0 }}>
        {props.title}
      </Typography.Heading>
      <Typography.Body color="secondary" component="p">
        {props.body}
      </Typography.Body>
    </div>
  );
}

export function EventList(props: { events: RAFActivityEvent[] }) {
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
