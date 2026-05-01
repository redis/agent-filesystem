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

  table {
    table-layout: auto !important;
    width: max-content !important;
    min-width: 100% !important;
  }

  thead th,
  tbody td {
    width: auto !important;
  }

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

/**
 * Shared dense table styling used across the app.
 *  - Compact 10px row padding
 *  - Uppercase, letter-spaced, muted column headers
 *  - Click-to-edit cursor on rows when used inside a clickable table
 */
export const DenseTableViewport = styled(TableViewport)`
  tbody tr {
    cursor: pointer;
  }

  tbody td {
    padding-top: 10px !important;
    padding-bottom: 10px !important;
    vertical-align: middle;
  }

  thead th {
    padding-top: 10px !important;
    padding-bottom: 10px !important;
    font-size: 10.5px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--afs-muted, #71717a);
  }
`;

export const RegistryTableViewport = DenseTableViewport;

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
    border-color: var(--afs-focus);
    box-shadow: 0 0 0 3px var(--afs-focus-soft);
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

/**
 * Wraps a table's search/toolbar row and table card so they share
 * a consistent vertical gap. Use this when a table has a search box
 * above the table body — it keeps the search-to-table spacing uniform
 * across PageStack and SectionCard contexts.
 */
export const TableBlock = styled.div`
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
`;

/* ---- View toggle ---- */
export const ToggleGroup = styled.div`
  display: inline-flex;
  gap: 2px;
  padding: 3px;
  border-radius: 10px;
  background: var(--afs-panel);
  border: 1px solid var(--afs-line);
`;

export const ToggleButton = styled.button<{ $active: boolean }>`
  border: none;
  border-radius: 7px;
  padding: 6px 14px;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
  color: ${({ $active }) => ($active ? "var(--afs-accent, #2563eb)" : "var(--afs-muted, #71717a)")};
  background: ${({ $active }) => ($active ? "var(--afs-accent-soft, rgba(37, 99, 235, 0.1))" : "transparent")};
  transition: background 160ms ease, color 160ms ease;

  &:hover {
    color: ${({ $active }) => ($active ? "var(--afs-accent, #2563eb)" : "var(--afs-ink, #18181b)")};
    background: ${({ $active }) => ($active ? "var(--afs-accent-soft, rgba(37, 99, 235, 0.1))" : "var(--afs-focus-soft, #f0f0f0)")};
  }

  &:focus-visible {
    outline: 2px solid var(--afs-focus);
    outline-offset: 2px;
  }
`;

/* ================================================================== */
/*  Workspace cards                                                    */
/* ================================================================== */

export const WorkspaceCardGrid = styled.div`
  display: grid;
  gap: 22px;
  grid-template-columns: repeat(auto-fill, minmax(max(25%, 360px), 1fr));
  padding: 16px 0;
`;

export const WorkspaceCard = styled.div`
  display: flex;
  flex-direction: column;
  border-radius: 16px;
  background: var(--afs-panel-strong);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
  overflow: hidden;
  cursor: pointer;
  transition: box-shadow 200ms ease, transform 200ms ease;

  &:hover {
    box-shadow: 0 10px 28px rgba(0, 0, 0, 0.12);
    transform: translateY(-2px);
  }
`;

/* ---- Top row: icon box + name ---- */
export const CardTopRow = styled.div`
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 16px 0;
`;

export const CardIconBox = styled.div`
  display: flex;
  align-items: center;
  justify-content: center;
  width: 52px;
  height: 52px;
  flex-shrink: 0;
  border-radius: 13px;
  background: var(--afs-bg-soft, #f0f0f0);
  color: var(--afs-ink-soft, #52525b);

  svg {
    width: 28px;
    height: 28px;
  }
`;

/* ---- Card body ---- */
export const CardBody = styled.div`
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px 16px 14px;
`;

/* ---- Name + description ---- */
export const CardName = styled.span`
  font-size: 20px;
  font-weight: 800;
  color: var(--afs-ink, #18181b);
  line-height: 1.2;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

export const CardDescription = styled.span`
  font-size: 11px;
  color: var(--afs-muted, #71717a);
  line-height: 1.4;
`;

/* ---- Detail lines (Database, ID) ---- */
export const CardDetailLines = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
`;

export const CardDetailLine = styled.div`
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 11px;
  line-height: 1.4;
`;

export const CardDetailLabel = styled.span`
  font-weight: 700;
  color: var(--afs-ink, #18181b);
  white-space: nowrap;
`;

export const CardDetailValue = styled.span`
  color: var(--afs-muted, #71717a);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
`;

/* ---- Stat boxes row ---- */
export const CardStatsRow = styled.div`
  display: flex;
  gap: 6px;
`;

export const CardStatBox = styled.div`
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1px;
  padding: 8px 4px;
  border: 1px solid var(--afs-line, #e4e4e7);
  border-radius: 10px;
  background: var(--afs-panel, #fafafa);
`;

export const CardStatLabel = styled.span`
  font-size: 9px;
  font-weight: 600;
  color: var(--afs-muted, #71717a);
  text-transform: capitalize;
  white-space: nowrap;
`;

export const CardStatValue = styled.span`
  font-size: 16px;
  font-weight: 800;
  color: var(--afs-ink, #18181b);
  line-height: 1.2;
`;

/* ---- Info boxes (agents + updated) ---- */
export const CardInfoRow = styled.div`
  display: flex;
  gap: 6px;
`;

export const CardInfoBox = styled.div<{ $highlight?: boolean }>`
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px 6px;
  border: 1px solid var(--afs-line, #e4e4e7);
  border-radius: 10px;
  background: var(--afs-panel, #fafafa);
  font-size: 11px;
  font-weight: 700;
  color: ${({ $highlight }) => ($highlight ? "#16a34a" : "var(--afs-muted, #71717a)")};
`;

/* ---- Action buttons row ---- */
export const CardButtonRow = styled.div`
  display: flex;
  gap: 8px;
  padding: 0 16px 16px;
`;

export const CardPrimaryButton = styled.button`
  flex: 1;
  padding: 10px 0;
  border: none;
  border-radius: 11px;
  background: #78716c;
  color: #fff;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
  transition: background 180ms ease;

  &:hover {
    background: #57534e;
  }
`;

export const CardSecondaryButton = styled.button`
  flex: 1;
  padding: 10px 0;
  border: none;
  border-radius: 11px;
  background: var(--afs-line, #e7e5e4);
  color: var(--afs-ink, #57534e);
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
  transition: background 180ms ease;

  &:hover {
    background: var(--afs-line-strong, #d6d3d1);
  }
`;
