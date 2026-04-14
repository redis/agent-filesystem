import { useLocation } from "@tanstack/react-router";
import { Typography } from "@redislabsdev/redis-ui-components";
import * as S from "./app-bar.styles";
import { resolveNavigationTitleParts } from "./navigation-items";

export function AppBar() {
  const location = useLocation();
  const title = resolveNavigationTitleParts(location.pathname);

  return (
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
    </S.HeaderContainer>
  );
}
