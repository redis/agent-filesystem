import { Button, Typography } from "@redis-ui/components";
import { Table } from "@redis-ui/table";
import type { ColumnDef, SortingState } from "@redis-ui/table";
import { useCallback, useMemo, useState } from "react";
import styled from "styled-components";
import {
  DialogBody,
  DialogCard,
  DialogCloseButton,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
  Field,
  FormGrid,
  InlineActions,
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
  TextArea,
  TextInput,
} from "../../components/afs-kit";
import {
  useCreateSavepointMutation,
  useRestoreSavepointMutation,
  useWorkspaceDiff,
} from "../../foundation/hooks/use-afs";
import { shortDateTime } from "../../foundation/time-format";
import * as S from "../../foundation/tables/workspace-table.styles";
import { getActiveWorkspaceView } from "../../foundation/workspace-browser-views";
import type {
  AFSDiffEntry,
  AFSSavepoint,
  AFSWorkspaceDetail,
  AFSWorkspaceDiffResponse,
  AFSWorkspaceView,
} from "../../foundation/types/afs";

type StudioTab = "browse" | "checkpoints" | "activity" | "settings";
type CheckpointSortField = "createdAt" | "name" | "actor" | "totalBytes";
type DiffDialogMode = "compare" | "restore";
type DiffDialogState = {
  savepoint: AFSSavepoint;
  mode: DiffDialogMode;
};

type Props = {
  workspace: AFSWorkspaceDetail;
  onBrowserViewChange: (view: AFSWorkspaceView) => void;
  onTabChange: (tab: StudioTab) => void;
};

