import { Button } from "@redis-ui/components";
import { useMemo, useState } from "react";
import styled from "styled-components";
import {
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
} from "../../components/afs-kit";
import { computeChangelogTotals, formatChangelogBytes } from "../../foundation/changelog-utils";
import { useInfiniteChangelog } from "../../foundation/hooks/use-afs";
import type { AFSChangelogEntry } from "../../foundation/types/afs";
import { ChangesTable } from "../../foundation/tables/changes-table";
import { FileHistoryDrawer } from "./-file-history-drawer";

const CHANGELOG_PAGE_SIZE = 100;

type Props = {
  databaseId?: string;
  workspaceId: string;
  editable: boolean;
};

export function ChangesTab({ databaseId, workspaceId, editable }: Props) {
  const [selectedChange, setSelectedChange] = useState<AFSChangelogEntry | null>(null);
  const query = useInfiniteChangelog({
    databaseId,
    workspaceId,
    limit: CHANGELOG_PAGE_SIZE,
    direction: "desc",
  });

  const entries = useMemo(
    () => query.data?.pages.flatMap((page) => page.entries) ?? [],
    [query.data],
  );
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
                Showing <strong>{entries.length}</strong> recent changes ·{" "}
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
          onOpenChange={(entry) => setSelectedChange(entry)}
        />
        {!query.isLoading && !query.isError && hasEntries && query.hasNextPage ? (
          <LoadMoreRow>
            <Button
              size="medium"
              variant="secondary-fill"
              onClick={() => void query.fetchNextPage()}
              disabled={query.isFetchingNextPage}
            >
              {query.isFetchingNextPage ? "Loading more…" : "Load more changes"}
            </Button>
          </LoadMoreRow>
        ) : null}
      </SectionCard>

      {selectedChange ? (
        <FileHistoryDrawer
          databaseId={databaseId}
          workspaceId={workspaceId}
          path={selectedChange.path}
          editable={editable}
          initialVersionId={selectedChange.versionId}
          onClose={() => setSelectedChange(null)}
        />
      ) : null}
    </SectionGrid>
  );
}

const HeaderSummary = styled.span`
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.5;
  max-width: 100%;
  min-width: 0;
  text-align: right;

  strong {
    color: var(--afs-ink);
    font-weight: 700;
  }

  @media (max-width: 720px) {
    text-align: left;
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

const LoadMoreRow = styled.div`
  display: flex;
  justify-content: center;
  padding-top: 16px;
`;
