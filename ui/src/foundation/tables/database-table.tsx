import { Menu } from "@redis-ui/components";
import { MoreactionsIcon } from "@redis-ui/icons/monochrome";
import { Table } from "@redis-ui/table";
import type { ColumnDef } from "@redis-ui/table";
import { useMemo, useState } from "react";
import styled, { css, keyframes } from "styled-components";
import type { AFSDatabaseScopeRecord } from "../database-scope";
import { formatBytes } from "../api/afs";
import * as S from "./workspace-table.styles";
import { DenseTableViewport } from "./workspace-table.styles";

type Props = {
  rows: AFSDatabaseScopeRecord[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  onEditDatabase: (databaseId: string) => void;
  onSetDefaultDatabase: (databaseId: string) => void;
  onRemoveDatabase: (databaseId: string) => void;
};

/* ------------------------------------------------------------------ */
/*  Small helpers                                                      */
/* ------------------------------------------------------------------ */

function formatOps(value: number): string {
  if (value >= 1000) {
    const k = value / 1000;
    return `${k >= 10 ? k.toFixed(0) : k.toFixed(1)}k`;
  }
  return `${value}`;
}

function formatRate(value: number): string {
  return `${Math.round(value * 100)}%`;
}

function shortenId(id: string): string {
  if (id.length <= 12) return id;
  return `${id.slice(0, 12)}…`;
}

type UsageTier = "ok" | "warn" | "critical";

function tierFor(usageFraction: number): UsageTier {
  if (usageFraction >= 0.9) return "critical";
  if (usageFraction >= 0.7) return "warn";
  return "ok";
}

/* ------------------------------------------------------------------ */
/*  Summary strip (4 cards above the table)                            */
/* ------------------------------------------------------------------ */

export function DatabaseSummaryStrip({ rows }: { rows: AFSDatabaseScopeRecord[] }) {
  const metrics = useMemo(() => {
    let healthy = 0;
    let totalAfsBytes = 0;
    let totalWorkspaces = 0;
    let capacitySum = 0;
    let capacityCount = 0;
    let atRisk = 0;
    let firstAtRisk: string | null = null;

    for (const row of rows) {
      if (row.isHealthy) healthy += 1;
      totalAfsBytes += row.afsTotalBytes;
      totalWorkspaces += row.workspaceCount;

      const stats = row.stats;
      if (stats && stats.maxMemoryBytes > 0) {
        const frac = stats.usedMemoryBytes / stats.maxMemoryBytes;
        capacitySum += frac;
        capacityCount += 1;

        if (frac >= 0.8) {
          atRisk += 1;
          if (firstAtRisk == null) firstAtRisk = row.displayName;
        }
      }

      if (!row.isHealthy) {
        atRisk += 1;
        if (firstAtRisk == null) firstAtRisk = row.displayName;
      }
    }

    const avgPct = capacityCount === 0 ? null : Math.round((capacitySum / capacityCount) * 100);

    return {
      total: rows.length,
      healthy,
      totalAfsBytes,
      totalWorkspaces,
      avgPct,
      atRisk,
      firstAtRisk,
    };
  }, [rows]);

  if (rows.length === 0) return null;

  return (
    <SummaryGrid>
      <SummaryCard>
        <SummaryLabel>Databases</SummaryLabel>
        <SummaryValue>{metrics.total}</SummaryValue>
        <SummaryDetail>
          {metrics.healthy} healthy{metrics.total !== metrics.healthy ? `, ${metrics.total - metrics.healthy} unavailable` : ""}
        </SummaryDetail>
      </SummaryCard>

      <SummaryCard>
        <SummaryLabel>Total Stored</SummaryLabel>
        <SummaryValue>{formatBytes(metrics.totalAfsBytes)}</SummaryValue>
        <SummaryDetail>
          {metrics.totalWorkspaces} workspace{metrics.totalWorkspaces === 1 ? "" : "s"}
        </SummaryDetail>
      </SummaryCard>

      <SummaryCard>
        <SummaryLabel>Capacity Used</SummaryLabel>
        <SummaryValue>{metrics.avgPct == null ? "—" : `${metrics.avgPct}%`}</SummaryValue>
        <SummaryDetail>
          {metrics.avgPct == null
            ? "No memory limits configured"
            : "Average across databases with a maxmemory limit"}
        </SummaryDetail>
      </SummaryCard>

      <SummaryCard $alert={metrics.atRisk > 0}>
        <SummaryLabel>At Risk</SummaryLabel>
        <SummaryValue>{metrics.atRisk}</SummaryValue>
        <SummaryDetail>
          {metrics.atRisk === 0
            ? "All databases healthy and below 80% capacity"
            : metrics.firstAtRisk
              ? `e.g. ${metrics.firstAtRisk}`
              : ""}
        </SummaryDetail>
      </SummaryCard>
    </SummaryGrid>
  );
}

/* ------------------------------------------------------------------ */
/*  Table                                                              */
/* ------------------------------------------------------------------ */

export function DatabaseTable({
  rows,
  loading = false,
  error = false,
  errorMessage = "Unable to load databases. Please retry.",
  onEditDatabase,
  onSetDefaultDatabase,
  onRemoveDatabase,
  toolbarAction,
}: Props & { toolbarAction?: React.ReactNode }) {
  const [search, setSearch] = useState("");
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const filteredRows = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (query === "") return rows;
    return rows.filter((row) =>
      [
        row.displayName ?? "",
        row.databaseName ?? "",
        row.description ?? "",
        row.endpointLabel ?? "",
        row.id ?? "",
      ].some((value) => value.toLowerCase().includes(query)),
    );
  }, [rows, search]);

  async function copyDatabaseId(id: string) {
    try {
      await navigator.clipboard.writeText(id);
      setCopiedId(id);
      window.setTimeout(() => {
        setCopiedId((current) => (current === id ? null : current));
      }, 1500);
    } catch {
      /* ignore clipboard failures */
    }
  }

  const columns = useMemo(
    () =>
      [
        /* ── Name column: dot + name + star + ID (with copy) ── */
        {
          accessorKey: "displayName",
          header: "Name",
          size: 240,
          enableSorting: false,
          cell: ({ row }) => {
            const isDefault = !!row.original.isDefault;
            const nameLabel = row.original.displayName || row.original.databaseName;
            const id = row.original.id;
            return (
              <NameStack>
                <NameLine>
                  <LiveDot
                    $active={row.original.isHealthy}
                    title={
                      row.original.isHealthy
                        ? "Connected"
                        : row.original.connectionError || "Unavailable"
                    }
                    aria-label={row.original.isHealthy ? "Connected" : "Unavailable"}
                  />
                  <NameButton
                    onClick={(event) => {
                      event.stopPropagation();
                      onEditDatabase(row.original.id);
                    }}
                  >
                    {nameLabel}
                  </NameButton>
                  <DefaultStarButton
                    type="button"
                    data-default-star
                    $filled={isDefault}
                    aria-label={
                      isDefault
                        ? `${nameLabel} is the default database`
                        : `Set ${nameLabel} as the default database`
                    }
                    title={
                      isDefault
                        ? "Default database for new workspaces"
                        : "Set as default database for new workspaces"
                    }
                    disabled={isDefault}
                    onClick={(event) => {
                      event.stopPropagation();
                      if (!isDefault) onSetDefaultDatabase(row.original.id);
                    }}
                  >
                    <StarIcon filled={isDefault} />
                  </DefaultStarButton>
                </NameLine>

                <IdRow>
                  <IdText title={id}>{shortenId(id)}</IdText>
                  <CopyButton
                    type="button"
                    aria-label={`Copy database ID ${id}`}
                    title={copiedId === id ? "Copied" : "Copy database ID"}
                    onClick={(event) => {
                      event.stopPropagation();
                      void copyDatabaseId(id);
                    }}
                  >
                    {copiedId === id ? <CheckIcon /> : <CopyIcon />}
                  </CopyButton>
                </IdRow>
              </NameStack>
            );
          },
        },

        /* ── Usage column: bar + used/max + workspaces summary ── */
        {
          id: "usage",
          header: "Usage",
          size: 220,
          enableSorting: false,
          cell: ({ row }) => {
            const stats = row.original.stats;
            const maxBytes = stats?.maxMemoryBytes ?? 0;
            const usedBytes = stats?.usedMemoryBytes ?? 0;
            const hasLimit = maxBytes > 0;
            const frac = hasLimit ? Math.min(1, usedBytes / maxBytes) : 0;
            const tier = hasLimit ? tierFor(frac) : "ok";
            const pct = Math.round(frac * 100);

            const workspaceSummary = (
              <UsageSubline>
                {row.original.workspaceCount} workspace{row.original.workspaceCount === 1 ? "" : "s"}
                {" · "}
                {formatBytes(row.original.afsTotalBytes)}
                {" · "}
                {row.original.afsFileCount.toLocaleString()} file{row.original.afsFileCount === 1 ? "" : "s"}
              </UsageSubline>
            );

            if (stats == null) {
              return (
                <UsageStack>
                  <UsageLine>
                    <UsageText>Awaiting sample…</UsageText>
                  </UsageLine>
                  {workspaceSummary}
                </UsageStack>
              );
            }

            return (
              <UsageStack>
                <UsageLine>
                  <UsageBarOuter>
                    <UsageBarInner
                      $pct={pct}
                      $tier={tier}
                      aria-valuemin={0}
                      aria-valuemax={100}
                      aria-valuenow={pct}
                      role="progressbar"
                    />
                  </UsageBarOuter>
                  <UsageText>
                    {hasLimit ? (
                      <>
                        <strong>{formatBytes(usedBytes)}</strong>
                        <Muted> / {formatBytes(maxBytes)}</Muted>
                        <PctPill $tier={tier}>{pct}%</PctPill>
                      </>
                    ) : (
                      <>
                        <strong>{formatBytes(usedBytes)}</strong>
                        <Muted> · no limit</Muted>
                      </>
                    )}
                  </UsageText>
                </UsageLine>
                {workspaceSummary}
              </UsageStack>
            );
          },
        },

        /* ── Load column: ops/sec + hit + clients/keys ── */
        {
          id: "load",
          header: "Load",
          size: 160,
          enableSorting: false,
          cell: ({ row }) => {
            const stats = row.original.stats;
            if (stats == null) {
              return <DimCell>—</DimCell>;
            }
            return (
              <LoadStack>
                <LoadLine>
                  <strong>{formatOps(stats.opsPerSec)}</strong>
                  <Muted> ops/s</Muted>
                  {stats.cacheHitRate > 0 ? (
                    <>
                      <Sep>·</Sep>
                      <strong>{formatRate(stats.cacheHitRate)}</strong>
                      <Muted> hit</Muted>
                    </>
                  ) : null}
                </LoadLine>
                <LoadSubline>
                  {stats.connectedClients} client{stats.connectedClients === 1 ? "" : "s"}
                  {" · "}
                  {stats.keyCount.toLocaleString()} key{stats.keyCount === 1 ? "" : "s"}
                </LoadSubline>
              </LoadStack>
            );
          },
        },

        /* ── Actions ── */
        {
          id: "actions",
          header: "",
          size: 40,
          maxSize: 40,
          enableSorting: false,
          cell: ({ row }) => (
            <Menu>
              <Menu.Trigger withButton={false}>
                <S.MoreActionsTrigger
                  aria-label={`More actions for ${row.original.displayName || row.original.databaseName}`}
                  onClick={(event) => {
                    event.stopPropagation();
                  }}
                >
                  <MoreactionsIcon size="S" />
                </S.MoreActionsTrigger>
              </Menu.Trigger>
              <Menu.Content align="end" onClick={(e: React.MouseEvent) => e.stopPropagation()}>
                <Menu.Content.Item
                  text={row.original.isDefault ? "Current default" : "Set as default"}
                  disabled={row.original.isDefault}
                  onClick={() => onSetDefaultDatabase(row.original.id)}
                />
                <Menu.Content.Item
                  text="Edit database"
                  onClick={() => onEditDatabase(row.original.id)}
                />
                <Menu.Content.Item
                  text="Delete database"
                  onClick={() => onRemoveDatabase(row.original.id)}
                />
              </Menu.Content>
            </Menu>
          ),
        },
      ] as ColumnDef<AFSDatabaseScopeRecord>[],
    [copiedId, onEditDatabase, onRemoveDatabase, onSetDefaultDatabase],
  );

  return (
    <>
      <S.HeadingWrap style={{ padding: 0 }}>
        <S.SearchInput
          value={search}
          onChange={setSearch}
          placeholder="Search databases..."
        />
        {toolbarAction ?? null}
      </S.HeadingWrap>

      {loading ? <S.EmptyState>Loading databases...</S.EmptyState> : null}
      {error ? <S.EmptyState role="alert">{errorMessage}</S.EmptyState> : null}
      {!loading && !error && filteredRows.length === 0 ? (
        <S.EmptyState>
          {rows.length === 0
            ? "No databases have been configured yet."
            : "No databases match the current filter."}
        </S.EmptyState>
      ) : null}

      {!loading && !error && filteredRows.length > 0 ? (
        <S.TableCard>
          <DatabaseTableViewport>
            <Table
              columns={columns}
              data={filteredRows}
              getRowId={(row) => row.id}
              stripedRows
              onRowClick={(rowData) => onEditDatabase(rowData.id)}
            />
          </DatabaseTableViewport>
        </S.TableCard>
      ) : null}
    </>
  );
}

