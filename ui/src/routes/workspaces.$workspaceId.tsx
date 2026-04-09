import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader, Typography } from "@redislabsdev/redis-ui-components";
import { useEffect, useMemo, useState } from "react";
import styled from "styled-components";
import { z } from "zod";
import {
  CardHeader,
  EditorArea,
  EditorPanel,
  Field,
  FileButton,
  FileList,
  FileStudio,
  FormGrid,
  InlineActions,
  MetaRow,
  PageStack,
  SavepointGrid,
  SavepointRow,
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
  Select,
  TabButton,
  Tabs,
  Tag,
  TextArea,
  TextInput,
  ToneChip,
} from "../components/afs-kit";
import { formatBytes } from "../foundation/api/afs";
import { useDatabaseScope } from "../foundation/database-scope";
import {
  useCreateSavepointMutation,
  useDeleteWorkspaceMutation,
  useRestoreSavepointMutation,
  useUpdateWorkspaceFileMutation,
  useWorkspace,
  useWorkspaceFileContent,
  useWorkspaceTree,
} from "../foundation/hooks/use-afs";
import { ActivityTable } from "../foundation/tables/activity-table";
import type {
  AFSActivityEvent,
  AFSDraftState,
  AFSWorkspaceDetail,
  AFSWorkspaceView,
} from "../foundation/types/afs";

type StudioTab = "overview" | "files" | "checkpoints" | "activity";

const workspaceStudioSearchSchema = z.object({
  tab: z.enum(["overview", "files", "checkpoints", "activity"]).optional(),
});

export const Route = createFileRoute("/workspaces/$workspaceId")({
  validateSearch: workspaceStudioSearchSchema,
  component: WorkspaceStudioPage,
});

