import { Button } from "@redis-ui/components";
import { useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import styled from "styled-components";
import type { SaveDatabaseInput } from "../foundation/types/afs";
import type { AFSDatabaseScopeRecord } from "../foundation/database-scope";
import {
  DialogActions,
  DialogBody,
  DialogCard,
  DialogCloseButton,
  DialogError,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
  Field,
  FieldGroup,
  FormGrid,
  TextInput,
  TwoColumnFields,
} from "./afs-kit";

type DatabaseFormState = {
  displayName: string;
  description: string;
  endpointLabel: string;
  dbIndex: string;
  username: string;
  password: string;
  useTLS: boolean;
};

function createInitialFormState(
  value?: Partial<DatabaseFormState>,
): DatabaseFormState {
  return {
    displayName: value?.displayName ?? "",
    description: value?.description ?? "",
    endpointLabel: value?.endpointLabel ?? "",
    dbIndex: value?.dbIndex ?? "0",
    username: value?.username ?? "",
    password: value?.password ?? "",
    useTLS: value?.useTLS ?? false,
  };
}

export type AddDatabaseDialogProps = {
  isOpen: boolean;
  onClose: () => void;
  saveDatabase: (input: SaveDatabaseInput) => Promise<void>;
  mode?: "create" | "edit";
  editingDatabase?: AFSDatabaseScopeRecord | null;
  onDelete?: (databaseId: string) => void;
};

export function AddDatabaseDialog({
  isOpen,
  onClose,
  saveDatabase,
  mode = "create",
  editingDatabase = null,
  onDelete,
}: AddDatabaseDialogProps) {
  const [formError, setFormError] = useState<string | null>(null);
  const [form, setForm] = useState<DatabaseFormState>(createInitialFormState());
  const nameInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (isOpen) {
      if (mode === "edit" && editingDatabase) {
        setForm(
          createInitialFormState({
            displayName: editingDatabase.displayName,
            description: editingDatabase.description,
            endpointLabel: editingDatabase.endpointLabel,
            dbIndex: editingDatabase.dbIndex,
            username: editingDatabase.username,
            password: editingDatabase.password,
            useTLS: editingDatabase.useTLS,
          }),
        );
      } else {
        setForm(createInitialFormState());
      }
      setFormError(null);
      const id = setTimeout(() => nameInputRef.current?.focus(), 60);
      return () => clearTimeout(id);
    }
  }, [isOpen, mode, editingDatabase]);

  function updateForm<TKey extends keyof DatabaseFormState>(
    key: TKey,
    value: DatabaseFormState[TKey],
  ) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  async function submitForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (form.displayName.trim() === "" || form.endpointLabel.trim() === "") {
      return;
    }

    try {
      setFormError(null);
      await saveDatabase({
        id: mode === "edit" ? editingDatabase?.id : undefined,
        name: form.displayName,
        description: form.description,
        redisAddr: form.endpointLabel,
        redisUsername: form.username,
        redisPassword: form.password,
        redisDB: Number.parseInt(form.dbIndex || "0", 10) || 0,
        redisTLS: form.useTLS,
      });
      onClose();
    } catch (error) {
      setFormError(
        error instanceof Error ? error.message : "Unable to save database.",
      );
    }
  }

  if (!isOpen) return null;

  return (
    <DialogOverlay
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <DialogCard>
        <DialogHeader>
          <div>
            <DialogTitle>
              {mode === "create"
                ? "Add database"
                : `Edit ${editingDatabase?.displayName || editingDatabase?.databaseName || "database"}`}
            </DialogTitle>
            <DialogBody>
              Configure how this database appears in the app and what connection
              settings it uses.
            </DialogBody>
          </div>
          <DialogCloseButton type="button" aria-label="Close" onClick={onClose}>
            &times;
          </DialogCloseButton>
        </DialogHeader>

        <FormGrid onSubmit={submitForm}>
          <Field>
            Name
            <TextInput
              ref={nameInputRef}
              value={form.displayName}
              onChange={(event) =>
                updateForm("displayName", event.target.value)
              }
              placeholder="localhost:6388"
            />
          </Field>

          <Field>
            Description
            <TextInput
              value={form.description}
              onChange={(event) =>
                updateForm("description", event.target.value)
              }
              placeholder="Shared Claude workspace database for local agent state."
            />
          </Field>

          <FieldGroup title="Connection Details">
            <Field>
              Redis address
              <TextInput
                value={form.endpointLabel}
                onChange={(event) =>
                  updateForm("endpointLabel", event.target.value)
                }
                placeholder="localhost:6379"
              />
            </Field>

            <TwoColumnFields>
              <Field>
                Username
                <TextInput
                  autoComplete="off"
                  value={form.username}
                  onChange={(event) => updateForm("username", event.target.value)}
                  placeholder="default"
                />
              </Field>
              <Field>
                Password
                <TextInput
                  autoComplete="off"
                  type="password"
                  value={form.password}
                  onChange={(event) => updateForm("password", event.target.value)}
                  placeholder="••••••••"
                />
              </Field>
            </TwoColumnFields>

            <InlineRow>
              <Field style={{ maxWidth: 100 }}>
                DB index
                <TextInput
                  type="number"
                  min="0"
                  max="15"
                  value={form.dbIndex}
                  onChange={(event) => updateForm("dbIndex", event.target.value)}
                  placeholder="0"
                />
              </Field>
              <ToggleLabel>
                <ToggleSwitch>
                  <input
                    checked={form.useTLS}
                    type="checkbox"
                    onChange={(event) =>
                      updateForm("useTLS", event.target.checked)
                    }
                  />
                  <ToggleTrack />
                </ToggleSwitch>
                Use TLS
              </ToggleLabel>
            </InlineRow>
          </FieldGroup>

          {formError ? (
            <DialogError role="alert">{formError}</DialogError>
          ) : null}

          <DialogActions>
            <DialogActionGroup>
              {mode === "edit" && editingDatabase && onDelete ? (
                <RemoveDatabaseButton
                  size="medium"
                  type="button"
                  onClick={() => onDelete(editingDatabase.id)}
                >
                  Remove database
                </RemoveDatabaseButton>
              ) : null}
            </DialogActionGroup>
            <DialogActionGroup>
              <Button
                size="medium"
                type="button"
                variant="secondary-fill"
                onClick={onClose}
              >
                Cancel
              </Button>
              <Button size="medium" type="submit">
                {mode === "create" ? "Add database" : "Save changes"}
              </Button>
            </DialogActionGroup>
          </DialogActions>
        </FormGrid>
      </DialogCard>
    </DialogOverlay>
  );
}

