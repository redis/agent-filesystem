import { Button, Typography } from "@redis-ui/components";
import { Table } from "@redis-ui/table";
import type { ColumnDef, SortingState } from "@redis-ui/table";
import { useMemo, useState, type ReactNode } from "react";
import styled from "styled-components";
import {
  DialogBody,
  DialogCard,
  DialogCloseButton,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
} from "../../components/afs-kit";
import { PlugIcon } from "../../components/lucide-icons";
import { getControlPlaneURL } from "../api/afs";
import type { AFSMCPProfile, AFSMCPToken } from "../types/afs";
import * as S from "./workspace-table.styles";

type MCPSortField = "name" | "workspaceName" | "lastUsedAt" | "expiresAt" | "createdAt";

type Props = {
  rows: AFSMCPToken[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  workspaceNameById?: Map<string, string>;
  databaseNameById?: Map<string, string>;
  toolbarAction?: ReactNode;
  onRevoke: (token: AFSMCPToken) => void;
  revoking?: boolean;
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

function formatProfile(profile: AFSMCPProfile) {
  switch (profile) {
    case "workspace-ro":
      return "Read only";
    case "workspace-rw":
      return "Read / write";
    case "workspace-rw-checkpoint":
      return "Read / write + checkpoints";
    case "admin-ro":
      return "Admin read only";
    case "admin-rw":
      return "Admin read / write";
    default:
      return profile;
  }
}

function formatTimestamp(value?: string) {
  if (!value) return "Never";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function timeAgoOrDate(value?: string) {
  if (!value) return "Never";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
  if (seconds < 60) return "Just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 14) return `${days}d ago`;
  return date.toLocaleDateString();
}

function expiryLabel(value?: string) {
  if (!value) return "No expiry";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const delta = date.getTime() - Date.now();
  if (delta <= 0) return "Expired";
  const hours = Math.floor(delta / (1000 * 60 * 60));
  if (hours < 24) return `${hours}h left`;
  const days = Math.floor(hours / 24);
  return `${days}d left`;
}

/* ------------------------------------------------------------------ */
/*  Detail dialog                                                     */
/* ------------------------------------------------------------------ */

function MCPServerDetailDialog({
  token,
  workspaceNameById,
  databaseNameById,
  revoking,
  onClose,
  onRevoke,
}: {
  token: AFSMCPToken;
  workspaceNameById?: Map<string, string>;
  databaseNameById?: Map<string, string>;
  revoking?: boolean;
  onClose: () => void;
  onRevoke: (token: AFSMCPToken) => void;
}) {
  const [copied, setCopied] = useState(false);
  const [confirmingRevoke, setConfirmingRevoke] = useState(false);
  const workspaceName =
    token.workspaceName || workspaceNameById?.get(token.workspaceId) || token.workspaceId;
  const configSnippet = buildHostedMCPTemplate(workspaceName);

  function copy() {
    void navigator.clipboard.writeText(configSnippet).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <DialogOverlay
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <DialogCard>
        <DialogHeader>
          <div>
            <DialogTitle>{token.name?.trim() || "MCP server"}</DialogTitle>
            <DialogBody>
              Hosted MCP server connected to workspace <strong>{workspaceName}</strong>.
            </DialogBody>
          </div>
          <DialogCloseButton onClick={onClose}>&times;</DialogCloseButton>
        </DialogHeader>

        <DetailGrid>
          <DetailField>
            <DetailLabel>Token ID</DetailLabel>
            <DetailValue style={{ fontFamily: "var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace)", fontSize: 12 }}>
              {token.id}
            </DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Profile</DetailLabel>
            <DetailValue>{formatProfile(token.profile)}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Workspace</DetailLabel>
            <DetailValue>{workspaceName}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Database</DetailLabel>
            <DetailValue>
              {databaseNameById?.get(token.databaseId) || token.databaseId}
            </DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Created</DetailLabel>
            <DetailValue>{formatTimestamp(token.createdAt)}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Last used</DetailLabel>
            <DetailValue>{token.lastUsedAt ? formatTimestamp(token.lastUsedAt) : "Never"}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Expires</DetailLabel>
            <DetailValue>{token.expiresAt ? formatTimestamp(token.expiresAt) : "Never"}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Access</DetailLabel>
            <DetailValue>{token.readonly ? "Read only" : "Read / write"}</DetailValue>
          </DetailField>
        </DetailGrid>

        <ConfigBlock>
          <DetailLabel>MCP client config</DetailLabel>
          <ConfigHint>
            Paste this into your client's MCP config (e.g. <code>claude_desktop_config.json</code>),
            and replace <code>{"<YOUR_TOKEN>"}</code> with the bearer token you copied at creation.
          </ConfigHint>
          <CodeBlock>{configSnippet}</CodeBlock>
          <ConfigActions>
            <Button size="small" variant="secondary-fill" onClick={copy}>
              {copied ? "Copied!" : "Copy config"}
            </Button>
          </ConfigActions>
        </ConfigBlock>

        {confirmingRevoke ? (
          <ConfirmRevokePanel role="alertdialog" aria-live="polite">
            <ConfirmRevokeTitle>Remove this MCP server?</ConfirmRevokeTitle>
            <ConfirmRevokeBody>
              Any agent using this token will immediately lose access to{" "}
              <strong>{workspaceName}</strong>. This can&rsquo;t be undone.
            </ConfirmRevokeBody>
            <ConfirmRevokeActions>
              <Button
                size="medium"
                type="button"
                variant="secondary-fill"
                onClick={() => setConfirmingRevoke(false)}
                disabled={revoking}
              >
                Cancel
              </Button>
              <RemoveButton
                size="medium"
                type="button"
                disabled={revoking}
                onClick={() => onRevoke(token)}
              >
                {revoking ? "Removing..." : "Yes, remove"}
              </RemoveButton>
            </ConfirmRevokeActions>
          </ConfirmRevokePanel>
        ) : (
          <DialogFooterRow>
            <RemoveButton
              size="medium"
              type="button"
              disabled={revoking}
              onClick={() => setConfirmingRevoke(true)}
            >
              Remove MCP server
            </RemoveButton>
            <Button size="medium" type="button" variant="secondary-fill" onClick={onClose}>
              Close
            </Button>
          </DialogFooterRow>
        )}
      </DialogCard>
    </DialogOverlay>
  );
}

function buildHostedMCPTemplate(workspaceName: string) {
  return JSON.stringify(
    {
      mcpServers: {
        [`afs-${workspaceName}`]: {
          url: `${getControlPlaneURL().replace(/\/+$/, "")}/mcp`,
          headers: {
            Authorization: "Bearer <YOUR_TOKEN>",
          },
        },
      },
    },
    null,
    2,
  );
}

/* ------------------------------------------------------------------ */
/*  Main component                                                    */
/* ------------------------------------------------------------------ */

export function MCPServersTable({
  rows,
  loading = false,
  error = false,
  errorMessage = "Unable to load MCP servers. Please retry.",
  workspaceNameById,
  databaseNameById,
  toolbarAction,
  onRevoke,
  revoking = false,
}: Props) {
  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] = useState<MCPSortField>("createdAt");
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("desc");
  const [selectedToken, setSelectedToken] = useState<AFSMCPToken | null>(null);

  const filteredRows = useMemo(() => {
    const query = search.trim().toLowerCase();
    const baseRows =
      query === ""
        ? rows
        : rows.filter((row) =>
            [
              row.name ?? "",
              row.id,
              row.workspaceName ?? "",
              row.workspaceId,
              row.databaseId,
              row.profile,
            ].some((value) => value.toLowerCase().includes(query)),
          );

    return [...baseRows].sort((left, right) => {
      const leftValue = sortValue(left, sortBy, workspaceNameById);
      const rightValue = sortValue(right, sortBy, workspaceNameById);
      return compareValues(leftValue, rightValue, sortDirection);
    });
  }, [rows, search, sortBy, sortDirection, workspaceNameById]);

  const sorting = useMemo<SortingState>(
    () => [{ id: sortBy, desc: sortDirection === "desc" }],
    [sortBy, sortDirection],
  );

  const columns = useMemo(
    () =>
      [
        {
          accessorKey: "name",
          header: "Name",
          size: 200,
          enableSorting: true,
          cell: ({ row }) => (
            <NameCell>
              <NameIconBox>
                <PlugIcon customSize={18} />
              </NameIconBox>
              <S.Stack>
                <Typography.Body component="strong">
                  {row.original.name?.trim() || "Unnamed"}
                </Typography.Body>
                <Typography.Body color="secondary" component="span" style={{ fontSize: 11 }}>
                  {row.original.id}
                </Typography.Body>
              </S.Stack>
            </NameCell>
          ),
        },
        {
          accessorKey: "workspaceName",
          header: "Workspace",
          size: 140,
          enableSorting: true,
          cell: ({ row }) => {
            const name =
              row.original.workspaceName
              || workspaceNameById?.get(row.original.workspaceId)
              || row.original.workspaceId;
            const db = databaseNameById?.get(row.original.databaseId) || row.original.databaseId;
            return (
              <S.Stack>
                <S.SingleLineText title={name}>{name}</S.SingleLineText>
                <Typography.Body color="secondary" component="span" style={{ fontSize: 11 }}>
                  {db}
                </Typography.Body>
              </S.Stack>
            );
          },
        },
        {
          accessorKey: "lastUsedAt",
          header: "Last used",
          size: 100,
          enableSorting: true,
          cell: ({ row }) => (
            <Typography.Body
              component="span"
              color={row.original.lastUsedAt ? undefined : "secondary"}
            >
              {timeAgoOrDate(row.original.lastUsedAt)}
            </Typography.Body>
          ),
        },
        {
          accessorKey: "expiresAt",
          header: "Expires",
          size: 100,
          enableSorting: true,
          cell: ({ row }) => (
            <S.Stack>
              <Typography.Body component="span">{expiryLabel(row.original.expiresAt)}</Typography.Body>
              {row.original.expiresAt ? (
                <Typography.Body color="secondary" component="span" style={{ fontSize: 11 }}>
                  {new Date(row.original.expiresAt).toLocaleDateString()}
                </Typography.Body>
              ) : null}
            </S.Stack>
          ),
        },
        {
          accessorKey: "createdAt",
          header: "Created",
          size: 100,
          enableSorting: true,
          cell: ({ row }) => (
            <Typography.Body component="span" color="secondary">
              {timeAgoOrDate(row.original.createdAt)}
            </Typography.Body>
          ),
        },
      ] as ColumnDef<AFSMCPToken>[],
    [databaseNameById, workspaceNameById],
  );

  const isFiltering = search.trim() !== "";

  return (
    <>
      <S.TableBlock>
      <S.HeadingWrap style={{ padding: 0 }}>
        <S.SearchInput
          value={search}
          onChange={setSearch}
          placeholder="Search MCP servers..."
        />
        {toolbarAction}
      </S.HeadingWrap>

      {loading ? <S.EmptyState>Loading MCP servers...</S.EmptyState> : null}
      {error ? <S.EmptyState role="alert">{errorMessage}</S.EmptyState> : null}
      {!loading && !error && filteredRows.length === 0 ? (
        <S.EmptyState>
          {isFiltering
            ? "No MCP servers match the current filter."
            : "No hosted MCP servers yet. Click \u201CAdd MCP server\u201D to create one."}
        </S.EmptyState>
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
                setSortBy(next.id as MCPSortField);
                setSortDirection(next.desc ? "desc" : "asc");
              }}
              enableSorting
              stripedRows
              onRowClick={(rowData) => setSelectedToken(rowData)}
            />
          </S.DenseTableViewport>
        </S.TableCard>
      ) : null}
      </S.TableBlock>

      {selectedToken != null ? (
        <MCPServerDetailDialog
          token={selectedToken}
          workspaceNameById={workspaceNameById}
          databaseNameById={databaseNameById}
          revoking={revoking}
          onClose={() => setSelectedToken(null)}
          onRevoke={(token) => {
            onRevoke(token);
            setSelectedToken(null);
          }}
        />
      ) : null}
    </>
  );
}

