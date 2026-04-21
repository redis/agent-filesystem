import { Typography } from "@redis-ui/components";
import { Table } from "@redis-ui/table";
import type { ColumnDef, SortingState } from "@redis-ui/table";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useStoredViewMode } from "../hooks/use-stored-view-mode";
import type { AFSAgentSession } from "../types/afs";
import * as S from "./workspace-table.styles";
import styled, { keyframes, css } from "styled-components";
import { filterAndSortAgents, normalizeSearchValue } from "./agents-table-utils";
import type { AgentSortField } from "./agents-table-utils";
import {
  DialogOverlay,
  DialogCard,
  DialogHeader,
  DialogTitle,
  DialogCloseButton,
  Tag,
  MetaRow,
} from "../../components/afs-kit";

/* ------------------------------------------------------------------ */
/*  Helper: is the agent "active" (seen in the last 60 s)?            */
/* ------------------------------------------------------------------ */
function isAgentActive(agent: AFSAgentSession): boolean {
  return agent.state === "active" || agent.state === "starting" || agent.state === "syncing";
}

function timeAgo(iso: string): string {
  const seconds = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function uptimeLabel(iso: string): string {
  const totalSec = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (totalSec < 0) return "0s";
  const days = Math.floor(totalSec / 86400);
  const hours = Math.floor((totalSec % 86400) / 3600);
  const minutes = Math.floor((totalSec % 3600) / 60);
  const seconds = totalSec % 60;
  if (days > 0) return `${days}d ${hours}h ${minutes}m`;
  if (hours > 0) return `${hours}h ${minutes}m ${seconds}s`;
  if (minutes > 0) return `${minutes}m ${seconds}s`;
  return `${seconds}s`;
}

/** Hook that ticks every second so uptime counters stay live. */
function useTick(intervalMs = 1000) {
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
}

/* ------------------------------------------------------------------ */
/*  Styled helpers                                                     */
/* ------------------------------------------------------------------ */
const pulse = keyframes`
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
`;

const ActiveDot = styled.span<{ $active: boolean }>`
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  background: ${({ $active }) => ($active ? "#22c55e" : "#d1d5db")};
  ${({ $active }) =>
    $active &&
    css`
      box-shadow: 0 0 6px rgba(34, 197, 94, 0.5);
      animation: ${pulse} 2s ease-in-out infinite;
    `}
`;

const AgentNameWrap = styled.div`
  display: flex;
  align-items: center;
  gap: 8px;
`;

/* ---- Detail dialog ---- */
const DetailGrid = styled.div`
  display: grid;
  gap: 16px;
  grid-template-columns: 1fr 1fr;
  margin-top: 8px;

  @media (max-width: 600px) {
    grid-template-columns: 1fr;
  }
`;

const DetailField = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
`;

const DetailLabel = styled.span`
  color: var(--afs-muted, #71717a);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
`;

const DetailValue = styled.span`
  color: var(--afs-ink, #18181b);
  font-size: 14px;
  word-break: break-all;
`;

/* ---- Card view ---- */
const CardGrid = styled.div`
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(auto-fill, minmax(max(25%, 300px), 1fr));
`;

const AgentCard = styled.button<{ $active: boolean }>`
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 22px;
  border: 1px solid var(--afs-line, #e4e4e7);
  border-radius: 16px;
  background: var(--afs-panel-strong);
  text-align: left;
  cursor: pointer;
  transition: border-color 180ms ease, box-shadow 180ms ease, transform 180ms ease;

  &:hover {
    border-color: ${({ $active }) => ($active ? "rgba(34,197,94,0.4)" : "var(--afs-line-strong, #a1a1aa)")};
    box-shadow: 0 8px 24px rgba(8, 6, 13, 0.08);
    transform: translateY(-2px);
  }

  ${({ $active }) =>
    $active &&
    css`
      border-color: rgba(34, 197, 94, 0.25);
    `}
`;

const CardTop = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
`;

const CardDot = styled(ActiveDot)`
  width: 10px;
  height: 10px;
`;

const CardHostname = styled.span`
  font-size: 15px;
  font-weight: 700;
  color: var(--afs-ink, #18181b);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
  flex: 1;
`;

const CardStatusBadge = styled.span<{ $active: boolean }>`
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  padding: 3px 10px;
  border-radius: 999px;
  flex-shrink: 0;
  background: ${({ $active }) => ($active ? "rgba(34,197,94,0.12)" : "rgba(161,161,170,0.12)")};
  color: ${({ $active }) => ($active ? "#16a34a" : "#71717a")};
`;

const CardWorkspace = styled.span`
  font-size: 13px;
  font-weight: 600;
  color: var(--afs-ink-soft, #3f3f46);
`;

const CardMeta = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: auto;
`;

const CardMetaTag = styled.span`
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border-radius: 999px;
  background: var(--afs-panel);
  color: var(--afs-muted, #71717a);
  border: 1px solid var(--afs-line);
  font-size: 11px;
  font-weight: 600;
`;

/* ---- Card footer with live clock ---- */
const spin = keyframes`
  from { transform: rotate(0deg); }
  to   { transform: rotate(360deg); }
`;

const CardFooter = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--afs-line);
  margin-top: 2px;
`;

const ClockWrap = styled.div<{ $active: boolean }>`
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  color: ${({ $active }) => ($active ? "#16a34a" : "var(--afs-muted, #71717a)")};
`;

const ClockIcon = styled.span<{ $spinning: boolean }>`
  display: inline-flex;
  font-size: 14px;
  ${({ $spinning }) =>
    $spinning &&
    css`
      animation: ${spin} 3s linear infinite;
    `}
`;

const UptimeLabel = styled.span`
  font-size: 11px;
  color: var(--afs-muted, #71717a);
  font-weight: 600;
`;

/* ---- Toolbar (no bounding card) ---- */
const ToolbarWrap = styled.div`
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
`;

/* ---- View toggle ---- */
const ToggleGroup = styled.div`
  display: inline-flex;
  gap: 2px;
  padding: 3px;
  border-radius: 10px;
  background: var(--afs-panel);
  border: 1px solid var(--afs-line);
`;

const ToggleButton = styled.button<{ $active: boolean }>`
  border: none;
  border-radius: 7px;
  padding: 6px 14px;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
  color: ${({ $active }) => ($active ? "var(--afs-ink, #18181b)" : "var(--afs-muted, #71717a)")};
  background: ${({ $active }) => ($active ? "#e4e4e7" : "transparent")};
  transition: background 160ms ease, color 160ms ease;

  &:hover {
    color: var(--afs-ink, #18181b);
    background: ${({ $active }) => ($active ? "#e4e4e7" : "#f0f0f0")};
  }
`;

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */
type Props = {
  rows: AFSAgentSession[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  toolbarAction?: ReactNode;
  onOpenWorkspace: (agent: AFSAgentSession) => void;
};

/* ------------------------------------------------------------------ */
/*  Detail dialog component                                            */
/* ------------------------------------------------------------------ */
function AgentDetailDialog({
  agent,
  onClose,
  onOpenWorkspace,
}: {
  agent: AFSAgentSession;
  onClose: () => void;
  onOpenWorkspace: (agent: AFSAgentSession) => void;
}) {
  const active = isAgentActive(agent);

  return (
    <DialogOverlay onClick={onClose}>
      <DialogCard onClick={(e) => e.stopPropagation()}>
        <DialogHeader>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <ActiveDot $active={active} style={{ width: 10, height: 10 }} />
            <DialogTitle>{agent.hostname || "Unknown Agent"}</DialogTitle>
          </div>
          <DialogCloseButton onClick={onClose}>&times;</DialogCloseButton>
        </DialogHeader>

        <DetailGrid>
          <DetailField>
            <DetailLabel>Status</DetailLabel>
            <DetailValue>
              {active ? "Active" : "Inactive"} &mdash; {agent.state}
            </DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Workspace</DetailLabel>
            <DetailValue>{agent.workspaceName}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Client Kind</DetailLabel>
            <DetailValue>{agent.clientKind || "client"}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Access Mode</DetailLabel>
            <DetailValue>{agent.readonly ? "Read-only" : "Read / Write"}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Operating System</DetailLabel>
            <DetailValue>{agent.operatingSystem || "Not reported"}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>AFS Version</DetailLabel>
            <DetailValue>{agent.afsVersion || "Unknown"}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Local Path</DetailLabel>
            <DetailValue>{agent.localPath || "Not reported"}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Session ID</DetailLabel>
            <DetailValue style={{ fontSize: 12 }}>{agent.sessionId}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Started</DetailLabel>
            <DetailValue>{new Date(agent.startedAt).toLocaleString()}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Last Seen</DetailLabel>
            <DetailValue>{new Date(agent.lastSeenAt).toLocaleString()}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Lease Expires</DetailLabel>
            <DetailValue>{new Date(agent.leaseExpiresAt).toLocaleString()}</DetailValue>
          </DetailField>
        </DetailGrid>

        <MetaRow style={{ marginTop: 20 }}>
          <Tag>{agent.clientKind || "client"}</Tag>
          <Tag>{agent.readonly ? "readonly" : "read/write"}</Tag>
          {agent.operatingSystem ? <Tag>{agent.operatingSystem}</Tag> : null}
        </MetaRow>

        <div style={{ display: "flex", gap: 10, marginTop: 18 }}>
          <S.TextActionButton
            type="button"
            onClick={() => onOpenWorkspace(agent)}
          >
            Open Workspace
          </S.TextActionButton>
        </div>
      </DialogCard>
    </DialogOverlay>
  );
}

/* ------------------------------------------------------------------ */
/*  Main component                                                     */
/* ------------------------------------------------------------------ */
export function AgentsTable({
  rows,
  loading = false,
  error = false,
  errorMessage = "Unable to load connected agents. Please retry.",
  toolbarAction,
  onOpenWorkspace,
}: Props) {
  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] = useState<AgentSortField>("lastSeenAt");
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("desc");
  const [viewMode, setViewMode] = useStoredViewMode("afs.agents.viewMode");
  const [selectedAgent, setSelectedAgent] = useState<AFSAgentSession | null>(null);

  // Tick every second so live counters (uptime, time-ago) update in real-time.
  useTick();

  const filteredRows = useMemo(
    () => filterAndSortAgents(rows, search, sortBy, sortDirection),
    [rows, search, sortBy, sortDirection],
  );

  const sorting = useMemo<SortingState>(
    () => [{ id: sortBy, desc: sortDirection === "desc" }],
    [sortBy, sortDirection],
  );
  const isFiltering = normalizeSearchValue(search) !== "";

  const columns = useMemo(
    () =>
      [
        {
          accessorKey: "hostname",
          header: "System Name",
          size: 200,
          enableSorting: true,
          cell: ({ row }) => {
            const active = isAgentActive(row.original);
            return (
              <AgentNameWrap>
                <ActiveDot $active={active} />
                <S.SingleLineText title={row.original.hostname || "unknown host"}>
                  {row.original.hostname || "unknown host"}
                </S.SingleLineText>
              </AgentNameWrap>
            );
          },
        },
        {
          accessorKey: "workspaceName",
          header: "Workspace",
          size: 180,
          enableSorting: true,
          cell: ({ row }) => (
            <S.SingleLineText title={row.original.workspaceName}>
              {row.original.workspaceName}
            </S.SingleLineText>
          ),
        },
        {
          accessorKey: "lastSeenAt",
          header: "Last Active",
          size: 120,
          enableSorting: true,
          cell: ({ row }) => {
            const active = isAgentActive(row.original);
            return (
              <Typography.Body
                component="span"
                color={active ? undefined : "secondary"}
              >
                {active ? "Active now" : timeAgo(row.original.lastSeenAt)}
              </Typography.Body>
            );
          },
        },
      ] as ColumnDef<AFSAgentSession>[],
    [],
  );

  return (
    <>
      <ToolbarWrap>
        <S.SearchInput
          value={search}
          onChange={setSearch}
          placeholder="Search by name, path, workspace..."
        />
        <ToggleGroup>
          <ToggleButton
            $active={viewMode === "cards"}
            onClick={() => setViewMode("cards")}
          >
            Cards
          </ToggleButton>
          <ToggleButton
            $active={viewMode === "table"}
            onClick={() => setViewMode("table")}
          >
            Table
          </ToggleButton>
        </ToggleGroup>
        {toolbarAction}
      </ToolbarWrap>

      {loading ? <S.EmptyState>Loading connected agents...</S.EmptyState> : null}
      {error ? <S.EmptyState role="alert">{errorMessage}</S.EmptyState> : null}
      {!loading && !error && filteredRows.length === 0 ? (
        <S.EmptyState>
          {isFiltering
            ? "No agents match the current filter."
            : "No connected agents are currently reporting in."}
        </S.EmptyState>
      ) : null}

      {/* ---- TABLE VIEW ---- */}
      {!loading && !error && filteredRows.length > 0 && viewMode === "table" ? (
        <S.TableCard>
          <S.DenseTableViewport>
            <Table
              columns={columns}
              data={filteredRows}
              sorting={sorting}
              manualSorting
              onSortingChange={(nextState) => {
                if (nextState.length === 0) {
                  setSortBy("lastSeenAt");
                  setSortDirection("desc");
                  return;
                }
                const next = nextState[0];
                setSortBy(next.id as AgentSortField);
                setSortDirection(next.desc ? "desc" : "asc");
              }}
              enableSorting
              stripedRows
              onRowClick={(rowData) => setSelectedAgent(rowData)}
            />
          </S.DenseTableViewport>
        </S.TableCard>
      ) : null}

      {/* ---- CARD VIEW ---- */}
      {!loading && !error && filteredRows.length > 0 && viewMode === "cards" ? (
        <CardGrid>
          {filteredRows.map((agent) => {
            const active = isAgentActive(agent);
            return (
              <AgentCard
                key={agent.sessionId}
                $active={active}
                onClick={() => setSelectedAgent(agent)}
              >
                <CardTop>
                  <CardDot $active={active} />
                  <CardHostname>{agent.hostname || "unknown host"}</CardHostname>
                  <CardStatusBadge $active={active}>
                    {active ? "Active" : "Inactive"}
                  </CardStatusBadge>
                </CardTop>

                <CardWorkspace>
                  {agent.workspaceName}
                  {agent.localPath ? <> &rarr; {agent.localPath}</> : null}
                </CardWorkspace>

                <CardMeta>
                  <CardMetaTag>{agent.clientKind || "client"}</CardMetaTag>
                  <CardMetaTag>{agent.readonly ? "RO" : "RW"}</CardMetaTag>
                  {agent.operatingSystem ? (
                    <CardMetaTag>{agent.operatingSystem}</CardMetaTag>
                  ) : null}
                  {agent.afsVersion ? (
                    <CardMetaTag>v{agent.afsVersion}</CardMetaTag>
                  ) : null}
                </CardMeta>

                <CardFooter>
                  <ClockWrap $active={active}>
                    <ClockIcon $spinning={active}>
                      {active ? "\u23F1" : "\u23F0"}
                    </ClockIcon>
                    {active
                      ? `Active \u00B7 ${timeAgo(agent.lastSeenAt)}`
                      : `Last seen ${timeAgo(agent.lastSeenAt)}`}
                  </ClockWrap>
                  <UptimeLabel>
                    Uptime: {uptimeLabel(agent.startedAt)}
                  </UptimeLabel>
                </CardFooter>
              </AgentCard>
            );
          })}
        </CardGrid>
      ) : null}

      {/* ---- DETAIL DIALOG ---- */}
      {selectedAgent != null ? (
        <AgentDetailDialog
          agent={selectedAgent}
          onClose={() => setSelectedAgent(null)}
          onOpenWorkspace={onOpenWorkspace}
        />
      ) : null}
    </>
  );
}
