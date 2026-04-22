import { Button } from "@redis-ui/components";
import { useEffect, useState } from "react";
import styled from "styled-components";
import {
  DialogActions,
  DialogError,
  Field,
  FormGrid,
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
  TextInput,
} from "../../components/afs-kit";
import type { AFSWorkspaceDetail } from "../../foundation/types/afs";

type Props = {
  workspace: AFSWorkspaceDetail;
  onSave: (input: { name: string; description: string }) => void | Promise<void>;
  isSaving: boolean;
  saveError?: string | null;
  onDelete: () => void;
  isDeleting: boolean;
};

export function SettingsTab({
  workspace,
  onSave,
  isSaving,
  saveError,
  onDelete,
  isDeleting,
}: Props) {
  const [name, setName] = useState(workspace.name);
  const [description, setDescription] = useState(workspace.description);
  const [localError, setLocalError] = useState<string | null>(null);

  useEffect(() => {
    setName(workspace.name);
    setDescription(workspace.description);
    setLocalError(null);
  }, [workspace.id, workspace.name, workspace.description]);

  const normalizedName = name.trim();
  const normalizedDescription = description.trim();
  const isDirty = normalizedName !== workspace.name || normalizedDescription !== workspace.description;

  return (
    <SectionGrid>
      <SectionCard $span={12}>
        <SectionHeader>
          <SectionTitle title="Workspace details" />
        </SectionHeader>

        <FormGrid
          onSubmit={(event) => {
            event.preventDefault();
            if (normalizedName === "") {
              setLocalError("Workspace name is required.");
              return;
            }
            setLocalError(null);
            void onSave({ name: normalizedName, description: normalizedDescription });
          }}
        >
          <Field>
            Workspace name
            <TextInput
              autoFocus
              value={name}
              onChange={(event) => {
                setLocalError(null);
                setName(event.target.value);
              }}
              placeholder="customer-portal"
            />
            <FieldHint>Renaming keeps the same stable workspace ID and URL.</FieldHint>
          </Field>

          <Field>
            Description
            <TextInput
              value={description}
              onChange={(event) => {
                setLocalError(null);
                setDescription(event.target.value);
              }}
              placeholder="What this workspace is for, who owns it, and why it exists."
            />
          </Field>

          {localError ? <DialogError role="alert">{localError}</DialogError> : null}
          {saveError ? <DialogError role="alert">{saveError}</DialogError> : null}

          <DialogActions style={{ justifyContent: "flex-end" }}>
            <Button size="medium" type="submit" disabled={isSaving || !isDirty}>
              {isSaving ? "Saving..." : "Save changes"}
            </Button>
          </DialogActions>
        </FormGrid>

        <MetaTable>
          <tbody>
            <MetaRow>
              <MetaLabel>Workspace ID</MetaLabel>
              <MetaValue>
                <MonoValue>{workspace.id}</MonoValue>
              </MetaValue>
            </MetaRow>
            <MetaRow>
              <MetaLabel>Database</MetaLabel>
              <MetaValue>{workspace.databaseName}</MetaValue>
            </MetaRow>
            <MetaRow>
              <MetaLabel>Redis key</MetaLabel>
              <MetaValue>
                <MonoValue>{workspace.redisKey}</MonoValue>
              </MetaValue>
            </MetaRow>
            {workspace.mountedPath ? (
              <MetaRow>
                <MetaLabel>Mounted path</MetaLabel>
                <MetaValue>{workspace.mountedPath}</MetaValue>
              </MetaRow>
            ) : null}
          </tbody>
        </MetaTable>
      </SectionCard>

      <DangerZoneCard>
        <DangerZoneHeader>
          <DangerZoneTitle>Danger zone</DangerZoneTitle>
          <DangerZoneDesc>
            Permanently delete this workspace and remove it from the registry.
          </DangerZoneDesc>
        </DangerZoneHeader>
        <DeleteWorkspaceButton size="large" disabled={isDeleting} onClick={onDelete}>
          {isDeleting ? "Deleting..." : "Delete workspace"}
        </DeleteWorkspaceButton>
      </DangerZoneCard>
    </SectionGrid>
  );
}

const MetaTable = styled.table`
  width: 100%;
  border-collapse: collapse;
  margin-top: 8px;
`;

const MetaRow = styled.tr`
  border-top: 1px solid var(--afs-line);

  &:first-child {
    border-top: none;
  }
`;

const MetaLabel = styled.th`
  width: 220px;
  padding: 14px 0;
  color: var(--afs-muted);
  font-size: 13px;
  font-weight: 600;
  text-align: left;
  vertical-align: top;
`;

const MetaValue = styled.td`
  padding: 14px 0;
  color: var(--afs-ink);
  font-size: 14px;
  line-height: 1.5;
  text-align: left;
`;

const MonoValue = styled.code`
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 13px;
`;

const FieldHint = styled.span`
  color: var(--afs-muted);
  font-size: 12px;
  font-weight: 500;
  line-height: 1.5;
`;

const DangerZoneCard = styled.div`
  grid-column: span 12;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 20px 24px;
  border: 1px solid rgba(220, 38, 38, 0.2);
  border-radius: 16px;
  background: rgba(220, 38, 38, 0.03);

  @media (max-width: 720px) {
    flex-direction: column;
    align-items: flex-start;
  }
`;

const DangerZoneHeader = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
`;

const DangerZoneTitle = styled.h3`
  margin: 0;
  color: #dc2626;
  font-size: 15px;
  font-weight: 700;
`;

const DangerZoneDesc = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.5;
`;

const DeleteWorkspaceButton = styled(Button)`
  && {
    white-space: nowrap;
    background: ${({ theme }) => theme.semantic.color.background.danger500};
    color: ${({ theme }) => theme.semantic.color.text.inverse};
    box-shadow: none;
  }

  &&:hover:not(:disabled),
  &&:focus-visible:not(:disabled) {
    background: ${({ theme }) => theme.semantic.color.background.danger600};
    color: ${({ theme }) => theme.semantic.color.text.inverse};
    box-shadow: none;
  }
`;
