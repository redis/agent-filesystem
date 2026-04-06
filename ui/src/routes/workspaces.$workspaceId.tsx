import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader, Typography } from "@redislabsdev/redis-ui-components";
import { useEffect, useMemo, useState } from "react";
import styled from "styled-components";
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
  HeroBody,
  HeroCard,
  HeroLayout,
  HeroMetaGrid,
  InlineActions,
  MetaItem,
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
import {
  useCreateSavepointMutation,
  useDeleteWorkspaceMutation,
  useRestoreSavepointMutation,
  useUpdateWorkspaceFileMutation,
  useWorkspace,
  useWorkspaceFileContent,
  useWorkspaceTree,
} from "../foundation/hooks/use-afs";
import type {
  AFSDraftState,
  AFSWorkspaceDetail,
  AFSWorkspaceSource,
  AFSWorkspaceStatus,
  AFSWorkspaceView,
} from "../foundation/types/afs";

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
  const capabilityCards = workspace ? buildCapabilityCards(workspace) : [];
  const activityActors = useMemo(
    () => new Set((workspace?.activity ?? []).map((event) => event.actor)).size,
    [workspace],
  );
  const lastActivity = workspace?.activity[0] ?? null;
  const tabCopy = describeTab(tab);

  if (workspaceQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  if (workspace == null) {
    throw new Error("Workspace not found.");
  }

  return (
    <PageStack>
      <HeroCard>
        <HeroLayout>
          <HeroBody>
            <SectionTitle
              eyebrow="Workspace Studio"
              title={workspace.name}
              body={workspace.description || "AFS workspace studio for browsing, editing, and checkpoint recovery."}
            />
            <InlineActions>
              <ToneChip $tone={workspace.status}>{statusLabel(workspace.status)}</ToneChip>
              <ToneChip $tone={workspace.draftState}>
                {draftStateLabel(workspace.draftState)}
              </ToneChip>
              <ToneChip $tone={workspace.source}>{sourceLabel(workspace.source)}</ToneChip>
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
            <MetaRow>
              <Tag>{workspace.databaseName}</Tag>
              <Tag>{workspace.redisKey}</Tag>
              <Tag>{workspace.cloudAccount}</Tag>
              {workspace.region ? <Tag>{workspace.region}</Tag> : null}
              {workspace.mountedPath ? <Tag>{workspace.mountedPath}</Tag> : null}
            </MetaRow>
            <SummaryStrip>
              <SummaryPanel>
                <SummaryLabel>Files</SummaryLabel>
                <SummaryValue>{workspace.fileCount}</SummaryValue>
                <SummaryBody>{workspace.folderCount} folders currently addressable in this studio.</SummaryBody>
              </SummaryPanel>
              <SummaryPanel>
                <SummaryLabel>Footprint</SummaryLabel>
                <SummaryValue>{formatBytes(workspace.totalBytes)}</SummaryValue>
                <SummaryBody>Content size stays visible so filesystem scale is never abstract.</SummaryBody>
              </SummaryPanel>
              <SummaryPanel>
                <SummaryLabel>Recovery</SummaryLabel>
                <SummaryValue>{workspace.checkpointCount}</SummaryValue>
                <SummaryBody>Checkpoints are nearby because safe rollback is part of editing.</SummaryBody>
              </SummaryPanel>
            </SummaryStrip>
          </HeroBody>

          <HeroMetaGrid>
            <MetaItem>
              <MetaLabel>Current head</MetaLabel>
              <MetaValue>{activeSavepoint?.name ?? "No checkpoint yet"}</MetaValue>
              <MetaBody>
                {activeSavepoint == null
                  ? "This workspace has not recorded a checkpoint yet."
                  : `Created ${new Date(activeSavepoint.createdAt).toLocaleString()} by ${activeSavepoint.author}.`}
              </MetaBody>
            </MetaItem>
            <MetaItem>
              <MetaLabel>Mutable surface</MetaLabel>
              <MetaValue>
                {workspace.capabilities.editWorkingCopy ? "Working copy editable" : "Read-only views"}
              </MetaValue>
              <MetaBody>
                {workspace.capabilities.editWorkingCopy
                  ? "This studio can commit draft edits directly into the working copy."
                  : "The current transport supports browsing and restore, but not direct edits."}
              </MetaBody>
            </MetaItem>
            <MetaItem>
              <MetaLabel>Checkpoint restore</MetaLabel>
              <MetaValue>
                {workspace.capabilities.restoreCheckpoint ? "Available" : "Unavailable"}
              </MetaValue>
              <MetaBody>
                Restore controls are surfaced with the history so recovery never feels hidden.
              </MetaBody>
            </MetaItem>
            <MetaItem>
              <MetaLabel>Last updated</MetaLabel>
              <MetaValue>{new Date(workspace.updatedAt).toLocaleString()}</MetaValue>
              <MetaBody>The studio should always make the most recent canonical mutation obvious.</MetaBody>
            </MetaItem>
          </HeroMetaGrid>
        </HeroLayout>
      </HeroCard>

      <SectionGrid>
        <SectionCard $span={12}>
          <SectionHeader>
            <SectionTitle eyebrow="Studio Views" title={tabCopy.title} body={tabCopy.body} />
          </SectionHeader>
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
        </SectionCard>
      </SectionGrid>

      {tab === "overview" ? (
        <>
          <SectionGrid>
            <SectionCard $span={4}>
              <SectionHeader>
                <SectionTitle
                  title="Workspace state"
                  body="These are the signals an operator needs before browsing files or restoring a checkpoint."
                />
              </SectionHeader>
              <StateGrid>
                <StateCard>
                  <StateLabel>Status</StateLabel>
                  <StateValue>{statusLabel(workspace.status)}</StateValue>
                </StateCard>
                <StateCard>
                  <StateLabel>Draft</StateLabel>
                  <StateValue>{draftStateLabel(workspace.draftState)}</StateValue>
                </StateCard>
                <StateCard>
                  <StateLabel>Source</StateLabel>
                  <StateValue>{sourceLabel(workspace.source)}</StateValue>
                </StateCard>
                <StateCard>
                  <StateLabel>Head savepoint</StateLabel>
                  <StateValue>{activeSavepoint?.name ?? "None"}</StateValue>
                </StateCard>
              </StateGrid>
              <MetaRow>
                {workspace.tags.map((tag) => (
                  <Tag key={tag}>{tag}</Tag>
                ))}
              </MetaRow>
            </SectionCard>

            <SectionCard $span={8}>
              <SectionHeader>
                <SectionTitle
                  title="Checkpoint history"
                  body="Recovery points are immutable. The studio should make it easy to compare them and promote one back to head."
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

          <SectionGrid>
            <SectionCard $span={7}>
              <SectionHeader>
                <SectionTitle
                  title="Capability surface"
                  body="Capability flags tell the UI which parts of the studio should present as live controls versus view-only instrumentation."
                />
              </SectionHeader>
              <CapabilityGrid>
                {capabilityCards.map((capability) => (
                  <CapabilityCard key={capability.title} $enabled={capability.enabled}>
                    <CapabilityStatus $enabled={capability.enabled}>
                      {capability.enabled ? "Enabled" : "Unavailable"}
                    </CapabilityStatus>
                    <CapabilityTitle>{capability.title}</CapabilityTitle>
                    <CapabilityBody>{capability.body}</CapabilityBody>
                  </CapabilityCard>
                ))}
              </CapabilityGrid>
            </SectionCard>

            <SectionCard $span={5}>
              <SectionHeader>
                <SectionTitle
                  title="Recent motion"
                  body="A short audit preview keeps recent workspace activity adjacent to state and checkpoint context."
                />
              </SectionHeader>
              <EventList events={workspace.activity.slice(0, 3)} />
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

          <SectionGrid>
            <SectionCard $span={5}>
              <SectionHeader>
                <SectionTitle
                  title="Current view"
                  body="The browser keeps working copy, saved head, and checkpoints visually distinct so operators know what they are looking at."
                />
              </SectionHeader>
              <StateGrid>
                <StateCard>
                  <StateLabel>View</StateLabel>
                  <StateValue>{currentViewLabel}</StateValue>
                </StateCard>
                <StateCard>
                  <StateLabel>Draft</StateLabel>
                  <StateValue>{draftStateLabel(workspace.draftState)}</StateValue>
                </StateCard>
                <StateCard>
                  <StateLabel>Files</StateLabel>
                  <StateValue>{workspace.fileCount}</StateValue>
                </StateCard>
                <StateCard>
                  <StateLabel>Editability</StateLabel>
                  <StateValue>{editable ? "Writable" : "Protected"}</StateValue>
                </StateCard>
              </StateGrid>
            </SectionCard>

            <SectionCard $span={7}>
              <SectionHeader>
                <SectionTitle
                  title="Checkpoint actions"
                  body="Restore stays close to the browser because operators often need to compare state before and after intervention."
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

      {tab === "activity" ? (
        <SectionGrid>
          <SectionCard $span={8}>
            <SectionHeader>
              <SectionTitle
                title="Workspace activity"
                body="Per-workspace activity reads as an audit lane for file edits, checkpoint creation, and imports."
              />
            </SectionHeader>
            <EventList events={workspace.activity} />
          </SectionCard>
          <SectionCard $span={4}>
            <SectionHeader>
              <SectionTitle
                title="Audit summary"
                body="A compact readout helps the operator see how lively this workspace has been without parsing the whole feed."
              />
            </SectionHeader>
            <StateGrid>
              <StateCard>
                <StateLabel>Events</StateLabel>
                <StateValue>{workspace.activity.length}</StateValue>
              </StateCard>
              <StateCard>
                <StateLabel>Actors</StateLabel>
                <StateValue>{activityActors}</StateValue>
              </StateCard>
              <StateCard>
                <StateLabel>Latest event</StateLabel>
                <StateValue>{lastActivity?.title ?? "None yet"}</StateValue>
              </StateCard>
              <StateCard>
                <StateLabel>Latest timestamp</StateLabel>
                <StateValue>
                  {lastActivity ? new Date(lastActivity.createdAt).toLocaleString() : "n/a"}
                </StateValue>
              </StateCard>
            </StateGrid>
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

function statusLabel(status: AFSWorkspaceStatus) {
  if (status === "healthy") return "Healthy";
  if (status === "syncing") return "Syncing";
  return "Attention";
}

function draftStateLabel(state: AFSDraftState) {
  return state === "dirty" ? "Draft dirty" : "Draft clean";
}

function sourceLabel(source: AFSWorkspaceSource) {
  if (source === "git-import") return "Git import";
  if (source === "cloud-import") return "Cloud import";
  return "Blank";
}

function buildCapabilityCards(workspace: AFSWorkspaceDetail) {
  return [
    {
      title: "Browse saved head",
      enabled: workspace.capabilities.browseHead,
      body: "View the canonical snapshot that currently defines the workspace head.",
    },
    {
      title: "Browse checkpoints",
      enabled: workspace.capabilities.browseCheckpoints,
      body: "Inspect immutable recovery points from earlier versions of the workspace.",
    },
    {
      title: "Browse working copy",
      enabled: workspace.capabilities.browseWorkingCopy,
      body: "Inspect mutable draft state without collapsing it into the canonical head.",
    },
    {
      title: "Edit working copy",
      enabled: workspace.capabilities.editWorkingCopy,
      body: "Write draft changes directly inside the studio when the transport allows it.",
    },
    {
      title: "Create checkpoint",
      enabled: workspace.capabilities.createCheckpoint,
      body: "Capture the current workspace as a named recovery point for future restore.",
    },
    {
      title: "Restore checkpoint",
      enabled: workspace.capabilities.restoreCheckpoint,
      body: "Promote a saved checkpoint back to head when you need to recover confidently.",
    },
  ];
}

function describeTab(tab: StudioTab) {
  if (tab === "files") {
    return {
      title: "Filesystem browser and editor",
      body: "File browsing should feel instrumented, with clear separation between saved views and mutable draft edits.",
    };
  }

  if (tab === "activity") {
    return {
      title: "Workspace audit lane",
      body: "Activity belongs in the studio because file edits, imports, and checkpoint work need a shared timeline.",
    };
  }

  return {
    title: "Workspace state and recovery",
    body: "Use the overview to understand current state, capability flags, and the checkpoint history you can safely restore.",
  };
}

const SummaryStrip = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(3, minmax(0, 1fr));

  @media (max-width: 860px) {
    grid-template-columns: 1fr;
  }
`;

const SummaryPanel = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 20px;
  padding: 16px;
  background: rgba(255, 255, 255, 0.76);
`;

const SummaryLabel = styled.div`
  color: var(--afs-muted);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const SummaryValue = styled.div`
  margin-top: 8px;
  color: var(--afs-ink);
  font-size: 1.25rem;
  font-weight: 700;
  line-height: 1.35;
`;

const SummaryBody = styled.p`
  margin: 8px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const MetaLabel = styled.span`
  color: var(--afs-muted);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const MetaValue = styled.span`
  color: var(--afs-ink);
  font-size: 1.1rem;
  font-weight: 700;
  line-height: 1.4;
`;

const MetaBody = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const StateGrid = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(2, minmax(0, 1fr));

  @media (max-width: 640px) {
    grid-template-columns: 1fr;
  }
`;

const StateCard = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 18px;
  padding: 16px;
  background: rgba(255, 255, 255, 0.74);
`;

const StateLabel = styled.div`
  color: var(--afs-muted);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const StateValue = styled.div`
  margin-top: 8px;
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
  line-height: 1.5;
`;

const CapabilityGrid = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(2, minmax(0, 1fr));

  @media (max-width: 720px) {
    grid-template-columns: 1fr;
  }
`;

const CapabilityCard = styled.div<{ $enabled: boolean }>`
  border: 1px solid
    ${({ $enabled }) => ($enabled ? "rgba(71, 191, 255, 0.18)" : "var(--afs-line)")};
  border-radius: 20px;
  padding: 16px;
  background: ${({ $enabled }) =>
    $enabled ? "rgba(71, 191, 255, 0.08)" : "rgba(255, 255, 255, 0.72)"};
`;

const CapabilityStatus = styled.div<{ $enabled: boolean }>`
  color: ${({ $enabled }) => ($enabled ? "#095b8a" : "var(--afs-muted)")};
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const CapabilityTitle = styled.div`
  margin-top: 10px;
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
`;

const CapabilityBody = styled.p`
  margin: 8px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const BrowserBanner = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin-bottom: 18px;

  @media (max-width: 860px) {
    grid-template-columns: 1fr;
  }
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
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.84), rgba(243, 246, 251, 0.92)),
    rgba(255, 255, 255, 0.8);
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