function sortValue(
  token: AFSMCPToken,
  field: MCPSortField,
  workspaceNameById?: Map<string, string>,
): string | number {
  switch (field) {
    case "name":
      return token.name?.trim() || token.id;
    case "workspaceName":
      return token.workspaceName || workspaceNameById?.get(token.workspaceId) || token.workspaceId;
    case "lastUsedAt":
      return token.lastUsedAt ? new Date(token.lastUsedAt).getTime() : 0;
    case "expiresAt":
      return token.expiresAt ? new Date(token.expiresAt).getTime() : Number.MAX_SAFE_INTEGER;
    case "createdAt":
    default:
      return new Date(token.createdAt).getTime();
  }
}

/* ------------------------------------------------------------------ */
/*  Styled                                                            */
/* ------------------------------------------------------------------ */

const DetailGrid = styled.div`
  display: grid;
  gap: 16px;
  grid-template-columns: 1fr 1fr;
  margin-top: 8px;

  @media (max-width: 600px) {
    grid-template-columns: 1fr;
  }
`;

const DetailField = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
`;

const DetailLabel = styled.span`
  color: var(--afs-muted, #71717a);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
`;

const DetailValue = styled.span`
  color: var(--afs-ink, #18181b);
  font-size: 14px;
  word-break: break-all;
`;

const NameCell = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
`;

const NameIconBox = styled.span`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: var(--afs-accent-soft, rgba(37, 99, 235, 0.08));
  color: var(--afs-accent, #2563eb);
`;

const ConfigBlock = styled.div`
  display: grid;
  gap: 8px;
  margin-top: 20px;
`;

const ConfigHint = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 12px;
  line-height: 1.55;

  code {
    font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    background: var(--afs-panel);
    padding: 1px 5px;
    border-radius: 6px;
    border: 1px solid var(--afs-line);
    font-size: 11px;
  }
`;

const CodeBlock = styled.pre`
  margin: 0;
  padding: 14px;
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.94);
  color: #e2e8f0;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
  overflow: auto;
  max-height: 240px;
`;

const ConfigActions = styled.div`
  display: flex;
  justify-content: flex-end;
`;

const DialogFooterRow = styled.div`
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-top: 24px;
`;

const ConfirmRevokePanel = styled.div`
  margin-top: 24px;
  padding: 16px 18px;
  border: 1px solid rgba(220, 38, 38, 0.28);
  border-radius: 14px;
  background: rgba(239, 68, 68, 0.06);
`;

const ConfirmRevokeTitle = styled.div`
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 700;
`;

const ConfirmRevokeBody = styled.p`
  margin: 6px 0 0;
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.55;
`;

const ConfirmRevokeActions = styled.div`
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 14px;

  @media (max-width: 600px) {
    flex-direction: column-reverse;
  }
`;

const RemoveButton = styled(Button)`
  && {
    background: ${({ theme }) => theme.semantic.color.background.danger500};
    border-color: ${({ theme }) => theme.semantic.color.background.danger500};
    color: ${({ theme }) => theme.semantic.color.text.inverse};
  }

  &&:hover:not(:disabled) {
    background: ${({ theme }) => theme.semantic.color.background.danger600};
    border-color: ${({ theme }) => theme.semantic.color.background.danger600};
  }
`;
