import { useLocation, useNavigate } from "@tanstack/react-router";
import { Menu, Typography } from "@redislabsdev/redis-ui-components";
import { CaretDownIcon } from "@redislabsdev/redis-ui-icons/monochrome";
import { useDatabaseScope } from "../foundation/database-scope";
import * as S from "./app-bar.styles";
import { resolveNavigationTitleParts } from "./navigation-items";

export function AppBar() {
  const location = useLocation();
  const navigate = useNavigate();
  const title = resolveNavigationTitleParts(location.pathname);
  const {
    databases,
    selectedDatabase,
    isLoading,
    selectDatabase,
  } = useDatabaseScope();

  const currentDatabaseLabel = `Database: ${selectedDatabase?.displayName || selectedDatabase?.databaseName || "current"}`;

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
        </S.HeaderTitleGroup>

        <S.HeaderActions>
          <Menu>
            <Menu.Trigger withButton={false}>
              <S.DatabaseTrigger type="button" disabled={isLoading}>
                <S.DatabaseTriggerValue>{isLoading ? "Database: loading..." : currentDatabaseLabel}</S.DatabaseTriggerValue>
                <S.TriggerCaret aria-hidden="true">
                  <CaretDownIcon size="S" />
                </S.TriggerCaret>
              </S.DatabaseTrigger>
            </Menu.Trigger>
            <Menu.Content align="end">
              {databases.map((database) => (
                <Menu.Content.Item
                  key={database.id}
                  text={
                    <S.DatabaseMenuItemText $selected={database.id === selectedDatabase?.id}>
                      {database.displayName || database.databaseName}
                    </S.DatabaseMenuItemText>
                  }
                  description={database.endpointLabel || undefined}
                  onClick={() => selectDatabase(database.id)}
                />
              ))}
              <Menu.Content.Item
                text="Configure databases..."
                onClick={() => void navigate({ to: "/databases" })}
              />
            </Menu.Content>
          </Menu>
        </S.HeaderActions>
      </S.HeaderContainer>
    </>
  );
}
