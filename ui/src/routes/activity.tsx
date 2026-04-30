import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button } from "@redis-ui/components";
import { useMemo, useState } from "react";
import styled from "styled-components";
import { z } from "zod";
import {
  NoticeBody,
  NoticeCard,
  NoticeTitle,
  PageStack,
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
  TabButton,
  Tabs,
} from "../components/afs-kit";
import { useDatabaseScope } from "../foundation/database-scope";
import { computeChangelogTotals, formatChangelogBytes } from "../foundation/changelog-utils";
import { useChangelog, useEvents } from "../foundation/hooks/use-afs";
import { ChangesTable } from "../foundation/tables/changes-table";
import { EventsTable } from "../foundation/tables/events-table";
import type { AFSChangelogEntry, AFSEventEntry } from "../foundation/types/afs";

const ACTIVITY_PAGE_SIZE = 25;

const activitySearchSchema = z.object({
  view: z.enum(["changes", "events"]).optional(),
});

export const Route = createFileRoute("/activity")({
  validateSearch: activitySearchSchema,
  component: ActivityPage,
});

function ActivityPage() {
  const navigate = useNavigate();
  const search = Route.useSearch();
  const { unavailableDatabases } = useDatabaseScope();
  const view = search.view ?? "changes";
  const [changelogCursors, setChangelogCursors] = useState<string[]>([]);
  const [eventCursors, setEventCursors] = useState<string[]>([]);
  const changelogCursor = changelogCursors[changelogCursors.length - 1];
  const eventCursor = eventCursors[eventCursors.length - 1];

  const eventsQuery = useEvents(
    {
      limit: ACTIVITY_PAGE_SIZE,
      direction: "desc",
      until: eventCursor,
    },
    view === "events",
  );
  const changelogQuery = useChangelog(
    {
      limit: ACTIVITY_PAGE_SIZE,
      direction: "desc",
      until: changelogCursor,
    },
    view === "changes",
  );

  const changelogEntries = changelogQuery.data?.entries ?? [];
  const eventRows = eventsQuery.data?.items ?? [];
  const changelogTotals = useMemo(
    () => computeChangelogTotals(changelogEntries),
    [changelogEntries],
  );
  const hasChangelogEntries = changelogEntries.length > 0;
  const canPageChangelogNext = Boolean(changelogQuery.data?.nextCursor) && changelogEntries.length === ACTIVITY_PAGE_SIZE;
  const canPageEventsNext = Boolean(eventsQuery.data?.nextCursor) && eventRows.length === ACTIVITY_PAGE_SIZE;

  function setView(nextView: "changes" | "events") {
    void navigate({
      to: "/activity",
      search: nextView === "changes" ? {} : { view: nextView },
      replace: true,
    });
  }

  function openEvent(event: AFSEventEntry) {
    if (event.workspaceId == null) {
      return;
    }

    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: event.workspaceId },
      search: {
        ...(event.databaseId ? { databaseId: event.databaseId } : {}),
        ...(event.kind === "checkpoint"
          ? { tab: "checkpoints" }
          : event.kind === "file"
            ? { tab: "browse" }
            : event.kind === "workspace"
              ? {}
              : { tab: "activity" }),
      },
    });
  }

  function openChange(entry: AFSChangelogEntry) {
    if (entry.workspaceId == null) {
      return;
    }

    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: entry.workspaceId },
      search: {
        ...(entry.databaseId ? { databaseId: entry.databaseId } : {}),
        tab: "changes",
      },
    });
  }

  return (
    <PageStack>
      {unavailableDatabases.length > 0 ? (
        <NoticeCard $tone="warning" role="status">
          <NoticeTitle>Some databases are unavailable</NoticeTitle>
          <NoticeBody>
            {view === "changes" ? "Changelog" : "Events"} below are partial while these databases are disconnected:{" "}
            {unavailableDatabases.map((database) => database.displayName || database.databaseName).join(", ")}.
          </NoticeBody>
        </NoticeCard>
      ) : null}

      <Tabs role="tablist" aria-label="Activity filters">
        <TabButton
          type="button"
          role="tab"
          aria-selected={view === "changes"}
          $active={view === "changes"}
          onClick={() => setView("changes")}
        >
          Changelog
        </TabButton>
        <TabButton
          type="button"
          role="tab"
          aria-selected={view === "events"}
          $active={view === "events"}
          onClick={() => setView("events")}
        >
          Events
        </TabButton>
      </Tabs>

      {view === "changes" ? (
        <SectionGrid>
          <SectionCard $span={12}>
            <SectionHeader>
              <SectionTitle title="Changelog" />
              <HeaderSummary>
                {changelogQuery.isLoading ? (
                  "Loading changelog…"
                ) : hasChangelogEntries ? (
                  <>
                    Showing <strong>{changelogEntries.length}</strong> changes on this page ·{" "}
                    <strong>{changelogTotals.added}</strong> added ·{" "}
                    <strong>{changelogTotals.modified}</strong> modified ·{" "}
                    <strong>{changelogTotals.deleted}</strong> deleted ·{" "}
                    <PositiveDelta>+{formatChangelogBytes(changelogTotals.bytesAdded)}</PositiveDelta>
                    {" / "}
                    <NegativeDelta>−{formatChangelogBytes(changelogTotals.bytesRemoved)}</NegativeDelta>
                  </>
                ) : (
                  "No changes yet"
                )}
              </HeaderSummary>
            </SectionHeader>
            <ChangesTable
              rows={changelogEntries}
              loading={changelogQuery.isLoading}
              error={changelogQuery.isError}
              errorMessage={
                changelogQuery.error instanceof Error
                  ? changelogQuery.error.message
                  : "Unable to load changes. Please retry."
              }
              emptyStateText="No changes have been recorded for any workspace yet."
              onOpenChange={openChange}
            />
            {!changelogQuery.isLoading &&
            !changelogQuery.isError &&
            hasChangelogEntries &&
            (changelogCursors.length > 0 || canPageChangelogNext) ? (
              <PaginationControls
                onPrevious={() => setChangelogCursors((cursors) => cursors.slice(0, -1))}
                onNext={() => {
                  const nextCursor = changelogQuery.data?.nextCursor;
                  if (nextCursor) {
                    setChangelogCursors((cursors) => [...cursors, nextCursor]);
                  }
                }}
                previousDisabled={changelogCursors.length === 0}
                nextDisabled={!canPageChangelogNext}
                loading={changelogQuery.isFetching}
              />
            ) : null}
          </SectionCard>
        </SectionGrid>
      ) : null}

      {view === "events" ? (
        <>
          <EventsTable
            rows={eventRows}
            loading={eventsQuery.isLoading}
            error={eventsQuery.isError}
            errorMessage={eventsQuery.error instanceof Error ? eventsQuery.error.message : undefined}
            onOpenEvent={openEvent}
          />
          {!eventsQuery.isLoading &&
          !eventsQuery.isError &&
          eventRows.length > 0 &&
          (eventCursors.length > 0 || canPageEventsNext) ? (
            <PaginationControls
              onPrevious={() => setEventCursors((cursors) => cursors.slice(0, -1))}
              onNext={() => {
                const nextCursor = eventsQuery.data?.nextCursor;
                if (nextCursor) {
                  setEventCursors((cursors) => [...cursors, nextCursor]);
                }
              }}
              previousDisabled={eventCursors.length === 0}
              nextDisabled={!canPageEventsNext}
              loading={eventsQuery.isFetching}
            />
          ) : null}
        </>
      ) : null}
    </PageStack>
  );
}

type PaginationControlsProps = {
  previousDisabled: boolean;
  nextDisabled: boolean;
  loading: boolean;
  onPrevious: () => void;
  onNext: () => void;
};

function PaginationControls({
  previousDisabled,
  nextDisabled,
  loading,
  onPrevious,
  onNext,
}: PaginationControlsProps) {
  return (
    <PaginationRow>
      <Button
        size="medium"
        variant="secondary-fill"
        onClick={onPrevious}
        disabled={previousDisabled || loading}
      >
        Previous
      </Button>
      <Button
        size="medium"
        variant="secondary-fill"
        onClick={onNext}
        disabled={nextDisabled || loading}
      >
        Next
      </Button>
    </PaginationRow>
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

const PaginationRow = styled.div`
  display: flex;
  gap: 8px;
  justify-content: center;
  padding-top: 16px;
`;
