import { Button } from "@redis-ui/components";
import { useMemo } from "react";
import styled from "styled-components";
import {
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
} from "../../components/afs-kit";
import { computeChangelogTotals, formatChangelogBytes } from "../../foundation/changelog-utils";
import { useEvents, useInfiniteChangelog } from "../../foundation/hooks/use-afs";
import { ChangesTable } from "../../foundation/tables/changes-table";
import type { HistoryTableRow } from "../../foundation/tables/changes-table";
import type { AFSEventEntry } from "../../foundation/types/afs";

const CHANGELOG_PAGE_SIZE = 100;
const LIFECYCLE_EVENT_PAGE_SIZE = 100;

type Props = {
  databaseId?: string;
  workspaceId: string;
};

export function HistoryTab({ databaseId, workspaceId }: Props) {
  const changelogQuery = useInfiniteChangelog({
    databaseId,
    workspaceId,
    limit: CHANGELOG_PAGE_SIZE,
    direction: "desc",
  });
  const eventsQuery = useEvents({
    databaseId,
    workspaceId,
    limit: LIFECYCLE_EVENT_PAGE_SIZE,
    direction: "desc",
  });

  const entries = useMemo(
    () => changelogQuery.data?.pages.flatMap((page) => page.entries) ?? [],
    [changelogQuery.data],
  );
  const lifecycleRows = useMemo(
    () => (eventsQuery.data?.items ?? []).flatMap(eventToHistoryRow),
    [eventsQuery.data],
  );
  const historyRows = useMemo(
    () =>
      [
        ...entries.map((entry): HistoryTableRow => ({ ...entry, historyType: "file" })),
        ...lifecycleRows,
      ].sort((left, right) => {
        const leftTime = Date.parse(left.occurredAt ?? "") || 0;
        const rightTime = Date.parse(right.occurredAt ?? "") || 0;
        return rightTime - leftTime;
      }),
    [entries, lifecycleRows],
  );
  const totals = useMemo(() => computeChangelogTotals(entries), [entries]);
  const hasHistory = historyRows.length > 0;
  const lifecycleCount = lifecycleRows.length;
  const loading = changelogQuery.isLoading || eventsQuery.isLoading;
  const error = changelogQuery.isError || eventsQuery.isError;
  const errorMessage = changelogQuery.error instanceof Error
    ? changelogQuery.error.message
    : eventsQuery.error instanceof Error
      ? eventsQuery.error.message
      : "Unable to load history. Please retry.";

  return (
    <SectionGrid>
      <SectionCard $span={12}>
        <SectionHeader>
          <SectionTitle title="History" />
          <HeaderSummary>
            {hasHistory ? (
              <>
                Showing <strong>{historyRows.length}</strong> recent rows ·{" "}
                <strong>{entries.length}</strong> file changes ·{" "}
                <strong>{lifecycleCount}</strong> events ·{" "}
                <strong>{totals.added}</strong> added ·{" "}
                <strong>{totals.modified}</strong> modified ·{" "}
                <strong>{totals.deleted}</strong> deleted ·{" "}
                <PositiveDelta>+{formatChangelogBytes(totals.bytesAdded)}</PositiveDelta>
                {" / "}
                <NegativeDelta>−{formatChangelogBytes(totals.bytesRemoved)}</NegativeDelta>
              </>
            ) : (
              "No history yet"
            )}
          </HeaderSummary>
        </SectionHeader>
        <ChangesTable
          rows={historyRows}
          loading={loading}
          error={error}
          errorMessage={errorMessage}
          emptyStateText="No history has been recorded for this workspace yet."
          detailHeader="Path / Detail"
          filterAllLabel="All history"
          loadingText="Loading history..."
          searchPlaceholder="Search by path, event, agent, user..."
        />
        {!loading && !error && entries.length > 0 && changelogQuery.hasNextPage ? (
          <LoadMoreRow>
            <Button
              size="medium"
              variant="secondary-fill"
              onClick={() => void changelogQuery.fetchNextPage()}
              disabled={changelogQuery.isFetchingNextPage}
            >
              {changelogQuery.isFetchingNextPage ? "Loading more..." : "Load more file changes"}
            </Button>
          </LoadMoreRow>
        ) : null}
      </SectionCard>
    </SectionGrid>
  );
}

function eventToHistoryRow(event: AFSEventEntry): HistoryTableRow[] {
  if (event.kind === "file") {
    return [];
  }

  return [{
    id: `event:${event.id}`,
    occurredAt: event.createdAt,
    workspaceId: event.workspaceId,
    workspaceName: event.workspaceName,
    databaseId: event.databaseId,
    databaseName: event.databaseName,
    sessionId: event.sessionId,
    user: event.user,
    label: event.label,
    op: event.op,
    path: event.path,
    prevPath: event.prevPath,
    sizeBytes: event.sizeBytes,
    deltaBytes: event.deltaBytes,
    contentHash: event.contentHash,
    prevHash: event.prevHash,
    mode: event.mode,
    checkpointId: event.checkpointId,
    source: event.source,
    actor: event.actor,
    eventDetail: event.extras?.detail,
    eventTitle: event.extras?.title,
    historyType: "event",
    hostname: event.hostname,
    kind: event.kind,
  }];
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

const LoadMoreRow = styled.div`
  display: flex;
  justify-content: center;
  padding-top: 16px;
`;
