import { Button, Typography } from "@redis-ui/components";
import { Fragment, useCallback, useMemo, useState } from "react";
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
import { compareValues } from "../../foundation/sort-compare";
import { shortDateTime } from "../../foundation/time-format";
import * as S from "../../foundation/tables/workspace-table.styles";
import { getActiveWorkspaceView } from "../../foundation/workspace-browser-views";
import type { StudioTab } from "../../foundation/workspace-tabs";
import type {
  AFSDiffEntry,
  AFSSavepoint,
  AFSWorkspaceDetail,
  AFSWorkspaceDiffResponse,
  AFSWorkspaceView,
} from "../../foundation/types/afs";

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
  const [expandedSavepointId, setExpandedSavepointId] = useState<string | null>(null);
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

  const isFiltering = search.trim() !== "";

  const setCheckpointSort = useCallback((field: CheckpointSortField) => {
    setSortBy((currentField) => {
      if (currentField === field) {
        setSortDirection((direction) => (direction === "asc" ? "desc" : "asc"));
        return currentField;
      }
      setSortDirection(field === "createdAt" ? "desc" : "asc");
      return field;
    });
  }, []);

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
                <CheckpointHistoryViewport>
                  <CheckpointHistoryTable
                    aria-label="Checkpoint history"
                  >
                    <thead>
                      <tr>
                        <SortableCheckpointHeader
                          field="createdAt"
                          activeField={sortBy}
                          direction={sortDirection}
                          onSort={setCheckpointSort}
                        >
                          Created
                        </SortableCheckpointHeader>
                        <SortableCheckpointHeader
                          field="name"
                          activeField={sortBy}
                          direction={sortDirection}
                          onSort={setCheckpointSort}
                        >
                          Checkpoint
                        </SortableCheckpointHeader>
                        <SortableCheckpointHeader
                          field="actor"
                          activeField={sortBy}
                          direction={sortDirection}
                          onSort={setCheckpointSort}
                        >
                          Actor
                        </SortableCheckpointHeader>
                        <SortableCheckpointHeader
                          field="totalBytes"
                          activeField={sortBy}
                          direction={sortDirection}
                          onSort={setCheckpointSort}
                        >
                          Contents
                        </SortableCheckpointHeader>
                        <CheckpointHeaderCell>Actions</CheckpointHeaderCell>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredSavepoints.map((savepoint) => {
                        const isActive = isActiveCheckpoint(workspace, savepoint);
                        const isExpanded = expandedSavepointId === savepoint.id;
                        return (
                          <Fragment key={savepoint.id}>
                            <CheckpointSummaryRow
                              $expanded={isExpanded}
                              onClick={() =>
                                setExpandedSavepointId((current) =>
                                  current === savepoint.id ? null : savepoint.id,
                                )
                              }
                            >
                              <CheckpointCell>{shortDateTime(savepoint.createdAt)}</CheckpointCell>
                              <CheckpointCell>
                                <CheckpointNameCell>
                                  <ExpandGlyph aria-hidden="true" $expanded={isExpanded}>
                                    &gt;
                                  </ExpandGlyph>
                                  <CheckpointNameStack>
                                    <CheckpointTitleRow>
                                      <CheckpointTitle title={savepoint.name}>
                                        {savepoint.name}
                                      </CheckpointTitle>
                                      {isActive ? <ActiveCheckpointBadge>Active</ActiveCheckpointBadge> : null}
                                    </CheckpointTitleRow>
                                    <S.SingleLineText title={savepoint.note || "No description provided."}>
                                      {savepoint.note || "No description provided."}
                                    </S.SingleLineText>
                                  </CheckpointNameStack>
                                </CheckpointNameCell>
                              </CheckpointCell>
                              <CheckpointCell>
                                <S.SingleLineText title={checkpointActor(savepoint) || "Unknown"}>
                                  {checkpointActor(savepoint) || "Unknown"}
                                </S.SingleLineText>
                              </CheckpointCell>
                              <CheckpointCell>
                                <S.Stack>
                                  <Typography.Body component="span">{savepoint.sizeLabel}</Typography.Body>
                                  <Typography.Body color="secondary" component="span">
                                    {savepoint.fileCount} files · {savepoint.folderCount} folders
                                  </Typography.Body>
                                </S.Stack>
                              </CheckpointCell>
                              <CheckpointCell onClick={(event) => event.stopPropagation()}>
                                <CheckpointActions>
                                  <S.TextActionButton
                                    type="button"
                                    onClick={() => setSelectedSavepoint(savepoint)}
                                  >
                                    Details
                                  </S.TextActionButton>
                                  <S.TextActionButton
                                    type="button"
                                    onClick={() => openSavepoint(savepoint)}
                                  >
                                    Browse
                                  </S.TextActionButton>
                                  <S.TextActionButton
                                    type="button"
                                    onClick={() => setDiffDialog({ savepoint, mode: "compare" })}
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
                                    onClick={() => setDiffDialog({ savepoint, mode: "restore" })}
                                  >
                                    Restore
                                  </S.TextActionButton>
                                </CheckpointActions>
                              </CheckpointCell>
                            </CheckpointSummaryRow>
                            {isExpanded ? (
                              <CheckpointExpandedRow>
                                <CheckpointExpandedCell colSpan={5}>
                                  <CheckpointExpandedPanel
                                    savepoint={savepoint}
                                    isActive={isActive}
                                  />
                                </CheckpointExpandedCell>
                              </CheckpointExpandedRow>
                            ) : null}
                          </Fragment>
                        );
                      })}
                    </tbody>
                  </CheckpointHistoryTable>
                </CheckpointHistoryViewport>
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

