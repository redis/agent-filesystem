import { Button, TableHeading, Typography } from "@redislabsdev/redis-ui-components";
import { Table } from "@redislabsdev/redis-ui-table";
import type { ColumnDef, SortingState } from "@redislabsdev/redis-ui-table";
import { useMemo, useState } from "react";
import { formatBytes } from "../api/raf";
import type { RAFWorkspaceSummary } from "../types/raf";
import { ToneChip } from "../../components/raf-kit";
import * as S from "./workspace-table.styles";

type WorkspaceSortField =
  | "name"
  | "databaseName"
  | "fileCount"
  | "totalBytes"
  | "sessionCount"
  | "checkpointCount"
  | "updatedAt";

type Props = {
  rows: RAFWorkspaceSummary[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  onOpenWorkspace: (workspaceId: string) => void;
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
  onOpenWorkspace,
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
            [row.name, row.databaseName, row.redisKey, row.region].some((value) =>
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

  const columns = useMemo(
    () =>
      [
        {
          accessorKey: "status",
          header: "Status",
          size: 24,
          cell: ({ row }) => <ToneChip $tone={row.original.status}>{row.original.status}</ToneChip>,
          enableSorting: false,
        },
        {
          accessorKey: "name",
          header: "Workspace",
          size: 110,
          enableSorting: true,
          cell: ({ row }) => (
            <S.Stack>
              <S.WorkspaceNameButton onClick={() => onOpenWorkspace(row.original.id)}>
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
          header: "Database",
          size: 90,
          enableSorting: true,
          cell: ({ row }) => (
            <S.Stack>
              <Typography.Body component="strong">{row.original.databaseName}</Typography.Body>
              <Typography.Body color="secondary" component="span">
                {row.original.region}
              </Typography.Body>
            </S.Stack>
          ),
        },
        {
          accessorKey: "fileCount",
          header: "Content",
          size: 70,
          enableSorting: true,
          cell: ({ row }) => (
            <S.Stack>
              <Typography.Body component="span">{row.original.folderCount} folders</Typography.Body>
              <Typography.Body color="secondary" component="span">
                {row.original.fileCount} files
              </Typography.Body>
            </S.Stack>
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
          accessorKey: "sessionCount",
          header: "Sessions",
          size: 60,
          enableSorting: true,
          cell: ({ row }) => (
            <S.Stack>
              <Typography.Body component="span">{row.original.sessionCount} sessions</Typography.Body>
              <Typography.Body color="secondary" component="span">
                {row.original.forkCount} forks · {row.original.dirtySessionCount} dirty
              </Typography.Body>
            </S.Stack>
          ),
        },
        {
          accessorKey: "checkpointCount",
          header: "Checkpoints",
          size: 65,
          enableSorting: true,
          cell: ({ row }) => (
            <S.Stack>
              <Typography.Body component="span">
                {row.original.checkpointCount} checkpoints
              </Typography.Body>
              <Typography.Body color="secondary" component="span">
                Last: {new Date(row.original.lastCheckpointAt).toLocaleDateString()}
              </Typography.Body>
            </S.Stack>
          ),
        },
        {
          accessorKey: "updatedAt",
          header: "Updated",
          size: 65,
          enableSorting: true,
          cell: ({ row }) => new Date(row.original.updatedAt).toLocaleString(),
        },
        {
          id: "actions",
          header: "",
          size: 42,
          enableSorting: false,
          cell: ({ row }) => (
            <Button size="medium" variant="secondary-fill" onClick={() => onOpenWorkspace(row.original.id)}>
              Open
            </Button>
          ),
        },
      ] as ColumnDef<RAFWorkspaceSummary>[],
    [onOpenWorkspace],
  );

  return (
    <S.TableCard>
      <S.HeadingWrap>
        <TableHeading>
          <TableHeading.Title>Agent Filesystems</TableHeading.Title>
        </TableHeading>
        <S.SearchInput
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="Search workspace, database, key, or region"
        />
      </S.HeadingWrap>

      {loading ? <S.EmptyState>Loading workspaces...</S.EmptyState> : null}
      {error ? <S.EmptyState role="alert">{errorMessage}</S.EmptyState> : null}
      {!loading && !error && filteredRows.length === 0 ? (
        <S.EmptyState>No workspaces match the current filter.</S.EmptyState>
      ) : null}

      {!loading && !error && filteredRows.length > 0 ? (
        <S.TableViewport>
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
        </S.TableViewport>
      ) : null}
    </S.TableCard>
  );
}
