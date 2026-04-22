import { useMemo } from "react";
import styled from "styled-components";
import {
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
} from "../../components/afs-kit";
import { ChangesTable } from "../../foundation/tables/changes-table";
import { useChangelog } from "../../foundation/hooks/use-afs";
import type { AFSChangelogEntry } from "../../foundation/types/afs";

type Props = {
  databaseId?: string;
  workspaceId: string;
};

type Totals = {
  added: number;
  modified: number;
  deleted: number;
  bytesAdded: number;
  bytesRemoved: number;
};

function computeTotals(entries: AFSChangelogEntry[]): Totals {
  let added = 0;
  let modified = 0;
  let deleted = 0;
  let bytesAdded = 0;
  let bytesRemoved = 0;
  for (const entry of entries) {
    switch (entry.op) {
      case "put":
      case "symlink":
      case "mkdir":
        if (entry.prevHash) {
          modified += 1;
        } else {
          added += 1;
        }
        break;
      case "delete":
      case "rmdir":
        deleted += 1;
        break;
      case "chmod":
        modified += 1;
        break;
    }
    const delta = entry.deltaBytes ?? 0;
    if (delta > 0) bytesAdded += delta;
    if (delta < 0) bytesRemoved += -delta;
  }
  return { added, modified, deleted, bytesAdded, bytesRemoved };
}

function formatBytes(n: number): string {
  if (n === 0) return "0 B";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

export function ChangesTab({ databaseId, workspaceId }: Props) {
  const query = useChangelog({
    databaseId,
    workspaceId,
    limit: 200,
    direction: "desc",
  });

  const entries = query.data?.entries ?? [];
  const totals = useMemo(() => computeTotals(entries), [entries]);
  const hasEntries = entries.length > 0;

  return (
    <SectionGrid>
      <SectionCard $span={12}>
        <SectionHeader>
          <SectionTitle title="Changelog" />
          <HeaderSummary>
            {hasEntries ? (
              <>
                <strong>{totals.added}</strong> added ·{" "}
                <strong>{totals.modified}</strong> modified ·{" "}
                <strong>{totals.deleted}</strong> deleted ·{" "}
                <PositiveDelta>+{formatBytes(totals.bytesAdded)}</PositiveDelta>
                {" / "}
                <NegativeDelta>−{formatBytes(totals.bytesRemoved)}</NegativeDelta>
              </>
            ) : (
              "No changes yet"
            )}
          </HeaderSummary>
        </SectionHeader>
        <ChangesTable
          rows={entries}
          loading={query.isLoading}
          error={query.isError}
          errorMessage={
            query.error instanceof Error
              ? query.error.message
              : "Unable to load changes. Please retry."
          }
        />
      </SectionCard>
    </SectionGrid>
  );
}

const HeaderSummary = styled.span`
  color: var(--afs-muted);
  font-size: 13px;
  white-space: nowrap;

  strong {
    color: var(--afs-ink);
    font-weight: 700;
  }
`;

const PositiveDelta = styled.span`
  color: #16a34a;
  font-weight: 700;
`;

const NegativeDelta = styled.span`
  color: #dc2626;
  font-weight: 700;
`;
