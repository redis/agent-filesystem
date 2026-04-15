import { Typography } from "@redis-ui/components";
import { Table } from "@redis-ui/table";
import type { ColumnDef, SortingState } from "@redis-ui/table";
import { useMemo, useState } from "react";
import type { AFSActivityEvent } from "../types/afs";
import * as S from "./workspace-table.styles";

type ActivitySortField = "createdAt" | "workspaceName" | "title" | "scope" | "actor";

type Props = {
  rows: AFSActivityEvent[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  onOpenActivity: (event: AFSActivityEvent) => void;
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

export function ActivityTable({
  rows,
  loading = false,
  error = false,
  errorMessage = "Unable to load activity. Please retry.",
  onOpenActivity,
}: Props) {
  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] = useState<ActivitySortField>("createdAt");
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("desc");

  const filteredRows = useMemo(() => {
    const query = search.trim().toLowerCase();
    const baseRows =
      query === ""
        ? rows
        : rows.filter((row) =>
            [
              row.workspaceName ?? "",
              row.workspaceId ?? "",
              row.title,
              row.detail,
              row.actor,
              row.scope,
              row.kind,
            ].some((value) => value.toLowerCase().includes(query)),
          );

    return [...baseRows].sort((left, right) => {
      const leftValue =
        sortBy === "workspaceName" ? left.workspaceName ?? left.workspaceId ?? "" : left[sortBy];
      const rightValue =
        sortBy === "workspaceName"
          ? right.workspaceName ?? right.workspaceId ?? ""
          : right[sortBy];

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
          accessorKey: "createdAt",
          header: "When",
          size: 56,
          enableSorting: true,
          cell: ({ row }) => {
            const d = new Date(row.original.createdAt);
            const now = new Date();
            const isToday = d.toDateString() === now.toDateString();
            const time = d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
            if (isToday) return time;
            const date = d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
            return `${date} ${time}`;
          },
        },
        {
          accessorKey: "workspaceName",
          header: "Workspace",
          size: 80,
          enableSorting: true,
          cell: ({ row }) => (
            <S.SingleLineText title={row.original.workspaceName ?? row.original.workspaceId ?? "Global"}>
              {row.original.workspaceName ?? row.original.workspaceId ?? "Global"}
            </S.SingleLineText>
          ),
        },
        {
          accessorKey: "title",
          header: "Activity",
          size: 160,
          enableSorting: true,
          cell: ({ row }) => (
            <S.Stack>
              <Typography.Body component="strong">{row.original.title}</Typography.Body>
              <Typography.Body color="secondary" component="span">
                {row.original.detail}
              </Typography.Body>
            </S.Stack>
          ),
        },
        {
          accessorKey: "actor",
          header: "Actor",
          size: 60,
          enableSorting: true,
        },
        {
          accessorKey: "scope",
          header: "Type",
          size: 50,
          enableSorting: true,
          cell: ({ row }) => (
            <S.SingleLineText title={`${row.original.scope} · ${row.original.kind}`}>
              {row.original.scope} · {row.original.kind}
            </S.SingleLineText>
          ),
        },
      ] as ColumnDef<AFSActivityEvent>[],
    [],
  );

  return (
    <>
      <S.HeadingWrap style={{ padding: 0 }}>
        <S.SearchInput
          value={search}
          onChange={setSearch}
          placeholder="Search activity..."
        />
      </S.HeadingWrap>

      {loading ? <S.EmptyState>Loading activity...</S.EmptyState> : null}
      {error ? <S.EmptyState role="alert">{errorMessage}</S.EmptyState> : null}
      {!loading && !error && filteredRows.length === 0 ? (
        <S.EmptyState>No activity matches the current filter.</S.EmptyState>
      ) : null}

      {!loading && !error && filteredRows.length > 0 ? (
        <S.TableCard>
          <S.TableViewport>
            <Table
              columns={columns}
              data={filteredRows}
              sorting={sorting}
              manualSorting
              onSortingChange={(nextState) => {
                if (nextState.length === 0) {
                  setSortBy("createdAt");
                  setSortDirection("desc");
                  return;
                }

                const next = nextState[0];
                setSortBy(next.id as ActivitySortField);
                setSortDirection(next.desc ? "desc" : "asc");
              }}
              enableSorting
              stripedRows
              onRowClick={(rowData) => onOpenActivity(rowData)}
            />
          </S.TableViewport>
        </S.TableCard>
      ) : null}
    </>
  );
}
