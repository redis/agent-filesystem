import { Button } from "@redis-ui/components";
import { useEffect, useMemo, useState } from "react";
import styled from "styled-components";
import { NoticeBody, NoticeCard, NoticeTitle, Tag } from "../../components/afs-kit";
import { afsApi, formatBytes } from "../../foundation/api/afs";
import {
  useChangelog,
  useDiffFileVersionsMutation,
  useFileHistory,
  useFileVersionContent,
  useRestoreFileVersionMutation,
  useUndeleteFileVersionMutation,
} from "../../foundation/hooks/use-afs";
import { shortDateTime } from "../../foundation/time-format";
import type {
  AFSChangelogEntry,
  AFSFileHistoryLineage,
  AFSFileHistoryResponse,
  AFSFileVersion,
} from "../../foundation/types/afs";

const HISTORY_PAGE_SIZE = 50;

type Props = {
  databaseId?: string;
  workspaceId: string;
  path: string;
  editable: boolean;
  initialVersionId?: string;
  onClose: () => void;
};

type SelectedVersion = {
  version: AFSFileVersion;
  lineage: AFSFileHistoryLineage;
};

export function FileHistoryDrawer({
  databaseId,
  workspaceId,
  path,
  editable,
  initialVersionId,
  onClose,
}: Props) {
  const pathActivityQuery = useChangelog(
    {
      databaseId,
      workspaceId,
      path,
      limit: 25,
      direction: "desc",
    },
    path.trim() !== "",
  );
  const historyQuery = useFileHistory(
    {
      databaseId,
      workspaceId,
      path,
      direction: "desc",
      limit: HISTORY_PAGE_SIZE,
    },
    path.trim() !== "",
  );
  const diffMutation = useDiffFileVersionsMutation();
  const restoreMutation = useRestoreFileVersionMutation();
  const undeleteMutation = useUndeleteFileVersionMutation();
  const [extraPages, setExtraPages] = useState<AFSFileHistoryResponse[]>([]);
  const [selectedKey, setSelectedKey] = useState<string>("");
  const [targetVersionId, setTargetVersionId] = useState(initialVersionId ?? "");
  const [panelMode, setPanelMode] = useState<"content" | "diff">("content");
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null);

  useEffect(() => {
    setExtraPages([]);
    setSelectedKey("");
    setTargetVersionId(initialVersionId ?? "");
    setPanelMode("content");
    setActionMessage(null);
    setLoadMoreError(null);
  }, [path, initialVersionId]);

  const mergedHistory = useMemo(() => {
    const pages = [historyQuery.data, ...extraPages].filter(Boolean) as AFSFileHistoryResponse[];
    if (pages.length === 0) {
      return null;
    }
    const order: string[] = [];
    const lineages = new Map<string, AFSFileHistoryLineage>();
    let nextCursor = "";
    for (const page of pages) {
      nextCursor = page.nextCursor ?? "";
      for (const lineage of page.lineages) {
        const existing = lineages.get(lineage.fileId);
        if (!existing) {
          order.push(lineage.fileId);
          lineages.set(lineage.fileId, {
            ...lineage,
            versions: [...lineage.versions],
          });
          continue;
        }
        existing.currentPath = lineage.currentPath;
        existing.state = lineage.state;
        existing.versions.push(...lineage.versions);
      }
    }
    return {
      workspaceId: pages[0].workspaceId,
      path: pages[0].path,
      order: pages[0].order,
      lineages: order.map((fileId) => lineages.get(fileId)).filter(Boolean) as AFSFileHistoryLineage[],
      nextCursor,
    } satisfies AFSFileHistoryResponse;
  }, [extraPages, historyQuery.data]);

  const flattenedVersions = useMemo(
    () =>
      (mergedHistory?.lineages ?? []).flatMap((lineage) =>
        lineage.versions.map((version) => ({
          lineage,
          version,
        })),
      ),
    [mergedHistory],
  );

  useEffect(() => {
    if (flattenedVersions.length === 0) {
      return;
    }
    if (targetVersionId) {
      const exact = flattenedVersions.find((entry) => entry.version.versionId === targetVersionId);
      if (exact) {
        const nextKey = versionSelectionKey(exact.version);
        if (selectedKey !== nextKey) {
          setSelectedKey(nextKey);
        }
        return;
      }
    }
    if (selectedKey !== "") {
      return;
    }
    setSelectedKey(versionSelectionKey(flattenedVersions[0].version));
  }, [flattenedVersions, selectedKey, targetVersionId]);

  const selected = useMemo<SelectedVersion | null>(
    () =>
      flattenedVersions.find((entry) => versionSelectionKey(entry.version) === selectedKey) ?? null,
    [flattenedVersions, selectedKey],
  );

  const selectedVersionContentQuery = useFileVersionContent(
    selected
      ? {
          databaseId,
          workspaceId,
          path,
          fileId: selected.version.fileId,
          ordinal: selected.version.ordinal,
        }
      : targetVersionId
        ? {
            databaseId,
            workspaceId,
            path,
            versionId: targetVersionId,
          }
        : {
            databaseId,
            workspaceId,
            path,
            versionId: "",
          },
    selected != null || Boolean(targetVersionId),
  );

  const displayedVersion = selected?.version;
  const displayedLineage = selected?.lineage;
  const content = selectedVersionContentQuery.data;
  const canDiff = displayedVersion != null && displayedVersion.kind !== "tombstone";
  const canRestore = editable && displayedVersion != null && displayedVersion.kind !== "tombstone";
  const canUndelete =
    editable && displayedVersion != null && (displayedVersion.kind === "tombstone" || displayedLineage?.state === "deleted");
  const busy =
    diffMutation.isPending || restoreMutation.isPending || undeleteMutation.isPending;

  async function handleLoadMore() {
    if (!mergedHistory?.nextCursor) {
      return;
    }
    try {
      setLoadMoreError(null);
      const page = await afsApi.getFileHistory({
        databaseId,
        workspaceId,
        path,
        direction: "desc",
        limit: HISTORY_PAGE_SIZE,
        cursor: mergedHistory.nextCursor,
      });
      setExtraPages((current) => [...current, page]);
    } catch (error) {
      setLoadMoreError(error instanceof Error ? error.message : "Unable to load more versions.");
    }
  }

  async function handleDiff() {
    if (!displayedVersion) {
      return;
    }
    setActionMessage(null);
    setPanelMode("diff");
    await diffMutation.mutateAsync({
      databaseId,
      workspaceId,
      path,
      from: { fileId: displayedVersion.fileId, ordinal: displayedVersion.ordinal },
      to: { ref: "head" },
    });
  }

  async function handleRestore() {
    if (!displayedVersion) {
      return;
    }
    const response = await restoreMutation.mutateAsync({
      databaseId,
      workspaceId,
      path,
      fileId: displayedVersion.fileId,
      ordinal: displayedVersion.ordinal,
    });
    setActionMessage(`Restored ${response.restoredFromVersionId} into the working copy.`);
  }

  async function handleUndelete() {
    if (!displayedVersion) {
      return;
    }
    const response = await undeleteMutation.mutateAsync({
      databaseId,
      workspaceId,
      path,
      fileId: displayedVersion.fileId,
      ordinal: displayedVersion.ordinal,
    });
    setActionMessage(`Undeleted ${response.undeletedFromVersionId} into the working copy.`);
  }

  function handlePathActivitySelect(entry: AFSChangelogEntry) {
    setActionMessage(null);
    setPanelMode("content");
    if (!entry.versionId) {
      return;
    }
    setTargetVersionId(entry.versionId);
    const exact = flattenedVersions.find((item) => item.version.versionId === entry.versionId);
    if (exact) {
      setSelectedKey(versionSelectionKey(exact.version));
    }
  }

  return (
    <Overlay onClick={onClose}>
      <Panel onClick={(event) => event.stopPropagation()} role="dialog" aria-modal="true">
        <Header>
          <HeaderText>
            <Title>File history</Title>
            <PathText>{path}</PathText>
          </HeaderText>
          <Button size="small" kind="ghost" onClick={onClose}>
            Close
          </Button>
        </Header>

        {actionMessage ? (
          <NoticeCard role="status">
            <NoticeTitle>History action complete</NoticeTitle>
            <NoticeBody>{actionMessage}</NoticeBody>
          </NoticeCard>
        ) : null}

        <Body>
          <Sidebar>
            <SidebarSection>
              <SidebarSectionHeader>
                <SectionTitle>Path activity</SectionTitle>
                <SectionCaption>Recent file operations for this path.</SectionCaption>
              </SidebarSectionHeader>
              {pathActivityQuery.isLoading ? <EmptyMessage>Loading path activity…</EmptyMessage> : null}
              {pathActivityQuery.isError ? (
                <EmptyMessage role="alert">
                  {pathActivityQuery.error instanceof Error ? pathActivityQuery.error.message : "Unable to load path activity."}
                </EmptyMessage>
              ) : null}
              {!pathActivityQuery.isLoading && !pathActivityQuery.isError && (pathActivityQuery.data?.entries.length ?? 0) === 0 ? (
                <EmptyMessage>No recent path activity for this file.</EmptyMessage>
              ) : null}
              {pathActivityQuery.data?.entries.map((entry) => {
                const linkedToSelected = entry.versionId != null
                  && displayedVersion != null
                  && entry.versionId === displayedVersion.versionId;
                return (
                  <ActivityButton
                    key={entry.id}
                    type="button"
                    $active={linkedToSelected}
                    onClick={() => handlePathActivitySelect(entry)}
                  >
                    <VersionTitleRow>
                      <strong>{entry.op}</strong>
                      <span>{shortDateTime(entry.occurredAt ?? "")}</span>
                    </VersionTitleRow>
                    <VersionMeta>
                      <span>{activityActorLabel(entry)}</span>
                      <span>{entry.versionId ? `v${linkedOrdinal(entry.versionId, flattenedVersions) ?? "?"}` : "event only"}</span>
                    </VersionMeta>
                  </ActivityButton>
                );
              })}
            </SidebarSection>

            <SidebarSection>
              <SidebarSectionHeader>
                <SectionTitle>File history</SectionTitle>
                <SectionCaption>Ordered versions for this file lineage.</SectionCaption>
              </SidebarSectionHeader>
            {historyQuery.isLoading ? <EmptyMessage>Loading file history…</EmptyMessage> : null}
            {historyQuery.isError ? (
              <EmptyMessage role="alert">
                {historyQuery.error instanceof Error ? historyQuery.error.message : "Unable to load file history."}
              </EmptyMessage>
            ) : null}
            {!historyQuery.isLoading && !historyQuery.isError && (mergedHistory?.lineages.length ?? 0) === 0 ? (
              <EmptyMessage>No tracked history yet for this path.</EmptyMessage>
            ) : null}
            {mergedHistory?.lineages.map((lineage) => (
              <LineageCard key={lineage.fileId}>
                <LineageHeader>
                  <Tag>{lineage.state}</Tag>
                  <LineageMeta>{lineage.fileId.slice(0, 12)}</LineageMeta>
                </LineageHeader>
                <VersionList>
                  {lineage.versions.map((version) => {
                    const selectedNow = versionSelectionKey(version) === selectedKey;
                    return (
                      <VersionButton
                        key={version.versionId}
                        type="button"
                        $active={selectedNow}
                        onClick={() => {
                          setSelectedKey(versionSelectionKey(version));
                          setPanelMode("content");
                          setActionMessage(null);
                        }}
                      >
                        <VersionTitleRow>
                          <strong>v{version.ordinal}</strong>
                          <span>{version.op}</span>
                        </VersionTitleRow>
                        <VersionMeta>
                          <span>{shortDateTime(version.createdAt)}</span>
                          <span>{version.versionId.slice(0, 12)}</span>
                        </VersionMeta>
                      </VersionButton>
                    );
                  })}
                </VersionList>
              </LineageCard>
            ))}

            {mergedHistory?.nextCursor ? (
              <LoadMoreWrap>
                <Button size="small" variant="secondary-fill" onClick={() => void handleLoadMore()}>
                  Load more history
                </Button>
                {loadMoreError ? <InlineError role="alert">{loadMoreError}</InlineError> : null}
              </LoadMoreWrap>
            ) : null}
            </SidebarSection>
          </Sidebar>

          <DetailPane>
            {displayedVersion ? (
              <>
                <DetailHeader>
                  <DetailMeta>
                    <DetailTitle>{displayedVersion.versionId}</DetailTitle>
                    <DetailTags>
                      <Tag>{displayedVersion.op}</Tag>
                      <Tag>{displayedVersion.kind}</Tag>
                      <Tag>ordinal {displayedVersion.ordinal}</Tag>
                      {displayedVersion.sizeBytes != null ? <Tag>{formatBytes(displayedVersion.sizeBytes)}</Tag> : null}
                    </DetailTags>
                  </DetailMeta>
                  <DetailActions>
                    <Button
                      size="small"
                      variant="secondary-fill"
                      onClick={() => void handleDiff()}
                      disabled={!canDiff || busy}
                    >
                      {diffMutation.isPending ? "Diffing…" : "Diff against head"}
                    </Button>
                    <Button
                      size="small"
                      variant="secondary-fill"
                      onClick={() => void handleRestore()}
                      disabled={!canRestore || busy}
                    >
                      {restoreMutation.isPending ? "Restoring…" : "Restore"}
                    </Button>
                    <Button
                      size="small"
                      onClick={() => void handleUndelete()}
                      disabled={!canUndelete || busy}
                    >
                      {undeleteMutation.isPending ? "Undeleting…" : "Undelete"}
                    </Button>
                  </DetailActions>
                </DetailHeader>

                <MetadataTable>
                  <tbody>
                    <tr>
                      <th>Path</th>
                      <td>{displayedVersion.path}</td>
                    </tr>
                    {displayedVersion.prevPath ? (
                      <tr>
                        <th>Previous path</th>
                        <td>{displayedVersion.prevPath}</td>
                      </tr>
                    ) : null}
                    <tr>
                      <th>File ID</th>
                      <td>{displayedVersion.fileId}</td>
                    </tr>
                    <tr>
                      <th>Source</th>
                      <td>{displayedVersion.source || "unknown"}</td>
                    </tr>
                    <tr>
                      <th>Created</th>
                      <td>{shortDateTime(displayedVersion.createdAt)}</td>
                    </tr>
                  </tbody>
                </MetadataTable>

                {panelMode === "diff" ? (
                  <>
                    {diffMutation.isError ? (
                      <InlineError role="alert">
                        {diffMutation.error instanceof Error ? diffMutation.error.message : "Unable to diff this version."}
                      </InlineError>
                    ) : null}
                    {diffMutation.data?.binary ? (
                      <EmptyMessage>Binary diff. A textual unified diff is not available for this version.</EmptyMessage>
                    ) : (
                      <CodeBlock>{diffMutation.data?.diff ?? ""}</CodeBlock>
                    )}
                  </>
                ) : selectedVersionContentQuery.isLoading ? (
                  <EmptyMessage>Loading version content…</EmptyMessage>
                ) : selectedVersionContentQuery.isError ? (
                  <InlineError role="alert">
                    {selectedVersionContentQuery.error instanceof Error ? selectedVersionContentQuery.error.message : "Unable to load version content."}
                  </InlineError>
                ) : content?.kind === "tombstone" ? (
                  <EmptyMessage>This version is a tombstone. Use undelete to bring the deleted lineage back.</EmptyMessage>
                ) : content?.binary ? (
                  <EmptyMessage>Binary file. History metadata is available, but the content is not rendered in the browser.</EmptyMessage>
                ) : (
                  <CodeBlock>{content?.content ?? content?.target ?? ""}</CodeBlock>
                )}
              </>
            ) : (
              <EmptyMessage>Select a version to inspect its content or compare it to head.</EmptyMessage>
            )}
          </DetailPane>
        </Body>
      </Panel>
    </Overlay>
  );
}

