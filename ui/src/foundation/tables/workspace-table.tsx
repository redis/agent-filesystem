import { Menu, Typography } from "@redis-ui/components";
import { FoldersIcon, MoreactionsIcon } from "@redis-ui/icons/monochrome";
import { Table } from "@redis-ui/table";
import type { ColumnDef, SortingState } from "@redis-ui/table";
import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import styled, { css, keyframes } from "styled-components";
import { formatBytes } from "../api/afs";
import { useStoredViewMode } from "../hooks/use-stored-view-mode";
import type { AFSWorkspaceSummary } from "../types/afs";
import type { StudioTab } from "../workspace-tabs";
import * as S from "./workspace-table.styles";

type WorkspaceSortField =
  | "name"
  | "cloudAccount"
  | "connectedAgents"
  | "databaseName"
  | "totalBytes"
  | "updatedAt";

type RowWorkspaceSortField = Exclude<WorkspaceSortField, "connectedAgents">;

/** Short date like "4/16 5:04p" */
function shortDateTime(iso: string): string {
  const d = new Date(iso);
  const month = d.getMonth() + 1;
  const day = d.getDate();
  let hours = d.getHours();
  const minutes = d.getMinutes();
  const ampm = hours >= 12 ? "p" : "a";
  hours = hours % 12 || 12;
  return `${month}/${day} ${hours}:${minutes.toString().padStart(2, "0")}${ampm}`;
}

/** Spelled-out relative time: "5 seconds ago", "2 minutes ago", "3 hours ago", "4 days ago" */
function relativeTimeAgo(iso: string): string {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 1000));
  if (seconds < 60) return `${seconds} second${seconds === 1 ? "" : "s"} ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} minute${minutes === 1 ? "" : "s"} ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} hour${hours === 1 ? "" : "s"} ago`;
  const days = Math.floor(hours / 24);
  return `${days} day${days === 1 ? "" : "s"} ago`;
}

/** Re-render once per minute so relative times stay fresh. */
function useMinuteTick() {
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 30000);
    return () => clearInterval(id);
  }, []);
}

