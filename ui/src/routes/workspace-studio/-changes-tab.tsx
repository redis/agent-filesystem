import { useMemo } from "react";
import styled from "styled-components";
import {
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
} from "../../components/afs-kit";
import { computeChangelogTotals, formatChangelogBytes } from "../../foundation/changelog-utils";
import { ChangesTable } from "../../foundation/tables/changes-table";
import { useChangelog } from "../../foundation/hooks/use-afs";

type Props = {
  databaseId?: string;
  workspaceId: string;
};

export function ChangesTab({ databaseId, workspaceId }: Props) {
  const query = useChangelog({
    databaseId,
    workspaceId,
    limit: 200,
    direction: "desc",
  });

  const entries = query.data?.entries ?? [];
  const totals = useMemo(() => computeChangelogTotals(entries), [entries]);
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
                <PositiveDelta>+{formatChangelogBytes(totals.bytesAdded)}</PositiveDelta>
                {" / "}
                <NegativeDelta>−{formatChangelogBytes(totals.bytesRemoved)}</NegativeDelta>
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