export function CheckpointsTab({ workspace, onBrowserViewChange, onTabChange }: Props) {
  const createSavepoint = useCreateSavepointMutation();
  const restoreSavepoint = useRestoreSavepointMutation();
  const restoreCheckpoint = restoreSavepoint.mutate;
  const restoreCheckpointPending = restoreSavepoint.isPending;

  const [savepointName, setSavepointName] = useState("");
  const [savepointNote, setSavepointNote] = useState("");
  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] = useState<CheckpointSortField>("createdAt");
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("desc");
  const [selectedSavepoint, setSelectedSavepoint] = useState<AFSSavepoint | null>(null);
  const [diffDialog, setDiffDialog] = useState<DiffDialogState | null>(null);

  const openSavepoint = useCallback(
    (savepoint: AFSSavepoint) => {
      onBrowserViewChange(
        isActiveCheckpoint(workspace, savepoint)
          ? getActiveWorkspaceView(workspace)
          : `checkpoint:${savepoint.id}`,
      );
      onTabChange("browse");
    },
    [onBrowserViewChange, onTabChange, workspace],
  );

  const filteredSavepoints = useMemo(() => {
    const query = search.trim().toLowerCase();
    const baseRows =
      query === ""
        ? workspace.savepoints
        : workspace.savepoints.filter((savepoint) =>
            [
              savepoint.id,
              savepoint.name,
              savepoint.note,
              savepoint.kind ?? "",
              savepoint.source ?? "",
              checkpointActor(savepoint),
              savepoint.author,
              savepoint.sizeLabel,
            ].some((value) => value.toLowerCase().includes(query)),
          );

    return [...baseRows].sort((left, right) =>
      compareValues(
        checkpointSortValue(left, sortBy),
        checkpointSortValue(right, sortBy),
        sortDirection,
      ),
    );
  }, [workspace.savepoints, search, sortBy, sortDirection]);

  const sorting = useMemo<SortingState>(
    () => [{ id: sortBy, desc: sortDirection === "desc" }],
    [sortBy, sortDirection],
  );

  const columns = useMemo<ColumnDef<AFSSavepoint>[]>(
    () => [
      {
        accessorKey: "createdAt",
        header: "Created",
        size: 110,
        enableSorting: true,
        cell: ({ row }) => shortDateTime(row.original.createdAt),
      },
      {
        accessorKey: "name",
        header: "Checkpoint",
        size: 260,
        enableSorting: true,
        cell: ({ row }) => {
          const savepoint = row.original;
          const isActive = isActiveCheckpoint(workspace, savepoint);
          return (
            <CheckpointNameCell>
              <CheckpointNameStack>
                <CheckpointTitleRow>
                  <CheckpointTitle title={savepoint.name}>{savepoint.name}</CheckpointTitle>
                  {isActive ? <ActiveCheckpointBadge>Active</ActiveCheckpointBadge> : null}
                </CheckpointTitleRow>
                <S.SingleLineText title={savepoint.note || "No description provided."}>
                  {savepoint.note || "No description provided."}
                </S.SingleLineText>
              </CheckpointNameStack>
            </CheckpointNameCell>
          );
        },
      },
      {
        id: "actor",
        header: "Actor",
        size: 140,
        enableSorting: true,
        cell: ({ row }) => (
          <S.SingleLineText title={checkpointActor(row.original) || "Unknown"}>
            {checkpointActor(row.original) || "Unknown"}
          </S.SingleLineText>
        ),
      },
      {
        accessorKey: "totalBytes",
        header: "Contents",
        size: 150,
        enableSorting: true,
        cell: ({ row }) => (
          <S.Stack>
            <Typography.Body component="span">{row.original.sizeLabel}</Typography.Body>
            <Typography.Body color="secondary" component="span">
              {row.original.fileCount} files · {row.original.folderCount} folders
            </Typography.Body>
          </S.Stack>
        ),
      },
      {
        id: "actions",
        header: "",
        size: 210,
        enableSorting: false,
        cell: ({ row }) => {
          const savepoint = row.original;
          const isActive = isActiveCheckpoint(workspace, savepoint);
          return (
            <CheckpointActions>
              <S.TextActionButton
                type="button"
                onClick={(event) => {
                  event.stopPropagation();
                  openSavepoint(savepoint);
                }}
              >
                Browse
              </S.TextActionButton>
              <S.TextActionButton
                type="button"
                onClick={(event) => {
                  event.stopPropagation();
                  setDiffDialog({ savepoint, mode: "compare" });
                }}
              >
                Compare
              </S.TextActionButton>
              <S.TextActionButton
                type="button"
                disabled={
                  !workspace.capabilities.restoreCheckpoint ||
                  restoreCheckpointPending ||
                  isActive
                }
                onClick={(event) => {
                  event.stopPropagation();
                  setDiffDialog({ savepoint, mode: "restore" });
                }}
              >
                Restore
              </S.TextActionButton>
            </CheckpointActions>
          );
        },
      },
    ],
    [
      openSavepoint,
      restoreCheckpointPending,
      workspace.capabilities.restoreCheckpoint,
      workspace,
    ],
  );

  const isFiltering = search.trim() !== "";

  return (
    <>
      {workspace.capabilities.createCheckpoint ? (
        <SectionGrid>
          <SectionCard $span={12}>
            <SectionHeader>
              <SectionTitle title="Create checkpoint" />
            </SectionHeader>
            <FormGrid
              onSubmit={(event) => {
                event.preventDefault();
                if (savepointName.trim() === "") {
                  return;
                }

                createSavepoint.mutate({
                  workspaceId: workspace.id,
                  name: savepointName,
                  note: savepointNote,
                });
                setSavepointName("");
                setSavepointNote("");
              }}
            >
              <Field>
                Checkpoint name
                <TextInput
                  value={savepointName}
                  onChange={(event) => setSavepointName(event.target.value)}
                  placeholder="after-editor-pass"
                />
              </Field>
              <Field>
                Checkpoint description
                <TextArea
                  value={savepointNote}
                  onChange={(event) => setSavepointNote(event.target.value)}
                  placeholder="Why this checkpoint exists."
                />
              </Field>
              <InlineActions>
                <Button
                  size="medium"
                  type="submit"
                  disabled={createSavepoint.isPending}
                >
                  Create checkpoint
                </Button>
              </InlineActions>
            </FormGrid>
          </SectionCard>
        </SectionGrid>
      ) : null}

      <SectionGrid>
        <SectionCard $span={12}>
          <SectionHeader>
            <SectionTitle title="Checkpoint history" />
          </SectionHeader>
          <S.TableBlock>
            <S.HeadingWrap style={{ padding: 0 }}>
              <S.SearchInput
                value={search}
                onChange={setSearch}
                placeholder="Search checkpoints..."
              />
            </S.HeadingWrap>

            {filteredSavepoints.length === 0 ? (
              <S.EmptyState>
                {isFiltering
                  ? "No checkpoints match the current filter."
                  : "No checkpoints recorded yet."}
              </S.EmptyState>
            ) : (
              <S.TableCard>
                <S.DenseTableViewport>
                  <Table
                    columns={columns}
                    data={filteredSavepoints}
                    getRowId={(row) => row.id}
                    sorting={sorting}
                    manualSorting
                    onSortingChange={(nextState) => {
                      if (nextState.length === 0) {
                        setSortBy("createdAt");
                        setSortDirection("desc");
                        return;
                      }

                      const next = nextState[0];
                      setSortBy(next.id as CheckpointSortField);
                      setSortDirection(next.desc ? "desc" : "asc");
                    }}
                    enableSorting
                    stripedRows
                    onRowClick={(rowData) => setSelectedSavepoint(rowData)}
                  />
                </S.DenseTableViewport>
              </S.TableCard>
            )}
          </S.TableBlock>
        </SectionCard>
      </SectionGrid>

      {selectedSavepoint ? (
        <CheckpointDetailDialog
          savepoint={selectedSavepoint}
          isActive={isActiveCheckpoint(workspace, selectedSavepoint)}
          restoreDisabled={
            !workspace.capabilities.restoreCheckpoint ||
            restoreCheckpointPending ||
            isActiveCheckpoint(workspace, selectedSavepoint)
          }
          onClose={() => setSelectedSavepoint(null)}
          onBrowse={() => {
            openSavepoint(selectedSavepoint);
            setSelectedSavepoint(null);
          }}
          onCompare={() => {
            setDiffDialog({ savepoint: selectedSavepoint, mode: "compare" });
            setSelectedSavepoint(null);
          }}
          onRestorePreview={() => {
            setDiffDialog({ savepoint: selectedSavepoint, mode: "restore" });
            setSelectedSavepoint(null);
          }}
        />
      ) : null}

      {diffDialog ? (
        <CheckpointDiffDialog
          workspace={workspace}
          savepoint={diffDialog.savepoint}
          mode={diffDialog.mode}
          restoreDisabled={
            !workspace.capabilities.restoreCheckpoint ||
            restoreCheckpointPending ||
            isActiveCheckpoint(workspace, diffDialog.savepoint)
          }
          restorePending={restoreCheckpointPending}
          onClose={() => setDiffDialog(null)}
          onBrowse={() => {
            openSavepoint(diffDialog.savepoint);
            setDiffDialog(null);
          }}
          onRestore={() => {
            restoreCheckpoint({
              databaseId: workspace.databaseId,
              workspaceId: workspace.id,
              savepointId: diffDialog.savepoint.id,
            });
            setDiffDialog(null);
          }}
        />
      ) : null}
    </>
  );
}

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