function SortableCheckpointHeader({
  field,
  activeField,
  direction,
  onSort,
  children,
}: {
  field: CheckpointSortField;
  activeField: CheckpointSortField;
  direction: "asc" | "desc";
  onSort: (field: CheckpointSortField) => void;
  children: string;
}) {
  const isActive = activeField === field;
  return (
    <CheckpointHeaderCell
      as="th"
      aria-sort={isActive ? (direction === "asc" ? "ascending" : "descending") : "none"}
    >
      <CheckpointSortButton type="button" onClick={() => onSort(field)}>
        {children}
        <SortIndicator>{isActive ? (direction === "asc" ? "^" : "v") : ""}</SortIndicator>
      </CheckpointSortButton>
    </CheckpointHeaderCell>
  );
}

function CheckpointExpandedPanel({
  savepoint,
  isActive,
}: {
  savepoint: AFSSavepoint;
  isActive: boolean;
}) {
  const actor = checkpointActor(savepoint) || "Unknown";
  const typeLabel = savepoint.kind ? formatCheckpointKind(savepoint.kind) : "Checkpoint";
  const sourceLabel = savepoint.source ? formatCheckpointSource(savepoint.source) : "Unknown";

  return (
    <ExpandedPanelStack>
      <ExpandedPanelHeader>
        <ExpandedPanelTitle>{savepoint.name}</ExpandedPanelTitle>
        <CheckpointBadgeRow>
          {isActive ? <ActiveCheckpointBadge>Active</ActiveCheckpointBadge> : null}
          <S.MetaBadge>{typeLabel}</S.MetaBadge>
          <S.MetaBadge>{sourceLabel}</S.MetaBadge>
          <S.MetaBadge>{actor}</S.MetaBadge>
        </CheckpointBadgeRow>
      </ExpandedPanelHeader>
      <ExpandedDescription>{savepoint.note || "No description provided."}</ExpandedDescription>
      <ExpandedDetailGrid>
        <DetailField>
          <DetailLabel>Checkpoint ID</DetailLabel>
          <DetailValue $mono title={savepoint.id}>{savepoint.id}</DetailValue>
        </DetailField>
        <DetailField>
          <DetailLabel>Created</DetailLabel>
          <DetailValue>{new Date(savepoint.createdAt).toLocaleString()}</DetailValue>
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
          <DetailLabel>Contents</DetailLabel>
          <DetailValue>
            {savepoint.fileCount.toLocaleString()} files · {savepoint.folderCount.toLocaleString()} folders · {savepoint.sizeLabel}
          </DetailValue>
        </DetailField>
      </ExpandedDetailGrid>
    </ExpandedPanelStack>
  );
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
            <DiffRowShell key={`${entry.op}:${entry.previousPath ?? ""}:${entry.path}`}>
              <DiffRow>
                <DiffOpBadge $op={entry.op}>{formatDiffOp(entry.op)}</DiffOpBadge>
                <DiffPathStack>
                  <DiffPath title={diffEntryPathTitle(entry)}>{diffEntryPath(entry)}</DiffPath>
                  <DiffMeta>{diffEntryMeta(entry)}</DiffMeta>
                </DiffPathStack>
                <DiffDelta>{formatDiffDelta(entry.deltaBytes)}</DiffDelta>
              </DiffRow>
              <InlineTextDiff entry={entry} />
            </DiffRowShell>
          ))}
          {hiddenCount > 0 ? (
            <DiffMessage>{hiddenCount.toLocaleString()} more changes not shown.</DiffMessage>
          ) : null}
        </DiffList>
      )}
    </DiffStack>
  );
}