type Props = {
  rows: AFSWorkspaceSummary[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  toolbarAction?: ReactNode;
  connectedAgentsByWorkspace?: Record<string, number>;
  onOpenWorkspace: (workspace: AFSWorkspaceSummary) => void;
  onPreviewWorkspace?: (workspace: AFSWorkspaceSummary) => void;
  onOpenWorkspaceTab?: (workspace: AFSWorkspaceSummary, tab: StudioTab) => void;
  onEditWorkspace: (workspace: AFSWorkspaceSummary) => void;
  onDeleteWorkspace: (workspace: AFSWorkspaceSummary) => void;
  deletingWorkspaceKey?: string | null;
};

function workspaceRowKey(workspace: AFSWorkspaceSummary) {
  return workspace.id;
}

function compareValues(
  left: string | number,
  right: string | number,
  direction: "asc" | "desc",
) {
  const result =
    typeof left === "number" && typeof right === "number"
      ? left - right
      : String(left).localeCompare(String(right));

  return direction === "asc" ? result : result * -1;
}

export function WorkspaceTable({
  rows,
  loading = false,
  error = false,
  errorMessage = "Unable to load workspaces. Please retry.",
  toolbarAction,
  connectedAgentsByWorkspace = {},
  onOpenWorkspace,
  onPreviewWorkspace,
  onOpenWorkspaceTab,
  onEditWorkspace,
  onDeleteWorkspace,
  deletingWorkspaceKey = null,
}: Props) {
  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] = useState<WorkspaceSortField>("updatedAt");
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("desc");
  const [viewMode, setViewMode] = useStoredViewMode("afs.workspaces.viewMode");
  const [copiedId, setCopiedId] = useState<string | null>(null);
  useMinuteTick();

  async function copyWorkspaceId(id: string) {
    try {
      await navigator.clipboard.writeText(id);
      setCopiedId(id);
      window.setTimeout(() => {
        setCopiedId((current) => (current === id ? null : current));
      }, 1500);
    } catch {
      /* ignore */
    }
  }

  const filteredRows = useMemo(() => {
    const query = search.trim().toLowerCase();
    const baseRows =
      query === ""
        ? rows
        : rows.filter((row) =>
            [row.name, row.databaseName, row.redisKey, row.region, row.cloudAccount].some((value) =>
              value.toLowerCase().includes(query),
            ),
          );

    return [...baseRows].sort((left, right) => {
      const leftValue = sortBy === "connectedAgents"
        ? connectedAgentsByWorkspace[workspaceRowKey(left)] ?? 0
        : left[sortBy as RowWorkspaceSortField];
      const rightValue = sortBy === "connectedAgents"
        ? connectedAgentsByWorkspace[workspaceRowKey(right)] ?? 0
        : right[sortBy as RowWorkspaceSortField];
      return compareValues(leftValue, rightValue, sortDirection);
    });
  }, [connectedAgentsByWorkspace, rows, search, sortBy, sortDirection]);

  const sorting = useMemo<SortingState>(
    () => [{ id: sortBy, desc: sortDirection === "desc" }],
    [sortBy, sortDirection],
  );
  const isFiltering = search.trim() !== "";

  const columns = useMemo(
    () =>
      [
        {
          accessorKey: "name",
          header: "Name",
          size: 160,
          enableSorting: true,
          cell: ({ row }) => (
            <S.WorkspaceNameButton
              onClick={(event) => {
                event.stopPropagation();
                onOpenWorkspace(row.original);
              }}
              onMouseEnter={() => onPreviewWorkspace?.(row.original)}
              onFocus={() => onPreviewWorkspace?.(row.original)}
            >
              {row.original.name}
            </S.WorkspaceNameButton>
          ),
        },
        {
          id: "connectedAgents",
          header: "Agents",
          size: 60,
          enableSorting: true,
          cell: ({ row }) => {
            const count = connectedAgentsByWorkspace[workspaceRowKey(row.original)] ?? 0;
            return (
              <S.CountCell>
                <LiveDot $active={count > 0} />
                <Typography.Body component="strong">{count}</Typography.Body>
              </S.CountCell>
            );
          },
        },
        {
          accessorKey: "totalBytes",
          header: "Size",
          size: 90,
          enableSorting: true,
          cell: ({ row }) => (
            <SizeCell>
              <strong>{formatBytes(row.original.totalBytes)}</strong>
              <DetailsMuted>
                {" "}
                · {row.original.fileCount} file{row.original.fileCount === 1 ? "" : "s"}
              </DetailsMuted>
            </SizeCell>
          ),
        },
        {
          accessorKey: "databaseName",
          header: "Details",
          size: 220,
          enableSorting: true,
          cell: ({ row }) => {
            const id = row.original.id;
            const idDisplay = id.length > 22 ? `${id.slice(0, 22)}…` : id;
            return (
              <DetailsStack>
                <DetailsRow>
                  <DetailsLabel>DB:</DetailsLabel>
                  <DetailsValue title={row.original.databaseName}>
                    {row.original.databaseName}
                  </DetailsValue>
                </DetailsRow>
                <DetailsRow>
                  <DetailsLabel>Workspace ID:</DetailsLabel>
                  <DetailsMono title={id}>{idDisplay}</DetailsMono>
                  <CopyButton
                    type="button"
                    aria-label={`Copy workspace ID ${id}`}
                    title={copiedId === id ? "Copied" : "Copy workspace ID"}
                    onClick={(event) => {
                      event.stopPropagation();
                      void copyWorkspaceId(id);
                    }}
                  >
                    {copiedId === id ? <CheckIcon /> : <CopyIcon />}
                  </CopyButton>
                </DetailsRow>
              </DetailsStack>
            );
          },
        },
        {
          accessorKey: "updatedAt",
          header: "Last updated",
          size: 130,
          enableSorting: true,
          cell: ({ row }) => (
            <UpdatedStack>
              <UpdatedDate>{shortDateTime(row.original.updatedAt)}</UpdatedDate>
              <UpdatedAgo>{relativeTimeAgo(row.original.updatedAt)}</UpdatedAgo>
            </UpdatedStack>
          ),
        },
        {
          id: "actions",
          header: "Actions",
          size: 72,
          maxSize: 72,
          enableSorting: false,
          cell: ({ row }) => (
            <Menu>
              <Menu.Trigger withButton={false}>
                <S.MoreActionsTrigger
                  aria-label={`More actions for ${row.original.name}`}
                  onClick={(event) => {
                    event.stopPropagation();
                  }}
                >
                  <MoreactionsIcon size="S" />
                </S.MoreActionsTrigger>
              </Menu.Trigger>
              <Menu.Content align="end" onClick={(e: React.MouseEvent) => e.stopPropagation()}>
                <Menu.Content.Item
                  text="Open workspace"
                  onClick={() => onOpenWorkspace(row.original)}
                />
                <Menu.Content.Item
                  text="Edit workspace"
                  onClick={() => onEditWorkspace(row.original)}
                />
                <Menu.Content.Item
                  text={deletingWorkspaceKey === workspaceRowKey(row.original) ? "Deleting..." : "Delete workspace"}
                  onClick={() => {
                    if (deletingWorkspaceKey === workspaceRowKey(row.original)) {
                      return;
                    }
                    onDeleteWorkspace(row.original);
                  }}
                />
              </Menu.Content>
            </Menu>
          ),
        },
      ] as ColumnDef<AFSWorkspaceSummary>[],
    [connectedAgentsByWorkspace, copiedId, deletingWorkspaceKey, onDeleteWorkspace, onEditWorkspace, onOpenWorkspace, onPreviewWorkspace],
  );

  return (
    <>
      <S.HeadingWrap style={{ padding: 0 }}>
        <S.SearchInput
          value={search}
          onChange={setSearch}
          placeholder="Search workspace, database, ..."
        />
        <S.ToggleGroup>
          <S.ToggleButton
            $active={viewMode === "cards"}
            onClick={() => setViewMode("cards")}
          >
            Cards
          </S.ToggleButton>
          <S.ToggleButton
            $active={viewMode === "table"}
            onClick={() => setViewMode("table")}
          >
            Table
          </S.ToggleButton>
        </S.ToggleGroup>
        {toolbarAction}
      </S.HeadingWrap>

      {loading ? <S.EmptyState>Loading workspaces...</S.EmptyState> : null}
      {error ? <S.EmptyState role="alert">{errorMessage}</S.EmptyState> : null}
      {!loading && !error && filteredRows.length === 0 ? (
        <S.EmptyState>
          {isFiltering
            ? "No workspaces match the current filter."
            : "No workspaces yet. Use Add workspace to create one."}
        </S.EmptyState>
      ) : null}

      {/* ---- TABLE VIEW ---- */}
      {!loading && !error && filteredRows.length > 0 && viewMode === "table" ? (
        <S.TableCard>
          <WorkspaceTableViewport>
            <Table
              columns={columns}
              data={filteredRows}
              sorting={sorting}
              manualSorting
              onSortingChange={(nextState) => {
                if (nextState.length === 0) {
                  setSortBy("updatedAt");
                  setSortDirection("desc");
                  return;
                }

                const next = nextState[0];
                const nextSortBy = next.id as WorkspaceSortField;
                setSortBy(nextSortBy);
                setSortDirection(next.desc ? "desc" : "asc");
              }}
              enableSorting
              stripedRows
              onRowClick={(rowData) => onOpenWorkspace(rowData)}
            />
          </WorkspaceTableViewport>
        </S.TableCard>
      ) : null}

      {/* ---- CARD VIEW ---- */}
      {!loading && !error && filteredRows.length > 0 && viewMode === "cards" ? (
        <S.WorkspaceCardGrid>
          {filteredRows.map((ws) => {
            const agentCount = connectedAgentsByWorkspace[workspaceRowKey(ws)] ?? 0;
            const hasAgents = agentCount > 0;
            return (
              <S.WorkspaceCard
                key={ws.id}
                onMouseEnter={() => onPreviewWorkspace?.(ws)}
                onFocus={() => onPreviewWorkspace?.(ws)}
                onClick={() => onOpenWorkspace(ws)}
              >
                <S.CardTopRow>
                  <S.CardIconBox>
                    <FoldersIcon size="XL" />
                  </S.CardIconBox>
                  <div style={{ display: "flex", flexDirection: "column", gap: 2, minWidth: 0 }}>
                    <S.CardName>{ws.name}</S.CardName>
                    <S.CardDescription>
                      {ws.cloudAccount} · {ws.region || "Local"}
                    </S.CardDescription>
                  </div>
                </S.CardTopRow>

                <S.CardBody>

                  <S.CardDetailLines>
                    <S.CardDetailLine>
                      <S.CardDetailLabel>Database</S.CardDetailLabel>
                      <S.CardDetailValue>{ws.databaseName}</S.CardDetailValue>
                    </S.CardDetailLine>
                    <S.CardDetailLine>
                      <S.CardDetailLabel>ID</S.CardDetailLabel>
                      <S.CardDetailValue>
                        {ws.id.length > 22 ? `${ws.id.slice(0, 22)}…` : ws.id}
                      </S.CardDetailValue>
                    </S.CardDetailLine>
                  </S.CardDetailLines>

                  <S.CardStatsRow>
                    <S.CardStatBox>
                      <S.CardStatLabel>Files</S.CardStatLabel>
                      <S.CardStatValue>{ws.fileCount}</S.CardStatValue>
                    </S.CardStatBox>
                    <S.CardStatBox>
                      <S.CardStatLabel>Folders</S.CardStatLabel>
                      <S.CardStatValue>{ws.folderCount}</S.CardStatValue>
                    </S.CardStatBox>
                    <S.CardStatBox>
                      <S.CardStatLabel>Size</S.CardStatLabel>
                      <S.CardStatValue>{formatBytes(ws.totalBytes)}</S.CardStatValue>
                    </S.CardStatBox>
                  </S.CardStatsRow>

                  <S.CardInfoRow>
                    <S.CardInfoBox $highlight={hasAgents}>
                      <LiveDot $active={hasAgents} />
                      {agentCount} Agent{agentCount !== 1 ? "s" : ""}
                    </S.CardInfoBox>
                    <S.CardInfoBox>
                      {new Date(ws.updatedAt).toLocaleDateString()}
                    </S.CardInfoBox>
                  </S.CardInfoRow>
                </S.CardBody>

                <S.CardButtonRow>
                  <S.CardPrimaryButton
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation();
                      onOpenWorkspaceTab?.(ws, "browse");
                    }}
                  >
                    Browse Files
                  </S.CardPrimaryButton>
                  <S.CardSecondaryButton
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation();
                      onOpenWorkspaceTab?.(ws, "checkpoints");
                    }}
                  >
                    Checkpoints
                  </S.CardSecondaryButton>
                </S.CardButtonRow>
              </S.WorkspaceCard>
            );
          })}
        </S.WorkspaceCardGrid>
      ) : null}
    </>
  );
}

