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

export const RegistryTableViewport = styled(TableViewport)`
  /* Remove vertical separator between Health | Workspace (columns 1|2). */
  thead tr > *:nth-child(1),
  [role="row"] > [role="columnheader"]:nth-child(1) {
    border-right: none !important;
    box-shadow: none !important;
  }

  thead tr > *:nth-child(2),
  [role="row"] > [role="columnheader"]:nth-child(2) {
    border-left: none !important;
  }

  /* Remove vertical separator between Updated | More actions (columns 7|8). */
  thead tr > *:nth-child(7),
  [role="row"] > [role="columnheader"]:nth-child(7) {
    border-right: none !important;
    box-shadow: none !important;
  }

  thead tr > *:nth-child(8),
  [role="row"] > [role="columnheader"]:nth-child(8) {
    border-left: none !important;
  }

  thead tr > *:nth-child(1)::before,
  thead tr > *:nth-child(1)::after,
  thead tr > *:nth-child(7)::before,
  thead tr > *:nth-child(7)::after,
  [role="row"] > [role="columnheader"]:nth-child(1)::before,
  [role="row"] > [role="columnheader"]:nth-child(1)::after,
  [role="row"] > [role="columnheader"]:nth-child(7)::before,
  [role="row"] > [role="columnheader"]:nth-child(7)::after {
    border: 0 !important;
    background: transparent !important;
    box-shadow: none !important;
    content: none !important;
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

export const StatusCaption = styled.span`
  color: ${({ theme }) => theme.semantic.color.text.neutral600};
  font-size: 12px;
`;

export const ActionRow = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
`;

export const CountCell = styled.div`
  display: flex;
  align-items: center;
  gap: 6px;
`;

export const MetaBadge = styled.span`
  display: inline-flex;
  align-items: center;
  padding: 2px 6px;
  border-radius: 999px;
  background: ${({ theme }) => theme.semantic.color.background.neutral100};
  color: ${({ theme }) => theme.semantic.color.text.neutral700};
  font-size: 11px;
  font-weight: 600;
  line-height: 1.4;
`;

export const HealthDot = styled.span<{ $active: boolean; $syncing?: boolean }>`
  width: 8px;
  height: 8px;
  min-width: 8px;
  border-radius: 999px;
  display: inline-block;
  background-color: ${({ $active, $syncing, theme }) =>
    !$active
      ? theme.semantic.color.background.danger500
      : $syncing
        ? theme.semantic.color.background.warning500
        : theme.semantic.color.background.success500};
`;

export const HealthCell = styled.div`
  display: flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  min-width: 16px;
  overflow: visible;
`;

const actionButtonBase = styled.button`
  border: none;
  background: transparent;
  padding: 0;
  font: inherit;
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
  transition: opacity 160ms ease;

  &:disabled {
    cursor: default;
    opacity: 0.45;
  }
`;

export const TextActionButton = styled(actionButtonBase)`
  color: ${({ theme }) => theme.semantic.color.text.primary600};
`;

export const DangerActionButton = styled(actionButtonBase)`
  color: #c2364a;
`;

export const MoreActionsTrigger = styled.button`
  border: none;
  background: transparent;
  cursor: pointer;
  width: 24px;
  height: 24px;
  border-radius: 4px;
  display: inline-flex;
  align-items: center;
  justify-content: center;

  &:hover {
    background: ${({ theme }) => theme.semantic.color.background.neutral100};
  }
`;

export const SearchInput = styled(TableHeading.SearchInput)`
  && {
    align-self: stretch;
  }

  flex: 1 1 320px;
  min-width: 0;
  width: 100%;
  border-radius: 8px !important;
  border: 1px solid ${({ theme }) => theme.semantic.color.border.neutral200} !important;
  background: ${({ theme }) => theme.semantic.color.background.neutral0} !important;
  box-shadow: none !important;
`;

export const HeadingWrap = styled.div`
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
  padding: 18px 20px 14px;
`;

export const SearchOnlyHeadingWrap = styled(HeadingWrap)`
  justify-content: flex-start;
`;
