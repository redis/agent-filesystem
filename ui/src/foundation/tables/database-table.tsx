import { Menu, Typography } from "@redis-ui/components";
import { MoreactionsIcon } from "@redis-ui/icons/monochrome";
import { Table } from "@redis-ui/table";
import type { ColumnDef } from "@redis-ui/table";
import { useMemo, useState } from "react";
import styled, { css, keyframes } from "styled-components";
import type { AFSDatabaseScopeRecord } from "../database-scope";
import * as S from "./workspace-table.styles";

type Props = {
  rows: AFSDatabaseScopeRecord[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  onEditDatabase: (databaseId: string) => void;
  onRemoveDatabase: (databaseId: string) => void;
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
  onEditDatabase,
  onRemoveDatabase,
  toolbarAction,
}: Props & { toolbarAction?: React.ReactNode }) {
  const [search, setSearch] = useState("");

  const filteredRows = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (query === "") return rows;
    return rows.filter((row) =>
      [
        row.displayName ?? "",
        row.databaseName ?? "",
        row.description ?? "",
        row.endpointLabel ?? "",
      ].some((value) => value.toLowerCase().includes(query)),
    );
  }, [rows, search]);

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
              <S.WorkspaceNameButton
                onClick={(event) => {
                  event.stopPropagation();
                  onEditDatabase(row.original.id);
                }}
              >
                {row.original.displayName || row.original.databaseName}
              </S.WorkspaceNameButton>
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
              <LiveDot $active={row.original.isHealthy} />
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
          id: "actions",
          header: "Actions",
          size: 72,
          maxSize: 72,
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
    [onEditDatabase, onRemoveDatabase],
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
          <ClickableTableViewport>
            <Table
              columns={columns}
              data={filteredRows}
              getRowId={(row) => row.id}
              stripedRows
              onRowClick={(rowData) => onEditDatabase(rowData.id)}
            />
          </ClickableTableViewport>
        </S.TableCard>
      ) : null}
    </>
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

const pulse = keyframes`
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
`;

const LiveDot = styled.span<{ $active: boolean }>`
  display: inline-block;
  width: 8px;
  height: 8px;
  margin-top: 5px;
  border-radius: 50%;
  flex-shrink: 0;
  background: ${({ $active }) => ($active ? "#22c55e" : "#dc2626")};
  ${({ $active }) =>
    $active &&
    css`
      box-shadow: 0 0 6px rgba(34, 197, 94, 0.5);
      animation: ${pulse} 2s ease-in-out infinite;
    `}
  ${({ $active }) =>
    !$active &&
    css`
      box-shadow: 0 0 6px rgba(220, 38, 38, 0.5);
    `}
`;

const ClickableTableViewport = styled(S.TableViewport)`
  tbody tr {
    cursor: pointer;
  }
`;
