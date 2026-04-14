import { Typography } from "@redislabsdev/redis-ui-components";
import { Table } from "@redislabsdev/redis-ui-table";
import type { ColumnDef } from "@redislabsdev/redis-ui-table";
import { useMemo } from "react";
import styled from "styled-components";
import type { AFSDatabaseScopeRecord } from "../database-scope";
import * as S from "./workspace-table.styles";

type Props = {
  rows: AFSDatabaseScopeRecord[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  onOpenDatabase: (databaseId: string) => void;
};

function formatCatalogTimestamp(value?: string) {
  if (value == null || value.trim() === "") {
    return "Not yet recorded";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString();
}

export function DatabaseTable({
  rows,
  loading = false,
  error = false,
  errorMessage = "Unable to load databases. Please retry.",
  onOpenDatabase,
}: Props) {
  const columns = useMemo(
    () =>
      [
        {
          accessorKey: "displayName",
          header: "Name",
          size: 120,
          enableSorting: false,
          cell: ({ row }) => (
            <S.Stack>
              <PlainText>{row.original.displayName || row.original.databaseName}</PlainText>
              {row.original.description ? (
                <SecondaryLine color="secondary" component="span">
                  {row.original.description}
                </SecondaryLine>
              ) : null}
            </S.Stack>
          ),
        },
        {
          accessorKey: "isHealthy",
          header: "Status",
          size: 84,
          enableSorting: false,
          cell: ({ row }) => (
            <StatusCell>
              <StatusDot $healthy={row.original.isHealthy} />
              <S.Stack>
                <PlainText>{row.original.isHealthy ? "Connected" : "Unavailable"}</PlainText>
                {!row.original.isHealthy && row.original.connectionError ? (
                  <SecondaryLine color="secondary" component="span">
                    {row.original.connectionError}
                  </SecondaryLine>
                ) : null}
                <SecondaryLine color="secondary" component="span">
                  {formatCatalogTimestamp(row.original.lastWorkspaceRefreshAt)}
                </SecondaryLine>
                {row.original.lastWorkspaceRefreshError ? (
                  <SecondaryLine color="secondary" component="span">
                    {row.original.lastWorkspaceRefreshError}
                  </SecondaryLine>
                ) : null}
              </S.Stack>
            </StatusCell>
          ),
        },
        {
          accessorKey: "endpointLabel",
          header: "Endpoint",
          size: 120,
          enableSorting: false,
          cell: ({ row }) => (
            <S.Stack>
              <PlainText>{row.original.endpointLabel || "Not configured"}</PlainText>
              <SecondaryLine color="secondary" component="span">
                DB {row.original.dbIndex}{row.original.useTLS ? " · TLS" : ""}
              </SecondaryLine>
            </S.Stack>
          ),
        },
        {
          accessorKey: "workspaceCount",
          header: "Workspaces",
          size: 60,
          enableSorting: false,
        },
        {
          accessorKey: "activeSessionCount",
          header: "Sessions",
          size: 72,
          enableSorting: false,
          cell: ({ row }) => (
            <S.Stack>
              <PlainText>{row.original.activeSessionCount}</PlainText>
              <SecondaryLine color="secondary" component="span">
                Reconcile: {formatCatalogTimestamp(row.original.lastSessionReconcileAt)}
              </SecondaryLine>
              {row.original.lastSessionReconcileError ? (
                <SecondaryLine color="secondary" component="span">
                  {row.original.lastSessionReconcileError}
                </SecondaryLine>
              ) : null}
            </S.Stack>
          ),
        },
      ] as ColumnDef<AFSDatabaseScopeRecord>[],
    [],
  );

  return (
    <S.TableCard>
      {loading ? <S.EmptyState>Loading databases...</S.EmptyState> : null}
      {error ? <S.EmptyState role="alert">{errorMessage}</S.EmptyState> : null}
      {!loading && !error && rows.length === 0 ? (
        <S.EmptyState>No databases have been configured yet.</S.EmptyState>
      ) : null}

      {!loading && !error && rows.length > 0 ? (
        <ClickableTableViewport>
          <Table
            columns={columns}
            data={rows}
            getRowId={(row) => row.id}
            stripedRows
            onRowClick={(rowData) => onOpenDatabase(rowData.id)}
          />
        </ClickableTableViewport>
      ) : null}
    </S.TableCard>
  );
}

const PlainText = styled.span`
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 400;
`;

const SecondaryLine = styled(Typography.Body)`
  && {
    font-size: 12px;
    line-height: 1.4;
  }
`;

const StatusCell = styled.div`
  display: flex;
  align-items: flex-start;
  gap: 10px;
`;

const StatusDot = styled.span<{ $healthy: boolean }>`
  display: inline-flex;
  width: 10px;
  height: 10px;
  margin-top: 4px;
  border-radius: 999px;
  background: ${({ $healthy }) => ($healthy ? "#16a34a" : "#dc2626")};
  box-shadow: ${({ $healthy }) =>
    $healthy
      ? "0 0 0 4px rgba(34, 197, 94, 0.12)"
      : "0 0 0 4px rgba(239, 68, 68, 0.12)"};
  flex-shrink: 0;
`;

const ClickableTableViewport = styled(S.TableViewport)`
  tbody tr {
    cursor: pointer;
  }
`;
