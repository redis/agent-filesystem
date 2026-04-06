import styled from "styled-components";
import { TableHeading } from "@redislabsdev/redis-ui-components";

export const TableCard = styled.div`
  border: 1px solid ${({ theme }) => theme.semantic.color.border.neutral200};
  border-radius: 8px;
  overflow: hidden;
  background: ${({ theme }) => theme.semantic.color.background.neutral0};
`;

export const TableViewport = styled.div`
  max-height: 720px;
  overflow: auto;

  thead th {
    position: sticky;
    top: 0;
    z-index: 2;
    background: ${({ theme }) => theme.semantic.color.background.neutral0};
  }

  tbody tr {
    transition: background 160ms ease;
  }

  tbody tr:hover {
    background: ${({ theme }) => theme.semantic.color.background.neutral100};
  }
`;

export const EmptyState = styled.div`
  padding: 40px;
  text-align: center;
  color: ${({ theme }) => theme.semantic.color.text.neutral600};
`;

export const WorkspaceNameButton = styled.button`
  border: none;
  background: transparent;
  padding: 0;
  color: ${({ theme }) => theme.semantic.color.text.primary600};
  font: inherit;
  font-weight: 700;
  cursor: pointer;
  text-align: left;

  &:hover {
    text-decoration: underline;
  }
`;

export const Stack = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
`;

export const SearchInput = styled(TableHeading.SearchInput)`
  width: 320px;
  border-radius: 8px !important;
  border: 1px solid ${({ theme }) => theme.semantic.color.border.neutral200} !important;
  background: ${({ theme }) => theme.semantic.color.background.neutral0} !important;
  box-shadow: none !important;

  @media (max-width: 800px) {
    width: 100%;
  }
`;

export const HeadingWrap = styled.div`
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  padding: 18px 20px 14px;

  @media (max-width: 800px) {
    flex-direction: column;
    align-items: stretch;
  }
`;
