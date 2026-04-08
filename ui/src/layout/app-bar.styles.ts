import styled from "styled-components";

export const HeaderContainer = styled.header`
  display: flex;
  justify-content: space-between;
  gap: 20px;
  height: 7rem;
  background-color: ${({ theme }) => theme.semantic.color.background.neutral0};
  border-bottom: 1px solid
    ${({ theme }) => theme.semantic.color.border.neutral200};
  padding: 1.2rem 2rem 1.2rem 3.2rem;
  align-items: center;

  @media (max-width: 1080px) {
    height: auto;
    padding: 1.2rem 1.4rem 1.2rem 1.8rem;
    flex-direction: column;
    align-items: stretch;
  }
`;

export const HeaderTitleGroup = styled.div`
  display: grid;
  gap: 8px;
  min-width: 0;
`;

export const HeaderActions = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  justify-content: flex-end;

  @media (max-width: 1080px) {
    justify-content: flex-start;
  }
`;

export const TitleSection = styled.span`
  color: ${({ theme }) => theme.semantic.color.text.neutral500};
`;

export const TitlePage = styled.span`
  color: ${({ theme }) => theme.semantic.color.text.neutral900};
`;

export const ScopeText = styled.span`
  color: ${({ theme }) => theme.semantic.color.text.neutral600};
  font-size: 13px;
  line-height: 1.5;
`;

export const DatabaseTrigger = styled.button`
  min-width: 320px;
  border: 1px solid ${({ theme }) => theme.semantic.color.border.neutral200};
  border-radius: 16px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(245, 248, 253, 0.92)),
    ${({ theme }) => theme.semantic.color.background.neutral0};
  padding: 12px 14px;
  text-align: left;
  cursor: pointer;
  display: grid;
  gap: 4px;
  transition:
    border-color 160ms ease,
    box-shadow 160ms ease,
    transform 160ms ease;

  &:hover:enabled {
    transform: translateY(-1px);
    border-color: rgba(170, 59, 255, 0.24);
    box-shadow: 0 12px 24px rgba(8, 6, 13, 0.08);
  }

  &:disabled {
    cursor: default;
    opacity: 0.75;
  }

  @media (max-width: 720px) {
    min-width: 0;
    width: 100%;
  }
`;

export const DatabaseTriggerLabel = styled.span`
  color: ${({ theme }) => theme.semantic.color.text.neutral600};
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
`;

export const DatabaseTriggerValueRow = styled.span`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
`;

export const DatabaseTriggerValue = styled.span`
  color: ${({ theme }) => theme.semantic.color.text.neutral900};
  font-size: 15px;
  font-weight: 700;
`;

export const DatabaseTriggerMeta = styled.span`
  color: ${({ theme }) => theme.semantic.color.text.neutral600};
  font-size: 12px;
  line-height: 1.45;
`;

export const TriggerCaret = styled.span`
  color: ${({ theme }) => theme.semantic.color.text.neutral500};
  font-size: 12px;
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