/* ================================================================== */
/*  Styled pieces                                                      */
/* ================================================================== */

const DEFAULT_AMBER = "#f59e0b";

/* ---- Name cell ---- */

const NameStack = styled.div`
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
`;

const NameLine = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
`;

const NameButton = styled.button`
  border: none;
  background: transparent;
  padding: 0;
  font: inherit;
  font-size: 14px;
  font-weight: 700;
  color: var(--afs-ink, #18181b);
  cursor: pointer;
  text-align: left;
  line-height: 1.2;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  &:hover {
    color: var(--afs-accent, #dc2626);
  }
`;

/* Glowing status dot, inline before the name */
const pulse = keyframes`
  0%, 100% { opacity: 1; }
  50%      { opacity: 0.45; }
`;

const LiveDot = styled.span<{ $active: boolean }>`
  flex-shrink: 0;
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: ${({ $active }) => ($active ? "#22c55e" : "#dc2626")};
  ${({ $active }) =>
    $active
      ? css`
          box-shadow: 0 0 6px rgba(34, 197, 94, 0.55);
          animation: ${pulse} 2s ease-in-out infinite;
        `
      : css`
          box-shadow: 0 0 6px rgba(220, 38, 38, 0.55);
        `}
`;

/* ---- Default star ---- */

const DefaultStarButton = styled.button<{ $filled: boolean }>`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  padding: 0;
  border: none;
  background: transparent;
  flex-shrink: 0;
  cursor: ${({ $filled }) => ($filled ? "default" : "pointer")};
  color: ${({ $filled }) => ($filled ? DEFAULT_AMBER : "var(--afs-muted, #71717a)")};
  opacity: ${({ $filled }) => ($filled ? 1 : 0)};
  transition: opacity 140ms ease, color 140ms ease, transform 140ms ease;

  &:hover:not(:disabled) {
    color: ${DEFAULT_AMBER};
    transform: scale(1.1);
  }

  &:disabled {
    cursor: default;
  }

  &:focus-visible {
    outline: 2px solid ${DEFAULT_AMBER};
    outline-offset: 2px;
    border-radius: 4px;
    opacity: 1;
  }
`;

function StarIcon({ filled }: { filled: boolean }) {
  return (
    <svg
      width="13"
      height="13"
      viewBox="0 0 24 24"
      fill={filled ? "currentColor" : "none"}
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
    </svg>
  );
}

/* ---- Database ID row ---- */

const IdRow = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding-left: 16px; /* align under the name, past the status dot */
`;

const IdText = styled.span`
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
  font-size: 11px;
  color: var(--afs-muted, #71717a);
  letter-spacing: 0;
  line-height: 1.2;
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

/* ---- Usage cell ---- */

const UsageStack = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
`;

const UsageLine = styled.div`
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
`;

const UsageBarOuter = styled.div`
  flex: 0 0 88px;
  height: 6px;
  background: rgba(8, 6, 13, 0.07);
  border-radius: 999px;
  overflow: hidden;
`;

const tierColor = (tier: UsageTier): string => {
  if (tier === "critical") return "#dc2626";
  if (tier === "warn") return "#f59e0b";
  return "#22c55e";
};

const tierSoft = (tier: UsageTier): string => {
  if (tier === "critical") return "rgba(220, 38, 38, 0.12)";
  if (tier === "warn") return "rgba(245, 158, 11, 0.15)";
  return "rgba(34, 197, 94, 0.14)";
};

const UsageBarInner = styled.div<{ $pct: number; $tier: UsageTier }>`
  height: 100%;
  width: ${({ $pct }) => `${$pct}%`};
  background: ${({ $tier }) => tierColor($tier)};
  border-radius: 999px;
  transition: width 400ms ease, background 200ms ease;
`;

const UsageText = styled.span`
  font-size: 12.5px;
  color: var(--afs-ink, #18181b);
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  display: inline-flex;
  align-items: center;
  gap: 6px;

  strong {
    font-weight: 700;
  }
`;

const Muted = styled.span`
  color: var(--afs-muted, #71717a);
`;

const PctPill = styled.span<{ $tier: UsageTier }>`
  display: inline-flex;
  align-items: center;
  padding: 0 6px;
  font-size: 10.5px;
  font-weight: 700;
  line-height: 16px;
  border-radius: 999px;
  background: ${({ $tier }) => tierSoft($tier)};
  color: ${({ $tier }) => tierColor($tier)};
  font-variant-numeric: tabular-nums;
`;

const UsageSubline = styled.span`
  font-size: 11.5px;
  color: var(--afs-muted, #71717a);
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
`;

/* ---- Load cell ---- */

const LoadStack = styled.div`
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
`;

const LoadLine = styled.span`
  font-size: 13px;
  color: var(--afs-ink, #18181b);
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  display: inline-flex;
  align-items: center;
  gap: 4px;

  strong {
    font-weight: 700;
  }
`;

const LoadSubline = styled.span`
  font-size: 11.5px;
  color: var(--afs-muted, #71717a);
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
`;

const Sep = styled.span`
  color: var(--afs-muted, #71717a);
  padding: 0 2px;
`;

const DimCell = styled.span`
  color: var(--afs-muted, #71717a);
  font-size: 13px;
`;

/* ---- Summary strip ---- */

const SummaryGrid = styled.div`
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 16px;

  @media (max-width: 900px) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  @media (max-width: 540px) {
    grid-template-columns: 1fr;
  }
`;

const SummaryCard = styled.div<{ $alert?: boolean }>`
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 14px 16px;
  border: 1px solid
    ${({ $alert }) => ($alert ? "rgba(220, 38, 38, 0.35)" : "var(--afs-line, #e4e4e7)")};
  border-radius: 12px;
  background: ${({ $alert }) => ($alert ? "rgba(220, 38, 38, 0.04)" : "var(--afs-panel-strong, #fff)")};
`;

const SummaryLabel = styled.span`
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--afs-muted, #71717a);
`;

const SummaryValue = styled.span`
  font-size: 24px;
  font-weight: 800;
  color: var(--afs-ink, #18181b);
  line-height: 1.1;
  letter-spacing: -0.02em;
  font-variant-numeric: tabular-nums;
`;

const SummaryDetail = styled.span`
  font-size: 12px;
  color: var(--afs-muted, #71717a);
  line-height: 1.35;
`;

/* ---- Table viewport: dense + database-specific hover reveals ---- */

const DatabaseTableViewport = styled(DenseTableViewport)`
  /* Reveal star + copy button on row hover */
  tbody tr:hover [data-default-star]:not(:disabled) {
    opacity: 0.55;
  }
  tbody tr:hover [data-default-star]:not(:disabled):hover {
    opacity: 1;
  }
  tbody tr:hover button[aria-label^="Copy database ID"] {
    opacity: 0.7;
  }
  tbody tr:hover button[aria-label^="Copy database ID"]:hover {
    opacity: 1;
  }
`;
