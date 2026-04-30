import { Typography } from "@redis-ui/components";
import { Table } from "@redis-ui/table";
import type { ColumnDef, SortingState } from "@redis-ui/table";
import { useMemo, useState } from "react";
import type { AFSEventEntry } from "../types/afs";
import * as S from "./workspace-table.styles";

type EventSortField = "createdAt" | "workspaceName" | "kind" | "actor" | "path";

type Props = {
  rows: AFSEventEntry[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  emptyStateText?: string;
  hideWorkspaceColumn?: boolean;
  onOpenEvent: (event: AFSEventEntry) => void;
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

export function EventsTable({
  rows,
  loading = false,
  error = false,
  errorMessage = "Unable to load events. Please retry.",
  emptyStateText = "No events match the current filter.",
  hideWorkspaceColumn = false,
  onOpenEvent,
}: Props) {
  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] = useState<EventSortField>("createdAt");
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
              row.databaseName ?? "",
              row.databaseId ?? "",
              row.kind,
              row.op,
              row.source ?? "",
              eventActor(row),
              row.sessionId ?? "",
              row.hostname ?? "",
              row.path ?? "",
              row.prevPath ?? "",
              row.checkpointId ?? "",
            ].some((value) => value.toLowerCase().includes(query)),
          );

    return [...baseRows].sort((left, right) => {
      const leftValue = eventSortValue(left, sortBy);
      const rightValue = eventSortValue(right, sortBy);
      return compareValues(leftValue, rightValue, sortDirection);
    });
  }, [rows, search, sortBy, sortDirection]);

  const sorting = useMemo<SortingState>(
    () => [{ id: sortBy, desc: sortDirection === "desc" }],
    [sortBy, sortDirection],
  );

  const columns = useMemo<ColumnDef<AFSEventEntry>[]>(
    () => {
      const cols: ColumnDef<AFSEventEntry>[] = [
        {
          accessorKey: "createdAt",
          header: "When",
          size: 64,
          enableSorting: true,
          cell: ({ row }) => formatEventTime(row.original.createdAt),
        },
        {
          accessorKey: "kind",
          header: "Event",
          size: 150,
          enableSorting: true,
          cell: ({ row }) => (
            <S.Stack>
              <Typography.Body component="strong">{eventTitle(row.original)}</Typography.Body>
              <Typography.Body color="secondary" component="span">
                {eventDetail(row.original)}
              </Typography.Body>
            </S.Stack>
          ),
        },
        {
          accessorKey: "actor",
          header: "Actor",
          size: 70,
          enableSorting: true,
          cell: ({ row }) => (
            <S.SingleLineText title={eventActor(row.original)}>
              {eventActor(row.original)}
            </S.SingleLineText>
          ),
        },
        {
          accessorKey: "path",
          header: "Path",
          size: 120,
          enableSorting: true,
          cell: ({ row }) => (
            <S.SingleLineText title={eventPath(row.original)}>
              {eventPath(row.original)}
            </S.SingleLineText>
          ),
        },
      ];

      if (!hideWorkspaceColumn) {
        cols.splice(2, 0, {
          accessorKey: "workspaceName",
          header: "Workspace",
          size: 90,
          enableSorting: true,
          cell: ({ row }) => (
            <S.SingleLineText title={row.original.workspaceName ?? row.original.workspaceId ?? "Global"}>
              {row.original.workspaceName ?? row.original.workspaceId ?? "Global"}
            </S.SingleLineText>
          ),
        });
      }

      return cols;
    },
    [hideWorkspaceColumn],
  );

  return (
    <S.TableBlock>
      <S.HeadingWrap style={{ padding: 0 }}>
        <S.SearchInput
          value={search}
          onChange={setSearch}
          placeholder="Search events..."
        />
      </S.HeadingWrap>

      {loading ? <S.EmptyState>Loading events...</S.EmptyState> : null}
      {error ? <S.EmptyState role="alert">{errorMessage}</S.EmptyState> : null}
      {!loading && !error && filteredRows.length === 0 ? (
        <S.EmptyState>{emptyStateText}</S.EmptyState>
      ) : null}

      {!loading && !error && filteredRows.length > 0 ? (
        <S.TableCard>
          <S.DenseTableViewport>
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
                setSortBy(next.id as EventSortField);
                setSortDirection(next.desc ? "desc" : "asc");
              }}
              enableSorting
              stripedRows
              onRowClick={(rowData) => onOpenEvent(rowData)}
            />
          </S.DenseTableViewport>
        </S.TableCard>
      ) : null}
    </S.TableBlock>
  );
}

function eventSortValue(event: AFSEventEntry, field: EventSortField): string | number {
  switch (field) {
    case "createdAt":
      return Date.parse(event.createdAt ?? "") || 0;
    case "workspaceName":
      return event.workspaceName ?? event.workspaceId ?? "";
    case "actor":
      return eventActor(event);
    case "path":
      return eventPath(event);
    case "kind":
    default:
      return eventTitle(event);
  }
}

function eventActor(event: AFSEventEntry) {
  return event.actor || event.label || event.user || event.sessionId || "afs";
}

function eventPath(event: AFSEventEntry) {
  if (event.prevPath && event.path && event.prevPath !== event.path) {
    return `${event.prevPath} -> ${event.path}`;
  }
  return event.path || event.checkpointId || "";
}

function eventTitle(event: AFSEventEntry) {
  return `${formatToken(event.kind)} ${formatToken(event.op)}`.trim();
}

function eventDetail(event: AFSEventEntry) {
  const parts = [
    event.source,
    eventPath(event),
    event.deltaBytes == null ? "" : formatDelta(event.deltaBytes),
  ].filter((part): part is string => Boolean(part && part.trim() !== ""));
  return parts.length === 0 ? event.id : parts.join(" · ");
}

function formatToken(value: string) {
  return value
    .split(/[-_.]/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function formatDelta(value: number) {
  if (value === 0) return "0 B";
  const sign = value > 0 ? "+" : "-";
  const absolute = Math.abs(value);
  if (absolute < 1024) return `${sign}${absolute} B`;
  if (absolute < 1024 * 1024) return `${sign}${(absolute / 1024).toFixed(absolute < 10 * 1024 ? 1 : 0)} KB`;
  return `${sign}${(absolute / (1024 * 1024)).toFixed(1)} MB`;
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
