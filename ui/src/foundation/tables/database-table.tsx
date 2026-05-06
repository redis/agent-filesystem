import { Button, Menu } from "@redis-ui/components";
import { MoreactionsIcon } from "@redis-ui/icons/monochrome";
import { Table } from "@redis-ui/table";
import { DatabaseIcon } from "../../components/lucide-icons";
import type { ColumnDef } from "@redis-ui/table";
import { useMemo, useState } from "react";
import styled from "styled-components";
import {
  DialogActions,
  DialogBody,
  DialogCard,
  DialogCloseButton,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
} from "../../components/afs-kit";
import { SurfaceCard } from "../../components/card-shell";
import type { AFSDatabaseScopeRecord } from "../database-scope";
import { formatBytes } from "../api/afs";
import { CheckIcon, CopyIcon } from "../clipboard-icons";
import { redisInsightUrl } from "../redis-insight";
import * as S from "./workspace-table.styles";
import { DenseTableViewport } from "./workspace-table.styles";
import { StatusNameCell, StatusNameLine } from "./status-name-cell";

type Props = {
  rows: AFSDatabaseScopeRecord[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  onEditDatabase: (databaseId: string) => void;
  onSetDefaultDatabase: (databaseId: string) => void;
  onRemoveDatabase: (databaseId: string) => void;
};

type CapabilityHelp = {
  capability: "arrays" | "search";
  databaseName: string;
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

function formatUsedMegabytes(value: number): string {
  const mb = value / (1024 * 1024);
  const digits = mb === 0 || mb >= 10 ? 0 : 1;
  return `${mb.toLocaleString(undefined, {
    maximumFractionDigits: digits,
    minimumFractionDigits: digits,
  })} MB`;
}

function arrayCapabilityLabel(supportsArrays: boolean | undefined) {
  if (supportsArrays === true) return "Arrays Enabled";
  if (supportsArrays === false) return "Arrays Coming soon";
  return "Arrays unknown";
}

function searchCapabilityLabel(supportsSearch: boolean | undefined) {
  if (supportsSearch === true) return "Search Enabled";
  if (supportsSearch === false) return "Search Unavailable";
  return "Search unknown";
}

/* ------------------------------------------------------------------ */
/*  Summary strip (4 cards above the table)                            */
/* ------------------------------------------------------------------ */

export function DatabaseSummaryStrip({
  rows,
}: {
  rows: AFSDatabaseScopeRecord[];
}) {
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

    const avgPct =
      capacityCount === 0
        ? null
        : Math.round((capacitySum / capacityCount) * 100);

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
          {metrics.healthy} healthy
          {metrics.total !== metrics.healthy
            ? `, ${metrics.total - metrics.healthy} unavailable`
            : ""}
        </SummaryDetail>
      </SummaryCard>

      <SummaryCard>
        <SummaryLabel>Total Stored</SummaryLabel>
        <SummaryValue>{formatBytes(metrics.totalAfsBytes)}</SummaryValue>
        <SummaryDetail>
          {metrics.totalWorkspaces} workspace
          {metrics.totalWorkspaces === 1 ? "" : "s"}
        </SummaryDetail>
      </SummaryCard>

      <SummaryCard>
        <SummaryLabel>Capacity Used</SummaryLabel>
        <SummaryValue>
          {metrics.avgPct == null ? "—" : `${metrics.avgPct}%`}
        </SummaryValue>
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
  const [capabilityHelp, setCapabilityHelp] = useState<CapabilityHelp | null>(
    null,
  );

  const filteredRows = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (query === "") return rows;
    return rows.filter((row) =>
      [
        row.displayName,
        row.databaseName,
        row.description,
        row.endpointLabel,
        row.id,
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
            const nameLabel =
              row.original.displayName || row.original.databaseName;
            const id = row.original.id;
            const canEdit = row.original.canEdit;
            const canSetDefault = row.original.canCreateWorkspaces;
            return (
              <StatusNameCell
                active={row.original.isHealthy}
                icon={<DatabaseIcon customSize={18} />}
                inactiveTone="danger"
                statusLabel={
                  row.original.isHealthy ? "Connected" : "Unavailable"
                }
                statusTitle={
                  row.original.isHealthy
                    ? "Connected"
                    : row.original.connectionError || "Unavailable"
                }
              >
                <StatusNameLine>
                  <NameButton
                    disabled={!canEdit}
                    onClick={(event) => {
                      event.stopPropagation();
                      if (canEdit) {
                        onEditDatabase(row.original.id);
                      }
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
                      !canSetDefault
                        ? `${nameLabel} is reserved for onboarding`
                        : isDefault
                          ? "Default database for new workspaces"
                          : "Set as default database for new workspaces"
                    }
                    disabled={isDefault || !canSetDefault}
                    onClick={(event) => {
                      event.stopPropagation();
                      if (!isDefault && canSetDefault)
                        onSetDefaultDatabase(row.original.id);
                    }}
                  >
                    <StarIcon filled={isDefault} />
                  </DefaultStarButton>
                </StatusNameLine>

                <IdRow>
                  <IdText title={id}>{id}</IdText>
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
                {!canEdit ? (
                  <ManagedHint>
                    {row.original.purpose === "onboarding"
                      ? "Managed by AFS Cloud"
                      : row.original.ownerLabel?.trim()
                        ? `Managed by ${row.original.ownerLabel}`
                        : "Managed by AFS Cloud"}
                  </ManagedHint>
                ) : null}
              </StatusNameCell>
            );
          },
        },

        /* ── Usage column: workspace count + used MB ── */
        {
          id: "usage",
          header: "Usage",
          size: 220,
          enableSorting: false,
          cell: ({ row }) => {
            const usedBytes =
              row.original.stats?.usedMemoryBytes ?? row.original.afsTotalBytes;
            return (
              <UsageStack>
                <UsageText>
                  {row.original.workspaceCount} workspace
                  {row.original.workspaceCount === 1 ? "" : "s"}
                </UsageText>
                <UsageSubline>
                  {formatUsedMegabytes(usedBytes)} used
                </UsageSubline>
              </UsageStack>
            );
          },
        },

        /* ── Load column: ops/sec + clients/keys ── */
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
                </LoadLine>
                <LoadSubline>
                  {stats.connectedClients} client
                  {stats.connectedClients === 1 ? "" : "s"}
                  {" · "}
                  {stats.keyCount.toLocaleString()} key
                  {stats.keyCount === 1 ? "" : "s"}
                </LoadSubline>
              </LoadStack>
            );
          },
        },

        {
          id: "version",
          header: "Version",
          size: 130,
          enableSorting: false,
          cell: ({ row }) => {
            const version = row.original.stats?.redisVersion?.trim();
            return version ? (
              <VersionText>{version}</VersionText>
            ) : (
              <DimCell>—</DimCell>
            );
          },
        },

        {
          id: "capabilities",
          header: "Capabilities",
          size: 210,
          enableSorting: false,
          cell: ({ row }) => {
            const databaseName =
              row.original.displayName || row.original.databaseName;
            return (
              <CapabilityStack>
                <CapabilityLine>
                  <StorageCapabilityBadge
                    $supported={row.original.supportsArrays}
                  >
                    {arrayCapabilityLabel(row.original.supportsArrays)}
                  </StorageCapabilityBadge>
                  {row.original.supportsArrays === false ? (
                    <CapabilityHelpButton
                      type="button"
                      aria-label={`Learn about Redis Arrays for ${databaseName}`}
                      title="Why is this unavailable?"
                      onClick={(event) => {
                        event.stopPropagation();
                        setCapabilityHelp({
                          capability: "arrays",
                          databaseName,
                        });
                      }}
                    >
                      ?
                    </CapabilityHelpButton>
                  ) : null}
                </CapabilityLine>
                <CapabilityLine>
                  <StorageCapabilityBadge
                    $supported={row.original.supportsSearch}
                  >
                    {searchCapabilityLabel(row.original.supportsSearch)}
                  </StorageCapabilityBadge>
                  {row.original.supportsSearch === false ? (
                    <CapabilityHelpButton
                      type="button"
                      aria-label={`Learn about Redis Search for ${databaseName}`}
                      title="Why is this unavailable?"
                      onClick={(event) => {
                        event.stopPropagation();
                        setCapabilityHelp({
                          capability: "search",
                          databaseName,
                        });
                      }}
                    >
                      ?
                    </CapabilityHelpButton>
                  ) : null}
                </CapabilityLine>
              </CapabilityStack>
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
              <Menu.Content
                align="end"
                onClick={(e: React.MouseEvent) => e.stopPropagation()}
              >
                <Menu.Content.Item
                  text={
                    row.original.isDefault
                      ? "Current default"
                      : "Set as default"
                  }
                  disabled={
                    row.original.isDefault || !row.original.canCreateWorkspaces
                  }
                  onClick={() => onSetDefaultDatabase(row.original.id)}
                />
                <Menu.Content.Item
                  text="Edit database"
                  disabled={!row.original.canEdit}
                  onClick={() => onEditDatabase(row.original.id)}
                />
                <Menu.Content.Item
                  text="Launch Redis Insight"
                  onClick={() => {
                    window.open(redisInsightUrl(), "_blank", "noreferrer");
                  }}
                />
                <Menu.Content.Item
                  text="Delete database"
                  disabled={!row.original.canDelete}
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
    <S.TableBlock>
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
              onRowClick={(rowData) => {
                if (rowData.canEdit) {
                  onEditDatabase(rowData.id);
                }
              }}
            />
          </DatabaseTableViewport>
        </S.TableCard>
      ) : null}
      {capabilityHelp ? (
        <CapabilityHelpDialog
          help={capabilityHelp}
          onClose={() => setCapabilityHelp(null)}
        />
      ) : null}
    </S.TableBlock>
  );
}

