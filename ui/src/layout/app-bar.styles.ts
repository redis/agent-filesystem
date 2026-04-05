import styled from "styled-components";

export const HeaderContainer = styled.header`
  display: flex;
  height: 7rem;
  background-color: ${({ theme }) => theme.semantic.color.background.neutral0};
  border-bottom: 1px solid
    ${({ theme }) => theme.semantic.color.border.neutral200};
  padding: 1.2rem 2rem 1.2rem 3.2rem;
  align-items: center;
`;

export const TitleSection = styled.span`
  color: ${({ theme }) => theme.semantic.color.text.neutral500};
`;

export const TitlePage = styled.span`
  color: ${({ theme }) => theme.semantic.color.text.neutral900};
`;
