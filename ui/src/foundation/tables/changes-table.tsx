import { Select, Typography } from "@redis-ui/components";
import { Table } from "@redis-ui/table";
import type { ColumnDef, SortingState } from "@redis-ui/table";
import { useMemo, useState } from "react";
import { shortDateTime } from "../time-format";
import type { AFSChangelogEntry } from "../types/afs";
import * as S from "./workspace-table.styles";

type ChangesSortField = "occurredAt" | "op" | "path" | "sessionId" | "deltaBytes";

type Props = {
  rows: AFSChangelogEntry[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  emptyStateText?: string;
  onOpenChange?: (entry: AFSChangelogEntry) => void;
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

export function ChangesTable({
  rows,
  loading = false,
  error = false,
  errorMessage = "Unable to load changes. Please retry.",
  emptyStateText = "No changes have been recorded for this workspace yet.",
  onOpenChange,
}: Props) {
  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] = useState<ChangesSortField>("occurredAt");
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("desc");
  const [opFilter, setOpFilter] = useState<string>("all");

  const ops = useMemo(() => {
    const set = new Set<string>();
    for (const row of rows) {
      if (row.op) set.add(row.op);
    }
    return Array.from(set).sort();
  }, [rows]);

  const filteredRows = useMemo(() => {
    const query = search.trim().toLowerCase();
    const baseRows = rows.filter((row) => {
      if (opFilter !== "all" && row.op !== opFilter) return false;
      if (query === "") return true;
      return [
        row.path,
        row.prevPath ?? "",
        row.workspaceName ?? "",
        row.databaseName ?? "",
        row.agentId ?? "",
        row.sessionId ?? "",
        row.label ?? "",
        row.user ?? "",
        row.op ?? "",
        row.source ?? "",
      ].some((value) => value.toLowerCase().includes(query));
    });

    return [...baseRows].sort((left, right) => {
      const leftValue =
        sortBy === "sessionId"
          ? (left.label ?? left.agentId ?? left.sessionId ?? "")
          : ((left[sortBy] ?? "") as string | number);
      const rightValue =
        sortBy === "sessionId"
          ? (right.label ?? right.agentId ?? right.sessionId ?? "")
          : ((right[sortBy] ?? "") as string | number);
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
          header: "Op",
          size: 60,
          enableSorting: true,
          cell: ({ row }) => (
            <Typography.Body component="strong">{row.original.op}</Typography.Body>
          ),
        },
        {
          accessorKey: "path",
          header: "Path",
          size: 240,
          enableSorting: true,
          cell: ({ row }) => (
            <S.Stack>
              <S.SingleLineText title={row.original.path}>
                {row.original.path}
              </S.SingleLineText>
              {row.original.prevPath ? (
                <Typography.Body color="secondary" component="span">
                  from {row.original.prevPath}
                </Typography.Body>
              ) : null}
            </S.Stack>
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
            const display = label || agentId || sessionId.slice(0, 8) || "—";
            const tooltip = [label, agentId, sessionId].filter(Boolean).join(" · ");
            return (
              <S.SingleLineText title={tooltip || display}>
                {display}
              </S.SingleLineText>
            );
          },
        },
      ] as ColumnDef<AFSChangelogEntry>[];
    },
    [rows],
  );

  return (
    <S.TableBlock>
      <S.HeadingWrap style={{ padding: 0, gap: 12, display: "flex", flexWrap: "wrap", alignItems: "center" }}>
        <S.SearchInput
          value={search}
          onChange={setSearch}
          placeholder="Search by path, agent, user..."
        />
        {ops.length > 1 ? (
          <OpFilter
            value={opFilter}
            ops={ops}
            onChange={setOpFilter}
          />
        ) : null}
      </S.HeadingWrap>

      {loading ? <S.EmptyState>Loading changes...</S.EmptyState> : null}
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
  onChange,
}: {
  value: string;
  ops: string[];
  onChange: (next: string) => void;
}) {
  const options = useMemo(
    () => [
      { value: "all", label: "All ops" },
      ...ops.map((op) => ({ value: op, label: op })),
    ],
    [ops],
  );

  return (
    <div style={{ minWidth: 160 }}>
      <Select
        options={options}
        value={value}
        onChange={(next) => onChange(next as string)}
        placeholder="All ops"
      />
    </div>
  );
}