function CapabilityHelpDialog({
  help,
  onClose,
}: {
  help: CapabilityHelp;
  onClose: () => void;
}) {
  const isArrays = help.capability === "arrays";
  return (
    <DialogOverlay
      role="presentation"
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <CapabilityDialogCard
        role="dialog"
        aria-modal="true"
        aria-labelledby="capability-help-title"
      >
        <DialogHeader>
          <div>
            <DialogTitle id="capability-help-title">
              {isArrays
                ? "Redis Arrays coming soon"
                : "Redis Search unavailable"}
            </DialogTitle>
            <DialogBody>
              {help.databaseName} does not currently expose this Redis
              capability.
            </DialogBody>
          </div>
          <DialogCloseButton aria-label="Close" onClick={onClose}>
            x
          </DialogCloseButton>
        </DialogHeader>

        <CapabilityHelpCopy>
          {isArrays
            ? "Redis Arrays are coming soon. They have not been accepted into the upstream Redis repository yet, so this capability will become available after Redis ships support and the database is upgraded."
            : "Redis Search is included in current Redis distributions that provide the query engine. Upgrade this database to the latest Redis release with Search support enabled, then refresh the database catalog."}
        </CapabilityHelpCopy>

        <DialogActions style={{ justifyContent: "flex-end", marginTop: 20 }}>
          <Button size="medium" onClick={onClose}>
            Close
          </Button>
        </DialogActions>
      </CapabilityDialogCard>
    </DialogOverlay>
  );
}

