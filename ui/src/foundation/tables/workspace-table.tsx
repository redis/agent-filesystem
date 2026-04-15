import { Menu, Typography } from "@redis-ui/components";
import { MoreactionsIcon } from "@redis-ui/icons/monochrome";
import { Table } from "@redis-ui/table";
import type { ColumnDef, SortingState } from "@redis-ui/table";
import { useMemo, useState, type ReactNode } from "react";
import styled, { css, keyframes } from "styled-components";
import { formatBytes } from "../api/afs";
import type { AFSWorkspaceSummary } from "../types/afs";
import * as S from "./workspace-table.styles";

type WorkspaceSortField =
  | "name"
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
  onEditWorkspace,
  onDeleteWorkspace,
  deletingWorkspaceKey = null,
}: Props) {
  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] = useState<WorkspaceSortField>("updatedAt");
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("desc");

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
            <S.Stack>
              <S.WorkspaceNameButton
                onClick={(event) => {
                  event.stopPropagation();
                  onOpenWorkspace(row.original);
                }}
              >
                {row.original.name}
              </S.WorkspaceNameButton>
              <SecondaryLine color="secondary" component="span">
                {row.original.redisKey}
              </SecondaryLine>
            </S.Stack>
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
              <SecondaryLine color="secondary" component="span">
                {row.original.cloudAccount} · {row.original.region}
              </SecondaryLine>
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

      {!loading && !error && filteredRows.length > 0 ? (
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
    </>
  );
}

const SecondaryLine = styled(Typography.Body)`
  && {
    font-size: 12px;
    line-height: 1.4;
  }
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
