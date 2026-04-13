import { createFileRoute } from "@tanstack/react-router";
import { Button } from "@redislabsdev/redis-ui-components";
import { useMemo, useState } from "react";
import type { FormEvent } from "react";
import styled from "styled-components";
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
  FormGrid,
  PageStack,
  SectionTitle,
  TextArea,
  TextInput,
  TwoColumnFields,
} from "../components/afs-kit";
import { DatabaseTable } from "../foundation/tables/database-table";
import { useDatabaseScope } from "../foundation/database-scope";

export const Route = createFileRoute("/databases")({
  component: DatabasesPage,
});

type DialogMode = "create" | "edit" | null;

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

function DatabasesPage() {
  const { databases, selectedDatabase, selectDatabase, saveDatabase, removeDatabase, isLoading } = useDatabaseScope();
  const [dialogMode, setDialogMode] = useState<DialogMode>(null);
  const [editingDatabaseId, setEditingDatabaseId] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [form, setForm] = useState<DatabaseFormState>(createInitialFormState());

  const editingDatabase = useMemo(
    () => databases.find((database) => database.id === editingDatabaseId) ?? null,
    [databases, editingDatabaseId],
  );

  function closeDialog() {
    setDialogMode(null);
    setEditingDatabaseId(null);
    setFormError(null);
    setForm(createInitialFormState());
  }

  function openCreateDialog() {
    setDialogMode("create");
    setEditingDatabaseId(null);
    setFormError(null);
    setForm(createInitialFormState());
  }

  function openEditDialog(databaseId: string) {
    const database = databases.find((item) => item.id === databaseId);
    if (database == null) {
      return;
    }

    setDialogMode("edit");
    setEditingDatabaseId(databaseId);
    setForm(
      createInitialFormState({
        displayName: database.displayName,
        description: database.description,
        endpointLabel: database.endpointLabel,
        dbIndex: database.dbIndex,
        username: database.username,
        password: database.password,
        useTLS: database.useTLS,
      }),
    );
  }

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
        id: editingDatabaseId ?? undefined,
        name: form.displayName,
        description: form.description,
        redisAddr: form.endpointLabel,
        redisUsername: form.username,
        redisPassword: form.password,
        redisDB: Number.parseInt(form.dbIndex || "0", 10) || 0,
        redisTLS: form.useTLS,
      });
      closeDialog();
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Unable to save database.");
    }
  }

  async function deleteDatabase(databaseId: string) {
    const database = databases.find((item) => item.id === databaseId);
    const confirmed = window.confirm(
      `Remove database "${database?.displayName || database?.databaseName || databaseId}" from the picker and configuration list?`,
    );
    if (!confirmed) {
      return;
    }

    try {
      await removeDatabase(databaseId);
      if (editingDatabaseId === databaseId) {
        closeDialog();
      }
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Unable to remove database.");
    }
  }

  const isDialogOpen = dialogMode != null;

  return (
    <PageStack>
      <PageSection>
        <PageHeader>
          <SectionTitle
            title="Databases"
            body="Manage database connections and select the active scope."
          />
          <Button size="medium" onClick={openCreateDialog}>
            Add database
          </Button>
        </PageHeader>

        <DatabaseTable
          rows={databases}
          loading={isLoading}
          selectedDatabaseId={selectedDatabase?.id ?? null}
          onSelectDatabase={selectDatabase}
          onEditDatabase={openEditDialog}
          onDeleteDatabase={deleteDatabase}
        />
      </PageSection>

      {isDialogOpen ? (
        <DialogOverlay
          onClick={(event) => {
            if (event.target === event.currentTarget) {
              closeDialog();
            }
          }}
        >
          <DialogCard>
            <DialogHeader>
              <div>
                <DialogTitle>
                  {dialogMode === "create" ? "Add database" : `Edit ${editingDatabase?.displayName || editingDatabase?.databaseName || "database"}`}
                </DialogTitle>
                <DialogBody>
                  Configure how this database appears in the selector and what connection settings it uses.
                </DialogBody>
              </div>
              <DialogCloseButton type="button" aria-label="Close" onClick={closeDialog}>
                &times;
              </DialogCloseButton>
            </DialogHeader>

            <FormGrid onSubmit={submitForm}>
              <Field>
                Name
                <TextInput
                  autoFocus
                  value={form.displayName}
                  onChange={(event) => updateForm("displayName", event.target.value)}
                  placeholder="localhost:6388"
                />
              </Field>

              <Field>
                Description
                <TextArea
                  value={form.description}
                  onChange={(event) => updateForm("description", event.target.value)}
                  placeholder="Shared Claude workspace database for local agent state."
                />
              </Field>

              <TwoColumnFields>
                <Field>
                  Redis address
                  <TextInput
                    value={form.endpointLabel}
                    onChange={(event) => updateForm("endpointLabel", event.target.value)}
                    placeholder="localhost:6379"
                  />
                </Field>
                <Field>
                  Database index
                  <TextInput
                    value={form.dbIndex}
                    onChange={(event) => updateForm("dbIndex", event.target.value)}
                    placeholder="0"
                  />
                </Field>
              </TwoColumnFields>

              <TwoColumnFields>
                <Field>
                  Username
                  <TextInput
                    value={form.username}
                    onChange={(event) => updateForm("username", event.target.value)}
                    placeholder="default"
                  />
                </Field>
                <Field>
                  Password
                  <TextInput
                    type="password"
                    value={form.password}
                    onChange={(event) => updateForm("password", event.target.value)}
                    placeholder="••••••••"
                  />
                </Field>
              </TwoColumnFields>

              <CheckboxRow>
                <label>
                  <input
                    checked={form.useTLS}
                    type="checkbox"
                    onChange={(event) => updateForm("useTLS", event.target.checked)}
                  />
                  Use TLS
                </label>
              </CheckboxRow>

              {formError ? <DialogError role="alert">{formError}</DialogError> : null}

              <DialogActions>
                <Button
                  size="medium"
                  type="button"
                  variant="secondary-fill"
                  onClick={closeDialog}
                >
                  Cancel
                </Button>
                <Button size="medium" type="submit">
                  {dialogMode === "create" ? "Add database" : "Save changes"}
                </Button>
              </DialogActions>
            </FormGrid>
          </DialogCard>
        </DialogOverlay>
      ) : null}
    </PageStack>
  );
}

const PageSection = styled.section`
  display: flex;
  flex-direction: column;
  gap: 18px;
`;

const PageHeader = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;

  @media (max-width: 720px) {
    flex-direction: column;
  }
`;

const CheckboxRow = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;

  label {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    color: var(--afs-ink);
    font-size: 14px;
  }
`;