/* ================================================================== */
/*  Styled pieces                                                      */
/* ================================================================== */

const DEFAULT_AMBER = "#f59e0b";

/* ---- Name cell ---- */

const NameButton = styled.button`
  border: none;
  background: transparent;
  padding: 0;
  font: inherit;
  font-size: 14px;
  font-weight: 700;
  color: var(--afs-ink, #18181b);
  cursor: pointer;
  flex: 1 1 auto;
  min-width: 0;
  text-align: left;
  line-height: 1.2;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  &:hover {
    color: var(--afs-accent, #dc2626);
  }

  &:disabled {
    cursor: default;
    color: var(--afs-ink, #18181b);
  }
`;

const ManagedHint = styled.div`
  font-size: 12px;
  line-height: 1.3;
  color: var(--afs-muted-ink, #7c6f63);
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
  color: ${({ $filled }) =>
    $filled ? DEFAULT_AMBER : "var(--afs-muted, #71717a)"};
  opacity: ${({ $filled }) => ($filled ? 1 : 0)};
  transition:
    opacity 140ms ease,
    color 140ms ease,
    transform 140ms ease;

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
  display: flex;
  align-items: center;
  width: 100%;
  gap: 4px;
  min-width: 0;
`;

const IdText = styled.span`
  flex: 1 1 auto;
  font-family: var(
    --afs-mono,
    ui-monospace,
    SFMono-Regular,
    Menlo,
    Consolas,
    monospace
  );
  font-size: 11px;
  color: var(--afs-muted, #71717a);
  letter-spacing: 0;
  line-height: 1.2;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const CopyButton = styled.button`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  width: 16px;
  height: 16px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--afs-muted, #71717a);
  cursor: pointer;
  border-radius: 4px;
  transition:
    background 140ms ease,
    color 140ms ease;
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

