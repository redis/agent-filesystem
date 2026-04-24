import styled from "styled-components";

export const HeaderContainer = styled.header`
  position: sticky;
  top: 0;
  z-index: 5;
  display: flex;
  justify-content: flex-start;
  gap: 16px;
  min-height: 5.25rem;
  background-color: var(--afs-bg-soft);
  border-bottom: 1px solid var(--afs-line);
  padding: 1rem 2rem 1rem 2rem;
  align-items: center;

  @media (max-width: 720px) {
    height: auto;
    padding: 1rem 1.4rem 1rem 1.8rem;
    flex-direction: column;
    align-items: stretch;
  }
`;

export const HeaderTitleGroup = styled.div`
  display: flex;
  align-items: center;
  flex: 1 1 auto;
  min-width: 0;
  color: var(--afs-ink);
`;

export const TitleStack = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
`;

export const TitleHeading = styled.h1`
  margin: 0;
  color: var(--afs-ink);
  font-size: 22px;
  font-weight: 700;
  line-height: 1.2;
`;

export const Subtitle = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 13px;
  font-weight: 400;
  line-height: 1.35;
`;

export const TitleSection = styled.span`
  color: var(--afs-muted);
`;

export const TitlePage = styled.span`
  color: var(--afs-ink);
`;