const WorkspaceTableViewport = styled(S.DenseTableViewport)`
  /* Reveal copy button on row hover */
  tbody tr:hover button[aria-label^="Copy workspace ID"] {
    opacity: 0.7;
  }
  tbody tr:hover button[aria-label^="Copy workspace ID"]:hover {
    opacity: 1;
  }
`;

const SizeCell = styled.span`
  font-size: 13px;
  color: var(--afs-ink, #18181b);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  strong {
    font-weight: 700;
  }
`;

const DetailsMuted = styled.span`
  color: var(--afs-muted, #71717a);
`;

const DetailsStack = styled.div`
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
`;

const DetailsRow = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-size: 12.5px;
  line-height: 1.3;
`;

const DetailsLabel = styled.span`
  color: var(--afs-muted, #71717a);
  font-weight: 700;
  font-size: 11px;
  letter-spacing: 0.02em;
  flex-shrink: 0;
`;

const DetailsValue = styled.span`
  color: var(--afs-ink, #18181b);
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
`;

const DetailsMono = styled.span`
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
  font-size: 11.5px;
  color: var(--afs-ink-soft, #3f3f46);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
`;

const CopyButton = styled.button`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--afs-muted, #71717a);
  cursor: pointer;
  border-radius: 4px;
  transition: background 140ms ease, color 140ms ease;
  opacity: 0;

  &:hover {
    background: rgba(8, 6, 13, 0.06);
    color: var(--afs-ink, #18181b);
  }

  &:focus-visible {
    outline: 2px solid var(--afs-accent, #dc2626);
    outline-offset: 1px;
    opacity: 1;
  }
`;

function CopyIcon() {
  return (
    <svg
      width="11"
      height="11"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </svg>
  );
}

function CheckIcon() {
  return (
    <svg
      width="11"
      height="11"
      viewBox="0 0 24 24"
      fill="none"
      stroke="#16a34a"
      strokeWidth="3"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
}

const UpdatedStack = styled.div`
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
`;

const UpdatedDate = styled.span`
  font-size: 13px;
  font-weight: 700;
  color: var(--afs-ink, #18181b);
  font-variant-numeric: tabular-nums;
  line-height: 1.2;
  white-space: nowrap;
`;

const UpdatedAgo = styled.span`
  font-size: 11.5px;
  color: var(--afs-muted, #71717a);
  line-height: 1.2;
  white-space: nowrap;
`;

const pulse = keyframes`
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
`;

const LiveDot = styled.span<{ $active: boolean }>`
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