/* ---- Usage cell ---- */

const UsageStack = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
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

const DimCell = styled.span`
  color: var(--afs-muted, #71717a);
  font-size: 13px;
`;

const VersionText = styled.code`
  display: inline-block;
  max-width: 120px;
  overflow: hidden;
  color: var(--afs-ink, #18181b);
  font-family: var(
    --afs-mono,
    ui-monospace,
    SFMono-Regular,
    Menlo,
    Consolas,
    monospace
  );
  font-size: 12px;
  line-height: 1.3;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

/* ---- Capabilities cell ---- */

const CapabilityStack = styled.div`
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
`;

const CapabilityLine = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
`;

const StorageCapabilityBadge = styled.span<{ $supported?: boolean }>`
  display: inline-flex;
  width: fit-content;
  align-items: center;
  border-radius: 999px;
  padding: 5px 10px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.01em;
  border: 1px solid
    ${({ $supported }) =>
      $supported === true
        ? "rgba(37, 99, 235, 0.24)"
        : $supported === false
          ? "rgba(220, 38, 38, 0.28)"
          : "var(--afs-line, #e4e4e7)"};
  background: ${({ $supported }) =>
    $supported === true
      ? "rgba(37, 99, 235, 0.08)"
      : $supported === false
        ? "rgba(220, 38, 38, 0.08)"
        : "var(--afs-panel, #fff)"};
  color: ${({ $supported }) =>
    $supported === true
      ? "#1d4ed8"
      : $supported === false
        ? "#b91c1c"
        : "var(--afs-ink-soft, #3f3f46)"};
`;

const CapabilityHelpButton = styled.button`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: 1px solid rgba(220, 38, 38, 0.32);
  border-radius: 50%;
  background: rgba(220, 38, 38, 0.06);
  color: #b91c1c;
  cursor: pointer;
  font-size: 11px;
  font-weight: 800;
  line-height: 1;

  &:hover,
  &:focus-visible {
    background: rgba(220, 38, 38, 0.12);
    outline: none;
  }
`;

const CapabilityDialogCard = styled(DialogCard)`
  max-width: 480px;
`;

const CapabilityHelpCopy = styled.p`
  margin: 0;
  color: var(--afs-ink-soft, #3f3f46);
  font-size: 14px;
  line-height: 1.6;
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

const SummaryCard = styled(SurfaceCard)<{ $alert?: boolean }>`
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 14px 16px;
  border: 1px solid
    ${({ $alert }) =>
      $alert ? "rgba(220, 38, 38, 0.35)" : "var(--afs-line, #e4e4e7)"};
  background: ${({ $alert }) =>
    $alert ? "rgba(220, 38, 38, 0.04)" : "var(--afs-panel-strong, #fff)"};
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