function versionSelectionKey(version: Pick<AFSFileVersion, "fileId" | "ordinal">) {
  return `${version.fileId}:${version.ordinal}`;
}

function linkedOrdinal(
  versionId: string,
  flattenedVersions: Array<{ lineage: AFSFileHistoryLineage; version: AFSFileVersion }>,
) {
  return flattenedVersions.find((entry) => entry.version.versionId === versionId)?.version.ordinal;
}

function activityActorLabel(entry: AFSChangelogEntry) {
  return entry.label?.trim()
    || entry.agentId?.trim()
    || entry.sessionId?.trim()
    || entry.user?.trim()
    || entry.source?.trim()
    || "system";
}

const Overlay = styled.div`
  position: fixed;
  inset: 0;
  z-index: 90;
  display: flex;
  justify-content: flex-end;
  background: rgba(10, 15, 29, 0.44);
  backdrop-filter: blur(2px);
`;

const Panel = styled.div`
  width: min(1100px, 96vw);
  height: 100%;
  background: var(--afs-bg);
  border-left: 1px solid var(--afs-line);
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 24px;
  overflow: hidden;
`;

const Header = styled.div`
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
`;

const HeaderText = styled.div`
  display: grid;
  gap: 6px;
`;

const Title = styled.h2`
  margin: 0;
  color: var(--afs-ink);
  font-size: 20px;
`;

