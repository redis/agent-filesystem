import { useEffect, useState } from "react";
import { useLocation } from "@tanstack/react-router";
import { Button, Menu, Typography } from "@redislabsdev/redis-ui-components";
import { Field, FormGrid, SectionTitle, TextInput } from "../components/afs-kit";
import { useDatabaseScope } from "../foundation/database-scope";
import * as S from "./app-bar.styles";
import { resolveNavigationTitleParts } from "./navigation-items";

type OpenDatabaseFormState = {
  displayName: string;
  databaseName: string;
  endpointLabel: string;
  dbIndex: string;
};

function createInitialFormState(): OpenDatabaseFormState {
  return {
    displayName: "",
    databaseName: "",
    endpointLabel: "",
    dbIndex: "0",
  };
}

export function AppBar() {
  const location = useLocation();
  const title = resolveNavigationTitleParts(location.pathname);
  const {
    databases,
    selectedDatabase,
    isLoading,
    selectDatabase,
    openDatabase,
    isOpenDatabaseDialogOpen,
    setOpenDatabaseDialogOpen,
  } = useDatabaseScope();
  const [form, setForm] = useState<OpenDatabaseFormState>(createInitialFormState);

  useEffect(() => {
    if (!isOpenDatabaseDialogOpen) {
      setForm(createInitialFormState());
    }
  }, [isOpenDatabaseDialogOpen]);

  const scopeText =
    selectedDatabase == null
      ? "No database selected. Open a Redis database to scope Overview, Workspaces, and Activity."
      : `Database: ${selectedDatabase.displayName} · ${selectedDatabase.workspaceCount} workspace${selectedDatabase.workspaceCount === 1 ? "" : "s"}.`;
  const triggerMeta =
    selectedDatabase == null
      ? "Open a Redis database to make one backend the active scope."
      : [
          selectedDatabase.endpointLabel,
          selectedDatabase.dbIndex !== "" ? `DB ${selectedDatabase.dbIndex}` : null,
          `${selectedDatabase.workspaceCount} workspace${selectedDatabase.workspaceCount === 1 ? "" : "s"}`,
        ]
          .filter(Boolean)
          .join(" · ");

  return (
    <>
      <S.HeaderContainer>
        <S.HeaderTitleGroup>
          <Typography.Heading component="h1" size="M">
            {title.section ? (
              <>
                <S.TitleSection>{title.section}</S.TitleSection>
                <S.TitlePage>{` / ${title.page}`}</S.TitlePage>
              </>
            ) : (
              title.page
            )}
          </Typography.Heading>
          <S.ScopeText>{scopeText}</S.ScopeText>
        </S.HeaderTitleGroup>

        <S.HeaderActions>
          <Menu>
            <Menu.Trigger withButton={false}>
              <S.DatabaseTrigger type="button" disabled={isLoading || databases.length === 0}>
                <S.DatabaseTriggerLabel>Current database</S.DatabaseTriggerLabel>
                <S.DatabaseTriggerValueRow>
                  <S.DatabaseTriggerValue>
                    {selectedDatabase?.displayName ?? "No database open"}
                  </S.DatabaseTriggerValue>
                  <S.TriggerCaret>v</S.TriggerCaret>
                </S.DatabaseTriggerValueRow>
                <S.DatabaseTriggerMeta>{isLoading ? "Loading databases..." : triggerMeta}</S.DatabaseTriggerMeta>
              </S.DatabaseTrigger>
            </Menu.Trigger>
            <Menu.Content align="end">
              {databases.map((database) => (
                <Menu.Content.Item
                  key={database.id}
                  text={
                    database.id === selectedDatabase?.id
                      ? `${database.displayName} (Current)`
                      : database.displayName
                  }
                  onClick={() => selectDatabase(database.id)}
                />
              ))}
              <Menu.Content.Item
                text="Open database..."
                onClick={() => setOpenDatabaseDialogOpen(true)}
              />
            </Menu.Content>
          </Menu>

          <Button size="medium" onClick={() => setOpenDatabaseDialogOpen(true)}>
            Open database
          </Button>
        </S.HeaderActions>
      </S.HeaderContainer>

      {isOpenDatabaseDialogOpen ? (
        <S.DialogOverlay
          onClick={(event) => {
            if (event.target === event.currentTarget) {
              setOpenDatabaseDialogOpen(false);
            }
          }}
        >
          <S.DialogCard>
            <S.DialogHeader>
              <SectionTitle
                eyebrow="Database Scope"
                title="Open a Redis database"
                body="The selected database becomes the active scope for Overview, Workspaces, and Activity. This keeps the UI simple now and leaves room for multi-database monitoring later."
              />
              <Button
                size="medium"
                variant="secondary-fill"
                onClick={() => setOpenDatabaseDialogOpen(false)}
              >
                Close
              </Button>
            </S.DialogHeader>

            <FormGrid
              onSubmit={(event) => {
                event.preventDefault();
                if (form.databaseName.trim() === "") {
                  return;
                }

                openDatabase({
                  displayName: form.displayName,
                  databaseName: form.databaseName,
                  endpointLabel: form.endpointLabel,
                  dbIndex: form.dbIndex,
                });
              }}
            >
              <Field>
                Connection name
                <TextInput
                  value={form.displayName}
                  onChange={(event) =>
                    setForm((current) => ({ ...current, displayName: event.target.value }))
                  }
                  placeholder="prod-primary"
                />
              </Field>
              <Field>
                Database name
                <TextInput
                  value={form.databaseName}
                  onChange={(event) =>
                    setForm((current) => ({ ...current, databaseName: event.target.value }))
                  }
                  placeholder="agentfs-prod-us-east-1"
                />
              </Field>
              <Field>
                Host or endpoint
                <TextInput
                  value={form.endpointLabel}
                  onChange={(event) =>
                    setForm((current) => ({ ...current, endpointLabel: event.target.value }))
                  }
                  placeholder="redis.example.com:6379"
                />
              </Field>
              <Field>
                Database index
                <TextInput
                  value={form.dbIndex}
                  onChange={(event) =>
                    setForm((current) => ({ ...current, dbIndex: event.target.value }))
                  }
                  placeholder="0"
                />
              </Field>

              <S.HelperText>
                If this database already exists in the current registry, opening it will simply switch the app scope to that database.
              </S.HelperText>

              <S.DialogActions>
                <Button size="medium" type="submit">
                  Open database
                </Button>
                <Button
                  size="medium"
                  type="button"
                  variant="secondary-fill"
                  onClick={() => setOpenDatabaseDialogOpen(false)}
                >
                  Cancel
                </Button>
              </S.DialogActions>
            </FormGrid>
          </S.DialogCard>
        </S.DialogOverlay>
      ) : null}
    </>
  );
}
