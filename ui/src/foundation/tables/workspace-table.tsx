import { Menu, Typography } from "@redis-ui/components";
import { FoldersIcon, MoreactionsIcon } from "@redis-ui/icons/monochrome";
import { Table } from "@redis-ui/table";
import type { ColumnDef, SortingState } from "@redis-ui/table";
import { useMemo, useState } from "react";
import type { ReactNode } from "react";
import styled, { css, keyframes } from "styled-components";
import { formatBytes } from "../api/afs";
import { useStoredViewMode } from "../hooks/use-stored-view-mode";
import type { AFSWorkspaceSummary } from "../types/afs";
import type { StudioTab } from "../workspace-tabs";
import * as S from "./workspace-table.styles";

type WorkspaceSortField =
  | "name"
  | "id"
  | "cloudAccount"
  | "connectedAgents"
  | "databaseName"
  | "totalBytes"
  | "updatedAt";

type RowWorkspaceSortField = Exclude<WorkspaceSortField, "connectedAgents">;

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
          header: "Workspace name",
          size: 110,
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
          accessorKey: "id",
          header: "Workspace ID",
          size: 120,
          enableSorting: true,
          cell: ({ row }) => (
            <TruncatedId title={row.original.id}>
              {row.original.id.length > 16
                ? `${row.original.id.slice(0, 16)}…`
                : row.original.id}
            </TruncatedId>
          ),
        },
        {
          id: "connectedAgents",
          header: "Agents",
          size: 52,
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
          size: 60,
          enableSorting: true,
          cell: ({ row }) => (
            <Typography.Body component="span">
              {formatBytes(row.original.totalBytes)} ({row.original.fileCount} files)
            </Typography.Body>
          ),
        },
        {
          accessorKey: "databaseName",
          header: "Database hosting",
          size: 110,
          enableSorting: true,
          cell: ({ row }) => (
            <S.Stack>
              <Typography.Body component="strong">{row.original.databaseName}</Typography.Body>
              {row.original.region ? (
                <SecondaryLine color="secondary" component="span">
                  {row.original.region}
                </SecondaryLine>
              ) : null}
            </S.Stack>
          ),
        },
        {
          accessorKey: "updatedAt",
          header: "Last updated",
          size: 120,
          enableSorting: true,
          cell: ({ row }) => new Date(row.original.updatedAt).toLocaleString(),
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
    [connectedAgentsByWorkspace, deletingWorkspaceKey, onDeleteWorkspace, onEditWorkspace, onOpenWorkspace],
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
          <S.RegistryTableViewport>
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
          </S.RegistryTableViewport>
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

const SecondaryLine = styled(Typography.Body)`
  && {
    font-size: 12px;
    line-height: 1.4;
  }
`;

const TruncatedId = styled.span`
  font-size: 12px;
  line-height: 1.4;
  color: var(--afs-muted);
  cursor: default;
  font-family: var(--afs-mono);
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