const PathText = styled.code`
  color: var(--afs-muted);
  font-size: 13px;
`;

const Body = styled.div`
  min-height: 0;
  display: grid;
  grid-template-columns: minmax(300px, 360px) minmax(0, 1fr);
  gap: 18px;

  @media (max-width: 980px) {
    grid-template-columns: 1fr;
  }
`;

const Sidebar = styled.div`
  min-height: 0;
  overflow: auto;
  border: 1px solid var(--afs-line);
  border-radius: 18px;
  background: var(--afs-panel);
  padding: 14px;
  display: grid;
  align-content: start;
  gap: 12px;
`;

const SidebarSection = styled.div`
  display: grid;
  gap: 10px;
`;

const SidebarSectionHeader = styled.div`
  display: grid;
  gap: 4px;
`;

const SectionTitle = styled.h3`
  margin: 0;
  color: var(--afs-ink);
  font-size: 14px;
`;

const SectionCaption = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 12px;
`;

const DetailPane = styled.div`
  min-height: 0;
  overflow: auto;
  border: 1px solid var(--afs-line);
  border-radius: 18px;
  background: var(--afs-panel-strong);
  padding: 18px;
  display: grid;
  align-content: start;
  gap: 16px;
`;

const LineageCard = styled.div`
  display: grid;
  gap: 10px;
  border: 1px solid var(--afs-line);
  border-radius: 14px;
  padding: 12px;
  background: rgba(255, 255, 255, 0.45);
