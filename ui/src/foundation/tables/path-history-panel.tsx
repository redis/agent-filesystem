import styled from "styled-components";
import type { AFSEventEntry } from "../types/afs";

type Props = {
  path: string;
  rows: AFSEventEntry[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  onOpenEvent?: (event: AFSEventEntry) => void;
};

export function PathHistoryPanel({
  path,
  rows,
  loading = false,
  error = false,
  errorMessage = "Unable to load path history.",
  onOpenEvent,
}: Props) {
  return (
    <Panel>
      <PanelHeader>
        <PanelTitle>Path history</PanelTitle>
        <PanelPath title={path}>{path}</PanelPath>
      </PanelHeader>

      {loading ? <PanelMessage>Loading history...</PanelMessage> : null}
      {error ? <PanelMessage role="alert">{errorMessage}</PanelMessage> : null}
      {!loading && !error && rows.length === 0 ? (
        <PanelMessage>No history found for this path.</PanelMessage>
      ) : null}

      {!loading && !error && rows.length > 0 ? (
        <EventList>
          {rows.map((event) => (
            <EventRow
              key={event.id}
              type="button"
              onClick={() => onOpenEvent?.(event)}
              $clickable={onOpenEvent != null}
            >
              <EventMain>
                <EventTitle>{eventTitle(event)}</EventTitle>
                <EventMeta>{eventMeta(event)}</EventMeta>
              </EventMain>
              <EventTags>
                {event.checkpointId ? (
                  <EventTag title={event.checkpointId}>checkpoint {shortID(event.checkpointId)}</EventTag>
                ) : null}
                {event.sessionId ? (
                  <EventTag title={event.sessionId}>session {shortID(event.sessionId)}</EventTag>
                ) : null}
              </EventTags>
            </EventRow>
          ))}
        </EventList>
      ) : null}
    </Panel>
  );
}

function eventTitle(event: AFSEventEntry) {
  const title = [formatToken(event.op), formatToken(event.kind)]
    .filter((part) => part !== "")
    .join(" ");
  return title || event.id;
}

function eventMeta(event: AFSEventEntry) {
  return [
    formatEventTime(event.createdAt),
    event.actor || event.label || event.user || event.sessionId || "afs",
    formatDelta(event.deltaBytes),
  ]
    .filter((part) => part !== "")
    .join(" / ");
}

function formatToken(value?: string) {
  return (value ?? "")
    .split(/[-_.]/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function formatEventTime(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const now = new Date();
  const isToday = date.toDateString() === now.toDateString();
  const time = date.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  if (isToday) return time;
  return `${date.toLocaleDateString(undefined, { month: "short", day: "numeric" })} ${time}`;
}

function formatDelta(value?: number) {
  if (value == null) return "";
  if (value === 0) return "0 B";
  const sign = value > 0 ? "+" : "-";
  const absolute = Math.abs(value);
  if (absolute < 1024) return `${sign}${absolute} B`;
  if (absolute < 1024 * 1024) return `${sign}${(absolute / 1024).toFixed(absolute < 10 * 1024 ? 1 : 0)} KB`;
  return `${sign}${(absolute / (1024 * 1024)).toFixed(1)} MB`;
}

function shortID(value: string) {
  return value.length <= 12 ? value : value.slice(0, 12);
}

const Panel = styled.aside`
  flex: 0 0 auto;
  border-top: 1px solid var(--afs-line);
  background: var(--afs-panel);
`;

const PanelHeader = styled.div`
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  gap: 10px;
  padding: 10px 16px;
  border-bottom: 1px solid var(--afs-line);
`;

const PanelTitle = styled.h3`
  margin: 0;
  color: var(--afs-ink);
  font-size: 13px;
  font-weight: 700;
`;

const PanelPath = styled.span`
  min-width: 0;
  overflow: hidden;
  color: var(--afs-muted);
  font-family: var(--afs-mono);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const PanelMessage = styled.div`
  padding: 18px 16px;
  color: var(--afs-muted);
  font-size: 13px;
`;

const EventList = styled.div`
  max-height: 220px;
  overflow: auto;
`;

const EventRow = styled.button<{ $clickable: boolean }>`
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px;
  width: 100%;
  min-height: 54px;
  padding: 10px 16px;
  border: none;
  border-bottom: 1px solid var(--afs-line);
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: ${({ $clickable }) => ($clickable ? "pointer" : "default")};

  &:hover {
    background: ${({ $clickable }) => ($clickable ? "var(--afs-panel-strong)" : "transparent")};
  }

  &:last-child {
    border-bottom: none;
  }
`;

const EventMain = styled.span`
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 4px;
`;

const EventTitle = styled.span`
  overflow: hidden;
  color: var(--afs-ink);
  font-size: 13px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const EventMeta = styled.span`
  overflow: hidden;
  color: var(--afs-muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const EventTags = styled.span`
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px;
`;

const EventTag = styled.span`
  display: inline-flex;
  align-items: center;
  min-height: 22px;
  padding: 0 7px;
  border: 1px solid var(--afs-line);
  border-radius: 999px;
  background: var(--afs-panel-strong);
  color: var(--afs-ink-soft);
  font-size: 11px;
  font-weight: 700;
  white-space: nowrap;
`;
