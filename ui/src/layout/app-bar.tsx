import { useLocation } from "@tanstack/react-router";
import { Typography } from "@redis-ui/components";
import * as S from "./app-bar.styles";
import { resolveNavigationTitleParts } from "./navigation-items";

export function AppBar() {
  const location = useLocation();
  const title = resolveNavigationTitleParts(location.pathname);

  return (
    <S.HeaderContainer>
      <S.HeaderTitleGroup>
        <S.TitleStack>
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
          {title.subtitle ? <S.Subtitle>{title.subtitle}</S.Subtitle> : null}
        </S.TitleStack>
      </S.HeaderTitleGroup>
    </S.HeaderContainer>
  );
}
