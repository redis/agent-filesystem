import { Typography } from "@redis-ui/components";
import { Table } from "@redis-ui/table";
import type { ColumnDef, SortingState } from "@redis-ui/table";
import { useMemo, useState } from "react";
import type { AFSChangelogEntry } from "../types/afs";
import * as S from "./workspace-table.styles";

type ChangesSortField = "occurredAt" | "op" | "path" | "sessionId" | "deltaBytes";

type Props = {
  rows: AFSChangelogEntry[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
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

function shortHash(h?: string): string {
  if (!h) return "";
  if (h.length <= 10) return h;
  return h.slice(0, 8) + "…";
}

export function ChangesTable({
  rows,
  loading = false,
  error = false,
  errorMessage = "Unable to load changes. Please retry.",
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
        row.sessionId ?? "",
        row.label ?? "",
        row.user ?? "",
        row.op ?? "",
        row.source ?? "",
      ].some((value) => value.toLowerCase().includes(query));
    });

    return [...baseRows].sort((left, right) => {
      const leftValue = (left[sortBy] ?? "") as string | number;
      const rightValue = (right[sortBy] ?? "") as string | number;
      return compareValues(leftValue, rightValue, sortDirection);
    });
  }, [rows, search, opFilter, sortBy, sortDirection]);

  const sorting = useMemo<SortingState>(
    () => [{ id: sortBy, desc: sortDirection === "desc" }],
    [sortBy, sortDirection],
  );

  const columns = useMemo(
    () =>
      [
        {
          accessorKey: "occurredAt",
          header: "When",
          size: 80,
          enableSorting: true,
          cell: ({ row }) => {
            const iso = row.original.occurredAt;
            if (!iso) return "—";
            const d = new Date(iso);
            const now = new Date();
            const isToday = d.toDateString() === now.toDateString();
            const time = d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
            if (isToday) return time;
            const date = d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
            return `${date} ${time}`;
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
          header: "Session",
          size: 120,
          enableSorting: true,
          cell: ({ row }) => {
            const label = row.original.label?.trim();
            const sessionId = row.original.sessionId ?? "";
            const display = label || sessionId.slice(0, 8) || "—";
            const tooltip = [label, sessionId].filter(Boolean).join(" · ");
            return (
              <S.SingleLineText title={tooltip || display}>
                {display}
              </S.SingleLineText>
            );
          },
        },
        {
          accessorKey: "source",
          header: "Source",
          size: 90,
          enableSorting: true,
          cell: ({ row }) => (
            <S.SingleLineText title={row.original.source ?? ""}>
              {row.original.source ?? "—"}
            </S.SingleLineText>
          ),
        },
        {
          accessorKey: "contentHash",
          header: "Hash",
          size: 90,
          enableSorting: false,
          cell: ({ row }) => (
            <Typography.Body component="span" color="secondary" style={{ fontFamily: "monospace", fontSize: 12 }}>
              {shortHash(row.original.contentHash)}
            </Typography.Body>
          ),
        },
      ] as ColumnDef<AFSChangelogEntry>[],
    [],
  );

  return (
    <>
      <S.HeadingWrap style={{ padding: 0, gap: 12, display: "flex", flexWrap: "wrap", alignItems: "center" }}>
        <S.SearchInput
          value={search}
          onChange={setSearch}
          placeholder="Search by path, session, user..."
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
        <S.EmptyState>No changes have been recorded for this workspace yet.</S.EmptyState>
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
    </>
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
  return (
    <select
      value={value}
      onChange={(event) => onChange(event.target.value)}
      style={{
        padding: "6px 10px",
        borderRadius: 8,
        border: "1px solid var(--afs-line, #e4e4e7)",
        background: "var(--afs-panel)",
        color: "var(--afs-ink, #18181b)",
        fontSize: 13,
        fontWeight: 600,
      }}
    >
      <option value="all">All ops</option>
      {ops.map((op) => (
        <option key={op} value={op}>
          {op}
        </option>
      ))}
    </select>
  );
}
