import styled from "styled-components";
import { TableHeading } from "@redis-ui/components";

export const TableCard = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 8px;
  overflow: hidden;
  background: var(--afs-panel-strong);
`;

export const TableViewport = styled.div`
  max-height: 720px;
  overflow: auto;

  thead th {
    position: sticky;
    top: 0;
    z-index: 2;
    background: var(--afs-panel-strong);
  }

  tbody tr {
    transition: background 160ms ease;
  }

  tbody tr:hover {
    background: var(--afs-panel);
  }
`;

export const RegistryTableViewport = styled(TableViewport)`
  /* Remove vertical separator between Updated | More actions (last two columns). */
  thead tr > *:nth-child(7),
  [role="row"] > [role="columnheader"]:nth-child(7) {
    border-right: none !important;
    box-shadow: none !important;
  }

  thead tr > *:nth-child(8),
  [role="row"] > [role="columnheader"]:nth-child(8) {
    border-left: none !important;
  }

  thead tr > *:nth-child(7)::before,
  thead tr > *:nth-child(7)::after,
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
  color: var(--afs-muted);
`;

export const WorkspaceNameButton = styled.button`
  border: none;
  background: transparent;
  padding: 0;
  color: var(--afs-ink);
  font: inherit;
  font-weight: 400;
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

export const SingleLineText = styled.span`
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

export const StatusCaption = styled.span`
  color: var(--afs-muted);
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
  background: var(--afs-panel);
  color: var(--afs-ink-soft);
  border: 1px solid var(--afs-line);
  font-size: 11px;
  font-weight: 600;
  line-height: 1.4;
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
  color: var(--afs-accent);
`;

export const DangerActionButton = styled(actionButtonBase)`
  color: #c2364a;
`;

export const MoreActionsTrigger = styled.button`
  border: 1px solid var(--afs-line);
  background: var(--afs-panel-strong);
  cursor: pointer;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--afs-ink-soft);
  box-shadow: 0 1px 2px rgba(8, 6, 13, 0.06);
  transition:
    background 160ms ease,
    border-color 160ms ease,
    box-shadow 160ms ease,
    transform 160ms ease;

  &:hover {
    background: var(--afs-panel);
    border-color: var(--afs-line-strong);
    box-shadow: 0 4px 12px rgba(8, 6, 13, 0.08);
    transform: translateY(-1px);
  }

  &:focus-visible {
    outline: none;
    border-color: var(--afs-accent);
    box-shadow: 0 0 0 3px var(--afs-accent-soft);
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
  border: 1px solid var(--afs-line) !important;
  background: var(--afs-panel-strong) !important;
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
