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
  onSetDefaultDatabase: (databaseId: string) => void;
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
  onSetDefaultDatabase,
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
          cell: ({ row }) => {
            const isDefault = !!row.original.isDefault;
            const nameLabel = row.original.displayName || row.original.databaseName;
            return (
              <S.Stack>
                <NameLine>
                  <S.WorkspaceNameButton
                    onClick={(event) => {
                      event.stopPropagation();
                      onEditDatabase(row.original.id);
                    }}
                  >
                    {nameLabel}
                  </S.WorkspaceNameButton>
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
                {row.original.description ? (
                  <SecondaryLine color="secondary" component="span">
                    {row.original.description}
                  </SecondaryLine>
                ) : null}
              </S.Stack>
            );
          },
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
    [onEditDatabase, onRemoveDatabase, onSetDefaultDatabase],
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

const NameLine = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 6px;
`;

const DEFAULT_AMBER = "#f59e0b";

const DefaultStarButton = styled.button<{ $filled: boolean }>`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  padding: 0;
  border: none;
  background: transparent;
  cursor: ${({ $filled }) => ($filled ? "default" : "pointer")};
  color: ${({ $filled }) => ($filled ? DEFAULT_AMBER : "var(--afs-muted, #71717a)")};
  /* Filled star is always visible; outline star is hidden until row hover. */
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
      width="14"
      height="14"
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

  /* Reveal the outline "set-as-default" star on row hover */
  tbody tr:hover [data-default-star]:not(:disabled) {
    opacity: 0.55;
  }

  tbody tr:hover [data-default-star]:not(:disabled):hover {
    opacity: 1;
  }
`;