const DialogActionGroup = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
`;

const RemoveDatabaseButton = styled(Button)`
  && {
    background: ${({ theme }) => theme.semantic.color.background.danger500};
    border-color: ${({ theme }) => theme.semantic.color.background.danger500};
    color: ${({ theme }) => theme.semantic.color.text.inverse};
  }

  &&:hover:not(:disabled) {
    background: ${({ theme }) => theme.semantic.color.background.danger600};
    border-color: ${({ theme }) => theme.semantic.color.background.danger600};
  }
`;

const InlineRow = styled.div`
  display: flex;
  align-items: flex-end;
  gap: 18px;
`;

const ToggleLabel = styled.label`
  display: inline-flex;
  align-items: center;
  gap: 10px;
  color: var(--afs-ink);
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
  padding-bottom: 10px;
`;

const ToggleSwitch = styled.span`
  position: relative;
  display: inline-flex;
  width: 36px;
  height: 20px;
  flex-shrink: 0;

  input {
    position: absolute;
    opacity: 0;
    width: 0;
    height: 0;
  }

  input:checked + span {
    background: var(--afs-focus, #2563eb);
  }

  input:checked + span::after {
    transform: translateX(16px);
  }

  input:focus-visible + span {
    box-shadow: 0 0 0 3px var(--afs-focus-soft);
  }
`;

const ToggleTrack = styled.span`
  position: absolute;
  inset: 0;
  border-radius: 999px;
  background: var(--afs-line-strong, #ccc);
  transition: background 160ms ease;
  cursor: pointer;

  &::after {
    content: "";
    position: absolute;
    top: 2px;
    left: 2px;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: white;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.15);
    transition: transform 160ms ease;
  }
`;
