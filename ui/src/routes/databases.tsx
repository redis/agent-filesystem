import { createFileRoute } from "@tanstack/react-router";
import { Button } from "@redislabsdev/redis-ui-components";
import { useMemo, useState } from "react";
import type { FormEvent } from "react";
import styled from "styled-components";
import {
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
            eyebrow="Database Scope"
            title="Configure databases"
            body="Manage the databases shown in the selector, edit connection metadata, and keep the current scope aligned with the database you want this UI to operate against."
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
              <SectionTitle
                eyebrow="Database Scope"
                title={dialogMode === "create" ? "Add database" : `Edit ${editingDatabase?.displayName || editingDatabase?.databaseName || "database"}`}
                body="These settings control how the database appears in the selector and what metadata is attached to it throughout the UI."
              />
              <Button size="medium" variant="secondary-fill" onClick={closeDialog}>
                Close
              </Button>
            </DialogHeader>

            <FormGrid onSubmit={submitForm}>
              <Field>
                Name
                <TextInput
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

              {formError ? <HelperText role="alert">{formError}</HelperText> : null}

              <DialogActions>
                <Button size="medium" type="submit">
                  {dialogMode === "create" ? "Add database" : "Save changes"}
                </Button>
                <Button
                  size="medium"
                  type="button"
                  variant="secondary-fill"
                  onClick={closeDialog}
                >
                  Cancel
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

const DialogOverlay = styled.div`
  position: fixed;
  inset: 0;
  z-index: 40;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(8, 6, 13, 0.36);
`;

const DialogCard = styled.div`
  width: min(720px, 100%);
  max-height: min(88vh, 760px);
  overflow: auto;
  border: 1px solid var(--afs-line);
  border-radius: 24px;
  padding: 24px;
  background: #fff;
  box-shadow: 0 18px 40px rgba(8, 6, 13, 0.12);
`;

const DialogHeader = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  margin-bottom: 18px;

  @media (max-width: 720px) {
    flex-direction: column;
  }
`;

const HelperText = styled.p`
  margin: 0;
  color: #c2364a;
  font-size: 13px;
  line-height: 1.6;
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

const DialogActions = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
`;