function checkpointSortValue(savepoint: AFSSavepoint, field: CheckpointSortField): string | number {
  switch (field) {
    case "createdAt":
      return Date.parse(savepoint.createdAt) || 0;
    case "actor":
      return checkpointActor(savepoint);
    case "totalBytes":
      return savepoint.totalBytes;
    case "name":
      return savepoint.name;
    default:
      return savepoint.name;
  }
}

function checkpointActor(savepoint: AFSSavepoint) {
  return savepoint.agentName || savepoint.agentId || savepoint.createdBy || savepoint.author || "";
}

function isActiveCheckpoint(workspace: AFSWorkspaceDetail, savepoint: AFSSavepoint) {
  return getActiveWorkspaceView(workspace) === "head" && savepoint.id === workspace.headSavepointId;
}

function formatCheckpointKind(kind: string) {
  return kind
    .split(/[-_]/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function formatCheckpointSource(source: string) {
  switch (source) {
    case "mcp":
      return "MCP";
    case "cli":
      return "CLI";
    default:
      return formatCheckpointKind(source);
  }
}

function CheckpointDetailDialog({
  savepoint,
  isActive,
  restoreDisabled,
  onClose,
  onBrowse,
  onCompare,
  onRestorePreview,
}: {
  savepoint: AFSSavepoint;
  isActive: boolean;
  restoreDisabled: boolean;
  onClose: () => void;
  onBrowse: () => void;
  onCompare: () => void;
  onRestorePreview: () => void;
}) {
  const actor = checkpointActor(savepoint) || "Unknown";
  const typeLabel = savepoint.kind ? formatCheckpointKind(savepoint.kind) : "Checkpoint";
  const sourceLabel = savepoint.source ? formatCheckpointSource(savepoint.source) : "Unknown";

  return (
    <DialogOverlay onClick={onClose}>
      <CheckpointDialogCard onClick={(event) => event.stopPropagation()}>
        <DialogHeader>
          <div>
            <DialogTitle>{savepoint.name}</DialogTitle>
            <DialogBody>{savepoint.note || "No description provided."}</DialogBody>
          </div>
          <DialogCloseButton type="button" aria-label="Close" onClick={onClose}>
            &times;
          </DialogCloseButton>
        </DialogHeader>

        <CheckpointBadgeRow>
          {isActive ? <ActiveCheckpointBadge>Active</ActiveCheckpointBadge> : null}
          <S.MetaBadge>{typeLabel}</S.MetaBadge>
          <S.MetaBadge>{sourceLabel}</S.MetaBadge>
          <S.MetaBadge>{actor}</S.MetaBadge>
        </CheckpointBadgeRow>

        <DetailGrid>
          <DetailField>
            <DetailLabel>Checkpoint ID</DetailLabel>
            <DetailValue $mono title={savepoint.id}>{savepoint.id}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Created</DetailLabel>
            <DetailValue>{new Date(savepoint.createdAt).toLocaleString()}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Actor</DetailLabel>
            <DetailValue>{actor}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Author</DetailLabel>
            <DetailValue>{savepoint.author || "Unknown"}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Kind</DetailLabel>
            <DetailValue>{typeLabel}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Source</DetailLabel>
            <DetailValue>{sourceLabel}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Agent Name</DetailLabel>
            <DetailValue>{savepoint.agentName || "Not set"}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Agent ID</DetailLabel>
            <DetailValue $mono title={savepoint.agentId || "Not set"}>
              {savepoint.agentId || "Not set"}
            </DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Created By</DetailLabel>
            <DetailValue>{savepoint.createdBy || "Unknown"}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Session ID</DetailLabel>
            <DetailValue $mono title={savepoint.sessionId || "Not set"}>
              {savepoint.sessionId || "Not set"}
            </DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Parent Checkpoint</DetailLabel>
            <DetailValue $mono title={savepoint.parentCheckpointId || "None"}>
              {savepoint.parentCheckpointId || "None"}
            </DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Manifest Hash</DetailLabel>
            <DetailValue $mono title={savepoint.manifestHash || "Not recorded"}>
              {savepoint.manifestHash || "Not recorded"}
            </DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Files</DetailLabel>
            <DetailValue>{savepoint.fileCount.toLocaleString()}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Folders</DetailLabel>
            <DetailValue>{savepoint.folderCount.toLocaleString()}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Size</DetailLabel>
            <DetailValue>{savepoint.sizeLabel}</DetailValue>
          </DetailField>
          <DetailField>
            <DetailLabel>Total Bytes</DetailLabel>
            <DetailValue>{savepoint.totalBytes.toLocaleString()}</DetailValue>
          </DetailField>
        </DetailGrid>

        <DialogFooter>
          <Button size="medium" variant="secondary-fill" onClick={onBrowse}>
            Browse checkpoint
          </Button>
          <Button size="medium" variant="secondary-fill" onClick={onCompare}>
            Compare with live
          </Button>
          <Button size="medium" disabled={restoreDisabled} onClick={onRestorePreview}>
            Restore checkpoint
          </Button>
        </DialogFooter>
      </CheckpointDialogCard>
    </DialogOverlay>
  );
}

function CheckpointDiffDialog({
  workspace,
  savepoint,
  mode,
  restoreDisabled,
  restorePending,
  onClose,
  onBrowse,
  onRestore,
}: {
  workspace: AFSWorkspaceDetail;
  savepoint: AFSSavepoint;
  mode: DiffDialogMode;
  restoreDisabled: boolean;
  restorePending: boolean;
  onClose: () => void;
  onBrowse: () => void;
  onRestore: () => void;
}) {
  const checkpointView = `checkpoint:${savepoint.id}` as AFSWorkspaceView;
  const activeView = getActiveWorkspaceView(workspace);
  const base = mode === "restore" ? activeView : checkpointView;
  const target = mode === "restore" ? checkpointView : activeView;
  const diffQuery = useWorkspaceDiff(
    {
      databaseId: workspace.databaseId,
      workspaceId: workspace.id,
      base,
      head: target,
    },
    true,
  );
  const diff = diffQuery.data;
  const title = mode === "restore" ? `Restore ${savepoint.name}` : `Compare ${savepoint.name}`;
  const body =
    mode === "restore"
      ? "Review how the active workspace will change before restoring this checkpoint."
      : "Review what changed between this checkpoint and the active workspace.";

  return (
    <DialogOverlay onClick={onClose}>
      <CheckpointDialogCard onClick={(event) => event.stopPropagation()}>
        <DialogHeader>
          <div>
            <DialogTitle>{title}</DialogTitle>
            <DialogBody>{body}</DialogBody>
          </div>
          <DialogCloseButton type="button" aria-label="Close" onClick={onClose}>
            &times;
          </DialogCloseButton>
        </DialogHeader>

        <CheckpointBadgeRow>
          <S.MetaBadge>{viewLabel(workspace, base)}</S.MetaBadge>
          <S.MetaBadge>to</S.MetaBadge>
          <S.MetaBadge>{viewLabel(workspace, target)}</S.MetaBadge>
        </CheckpointBadgeRow>

        {diffQuery.isLoading ? (
          <DiffMessage>Loading checkpoint diff...</DiffMessage>
        ) : diffQuery.isError ? (
          <DiffMessage role="alert">
            {diffQuery.error instanceof Error
              ? diffQuery.error.message
              : "Unable to load checkpoint diff."}
          </DiffMessage>
        ) : diff ? (
          <DiffReview diff={diff} />
        ) : null}

        <DialogFooter>
          <Button size="medium" variant="secondary-fill" onClick={onBrowse}>
            Browse checkpoint
          </Button>
          {mode === "restore" ? (
            <Button
              size="medium"
              disabled={restoreDisabled || restorePending || diffQuery.isLoading || diffQuery.isError}
              onClick={onRestore}
            >
              {restorePending ? "Restoring..." : "Confirm restore"}
            </Button>
          ) : null}
        </DialogFooter>
      </CheckpointDialogCard>
    </DialogOverlay>
  );
}

function DiffReview({ diff }: { diff: AFSWorkspaceDiffResponse }) {
  const entries = diff.entries.slice(0, 160);
  const hiddenCount = diff.entries.length - entries.length;

  return (
    <DiffStack>
      <DiffSummaryGrid>
        <DiffStat>
          <DetailLabel>Total</DetailLabel>
          <DiffStatValue>{diff.summary.total.toLocaleString()}</DiffStatValue>
        </DiffStat>
        <DiffStat>
          <DetailLabel>Created</DetailLabel>
          <DiffStatValue>{diff.summary.created.toLocaleString()}</DiffStatValue>
        </DiffStat>
        <DiffStat>
          <DetailLabel>Updated</DetailLabel>
          <DiffStatValue>{diff.summary.updated.toLocaleString()}</DiffStatValue>
        </DiffStat>
        <DiffStat>
          <DetailLabel>Deleted</DetailLabel>
          <DiffStatValue>{diff.summary.deleted.toLocaleString()}</DiffStatValue>
        </DiffStat>
        <DiffStat>
          <DetailLabel>Renamed</DetailLabel>
          <DiffStatValue>{diff.summary.renamed.toLocaleString()}</DiffStatValue>
        </DiffStat>
        <DiffStat>
          <DetailLabel>Bytes</DetailLabel>
          <DiffStatValue>
            +{formatBytesForDiff(diff.summary.bytesAdded)} / -{formatBytesForDiff(diff.summary.bytesRemoved)}
          </DiffStatValue>
        </DiffStat>
      </DiffSummaryGrid>

      {diff.entries.length === 0 ? (
        <DiffMessage>No file changes between these states.</DiffMessage>
      ) : (
        <DiffList>
          {entries.map((entry) => (
            <DiffRow key={`${entry.op}:${entry.previousPath ?? ""}:${entry.path}`}>
              <DiffOpBadge $op={entry.op}>{formatDiffOp(entry.op)}</DiffOpBadge>
              <DiffPathStack>
                <DiffPath title={diffEntryPathTitle(entry)}>{diffEntryPath(entry)}</DiffPath>
                <DiffMeta>{diffEntryMeta(entry)}</DiffMeta>
              </DiffPathStack>
              <DiffDelta>{formatDiffDelta(entry.deltaBytes)}</DiffDelta>
            </DiffRow>
          ))}
          {hiddenCount > 0 ? (
            <DiffMessage>{hiddenCount.toLocaleString()} more changes not shown.</DiffMessage>
          ) : null}
        </DiffList>
      )}
    </DiffStack>
  );
}

function viewLabel(workspace: AFSWorkspaceDetail, view: AFSWorkspaceView) {
  if (view === "working-copy" || view === "head") return "Active workspace";
  const checkpointId = view.replace(/^checkpoint:/, "");
  const savepoint = workspace.savepoints.find((item) => item.id === checkpointId);
  return savepoint ? `Checkpoint ${savepoint.name}` : `Checkpoint ${checkpointId}`;
}

function formatDiffOp(op: AFSDiffEntry["op"]) {
  switch (op) {
    case "create":
      return "Create";
    case "update":
      return "Update";
    case "delete":
      return "Delete";
    case "rename":
      return "Rename";
    case "metadata":
      return "Metadata";
    default:
      return op;
  }
}

function diffEntryPath(entry: AFSDiffEntry) {
  if (entry.op === "rename" && entry.previousPath) {
    return `${entry.previousPath} -> ${entry.path}`;
  }
  return entry.path;
}

function diffEntryPathTitle(entry: AFSDiffEntry) {
  if (entry.op === "rename" && entry.previousPath) {
    return `${entry.previousPath} -> ${entry.path}`;
  }
  return entry.path;
}

function diffEntryMeta(entry: AFSDiffEntry) {
  const kind = entry.kind ?? entry.previousKind ?? "file";
  const before = entry.previousSizeBytes == null ? "" : formatBytesForDiff(entry.previousSizeBytes);
  const after = entry.sizeBytes == null ? "" : formatBytesForDiff(entry.sizeBytes);
  if (before !== "" && after !== "" && before !== after) {
    return `${kind} · ${before} -> ${after}`;
  }
  return before || after ? `${kind} · ${before || after}` : kind;
}

function formatBytesForDiff(value: number) {
  if (value === 0) return "0 B";
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(value < 10 * 1024 ? 1 : 0)} KB`;
  }
  if (value < 1024 * 1024 * 1024) {
    return `${(value / (1024 * 1024)).toFixed(1)} MB`;
  }
  return `${(value / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

function formatDiffDelta(value?: number) {
  if (value == null || value === 0) return "";
  return value > 0 ? `+${formatBytesForDiff(value)}` : `-${formatBytesForDiff(Math.abs(value))}`;
}

const CheckpointNameCell = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
`;

const CheckpointNameStack = styled(S.Stack)`
  flex: 1 1 auto;
  min-width: 0;
`;

const CheckpointTitleRow = styled.div`
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  min-width: 0;
`;

const CheckpointTitle = styled.span`
  min-width: 0;
  overflow: hidden;
  color: var(--afs-ink);
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const CheckpointActions = styled.div`
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  white-space: nowrap;
`;

const ActiveCheckpointBadge = styled(S.MetaBadge)`
  border-color: #15803d;
  background: #16a34a;
  color: #fff;
  font-weight: 800;
  box-shadow: 0 0 0 2px rgba(22, 163, 74, 0.16);
`;

const CheckpointDialogCard = styled(DialogCard)`
  width: min(980px, 100%);
`;

const CheckpointBadgeRow = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 18px;
`;

const DiffStack = styled.div`
  display: grid;
  gap: 16px;
`;

const DiffSummaryGrid = styled.div`
  display: grid;
  gap: 10px;
  grid-template-columns: repeat(6, minmax(0, 1fr));

  @media (max-width: 860px) {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  @media (max-width: 560px) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
`;

const DiffStat = styled.div`
  min-width: 0;
  border: 1px solid var(--afs-line);
  border-radius: 8px;
  padding: 10px;
  background: var(--afs-panel);
`;

const DiffStatValue = styled.span`
  display: block;
  overflow: hidden;
  color: var(--afs-ink);
  font-size: 16px;
  font-weight: 800;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const DiffList = styled.div`
  display: grid;
  max-height: min(420px, 48vh);
  overflow: auto;
  border: 1px solid var(--afs-line);
  border-radius: 8px;
  background: var(--afs-panel-strong);
`;

const DiffRow = styled.div`
  display: grid;
  grid-template-columns: 90px minmax(0, 1fr) 96px;
  gap: 12px;
  align-items: center;
  min-height: 48px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--afs-line);

  &:last-child {
    border-bottom: none;
  }

  @media (max-width: 640px) {
    grid-template-columns: 76px minmax(0, 1fr);
  }
`;

const opTone = {
  create: "#15803d",
  update: "#2563eb",
  delete: "#b91c1c",
  rename: "#7c3aed",
  metadata: "#64748b",
} as const;

const DiffOpBadge = styled.span<{ $op: AFSDiffEntry["op"] }>`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 78px;
  border: 1px solid ${({ $op }) => opTone[$op]};
  border-radius: 999px;
  padding: 3px 8px;
  color: ${({ $op }) => opTone[$op]};
  font-size: 11px;
  font-weight: 800;
`;

const DiffPathStack = styled.div`
  display: grid;
  gap: 3px;
  min-width: 0;
`;

const DiffPath = styled.span`
  overflow: hidden;
  color: var(--afs-ink);
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
  font-size: 12px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const DiffMeta = styled.span`
  overflow: hidden;
  color: var(--afs-muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const DiffDelta = styled.span`
  color: var(--afs-muted);
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
  font-size: 12px;
  text-align: right;

  @media (max-width: 640px) {
    display: none;
  }
`;

const DiffMessage = styled.div`
  padding: 24px;
  border: 1px solid var(--afs-line);
  border-radius: 8px;
  background: var(--afs-panel);
  color: var(--afs-muted);
  text-align: center;
`;

const DetailGrid = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(2, minmax(0, 1fr));

  @media (max-width: 720px) {
    grid-template-columns: 1fr;
  }
`;

const DetailField = styled.div`
  min-width: 0;
  border: 1px solid var(--afs-line);
  border-radius: 8px;
  padding: 12px;
  background: var(--afs-panel);
`;

const DetailLabel = styled.span`
  display: block;
  margin-bottom: 5px;
  color: var(--afs-muted);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
`;

const DetailValue = styled.span<{ $mono?: boolean }>`
  display: block;
  min-width: 0;
  overflow: hidden;
  color: var(--afs-ink);
  font-family: ${({ $mono }) => ($mono ? "var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace)" : "inherit")};
  font-size: ${({ $mono }) => ($mono ? "12px" : "14px")};
  font-weight: 600;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
`;