`;

const LineageHeader = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
`;

const LineageMeta = styled.code`
  color: var(--afs-muted);
  font-size: 12px;
`;

const VersionList = styled.div`
  display: grid;
  gap: 8px;
`;

const VersionButton = styled.button<{ $active: boolean }>`
  width: 100%;
  text-align: left;
  border-radius: 12px;
  border: 1px solid ${({ $active }) => ($active ? "var(--afs-focus)" : "var(--afs-line)")};
  background: ${({ $active }) => ($active ? "var(--afs-focus-soft)" : "var(--afs-panel-strong)")};
  color: var(--afs-ink);
  padding: 10px 12px;
  cursor: pointer;
`;

const ActivityButton = styled(VersionButton)`
  background: ${({ $active }) => ($active ? "var(--afs-focus-soft)" : "rgba(255, 255, 255, 0.45)")};
`;

const VersionTitleRow = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
`;

const VersionMeta = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 12px;
  color: var(--afs-muted);
  font-size: 12px;
  margin-top: 6px;
`;

const DetailHeader = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;

  @media (max-width: 760px) {
    flex-direction: column;
  }
`;

const DetailMeta = styled.div`
  display: grid;
  gap: 8px;
`;

const DetailTitle = styled.h3`
  margin: 0;
  color: var(--afs-ink);
  font-size: 15px;
  line-height: 1.5;
  word-break: break-all;
`;

const DetailTags = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
`;

const DetailActions = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
`;

const MetadataTable = styled.table`
  width: 100%;
  border-collapse: collapse;

  th,
  td {
    text-align: left;
    padding: 10px 0;
    border-top: 1px solid var(--afs-line);
    vertical-align: top;
    font-size: 13px;
  }

  th {
    width: 140px;
    color: var(--afs-muted);
  }

  td {
    color: var(--afs-ink);
    word-break: break-word;
  }
`;

const CodeBlock = styled.pre`
  margin: 0;
  min-height: 240px;
  overflow: auto;
  border-radius: 14px;
  border: 1px solid var(--afs-line);
  background: #09101d;
  color: #dde9ff;
  padding: 16px;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
`;

const EmptyMessage = styled.div`
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const InlineError = styled.div`
  color: #b91c1c;
  font-size: 13px;
  line-height: 1.5;
`;

const LoadMoreWrap = styled.div`
  display: grid;
  gap: 8px;
`;
