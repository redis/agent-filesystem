import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader, Typography } from "@redislabsdev/redis-ui-components";
import { useEffect, useState } from "react";
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
} from "../components/raf-kit";
import {
  useCreateSavepointMutation,
  useDeleteWorkspaceMutation,
  useRestoreSavepointMutation,
  useUpdateWorkspaceFileMutation,
  useWorkspace,
} from "../foundation/hooks/use-raf";

type StudioTab = "overview" | "editor" | "activity";

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
  const [selectedFilePath, setSelectedFilePath] = useState("");
  const [draftContent, setDraftContent] = useState("");
  const [savepointName, setSavepointName] = useState("");
  const [savepointNote, setSavepointNote] = useState("");
  const [rollbackTarget, setRollbackTarget] = useState("");

  const workspace = workspaceQuery.data;
  const selectedFile = workspace?.files.find((file) => file.path === selectedFilePath) ?? null;
  const activeSavepoint =
    workspace?.savepoints.find((savepoint) => savepoint.id === workspace.headSavepointId) ?? null;

  useEffect(() => {
    if (workspace == null) {
      setSelectedFilePath("");
      return;
    }

    const firstPath = workspace.files[0]?.path ?? "";
    setSelectedFilePath((current) =>
      workspace.files.some((file) => file.path === current) ? current : firstPath,
    );
    setRollbackTarget((current) =>
      workspace.savepoints.some((savepoint) => savepoint.id === current)
        ? current
        : workspace.headSavepointId,
    );
  }, [workspace]);

  useEffect(() => {
    setDraftContent(selectedFile?.content ?? "");
  }, [selectedFile]);

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
              body={workspace.description}
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
            <Tag>{workspace.region}</Tag>
            <Tag>{workspace.mountedPath}</Tag>
            <Tag>{workspace.files.length} files</Tag>
          </InlineActions>
          <div style={{ marginTop: 20 }}>
            <Tabs>
              <TabButton $active={tab === "overview"} onClick={() => setTab("overview")}>
                Overview
              </TabButton>
              <TabButton $active={tab === "editor"} onClick={() => setTab("editor")}>
                Browser + Editor
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
                body="AFS now tracks one live working copy and one checkpoint timeline per workspace."
              />
            </SectionHeader>
            <CardHeader>
              <div>
                <Typography.Heading component="h3" size="S">
                  {workspace.name}
                </Typography.Heading>
                <Typography.Body color="secondary" component="p">
                  {workspace.description}
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
          </SectionCard>

          <SectionCard $span={7}>
            <SectionHeader>
              <SectionTitle
                title="Checkpoint history"
                body="Each checkpoint is immutable. Restoring rematerializes the workspace from Redis."
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
                        restoreSavepoint.isPending || savepoint.id === workspace.headSavepointId
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

      {tab === "editor" ? (
        <SectionGrid>
          <SectionCard $span={12}>
            <SectionHeader>
              <SectionTitle
                title="Browser + editor"
                body="Browse the materialized workspace, edit draft files, and capture checkpoints from the current working tree."
              />
            </SectionHeader>

            <FileStudio>
              <div>
                <Typography.Heading component="h3" size="S">
                  Workspace files
                </Typography.Heading>
                <FileList style={{ marginTop: 14 }}>
                  {workspace.files.map((file) => (
                    <FileButton
                      key={file.path}
                      $active={file.path === selectedFilePath}
                      onClick={() => setSelectedFilePath(file.path)}
                    >
                      <Typography.Body component="strong">{file.path}</Typography.Body>
                      <Typography.Body color="secondary" component="p">
                        {file.language} · {new Date(file.modifiedAt).toLocaleString()}
                      </Typography.Body>
                    </FileButton>
                  ))}
                </FileList>
              </div>

              <EditorPanel>
                {selectedFile == null ? (
                  <Typography.Body color="secondary" component="p">
                    Select a file to start editing.
                  </Typography.Body>
                ) : (
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
                )}
              </EditorPanel>
            </FileStudio>

            <SectionGrid style={{ marginTop: 16 }}>
              <SectionCard $span={5}>
                <SectionTitle
                  title="Working copy"
                  body="Draft changes stay local until you capture a checkpoint."
                />
                <InlineActions style={{ marginTop: 14 }}>
                  <ToneChip $tone={workspace.draftState}>{workspace.draftState}</ToneChip>
                  <Tag>{activeSavepoint?.name ?? "n/a"}</Tag>
                  <Tag>{workspace.files.length} files</Tag>
                </InlineActions>
              </SectionCard>

              <SectionCard $span={7}>
                <SectionTitle
                  title="Checkpoint actions"
                  body="Capture the current tree or rematerialize from a saved checkpoint."
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
                      disabled={restoreSavepoint.isPending}
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
              body="Per-workspace events are already scoped for a future Redis-backed audit stream."
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
