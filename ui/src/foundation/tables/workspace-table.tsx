import { Menu, Typography } from "@redislabsdev/redis-ui-components";
import { MoreactionsIcon } from "@redislabsdev/redis-ui-icons/monochrome";
import { Table } from "@redislabsdev/redis-ui-table";
import type { ColumnDef, SortingState } from "@redislabsdev/redis-ui-table";
import { useMemo, useState, type ReactNode } from "react";
import { formatBytes } from "../api/afs";
import type { AFSWorkspaceSummary } from "../types/afs";
import * as S from "./workspace-table.styles";

type WorkspaceSortField =
  | "name"
  | "cloudAccount"
  | "databaseName"
  | "fileCount"
  | "totalBytes"
  | "checkpointCount"
  | "updatedAt";

type Props = {
  rows: AFSWorkspaceSummary[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  toolbarAction?: ReactNode;
  onOpenWorkspace: (workspaceId: string) => void;
  onEditWorkspace: (workspaceId: string) => void;
  onDeleteWorkspace: (workspaceId: string) => void;
  deletingWorkspaceId?: string | null;
};

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
  onOpenWorkspace,
  onEditWorkspace,
  onDeleteWorkspace,
  deletingWorkspaceId = null,
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
      const leftValue = left[sortBy];
      const rightValue = right[sortBy];
      return compareValues(leftValue, rightValue, sortDirection);
    });
  }, [rows, search, sortBy, sortDirection]);

  const sorting = useMemo<SortingState>(
    () => [{ id: sortBy, desc: sortDirection === "desc" }],
    [sortBy, sortDirection],
  );
  const isFiltering = search.trim() !== "";

  const columns = useMemo(
    () =>
      [
        {
          id: "health",
          accessorKey: "status",
          header: "",
          size: 20,
          minSize: 20,
          maxSize: 20,
          enableSorting: false,
          cell: ({ row }) => (
            <S.HealthCell>
              <S.HealthDot
                $active={row.original.status !== "attention"}
                $syncing={row.original.status === "syncing"}
                aria-label={`health-${row.original.status}`}
              />
            </S.HealthCell>
          ),
        },
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
                  onOpenWorkspace(row.original.id);
                }}
              >
                {row.original.name}
              </S.WorkspaceNameButton>
              <Typography.Body color="secondary" component="span">
                {row.original.redisKey}
              </Typography.Body>
            </S.Stack>
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
              <Typography.Body color="secondary" component="span">
                {row.original.cloudAccount} · {row.original.region}
              </Typography.Body>
            </S.Stack>
          ),
        },
        {
          accessorKey: "fileCount",
          header: "Files/Folder",
          size: 70,
          enableSorting: true,
          cell: ({ row }) => (
            <Typography.Body component="span">
              {row.original.fileCount}/{row.original.folderCount}
            </Typography.Body>
          ),
        },
        {
          accessorKey: "totalBytes",
          header: "Size",
          size: 50,
          enableSorting: true,
          cell: ({ row }) => formatBytes(row.original.totalBytes),
        },
        {
          accessorKey: "checkpointCount",
          header: "Checkpoints",
          size: 65,
          enableSorting: true,
          cell: ({ row }) => row.original.checkpointCount,
        },
        {
          accessorKey: "updatedAt",
          header: "Last updated",
          size: 65,
          enableSorting: true,
          cell: ({ row }) => new Date(row.original.updatedAt).toLocaleString(),
        },
        {
          id: "actions",
          header: "",
          size: 10,
          maxSize: 10,
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
              <Menu.Content align="end">
                <Menu.Content.Item
                  text="Open workspace"
                  onClick={() => onOpenWorkspace(row.original.id)}
                />
                <Menu.Content.Item
                  text="Edit workspace"
                  onClick={() => onEditWorkspace(row.original.id)}
                />
                <Menu.Content.Item
                  text={deletingWorkspaceId === row.original.id ? "Deleting..." : "Delete workspace"}
                  onClick={() => {
                    if (deletingWorkspaceId === row.original.id) {
                      return;
                    }
                    onDeleteWorkspace(row.original.id);
                  }}
                />
              </Menu.Content>
            </Menu>
          ),
        },
      ] as ColumnDef<AFSWorkspaceSummary>[],
    [deletingWorkspaceId, onDeleteWorkspace, onEditWorkspace, onOpenWorkspace],
  );

  return (
    <S.TableCard>
      <S.HeadingWrap>
        <S.SearchInput
          value={search}
          onChange={(event) => setSearch(event.target.value)}
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
            onRowClick={(rowData) => onOpenWorkspace(rowData.id)}
          />
        </S.RegistryTableViewport>
      ) : null}
    </S.TableCard>
  );
}