function WorkspaceStudioPage() {
  const navigate = useNavigate();
  const { workspaceId } = Route.useParams();
  const search = Route.useSearch();
  const { selectedDatabaseId } = useDatabaseScope();
  const workspaceQuery = useWorkspace(selectedDatabaseId, workspaceId);
  const deleteWorkspace = useDeleteWorkspaceMutation();
  const updateFile = useUpdateWorkspaceFileMutation();
  const createSavepoint = useCreateSavepointMutation();
  const restoreSavepoint = useRestoreSavepointMutation();

  const [browserView, setBrowserView] = useState<AFSWorkspaceView>("head");
  const [currentPath, setCurrentPath] = useState("/");
  const [selectedPath, setSelectedPath] = useState("");
  const [draftContent, setDraftContent] = useState("");
  const [savepointName, setSavepointName] = useState("");
  const [savepointNote, setSavepointNote] = useState("");
  const [rollbackTarget, setRollbackTarget] = useState("");

  const workspace = workspaceQuery.data;
  const tab = search.tab ?? "overview";

  useEffect(() => {
    if (workspace == null) {
      setBrowserView("head");
      setCurrentPath("/");
      setSelectedPath("");
      return;
    }

    const defaultView = defaultBrowserView(workspace);
    const allowedViews = browserViewOptions(workspace).map((item) => item.value);

    setBrowserView((current) => (allowedViews.includes(current) ? current : defaultView));
    setCurrentPath("/");
    setSelectedPath("");
    setRollbackTarget((current) =>
      workspace.savepoints.some((savepoint) => savepoint.id === current)
        ? current
        : workspace.headSavepointId,
    );
  }, [workspace]);

  const treeQuery = useWorkspaceTree(
    {
      databaseId: selectedDatabaseId ?? "",
      workspaceId,
      view: browserView,
      path: currentPath,
      depth: 1,
    },
    workspace != null,
  );

  const selectedFileQuery = useWorkspaceFileContent(
    {
      databaseId: selectedDatabaseId ?? "",
      workspaceId,
      view: browserView,
      path: selectedPath,
    },
    workspace != null && selectedPath !== "",
  );

  useEffect(() => {
    const file = selectedFileQuery.data;
    setDraftContent(file?.content ?? file?.target ?? "");
  }, [
    selectedFileQuery.data?.content,
    selectedFileQuery.data?.revision,
    selectedFileQuery.data?.target,
  ]);

  const browserItems = treeQuery.data?.items ?? [];
  const selectedFile = selectedFileQuery.data;
  const editable =
    workspace?.capabilities.editWorkingCopy === true &&
    browserView === "working-copy" &&
    selectedFile?.kind === "file";
  const currentViewLabel = useMemo(() => viewLabel(browserView, workspace), [browserView, workspace]);
  const latestActivity = workspace?.activity[0] ?? null;

  function setStudioTab(nextTab: StudioTab) {
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId },
      search: nextTab === "overview" ? {} : { tab: nextTab },
      replace: true,
    });
  }

  function deleteCurrentWorkspace() {
    const confirmed = window.confirm(
      `Delete workspace "${workspace?.name ?? workspaceId}"? This removes it from the workspace registry.`,
    );

    if (!confirmed) {
      return;
    }

    deleteWorkspace.mutate({
      databaseId: selectedDatabaseId ?? "",
      workspaceId,
    }, {
      onSuccess: () => {
        void navigate({ to: "/workspaces" });
      },
    });
  }

  if (workspaceQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  if (selectedDatabaseId == null) {
    throw new Error("No database selected.");
  }

  if (workspace == null) {
    throw new Error("Workspace not found.");
  }

  return (
    <PageStack>
      <StudioNavRow>
        <BreadcrumbButton
          type="button"
          onClick={() => {
            void navigate({ to: "/workspaces" });
          }}
        >
          Workspaces
        </BreadcrumbButton>
        <BreadcrumbSeparator>/</BreadcrumbSeparator>
        <BreadcrumbCurrent>{workspace.name}</BreadcrumbCurrent>
      </StudioNavRow>

      <SectionGrid>
        <SectionCard $span={12}>
          <TabsToolbar>
            <Tabs>
              <TabButton $active={tab === "overview"} onClick={() => setStudioTab("overview")}>
                Overview
              </TabButton>
              <TabButton $active={tab === "files"} onClick={() => setStudioTab("files")}>
                Files
              </TabButton>
              <TabButton $active={tab === "checkpoints"} onClick={() => setStudioTab("checkpoints")}>
                Checkpoints
              </TabButton>
              <TabButton $active={tab === "activity"} onClick={() => setStudioTab("activity")}>
                Activity
              </TabButton>
            </Tabs>
            <DeleteWorkspaceButton
              size="medium"
              disabled={deleteWorkspace.isPending}
              onClick={deleteCurrentWorkspace}
            >
              {deleteWorkspace.isPending ? "Deleting..." : "Delete workspace"}
            </DeleteWorkspaceButton>
          </TabsToolbar>
        </SectionCard>
      </SectionGrid>

      {tab === "overview" ? (
        <SectionGrid>
          <SectionCard $span={12}>
            <SectionHeader>
              <SectionTitle title="Status" />
            </SectionHeader>
            <StatusTable>
              <tbody>
                <StatusRow>
                  <StatusLabel>Files</StatusLabel>
                  <StatusValue>{workspace.fileCount.toLocaleString()}</StatusValue>
                </StatusRow>
                <StatusRow>
                  <StatusLabel>Folders</StatusLabel>
                  <StatusValue>{workspace.folderCount.toLocaleString()}</StatusValue>
                </StatusRow>
                <StatusRow>
                  <StatusLabel>Checkpoints</StatusLabel>
                  <StatusValue>{workspace.checkpointCount.toLocaleString()}</StatusValue>
                </StatusRow>
                <StatusRow>
                  <StatusLabel>Size</StatusLabel>
                  <StatusValue>{formatBytes(workspace.totalBytes)}</StatusValue>
                </StatusRow>
                <StatusRow>
                  <StatusLabel>Last updated</StatusLabel>
                  <StatusValue>{new Date(workspace.updatedAt).toLocaleString()}</StatusValue>
                </StatusRow>
                <StatusRow>
                  <StatusLabel>Latest activity</StatusLabel>
                  <StatusValue>
                    {latestActivity == null
                      ? "No activity yet"
                      : `${latestActivity.title} · ${new Date(latestActivity.createdAt).toLocaleString()}`}
                  </StatusValue>
                </StatusRow>
                <StatusRow>
                  <StatusLabel>Database</StatusLabel>
                  <StatusValue>{workspace.databaseName}</StatusValue>
                </StatusRow>
                <StatusRow>
                  <StatusLabel>Redis key</StatusLabel>
                  <StatusValue>{workspace.redisKey}</StatusValue>
                </StatusRow>
                {workspace.mountedPath ? (
                  <StatusRow>
                    <StatusLabel>Mounted path</StatusLabel>
                    <StatusValue>{workspace.mountedPath}</StatusValue>
                  </StatusRow>
                ) : null}
              </tbody>
            </StatusTable>
          </SectionCard>
        </SectionGrid>
      ) : null}

      {tab === "checkpoints" ? (
        <>
          <SectionGrid>
            <SectionCard $span={8}>
              <SectionHeader>
                <SectionTitle
                  title="Checkpoint history"
                  body="Recovery points live under each workspace so you can compare history, browse older state, and restore with confidence."
                />
              </SectionHeader>
              <SavepointGrid>
                {workspace.savepoints.map((savepoint) => (
                  <SavepointRow key={savepoint.id}>
                    <div>
                      <Typography.Body component="strong">{savepoint.name}</Typography.Body>
                      <Typography.Body color="secondary" component="p">
                        {savepoint.note || "No note provided."}
                      </Typography.Body>
                      <MetaRow>
                        <Tag>{savepoint.fileCount} files</Tag>
                        <Tag>{savepoint.folderCount} folders</Tag>
                        <Tag>{savepoint.sizeLabel}</Tag>
                        <Tag>{new Date(savepoint.createdAt).toLocaleString()}</Tag>
                        {savepoint.id === workspace.headSavepointId ? <Tag>Current head</Tag> : null}
                      </MetaRow>
                    </div>
                    <InlineActions>
                      <Button
                        size="medium"
                        variant="secondary-fill"
                        onClick={() => {
                          setBrowserView(
                            savepoint.id === workspace.headSavepointId
                              ? "head"
                              : `checkpoint:${savepoint.id}`,
                          );
                          setStudioTab("files");
                        }}
                      >
                        Browse
                      </Button>
                      <Button
                        size="medium"
                        variant="secondary-fill"
                        disabled={
                          !workspace.capabilities.restoreCheckpoint ||
                          restoreSavepoint.isPending ||
                          savepoint.id === workspace.headSavepointId
                        }
                        onClick={() =>
                          restoreSavepoint.mutate({
                            databaseId: selectedDatabaseId,
                            workspaceId: workspace.id,
                            savepointId: savepoint.id,
                          })
                        }
                      >
                        Restore
                      </Button>
                    </InlineActions>
                  </SavepointRow>
                ))}
              </SavepointGrid>
            </SectionCard>
          </SectionGrid>

          <SectionGrid>
            <SectionCard $span={12}>
              <SectionHeader>
                <SectionTitle
                  title="Checkpoint actions"
                  body="Create and restore checkpoints here so recovery work stays in one dedicated view."
                />
              </SectionHeader>
              <ActionStack>
                <Field>
                  Restore target
                  <Select
                    value={rollbackTarget}
                    onChange={(event) => setRollbackTarget(event.target.value)}
                  >
                    {workspace.savepoints.map((savepoint) => (
                      <option key={savepoint.id} value={savepoint.id}>
                        {savepoint.name}
                      </option>
                    ))}
                  </Select>
                </Field>
                <InlineActions>
                  <Button
                    size="medium"
                    variant="secondary-fill"
                    disabled={
                      !workspace.capabilities.restoreCheckpoint ||
                      restoreSavepoint.isPending ||
                      rollbackTarget === workspace.headSavepointId
                    }
                    onClick={() =>
                      restoreSavepoint.mutate({
                        databaseId: selectedDatabaseId,
                        workspaceId: workspace.id,
                        savepointId: rollbackTarget,
                      })
                    }
                  >
                    Restore checkpoint
                  </Button>
                </InlineActions>

                {workspace.capabilities.createCheckpoint ? (
                  <FormGrid
                    onSubmit={(event) => {
                      event.preventDefault();
                      if (savepointName.trim() === "") {
                        return;
                      }

                      createSavepoint.mutate({
                        databaseId: selectedDatabaseId,
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
                      Checkpoint note
                      <TextArea
                        value={savepointNote}
                        onChange={(event) => setSavepointNote(event.target.value)}
                        placeholder="Why this checkpoint exists."
                      />
                    </Field>
                    <Button
                      size="medium"
                      variant="secondary-fill"
                      type="submit"
                      disabled={createSavepoint.isPending}
                    >
                      Create checkpoint
                    </Button>
                  </FormGrid>
                ) : (
                  <PanelNote>
                    Checkpoint creation is unavailable in this transport because it needs a connected
                    working copy rather than canonical saved state alone.
                  </PanelNote>
                )}
              </ActionStack>
            </SectionCard>
          </SectionGrid>
        </>
      ) : null}

      {tab === "files" ? (
        <>
          <SectionGrid>
            <SectionCard $span={12}>
              <SectionHeader>
                <SectionTitle
                  title="Filesystem browser"
                  body="Directory slices and file content load lazily so this studio can scale from quick spot checks to real editing sessions."
                />
              </SectionHeader>

              <BrowserBanner>
                <BrowserMetric>
                  <BrowserMetricLabel>Current view</BrowserMetricLabel>
                  <BrowserMetricValue>{currentViewLabel}</BrowserMetricValue>
                </BrowserMetric>
                <BrowserMetric>
                  <BrowserMetricLabel>Edit mode</BrowserMetricLabel>
                  <BrowserMetricValue>{editable ? "Live draft editing" : "Read only"}</BrowserMetricValue>
                </BrowserMetric>
                <BrowserMetric>
                  <BrowserMetricLabel>Current directory</BrowserMetricLabel>
                  <BrowserMetricValue>{currentPath}</BrowserMetricValue>
                </BrowserMetric>
              </BrowserBanner>

              <FileStudio>
                <BrowserPanel>
                  <CardHeader>
                    <div>
                      <Typography.Heading component="h3" size="S">
                        Directory slice
                      </Typography.Heading>
                      <Typography.Body color="secondary" component="p">
                        {currentPath}
                      </Typography.Body>
                    </div>
                    <InlineActions>
                      <Button
                        size="medium"
                        variant="secondary-fill"
                        disabled={currentPath === "/"}
                        onClick={() => {
                          setCurrentPath(parentPath(currentPath));
                          setSelectedPath("");
                        }}
                      >
                        Up
                      </Button>
                    </InlineActions>
                  </CardHeader>

                  <Field>
                    View
                    <Select
                      value={browserView}
                      onChange={(event) => {
                        setBrowserView(event.target.value as AFSWorkspaceView);
                        setCurrentPath("/");
                        setSelectedPath("");
                      }}
                    >
                      {browserViewOptions(workspace).map((option) => (
                        <option key={option.value} value={option.value}>
                          {option.label}
                        </option>
                      ))}
                    </Select>
                  </Field>

                  {treeQuery.isLoading ? (
                    <PanelNote>Loading directory contents...</PanelNote>
                  ) : null}

                  {treeQuery.isError ? <PanelNote>Unable to load this directory.</PanelNote> : null}

                  <FileList style={{ marginTop: 14 }}>
                    {browserItems.map((item) => (
                      <FileButton
                        key={item.path}
                        $active={item.path === selectedPath}
                        onClick={() => {
                          if (item.kind === "dir") {
                            setCurrentPath(item.path);
                            setSelectedPath("");
                            return;
                          }
                          setSelectedPath(item.path);
                        }}
                      >
                        <FileButtonHeader>
                          <Typography.Body component="strong">{item.name}</Typography.Body>
                          <FileKind>{item.kind}</FileKind>
                        </FileButtonHeader>
                        <Typography.Body color="secondary" component="p">
                          {item.kind !== "dir" ? `${formatItemSize(item.size)} · ` : ""}
                          {item.modifiedAt
                            ? new Date(item.modifiedAt).toLocaleString()
                            : "No modification timestamp"}
                        </Typography.Body>
                      </FileButton>
                    ))}
                    {!treeQuery.isLoading && browserItems.length === 0 ? (
                      <PanelNote>This directory is empty.</PanelNote>
                    ) : null}
                  </FileList>
                </BrowserPanel>

                <EditorPanel>
                  {selectedPath === "" ? (
                    <PanelNote>Select a file to inspect its content.</PanelNote>
                  ) : selectedFileQuery.isLoading ? (
                    <PanelNote>Loading file content...</PanelNote>
                  ) : selectedFile == null ? (
                    <PanelNote>Select a file to inspect its content.</PanelNote>
                  ) : selectedFile.binary ? (
                    <EditorStack>
                      <CardHeader>
                        <div>
                          <Typography.Heading component="h3" size="S">
                            {selectedFile.path}
                          </Typography.Heading>
                          <Typography.Body color="secondary" component="p">
                            Binary asset
                          </Typography.Body>
                        </div>
                      </CardHeader>
                      <Typography.Body color="secondary" component="p">
                        This item looks binary, so the studio is showing metadata instead of raw
                        content.
                      </Typography.Body>
                      <MetaRow>
                        <Tag>{selectedFile.language}</Tag>
                        <Tag>{formatItemSize(selectedFile.size)}</Tag>
                        <Tag>{selectedFile.kind}</Tag>
                      </MetaRow>
                    </EditorStack>
                  ) : editable ? (
                    <form
                      onSubmit={(event) => {
                        event.preventDefault();
                        updateFile.mutate({
                          databaseId: selectedDatabaseId,
                          workspaceId: workspace.id,
                          path: selectedFile.path,
                          content: draftContent,
                        });
                      }}
                    >
                      <EditorStack>
                        <CardHeader>
                          <div>
                            <Typography.Heading component="h3" size="S">
                              {selectedFile.path}
                            </Typography.Heading>
                            <Typography.Body color="secondary" component="p">
                              {selectedFile.language}
                            </Typography.Body>
                          </div>
                          <ToneChip $tone={workspace.draftState}>
                            {draftStateLabel(workspace.draftState)}
                          </ToneChip>
                        </CardHeader>
                        <EditorArea
                          value={draftContent}
                          onChange={(event) => setDraftContent(event.target.value)}
                        />
                        <InlineActions>
                          <Button size="medium" type="submit" disabled={updateFile.isPending}>
                            Save file
                          </Button>
                          <EditorNote>
                            Writes are limited to the working copy so the saved head stays explicit.
                          </EditorNote>
                        </InlineActions>
                      </EditorStack>
                    </form>
                  ) : (
                    <EditorStack>
                      <CardHeader>
                        <div>
                          <Typography.Heading component="h3" size="S">
                            {selectedFile.path}
                          </Typography.Heading>
                          <Typography.Body color="secondary" component="p">
                            {selectedFile.language}
                          </Typography.Body>
                        </div>
                        <MetaRow>
                          <Tag>{selectedFile.kind}</Tag>
                          <Tag>{formatItemSize(selectedFile.size)}</Tag>
                        </MetaRow>
                      </CardHeader>
                      <EditorArea
                        readOnly
                        value={selectedFile.content ?? selectedFile.target ?? ""}
                      />
                    </EditorStack>
                  )}
                </EditorPanel>
              </FileStudio>
            </SectionCard>
          </SectionGrid>

        </>
      ) : null}

      {tab === "activity" ? (
        <SectionGrid>
          <SectionCard $span={12}>
            <SectionHeader>
              <SectionTitle
                title="Workspace activity"
                body="Per-workspace activity should be sortable and actionable so operators can jump straight to the relevant surface."
              />
            </SectionHeader>
            <ActivityTable
              rows={workspace.activity}
              onOpenActivity={(event) => setStudioTab(activityDestinationTab(event))}
            />
          </SectionCard>
        </SectionGrid>
      ) : null}
    </PageStack>
  );
}

function defaultBrowserView(workspace: AFSWorkspaceDetail): AFSWorkspaceView {
  if (workspace.capabilities.browseWorkingCopy) {
    return "working-copy";
  }
  return "head";
}

function browserViewOptions(workspace: AFSWorkspaceDetail) {
  const options: Array<{ value: AFSWorkspaceView; label: string }> = [];

  if (workspace.capabilities.browseWorkingCopy) {
    options.push({ value: "working-copy", label: "Working copy" });
  }
  if (workspace.capabilities.browseHead) {
    options.push({ value: "head", label: "Saved head" });
  }
  if (workspace.capabilities.browseCheckpoints) {
    for (const savepoint of workspace.savepoints) {
      options.push({
        value: `checkpoint:${savepoint.id}`,
        label: `Checkpoint: ${savepoint.name}`,
      });
    }
  }

  return options;
}

function viewLabel(view: AFSWorkspaceView, workspace: AFSWorkspaceDetail | undefined) {
  if (view === "working-copy") {
    return "Working copy";
  }
  if (view === "head") {
    return "Saved head";
  }
  const checkpointId = view.replace(/^checkpoint:/, "");
  const checkpoint = workspace?.savepoints.find((savepoint) => savepoint.id === checkpointId);
  return checkpoint == null ? "Checkpoint" : `Checkpoint: ${checkpoint.name}`;
}

function parentPath(value: string) {
  if (value === "/" || value === "") {
    return "/";
  }
  const parts = value.split("/").filter(Boolean);
  parts.pop();
  return parts.length === 0 ? "/" : `/${parts.join("/")}`;
}

function formatItemSize(size: number) {
  if (size === 0) {
    return "0 KB";
  }
  return formatBytes(size);
}

function draftStateLabel(state: AFSDraftState) {
  return state === "dirty" ? "Draft dirty" : "Draft clean";
}

function activityDestinationTab(event: AFSActivityEvent): StudioTab {
  if (event.scope === "savepoint") {
    return "checkpoints";
  }
  if (event.scope === "file") {
    return "files";
  }
  if (event.scope === "workspace") {
    return "overview";
  }
  return "activity";
}

const BrowserBanner = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin-bottom: 18px;

  @media (max-width: 860px) {
    grid-template-columns: 1fr;
  }
`;

const StudioNavRow = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 24px;
`;

const BreadcrumbButton = styled.button`
  border: none;
  background: transparent;
  padding: 0;
  color: var(--afs-ink);
  font: inherit;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;

  &:hover {
    text-decoration: underline;
  }
`;

const BreadcrumbSeparator = styled.span`
  color: var(--afs-muted);
  font-size: 14px;
`;

const BreadcrumbCurrent = styled.span`
  color: var(--afs-muted);
  font-size: 14px;
  font-weight: 500;
`;

const TabsToolbar = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: center;

  @media (max-width: 860px) {
    flex-direction: column;
    align-items: stretch;
  }
`;

const DeleteWorkspaceButton = styled(Button)`
  && {
    background: ${({ theme }) => theme.semantic.color.background.danger500};
    border-color: ${({ theme }) => theme.semantic.color.background.danger500};
    color: ${({ theme }) => theme.semantic.color.text.inverse};
  }

  &&:hover:not(:disabled),
  &&:focus-visible:not(:disabled) {
    background: ${({ theme }) => theme.semantic.color.background.danger600};
    border-color: ${({ theme }) => theme.semantic.color.background.danger600};
    color: ${({ theme }) => theme.semantic.color.text.inverse};
  }
`;

const StatusTable = styled.table`
  width: 100%;
  border-collapse: collapse;
`;

const StatusRow = styled.tr`
  border-top: 1px solid var(--afs-line);

  &:first-child {
    border-top: none;
  }
`;

const StatusLabel = styled.th`
  width: 220px;
  padding: 14px 0;
  color: var(--afs-muted);
  font-size: 13px;
  font-weight: 600;
  text-align: left;
  vertical-align: top;
`;

const StatusValue = styled.td`
  padding: 14px 0;
  color: var(--afs-ink);
  font-size: 14px;
  line-height: 1.5;
  text-align: left;
`;

const BrowserMetric = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 18px;
  padding: 14px 16px;
  background: rgba(255, 255, 255, 0.72);
`;

const BrowserMetricLabel = styled.div`
  color: var(--afs-muted);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const BrowserMetricValue = styled.div`
  margin-top: 7px;
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
  line-height: 1.45;
`;

const BrowserPanel = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 24px;
  padding: 18px;
  background: #fff;
`;

const PanelNote = styled.p`
  margin: 14px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const FileButtonHeader = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 10px;
  align-items: center;
`;

const FileKind = styled.span`
  display: inline-flex;
  align-items: center;
  padding: 4px 8px;
  border-radius: 999px;
  background: rgba(8, 6, 13, 0.08);
  color: var(--afs-muted);
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const EditorStack = styled.div`
  display: grid;
  gap: 16px;
`;

const EditorNote = styled.span`
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.5;
`;

const ActionStack = styled.div`
  display: grid;
  gap: 16px;
`;
