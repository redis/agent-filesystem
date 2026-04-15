import { Button, Typography } from "@redis-ui/components";
import { useState } from "react";
import {
  Field,
  FormGrid,
  InlineActions,
  MetaRow,
  SavepointGrid,
  SavepointRow,
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
  Tag,
  TextArea,
  TextInput,
} from "../../components/afs-kit";
import {
  useCreateSavepointMutation,
  useRestoreSavepointMutation,
} from "../../foundation/hooks/use-afs";
import type { AFSWorkspaceDetail, AFSWorkspaceView } from "../../foundation/types/afs";

type StudioTab = "overview" | "files" | "checkpoints" | "activity";

type Props = {
  workspace: AFSWorkspaceDetail;
  onBrowserViewChange: (view: AFSWorkspaceView) => void;
  onTabChange: (tab: StudioTab) => void;
};

export function CheckpointsTab({ workspace, onBrowserViewChange, onTabChange }: Props) {
  const createSavepoint = useCreateSavepointMutation();
  const restoreSavepoint = useRestoreSavepointMutation();

  const [savepointName, setSavepointName] = useState("");
  const [savepointNote, setSavepointNote] = useState("");

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
                Checkpoint note
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
          <SavepointGrid>
            {workspace.savepoints.length === 0 ? (
              <Typography.Body color="secondary" component="p">
                No checkpoints recorded yet.
              </Typography.Body>
            ) : null}
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
                      onBrowserViewChange(
                        savepoint.id === workspace.headSavepointId
                          ? "head"
                          : `checkpoint:${savepoint.id}`,
                      );
                      onTabChange("files");
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
    </>
  );
}
