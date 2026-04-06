import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader, Typography } from "@redislabsdev/redis-ui-components";
import { useEffect, useMemo, useState } from "react";
import {
  CardHeader,
  EditorArea,
  EditorPanel,
  EventList,
  Field,
  FileButton,
  FileList,
  FileStudio,
  FormGrid,
  InlineActions,
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
import {
  useCreateSavepointMutation,
  useDeleteWorkspaceMutation,
  useRestoreSavepointMutation,
  useUpdateWorkspaceFileMutation,
  useWorkspace,
  useWorkspaceFileContent,
  useWorkspaceTree,
} from "../foundation/hooks/use-afs";
import type { AFSWorkspaceDetail, AFSWorkspaceView } from "../foundation/types/afs";

type StudioTab = "overview" | "files" | "activity";

export const Route = createFileRoute("/workspaces/$workspaceId")({
  component: WorkspaceStudioPage,
});

function WorkspaceStudioPage() {
  const navigate = useNavigate();
  const { workspaceId } = Route.useParams();
  const workspaceQuery = useWorkspace(workspaceId);
  const deleteWorkspace = useDeleteWorkspaceMutation();
  const updateFile = useUpdateWorkspaceFileMutation();
  const createSavepoint = useCreateSavepointMutation();
  const restoreSavepoint = useRestoreSavepointMutation();

  const [tab, setTab] = useState<StudioTab>("overview");
  const [browserView, setBrowserView] = useState<AFSWorkspaceView>("head");
  const [currentPath, setCurrentPath] = useState("/");
  const [selectedPath, setSelectedPath] = useState("");
  const [draftContent, setDraftContent] = useState("");
  const [savepointName, setSavepointName] = useState("");
  const [savepointNote, setSavepointNote] = useState("");
  const [rollbackTarget, setRollbackTarget] = useState("");

  const workspace = workspaceQuery.data;
  const activeSavepoint =
    workspace?.savepoints.find((savepoint) => savepoint.id === workspace.headSavepointId) ?? null;

  useEffect(() => {
    if (workspace == null) {
      setBrowserView("head");
      setCurrentPath("/");
      setSelectedPath("");
      return;
    }

    const defaultView = defaultBrowserView(workspace);
    const allowedViews = browserViewOptions(workspace).map((item) => item.value);

    setBrowserView((current) =>
      allowedViews.includes(current) ? current : defaultView,
    );
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
      workspaceId,
      view: browserView,
      path: currentPath,
      depth: 1,
    },
    workspace != null,
  );

  const selectedFileQuery = useWorkspaceFileContent(
    {
      workspaceId,
      view: browserView,
      path: selectedPath,
    },
    workspace != null && selectedPath !== "",
  );

  useEffect(() => {
    const file = selectedFileQuery.data;
    setDraftContent(file?.content ?? file?.target ?? "");
  }, [selectedFileQuery.data?.revision, selectedFileQuery.data?.target]);

  const browserItems = treeQuery.data?.items ?? [];
  const selectedFile = selectedFileQuery.data;
  const editable =
    workspace?.capabilities.editWorkingCopy === true &&
    browserView === "working-copy" &&
    selectedFile?.kind === "file";

  const currentViewLabel = useMemo(() => viewLabel(browserView, workspace), [browserView, workspace]);

  if (workspaceQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  if (workspace == null) {
    throw new Error("Workspace not found.");
  }

  return (
    <PageStack>
      <SectionGrid>
        <SectionCard $span={12}>
          <SectionHeader>
            <SectionTitle
              eyebrow="Studio"
              title={workspace.name}
              body={workspace.description || "AFS workspace studio and checkpoint browser."}
            />
            <InlineActions>
              <ToneChip $tone={workspace.status}>{workspace.status}</ToneChip>
              <ToneChip $tone={workspace.source}>{workspace.source}</ToneChip>
              <Button
                size="medium"
                variant="secondary-fill"
                disabled={deleteWorkspace.isPending}
                onClick={() =>
                  deleteWorkspace.mutate(workspace.id, {
                    onSuccess: () => {
                      void navigate({ to: "/workspaces" });
                    },
                  })
                }
              >
                Delete workspace
              </Button>
            </InlineActions>
          </SectionHeader>
          <InlineActions>
            <Tag>{workspace.databaseName}</Tag>
            <Tag>{workspace.redisKey}</Tag>
            <Tag>{workspace.cloudAccount}</Tag>
            {workspace.region ? <Tag>{workspace.region}</Tag> : null}
            <Tag>{workspace.fileCount} files</Tag>
            <Tag>{workspace.checkpointCount} checkpoints</Tag>
          </InlineActions>
          <div style={{ marginTop: 20 }}>
            <Tabs>
              <TabButton $active={tab === "overview"} onClick={() => setTab("overview")}>
                Overview
              </TabButton>
              <TabButton $active={tab === "files"} onClick={() => setTab("files")}>
                Files
              </TabButton>
              <TabButton $active={tab === "activity"} onClick={() => setTab("activity")}>
                Activity
              </TabButton>
            </Tabs>
          </div>
        </SectionCard>
      </SectionGrid>

      {tab === "overview" ? (
        <SectionGrid>
          <SectionCard $span={5}>
            <SectionHeader>
              <SectionTitle
                title="Workspace state"
                body="The hosted control plane shows canonical Redis-backed state, plus capability flags that tell the UI whether live working-copy actions are available."
              />
            </SectionHeader>
            <CardHeader>
              <div>
                <Typography.Heading component="h3" size="S">
                  {workspace.name}
                </Typography.Heading>
                <Typography.Body color="secondary" component="p">
                  {workspace.description || "No description provided."}
                </Typography.Body>
              </div>
              <InlineActions>
                <ToneChip $tone={workspace.draftState}>{workspace.draftState}</ToneChip>
              </InlineActions>
            </CardHeader>
            <Typography.Body color="secondary" component="p">
              Head checkpoint: {activeSavepoint?.name ?? "n/a"}
            </Typography.Body>
            <Typography.Body color="secondary" component="p">
              Updated {new Date(workspace.updatedAt).toLocaleString()}
            </Typography.Body>
            <InlineActions style={{ marginTop: 14 }}>
              {workspace.tags.map((tag) => (
                <Tag key={tag}>{tag}</Tag>
              ))}
            </InlineActions>
            <InlineActions style={{ marginTop: 14 }}>
              <Tag>{workspace.capabilities.browseHead ? "Head browser" : "No head browser"}</Tag>
              <Tag>
                {workspace.capabilities.browseWorkingCopy
                  ? "Working copy visible"
                  : "Working copy requires connector"}
              </Tag>
            </InlineActions>
          </SectionCard>

          <SectionCard $span={7}>
            <SectionHeader>
              <SectionTitle
                title="Checkpoint history"
                body="Checkpoints are immutable. Restoring through the hosted control plane moves the canonical workspace head in Redis without assuming access to a local tree."
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
                    <InlineActions style={{ marginTop: 10 }}>
                      <Tag>{savepoint.fileCount} files</Tag>
                      <Tag>{savepoint.sizeLabel}</Tag>
                      <Tag>{new Date(savepoint.createdAt).toLocaleString()}</Tag>
                      {savepoint.id === workspace.headSavepointId ? <Tag>Current head</Tag> : null}
                    </InlineActions>
                  </div>
                  <InlineActions>
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
      ) : null}

      {tab === "files" ? (
        <SectionGrid>
          <SectionCard $span={12}>
            <SectionHeader>
              <SectionTitle
                title="Filesystem browser"
                body="Directory slices and file content now load lazily. In hosted mode you browse canonical saved state first, then layer in live working-copy access later via capabilities."
              />
            </SectionHeader>

            <SectionGrid style={{ marginBottom: 16 }}>
              <SectionCard $span={4}>
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
              </SectionCard>
              <SectionCard $span={8}>
                <SectionTitle
                  title={currentViewLabel}
                  body={
                    workspace.capabilities.browseWorkingCopy
                      ? "Working copy and saved checkpoints are both available in this workspace."
                      : "This control plane can browse saved head and checkpoints. Live working-copy access is reserved for a future connector."
                  }
                />
              </SectionCard>
            </SectionGrid>

            <FileStudio>
              <div>
                <CardHeader>
                  <div>
                    <Typography.Heading component="h3" size="S">
                      Current directory
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

                {treeQuery.isLoading ? (
                  <Typography.Body color="secondary" component="p">
                    Loading directory contents...
                  </Typography.Body>
                ) : null}

                {treeQuery.isError ? (
                  <Typography.Body color="secondary" component="p">
                    Unable to load this directory.
                  </Typography.Body>
                ) : null}

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
                      <Typography.Body component="strong">{item.name}</Typography.Body>
                      <Typography.Body color="secondary" component="p">
                        {item.kind}
                        {item.kind !== "dir" ? ` · ${formatItemSize(item.size)}` : ""}
                        {item.modifiedAt ? ` · ${new Date(item.modifiedAt).toLocaleString()}` : ""}
                      </Typography.Body>
                    </FileButton>
                  ))}
                  {!treeQuery.isLoading && browserItems.length === 0 ? (
                    <Typography.Body color="secondary" component="p">
                      This directory is empty.
                    </Typography.Body>
                  ) : null}
                </FileList>
              </div>

              <EditorPanel>
                {selectedPath === "" ? (
                  <Typography.Body color="secondary" component="p">
                    Select a file to inspect its content.
                  </Typography.Body>
                ) : selectedFileQuery.isLoading ? (
                  <Typography.Body color="secondary" component="p">
                    Loading file content...
                  </Typography.Body>
                ) : selectedFile == null ? (
                  <Typography.Body color="secondary" component="p">
                    Select a file to inspect its content.
                  </Typography.Body>
                ) : selectedFile.binary ? (
                  <div style={{ display: "grid", gap: 12 }}>
                    <Typography.Heading component="h3" size="S">
                      {selectedFile.path}
                    </Typography.Heading>
                    <Typography.Body color="secondary" component="p">
                      This item looks binary, so the hosted browser is showing metadata only.
                    </Typography.Body>
                    <InlineActions>
                      <Tag>{selectedFile.language}</Tag>
                      <Tag>{formatItemSize(selectedFile.size)}</Tag>
                    </InlineActions>
                  </div>
                ) : editable ? (
                  <form
                    onSubmit={(event) => {
                      event.preventDefault();
                      updateFile.mutate({
                        workspaceId: workspace.id,
                        path: selectedFile.path,
                        content: draftContent,
                      });
                    }}
                    style={{ display: "grid", gap: 16 }}
                  >
                    <CardHeader>
                      <div>
                        <Typography.Heading component="h3" size="S">
                          {selectedFile.path}
                        </Typography.Heading>
                        <Typography.Body color="secondary" component="p">
                          {selectedFile.language}
                        </Typography.Body>
                      </div>
                      <ToneChip $tone={workspace.draftState}>{workspace.draftState}</ToneChip>
                    </CardHeader>
                    <EditorArea
                      value={draftContent}
                      onChange={(event) => setDraftContent(event.target.value)}
                    />
                    <InlineActions>
                      <Button size="medium" type="submit" disabled={updateFile.isPending}>
                        Save file
                      </Button>
                    </InlineActions>
                  </form>
                ) : (
                  <div style={{ display: "grid", gap: 16 }}>
                    <CardHeader>
                      <div>
                        <Typography.Heading component="h3" size="S">
                          {selectedFile.path}
                        </Typography.Heading>
                        <Typography.Body color="secondary" component="p">
                          {selectedFile.language}
                        </Typography.Body>
                      </div>
                      <InlineActions>
                        <Tag>{selectedFile.kind}</Tag>
                        <Tag>{formatItemSize(selectedFile.size)}</Tag>
                      </InlineActions>
                    </CardHeader>
                    <EditorArea readOnly value={selectedFile.content ?? selectedFile.target ?? ""} />
                  </div>
                )}
              </EditorPanel>
            </FileStudio>

            <SectionGrid style={{ marginTop: 16 }}>
              <SectionCard $span={5}>
                <SectionTitle
                  title="Current view"
                  body="The workspace browser keeps canonical saved state and live working-copy capabilities distinct."
                />
                <InlineActions style={{ marginTop: 14 }}>
                  <Tag>{currentViewLabel}</Tag>
                  <ToneChip $tone={workspace.draftState}>{workspace.draftState}</ToneChip>
                  <Tag>{workspace.fileCount} files</Tag>
                </InlineActions>
              </SectionCard>

              <SectionCard $span={7}>
                <SectionTitle
                  title="Checkpoint actions"
                  body="Restore any saved checkpoint now. Creation remains gated by capability flags so the hosted UI does not assume a connected local working copy."
                />
                <div style={{ marginTop: 16, display: "grid", gap: 14 }}>
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
                        !workspace.capabilities.restoreCheckpoint || restoreSavepoint.isPending
                      }
                      onClick={() =>
                        restoreSavepoint.mutate({
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
                    <Typography.Body color="secondary" component="p">
                      Checkpoint creation is unavailable in this transport because it needs a connected working copy rather than canonical saved state alone.
                    </Typography.Body>
                  )}
                </div>
              </SectionCard>
            </SectionGrid>
          </SectionCard>
        </SectionGrid>
      ) : null}

      {tab === "activity" ? (
        <SectionGrid>
          <SectionCard $span={12}>
            <SectionTitle
              title="Workspace activity"
              body="Per-workspace activity now comes from the control-plane audit feed and stays separate from file browsing."
            />
            <div style={{ marginTop: 16 }}>
              <EventList events={workspace.activity} />
            </div>
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
  if (size >= 1024 * 1024) {
    return `${(size / (1024 * 1024)).toFixed(1)} MB`;
  }
  if (size >= 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  return `${size} B`;
}
