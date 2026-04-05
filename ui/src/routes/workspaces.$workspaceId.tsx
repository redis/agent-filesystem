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
  SessionButton,
  SessionLayout,
  SessionRail,
  TabButton,
  Tabs,
  Tag,
  TextArea,
  TextInput,
  ToneChip,
  TwoColumnFields,
} from "../components/raf-kit";
import {
  useCreateSavepointMutation,
  useCreateSessionMutation,
  useDeleteSessionMutation,
  useDeleteWorkspaceMutation,
  useRollbackSessionMutation,
  useUpdateSessionFileMutation,
  useWorkspace,
} from "../foundation/hooks/use-raf";
import type { RAFSession } from "../foundation/types/raf";

type StudioTab = "sessions" | "editor" | "activity";

export const Route = createFileRoute("/workspaces/$workspaceId")({
  component: WorkspaceStudioPage,
});

function WorkspaceStudioPage() {
  const navigate = useNavigate();
  const { workspaceId } = Route.useParams();
  const workspaceQuery = useWorkspace(workspaceId);
  const createSession = useCreateSessionMutation();
  const deleteSession = useDeleteSessionMutation();
  const deleteWorkspace = useDeleteWorkspaceMutation();
  const updateFile = useUpdateSessionFileMutation();
  const createSavepoint = useCreateSavepointMutation();
  const rollbackSession = useRollbackSessionMutation();

  const [tab, setTab] = useState<StudioTab>("sessions");
  const [selectedSessionId, setSelectedSessionId] = useState("");
  const [selectedFilePath, setSelectedFilePath] = useState("");
  const [draftContent, setDraftContent] = useState("");
  const [sessionName, setSessionName] = useState("");
  const [sessionDescription, setSessionDescription] = useState("");
  const [importName, setImportName] = useState("");
  const [savepointName, setSavepointName] = useState("");
  const [savepointNote, setSavepointNote] = useState("");
  const [rollbackTarget, setRollbackTarget] = useState("");

  useEffect(() => {
    const workspace = workspaceQuery.data;
    if (workspace == null) {
      return;
    }

    const defaultSessionId = workspace.defaultSessionId || workspace.sessions[0]?.id || "";
    setSelectedSessionId((current) =>
      workspace.sessions.some((session) => session.id === current) ? current : defaultSessionId,
    );
  }, [workspaceQuery.data]);

  const workspace = workspaceQuery.data;
  const activeSession =
    workspace?.sessions.find((session) => session.id === selectedSessionId) ?? null;

  useEffect(() => {
    if (activeSession == null) {
      setSelectedFilePath("");
      setDraftContent("");
      return;
    }

    const firstPath = activeSession.files[0]?.path ?? "";
    setSelectedFilePath((current) =>
      activeSession.files.some((file) => file.path === current) ? current : firstPath,
    );
  }, [activeSession]);

  useEffect(() => {
    if (activeSession == null) {
      return;
    }

    const file = activeSession.files.find((item) => item.path === selectedFilePath);
    setDraftContent(file?.content ?? "");
    setRollbackTarget(activeSession.headSavepointId);
  }, [activeSession, selectedFilePath]);

  if (workspaceQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  if (workspace == null) {
    throw new Error("Workspace not found.");
  }

  const selectedFile =
    activeSession?.files.find((file) => file.path === selectedFilePath) ?? null;

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
          </InlineActions>
          <div style={{ marginTop: 20 }}>
            <Tabs>
              <TabButton $active={tab === "sessions"} onClick={() => setTab("sessions")}>
                Sessions
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

      {tab === "sessions" ? (
        <SectionGrid>
          <SectionCard $span={12}>
            <SessionLayout>
              <div>
                <SectionHeader>
                  <SectionTitle
                    title="Sessions"
                    body="Branch, import, and manage RAF sessions for this workspace."
                  />
                </SectionHeader>
                <SessionRail>
                  {workspace.sessions.map((session) => (
                    <SessionButton
                      key={session.id}
                      $active={session.id === activeSession?.id}
                      onClick={() => setSelectedSessionId(session.id)}
                    >
                      <Typography.Body component="strong">{session.name}</Typography.Body>
                      <Typography.Body color="secondary" component="p">
                        {session.description}
                      </Typography.Body>
                      <InlineActions style={{ marginTop: 10 }}>
                        <ToneChip $tone={session.status}>{session.status}</ToneChip>
                        <Tag>{session.kind}</Tag>
                      </InlineActions>
                    </SessionButton>
                  ))}
                </SessionRail>
              </div>

              <div>
                {activeSession == null ? null : (
                  <SectionGrid>
                    <SectionCard $span={7}>
                      <CardHeader>
                        <div>
                          <Typography.Heading component="h3" size="S">
                            {activeSession.name}
                          </Typography.Heading>
                          <Typography.Body color="secondary" component="p">
                            {activeSession.description}
                          </Typography.Body>
                        </div>
                        <InlineActions>
                          <ToneChip $tone={activeSession.status}>{activeSession.status}</ToneChip>
                          <Tag>{activeSession.kind}</Tag>
                        </InlineActions>
                      </CardHeader>
                      <Typography.Body color="secondary" component="p">
                        Head savepoint:{" "}
                        {activeSession.savepoints.find(
                          (savepoint) => savepoint.id === activeSession.headSavepointId,
                        )?.name ?? "n/a"}
                      </Typography.Body>
                      <Typography.Body color="secondary" component="p">
                        Updated {new Date(activeSession.updatedAt).toLocaleString()}
                      </Typography.Body>
                      <InlineActions style={{ marginTop: 16 }}>
                        <Button
                          size="medium"
                          variant="secondary-fill"
                          disabled={deleteSession.isPending || workspace.sessions.length <= 1}
                          onClick={() =>
                            deleteSession.mutate({
                              workspaceId: workspace.id,
                              sessionId: activeSession.id,
                            })
                          }
                        >
                          Delete session
                        </Button>
                      </InlineActions>
                    </SectionCard>

                    <SectionCard $span={5}>
                      <SectionTitle
                        title="Create session"
                        body="Fork from the current default session for focused work."
                      />
                      <div style={{ marginTop: 16 }}>
                        <FormGrid
                          onSubmit={(event) => {
                            event.preventDefault();
                            if (sessionName.trim() === "") {
                              return;
                            }

                            createSession.mutate({
                              workspaceId: workspace.id,
                              name: sessionName,
                              description: sessionDescription,
                              mode: "branch",
                              baseSessionId: activeSession.id,
                            });
                            setSessionName("");
                            setSessionDescription("");
                          }}
                        >
                          <Field>
                            Session name
                            <TextInput
                              value={sessionName}
                              onChange={(event) => setSessionName(event.target.value)}
                              placeholder="qa-pass"
                            />
                          </Field>
                          <Field>
                            Description
                            <TextArea
                              value={sessionDescription}
                              onChange={(event) => setSessionDescription(event.target.value)}
                              placeholder="Short summary of what the session is for."
                            />
                          </Field>
                          <Button size="medium" type="submit" disabled={createSession.isPending}>
                            Create branch session
                          </Button>
                        </FormGrid>
                      </div>
                    </SectionCard>

                    <SectionCard $span={12}>
                      <SectionHeader>
                        <SectionTitle
                          title="Savepoints"
                          body="Checkpoint, inspect, and roll back immutable session state."
                        />
                      </SectionHeader>
                      <SavepointGrid>
                        {activeSession.savepoints.map((savepoint) => (
                          <SavepointRow key={savepoint.id}>
                            <div>
                              <Typography.Body component="strong">
                                {savepoint.name}
                              </Typography.Body>
                              <Typography.Body color="secondary" component="p">
                                {savepoint.note}
                              </Typography.Body>
                              <InlineActions style={{ marginTop: 10 }}>
                                <Tag>{savepoint.fileCount} files</Tag>
                                <Tag>{savepoint.sizeLabel}</Tag>
                                <Tag>{new Date(savepoint.createdAt).toLocaleString()}</Tag>
                              </InlineActions>
                            </div>
                            <InlineActions>
                              <Button
                                size="medium"
                                variant="secondary-fill"
                                disabled={rollbackSession.isPending}
                                onClick={() =>
                                  rollbackSession.mutate({
                                    workspaceId: workspace.id,
                                    sessionId: activeSession.id,
                                    savepointId: savepoint.id,
                                  })
                                }
                              >
                                Roll back
                              </Button>
                            </InlineActions>
                          </SavepointRow>
                        ))}
                      </SavepointGrid>
                    </SectionCard>
                  </SectionGrid>
                )}
              </div>
            </SessionLayout>
          </SectionCard>
        </SectionGrid>
      ) : null}

      {tab === "editor" && activeSession != null ? (
        <SectionGrid>
          <SectionCard $span={12}>
            <SectionHeader>
              <SectionTitle
                title="Browser + editor"
                body="A RAF-specific studio for browsing files, editing drafts, importing sessions, and capturing savepoints."
              />
            </SectionHeader>

            <TwoColumnFields style={{ marginBottom: 16 }}>
              <Field>
                Import a session
                <TextInput
                  value={importName}
                  onChange={(event) => setImportName(event.target.value)}
                  placeholder="imported-support-thread"
                />
              </Field>
              <Field>
                Roll back target
                <Select
                  value={rollbackTarget}
                  onChange={(event) => setRollbackTarget(event.target.value)}
                >
                  {activeSession.savepoints.map((savepoint) => (
                    <option key={savepoint.id} value={savepoint.id}>
                      {savepoint.name}
                    </option>
                  ))}
                </Select>
              </Field>
            </TwoColumnFields>

            <InlineActions style={{ marginBottom: 20 }}>
              <Button
                size="medium"
                onClick={() => {
                  if (importName.trim() === "") {
                    return;
                  }

                  createSession.mutate({
                    workspaceId: workspace.id,
                    name: importName,
                    description: "Imported into RAF from the Web UI.",
                    mode: "imported",
                    baseSessionId: activeSession.id,
                  });
                  setImportName("");
                }}
              >
                Import session
              </Button>
              <Button
                size="medium"
                variant="secondary-fill"
                disabled={rollbackSession.isPending}
                onClick={() =>
                  rollbackSession.mutate({
                    workspaceId: workspace.id,
                    sessionId: activeSession.id,
                    savepointId: rollbackTarget,
                  })
                }
              >
                Roll back selection
              </Button>
            </InlineActions>

            <FileStudio>
              <div>
                <Typography.Heading component="h3" size="S">
                  {activeSession.name} files
                </Typography.Heading>
                <FileList style={{ marginTop: 14 }}>
                  {activeSession.files.map((file) => (
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
                  <FormGrid
                    onSubmit={(event) => {
                      event.preventDefault();
                      updateFile.mutate({
                        workspaceId: workspace.id,
                        sessionId: activeSession.id,
                        path: selectedFile.path,
                        content: draftContent,
                      });
                    }}
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
                      <ToneChip $tone={activeSession.status}>{activeSession.status}</ToneChip>
                    </CardHeader>
                    <EditorArea
                      value={draftContent}
                      onChange={(event) => setDraftContent(event.target.value)}
                    />
                    <SectionGrid>
                      <SectionCard $span={7}>
                        <SectionTitle
                          title="Save draft"
                          body="Persists the current file back into the selected RAF session."
                        />
                        <InlineActions style={{ marginTop: 14 }}>
                          <Button size="medium" type="submit" disabled={updateFile.isPending}>
                            Save file
                          </Button>
                        </InlineActions>
                      </SectionCard>

                      <SectionCard $span={5}>
                        <SectionTitle
                          title="Checkpoint"
                          body="Create a savepoint from the current local session state."
                        />
                        <FormGrid
                          style={{ marginTop: 14 }}
                          onSubmit={(event) => {
                            event.preventDefault();
                            if (savepointName.trim() === "") {
                              return;
                            }

                            createSavepoint.mutate({
                              workspaceId: workspace.id,
                              sessionId: activeSession.id,
                              name: savepointName,
                              note: savepointNote,
                            });
                            setSavepointName("");
                            setSavepointNote("");
                          }}
                        >
                          <TextInput
                            value={savepointName}
                            onChange={(event) => setSavepointName(event.target.value)}
                            placeholder="after-editor-pass"
                          />
                          <TextArea
                            value={savepointNote}
                            onChange={(event) => setSavepointNote(event.target.value)}
                            placeholder="Why this checkpoint exists."
                          />
                          <Button
                            size="medium"
                            variant="secondary-fill"
                            type="submit"
                            disabled={createSavepoint.isPending}
                          >
                            Create savepoint
                          </Button>
                        </FormGrid>
                      </SectionCard>
                    </SectionGrid>
                  </FormGrid>
                )}
              </EditorPanel>
            </FileStudio>
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
