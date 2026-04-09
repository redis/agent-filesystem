import styled from "styled-components";

export const HeaderContainer = styled.header`
  display: flex;
  justify-content: flex-start;
  gap: 16px;
  min-height: 5.25rem;
  background-color: ${({ theme }) => theme.semantic.color.background.neutral0};
  border-bottom: 1px solid
    ${({ theme }) => theme.semantic.color.border.neutral200};
  padding: 1rem 2rem 1rem 3.2rem;
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
`;

export const HeaderActions = styled.div`
  display: flex;
  flex-wrap: nowrap;
  gap: 8px;
  align-items: center;
  justify-content: flex-end;
  margin-left: auto;

  @media (max-width: 720px) {
    justify-content: flex-start;
    margin-left: 0;
  }
`;

export const TitleSection = styled.span`
  color: ${({ theme }) => theme.semantic.color.text.neutral500};
`;

export const TitlePage = styled.span`
  color: ${({ theme }) => theme.semantic.color.text.neutral900};
`;

export const DatabaseTrigger = styled.button`
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  min-width: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  padding: 0;
  cursor: pointer;
  transition:
    color 160ms ease,
    opacity 160ms ease;

  &:hover:enabled {
    opacity: 0.78;
  }

  &:focus-visible {
    outline: 2px solid ${({ theme }) => theme.semantic.color.border.brand500};
    outline-offset: 4px;
  }

  &:disabled {
    cursor: default;
    opacity: 0.75;
  }

  @media (max-width: 720px) {
    justify-content: flex-start;
  }
`;

export const DatabaseTriggerValue = styled.span`
  color: ${({ theme }) => theme.semantic.color.text.neutral900};
  font-size: 14px;
  font-weight: 600;
  line-height: 1.2;
`;

export const TriggerCaret = styled.span`
  display: inline-flex;
  align-items: center;
  color: ${({ theme }) => theme.semantic.color.icon.neutral500};
`;

export const DatabaseMenuItemText = styled.span<{ $selected: boolean }>`
  font-weight: ${({ $selected }) => ($selected ? 800 : 600)};
`;

export const DialogOverlay = styled.div`
  position: fixed;
  inset: 0;
  z-index: 40;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(8, 6, 13, 0.36);
`;

export const DialogCard = styled.div`
  width: min(560px, 100%);
  max-height: min(88vh, 760px);
  overflow: auto;
  border: 1px solid var(--afs-line);
  border-radius: 24px;
  padding: 24px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(249, 251, 255, 0.94)),
    var(--afs-panel);
  box-shadow: var(--afs-shadow);
`;

export const DialogHeader = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  margin-bottom: 18px;

  @media (max-width: 720px) {
    flex-direction: column;
  }
`;

export const DialogActions = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
`;

export const HelperText = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.6;
`;
