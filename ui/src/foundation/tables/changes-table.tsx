import { Select, Typography } from "@redis-ui/components";
import { Table } from "@redis-ui/table";
import type { ColumnDef, SortingState } from "@redis-ui/table";
import { useMemo, useState } from "react";
import styled from "styled-components";
import { shortDateTime } from "../time-format";
import type { AFSChangelogEntry } from "../types/afs";
import { truncateMiddlePath } from "./changes-table-utils";
import * as S from "./workspace-table.styles";

type ChangesSortField = "occurredAt" | "op" | "path" | "sessionId" | "deltaBytes";

export type HistoryTableRow = Omit<AFSChangelogEntry, "path"> & {
  path?: string;
  actor?: string;
  eventDetail?: string;
  eventTitle?: string;
  historyType?: "file" | "event";
  hostname?: string;
  kind?: string;
};

type Props = {
  rows: HistoryTableRow[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  emptyStateText?: string;
  detailHeader?: string;
  filterAllLabel?: string;
  loadingText?: string;
  searchPlaceholder?: string;
  onOpenChange?: (entry: HistoryTableRow) => void;
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

function formatSignedBytes(n?: number): string {
  if (n === undefined || n === 0) return "—";
  const sign = n > 0 ? "+" : "−";
  const abs = Math.abs(n);
  if (abs < 1024) return `${sign}${abs} B`;
  if (abs < 1024 * 1024) return `${sign}${(abs / 1024).toFixed(1)} KB`;
  if (abs < 1024 * 1024 * 1024) return `${sign}${(abs / (1024 * 1024)).toFixed(1)} MB`;
  return `${sign}${(abs / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function changeOperationLabel(row: Pick<HistoryTableRow, "historyType" | "kind" | "op" | "prevHash" | "prevPath">): string {
  if (row.historyType === "event" && row.kind !== "file") {
    return lifecycleOperationLabel(row.kind, row.op);
  }

  const hasPrevious = Boolean(row.prevHash?.trim() || row.prevPath?.trim());
  switch (row.op) {
    case "put":
      return hasPrevious ? "Update" : "Create";
    case "delete":
      return "Delete";
    case "mkdir":
      return "Create folder";
    case "rmdir":
      return "Delete folder";
    case "symlink":
      return hasPrevious ? "Update link" : "Create link";
    case "chmod":
      return "Change mode";
    default:
      return row.op || "Unknown";
  }
}

function lifecycleOperationLabel(kind?: string, op?: string): string {
  switch (`${kind ?? ""}.${op ?? ""}`) {
    case "checkpoint.save":
      return "Checkpoint created";
    case "checkpoint.restore":
      return "Checkpoint restored";
    case "workspace.create":
      return "Workspace created";
    case "workspace.import":
      return "Workspace imported";
    case "workspace.fork":
      return "Workspace forked";
    case "workspace.update":
      return "Workspace updated";
    case "session.start":
      return "Session started";
    case "session.close":
      return "Session closed";
    case "session.stale":
      return "Session stale";
    case "process.start":
      return "Process started";
    case "process.exit":
      return "Process exited";
    default:
      return [formatToken(kind), formatToken(op)].filter(Boolean).join(" ") || "Event";
  }
}

export function ChangesTable({
  rows,
  loading = false,
  error = false,
  errorMessage = "Unable to load changes. Please retry.",
  emptyStateText = "No changes have been recorded for this workspace yet.",
  detailHeader = "Path",
  filterAllLabel = "All changes",
  loadingText = "Loading changes...",
  searchPlaceholder = "Search by path, agent, user...",
  onOpenChange,
}: Props) {
  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] = useState<ChangesSortField>("occurredAt");
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("desc");
  const [opFilter, setOpFilter] = useState<string>("all");

  const operations = useMemo(() => {
    const set = new Set<string>();
    for (const row of rows) {
      set.add(changeOperationLabel(row));
    }
    return Array.from(set).sort();
  }, [rows]);

  const filteredRows = useMemo(() => {
    const query = search.trim().toLowerCase();
    const baseRows = rows.filter((row) => {
      const operation = changeOperationLabel(row);
      if (opFilter !== "all" && operation !== opFilter) return false;
      if (query === "") return true;
      return [
        row.path ?? "",
        row.prevPath ?? "",
        row.versionId ?? "",
        row.fileId ?? "",
        row.workspaceName ?? "",
        row.databaseName ?? "",
        row.agentId ?? "",
        row.sessionId ?? "",
        row.label ?? "",
        row.user ?? "",
        row.actor ?? "",
        row.checkpointId ?? "",
        row.eventTitle ?? "",
        row.eventDetail ?? "",
        row.hostname ?? "",
        row.kind ?? "",
        operation,
        row.source ?? "",
      ].some((value) => value.toLowerCase().includes(query));
    });

    return [...baseRows].sort((left, right) => {
      const leftValue =
        sortBy === "sessionId"
          ? (left.label ?? left.agentId ?? left.sessionId ?? "")
          : sortBy === "op"
            ? changeOperationLabel(left)
          : (left[sortBy] ?? "");
      const rightValue =
        sortBy === "sessionId"
          ? (right.label ?? right.agentId ?? right.sessionId ?? "")
          : sortBy === "op"
            ? changeOperationLabel(right)
          : (right[sortBy] ?? "");
      return compareValues(leftValue, rightValue, sortDirection);
    });
  }, [rows, search, opFilter, sortBy, sortDirection]);

  const sorting = useMemo<SortingState>(
    () => [{ id: sortBy, desc: sortDirection === "desc" }],
    [sortBy, sortDirection],
  );

  const columns = useMemo(
    () => {
      const showWorkspaceContext = rows.some(
        (row) => Boolean(row.workspaceName?.trim()) || Boolean(row.databaseName?.trim()),
      );

      return [
        {
          accessorKey: "occurredAt",
          header: "When",
          size: 80,
          enableSorting: true,
          cell: ({ row }) => {
            const iso = row.original.occurredAt;
            if (!iso) return "—";
            return shortDateTime(iso);
          },
        },
        {
          accessorKey: "op",
          header: "Change",
          size: 120,
          enableSorting: true,
          cell: ({ row }) => (
            <Typography.Body component="strong">{changeOperationLabel(row.original)}</Typography.Body>
          ),
        },
        {
          accessorKey: "path",
          header: detailHeader,
          size: 280,
          minSize: 220,
          maxSize: 320,
          enableSorting: true,
          cell: ({ row }) => <HistoryDetailCell row={row.original} />,
        },
        {
          id: "version",
          header: "Version",
          size: 120,
          enableSorting: false,
          cell: ({ row }) => (
            row.original.versionId ? (
              <S.Stack>
                <S.SingleLineText title={row.original.versionId}>
                  {row.original.versionId.slice(0, 12)}
                </S.SingleLineText>
                {row.original.fileId ? (
                  <Typography.Body color="secondary" component="span">
                    {row.original.fileId.slice(0, 12)}
                  </Typography.Body>
                ) : null}
              </S.Stack>
            ) : (
              <Typography.Body color="secondary" component="span">—</Typography.Body>
            )
          ),
        },
        ...(showWorkspaceContext
          ? [
              {
                id: "workspace",
                header: "Workspace",
                size: 170,
                enableSorting: false,
                cell: ({ row }) => (
                  <S.Stack>
                    <S.SingleLineText title={row.original.workspaceName ?? row.original.workspaceId ?? ""}>
                      {row.original.workspaceName ?? row.original.workspaceId ?? "—"}
                    </S.SingleLineText>
                    <Typography.Body color="secondary" component="span">
                      {row.original.databaseName ?? row.original.databaseId ?? "—"}
                    </Typography.Body>
                  </S.Stack>
                ),
              },
            ]
          : []),
        {
          accessorKey: "deltaBytes",
          header: "Delta",
          size: 80,
          enableSorting: true,
          cell: ({ row }) => {
            const delta = row.original.deltaBytes ?? 0;
            const color = delta > 0 ? "#16a34a" : delta < 0 ? "#dc2626" : undefined;
            return (
              <Typography.Body component="span" style={color ? { color } : undefined}>
                {formatSignedBytes(row.original.deltaBytes)}
              </Typography.Body>
            );
          },
        },
        {
          accessorKey: "sessionId",
          header: "Agent",
          size: 120,
          enableSorting: true,
          cell: ({ row }) => {
            const label = row.original.label?.trim();
            const agentId = row.original.agentId?.trim();
            const sessionId = row.original.sessionId ?? "";
            const actor = row.original.actor?.trim();
            const display = label || actor || agentId || sessionId.slice(0, 8) || "—";
            const tooltip = [label, actor, agentId, sessionId].filter(Boolean).join(" · ");
            return (
              <S.SingleLineText title={tooltip || display}>
                {display}
              </S.SingleLineText>
            );
          },
        },
      ] as ColumnDef<HistoryTableRow>[];
    },
    [detailHeader, rows],
  );

  return (
    <S.TableBlock>
      <S.HeadingWrap style={{ padding: 0, gap: 12, display: "flex", flexWrap: "wrap", alignItems: "center" }}>
        <S.SearchInput
          value={search}
          onChange={setSearch}
          placeholder={searchPlaceholder}
        />
        {operations.length > 1 ? (
          <OpFilter
            value={opFilter}
            ops={operations}
            allLabel={filterAllLabel}
            onChange={setOpFilter}
          />
        ) : null}
      </S.HeadingWrap>

      {loading ? <S.EmptyState>{loadingText}</S.EmptyState> : null}
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
              getRowId={(row) =>
                `${row.databaseId ?? "all"}:${row.workspaceId ?? "unknown"}:${row.id}`
              }
              sorting={sorting}
              manualSorting
              onRowClick={onOpenChange}
              onSortingChange={(nextState) => {
                if (nextState.length === 0) {
                  setSortBy("occurredAt");
                  setSortDirection("desc");
                  return;
                }
                const next = nextState[0];
                setSortBy(next.id as ChangesSortField);
                setSortDirection(next.desc ? "desc" : "asc");
              }}
              enableSorting
              stripedRows
            />
          </S.DenseTableViewport>
        </S.TableCard>
      ) : null}
    </S.TableBlock>
  );
}

function OpFilter({
  value,
  ops,
  allLabel,
  onChange,
}: {
  value: string;
  ops: string[];
  allLabel: string;
  onChange: (next: string) => void;
}) {
  const options = useMemo(
    () => [
      { value: "all", label: allLabel },
      ...ops.map((op) => ({ value: op, label: op })),
    ],
    [allLabel, ops],
  );

  return (
    <div style={{ minWidth: 160 }}>
      <Select
        options={options}
        value={value}
        onChange={(next) => onChange(next)}
        placeholder={allLabel}
      />
    </div>
  );
}

function HistoryDetailCell({ row }: { row: HistoryTableRow }) {
  if (row.historyType === "event" && row.kind !== "file") {
    const detail = lifecycleDetail(row);
    const secondary = lifecycleSecondary(row, detail);
    return (
      <HistoryDetailStack>
        <HistoryDetailText title={detail}>{detail}</HistoryDetailText>
        {secondary ? (
          <HistoryDetailSecondary title={secondary}>
            {secondary}
          </HistoryDetailSecondary>
        ) : null}
      </HistoryDetailStack>
    );
  }

  const path = row.path ?? "";
  const displayPath = path ? truncateMiddlePath(path) : "—";
  const previousPath = row.prevPath ?? "";
  const displayPreviousPath = previousPath ? truncateMiddlePath(previousPath) : "";

  return (
    <HistoryDetailStack>
      <HistoryDetailText title={path}>{displayPath}</HistoryDetailText>
      {previousPath ? (
        <HistoryDetailSecondary title={previousPath}>
          from {displayPreviousPath}
        </HistoryDetailSecondary>
      ) : null}
    </HistoryDetailStack>
  );
}

function lifecycleDetail(row: HistoryTableRow): string {
  return row.eventTitle
    || row.eventDetail
    || checkpointLabel(row.checkpointId)
    || sessionLabel(row.sessionId)
    || row.hostname
    || row.source
    || "Workspace event";
}

function lifecycleSecondary(row: HistoryTableRow, detail: string): string {
  return [
    row.eventTitle && row.eventDetail ? row.eventDetail : "",
    checkpointLabel(row.checkpointId),
    sessionLabel(row.sessionId),
    row.hostname,
    row.source,
  ]
    .filter((part): part is string => Boolean(part && part.trim() !== "" && part !== detail))
    .join(" · ");
}

function checkpointLabel(value?: string): string {
  if (!value) return "";
  return `checkpoint ${shortID(value)}`;
}

function sessionLabel(value?: string): string {
  if (!value) return "";
  return `session ${shortID(value)}`;
}

function shortID(value: string): string {
  return value.length <= 12 ? value : value.slice(0, 12);
}

function formatToken(value?: string) {
  return (value ?? "")
    .split(/[-_.]/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

const HistoryDetailStack = styled(S.Stack)`
  width: clamp(220px, 28vw, 320px);
  max-width: 320px;
  min-width: 0;
`;

const HistoryDetailText = styled(S.SingleLineText)`
  max-width: 100%;
`;

const HistoryDetailSecondary = styled.span`
  display: block;
  min-width: 0;
  max-width: 100%;
  overflow: hidden;
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.45;
  text-overflow: ellipsis;
  white-space: nowrap;
`;