function InlineTextDiff({ entry }: { entry: AFSDiffEntry }) {
  const textDiff = entry.textDiff;
  if (textDiff == null) {
    return null;
  }
  if (!textDiff.available) {
    return textDiff.skippedReason ? (
      <DiffInlineBlock>
        <DiffInlineNotice>Text diff not shown: {textDiff.skippedReason}</DiffInlineNotice>
      </DiffInlineBlock>
    ) : null;
  }

  const hunks = textDiff.hunks ?? [];
  if (hunks.length === 0) {
    return null;
  }

  return (
    <DiffInlineBlock>
      {hunks.slice(0, 8).map((hunk, hunkIndex) => (
        <DiffHunk key={`${entry.path}:${hunk.oldStart}:${hunk.newStart}:${hunkIndex}`}>
          <DiffHunkHeader>
            @@ -{hunk.oldStart},{hunk.oldLines} +{hunk.newStart},{hunk.newLines} @@
          </DiffHunkHeader>
          {hunk.lines.slice(0, 80).map((line, lineIndex) => (
            <DiffCodeLine
              key={`${line.oldLine ?? ""}:${line.newLine ?? ""}:${lineIndex}`}
              $kind={line.kind}
            >
              <DiffLineNo>{line.oldLine ?? ""}</DiffLineNo>
              <DiffLineNo>{line.newLine ?? ""}</DiffLineNo>
              <DiffLineSign>{diffLineSign(line.kind)}</DiffLineSign>
              <DiffLineText>{line.text === "" ? " " : line.text}</DiffLineText>
            </DiffCodeLine>
          ))}
          {hunk.lines.length > 80 ? (
            <DiffInlineNotice>{hunk.lines.length - 80} more lines not shown.</DiffInlineNotice>
          ) : null}
        </DiffHunk>
      ))}
      {hunks.length > 8 ? (
        <DiffInlineNotice>{hunks.length - 8} more hunks not shown.</DiffInlineNotice>
      ) : null}
    </DiffInlineBlock>
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

function diffLineSign(kind: "context" | "delete" | "insert") {
  switch (kind) {
    case "delete":
      return "-";
    case "insert":
      return "+";
    default:
      return " ";
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

const CheckpointHistoryViewport = styled.div`
  max-height: 720px;
  overflow: auto;
`;

const CheckpointHistoryTable = styled.table`
  width: 100%;
  min-width: 860px;
  border-collapse: collapse;
  table-layout: fixed;
`;

const CheckpointHeaderCell = styled.th`
  position: sticky;
  top: 0;
  z-index: 2;
  padding: 10px 12px;
  border-bottom: 1px solid var(--afs-line);
  background: var(--afs-panel-strong);
  color: var(--afs-muted);
  font-size: 10.5px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-align: left;
  text-transform: uppercase;

  &:first-child {
    width: 120px;
  }

  &:nth-child(2) {
    width: 34%;
  }

  &:nth-child(3) {
    width: 140px;
  }

  &:nth-child(4) {
    width: 170px;
  }

  &:last-child {
    width: 250px;
    text-align: right;
  }
`;

const CheckpointSortButton = styled.button`
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: none;
  background: transparent;
  color: inherit;
  cursor: pointer;
  font: inherit;
  letter-spacing: inherit;
  padding: 0;
  text-transform: inherit;
`;

const SortIndicator = styled.span`
  display: inline-block;
  width: 10px;
  color: var(--afs-ink-soft);
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
`;

const CheckpointSummaryRow = styled.tr<{ $expanded: boolean }>`
  cursor: pointer;
  background: ${({ $expanded }) => ($expanded ? "var(--afs-panel)" : "transparent")};
  transition: background 160ms ease;

  &:hover {
    background: var(--afs-panel);
  }
`;

const CheckpointCell = styled.td`
  min-width: 0;
  padding: 10px 12px;
  border-bottom: 1px solid var(--afs-line);
  color: var(--afs-ink);
  font-size: 13px;
  vertical-align: middle;

  &:last-child {
    text-align: right;
  }
`;

const CheckpointExpandedRow = styled.tr`
  background: var(--afs-panel);
`;

const CheckpointExpandedCell = styled.td`
  padding: 0 12px 14px 132px;
  border-bottom: 1px solid var(--afs-line);

  @media (max-width: 760px) {
    padding-left: 12px;
  }
`;

const ExpandGlyph = styled.span<{ $expanded: boolean }>`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  color: var(--afs-muted);
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
  font-size: 12px;
  transform: rotate(${({ $expanded }) => ($expanded ? "90deg" : "0deg")});
  transition: transform 160ms ease;
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
  gap: 10px;
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

const ExpandedPanelStack = styled.div`
  display: grid;
  gap: 10px;
  padding: 12px;
  border: 1px solid var(--afs-line);
  border-radius: 8px;
  background: var(--afs-panel-strong);
`;

const ExpandedPanelHeader = styled.div`
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;

  ${CheckpointBadgeRow} {
    margin-bottom: 0;
    justify-content: flex-end;
  }
`;

const ExpandedPanelTitle = styled.span`
  min-width: 0;
  overflow: hidden;
  color: var(--afs-ink);
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const ExpandedDescription = styled.div`
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.45;
`;

const ExpandedDetailGrid = styled.div`
  display: grid;
  gap: 10px;
  grid-template-columns: repeat(3, minmax(0, 1fr));

  @media (max-width: 900px) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  @media (max-width: 640px) {
    grid-template-columns: 1fr;
  }
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

const DiffRowShell = styled.div`
  display: grid;
  border-bottom: 1px solid var(--afs-line);

  &:last-child {
    border-bottom: none;
  }
`;

const DiffRow = styled.div`
  display: grid;
  grid-template-columns: 90px minmax(0, 1fr) 96px;
  gap: 12px;
  align-items: center;
  min-height: 48px;
  padding: 10px 12px;

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

const DiffInlineBlock = styled.div`
  display: grid;
  gap: 8px;
  margin: 0 12px 12px 114px;
  overflow: hidden;
  border: 1px solid var(--afs-line);
  border-radius: 8px;
  background: #fff;
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);

  @media (max-width: 640px) {
    margin-left: 12px;
  }
`;

const DiffHunk = styled.div`
  display: grid;
`;

const DiffHunkHeader = styled.div`
  padding: 7px 10px;
  border-bottom: 1px solid var(--afs-line);
  background: #f8fafc;
  color: #475569;
  font-size: 11px;
  font-weight: 700;
`;

const DiffCodeLine = styled.div<{ $kind: "context" | "delete" | "insert" }>`
  display: grid;
  grid-template-columns: 44px 44px 18px minmax(0, 1fr);
  min-width: 0;
  background: ${({ $kind }) =>
    $kind === "insert" ? "#f0fdf4" : $kind === "delete" ? "#fef2f2" : "#fff"};
  color: ${({ $kind }) =>
    $kind === "insert" ? "#166534" : $kind === "delete" ? "#991b1b" : "#334155"};
  font-size: 12px;
  line-height: 1.45;
`;

const DiffLineNo = styled.span`
  user-select: none;
  border-right: 1px solid rgba(148, 163, 184, 0.28);
  padding: 2px 8px;
  color: #94a3b8;
  text-align: right;
`;

const DiffLineSign = styled.span`
  user-select: none;
  padding: 2px 0;
  text-align: center;
`;

const DiffLineText = styled.span`
  min-width: 0;
  padding: 2px 10px 2px 0;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
`;

const DiffInlineNotice = styled.div`
  padding: 8px 10px;
  color: var(--afs-muted);
  font-size: 12px;
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
